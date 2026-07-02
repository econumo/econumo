// Package passwordrequestrepo persists password-reset codes
// (users_password_requests) for the user module's remind/reset flow.
//
// Expiry is evaluated in the domain (PasswordRequest.IsExpired), not in SQL.
package passwordrequestrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	appuser "github.com/econumo/econumo/internal/app/user"
	domuser "github.com/econumo/econumo/internal/domain/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	passwordRequestRow     = sqlitegen.UsersPasswordRequest
	getByUserAndCodeParams = sqlitegen.GetUserPasswordRequestByUserAndCodeParams
	insertParams           = sqlitegen.InsertUserPasswordRequestParams
)

type querier interface {
	DeleteUserPasswordRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserPasswordRequest(ctx context.Context, db backend.DBTX, p insertParams) error
	GetUserPasswordRequestByUserAndCode(ctx context.Context, db backend.DBTX, p getByUserAndCodeParams) (passwordRequestRow, error)
	DeleteUserPasswordRequest(ctx context.Context, db backend.DBTX, id string) error
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ appuser.PasswordRequests = (*Repo)(nil)

func New(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("passwordrequestrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserPasswordRequestsByUser(ctx, r.db(ctx), userID.String())
}

func (r *Repo) Save(ctx context.Context, pr *domuser.PasswordRequest) error {
	return r.q.InsertUserPasswordRequest(ctx, r.db(ctx), insertParams{
		ID:        pr.Id().String(),
		UserID:    pr.UserId().String(),
		Code:      pr.Code(),
		CreatedAt: pr.CreatedAt(),
		UpdatedAt: pr.UpdatedAt(),
		ExpiredAt: pr.ExpiredAt(),
	})
}

func (r *Repo) GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*domuser.PasswordRequest, error) {
	row, err := r.q.GetUserPasswordRequestByUserAndCode(ctx, r.db(ctx), getByUserAndCodeParams{UserID: userID.String(), Code: code})
	if err != nil {
		return nil, mapErr(err)
	}
	return reconstitute(row.ID, row.UserID, row.Code, row.CreatedAt, row.UpdatedAt, row.ExpiredAt)
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteUserPasswordRequest(ctx, r.db(ctx), id.String())
}

func reconstitute(id, userID, code string, created, updated, expired time.Time) (*domuser.PasswordRequest, error) {
	rid, err := vo.ParseId(id)
	if err != nil {
		return nil, err
	}
	uid, err := vo.ParseId(userID)
	if err != nil {
		return nil, err
	}
	return domuser.ReconstitutePasswordRequest(rid, uid, code, created, updated, expired), nil
}

func mapErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.NewNotFound("Password request not found")
	}
	return err
}
