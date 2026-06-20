// Service wiring for the budget module: the use-case orchestrator, its many
// dependency seams (the budget Repository, the read-model reports, the currency
// convertor + average rates, the cross-module account/user/metadata lookups),
// the constructor, and the loaded-aggregate helper. Individual use cases live in
// sibling files; the heavy read lives in builder*.go.
package budget

import (
	"context"
	"time"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Clock supplies the current time.
type Clock interface{ Now() time.Time }

// TxRunner is the transaction boundary the service owns.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// OwnerView is the minimal user shape embedded in a budget access entry.
type OwnerView struct {
	ID     string
	Name   string
	Avatar string
}

// UserLookup resolves a budget participant's id/name/avatar + their currency code.
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (OwnerView, error)
	// CurrencyCode returns the user's default currency code (for createBudget when
	// no currencyId is supplied).
	CurrencyCode(ctx context.Context, userID string) (string, error)
	// SetActiveBudget records the user's active budget id (PHP userService.updateBudget).
	SetActiveBudget(ctx context.Context, userID, budgetID vo.Id) error
}

// AccountView is an account as the filters builder needs it: id + currency + owner.
type AccountView struct {
	ID         string
	CurrencyID string
	OwnerID    string
}

// AccountLookup resolves accounts owned by the budget participants + ownership.
type AccountLookup interface {
	AccountsForOwners(ctx context.Context, userIDs []vo.Id) ([]AccountView, error)
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
}

// CurrencyLookup resolves a currency id by code (createBudget default currency).
type CurrencyLookup interface {
	GetIDByCode(ctx context.Context, code string) (string, error)
}

// Service is the budget module orchestrator.
type Service struct {
	repo      dombudget.Repository
	read      ReadModel
	convertor Convertor
	rates     AverageRateLookup
	users     UserLookup
	accounts  AccountLookup
	currency  CurrencyLookup
	metadata  MetadataLookup
	tx        TxRunner
	clock     Clock

	// accountOwners is a per-call cache (set fresh per Service is fine; the
	// Service is constructed once, so guard via a small map populated lazily and
	// only read within a single request — acceptable for owner ids which are
	// immutable).
	accountOwners map[string]string
}

// NewService wires the budget service.
func NewService(
	repo dombudget.Repository,
	read ReadModel,
	convertor Convertor,
	rates AverageRateLookup,
	users UserLookup,
	accounts AccountLookup,
	currency CurrencyLookup,
	metadata MetadataLookup,
	tx TxRunner,
	clock Clock,
) *Service {
	return &Service{
		repo: repo, read: read, convertor: convertor, rates: rates,
		users: users, accounts: accounts, currency: currency, metadata: metadata,
		tx: tx, clock: clock, accountOwners: map[string]string{},
	}
}

// budgetAggregate is the loaded budget with its related rows, assembled once and
// passed to the builders (avoids re-querying access/excluded/folders/envelopes/
// elements repeatedly).
type budgetAggregate struct {
	budget             *dombudget.Budget
	access             []*dombudget.BudgetAccess
	excludedAccountIDs []vo.Id
	folders            []*dombudget.BudgetFolder
	envelopes          []*dombudget.BudgetEnvelope
	elements           []*dombudget.BudgetElement
}

// loadAggregate loads a budget and its related rows.
func (s *Service) loadAggregate(ctx context.Context, budgetID vo.Id) (*budgetAggregate, error) {
	b, err := s.repo.GetByID(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	access, err := s.repo.ListAccess(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	excluded, err := s.repo.ExcludedAccountIDs(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	folders, err := s.repo.ListFolders(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	envelopes, err := s.repo.ListEnvelopes(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	elements, err := s.repo.ListElements(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	return &budgetAggregate{budget: b, access: access, excludedAccountIDs: excluded, folders: folders, envelopes: envelopes, elements: elements}, nil
}

// roleGuest returns the guest role (a "reader" in PHP terms).
func roleGuest() dombudget.UserRole { return dombudget.RoleGuest }

// firstOfMonth returns the first of t's month at 00:00 in t's location.
func firstOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}
