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
	txRow             = sqlitegen.Transaction
	upsertParams      = sqlitegen.UpsertTransactionParams
	linkParams        = sqlitegen.LinkTransactionToRecurringParams
	listAccParams     = sqlitegen.ListTransactionsByAccountParams
	exportAcctRow     = sqlitegen.ListExportAccountsForUserRow
	insertLabelParams = sqlitegen.InsertTransactionLabelParams
)

type querier interface {
	GetTransactionByID(ctx context.Context, db backend.DBTX, id string) (txRow, error)
	UpsertTransaction(ctx context.Context, db backend.DBTX, p upsertParams) error
	LinkTransactionToRecurring(ctx context.Context, db backend.DBTX, p linkParams) error
	DeleteTransaction(ctx context.Context, db backend.DBTX, id string) error
	ListTransactionsByAccount(ctx context.Context, db backend.DBTX, p listAccParams) ([]txRow, error)
	ListExportAccountsForUser(ctx context.Context, db backend.DBTX, userID string) ([]exportAcctRow, error)
	DeleteTransactionLabels(ctx context.Context, db backend.DBTX, transactionID string) error
	InsertTransactionLabel(ctx context.Context, db backend.DBTX, p insertLabelParams) error
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
		RecurringID:        idPtr(t.RecurringID),
	})
}

// ReplaceLabels rewrites a transaction's label links. The caller runs this
// inside the same tx as Save, so the delete-then-insert is atomic and a
// re-save is idempotent (never duplicates a pair).
func (r *Repo) ReplaceLabels(ctx context.Context, transactionID vo.Id, labelIDs []vo.Id) error {
	db := r.db(ctx)
	if err := r.q.DeleteTransactionLabels(ctx, db, transactionID.String()); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := r.q.InsertTransactionLabel(ctx, db, insertLabelParams{
			TransactionID: transactionID.String(),
			LabelID:       id.String(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// labelsByTransactionIDsChunkSize bounds each round trip's IN list well under
// either engine's bind-param ceiling (SQLite 32766, PostgreSQL 65535): unlike
// the account-bounded queries elsewhere in this file, the caller here is a
// transaction count (CSV export, an unpaginated list endpoint), which is not
// naturally small, so the batch loader chunks itself rather than trusting
// every call site to pre-page its ids.
const labelsByTransactionIDsChunkSize = 500

// LabelsByTransactionIDs batch-loads label ids for many transactions, keyed by
// transaction id. hydrate is per-row and takes no ctx, so labels are attached
// after hydration by the caller; loading per row here would be an N+1 on
// every list endpoint. An empty ids is a no-op on SQLite but a syntax error on
// PostgreSQL ("IN ()"), so it short-circuits before building the query.
func (r *Repo) LabelsByTransactionIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make(map[string][]string, len(ids))
	for len(ids) > 0 {
		n := len(ids)
		if n > labelsByTransactionIDsChunkSize {
			n = labelsByTransactionIDsChunkSize
		}
		chunk := ids[:n]
		ids = ids[n:]
		if err := r.labelsByTransactionIDsChunk(ctx, chunk, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *Repo) labelsByTransactionIDsChunk(ctx context.Context, ids []vo.Id, out map[string][]string) error {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id.String()
	}
	// ORDER BY pins row order across engines (see ListByAccountIDs above for the
	// same hazard): without it, SQLite satisfies the query from the
	// (transaction_id, label_id) PK index (label_id order) while PostgreSQL
	// seq-scans a small table in insertion order, so the label slice per
	// transaction would diverge between engines.
	query := "SELECT transaction_id, label_id FROM transactions_labels WHERE transaction_id IN (" +
		placeholders(r.driver, 1, len(args)) + ") ORDER BY transaction_id, label_id"
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var txID, labelID string
		if err := rows.Scan(&txID, &labelID); err != nil {
			return err
		}
		out[txID] = append(out[txID], labelID)
	}
	return rows.Err()
}

// ImportedTransactionIDs reads import_transaction_links, a table the imports
// feature owns. This is the one place the transaction feature touches it —
// SQL, not a Go import, so archtest cannot see the coupling; keep it here and
// nowhere else. Same chunking + empty-IN guard as LabelsByTransactionIDs.
func (r *Repo) ImportedTransactionIDs(ctx context.Context, ids []vo.Id) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	for len(ids) > 0 {
		n := len(ids)
		if n > labelsByTransactionIDsChunkSize {
			n = labelsByTransactionIDsChunkSize
		}
		chunk := ids[:n]
		ids = ids[n:]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id.String()
		}
		query := "SELECT DISTINCT transaction_id FROM import_transaction_links WHERE transaction_id IN (" +
			placeholders(r.driver, 1, len(args)) + ")"
		rows, err := r.db(ctx).QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var txID string
			if err := rows.Scan(&txID); err != nil {
				rows.Close()
				return nil, err
			}
			out[txID] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// LinkRecurring stamps recurring_id on an existing transaction (Save never
// touches the column, so an explicit link needs this dedicated write).
func (r *Repo) LinkRecurring(ctx context.Context, id, recurringID vo.Id, now time.Time) error {
	rid := recurringID.String()
	return r.q.LinkTransactionToRecurring(ctx, r.db(ctx), linkParams{
		ID:          id.String(),
		RecurringID: &rid,
		UpdatedAt:   now,
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
// accountIDs, bounded by filter.PeriodStart/PeriodEnd (both non-zero) and
// narrowed by filter's classification fields. Built by hand (dynamic IN +
// optional predicates); placeholders differ per engine. filter's zero value
// appends no predicate beyond the accounts, so callers that never set it (the
// account-only paths) get exactly today's query.
func (r *Repo) ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, filter model.TransactionFilter) ([]*model.Transaction, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	ids := make([]any, len(accountIDs))
	for i, id := range accountIDs {
		ids[i] = id.String()
	}
	usePeriod := !filter.PeriodStart.IsZero() && !filter.PeriodEnd.IsZero()

	const cols = "id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient, recurring_id"
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
		args = append(args, filter.PeriodStart, filter.PeriodEnd)
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
			&row.SpentAt, &row.Type, &row.Amount, &row.AmountRecipient, &row.RecurringID,
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
	recurring, err := parseOpt(row.RecurringID)
	if err != nil {
		return nil, err
	}
	return model.FromState(model.NewState{
		ID: id, UserID: userID, Type: model.TransactionType(row.Type), AccountID: accountID,
		AccountRecipID: recip, Amount: row.Amount, AmountRecipient: row.AmountRecipient,
		CategoryID: cat, PayeeID: payee, TagID: tag, Description: row.Description,
		SpentAt: row.SpentAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		RecurringID: recurring,
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
func (sqliteQuerier) LinkTransactionToRecurring(ctx context.Context, db backend.DBTX, p linkParams) error {
	return sqlitegen.New(db).LinkTransactionToRecurring(ctx, p)
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
func (sqliteQuerier) DeleteTransactionLabels(ctx context.Context, db backend.DBTX, transactionID string) error {
	return sqlitegen.New(db).DeleteTransactionLabels(ctx, transactionID)
}
func (sqliteQuerier) InsertTransactionLabel(ctx context.Context, db backend.DBTX, p insertLabelParams) error {
	return sqlitegen.New(db).InsertTransactionLabel(ctx, p)
}

type pgsqlQuerier struct{}

func (pgsqlQuerier) GetTransactionByID(ctx context.Context, db backend.DBTX, id string) (txRow, error) {
	t, err := pgsqlgen.New(db).GetTransactionByID(ctx, id)
	return txRow(t), err
}
func (pgsqlQuerier) UpsertTransaction(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertTransaction(ctx, pgsqlgen.UpsertTransactionParams(p))
}
func (pgsqlQuerier) LinkTransactionToRecurring(ctx context.Context, db backend.DBTX, p linkParams) error {
	return pgsqlgen.New(db).LinkTransactionToRecurring(ctx, pgsqlgen.LinkTransactionToRecurringParams(p))
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
func (pgsqlQuerier) DeleteTransactionLabels(ctx context.Context, db backend.DBTX, transactionID string) error {
	return pgsqlgen.New(db).DeleteTransactionLabels(ctx, transactionID)
}
func (pgsqlQuerier) InsertTransactionLabel(ctx context.Context, db backend.DBTX, p insertLabelParams) error {
	return pgsqlgen.New(db).InsertTransactionLabel(ctx, pgsqlgen.InsertTransactionLabelParams(p))
}
