// FolderRepo implements account.FolderRepository (folders +
// accounts_folders membership) over the sqlc-generated queries.
package repo

import (
	"context"
	"database/sql"
	"errors"

	domaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Canonical folder types. The membership query selects (folder_id, account_id),
// which sqlc maps to the AccountsFolder model.
type (
	folderRow     = sqlitegen.Folder
	upsertFolderP = sqlitegen.UpsertFolderParams
	addMemberP    = sqlitegen.AddAccountToFolderParams
	removeMemberP = sqlitegen.RemoveAccountFromFolderParams
	membershipRow = sqlitegen.AccountsFolder
)

// folderQuerier is the engine-agnostic folder surface.
type folderQuerier interface {
	GetFolder(ctx context.Context, db backend.DBTX, id string) (folderRow, error)
	ListFoldersByUser(ctx context.Context, db backend.DBTX, userID string) ([]folderRow, error)
	CountFoldersByUser(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertFolder(ctx context.Context, db backend.DBTX, p upsertFolderP) error
	DeleteFolder(ctx context.Context, db backend.DBTX, id string) error
	ListFolderAccountIDs(ctx context.Context, db backend.DBTX, folderID string) ([]string, error)
	ListFolderMembershipsByUser(ctx context.Context, db backend.DBTX, userID string) ([]membershipRow, error)
	AddAccountToFolder(ctx context.Context, db backend.DBTX, p addMemberP) error
	RemoveAccountFromFolder(ctx context.Context, db backend.DBTX, p removeMemberP) error
}

// FolderRepo is the concrete folder repository.
type FolderRepo struct {
	tx *backend.TxManager
	q  folderQuerier
}

var _ domaccount.FolderRepository = (*FolderRepo)(nil)

// NewFolderRepo selects the engine folder querier by driver name.
func NewFolderRepo(driver string, tx *backend.TxManager) *FolderRepo {
	switch driver {
	case "sqlite":
		return &FolderRepo{tx: tx, q: sqliteFolderQuerier{}}
	case "postgresql":
		return &FolderRepo{tx: tx, q: pgsqlFolderQuerier{}}
	default:
		panic("accountrepo: unknown database driver " + driver)
	}
}

func (r *FolderRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// NextIdentity allocates a fresh folder id.
func (r *FolderRepo) NextIdentity() vo.Id { return vo.NewId() }

// GetByID loads a folder by id.
func (r *FolderRepo) GetByID(ctx context.Context, id vo.Id) (*domaccount.Folder, error) {
	row, err := r.q.GetFolder(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Folder not found")
		}
		return nil, err
	}
	return hydrateFolder(row)
}

// ListByUser returns the user's folders.
func (r *FolderRepo) ListByUser(ctx context.Context, userID vo.Id) ([]*domaccount.Folder, error) {
	rows, err := r.q.ListFoldersByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*domaccount.Folder, 0, len(rows))
	for _, row := range rows {
		f, herr := hydrateFolder(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, f)
	}
	return out, nil
}

// CountByUser returns the number of folders the user has.
func (r *FolderRepo) CountByUser(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountFoldersByUser(ctx, r.db(ctx), userID.String())
	return int(n), err
}

// Save upserts a folder row.
func (r *FolderRepo) Save(ctx context.Context, f *domaccount.Folder) error {
	return r.q.UpsertFolder(ctx, r.db(ctx), upsertFolderP{
		ID:        f.Id().String(),
		UserID:    f.UserId().String(),
		Name:      f.Name(),
		Position:  f.Position(),
		IsVisible: f.IsVisible(),
		CreatedAt: f.CreatedAt(),
		UpdatedAt: f.UpdatedAt(),
	})
}

// Delete removes a folder row (accounts_folders rows cascade).
func (r *FolderRepo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteFolder(ctx, r.db(ctx), id.String())
}

// MembershipsByUser returns folderID -> []accountID for the user's folders.
func (r *FolderRepo) MembershipsByUser(ctx context.Context, userID vo.Id) (map[string][]string, error) {
	rows, err := r.q.ListFolderMembershipsByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string)
	for _, row := range rows {
		out[row.FolderID] = append(out[row.FolderID], row.AccountID)
	}
	return out, nil
}

// FolderAccountIDs returns the account ids in a folder.
func (r *FolderRepo) FolderAccountIDs(ctx context.Context, folderID vo.Id) ([]string, error) {
	return r.q.ListFolderAccountIDs(ctx, r.db(ctx), folderID.String())
}

// AddAccount adds an account to a folder (idempotent).
func (r *FolderRepo) AddAccount(ctx context.Context, folderID, accountID vo.Id) error {
	return r.q.AddAccountToFolder(ctx, r.db(ctx), addMemberP{FolderID: folderID.String(), AccountID: accountID.String()})
}

// RemoveAccount removes an account from a folder.
func (r *FolderRepo) RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error {
	return r.q.RemoveAccountFromFolder(ctx, r.db(ctx), removeMemberP{FolderID: folderID.String(), AccountID: accountID.String()})
}

func hydrateFolder(row folderRow) (*domaccount.Folder, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return domaccount.FolderFromState(id, userID, row.Name, row.Position, row.IsVisible, row.CreatedAt, row.UpdatedAt), nil
}

type sqliteFolderQuerier struct{}

func (sqliteFolderQuerier) GetFolder(ctx context.Context, db backend.DBTX, id string) (folderRow, error) {
	return sqlitegen.New(db).GetFolderByID(ctx, id)
}
func (sqliteFolderQuerier) ListFoldersByUser(ctx context.Context, db backend.DBTX, userID string) ([]folderRow, error) {
	return sqlitegen.New(db).ListFoldersByUser(ctx, userID)
}
func (sqliteFolderQuerier) CountFoldersByUser(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return sqlitegen.New(db).CountFoldersByUser(ctx, userID)
}
func (sqliteFolderQuerier) UpsertFolder(ctx context.Context, db backend.DBTX, p upsertFolderP) error {
	return sqlitegen.New(db).UpsertFolder(ctx, p)
}
func (sqliteFolderQuerier) DeleteFolder(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteFolder(ctx, id)
}
func (sqliteFolderQuerier) ListFolderAccountIDs(ctx context.Context, db backend.DBTX, folderID string) ([]string, error) {
	return sqlitegen.New(db).ListFolderAccountIDs(ctx, folderID)
}
func (sqliteFolderQuerier) ListFolderMembershipsByUser(ctx context.Context, db backend.DBTX, userID string) ([]membershipRow, error) {
	return sqlitegen.New(db).ListFolderMembershipsByUser(ctx, userID)
}
func (sqliteFolderQuerier) AddAccountToFolder(ctx context.Context, db backend.DBTX, p addMemberP) error {
	return sqlitegen.New(db).AddAccountToFolder(ctx, p)
}
func (sqliteFolderQuerier) RemoveAccountFromFolder(ctx context.Context, db backend.DBTX, p removeMemberP) error {
	return sqlitegen.New(db).RemoveAccountFromFolder(ctx, p)
}

type pgsqlFolderQuerier struct{}

func (pgsqlFolderQuerier) GetFolder(ctx context.Context, db backend.DBTX, id string) (folderRow, error) {
	f, err := pgsqlgen.New(db).GetFolderByID(ctx, id)
	return folderRow(f), err
}
func (pgsqlFolderQuerier) ListFoldersByUser(ctx context.Context, db backend.DBTX, userID string) ([]folderRow, error) {
	rows, err := pgsqlgen.New(db).ListFoldersByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]folderRow, len(rows))
	for i, f := range rows {
		out[i] = folderRow(f)
	}
	return out, nil
}
func (pgsqlFolderQuerier) CountFoldersByUser(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountFoldersByUser(ctx, userID)
}
func (pgsqlFolderQuerier) UpsertFolder(ctx context.Context, db backend.DBTX, p upsertFolderP) error {
	return pgsqlgen.New(db).UpsertFolder(ctx, pgsqlgen.UpsertFolderParams(p))
}
func (pgsqlFolderQuerier) DeleteFolder(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteFolder(ctx, id)
}
func (pgsqlFolderQuerier) ListFolderAccountIDs(ctx context.Context, db backend.DBTX, folderID string) ([]string, error) {
	return pgsqlgen.New(db).ListFolderAccountIDs(ctx, folderID)
}
func (pgsqlFolderQuerier) ListFolderMembershipsByUser(ctx context.Context, db backend.DBTX, userID string) ([]membershipRow, error) {
	rows, err := pgsqlgen.New(db).ListFolderMembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]membershipRow, len(rows))
	for i, m := range rows {
		out[i] = membershipRow(m)
	}
	return out, nil
}
func (pgsqlFolderQuerier) AddAccountToFolder(ctx context.Context, db backend.DBTX, p addMemberP) error {
	return pgsqlgen.New(db).AddAccountToFolder(ctx, pgsqlgen.AddAccountToFolderParams(p))
}
func (pgsqlFolderQuerier) RemoveAccountFromFolder(ctx context.Context, db backend.DBTX, p removeMemberP) error {
	return pgsqlgen.New(db).RemoveAccountFromFolder(ctx, pgsqlgen.RemoveAccountFromFolderParams(p))
}
