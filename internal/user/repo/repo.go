// Package repo implements the user feature's persistence: Repo (the
// write-side user.Repository), ReadRepo (the CQRS read side), and
// PasswordRequestRepo (the remind/reset flow's password-request store).
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

// userRow mirrors the sqlc-generated GetUserByID*Row shape (both dialects):
// the users SELECTs list columns explicitly and don't include language (it's
// write-only, see Repository.UpdateLanguage), so sqlc emits per-query row
// types instead of reusing User; this struct is the common shape both
// convert to via a plain field-for-field type conversion.
type (
	userRow struct {
		ID            string
		Identifier    string
		Email         string
		Name          string
		Avatar        string
		Password      string
		Salt          string
		CreatedAt     time.Time
		UpdatedAt     time.Time
		IsActive      bool
		Algorithm     string
		AccessLevel   string
		AccessUntil   *time.Time
		Timezone      string
		EmailVerified bool
	}
	optionRow      = sqlitegen.UsersOption
	userParams     = sqlitegen.UpsertUserParams
	optionParams   = sqlitegen.UpsertUserOptionParams
	languageParams = sqlitegen.UpdateUserLanguageParams
	timezoneParams = sqlitegen.UpdateUserTimezoneParams
)

type querier interface {
	GetUserByID(ctx context.Context, db backend.DBTX, id string) (userRow, error)
	GetUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (userRow, error)
	ExistsUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (bool, error)
	GetUserByEmail(ctx context.Context, db backend.DBTX, email string) (userRow, error)
	ExistsUserByEmail(ctx context.Context, db backend.DBTX, email string) (bool, error)
	ListUserIDs(ctx context.Context, db backend.DBTX) ([]string, error)
	UpsertUser(ctx context.Context, db backend.DBTX, p userParams) error
	GetUserOptions(ctx context.Context, db backend.DBTX, userID string) ([]optionRow, error)
	UpsertUserOption(ctx context.Context, db backend.DBTX, p optionParams) error
	UpdateUserLanguage(ctx context.Context, db backend.DBTX, p languageParams) error
	GetUserTimezone(ctx context.Context, db backend.DBTX, id string) (string, error)
	UpdateUserTimezone(ctx context.Context, db backend.DBTX, p timezoneParams) error
	GetUserLanguage(ctx context.Context, db backend.DBTX, id string) (string, error)
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ user.Repository = (*Repo)(nil)

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

func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}}
}

func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.User, error) {
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
func (r *Repo) GetHeaderByID(ctx context.Context, id vo.Id) (model.Header, error) {
	row, err := r.q.GetUserByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Header{}, errs.NewNotFound("User not found")
		}
		return model.Header{}, err
	}
	return model.Header{
		ID: row.ID, Name: row.Name, Avatar: row.Avatar,
		AccessLevel: model.AccessLevel(row.AccessLevel), AccessUntil: row.AccessUntil,
	}, nil
}

func (r *Repo) GetByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	row, err := r.q.GetUserByIdentifier(ctx, r.db(ctx), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("User not found")
		}
		return nil, err
	}
	return r.hydrate(ctx, row)
}

func (r *Repo) ExistsByIdentifier(ctx context.Context, identifier string) (bool, error) {
	return r.q.ExistsUserByIdentifier(ctx, r.db(ctx), identifier)
}

func (r *Repo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	row, err := r.q.GetUserByEmail(ctx, r.db(ctx), email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("User not found")
		}
		return nil, err
	}
	return r.hydrate(ctx, row)
}

func (r *Repo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.q.ExistsUserByEmail(ctx, r.db(ctx), email)
}

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

func (r *Repo) GetOptions(ctx context.Context, userID vo.Id) ([]model.UserOption, error) {
	rows, err := r.q.GetUserOptions(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return toDomainOptions(rows)
}

// Save upserts the user row and all of its option rows. The caller runs this
// inside TxManager.WithTx so the row + options commit atomically.
func (r *Repo) Save(ctx context.Context, u *model.User) error {
	db := r.db(ctx)
	if err := r.q.UpsertUser(ctx, db, userParams{
		ID:            u.ID.String(),
		Identifier:    u.Identifier,
		Email:         u.Email,
		Name:          u.Name,
		Avatar:        u.Avatar,
		Password:      u.Password,
		Salt:          u.Salt,
		Algorithm:     u.Algorithm,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
		IsActive:      u.IsActive,
		AccessLevel:   string(u.AccessLevel),
		AccessUntil:   u.AccessUntil,
		EmailVerified: u.EmailVerified,
	}); err != nil {
		return err
	}
	for _, o := range u.Options {
		if err := r.q.UpsertUserOption(ctx, db, optionParams{
			ID:        o.ID.String(),
			UserID:    u.ID.String(),
			Name:      o.Name,
			Value:     o.Value,
			CreatedAt: o.CreatedAt,
			UpdatedAt: o.UpdatedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

// UpdateLanguage persists the user's last selected UI language, write-only
// (see user.Repository.UpdateLanguage). A missing id simply affects 0 rows.
func (r *Repo) UpdateLanguage(ctx context.Context, id vo.Id, language string) error {
	return r.q.UpdateUserLanguage(ctx, r.db(ctx), languageParams{
		Language: language,
		ID:       id.String(),
	})
}

func (r *Repo) GetTimezone(ctx context.Context, id vo.Id) (string, error) {
	tz, err := r.q.GetUserTimezone(ctx, r.db(ctx), id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return "", errs.NewNotFound("User not found")
	}
	return tz, err
}

func (r *Repo) UpdateTimezone(ctx context.Context, id vo.Id, tz string) error {
	return r.q.UpdateUserTimezone(ctx, r.db(ctx), timezoneParams{Timezone: tz, ID: id.String()})
}

func (r *Repo) GetLanguage(ctx context.Context, id vo.Id) (string, error) {
	lang, err := r.q.GetUserLanguage(ctx, r.db(ctx), id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return "", errs.NewNotFound("User not found")
	}
	return lang, err
}

func (r *Repo) hydrate(ctx context.Context, row userRow) (*model.User, error) {
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
	return &model.User{ID: id, Identifier: row.Identifier, Email: row.Email, Name: row.Name,
		Avatar: row.Avatar, Password: row.Password, Salt: row.Salt, Algorithm: row.Algorithm,
		IsActive: row.IsActive, EmailVerified: row.EmailVerified, AccessLevel: model.AccessLevel(row.AccessLevel), AccessUntil: row.AccessUntil,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Options: opts}, nil
}

func toDomainOptions(rows []optionRow) ([]model.UserOption, error) {
	opts := make([]model.UserOption, 0, len(rows))
	for _, o := range rows {
		oid, err := vo.ParseId(o.ID)
		if err != nil {
			return nil, err
		}
		opts = append(opts, model.ReconstituteUserOption(oid, o.Name, o.Value, o.CreatedAt, o.UpdatedAt))
	}
	return opts, nil
}
