package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// BudgetStore is the Budget aggregate root's own persistence surface:
// identity, lookup/listing, the write/delete, and the excluded-accounts join
// table. Consumed by loadAggregate (usecase.go), CreateBudget (create.go),
// UpdateBudget/DeleteBudget/ResetBudget (crud.go), toggleAccount
// (accounts.go), and GetBudgetList (read.go). A missing budget returns an
// *errs.NotFoundError.
type BudgetStore interface {
	GetByID(ctx context.Context, id vo.Id) (*Budget, error)
	// ListForUser returns budgets the user owns or has (any) access to.
	ListForUser(ctx context.Context, userID vo.Id) ([]*Budget, error)
	Save(ctx context.Context, b *Budget) error
	Delete(ctx context.Context, id vo.Id) error

	ExcludedAccountIDs(ctx context.Context, budgetID vo.Id) ([]vo.Id, error)
	ExcludeAccount(ctx context.Context, budgetID, accountID vo.Id) error
	IncludeAccount(ctx context.Context, budgetID, accountID vo.Id) error
}

// AccessStore is a budget's participant-grant persistence surface. Consumed by
// loadAggregate (usecase.go) and GrantAccess/AcceptAccess/RevokeAccess/
// DeclineAccess (accesssvc.go). NextIdentity allocates a fresh grant id (a
// grant's id is otherwise unused on the wire — see repo/repo.go's hydrateAccess).
type AccessStore interface {
	NextIdentity() vo.Id

	ListAccess(ctx context.Context, budgetID vo.Id) ([]*BudgetAccess, error)
	GetAccess(ctx context.Context, budgetID, userID vo.Id) (*BudgetAccess, error)
	SaveAccess(ctx context.Context, a *BudgetAccess) error
	DeleteAccess(ctx context.Context, budgetID, userID vo.Id) error
}

// FolderStore is the budget-folder persistence surface. Consumed by
// loadAggregate (usecase.go) and CreateFolder/UpdateFolder/DeleteFolder/
// OrderFolderList (folders.go). Folder ids are always client-supplied, so
// there is no NextIdentity here.
type FolderStore interface {
	ListFolders(ctx context.Context, budgetID vo.Id) ([]*BudgetFolder, error)
	GetFolder(ctx context.Context, id vo.Id) (*BudgetFolder, error)
	SaveFolder(ctx context.Context, f *BudgetFolder) error
	DeleteFolder(ctx context.Context, id vo.Id) error
}

// EnvelopeStore is the budget-envelope persistence surface, including its
// category-membership join table. Consumed by loadAggregate (usecase.go),
// CreateEnvelope/UpdateEnvelope/DeleteEnvelope (envelopes.go), and
// restoreElementsOrder (move.go). Envelope ids are always client-supplied, so
// there is no NextIdentity here.
type EnvelopeStore interface {
	ListEnvelopes(ctx context.Context, budgetID vo.Id) ([]*BudgetEnvelope, error)
	GetEnvelope(ctx context.Context, id vo.Id) (*BudgetEnvelope, error)
	SaveEnvelope(ctx context.Context, e *BudgetEnvelope) error
	DeleteEnvelope(ctx context.Context, id vo.Id) error
	// EnvelopeCategoryIDs returns the category ids assigned to an envelope.
	EnvelopeCategoryIDs(ctx context.Context, envelopeID vo.Id) ([]vo.Id, error)
	AddEnvelopeCategory(ctx context.Context, envelopeID, categoryID vo.Id) error
	RemoveEnvelopeCategory(ctx context.Context, envelopeID, categoryID vo.Id) error
}

// ElementStore is a budget element's persistence surface (an envelope,
// category, or tag's presence/position/currency/folder within a budget).
// Consumed by loadAggregate (usecase.go), seedCategoryElements/
// seedTagElements (create.go), CreateEnvelope/UpdateEnvelope/DeleteEnvelope
// (envelopes.go), ChangeElementCurrency/SetLimit (accounts.go), and
// MoveElementList/shiftElements/restoreElementsOrder (move.go). NextIdentity
// allocates a fresh element id.
type ElementStore interface {
	NextIdentity() vo.Id

	ListElements(ctx context.Context, budgetID vo.Id) ([]*BudgetElement, error)
	GetElement(ctx context.Context, id vo.Id) (*BudgetElement, error)
	// GetElementByExternal finds an element by its (budget, externalId) pair.
	GetElementByExternal(ctx context.Context, budgetID, externalID vo.Id) (*BudgetElement, error)
	SaveElement(ctx context.Context, e *BudgetElement) error
	DeleteElement(ctx context.Context, id vo.Id) error
}

// LimitStore is a budget element's per-period spending-limit persistence
// surface. Consumed by ChangeElementCurrency/SetLimit (accounts.go), and
// ResetBudget's clear-all (crud.go). NextIdentity allocates a fresh limit id.
type LimitStore interface {
	NextIdentity() vo.Id

	// ListLimitsForPeriod returns the limits for a budget's elements in a period.
	ListLimitsForPeriod(ctx context.Context, budgetID vo.Id, period time.Time) ([]*BudgetElementLimit, error)
	GetLimit(ctx context.Context, elementID vo.Id, period time.Time) (*BudgetElementLimit, error)
	SaveLimit(ctx context.Context, l *BudgetElementLimit) error
	DeleteLimit(ctx context.Context, id vo.Id) error
	// DeleteLimitsByBudget removes every limit of every element of a budget (reset).
	DeleteLimitsByBudget(ctx context.Context, budgetID vo.Id) error
}

// Repository is the budget aggregate's full persistence port — the composite
// of BudgetStore, AccessStore, FolderStore, EnvelopeStore, ElementStore and
// LimitStore. It exists for wiring (one constructor param in server.go);
// consumers depend on the narrowest role they actually use.
type Repository interface {
	BudgetStore
	AccessStore
	FolderStore
	EnvelopeStore
	ElementStore
	LimitStore
}
