// Service wiring for the budget module: the use-case orchestrator, its many
// dependency seams (the budget Repository, the read-model reports, the currency
// convertor + average rates, the cross-module account/user/metadata lookups),
// the constructor, and the loaded-aggregate helper. Individual use cases live in
// sibling files; the heavy read lives in builder*.go.
package budget

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the budget module orchestrator. The one Repository constructor
// param splits into its role interfaces here so every use-case file
// references the narrowest surface it actually needs.
type Service struct {
	budgets     BudgetStore
	access      AccessStore
	folders     FolderStore
	envelopes   EnvelopeStore
	elements    ElementStore
	limits      LimitStore
	read        ReadModel
	convertor   Convertor
	rates       AverageRateLookup
	users       UserLookup
	accounts    AccountLookup
	currency    CurrencyLookup
	metadata    MetadataLookup
	connections Connections
	tx          port.TxRunner
	clock       port.Clock

	// accountOwners memoizes account -> owner id for ownsAccount (owner ids are
	// immutable). Reads never touch it: get-budget resolves owners from the
	// account views it already loaded, so this map stays off the read hot path.
	accountOwners map[string]string
	txLabels      TransactionLabels
}

// SetTransactionLabels wires the reporting-label lookup. It is a setter rather
// than a constructor parameter because the transaction repo that backs it is
// built after the budget service at composition time; called once, in BuildAPI.
func (s *Service) SetTransactionLabels(l TransactionLabels) { s.txLabels = l }

func NewService(
	repo Repository,
	read ReadModel,
	convertor Convertor,
	rates AverageRateLookup,
	users UserLookup,
	accounts AccountLookup,
	currency CurrencyLookup,
	metadata MetadataLookup,
	connections Connections,
	tx port.TxRunner,
	clock port.Clock,
) *Service {
	return &Service{
		budgets: repo, access: repo, folders: repo, envelopes: repo, elements: repo, limits: repo,
		read: read, convertor: convertor, rates: rates,
		users: users, accounts: accounts, currency: currency, metadata: metadata, connections: connections,
		tx: tx, clock: clock, accountOwners: map[string]string{},
	}
}

// budgetAggregate is the loaded budget with its related rows, assembled once and
// passed to the builders (avoids re-querying access/member accounts/folders/
// envelopes/elements repeatedly).
type budgetAggregate struct {
	budget    *model.Budget
	access    []*model.BudgetAccess
	accounts  []model.BudgetAccount
	folders   []*model.BudgetFolder
	envelopes []*model.BudgetEnvelope
	elements  []*model.BudgetElement
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
	accounts, err := s.budgets.MemberAccounts(ctx, budgetID)
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
	return &budgetAggregate{budget: b, access: access, accounts: accounts, folders: folders, envelopes: envelopes, elements: elements}, nil
}

// hasAccount reports whether accountID is already a member of this budget.
func (a *budgetAggregate) hasAccount(accountID vo.Id) bool {
	for _, m := range a.accounts {
		if m.AccountID.Equal(accountID) {
			return true
		}
	}
	return false
}

// hasFolder reports whether folderID is one of this budget's folders. Child
// mutators role-check the request's budget, then reach a folder by its own id;
// this guards against reaching a folder that belongs to a DIFFERENT budget
// (the id alone is not authorization).
func (a *budgetAggregate) hasFolder(folderID vo.Id) bool {
	for _, f := range a.folders {
		if f.ID.Equal(folderID) {
			return true
		}
	}
	return false
}

// hasEnvelope reports whether envelopeID is one of this budget's envelopes (see
// hasFolder for why the id alone is insufficient authorization).
func (a *budgetAggregate) hasEnvelope(envelopeID vo.Id) bool {
	for _, e := range a.envelopes {
		if e.ID.Equal(envelopeID) {
			return true
		}
	}
	return false
}

// requireFreeFolderID rejects a create whose client-supplied id already exists
// in ANY budget: the folder upsert is keyed on id, so reusing a foreign id
// would overwrite that row instead of creating a new one.
func (s *Service) requireFreeFolderID(ctx context.Context, id vo.Id) error {
	_, err := s.folders.GetFolder(ctx, id)
	if err == nil {
		return accessDenied()
	}
	var nf *errs.NotFoundError
	if errors.As(err, &nf) {
		return nil
	}
	return err
}

// requireFreeEnvelopeID is the envelope counterpart of requireFreeFolderID.
func (s *Service) requireFreeEnvelopeID(ctx context.Context, id vo.Id) error {
	_, err := s.envelopes.GetEnvelope(ctx, id)
	if err == nil {
		return accessDenied()
	}
	var nf *errs.NotFoundError
	if errors.As(err, &nf) {
		return nil
	}
	return err
}

// roleGuest returns the guest role (the read-only "reader" role).
func roleGuest() model.BudgetRole { return model.BudgetRoleGuest }

// localMonth returns the first of now's month as seen in loc (the caller's
// request timezone), expressed as a UTC-typed wall-clock. model.Budget timestamps are
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
