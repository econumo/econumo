// Ports: the consumer-side interfaces the onboarding feature declares for
// capabilities other features and infrastructure provide. Implementations are
// wired in internal/server — features never import each other.
package onboarding

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BudgetWriter is the budget write the background savings classification makes.
type BudgetWriter interface {
	// MarkSavingsMembers flags accounts that are still live members as savings
	// and skips the rest.
	MarkSavingsMembers(ctx context.Context, userID, budgetID vo.Id, accountIDs []vo.Id) error
}

// AccountReader loads accounts by id; ids that do not exist are left out.
type AccountReader interface {
	AccountsByIDs(ctx context.Context, ids []vo.Id) ([]model.OnboardingAccount, error)
}

// TimezoneReader returns the user's stored IANA timezone ("" when never seen).
type TimezoneReader interface {
	StoredTimezone(ctx context.Context, userID vo.Id) (string, error)
}

// Completer is one chat completion with the configured AI provider. userID
// names the requesting user (the econumo gateway scheme signs a token for it).
type Completer interface {
	Complete(ctx context.Context, userID, system, user string) (string, error)
}

// AttemptLimiter caps the AI classification per user; every job counts.
type AttemptLimiter interface {
	Allow(ctx context.Context, scope, key string) error
	Reserve(scope, key string) error
}

// Steps are the dependencies of the three step writes (save-categories,
// save-accounts, save-plan). Every write a step makes goes through the owning
// feature's own use case, inside the step's one transaction.
type Steps struct {
	Tx         port.TxRunner
	Ops        port.OperationGuard
	Clock      port.Clock
	Users      UserSteps
	Categories CategorySteps
	Accounts   AccountSteps
	Currencies CurrencySteps
	Budgets    BudgetSteps
}

// UserSteps is the user feature: the row lock, the step state and the profile
// currency.
type UserSteps interface {
	// LockForOnboarding takes the user row lock in the caller's transaction.
	LockForOnboarding(ctx context.Context, userID vo.Id) (*model.CurrentUserResult, error)
	MarkOnboardingStep(ctx context.Context, userID vo.Id, step string) (*model.CurrentUserResult, error)
	UpdateCurrency(ctx context.Context, userID vo.Id, req model.UpdateCurrencyRequest) (*model.UpdateCurrencyResult, error)
}

// CategorySteps is the category feature's create and sort, plus the caller's
// own categories (archived included) in display order.
type CategorySteps interface {
	CreateCategory(ctx context.Context, userID vo.Id, req model.CreateCategoryRequest) (*model.CreateCategoryResult, error)
	SortCategoryList(ctx context.Context, userID vo.Id, req model.SortCategoryListRequest) (*model.SortCategoryListResult, error)
	OwnCategories(ctx context.Context, userID vo.Id) ([]model.CategoryResult, error)
}

// AccountSteps is the account feature's create, the caller's own live
// accounts, and the id of their first folder ("" when they have none).
type AccountSteps interface {
	CreateAccount(ctx context.Context, userID vo.Id, req model.CreateAccountRequest) (*model.CreateAccountResult, error)
	OwnLiveAccounts(ctx context.Context, userID vo.Id) ([]model.OnboardingAccount, error)
	FirstFolderID(ctx context.Context, userID vo.Id) (string, error)
}

// CurrencySteps is the currency feature: a currency's code (a NotFound error
// when it does not exist) and the caller's visible set.
type CurrencySteps interface {
	CurrencyCode(ctx context.Context, currencyID string) (string, error)
	HideAllCurrencies(ctx context.Context, userID vo.Id) (*model.HideAllCurrenciesResult, error)
	ShowCurrency(ctx context.Context, userID vo.Id, req model.ShowCurrencyRequest) (*model.ShowCurrencyResult, error)
}

// BudgetSteps is the budget feature's create, add-account and set-limit use
// cases, plus the reads that pick the budget and its current period.
type BudgetSteps interface {
	CreateBudget(ctx context.Context, userID vo.Id, req model.CreateBudgetRequest) (*model.CreateBudgetResult, error)
	AddAccount(ctx context.Context, userID vo.Id, req model.AddAccountRequest) (*model.AddAccountResult, error)
	SetLimit(ctx context.Context, userID vo.Id, req model.SetLimitRequest) (*model.SetLimitResult, error)
	DefaultBudget(ctx context.Context, userID vo.Id, preferred string) (id vo.Id, writable, found bool, err error)
	CurrentPeriod(ctx context.Context, userID, budgetID vo.Id) (time.Time, error)
}
