// Package tagrepo implements domain/tag.Repository over the sqlc-generated
// queries, working uniformly across both database engines.
//
// Engine duality, minimized — identical to the category/user repos' approach.
// The whole repository (every method, plus row<->domain mapping) is written ONCE
// here against a single `querier` interface expressed in the canonical (sqlite-
// generated) types. The sqlite adapter (sqlite.go) is a near-native passthrough;
// the pgsql adapter (pgsql.go) is a thin whole-struct conversion shim. The
// engine is chosen once at construction; every query runs through
// TxManager.Querier(ctx) so it transparently joins the active transaction.
//
// Idempotency for create-tag is NOT here — it is the shared
// internal/infra/repo/operation.Guard, wired alongside this repo in main.go.
package tagrepo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtag "github.com/econumo/econumo/internal/domain/tag"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// Canonical row/param types: the sqlc-generated types are field-identical across
// engines, so the repo speaks one engine's types (sqlite's) everywhere and the
// pgsql shim copies into them.
type (
	tagRow       = sqlitegen.Tag
	upsertParams = sqlitegen.UpsertTagParams
)

// querier is the engine-agnostic query surface this repo needs, expressed in the
// canonical types. The two engine adapters (sqlite.go / pgsql.go) implement it.
type querier interface {
	GetTagByID(ctx context.Context, db backend.DBTX, id string) (tagRow, error)
	ListTagsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]tagRow, error)
	CountTagsByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertTag(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteTag(ctx context.Context, db backend.DBTX, id string) error
}

// Repo is the concrete tag repository. It holds the TxManager (source of the
// context-bound DBTX) and the engine querier. It satisfies domain/tag.Repository.
type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ domtag.Repository = (*Repo)(nil)

// NewRepo selects the engine querier by driver name, panicking on an unknown
// driver. driver matches config.DatabaseDriver: "sqlite" | "postgresql".
func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("tagrepo: unknown database driver " + driver)
	}
}

// NewSQLiteRepo builds a tag repository backed by the sqlite queries.
func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}}
}

// NewPgsqlRepo builds a tag repository backed by the pgsql queries.
func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// NextIdentity allocates a fresh tag id.
func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

// GetByID loads a tag by id.
func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*domtag.Tag, error) {
	row, err := r.q.GetTagByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Tag not found")
		}
		return nil, err
	}
	return hydrate(row)
}

// ListByOwner returns the owner's tags ordered by position.
func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*domtag.Tag, error) {
	rows, err := r.q.ListTagsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*domtag.Tag, 0, len(rows))
	for _, row := range rows {
		t, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, t)
	}
	return out, nil
}

// CountByOwner returns the number of tags the owner has.
func (r *Repo) CountByOwner(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountTagsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Save upserts a tag. The caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, t *domtag.Tag) error {
	return r.q.UpsertTag(ctx, r.db(ctx), upsertParams{
		ID:         t.Id().String(),
		UserID:     t.UserId().String(),
		Name:       t.Name(),
		Position:   t.Position(),
		IsArchived: t.IsArchived(),
		CreatedAt:  t.CreatedAt(),
		UpdatedAt:  t.UpdatedAt(),
	})
}

// Delete removes a tag by id.
func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteTag(ctx, r.db(ctx), id.String())
}

// hydrate reconstitutes a Tag aggregate from a row.
func hydrate(row tagRow) (*domtag.Tag, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return domtag.FromState(
		id, userID, row.Name, row.Position, row.IsArchived, row.CreatedAt, row.UpdatedAt,
	), nil
}
