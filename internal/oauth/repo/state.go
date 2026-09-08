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
	stateRow          = sqlitegen.OauthState
	insertStateParams = sqlitegen.InsertOAuthStateParams
)

type stateQuerier interface {
	InsertOAuthState(ctx context.Context, db backend.DBTX, p insertStateParams) error
	GetOAuthState(ctx context.Context, db backend.DBTX, hash string) (stateRow, error)
	DeleteOAuthState(ctx context.Context, db backend.DBTX, hash string) error
	DeleteExpiredOAuthStates(ctx context.Context, db backend.DBTX, cutoff time.Time) (int64, error)
}

type StateRepo struct {
	tx *backend.TxManager
	q  stateQuerier
}

var _ appoauth.States = (*StateRepo)(nil)

func NewStateRepo(driver string, tx *backend.TxManager) *StateRepo {
	switch driver {
	case "sqlite":
		return &StateRepo{tx: tx, q: stateSqliteQuerier{}}
	case "postgresql":
		return &StateRepo{tx: tx, q: statePgsqlQuerier{}}
	default:
		panic("oauthrepo: unknown database driver " + driver)
	}
}

func (r *StateRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *StateRepo) Insert(ctx context.Context, s *model.OAuthState) error {
	var link *string
	if !s.LinkUserID.IsZero() {
		v := s.LinkUserID.String()
		link = &v
	}
	return r.q.InsertOAuthState(ctx, r.db(ctx), insertStateParams{
		StateHash: s.StateHash, Provider: s.Provider, Nonce: s.Nonce, CodeVerifier: s.CodeVerifier,
		FlowHash: s.FlowHash, Client: s.Client, Intent: s.Intent, LinkUserID: link,
		CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt,
	})
}

func (r *StateRepo) Get(ctx context.Context, hash string) (*model.OAuthState, error) {
	row, err := r.q.GetOAuthState(ctx, r.db(ctx), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("State not found")
		}
		return nil, err
	}
	s := &model.OAuthState{StateHash: row.StateHash, Provider: row.Provider, Nonce: row.Nonce, CodeVerifier: row.CodeVerifier,
		FlowHash: row.FlowHash, Client: row.Client, Intent: row.Intent, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}
	if row.LinkUserID != nil {
		id, perr := vo.ParseId(*row.LinkUserID)
		if perr != nil {
			return nil, perr
		}
		s.LinkUserID = id
	}
	return s, nil
}

func (r *StateRepo) Delete(ctx context.Context, hash string) error {
	return r.q.DeleteOAuthState(ctx, r.db(ctx), hash)
}

func (r *StateRepo) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.DeleteExpiredOAuthStates(ctx, r.db(ctx), cutoff)
}
