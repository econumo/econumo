// Package idempotency stores Idempotency-Key rows: one per (user, key), holding
// the id of the resource the first request with that key created, so a retried
// offline write is recognised instead of applied twice.
package idempotency

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	retention     = 90 * 24 * time.Hour
	purgeInterval = 10 * time.Minute
)

type (
	getParams    = sqlitegen.GetIdempotencyKeyParams
	insertParams = sqlitegen.InsertIdempotencyKeyParams
	deleteParams = sqlitegen.DeleteIdempotencyKeyParams
)

type querier interface {
	Get(ctx context.Context, db backend.DBTX, p getParams) (*string, error)
	Insert(ctx context.Context, db backend.DBTX, p insertParams) error
	Delete(ctx context.Context, db backend.DBTX, p deleteParams) error
	Purge(ctx context.Context, db backend.DBTX, before time.Time) error
}

// Store runs every query through TxManager.Querier(ctx), so it joins the
// caller's active transaction.
type Store struct {
	tx    *backend.TxManager
	q     querier
	clock port.Clock

	mu        sync.Mutex
	lastPurge time.Time
}

func NewStore(driver string, tx *backend.TxManager, clock port.Clock) *Store {
	switch driver {
	case "sqlite":
		return &Store{tx: tx, q: sqliteQuerier{}, clock: clock}
	case "postgresql":
		return &Store{tx: tx, q: pgsqlQuerier{}, clock: clock}
	default:
		panic("idempotency: unknown database driver " + driver)
	}
}

// Find returns the resource id stored for the key; "" when the row has none.
func (s *Store) Find(ctx context.Context, userID vo.Id, key string) (string, bool, error) {
	r, err := s.q.Get(ctx, s.tx.Querier(ctx), getParams{UserID: userID.String(), IdempotencyKey: key})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if r == nil {
		return "", true, nil
	}
	return *r, true, nil
}

// Insert fails on a duplicate (user, key); an empty resourceID stores NULL.
func (s *Store) Insert(ctx context.Context, userID vo.Id, key, resourceID string) error {
	var res *string
	if resourceID != "" {
		res = &resourceID
	}
	return s.q.Insert(ctx, s.tx.Querier(ctx), insertParams{
		UserID:         userID.String(),
		IdempotencyKey: key,
		ResourceID:     res,
		CreatedAt:      s.clock.Now(),
	})
}

func (s *Store) Delete(ctx context.Context, userID vo.Id, key string) error {
	return s.q.Delete(ctx, s.tx.Querier(ctx), deleteParams{UserID: userID.String(), IdempotencyKey: key})
}

// MaybePurge drops rows older than ninety days, at most once every ten minutes
// per process. Best-effort: a failure is logged and never reaches the request.
func (s *Store) MaybePurge(ctx context.Context) {
	now := s.clock.Now()
	s.mu.Lock()
	if !s.lastPurge.IsZero() && now.Sub(s.lastPurge) < purgeInterval {
		s.mu.Unlock()
		return
	}
	s.lastPurge = now
	s.mu.Unlock()
	if err := s.q.Purge(ctx, s.tx.Querier(ctx), now.Add(-retention)); err != nil {
		slog.WarnContext(ctx, "purge idempotency keys", "err", err)
	}
}

type sqliteQuerier struct{}

func (sqliteQuerier) Get(ctx context.Context, db backend.DBTX, p getParams) (*string, error) {
	return sqlitegen.New(db).GetIdempotencyKey(ctx, p)
}

func (sqliteQuerier) Insert(ctx context.Context, db backend.DBTX, p insertParams) error {
	return sqlitegen.New(db).InsertIdempotencyKey(ctx, p)
}

func (sqliteQuerier) Delete(ctx context.Context, db backend.DBTX, p deleteParams) error {
	return sqlitegen.New(db).DeleteIdempotencyKey(ctx, p)
}

func (sqliteQuerier) Purge(ctx context.Context, db backend.DBTX, before time.Time) error {
	return sqlitegen.New(db).PurgeIdempotencyKeys(ctx, before)
}

type pgsqlQuerier struct{}

func (pgsqlQuerier) Get(ctx context.Context, db backend.DBTX, p getParams) (*string, error) {
	return pgsqlgen.New(db).GetIdempotencyKey(ctx, pgsqlgen.GetIdempotencyKeyParams(p))
}

func (pgsqlQuerier) Insert(ctx context.Context, db backend.DBTX, p insertParams) error {
	return pgsqlgen.New(db).InsertIdempotencyKey(ctx, pgsqlgen.InsertIdempotencyKeyParams(p))
}

func (pgsqlQuerier) Delete(ctx context.Context, db backend.DBTX, p deleteParams) error {
	return pgsqlgen.New(db).DeleteIdempotencyKey(ctx, pgsqlgen.DeleteIdempotencyKeyParams(p))
}

func (pgsqlQuerier) Purge(ctx context.Context, db backend.DBTX, before time.Time) error {
	return pgsqlgen.New(db).PurgeIdempotencyKeys(ctx, before)
}
