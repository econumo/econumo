// Package accountrepo implements domain/account.Repository (accounts +
// accounts_options + the balance read + the balance-correction insert) over the
// sqlc-generated queries.
//
// The EXCEPTION to the usual whole-struct shim is the balance SUM queries:
// SQLite's sqlc rejects sqlc.arg() in subqueries, so the sqlite balance params
// are EXPANDED (repeated positional args) while pgsql dedups them. Those two
// queries therefore get a hand-written per-engine adapter (balanceQuerier) that
// builds each engine's param struct.
package accountrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type (
	accountRow        = sqlitegen.Account
	optionRow         = sqlitegen.AccountsOption
	upsertAccountP    = sqlitegen.UpsertAccountParams
	getOptionP        = sqlitegen.GetAccountOptionParams
	upsertOptionP     = sqlitegen.UpsertAccountOptionParams
	insertCorrectionP = sqlitegen.InsertCorrectionTransactionParams
)

// balanceResult is one account's balance already rendered to a decimal STRING.
// Each engine adapter formats its native balance into this: SQLite's SUM is a
// float (rendered to 8 decimals), PostgreSQL's SUM is exact NUMERIC (passed
// through as text). The repo then normalizes the string via vo.NewDecimal.
type balanceResult struct {
	AccountID string
	Balance   string
}

// querier is the engine-agnostic surface for the field-identical model queries.
type querier interface {
	GetAccount(ctx context.Context, db backend.DBTX, id string) (accountRow, error)
	ListAvailableAccounts(ctx context.Context, db backend.DBTX, userID string) ([]accountRow, error)
	CountAvailableAccounts(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertAccount(ctx context.Context, db backend.DBTX, p upsertAccountP) error
	GetAccountOption(ctx context.Context, db backend.DBTX, p getOptionP) (optionRow, error)
	ListAccountOptionsByUser(ctx context.Context, db backend.DBTX, userID string) ([]optionRow, error)
	UpsertAccountOption(ctx context.Context, db backend.DBTX, p upsertOptionP) error
	InsertCorrectionTransaction(ctx context.Context, db backend.DBTX, p insertCorrectionP) error
}

// balanceQuerier is the per-engine balance surface. The two engines' generated
// param structs differ (see package doc), so the adapter builds them by hand
// from plain args.
type balanceQuerier interface {
	GetAccountBalance(ctx context.Context, db backend.DBTX, accountID string, before time.Time) (string, error)
	ListAccountBalancesForUser(ctx context.Context, db backend.DBTX, userID string, before time.Time) ([]balanceResult, error)
}

// Repo implements domain/account.Repository.
type Repo struct {
	tx *backend.TxManager
	q  querier
	b  balanceQuerier
}

var _ domaccount.Repository = (*Repo)(nil)

// NewRepo selects the engine adapters by driver name.
func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}, b: sqliteBalance{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}, b: pgsqlBalance{}}
	default:
		panic("accountrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// NextIdentity allocates a fresh id (used for accounts and corrections).
func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

// GetByID loads an account by id.
func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*domaccount.Account, error) {
	row, err := r.q.GetAccount(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Account not found")
		}
		return nil, err
	}
	return hydrateAccount(row)
}

// ListAvailable returns the user's non-deleted accounts.
func (r *Repo) ListAvailable(ctx context.Context, userID vo.Id) ([]*domaccount.Account, error) {
	rows, err := r.q.ListAvailableAccounts(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*domaccount.Account, 0, len(rows))
	for _, row := range rows {
		a, herr := hydrateAccount(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, a)
	}
	return out, nil
}

// CountAvailable returns the number of non-deleted accounts the user has.
func (r *Repo) CountAvailable(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountAvailableAccounts(ctx, r.db(ctx), userID.String())
	return int(n), err
}

// Save upserts an account row.
func (r *Repo) Save(ctx context.Context, a *domaccount.Account) error {
	return r.q.UpsertAccount(ctx, r.db(ctx), upsertAccountP{
		ID:         a.Id().String(),
		CurrencyID: a.CurrencyId().String(),
		UserID:     a.UserId().String(),
		Name:       a.Name(),
		Type:       a.Type().Int16(),
		Icon:       a.Icon(),
		IsDeleted:  a.IsDeleted(),
		CreatedAt:  a.CreatedAt(),
		UpdatedAt:  a.UpdatedAt(),
	})
}

// GetPosition returns the account's per-user position (accounts_options).
func (r *Repo) GetPosition(ctx context.Context, accountID, userID vo.Id) (int16, bool, error) {
	row, err := r.q.GetAccountOption(ctx, r.db(ctx), getOptionP{AccountID: accountID.String(), UserID: userID.String()})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return row.Position, true, nil
}

// MaxPosition returns the highest accounts_options.position for the user (0 if
// none).
func (r *Repo) MaxPosition(ctx context.Context, userID vo.Id) (int16, error) {
	rows, err := r.q.ListAccountOptionsByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	var max int16
	for _, row := range rows {
		if row.Position > max {
			max = row.Position
		}
	}
	return max, nil
}

// SavePosition upserts an accounts_options row.
func (r *Repo) SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error {
	return r.q.UpsertAccountOption(ctx, r.db(ctx), upsertOptionP{
		AccountID: accountID.String(),
		UserID:    userID.String(),
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// Balance returns one account's balance as of `before`, normalized.
func (r *Repo) Balance(ctx context.Context, accountID vo.Id, before time.Time) (string, error) {
	raw, err := r.b.GetAccountBalance(ctx, r.db(ctx), accountID.String(), before)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "0", nil
		}
		return "", err
	}
	return vo.NewDecimal(raw).String(), nil
}

// Balances returns balances for all the user's non-deleted accounts as of
// `before`, keyed by account id (normalized).
func (r *Repo) Balances(ctx context.Context, userID vo.Id, before time.Time) (map[string]string, error) {
	rows, err := r.b.ListAccountBalancesForUser(ctx, r.db(ctx), userID.String(), before)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.AccountID] = vo.NewDecimal(row.Balance).String()
	}
	return out, nil
}

// SaveCorrection inserts a balance-correction transaction.
func (r *Repo) SaveCorrection(ctx context.Context, c domaccount.Correction) error {
	return r.q.InsertCorrectionTransaction(ctx, r.db(ctx), insertCorrectionP{
		ID:          c.ID.String(),
		UserID:      c.UserID.String(),
		AccountID:   c.AccountID.String(),
		Description: c.Description,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.CreatedAt,
		SpentAt:     c.SpentAt,
		Type:        c.Type,
		Amount:      c.Amount,
	})
}

// hydrateAccount reconstitutes an Account from a row.
func hydrateAccount(row accountRow) (*domaccount.Account, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	currencyID, err := vo.ParseId(row.CurrencyID)
	if err != nil {
		return nil, err
	}
	return domaccount.FromState(
		id, userID, currencyID, row.Name, domaccount.Type(row.Type), row.Icon,
		row.IsDeleted, row.CreatedAt, row.UpdatedAt,
	), nil
}
