// Service wiring for the budget module: the use-case orchestrator, its many
// dependency seams (the budget Repository, the read-model reports, the currency
// convertor + average rates, the cross-module account/user/metadata lookups),
// the constructor, and the loaded-aggregate helper. Individual use cases live in
// sibling files; the heavy read lives in builder*.go.
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// OwnerView is the minimal user shape embedded in a budget access entry.
type OwnerView struct {
	ID     string
	Name   string
	Avatar string
}

// AccountView is an account as the filters builder needs it: id + currency + owner.
type AccountView struct {
	ID         string
	CurrencyID string
	OwnerID    string
}

// Service is the budget module orchestrator. The one Repository constructor
// param splits into its role interfaces here so every use-case file
// references the narrowest surface it actually needs.
type Service struct {
	budgets   BudgetStore
	access    AccessStore
	folders   FolderStore
	envelopes EnvelopeStore
	elements  ElementStore
	limits    LimitStore
	read      ReadModel
	convertor Convertor
	rates     AverageRateLookup
	users     UserLookup
	accounts  AccountLookup
	currency  CurrencyLookup
	metadata  MetadataLookup
	tx        port.TxRunner
	clock     port.Clock

	// accountOwners is a per-call cache (set fresh per Service is fine; the
	// Service is constructed once, so guard via a small map populated lazily and
	// only read within a single request — acceptable for owner ids which are
	// immutable).
	accountOwners map[string]string
}

func NewService(
	repo Repository,
	read ReadModel,
	convertor Convertor,
	rates AverageRateLookup,
	users UserLookup,
	accounts AccountLookup,
	currency CurrencyLookup,
	metadata MetadataLookup,
	tx port.TxRunner,
	clock port.Clock,
) *Service {
	return &Service{
		budgets: repo, access: repo, folders: repo, envelopes: repo, elements: repo, limits: repo,
		read: read, convertor: convertor, rates: rates,
		users: users, accounts: accounts, currency: currency, metadata: metadata,
		tx: tx, clock: clock, accountOwners: map[string]string{},
	}
}

// budgetAggregate is the loaded budget with its related rows, assembled once and
// passed to the builders (avoids re-querying access/excluded/folders/envelopes/
// elements repeatedly).
type budgetAggregate struct {
	budget             *Budget
	access             []*BudgetAccess
	excludedAccountIDs []vo.Id
	folders            []*BudgetFolder
	envelopes          []*BudgetEnvelope
	elements           []*BudgetElement
}

func (s *Service) loadAggregate(ctx context.Context, budgetID vo.Id) (*budgetAggregate, error) {
	b, err := s.budgets.GetByID(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	access, err := s.access.ListAccess(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	excluded, err := s.budgets.ExcludedAccountIDs(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	folders, err := s.folders.ListFolders(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	envelopes, err := s.envelopes.ListEnvelopes(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	elements, err := s.elements.ListElements(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	return &budgetAggregate{budget: b, access: access, excludedAccountIDs: excluded, folders: folders, envelopes: envelopes, elements: elements}, nil
}

// roleGuest returns the guest role (the read-only "reader" role).
func roleGuest() UserRole { return RoleGuest }

// localMonth returns the first of now's month as seen in loc (the caller's
// request timezone), expressed as a UTC-typed wall-clock. Budget timestamps are
// stored as naive "Y-m-d H:i:s", so the value must be UTC-typed to serialize as
// that bare wall-clock. Snapping in UTC instead would start a budget in the
// NEXT month for a behind-UTC caller creating it on the evening of the 30th/31st
// (their local month hasn't rolled over yet, the server's UTC month has).
func localMonth(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, _ := now.In(loc).Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}
