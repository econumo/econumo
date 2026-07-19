// AccessRepo implements account.AccessStore over the shared accounts_access
// queries (generated once, in internal/infra/storage/sqlc/query/*/connection.sql).
package repo

import (
	"context"
	"database/sql"
	"errors"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type accessUpsertParams = sqlitegen.UpsertAccountAccessParams

type accessQuerier interface {
	GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error)
	UpsertAccountAccess(ctx context.Context, db backend.DBTX, p accessUpsertParams) error
	DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error
	ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error)
	ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	ListPendingReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error
}

// AccessRepo implements account.AccessStore.
type AccessRepo struct {
	tx *backend.TxManager
	q  accessQuerier
}

var _ appaccount.AccessStore = (*AccessRepo)(nil)

// NewAccessRepo selects the engine adapter by driver name.
func NewAccessRepo(driver string, tx *backend.TxManager) *AccessRepo {
	switch driver {
	case "sqlite":
		return &AccessRepo{tx: tx, q: accessSqlite{}}
	case "postgresql":
		return &AccessRepo{tx: tx, q: accessPgsql{}}
	default:
		panic("accountrepo: unknown database driver " + driver)
	}
}

func (r *AccessRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// Get loads the grant for (accountID, userID).
func (r *AccessRepo) Get(ctx context.Context, accountID, userID vo.Id) (*model.AccountAccess, error) {
	row, err := r.q.GetAccountAccess(ctx, r.db(ctx), accountID.String(), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("AccountAccess not found")
		}
		return nil, err
	}
	return hydrateAccess(row)
}

// Save upserts a grant.
func (r *AccessRepo) Save(ctx context.Context, a *model.AccountAccess) error {
	return r.q.UpsertAccountAccess(ctx, r.db(ctx), accessUpsertParams{
		AccountID: a.AccountID.String(), UserID: a.UserID.String(),
		Role: a.Role.Int16(), IsAccepted: a.IsAccepted,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	})
}

// Delete removes the grant for (accountID, userID).
func (r *AccessRepo) Delete(ctx context.Context, accountID, userID vo.Id) error {
	return r.q.DeleteAccountAccess(ctx, r.db(ctx), accountID.String(), userID.String())
}

// ListByAccount returns all grants on one account.
func (r *AccessRepo) ListByAccount(ctx context.Context, accountID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListByAccount(ctx, r.db(ctx), accountID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

// ListReceived returns grants made TO userID.
func (r *AccessRepo) ListReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListReceived(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

// ListPendingReceived returns the user's pending (not yet accepted) received
// grants, ordered by grant creation.
func (r *AccessRepo) ListPendingReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListPendingReceived(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

// ListIssued returns grants on accounts owned by userID.
func (r *AccessRepo) ListIssued(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListIssued(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

// DeleteOption removes a user's accounts_options row for an account.
func (r *AccessRepo) DeleteOption(ctx context.Context, accountID, userID vo.Id) error {
	return r.q.DeleteAccountOptionForUser(ctx, r.db(ctx), accountID.String(), userID.String())
}

func hydrateAccessAll(rows []accessRow) ([]*model.AccountAccess, error) {
	out := make([]*model.AccountAccess, 0, len(rows))
	for _, row := range rows {
		a, err := hydrateAccess(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

type accessSqlite struct{}

var _ accessQuerier = accessSqlite{}

func (accessSqlite) GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error) {
	return sqlitegen.New(db).GetAccountAccess(ctx, sqlitegen.GetAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (accessSqlite) UpsertAccountAccess(ctx context.Context, db backend.DBTX, p accessUpsertParams) error {
	return sqlitegen.New(db).UpsertAccountAccess(ctx, p)
}
func (accessSqlite) DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return sqlitegen.New(db).DeleteAccountAccess(ctx, sqlitegen.DeleteAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (accessSqlite) ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListAccountAccessByAccount(ctx, accountID)
}
func (accessSqlite) ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListReceivedAccountAccess(ctx, userID)
}
func (accessSqlite) ListPendingReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListPendingReceivedAccountAccess(ctx, userID)
}
func (accessSqlite) ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListIssuedAccountAccess(ctx, userID)
}
func (accessSqlite) DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return sqlitegen.New(db).DeleteAccountOptionForUser(ctx, sqlitegen.DeleteAccountOptionForUserParams{AccountID: accountID, UserID: userID})
}

type accessPgsql struct{}

var _ accessQuerier = accessPgsql{}

func (accessPgsql) GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error) {
	row, err := pgsqlgen.New(db).GetAccountAccess(ctx, pgsqlgen.GetAccountAccessParams{AccountID: accountID, UserID: userID})
	return accessRow(row), err
}
func (accessPgsql) UpsertAccountAccess(ctx context.Context, db backend.DBTX, p accessUpsertParams) error {
	return pgsqlgen.New(db).UpsertAccountAccess(ctx, pgsqlgen.UpsertAccountAccessParams(p))
}
func (accessPgsql) DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return pgsqlgen.New(db).DeleteAccountAccess(ctx, pgsqlgen.DeleteAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (accessPgsql) ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListAccountAccessByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, row := range rows {
		out[i] = accessRow(row)
	}
	return out, nil
}
func (accessPgsql) ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListReceivedAccountAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, row := range rows {
		out[i] = accessRow(row)
	}
	return out, nil
}
func (accessPgsql) ListPendingReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListPendingReceivedAccountAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, row := range rows {
		out[i] = accessRow(row)
	}
	return out, nil
}
func (accessPgsql) ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListIssuedAccountAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, row := range rows {
		out[i] = accessRow(row)
	}
	return out, nil
}
func (accessPgsql) DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return pgsqlgen.New(db).DeleteAccountOptionForUser(ctx, pgsqlgen.DeleteAccountOptionForUserParams{AccountID: accountID, UserID: userID})
}
