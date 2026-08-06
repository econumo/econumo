// Package repo implements recurring.Repository.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	domrecurring "github.com/econumo/econumo/internal/recurring"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	rtRow             = sqlitegen.RecurringTransaction
	upsertParams      = sqlitegen.UpsertRecurringTransactionParams
	insertLabelParams = sqlitegen.InsertRecurringLabelParams
)

type querier interface {
	GetRecurringTransactionByID(ctx context.Context, db backend.DBTX, id string) (rtRow, error)
	UpsertRecurringTransaction(ctx context.Context, db backend.DBTX, arg upsertParams) error
	DeleteRecurringTransaction(ctx context.Context, db backend.DBTX, id string) error
	DeleteRecurringLabels(ctx context.Context, db backend.DBTX, recurringTransactionID string) error
	InsertRecurringLabel(ctx context.Context, db backend.DBTX, p insertLabelParams) error
}

type Repo struct {
	driver string
	tx     *backend.TxManager
	q      querier
}

var _ domrecurring.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{driver: driver, tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{driver: driver, tx: tx, q: pgsqlQuerier{}}
	}
	panic(fmt.Sprintf("unknown database driver %q", driver))
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.RecurringTransaction, error) {
	row, err := r.q.GetRecurringTransactionByID(ctx, r.db(ctx), id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.NewNotFound("Recurring transaction not found")
	}
	if err != nil {
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) Save(ctx context.Context, rt *model.RecurringTransaction) error {
	return r.q.UpsertRecurringTransaction(ctx, r.db(ctx), upsertParams{
		ID:                 rt.ID.String(),
		UserID:             rt.UserID.String(),
		AccountID:          rt.AccountID.String(),
		AccountRecipientID: idPtr(rt.AccountRecipID),
		CategoryID:         idPtr(rt.CategoryID),
		PayeeID:            idPtr(rt.PayeeID),
		TagID:              idPtr(rt.TagID),
		Type:               rt.Type.Int16(),
		Amount:             rt.Amount,
		Description:        rt.Description,
		Schedule:           string(rt.Schedule),
		NextPaymentAt:      rt.NextPaymentAt,
		ScheduledDay:       rt.ScheduledDay,
		CreatedAt:          rt.CreatedAt,
		UpdatedAt:          rt.UpdatedAt,
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteRecurringTransaction(ctx, r.db(ctx), id.String())
}

// ReplaceLabels rewrites a template's label links. The caller runs this
// inside the same tx as Save, so the delete-then-insert is atomic and a
// re-save is idempotent (never duplicates a pair).
func (r *Repo) ReplaceLabels(ctx context.Context, recurringID vo.Id, labelIDs []vo.Id) error {
	db := r.db(ctx)
	if err := r.q.DeleteRecurringLabels(ctx, db, recurringID.String()); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := r.q.InsertRecurringLabel(ctx, db, insertLabelParams{
			RecurringTransactionID: recurringID.String(),
			LabelID:                id.String(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// labelsByRecurringIDsChunkSize mirrors labelsByTransactionIDsChunkSize in the
// transaction repo, bounding each round trip's IN list well under either
// engine's bind-param ceiling.
const labelsByRecurringIDsChunkSize = 500

// LabelsByRecurringIDs batch-loads label ids for many templates, keyed by
// template id. An empty ids is a no-op on SQLite but a syntax error on
// PostgreSQL ("IN ()"), so it short-circuits before building the query.
func (r *Repo) LabelsByRecurringIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make(map[string][]string, len(ids))
	for len(ids) > 0 {
		n := len(ids)
		if n > labelsByRecurringIDsChunkSize {
			n = labelsByRecurringIDsChunkSize
		}
		chunk := ids[:n]
		ids = ids[n:]
		if err := r.labelsByRecurringIDsChunk(ctx, chunk, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *Repo) labelsByRecurringIDsChunk(ctx context.Context, ids []vo.Id, out map[string][]string) error {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id.String()
	}
	// ORDER BY pins row order across engines: without it, SQLite satisfies the
	// query from the (recurring_transaction_id, label_id) PK index (label_id
	// order) while PostgreSQL seq-scans a small table in insertion order, so the
	// label slice per template would diverge between engines.
	query := "SELECT recurring_transaction_id, label_id FROM recurring_transactions_labels WHERE recurring_transaction_id IN (" +
		placeholders(r.driver, 1, len(args)) + ") ORDER BY recurring_transaction_id, label_id"
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var rtID, labelID string
		if err := rows.Scan(&rtID, &labelID); err != nil {
			return err
		}
		out[rtID] = append(out[rtID], labelID)
	}
	return rows.Err()
}

// Variadic IN list, so this is hand-built SQL like the transaction repo's
// ListByAccountIDs; column order matches the sqlc SELECTs so hydrate is shared.
func (r *Repo) ListByAccountIDs(ctx context.Context, accountIDs []vo.Id) ([]*model.RecurringTransaction, error) {
	if len(accountIDs) == 0 {
		return []*model.RecurringTransaction{}, nil
	}
	args := make([]any, len(accountIDs))
	for i, id := range accountIDs {
		args[i] = id.String()
	}
	query := fmt.Sprintf(`SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
       type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at
FROM recurring_transactions
WHERE account_id IN (%s)
ORDER BY next_payment_at, id`, placeholders(r.driver, 1, len(accountIDs)))
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.RecurringTransaction{}
	for rows.Next() {
		var row rtRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.AccountID, &row.AccountRecipientID,
			&row.CategoryID, &row.PayeeID, &row.TagID, &row.Type, &row.Amount, &row.Description,
			&row.Schedule, &row.NextPaymentAt, &row.ScheduledDay, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		rt, err := hydrate(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func placeholders(driver string, start, n int) string {
	parts := make([]string, n)
	for i := range parts {
		if driver == "postgresql" {
			parts[i] = fmt.Sprintf("$%d", start+i)
		} else {
			parts[i] = "?"
		}
	}
	return strings.Join(parts, ", ")
}

func hydrate(row rtRow) (*model.RecurringTransaction, error) {
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
	return model.RecurringFromState(model.RecurringNewState{
		ID: id, UserID: userID, Type: model.TransactionType(row.Type),
		AccountID: accountID, AccountRecipID: recip, Amount: row.Amount,
		CategoryID: cat, PayeeID: payee, TagID: tag, Description: row.Description,
		Schedule: model.RecurringSchedule(row.Schedule), NextPaymentAt: row.NextPaymentAt,
		ScheduledDay: row.ScheduledDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func idPtr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

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
