// EmailVerificationRepo persists login email-verification codes
// (users_email_verifications). Expiry is evaluated in the domain
// (EmailVerification.IsExpired), not in SQL.
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
	emailVerificationRow           = sqlitegen.UsersEmailVerification
	emailVerificationInsertParams  = sqlitegen.InsertUserEmailVerificationParams
	emailVerificationConsumeParams = sqlitegen.ConsumeUserEmailVerificationParams
)

type emailVerificationQuerier interface {
	DeleteUserEmailVerificationsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserEmailVerification(ctx context.Context, db backend.DBTX, p emailVerificationInsertParams) error
	GetUserEmailVerificationByUser(ctx context.Context, db backend.DBTX, userID string) (emailVerificationRow, error)
	ConsumeUserEmailVerification(ctx context.Context, db backend.DBTX, p emailVerificationConsumeParams) (int64, error)
}

type EmailVerificationRepo struct {
	tx *backend.TxManager
	q  emailVerificationQuerier
}

var _ user.EmailVerifications = (*EmailVerificationRepo)(nil)

func NewEmailVerificationRepo(driver string, tx *backend.TxManager) *EmailVerificationRepo {
	switch driver {
	case "sqlite":
		return &EmailVerificationRepo{tx: tx, q: emailVerificationSqliteQuerier{}}
	case "postgresql":
		return &EmailVerificationRepo{tx: tx, q: emailVerificationPgsqlQuerier{}}
	default:
		panic("emailverificationrepo: unknown database driver " + driver)
	}
}

func (r *EmailVerificationRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *EmailVerificationRepo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserEmailVerificationsByUser(ctx, r.db(ctx), userID.String())
}

// Consume takes the checked code back row-counted (see user.EmailVerifications).
func (r *EmailVerificationRepo) Consume(ctx context.Context, id, userID vo.Id) (int64, error) {
	return r.q.ConsumeUserEmailVerification(ctx, r.db(ctx), emailVerificationConsumeParams{
		ID:     id.String(),
		UserID: userID.String(),
	})
}

func (r *EmailVerificationRepo) Save(ctx context.Context, v *model.EmailVerification) error {
	return r.q.InsertUserEmailVerification(ctx, r.db(ctx), emailVerificationInsertParams{
		ID:        v.ID.String(),
		UserID:    v.UserID.String(),
		Code:      v.Code,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
		ExpiredAt: v.ExpiredAt,
	})
}

func (r *EmailVerificationRepo) GetByUser(ctx context.Context, userID vo.Id) (*model.EmailVerification, error) {
	row, err := r.q.GetUserEmailVerificationByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Email verification not found")
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
	return &model.EmailVerification{ID: id, UserID: uid, Code: row.Code,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ExpiredAt: row.ExpiredAt}, nil
}
