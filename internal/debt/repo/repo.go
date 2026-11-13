// Package repo implements debt.Repository.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	debtRow              = sqlitegen.Debt
	upsertParams         = sqlitegen.UpsertDebtParams
	ruleParams           = sqlitegen.InsertDebtRuleParams
	snapshotParams       = sqlitegen.InsertDebtSnapshotParams
	updateSnapshotParams = sqlitegen.UpdateDebtSnapshotParams
	splitParams          = sqlitegen.InsertDebtSplitParams
)

type querier interface {
	LockBudget(ctx context.Context, db backend.DBTX, budgetID string) error
	GetDebtByID(ctx context.Context, db backend.DBTX, id string) (debtRow, error)
	ListDebtsByBudget(ctx context.Context, db backend.DBTX, budgetID string) ([]debtRow, error)
	UpsertDebt(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteDebt(ctx context.Context, db backend.DBTX, id string) error
	DeleteDebtRules(ctx context.Context, db backend.DBTX, debtID string) error
	InsertDebtRule(ctx context.Context, db backend.DBTX, p ruleParams) error
	InsertDebtSnapshot(ctx context.Context, db backend.DBTX, p snapshotParams) error
	UpdateDebtSnapshot(ctx context.Context, db backend.DBTX, p updateSnapshotParams) error
	DeleteDebtSnapshot(ctx context.Context, db backend.DBTX, id string) error
	InsertDebtSplit(ctx context.Context, db backend.DBTX, p splitParams) error
}

type Repo struct {
	tx     *backend.TxManager
	q      querier
	driver string
}

var _ debt.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("debtrepo: unknown database driver " + driver)
	}
}

func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}, driver: "sqlite"}
}

func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}, driver: "postgresql"}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

type heldBudgetsKey struct{}

type heldBudgets struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (r *Repo) LockBudget(ctx context.Context, budgetID vo.Id) error {
	if err := r.q.LockBudget(ctx, r.db(ctx), budgetID.String()); err != nil {
		return err
	}
	if v, ok := r.tx.TxValue(ctx, heldBudgetsKey{}, func() any { return &heldBudgets{ids: map[string]bool{}} }); ok {
		held := v.(*heldBudgets)
		held.mu.Lock()
		held.ids[budgetID.String()] = true
		held.mu.Unlock()
	}
	return nil
}

func (r *Repo) HoldsBudgetLock(ctx context.Context, budgetID vo.Id) bool {
	v, ok := r.tx.TxValue(ctx, heldBudgetsKey{}, func() any { return &heldBudgets{ids: map[string]bool{}} })
	if !ok {
		return false
	}
	held := v.(*heldBudgets)
	held.mu.Lock()
	defer held.mu.Unlock()
	return held.ids[budgetID.String()]
}

// LockRuleCategories reads the categories, key-share-locking them on
// PostgreSQL (SQLite has no row-level lock modes; its single writer already
// serializes transactions). A category already deleted is not found: the
// rule write would otherwise fail its foreign key.
func (r *Repo) LockRuleCategories(ctx context.Context, categoryIDs []vo.Id) error {
	if len(categoryIDs) == 0 {
		return nil
	}
	ids := slices.Clone(categoryIDs)
	slices.SortFunc(ids, func(a, b vo.Id) int { return strings.Compare(a.String(), b.String()) })
	ids = slices.CompactFunc(ids, func(a, b vo.Id) bool { return a.Equal(b) })
	query := "SELECT id FROM categories WHERE id IN (" + placeholders(r.driver, len(ids)) + ") ORDER BY id"
	if r.driver == "postgresql" {
		query += " FOR KEY SHARE"
	}
	rows, err := r.db(ctx).QueryContext(ctx, query, idArgs(ids)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		found++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found < len(ids) {
		return errs.NewNotFound("Category not found")
	}
	return nil
}

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.Debt, error) {
	row, err := r.q.GetDebtByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Debt not found")
		}
		return nil, err
	}
	out, err := r.hydrateAll(ctx, []debtRow{row})
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

func (r *Repo) ListByBudget(ctx context.Context, budgetID vo.Id) ([]*model.Debt, error) {
	rows, err := r.q.ListDebtsByBudget(ctx, r.db(ctx), budgetID.String())
	if err != nil {
		return nil, err
	}
	return r.hydrateAll(ctx, rows)
}

// Save: the caller runs this inside TxManager.WithTx, so the rules are
// replaced atomically with the row.
func (r *Repo) Save(ctx context.Context, d *model.Debt) error {
	p := upsertParams{
		ID:         d.ID.String(),
		BudgetID:   d.BudgetID.String(),
		Name:       d.Name,
		Icon:       d.Icon,
		CurrencyID: d.CurrencyID.String(),
		Payment:    d.Payment.String(),
		Schedule:   string(d.Schedule),
		CreatedDay: dateText(d.CreatedDay),
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}
	if c := d.Current; c != nil {
		asOf, balance, last := dateText(c.AsOf), c.Balance.String(), dateText(c.LastPaymentDate)
		p.AsOf, p.Balance, p.LastPaymentDate = &asOf, &balance, &last
	}
	if d.StartDate != nil {
		s := dateText(*d.StartDate)
		p.StartDate = &s
	}
	if d.IsSetUp() {
		borrowed := d.AmountBorrowed.String()
		p.AmountBorrowed = &borrowed
	}
	if err := r.q.UpsertDebt(ctx, r.db(ctx), p); err != nil {
		return err
	}
	if err := r.q.DeleteDebtRules(ctx, r.db(ctx), d.ID.String()); err != nil {
		return err
	}
	for _, rule := range d.Rules {
		err := r.q.InsertDebtRule(ctx, r.db(ctx), ruleParams{
			ID: rule.ID.String(), DebtID: d.ID.String(), CategoryID: rule.CategoryID.String(),
			PayeeID: idPtr(rule.PayeeID), TagID: idPtr(rule.TagID), LabelID: idPtr(rule.LabelID), CreatedAt: rule.CreatedAt,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteDebt(ctx, r.db(ctx), id.String())
}

func (r *Repo) AddSnapshot(ctx context.Context, s *model.DebtSnapshot) error {
	return r.q.InsertDebtSnapshot(ctx, r.db(ctx), snapshotParams{
		ID:              s.ID.String(),
		DebtID:          s.DebtID.String(),
		AsOf:            dateText(s.AsOf),
		Balance:         s.Balance.String(),
		Payment:         s.Payment.String(),
		Schedule:        string(s.Schedule),
		LastPaymentDate: dateText(s.LastPaymentDate),
		CreatedAt:       s.CreatedAt,
	})
}

func (r *Repo) UpdateSnapshot(ctx context.Context, s *model.DebtSnapshot) error {
	return r.q.UpdateDebtSnapshot(ctx, r.db(ctx), updateSnapshotParams{
		ID:              s.ID.String(),
		AsOf:            dateText(s.AsOf),
		Balance:         s.Balance.String(),
		Payment:         s.Payment.String(),
		Schedule:        string(s.Schedule),
		LastPaymentDate: dateText(s.LastPaymentDate),
	})
}

func (r *Repo) DeleteSnapshot(ctx context.Context, id vo.Id) error {
	return r.q.DeleteDebtSnapshot(ctx, r.db(ctx), id.String())
}

func (r *Repo) InsertSplit(ctx context.Context, s model.DebtSplit) error {
	return r.q.InsertDebtSplit(ctx, r.db(ctx), splitParams{
		DebtID:        s.DebtID.String(),
		TransactionID: s.TransactionID.String(),
		Interest:      s.Interest.String(),
	})
}

// The id-list reads below are hand-built (variadic IN); only the placeholders
// differ per engine.

func (r *Repo) CandidateTransactions(ctx context.Context, categoryIDs, accountIDs []vo.Id) ([]debt.CandidateTx, error) {
	if len(categoryIDs) == 0 || len(accountIDs) == 0 {
		return nil, nil
	}
	where := " WHERE t.type = 0 AND t.category_id IN (" + placeholders(r.driver, len(categoryIDs)) +
		") AND t.account_id IN (" + placeholdersFrom(r.driver, len(categoryIDs)+1, len(accountIDs)) + ")"
	args := append(idArgs(categoryIDs), idArgs(accountIDs)...)
	labels, err := r.candidateLabels(ctx, where, args)
	if err != nil {
		return nil, err
	}
	query := "SELECT t.id, t.spent_at, t.amount, a.currency_id, t.category_id, t.payee_id, t.tag_id FROM transactions t JOIN accounts a ON a.id = t.account_id" +
		where + " ORDER BY t.spent_at, t.id"
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []debt.CandidateTx
	for rows.Next() {
		var id, amount, currency, category string
		var payee, tag *string
		var spentAt time.Time
		if err := rows.Scan(&id, &spentAt, &amount, &currency, &category, &payee, &tag); err != nil {
			return nil, err
		}
		c := debt.CandidateTx{SpentAt: spentAt, Amount: vo.NewDecimal(amount), LabelIDs: labels[id]}
		if c.ID, err = vo.ParseId(id); err != nil {
			return nil, err
		}
		if c.CurrencyID, err = vo.ParseId(currency); err != nil {
			return nil, err
		}
		if c.CategoryID, err = vo.ParseId(category); err != nil {
			return nil, err
		}
		if c.PayeeID, err = parseIdPtr(payee); err != nil {
			return nil, err
		}
		if c.TagID, err = parseIdPtr(tag); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// candidateLabels reads the labels of the expenses CandidateTransactions
// returns, in one query.
func (r *Repo) candidateLabels(ctx context.Context, where string, args []any) (map[string][]vo.Id, error) {
	query := "SELECT tl.transaction_id, tl.label_id FROM transactions_labels tl JOIN transactions t ON t.id = tl.transaction_id" +
		where + " ORDER BY tl.transaction_id, tl.label_id"
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]vo.Id{}
	for rows.Next() {
		var txID, labelID string
		if err := rows.Scan(&txID, &labelID); err != nil {
			return nil, err
		}
		lid, err := vo.ParseId(labelID)
		if err != nil {
			return nil, err
		}
		out[txID] = append(out[txID], lid)
	}
	return out, rows.Err()
}

var classificationColumns = map[string]string{
	"category": "category_id",
	"payee":    "payee_id",
	"tag":      "tag_id",
	"label":    "label_id",
}

func (r *Repo) ListByClassification(ctx context.Context, kind string, id vo.Id) ([]*model.Debt, error) {
	column, ok := classificationColumns[kind]
	if !ok {
		return nil, fmt.Errorf("debtrepo: unknown classification kind %q", kind)
	}
	return r.listDebts(ctx, " WHERE id IN (SELECT debt_id FROM debts_rules WHERE "+column+" = "+placeholder(r.driver, 1)+")",
		id.String())
}

func (r *Repo) ListByBudgets(ctx context.Context, budgetIDs []vo.Id) ([]*model.Debt, error) {
	if len(budgetIDs) == 0 {
		return nil, nil
	}
	return r.listDebts(ctx, " WHERE budget_id IN ("+placeholders(r.driver, len(budgetIDs))+")", idArgs(budgetIDs)...)
}

const debtCols = "id, budget_id, name, icon, currency_id, start_date, amount_borrowed, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at"

func (r *Repo) listDebts(ctx context.Context, where string, args ...any) ([]*model.Debt, error) {
	rows, err := r.db(ctx).QueryContext(ctx, "SELECT "+debtCols+" FROM debts"+where+" ORDER BY created_at, id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scanned []debtRow
	for rows.Next() {
		var row debtRow
		if err := rows.Scan(&row.ID, &row.BudgetID, &row.Name, &row.Icon, &row.CurrencyID, &row.StartDate, &row.AmountBorrowed, &row.AsOf,
			&row.Balance, &row.Payment, &row.Schedule, &row.LastPaymentDate, &row.CreatedDay, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.hydrateAll(ctx, scanned)
}

// rulesOf reads the rules of the debts, each debt's in created_at, id order.
func (r *Repo) rulesOf(ctx context.Context, debtIDs []vo.Id) (map[vo.Id][]model.DebtRule, error) {
	out := map[vo.Id][]model.DebtRule{}
	if len(debtIDs) == 0 {
		return out, nil
	}
	query := "SELECT id, debt_id, category_id, payee_id, tag_id, label_id, created_at FROM debts_rules WHERE debt_id IN (" +
		placeholders(r.driver, len(debtIDs)) + ") ORDER BY debt_id, created_at, id"
	rows, err := r.db(ctx).QueryContext(ctx, query, idArgs(debtIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, debtID, category string
		var payee, tag, label *string
		var created time.Time
		if err := rows.Scan(&id, &debtID, &category, &payee, &tag, &label, &created); err != nil {
			return nil, err
		}
		rule := model.DebtRule{CreatedAt: created}
		var did vo.Id
		if rule.ID, err = vo.ParseId(id); err != nil {
			return nil, err
		}
		if did, err = vo.ParseId(debtID); err != nil {
			return nil, err
		}
		if rule.CategoryID, err = vo.ParseId(category); err != nil {
			return nil, err
		}
		if rule.PayeeID, err = parseIdPtr(payee); err != nil {
			return nil, err
		}
		if rule.TagID, err = parseIdPtr(tag); err != nil {
			return nil, err
		}
		if rule.LabelID, err = parseIdPtr(label); err != nil {
			return nil, err
		}
		out[did] = append(out[did], rule)
	}
	return out, rows.Err()
}

func (r *Repo) Snapshots(ctx context.Context, debtIDs []vo.Id) ([]*model.DebtSnapshot, error) {
	if len(debtIDs) == 0 {
		return nil, nil
	}
	query := "SELECT id, debt_id, as_of, balance, payment, schedule, last_payment_date, created_at FROM debts_snapshots WHERE debt_id IN (" +
		placeholders(r.driver, len(debtIDs)) + ") ORDER BY debt_id, as_of, created_at, id"
	rows, err := r.db(ctx).QueryContext(ctx, query, idArgs(debtIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.DebtSnapshot
	for rows.Next() {
		var id, debtID, balance, payment, schedule string
		var asOf, last, createdAt time.Time
		if err := rows.Scan(&id, &debtID, &asOf, &balance, &payment, &schedule, &last, &createdAt); err != nil {
			return nil, err
		}
		sid, err := vo.ParseId(id)
		if err != nil {
			return nil, err
		}
		did, err := vo.ParseId(debtID)
		if err != nil {
			return nil, err
		}
		out = append(out, &model.DebtSnapshot{ID: sid, DebtID: did, AsOf: day(asOf), Balance: vo.NewDecimal(balance),
			Payment: vo.NewDecimal(payment), Schedule: model.RecurringSchedule(schedule), LastPaymentDate: day(last), CreatedAt: createdAt})
	}
	return out, rows.Err()
}

func (r *Repo) Splits(ctx context.Context, debtIDs []vo.Id) ([]model.DebtSplit, error) {
	if len(debtIDs) == 0 {
		return nil, nil
	}
	query := "SELECT debt_id, transaction_id, interest FROM debts_payments WHERE debt_id IN (" +
		placeholders(r.driver, len(debtIDs)) + ") ORDER BY debt_id, transaction_id"
	rows, err := r.db(ctx).QueryContext(ctx, query, idArgs(debtIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DebtSplit
	for rows.Next() {
		var debtID, txID, interest string
		if err := rows.Scan(&debtID, &txID, &interest); err != nil {
			return nil, err
		}
		did, err := vo.ParseId(debtID)
		if err != nil {
			return nil, err
		}
		tid, err := vo.ParseId(txID)
		if err != nil {
			return nil, err
		}
		out = append(out, model.DebtSplit{DebtID: did, TransactionID: tid, Interest: vo.NewDecimal(interest)})
	}
	return out, rows.Err()
}

func (r *Repo) DeleteSplits(ctx context.Context, debtID vo.Id, transactionIDs []vo.Id) error {
	if len(transactionIDs) == 0 {
		return nil
	}
	query := "DELETE FROM debts_payments WHERE debt_id = " + placeholder(r.driver, 1) +
		" AND transaction_id IN (" + placeholdersFrom(r.driver, 2, len(transactionIDs)) + ")"
	args := append([]any{debtID.String()}, idArgs(transactionIDs)...)
	_, err := r.db(ctx).ExecContext(ctx, query, args...)
	return err
}

func (r *Repo) hydrateAll(ctx context.Context, rows []debtRow) ([]*model.Debt, error) {
	out := make([]*model.Debt, 0, len(rows))
	ids := make([]vo.Id, 0, len(rows))
	for _, row := range rows {
		d, err := hydrate(row)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
		ids = append(ids, d.ID)
	}
	rules, err := r.rulesOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, d := range out {
		d.Rules = rules[d.ID]
	}
	return out, nil
}

func hydrate(row debtRow) (*model.Debt, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	budget, err := vo.ParseId(row.BudgetID)
	if err != nil {
		return nil, err
	}
	currency, err := vo.ParseId(row.CurrencyID)
	if err != nil {
		return nil, err
	}
	var start *time.Time
	if row.StartDate != nil {
		s := day(*row.StartDate)
		start = &s
	}
	d := &model.Debt{
		ID: id, BudgetID: budget, CurrencyID: currency, Name: row.Name, Icon: row.Icon, StartDate: start,
		Payment: vo.NewDecimal(row.Payment), Schedule: model.RecurringSchedule(row.Schedule),
		CreatedDay: day(row.CreatedDay), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.AsOf != nil && row.Balance != nil && row.LastPaymentDate != nil {
		if row.AmountBorrowed != nil {
			d.AmountBorrowed = vo.NewDecimal(*row.AmountBorrowed)
		}
		d.Current = &model.DebtTerms{
			AsOf:            day(*row.AsOf),
			Balance:         vo.NewDecimal(*row.Balance),
			Payment:         d.Payment,
			Schedule:        d.Schedule,
			LastPaymentDate: day(*row.LastPaymentDate),
		}
	}
	return d, nil
}

func dateText(t time.Time) string { return t.Format(datetime.DateLayout) }

func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func idPtr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func parseIdPtr(s *string) (*vo.Id, error) {
	if s == nil {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func idArgs(ids []vo.Id) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id.String()
	}
	return args
}

func placeholder(driver string, pos int) string {
	if driver == "postgresql" {
		return "$" + strconv.Itoa(pos)
	}
	return "?"
}

func placeholders(driver string, n int) string { return placeholdersFrom(driver, 1, n) }

func placeholdersFrom(driver string, start, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = placeholder(driver, start+i)
	}
	return strings.Join(parts, ",")
}
