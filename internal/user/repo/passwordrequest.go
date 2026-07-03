// PasswordRequestRepo persists password-reset codes (users_password_requests)
// for the remind/reset flow.
//
// Expiry is evaluated in the domain (PasswordRequest.IsExpired), not in SQL.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

type (
	passwordRequestRow     = sqlitegen.UsersPasswordRequest
	getByUserAndCodeParams = sqlitegen.GetUserPasswordRequestByUserAndCodeParams
	insertParams           = sqlitegen.InsertUserPasswordRequestParams
)

type passwordRequestQuerier interface {
	DeleteUserPasswordRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserPasswordRequest(ctx context.Context, db backend.DBTX, p insertParams) error
	GetUserPasswordRequestByUserAndCode(ctx context.Context, db backend.DBTX, p getByUserAndCodeParams) (passwordRequestRow, error)
	DeleteUserPasswordRequest(ctx context.Context, db backend.DBTX, id string) error
}

type PasswordRequestRepo struct {
	tx *backend.TxManager
	q  passwordRequestQuerier
}

var _ user.PasswordRequests = (*PasswordRequestRepo)(nil)

func NewPasswordRequestRepo(driver string, tx *backend.TxManager) *PasswordRequestRepo {
	switch driver {
	case "sqlite":
		return &PasswordRequestRepo{tx: tx, q: passwordRequestSqliteQuerier{}}
	case "postgresql":
		return &PasswordRequestRepo{tx: tx, q: passwordRequestPgsqlQuerier{}}
	default:
		panic("passwordrequestrepo: unknown database driver " + driver)
	}
}

func (r *PasswordRequestRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *PasswordRequestRepo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserPasswordRequestsByUser(ctx, r.db(ctx), userID.String())
}

func (r *PasswordRequestRepo) Save(ctx context.Context, pr *user.PasswordRequest) error {
	return r.q.InsertUserPasswordRequest(ctx, r.db(ctx), insertParams{
		ID:        pr.ID.String(),
		UserID:    pr.UserID.String(),
		Code:      pr.Code,
		CreatedAt: pr.CreatedAt,
		UpdatedAt: pr.UpdatedAt,
		ExpiredAt: pr.ExpiredAt,
	})
}

func (r *PasswordRequestRepo) GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*user.PasswordRequest, error) {
	row, err := r.q.GetUserPasswordRequestByUserAndCode(ctx, r.db(ctx), getByUserAndCodeParams{UserID: userID.String(), Code: code})
	if err != nil {
		return nil, mapErr(err)
	}
	return reconstitute(row.ID, row.UserID, row.Code, row.CreatedAt, row.UpdatedAt, row.ExpiredAt)
}

func (r *PasswordRequestRepo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteUserPasswordRequest(ctx, r.db(ctx), id.String())
}

func reconstitute(id, userID, code string, created, updated, expired time.Time) (*user.PasswordRequest, error) {
	rid, err := vo.ParseId(id)
	if err != nil {
		return nil, err
	}
	uid, err := vo.ParseId(userID)
	if err != nil {
		return nil, err
	}
	return &user.PasswordRequest{ID: rid, UserID: uid, Code: code, CreatedAt: created, UpdatedAt: updated, ExpiredAt: expired}, nil
}

func mapErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.NewNotFound("Password request not found")
	}
	return err
}
