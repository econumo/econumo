package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	handoffRow          = sqlitegen.OauthHandoff
	insertHandoffParams = sqlitegen.InsertOAuthHandoffParams
)

type handoffQuerier interface {
	InsertOAuthHandoff(ctx context.Context, db backend.DBTX, p insertHandoffParams) error
	GetOAuthHandoff(ctx context.Context, db backend.DBTX, codeHash string) (handoffRow, error)
	DeleteOAuthHandoff(ctx context.Context, db backend.DBTX, codeHash string) error
	DeleteExpiredOAuthHandoffs(ctx context.Context, db backend.DBTX, cutoff time.Time) (int64, error)
}

type HandoffRepo struct {
	tx *backend.TxManager
	q  handoffQuerier
}

var _ appoauth.Handoffs = (*HandoffRepo)(nil)

func NewHandoffRepo(driver string, tx *backend.TxManager) *HandoffRepo {
	switch driver {
	case "sqlite":
		return &HandoffRepo{tx: tx, q: handoffSqliteQuerier{}}
	case "postgresql":
		return &HandoffRepo{tx: tx, q: handoffPgsqlQuerier{}}
	default:
		panic("oauthrepo: unknown database driver " + driver)
	}
}

func (r *HandoffRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *HandoffRepo) Insert(ctx context.Context, h *model.OAuthHandoff) error {
	return r.q.InsertOAuthHandoff(ctx, r.db(ctx), insertHandoffParams{
		CodeHash: h.CodeHash, UserID: h.UserID.String(), Provider: h.Provider, FlowHash: h.FlowHash,
		IDToken: h.IDToken, CreatedAt: h.CreatedAt, ExpiresAt: h.ExpiresAt,
	})
}

func (r *HandoffRepo) Get(ctx context.Context, codeHash string) (*model.OAuthHandoff, error) {
	row, err := r.q.GetOAuthHandoff(ctx, r.db(ctx), codeHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Handoff not found")
		}
		return nil, err
	}
	uid, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.OAuthHandoff{CodeHash: row.CodeHash, UserID: uid, Provider: row.Provider, FlowHash: row.FlowHash,
		IDToken: row.IDToken, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}, nil
}

func (r *HandoffRepo) Delete(ctx context.Context, codeHash string) error {
	return r.q.DeleteOAuthHandoff(ctx, r.db(ctx), codeHash)
}

func (r *HandoffRepo) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.DeleteExpiredOAuthHandoffs(ctx, r.db(ctx), cutoff)
}
