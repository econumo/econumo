// Package connectionrepo implements domain/connection.AccountAccessRepository
// over the sqlc-generated queries, both engines. The static queries use the
// canonical (sqlite-typed) shim; the pgsql adapter is a thin whole-struct
// conversion because the generated row/param shapes are field-identical.
package connectionrepo

import (
	"context"
	"database/sql"
	"errors"

	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type (
	accessRow    = sqlitegen.AccountsAccess
	upsertParams = sqlitegen.UpsertAccountAccessParams
)

type querier interface {
	GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error)
	UpsertAccountAccess(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error
	ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error)
	AccountOwnerID(ctx context.Context, db backend.DBTX, accountID string) (string, error)
	ListConnectedUserIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error)
	DeleteConnectionLink(ctx context.Context, db backend.DBTX, a, b string) error
	DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error
}

// Repo implements domain/connection.AccountAccessRepository.
type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ domconnection.AccountAccessRepository = (*Repo)(nil)

// NewRepo selects the engine adapter by driver name.
func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("connectionrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// Get loads the grant for (accountID, userID).
func (r *Repo) Get(ctx context.Context, accountID, userID vo.Id) (*domconnection.AccountAccess, error) {
	row, err := r.q.GetAccountAccess(ctx, r.db(ctx), accountID.String(), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("AccountAccess not found")
		}
		return nil, err
	}
	return hydrate(row)
}

// Save upserts a grant.
func (r *Repo) Save(ctx context.Context, a *domconnection.AccountAccess) error {
	return r.q.UpsertAccountAccess(ctx, r.db(ctx), upsertParams{
		AccountID: a.AccountId().String(),
		UserID:    a.UserId().String(),
		Role:      a.Role().Int16(),
		CreatedAt: a.CreatedAt(),
		UpdatedAt: a.UpdatedAt(),
	})
}

// Delete removes the grant for (accountID, userID).
func (r *Repo) Delete(ctx context.Context, accountID, userID vo.Id) error {
	return r.q.DeleteAccountAccess(ctx, r.db(ctx), accountID.String(), userID.String())
}

// ListReceived returns grants made TO userID.
func (r *Repo) ListReceived(ctx context.Context, userID vo.Id) ([]*domconnection.AccountAccess, error) {
	rows, err := r.q.ListReceived(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAll(rows)
}

// ListIssued returns grants on accounts owned by userID.
func (r *Repo) ListIssued(ctx context.Context, userID vo.Id) ([]*domconnection.AccountAccess, error) {
	rows, err := r.q.ListIssued(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAll(rows)
}

// ListByAccount returns all grants on one account (for the account's
// sharedAccess[] embed).
func (r *Repo) ListByAccount(ctx context.Context, accountID vo.Id) ([]*domconnection.AccountAccess, error) {
	rows, err := r.q.ListByAccount(ctx, r.db(ctx), accountID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAll(rows)
}

// AccountOwner returns the owner user id of an account.
func (r *Repo) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	s, err := r.q.AccountOwnerID(ctx, r.db(ctx), accountID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return vo.Id{}, errs.NewNotFound("Account not found")
		}
		return vo.Id{}, err
	}
	return vo.ParseId(s)
}

// ConnectedUserIDs returns the users linked to userID.
func (r *Repo) ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	rows, err := r.q.ListConnectedUserIDs(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]vo.Id, 0, len(rows))
	for _, s := range rows {
		id, perr := vo.ParseId(s)
		if perr != nil {
			return nil, perr
		}
		out = append(out, id)
	}
	return out, nil
}

// DeleteConnection removes the symmetric link between two users.
func (r *Repo) DeleteConnection(ctx context.Context, a, b vo.Id) error {
	return r.q.DeleteConnectionLink(ctx, r.db(ctx), a.String(), b.String())
}

// DeleteOption removes a user's accounts_options row for an account.
func (r *Repo) DeleteOption(ctx context.Context, accountID, userID vo.Id) error {
	return r.q.DeleteAccountOptionForUser(ctx, r.db(ctx), accountID.String(), userID.String())
}

func hydrate(row accessRow) (*domconnection.AccountAccess, error) {
	accountID, err := vo.ParseId(row.AccountID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return domconnection.FromState(accountID, userID, domconnection.Role(row.Role), row.CreatedAt, row.UpdatedAt), nil
}

func hydrateAll(rows []accessRow) ([]*domconnection.AccountAccess, error) {
	out := make([]*domconnection.AccountAccess, 0, len(rows))
	for _, row := range rows {
		a, err := hydrate(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// --- engine adapters ---

type sqliteQuerier struct{}

func (sqliteQuerier) GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error) {
	return sqlitegen.New(db).GetAccountAccess(ctx, sqlitegen.GetAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (sqliteQuerier) UpsertAccountAccess(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertAccountAccess(ctx, p)
}
func (sqliteQuerier) DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return sqlitegen.New(db).DeleteAccountAccess(ctx, sqlitegen.DeleteAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (sqliteQuerier) ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListReceivedAccountAccess(ctx, userID)
}
func (sqliteQuerier) ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListIssuedAccountAccess(ctx, userID)
}
func (sqliteQuerier) ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListAccountAccessByAccount(ctx, accountID)
}
func (sqliteQuerier) AccountOwnerID(ctx context.Context, db backend.DBTX, accountID string) (string, error) {
	return sqlitegen.New(db).AccountOwnerID(ctx, accountID)
}
func (sqliteQuerier) ListConnectedUserIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error) {
	return sqlitegen.New(db).ListConnectedUserIDs(ctx, userID)
}
func (sqliteQuerier) DeleteConnectionLink(ctx context.Context, db backend.DBTX, a, b string) error {
	return sqlitegen.New(db).DeleteConnectionLink(ctx, sqlitegen.DeleteConnectionLinkParams{
		UserID: a, ConnectedUserID: b, UserID_2: b, ConnectedUserID_2: a,
	})
}
func (sqliteQuerier) DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return sqlitegen.New(db).DeleteAccountOptionForUser(ctx, sqlitegen.DeleteAccountOptionForUserParams{AccountID: accountID, UserID: userID})
}

type pgsqlQuerier struct{}

func (pgsqlQuerier) GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error) {
	row, err := pgsqlgen.New(db).GetAccountAccess(ctx, pgsqlgen.GetAccountAccessParams{AccountID: accountID, UserID: userID})
	return accessRow(row), err
}
func (pgsqlQuerier) UpsertAccountAccess(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertAccountAccess(ctx, pgsqlgen.UpsertAccountAccessParams(p))
}
func (pgsqlQuerier) DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return pgsqlgen.New(db).DeleteAccountAccess(ctx, pgsqlgen.DeleteAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (pgsqlQuerier) ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListReceivedAccountAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, r := range rows {
		out[i] = accessRow(r)
	}
	return out, nil
}
func (pgsqlQuerier) ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListIssuedAccountAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, r := range rows {
		out[i] = accessRow(r)
	}
	return out, nil
}
func (pgsqlQuerier) ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListAccountAccessByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, r := range rows {
		out[i] = accessRow(r)
	}
	return out, nil
}
func (pgsqlQuerier) AccountOwnerID(ctx context.Context, db backend.DBTX, accountID string) (string, error) {
	return pgsqlgen.New(db).AccountOwnerID(ctx, accountID)
}
func (pgsqlQuerier) ListConnectedUserIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error) {
	return pgsqlgen.New(db).ListConnectedUserIDs(ctx, userID)
}
func (pgsqlQuerier) DeleteConnectionLink(ctx context.Context, db backend.DBTX, a, b string) error {
	return pgsqlgen.New(db).DeleteConnectionLink(ctx, pgsqlgen.DeleteConnectionLinkParams{
		UserID: a, ConnectedUserID: b, UserID_2: b, ConnectedUserID_2: a,
	})
}
func (pgsqlQuerier) DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return pgsqlgen.New(db).DeleteAccountOptionForUser(ctx, pgsqlgen.DeleteAccountOptionForUserParams{AccountID: accountID, UserID: userID})
}
