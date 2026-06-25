// Package passwordrequestrepo persists password-reset codes
// (users_password_requests) for the user module's remind/reset flow. Like the
// currency lookup it selects the engine with a per-method driver switch — the
// table has only a handful of tiny queries. Expiry is evaluated in the domain
// (PasswordRequest.IsExpired), not in SQL.
package passwordrequestrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// Repo implements app/user.PasswordRequests.
type Repo struct {
	tx     *backend.TxManager
	driver string
}

var _ appuser.PasswordRequests = (*Repo)(nil)

// New builds the password-request repository for the given engine.
func New(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite", "postgresql":
		return &Repo{tx: tx, driver: driver}
	default:
		panic("passwordrequestrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// DeleteByUser removes all of a user's pending reset codes.
func (r *Repo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	db := r.db(ctx)
	switch r.driver {
	case "sqlite":
		return sqlitegen.New(db).DeleteUserPasswordRequestsByUser(ctx, userID.String())
	default:
		return pgsqlgen.New(db).DeleteUserPasswordRequestsByUser(ctx, userID.String())
	}
}

// Save inserts a new reset request.
func (r *Repo) Save(ctx context.Context, pr *domuser.PasswordRequest) error {
	db := r.db(ctx)
	switch r.driver {
	case "sqlite":
		return sqlitegen.New(db).InsertUserPasswordRequest(ctx, sqlitegen.InsertUserPasswordRequestParams{
			ID:        pr.Id().String(),
			UserID:    pr.UserId().String(),
			Code:      pr.Code(),
			CreatedAt: pr.CreatedAt(),
			UpdatedAt: pr.UpdatedAt(),
			ExpiredAt: pr.ExpiredAt(),
		})
	default:
		return pgsqlgen.New(db).InsertUserPasswordRequest(ctx, pgsqlgen.InsertUserPasswordRequestParams{
			ID:        pr.Id().String(),
			UserID:    pr.UserId().String(),
			Code:      pr.Code(),
			CreatedAt: pr.CreatedAt(),
			UpdatedAt: pr.UpdatedAt(),
			ExpiredAt: pr.ExpiredAt(),
		})
	}
}

// GetByUserAndCode loads a user's request matching code, or NotFound.
func (r *Repo) GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*domuser.PasswordRequest, error) {
	db := r.db(ctx)
	switch r.driver {
	case "sqlite":
		row, err := sqlitegen.New(db).GetUserPasswordRequestByUserAndCode(ctx, sqlitegen.GetUserPasswordRequestByUserAndCodeParams{UserID: userID.String(), Code: code})
		if err != nil {
			return nil, mapErr(err)
		}
		return reconstitute(row.ID, row.UserID, row.Code, row.CreatedAt, row.UpdatedAt, row.ExpiredAt)
	default:
		row, err := pgsqlgen.New(db).GetUserPasswordRequestByUserAndCode(ctx, pgsqlgen.GetUserPasswordRequestByUserAndCodeParams{UserID: userID.String(), Code: code})
		if err != nil {
			return nil, mapErr(err)
		}
		return reconstitute(row.ID, row.UserID, row.Code, row.CreatedAt, row.UpdatedAt, row.ExpiredAt)
	}
}

// Delete removes a request by id.
func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	db := r.db(ctx)
	switch r.driver {
	case "sqlite":
		return sqlitegen.New(db).DeleteUserPasswordRequest(ctx, id.String())
	default:
		return pgsqlgen.New(db).DeleteUserPasswordRequest(ctx, id.String())
	}
}

// reconstitute rebuilds the domain entity from row fields.
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

// mapErr maps a missing row to the domain NotFound error.
func mapErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.NewNotFound("Password request not found")
	}
	return err
}
