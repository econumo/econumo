package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// inviteRow is the engine-agnostic invite row (code + expiry both nullable).
type inviteRow struct {
	UserID    string
	Code      *string
	ExpiredAt *time.Time
}

// inviteQuerier is the engine surface the invite repo needs. getByCode takes
// `now` already rendered the way the engine compares: a 'Y-m-d H:i:s' string for
// sqlite (so it compares against the stored datetime TEXT via datetime()), a
// time.Time for pgsql.
type inviteQuerier interface {
	GetByUser(ctx context.Context, db backend.DBTX, userID string) (inviteRow, error)
	GetByCode(ctx context.Context, db backend.DBTX, code string, now time.Time) (inviteRow, error)
	Upsert(ctx context.Context, db backend.DBTX, userID string, code *string, expiredAt *time.Time) error
}

// InviteRepo implements connection.InviteRepository over
// users_connections_invites (one row per user, code unique).
type InviteRepo struct {
	tx *backend.TxManager
	q  inviteQuerier
}

var _ domconnection.InviteRepository = (*InviteRepo)(nil)

// NewInviteRepo selects the engine adapter by driver name.
func NewInviteRepo(driver string, tx *backend.TxManager) *InviteRepo {
	switch driver {
	case "sqlite":
		return &InviteRepo{tx: tx, q: sqliteInviteQuerier{}}
	case "postgresql":
		return &InviteRepo{tx: tx, q: pgsqlInviteQuerier{}}
	default:
		panic("connectionrepo: unknown database driver " + driver)
	}
}

func (r *InviteRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// GetByUser returns the user's invite row, or nil if they have none.
func (r *InviteRepo) GetByUser(ctx context.Context, userID vo.Id) (*domconnection.ConnectionInvite, error) {
	row, err := r.q.GetByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return hydrateInvite(row)
}

// GetByCode returns the non-expired invite bearing the code; NotFound otherwise.
func (r *InviteRepo) GetByCode(ctx context.Context, code domconnection.ConnectionCode, now time.Time) (*domconnection.ConnectionInvite, error) {
	row, err := r.q.GetByCode(ctx, r.db(ctx), code.Value(), now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("ConnectionInvite not found")
		}
		return nil, err
	}
	return hydrateInvite(row)
}

// Save upserts the user's invite row.
func (r *InviteRepo) Save(ctx context.Context, inv *domconnection.ConnectionInvite) error {
	var code *string
	if c := inv.Code(); !c.IsZero() {
		v := c.Value()
		code = &v
	}
	return r.q.Upsert(ctx, r.db(ctx), inv.UserId().String(), code, inv.ExpiredAt())
}

func hydrateInvite(row inviteRow) (*domconnection.ConnectionInvite, error) {
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	code := ""
	if row.Code != nil {
		code = *row.Code
	}
	return domconnection.InviteFromState(userID, code, row.ExpiredAt), nil
}

type sqliteInviteQuerier struct{}

func (sqliteInviteQuerier) GetByUser(ctx context.Context, db backend.DBTX, userID string) (inviteRow, error) {
	row, err := sqlitegen.New(db).GetConnectionInviteByUser(ctx, userID)
	return inviteRow{UserID: row.UserID, Code: row.Code, ExpiredAt: row.ExpiredAt}, err
}
func (sqliteInviteQuerier) GetByCode(ctx context.Context, db backend.DBTX, code string, now time.Time) (inviteRow, error) {
	// sqlite compares datetime(expired_at) >= datetime(?) with a 'Y-m-d H:i:s'
	// string bound (a time.Time mis-compares against the stored datetime TEXT).
	row, err := sqlitegen.New(db).GetConnectionInviteByCode(ctx, sqlitegen.GetConnectionInviteByCodeParams{
		Code: &code, Datetime: now.Format(datetime.Layout),
	})
	return inviteRow{UserID: row.UserID, Code: row.Code, ExpiredAt: row.ExpiredAt}, err
}
func (sqliteInviteQuerier) Upsert(ctx context.Context, db backend.DBTX, userID string, code *string, expiredAt *time.Time) error {
	// Store expired_at as a 'Y-m-d H:i:s' string (not a *time.Time): the modernc
	// driver serializes time.Time as RFC3339 with a 'T'/'Z'/fractional seconds,
	// which SQLite's datetime() CANNOT parse (it returns ""), breaking the
	// by-code expiry comparison. A plain 'Y-m-d H:i:s' string is what datetime()
	// expects. Done via a raw upsert since the generated param type is *time.Time.
	// (Same SQLite datetime-binding gotcha handled in the budget repo.)
	var exp any
	if expiredAt != nil {
		exp = expiredAt.Format(datetime.Layout)
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO users_connections_invites (user_id, code, expired_at) VALUES (?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET code = excluded.code, expired_at = excluded.expired_at`,
		userID, code, exp)
	return err
}

type pgsqlInviteQuerier struct{}

func (pgsqlInviteQuerier) GetByUser(ctx context.Context, db backend.DBTX, userID string) (inviteRow, error) {
	row, err := pgsqlgen.New(db).GetConnectionInviteByUser(ctx, userID)
	return inviteRow{UserID: row.UserID, Code: row.Code, ExpiredAt: row.ExpiredAt}, err
}
func (pgsqlInviteQuerier) GetByCode(ctx context.Context, db backend.DBTX, code string, now time.Time) (inviteRow, error) {
	row, err := pgsqlgen.New(db).GetConnectionInviteByCode(ctx, pgsqlgen.GetConnectionInviteByCodeParams{
		Code: &code, ExpiredAt: &now,
	})
	return inviteRow{UserID: row.UserID, Code: row.Code, ExpiredAt: row.ExpiredAt}, err
}
func (pgsqlInviteQuerier) Upsert(ctx context.Context, db backend.DBTX, userID string, code *string, expiredAt *time.Time) error {
	return pgsqlgen.New(db).UpsertConnectionInvite(ctx, pgsqlgen.UpsertConnectionInviteParams{
		UserID: userID, Code: code, ExpiredAt: expiredAt,
	})
}
