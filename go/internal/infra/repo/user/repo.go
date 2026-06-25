// Package userrepo implements domain/user.Repository over the sqlc-generated
// queries, working uniformly across both database engines.
//
// Engine duality, minimized. sqlc emits a distinct package per engine
// (sqlitegen and pgsqlgen). Their row/param structs are now byte-identical
// field-for-field (unified via sqlc overrides), but Go still treats
// sqlitegen.User and pgsqlgen.User as *distinct types*, and sqlitegen.New(DBTX)
// vs pgsqlgen.New(DBTX) return different *Queries. So some per-engine selection
// is unavoidable — but it is now reduced to the absolute minimum:
//
//   - The whole repository (every method, plus the row->domain and
//     domain->params mapping) is written ONCE in this file against a single
//     `querier` interface. There is no engine branching anywhere below.
//   - The interface speaks the sqlite-generated types as the canonical shared
//     shapes. The sqlite engine satisfies it almost natively (sqliteQuerier, in
//     sqlite.go) — only ExistsUserByIdentifier's int64->bool needs a one-line
//     wrap. The pgsql engine (pgsqlQuerier, in pgsql.go) is a thin shim that
//     field-copies between the identically-shaped structs.
//
// Cost of the NEXT module's repo: this one file (interface + mapping), plus the
// two short adapters in sqlite.go / pgsql.go. The engine choice is made once at
// construction (NewRepo by driver); every query then runs through
// TxManager.Querier(ctx) so it transparently joins whatever transaction WithTx
// has opened.
package userrepo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/domain/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// Canonical row/param types: the sqlc-generated types are field-identical across
// engines, so the repo speaks one engine's types (sqlite's) everywhere and the
// pgsql shim copies into them. Aliases keep the rest of the file engine-neutral.
type (
	userRow      = sqlitegen.User
	optionRow    = sqlitegen.UsersOption
	userParams   = sqlitegen.UpsertUserParams
	optionParams = sqlitegen.UpsertUserOptionParams
)

// querier is the engine-agnostic query surface this repo needs, expressed in the
// canonical types. Each method takes the context-bound DBTX so the same querier
// value is stateless and always runs on the right executor (pool or active tx).
// The two engine adapters (sqlite.go / pgsql.go) implement it.
type querier interface {
	GetUserByID(ctx context.Context, db backend.DBTX, id string) (userRow, error)
	GetUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (userRow, error)
	ExistsUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (bool, error)
	ListUserIDs(ctx context.Context, db backend.DBTX) ([]string, error)
	UpsertUser(ctx context.Context, db backend.DBTX, p userParams) error
	GetUserOptions(ctx context.Context, db backend.DBTX, userID string) ([]optionRow, error)
	UpsertUserOption(ctx context.Context, db backend.DBTX, p optionParams) error
}

// Repo is the concrete user repository. It holds the TxManager (source of the
// context-bound DBTX) and the engine querier. It satisfies domain/user.Repository.
type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ user.Repository = (*Repo)(nil)

// NewRepo selects the engine querier by driver name, panicking on an unknown
// driver (a wiring mistake should fail loudly at startup). driver matches
// config.DatabaseDriver: "sqlite" | "postgresql".
func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("userrepo: unknown database driver " + driver)
	}
}

// NewSQLiteRepo builds a user repository backed by the sqlite-generated queries.
func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}}
}

// NewPgsqlRepo builds a user repository backed by the pgsql-generated queries.
func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// NextIdentity allocates a fresh user id.
func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

// GetByID loads a user with its options by id.
func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*user.User, error) {
	row, err := r.q.GetUserByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("User not found")
		}
		return nil, err
	}
	return r.hydrate(ctx, row)
}

// GetHeaderByID loads ONLY a user's public display header (id, name, avatar)
// without the options rows. Owner/author embeds use this instead of GetByID so a
// list of N rows costs one user-row query per distinct user, not two (GetByID
// also issues a GetUserOptions query that those embeds never read).
func (r *Repo) GetHeaderByID(ctx context.Context, id vo.Id) (user.Header, error) {
	row, err := r.q.GetUserByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user.Header{}, errs.NewNotFound("User not found")
		}
		return user.Header{}, err
	}
	return user.Header{ID: row.ID, Name: row.Name, AvatarURL: row.AvatarUrl}, nil
}

// GetByIdentifier loads a user with its options by the md5 auth identifier.
func (r *Repo) GetByIdentifier(ctx context.Context, identifier string) (*user.User, error) {
	row, err := r.q.GetUserByIdentifier(ctx, r.db(ctx), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("User not found")
		}
		return nil, err
	}
	return r.hydrate(ctx, row)
}

// ExistsByIdentifier reports whether a user with the identifier exists.
func (r *Repo) ExistsByIdentifier(ctx context.Context, identifier string) (bool, error) {
	return r.q.ExistsUserByIdentifier(ctx, r.db(ctx), identifier)
}

// ListIDs returns all user ids.
func (r *Repo) ListIDs(ctx context.Context) ([]vo.Id, error) {
	raw, err := r.q.ListUserIDs(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	ids := make([]vo.Id, 0, len(raw))
	for _, s := range raw {
		id, perr := vo.ParseId(s)
		if perr != nil {
			return nil, perr
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetOptions loads only the option rows for a user.
func (r *Repo) GetOptions(ctx context.Context, userID vo.Id) ([]user.UserOption, error) {
	rows, err := r.q.GetUserOptions(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return toDomainOptions(rows)
}

// Save upserts the user row and all of its option rows. The caller runs this
// inside TxManager.WithTx so the row + options commit atomically.
func (r *Repo) Save(ctx context.Context, u *user.User) error {
	db := r.db(ctx)
	if err := r.q.UpsertUser(ctx, db, userParams{
		ID:         u.Id().String(),
		Identifier: u.Identifier(),
		Email:      u.Email(),
		Name:       u.Name(),
		AvatarUrl:  u.AvatarURL(),
		Password:   u.Password(),
		Salt:       u.Salt(),
		CreatedAt:  u.CreatedAt(),
		UpdatedAt:  u.UpdatedAt(),
		IsActive:   u.IsActive(),
	}); err != nil {
		return err
	}
	for _, o := range u.Options() {
		if err := r.q.UpsertUserOption(ctx, db, optionParams{
			ID:        o.Id().String(),
			UserID:    u.Id().String(),
			Name:      o.Name(),
			Value:     o.Value(),
			CreatedAt: o.CreatedAt(),
			UpdatedAt: o.UpdatedAt(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// hydrate loads a user's options and reconstitutes the aggregate.
func (r *Repo) hydrate(ctx context.Context, row userRow) (*user.User, error) {
	optRows, err := r.q.GetUserOptions(ctx, r.db(ctx), row.ID)
	if err != nil {
		return nil, err
	}
	opts, err := toDomainOptions(optRows)
	if err != nil {
		return nil, err
	}
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	return user.FromState(
		id, row.Identifier, row.Email, row.Name, row.AvatarUrl,
		row.Password, row.Salt, row.IsActive, row.CreatedAt, row.UpdatedAt, opts,
	), nil
}

func toDomainOptions(rows []optionRow) ([]user.UserOption, error) {
	opts := make([]user.UserOption, 0, len(rows))
	for _, o := range rows {
		oid, err := vo.ParseId(o.ID)
		if err != nil {
			return nil, err
		}
		opts = append(opts, user.ReconstituteUserOption(oid, o.Name, o.Value, o.CreatedAt, o.UpdatedAt))
	}
	return opts, nil
}
