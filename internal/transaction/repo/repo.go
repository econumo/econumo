// Package repo implements transaction.Repository over the sqlc-generated
// queries, both engines. The static queries use the canonical (sqlite-typed)
// shim; the dynamic ListByAccountIDs (variadic IN list, optional period) is
// hand-built per engine because sqlc does not handle dynamic IN sets portably.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/transaction"
)

type (
	txRow         = sqlitegen.Transaction
	upsertParams  = sqlitegen.UpsertTransactionParams
	listAccParams = sqlitegen.ListTransactionsByAccountParams
	exportAcctRow = sqlitegen.ListExportAccountsForUserRow
)

type querier interface {
	GetTransactionByID(ctx context.Context, db backend.DBTX, id string) (txRow, error)
	UpsertTransaction(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteTransaction(ctx context.Context, db backend.DBTX, id string) error
	ListTransactionsByAccount(ctx context.Context, db backend.DBTX, p listAccParams) ([]txRow, error)
	ListExportAccountsForUser(ctx context.Context, db backend.DBTX, userID string) ([]exportAcctRow, error)
}

// ExportAccountRow is one accessible account (own + shared) with its currency
// code, for the CSV export. Field-identical across engines.
type ExportAccountRow = exportAcctRow

// ListExportAccountsForUser returns the user's accessible accounts (own + shared
// via accounts_access, not deleted) with their currency code joined.
func (r *Repo) ListExportAccountsForUser(ctx context.Context, userID vo.Id) ([]ExportAccountRow, error) {
	return r.q.ListExportAccountsForUser(ctx, r.db(ctx), userID.String())
}

// Repo implements transaction.Repository.
type Repo struct {
	tx     *backend.TxManager
	q      querier
	driver string
}

var _ domtransaction.Repository = (*Repo)(nil)

// NewRepo selects the engine adapter by driver name.
func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}, driver: driver}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}, driver: driver}
	default:
		panic("transactionrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// NextIdentity allocates a fresh transaction id.
func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

// GetByID loads a transaction by id.
func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error) {
	row, err := r.q.GetTransactionByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Transaction not found")
		}
		return nil, err
	}
	return hydrate(row)
}

// Save upserts a transaction.
func (r *Repo) Save(ctx context.Context, t *model.Transaction) error {
	return r.q.UpsertTransaction(ctx, r.db(ctx), upsertParams{
		ID:                 t.ID.String(),
		UserID:             t.UserID.String(),
		AccountID:          t.AccountID.String(),
		AccountRecipientID: idPtr(t.AccountRecipID),
		CategoryID:         idPtr(t.CategoryID),
		PayeeID:            idPtr(t.PayeeID),
		TagID:              idPtr(t.TagID),
		Description:        t.Description,
		CreatedAt:          t.CreatedAt,
		UpdatedAt:          t.UpdatedAt,
		SpentAt:            t.SpentAt,
		Type:               t.Type.Int16(),
		Amount:             t.Amount,
		AmountRecipient:    t.AmountRecipient,
	})
}

// Delete removes a transaction by id.
func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteTransaction(ctx, r.db(ctx), id.String())
}

// ListByAccount returns transactions on an account (source or recipient).
func (r *Repo) ListByAccount(ctx context.Context, accountID vo.Id) ([]*model.Transaction, error) {
	s := accountID.String()
	rows, err := r.q.ListTransactionsByAccount(ctx, r.db(ctx), listAccParams{AccountID: s, AccountRecipientID: &s})
	if err != nil {
		return nil, err
	}
	return hydrateAll(rows)
}

// ListByAccountIDs returns transactions whose source OR recipient is in
// accountIDs, optionally bounded by [periodStart, periodEnd) and narrowed by
// filter. Built by hand (dynamic IN + optional predicates); placeholders
// differ per engine. filter's zero value appends no classification predicate,
// so callers that never set it (the account/period-only paths) get exactly
// today's query.
func (r *Repo) ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, periodStart, periodEnd time.Time, filter model.TransactionFilter) ([]*model.Transaction, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	ids := make([]any, len(accountIDs))
	for i, id := range accountIDs {
		ids[i] = id.String()
	}
	usePeriod := !periodStart.IsZero() && !periodEnd.IsZero()

	const cols = "id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient"
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(cols)
	b.WriteString(" FROM transactions WHERE (account_id IN (")
	in1 := placeholders(r.driver, 1, len(ids))
	b.WriteString(in1)
	b.WriteString(") OR account_recipient_id IN (")
	in2 := placeholders(r.driver, 1+len(ids), len(ids))
	b.WriteString(in2)
	b.WriteString("))")

	args := make([]any, 0, len(ids)*2+5)
	args = append(args, ids...)
	args = append(args, ids...)
	next := 1 + 2*len(ids)
	if usePeriod {
		b.WriteString(" AND spent_at >= ")
		b.WriteString(placeholders(r.driver, next, 1))
		next++
		b.WriteString(" AND spent_at < ")
		b.WriteString(placeholders(r.driver, next, 1))
		next++
		args = append(args, periodStart, periodEnd)
	}
	if filter.Uncategorized {
		b.WriteString(" AND category_id IS NULL")
	} else if filter.CategoryID != nil {
		b.WriteString(" AND category_id = ")
		b.WriteString(placeholders(r.driver, next, 1))
		next++
		args = append(args, filter.CategoryID.String())
	}
	if filter.PayeeID != nil {
		b.WriteString(" AND payee_id = ")
		b.WriteString(placeholders(r.driver, next, 1))
		next++
		args = append(args, filter.PayeeID.String())
	}
	if filter.TagID != nil {
		b.WriteString(" AND tag_id = ")
		b.WriteString(placeholders(r.driver, next, 1))
		next++
		args = append(args, filter.TagID.String())
	}
	// Newest first (the transaction-list convention, see ListTransactionsByAccount)
	// with id as the stable tie-break: without an ORDER BY the row order diverges
	// between SQLite and PostgreSQL, and the CSV export serves this order directly.
	b.WriteString(" ORDER BY spent_at DESC, id")

	rows, err := r.db(ctx).QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Transaction
	for rows.Next() {
		var row txRow
		if serr := rows.Scan(
			&row.ID, &row.UserID, &row.AccountID, &row.AccountRecipientID, &row.CategoryID,
			&row.PayeeID, &row.TagID, &row.Description, &row.CreatedAt, &row.UpdatedAt,
			&row.SpentAt, &row.Type, &row.Amount, &row.AmountRecipient,
		); serr != nil {
			return nil, serr
		}
		t, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// placeholders builds a comma-separated placeholder list of n params starting at
// position `start`: "?,?,..." for sqlite, "$start,$start+1,..." for pgsql.
func placeholders(driver string, start, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		if driver == "postgresql" {
			parts[i] = "$" + itoa(start+i)
		} else {
			parts[i] = "?"
		}
	}
	return strings.Join(parts, ",")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func idPtr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func hydrate(row txRow) (*model.Transaction, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	accountID, err := vo.ParseId(row.AccountID)
	if err != nil {
		return nil, err
	}
	recip, err := parseOpt(row.AccountRecipientID)
	if err != nil {
		return nil, err
	}
	cat, err := parseOpt(row.CategoryID)
	if err != nil {
		return nil, err
	}
	payee, err := parseOpt(row.PayeeID)
	if err != nil {
		return nil, err
	}
	tag, err := parseOpt(row.TagID)
	if err != nil {
		return nil, err
	}
	return model.FromState(model.NewState{
		ID: id, UserID: userID, Type: model.TransactionType(row.Type), AccountID: accountID,
		AccountRecipID: recip, Amount: row.Amount, AmountRecipient: row.AmountRecipient,
		CategoryID: cat, PayeeID: payee, TagID: tag, Description: row.Description,
		SpentAt: row.SpentAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func hydrateAll(rows []txRow) ([]*model.Transaction, error) {
	out := make([]*model.Transaction, 0, len(rows))
	for _, row := range rows {
		t, err := hydrate(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// parseOpt parses an optional id string into an optional vo.Id.
func parseOpt(s *string) (*vo.Id, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

type sqliteQuerier struct{}

func (sqliteQuerier) GetTransactionByID(ctx context.Context, db backend.DBTX, id string) (txRow, error) {
	return sqlitegen.New(db).GetTransactionByID(ctx, id)
}
func (sqliteQuerier) UpsertTransaction(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertTransaction(ctx, p)
}
func (sqliteQuerier) DeleteTransaction(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteTransaction(ctx, id)
}
func (sqliteQuerier) ListTransactionsByAccount(ctx context.Context, db backend.DBTX, p listAccParams) ([]txRow, error) {
	return sqlitegen.New(db).ListTransactionsByAccount(ctx, p)
}
func (sqliteQuerier) ListExportAccountsForUser(ctx context.Context, db backend.DBTX, userID string) ([]exportAcctRow, error) {
	return sqlitegen.New(db).ListExportAccountsForUser(ctx, sqlitegen.ListExportAccountsForUserParams{UserID: userID, UserID_2: userID})
}

type pgsqlQuerier struct{}

func (pgsqlQuerier) GetTransactionByID(ctx context.Context, db backend.DBTX, id string) (txRow, error) {
	t, err := pgsqlgen.New(db).GetTransactionByID(ctx, id)
	return txRow(t), err
}
func (pgsqlQuerier) UpsertTransaction(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertTransaction(ctx, pgsqlgen.UpsertTransactionParams(p))
}
func (pgsqlQuerier) DeleteTransaction(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteTransaction(ctx, id)
}
func (pgsqlQuerier) ListTransactionsByAccount(ctx context.Context, db backend.DBTX, p listAccParams) ([]txRow, error) {
	rows, err := pgsqlgen.New(db).ListTransactionsByAccount(ctx, pgsqlgen.ListTransactionsByAccountParams(p))
	if err != nil {
		return nil, err
	}
	out := make([]txRow, len(rows))
	for i, t := range rows {
		out[i] = txRow(t)
	}
	return out, nil
}
func (pgsqlQuerier) ListExportAccountsForUser(ctx context.Context, db backend.DBTX, userID string) ([]exportAcctRow, error) {
	rows, err := pgsqlgen.New(db).ListExportAccountsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]exportAcctRow, len(rows))
	for i, v := range rows {
		out[i] = exportAcctRow(v)
	}
	return out, nil
}
