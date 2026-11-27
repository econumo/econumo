package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/transaction"
)

type (
	crossOwnerRow     = sqlitegen.ListCrossOwnerTransfersRow
	wipeCorrectionRow = sqlitegen.InsertWipeCorrectionParams
)

// The cascade of the user row (and the SET NULL it triggers) changes what
// these rows show without passing through this repo, so their accounts are
// queued here, before anything is deleted. The recurring condition names the
// templates the cascade removes: the user's own and any on the user's accounts.
var losingReferenceConds = []struct {
	cond  string
	nargs int
}{
	{"category_id IN (SELECT c.id FROM categories c WHERE c.user_id = ?)", 1},
	{"payee_id IN (SELECT p.id FROM payees p WHERE p.user_id = ?)", 1},
	{"tag_id IN (SELECT tg.id FROM tags tg WHERE tg.user_id = ?)", 1},
	{"id IN (SELECT tl.transaction_id FROM transactions_labels tl WHERE tl.label_id IN (SELECT l.id FROM labels l WHERE l.user_id = ?))", 1},
	{"id IN (SELECT il.transaction_id FROM import_transaction_links il WHERE il.source_id IN (SELECT s.id FROM import_sources s WHERE s.user_id = ?))", 1},
	{"recurring_id IN (SELECT rt.id FROM recurring_transactions rt WHERE rt.user_id = ?" +
		" OR rt.account_id IN (SELECT a.id FROM accounts a WHERE a.user_id = ?)" +
		" OR rt.account_recipient_id IN (SELECT a.id FROM accounts a WHERE a.user_id = ?))", 3},
}

// BumpAccountsLosingReferences queues the accounts whose rows the user's
// deletion cascade will change.
func (r *Repo) BumpAccountsLosingReferences(ctx context.Context, userID vo.Id) error {
	for _, c := range losingReferenceConds {
		args := make([]any, c.nargs)
		for i := range args {
			args[i] = userID.String()
		}
		if err := r.rev.BumpWhere(ctx, c.cond, args...); err != nil {
			return err
		}
	}
	return nil
}

// ReassignAuthorInForeignAccounts hands the user's rows in other people's
// accounts to each account's owner; updated_at is deliberately kept.
func (r *Repo) ReassignAuthorInForeignAccounts(ctx context.Context, userID vo.Id) error {
	id := userID.String()
	if err := r.rev.BumpWhere(ctx, "user_id = ? AND account_id IN (SELECT o.id FROM accounts o WHERE o.user_id <> ?)", id, id); err != nil {
		return err
	}
	return r.q.ReassignAuthorInForeignAccounts(ctx, r.db(ctx), id)
}

// ListCrossOwnerTransfers lists transfers with exactly one side on the user's
// accounts, soft-deleted accounts included.
func (r *Repo) ListCrossOwnerTransfers(ctx context.Context, userID vo.Id) ([]domtransaction.CrossOwnerTransfer, error) {
	rows, err := r.q.ListCrossOwnerTransfers(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]domtransaction.CrossOwnerTransfer, 0, len(rows))
	for _, row := range rows {
		t := domtransaction.CrossOwnerTransfer{
			Amount:           row.Amount,
			AmountRecipient:  row.AmountRecipient,
			Description:      row.Description,
			SpentAt:          row.SpentAt,
			SortKey:          row.SortKey,
			RecipientSortKey: row.RecipientSortKey,
		}
		recipient := ""
		if row.AccountRecipientID != nil {
			recipient = *row.AccountRecipientID
		}
		for _, p := range []struct {
			dst *vo.Id
			src string
		}{
			{&t.ID, row.ID}, {&t.SourceAccountID, row.AccountID}, {&t.RecipientAccountID, recipient},
			{&t.SourceOwnerID, row.SourceOwnerID}, {&t.RecipientOwnerID, row.RecipientOwnerID},
		} {
			if *p.dst, err = vo.ParseId(p.src); err != nil {
				return nil, err
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// ReplaceTransferWithCorrection swaps a cross-owner transfer for its surviving
// side's correction, written at the original's key so the day's order holds.
// Both sides are bumped: the surviving one changed, and the deleted user's
// account goes with the cascade anyway.
func (r *Repo) ReplaceTransferWithCorrection(ctx context.Context, t domtransaction.CrossOwnerTransfer, c *domtransaction.WipeCorrection) error {
	if err := r.rev.Bump(ctx, t.SourceAccountID.String(), t.RecipientAccountID.String()); err != nil {
		return err
	}
	db := r.db(ctx)
	if c != nil {
		if err := r.q.InsertWipeCorrection(ctx, db, wipeCorrectionRow{
			ID:          c.ID.String(),
			UserID:      c.AuthorID.String(),
			AccountID:   c.AccountID.String(),
			Description: c.Description,
			CreatedAt:   c.CreatedAt,
			UpdatedAt:   c.CreatedAt,
			SpentAt:     c.SpentAt,
			Type:        c.Type.Int16(),
			Amount:      c.Amount,
			SortKey:     c.SortKey,
		}); err != nil {
			return err
		}
	}
	return r.q.DeleteTransaction(ctx, db, t.ID.String())
}

func (sqliteQuerier) ReassignAuthorInForeignAccounts(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).ReassignAuthorInForeignAccounts(ctx, sqlitegen.ReassignAuthorInForeignAccountsParams{UserID: userID, UserID_2: userID})
}
func (sqliteQuerier) ListCrossOwnerTransfers(ctx context.Context, db backend.DBTX, userID string) ([]crossOwnerRow, error) {
	return sqlitegen.New(db).ListCrossOwnerTransfers(ctx, sqlitegen.ListCrossOwnerTransfersParams{UserID: userID, UserID_2: userID, UserID_3: userID, UserID_4: userID})
}
func (sqliteQuerier) InsertWipeCorrection(ctx context.Context, db backend.DBTX, p wipeCorrectionRow) error {
	return sqlitegen.New(db).InsertWipeCorrection(ctx, p)
}

func (pgsqlQuerier) ReassignAuthorInForeignAccounts(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).ReassignAuthorInForeignAccounts(ctx, userID)
}
func (pgsqlQuerier) ListCrossOwnerTransfers(ctx context.Context, db backend.DBTX, userID string) ([]crossOwnerRow, error) {
	rows, err := pgsqlgen.New(db).ListCrossOwnerTransfers(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]crossOwnerRow, len(rows))
	for i, v := range rows {
		out[i] = crossOwnerRow(v)
	}
	return out, nil
}
func (pgsqlQuerier) InsertWipeCorrection(ctx context.Context, db backend.DBTX, p wipeCorrectionRow) error {
	return pgsqlgen.New(db).InsertWipeCorrection(ctx, pgsqlgen.InsertWipeCorrectionParams(p))
}
