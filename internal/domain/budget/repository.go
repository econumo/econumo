package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Repository is the budget aggregate's persistence port. It spans the eight
// budget tables; the app services depend only on this interface. Missing rows
// return an *errs.NotFoundError.
type Repository interface {
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*Budget, error)
	// ListForUser returns budgets the user owns or has (any) access to.
	ListForUser(ctx context.Context, userID vo.Id) ([]*Budget, error)
	Save(ctx context.Context, b *Budget) error
	Delete(ctx context.Context, id vo.Id) error

	ExcludedAccountIDs(ctx context.Context, budgetID vo.Id) ([]vo.Id, error)
	ExcludeAccount(ctx context.Context, budgetID, accountID vo.Id) error
	IncludeAccount(ctx context.Context, budgetID, accountID vo.Id) error

	ListAccess(ctx context.Context, budgetID vo.Id) ([]*BudgetAccess, error)
	GetAccess(ctx context.Context, budgetID, userID vo.Id) (*BudgetAccess, error)
	SaveAccess(ctx context.Context, a *BudgetAccess) error
	DeleteAccess(ctx context.Context, budgetID, userID vo.Id) error

	ListFolders(ctx context.Context, budgetID vo.Id) ([]*BudgetFolder, error)
	GetFolder(ctx context.Context, id vo.Id) (*BudgetFolder, error)
	SaveFolder(ctx context.Context, f *BudgetFolder) error
	DeleteFolder(ctx context.Context, id vo.Id) error

	ListEnvelopes(ctx context.Context, budgetID vo.Id) ([]*BudgetEnvelope, error)
	GetEnvelope(ctx context.Context, id vo.Id) (*BudgetEnvelope, error)
	SaveEnvelope(ctx context.Context, e *BudgetEnvelope) error
	DeleteEnvelope(ctx context.Context, id vo.Id) error
	// EnvelopeCategoryIDs returns the category ids assigned to an envelope.
	EnvelopeCategoryIDs(ctx context.Context, envelopeID vo.Id) ([]vo.Id, error)
	AddEnvelopeCategory(ctx context.Context, envelopeID, categoryID vo.Id) error
	RemoveEnvelopeCategory(ctx context.Context, envelopeID, categoryID vo.Id) error

	ListElements(ctx context.Context, budgetID vo.Id) ([]*BudgetElement, error)
	GetElement(ctx context.Context, id vo.Id) (*BudgetElement, error)
	// GetElementByExternal finds an element by its (budget, externalId) pair.
	GetElementByExternal(ctx context.Context, budgetID, externalID vo.Id) (*BudgetElement, error)
	SaveElement(ctx context.Context, e *BudgetElement) error
	DeleteElement(ctx context.Context, id vo.Id) error

	// ListLimitsForPeriod returns the limits for a budget's elements in a period.
	ListLimitsForPeriod(ctx context.Context, budgetID vo.Id, period time.Time) ([]*BudgetElementLimit, error)
	GetLimit(ctx context.Context, elementID vo.Id, period time.Time) (*BudgetElementLimit, error)
	SaveLimit(ctx context.Context, l *BudgetElementLimit) error
	DeleteLimit(ctx context.Context, id vo.Id) error
	// DeleteLimitsByBudget removes every limit of every element of a budget (reset).
	DeleteLimitsByBudget(ctx context.Context, budgetID vo.Id) error
}
