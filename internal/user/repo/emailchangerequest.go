// EmailChangeRequestRepo persists pending self-service email changes
// (users_email_change_requests). Expiry is evaluated in the domain
// (EmailChangeRequest.IsExpired), not in SQL.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

type (
	emailChangeRow          = sqlitegen.UsersEmailChangeRequest
	emailChangeInsertParams = sqlitegen.InsertUserEmailChangeRequestParams
)

type emailChangeQuerier interface {
	DeleteUserEmailChangeRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserEmailChangeRequest(ctx context.Context, db backend.DBTX, p emailChangeInsertParams) error
	GetUserEmailChangeRequestByUser(ctx context.Context, db backend.DBTX, userID string) (emailChangeRow, error)
}

type EmailChangeRequestRepo struct {
	tx *backend.TxManager
	q  emailChangeQuerier
}

var _ user.EmailChangeRequests = (*EmailChangeRequestRepo)(nil)

func NewEmailChangeRequestRepo(driver string, tx *backend.TxManager) *EmailChangeRequestRepo {
	switch driver {
	case "sqlite":
		return &EmailChangeRequestRepo{tx: tx, q: emailChangeSqliteQuerier{}}
	case "postgresql":
		return &EmailChangeRequestRepo{tx: tx, q: emailChangePgsqlQuerier{}}
	default:
		panic("emailchangerequestrepo: unknown database driver " + driver)
	}
}

func (r *EmailChangeRequestRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *EmailChangeRequestRepo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserEmailChangeRequestsByUser(ctx, r.db(ctx), userID.String())
}

func (r *EmailChangeRequestRepo) Save(ctx context.Context, cr *model.EmailChangeRequest) error {
	return r.q.InsertUserEmailChangeRequest(ctx, r.db(ctx), emailChangeInsertParams{
		ID:        cr.ID.String(),
		UserID:    cr.UserID.String(),
		NewEmail:  cr.NewEmail,
		Code:      cr.Code,
		CreatedAt: cr.CreatedAt,
		UpdatedAt: cr.UpdatedAt,
		ExpiredAt: cr.ExpiredAt,
	})
}

func (r *EmailChangeRequestRepo) GetByUser(ctx context.Context, userID vo.Id) (*model.EmailChangeRequest, error) {
	row, err := r.q.GetUserEmailChangeRequestByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Email change request not found")
		}
		return nil, err
	}
	id, perr := vo.ParseId(row.ID)
	if perr != nil {
		return nil, perr
	}
	uid, perr := vo.ParseId(row.UserID)
	if perr != nil {
		return nil, perr
	}
	return &model.EmailChangeRequest{ID: id, UserID: uid, NewEmail: row.NewEmail, Code: row.Code,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ExpiredAt: row.ExpiredAt}, nil
}
