// Package passwordrequestrepo persists password-reset codes
// (users_password_requests) for the user module's remind/reset flow.
//
// Engine duality, minimized — the same approach as the tag/category/user repos.
// Every method is written ONCE against a single `querier` interface expressed in
// the canonical (sqlite-generated) types. The sqlite adapter (sqlite.go) is a
// native passthrough; the pgsql adapter (pgsql.go) is a thin whole-struct
// conversion shim. The engine is chosen once at construction, so the method
// bodies carry no per-driver branching. Every query runs through
// TxManager.Querier(ctx) so it transparently joins the active transaction.
//
// Expiry is evaluated in the domain (PasswordRequest.IsExpired), not in SQL.
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
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// Canonical row/param types: the sqlc-generated types are field-identical across
// engines, so the repo speaks one engine's types (sqlite's) everywhere and the
// pgsql shim copies into them.
type (
	passwordRequestRow     = sqlitegen.UsersPasswordRequest
	getByUserAndCodeParams = sqlitegen.GetUserPasswordRequestByUserAndCodeParams
	insertParams           = sqlitegen.InsertUserPasswordRequestParams
)

// querier is the engine-agnostic query surface this repo needs, expressed in the
// canonical types. The two engine adapters (sqlite.go / pgsql.go) implement it.
type querier interface {
	DeleteUserPasswordRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserPasswordRequest(ctx context.Context, db backend.DBTX, p insertParams) error
	GetUserPasswordRequestByUserAndCode(ctx context.Context, db backend.DBTX, p getByUserAndCodeParams) (passwordRequestRow, error)
	DeleteUserPasswordRequest(ctx context.Context, db backend.DBTX, id string) error
}

// Repo implements app/user.PasswordRequests.
type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ appuser.PasswordRequests = (*Repo)(nil)

// New selects the engine querier by driver name, panicking on an unknown driver.
// driver matches config.DatabaseDriver: "sqlite" | "postgresql".
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

// DeleteByUser removes all of a user's pending reset codes.
func (r *Repo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserPasswordRequestsByUser(ctx, r.db(ctx), userID.String())
}

// Save inserts a new reset request.
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

// GetByUserAndCode loads a user's request matching code, or NotFound.
func (r *Repo) GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*domuser.PasswordRequest, error) {
	row, err := r.q.GetUserPasswordRequestByUserAndCode(ctx, r.db(ctx), getByUserAndCodeParams{UserID: userID.String(), Code: code})
	if err != nil {
		return nil, mapErr(err)
	}
	return reconstitute(row.ID, row.UserID, row.Code, row.CreatedAt, row.UpdatedAt, row.ExpiredAt)
}

// Delete removes a request by id.
func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteUserPasswordRequest(ctx, r.db(ctx), id.String())
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
