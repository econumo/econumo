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

// DebtSteps is the debt feature as onboarding's debt detection uses it; the
// debts belong to a budget, so every call names the one the step works in.
type DebtSteps interface {
	// CreateDebt attaches a debt not set up (no amount borrowed, start or
	// end) to the category's element in the budget. created is false, with
	// no error, when the debt feature refuses it: the category already
	// carries an active debt, it cannot carry one (an income category, one
	// in an envelope), or the caller cannot plan the budget.
	CreateDebt(ctx context.Context, userID, budgetID, categoryID vo.Id, payment vo.DecimalNumber, schedule model.RecurringSchedule) (created bool, err error)
}

// AttemptLimiter caps the AI classification per user; every job counts.
type AttemptLimiter interface {
	Allow(ctx context.Context, scope, key string) error
	Reserve(scope, key string) error
}

// Steps are the dependencies of the step writes (finish-import,
// save-categories, save-accounts, save-plan), the plan suggestions and the
// background import review. Every write goes through the owning feature's own
// use case; a step's writes run inside the step's one transaction.
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
	// OnboardingUser reads the current user without the lock.
	OnboardingUser(ctx context.Context, userID vo.Id) (*model.CurrentUserResult, error)
	UpdateCurrency(ctx context.Context, userID vo.Id, req model.UpdateCurrencyRequest) (*model.UpdateCurrencyResult, error)
}

// CategorySteps is the category feature's create, sort, archive and icon
// change, plus the caller's own categories (archived included) in display order.
type CategorySteps interface {
	CreateCategory(ctx context.Context, userID vo.Id, req model.CreateCategoryRequest) (*model.CreateCategoryResult, error)
	SortCategoryList(ctx context.Context, userID vo.Id, req model.SortCategoryListRequest) (*model.SortCategoryListResult, error)
	ArchiveCategory(ctx context.Context, userID vo.Id, req model.ArchiveCategoryRequest) (*model.ArchiveCategoryResult, error)
	UnarchiveCategory(ctx context.Context, userID vo.Id, req model.UnarchiveCategoryRequest) (*model.UnarchiveCategoryResult, error)
	// ReplaceIcon changes the icon only while the category still shows from.
	ReplaceIcon(ctx context.Context, userID, id vo.Id, from, to string) (bool, error)
	OwnCategories(ctx context.Context, userID vo.Id) ([]model.CategoryResult, error)
}

// AccountSteps is the account feature's create, folder, move and icon use
// cases, plus the caller's own live accounts (with their icon, currency code
// and whether a hidden folder holds them).
type AccountSteps interface {
	CreateAccount(ctx context.Context, userID vo.Id, req model.CreateAccountRequest) (*model.CreateAccountResult, error)
	OwnLiveAccounts(ctx context.Context, userID vo.Id) ([]model.OnboardingAccount, error)
	GetFolderList(ctx context.Context, userID vo.Id) (*model.GetFolderListResult, error)
	CreateFolder(ctx context.Context, userID vo.Id, req model.CreateFolderRequest) (*model.CreateFolderResult, error)
	HideFolder(ctx context.Context, userID vo.Id, req model.HideFolderRequest) (*model.HideFolderResult, error)
	ShowFolder(ctx context.Context, userID vo.Id, req model.ShowFolderRequest) (*model.ShowFolderResult, error)
	MoveAccount(ctx context.Context, userID vo.Id, req model.MoveAccountRequest) (*model.MoveAccountResult, error)
	// ReplaceIcon changes the icon only while the account still shows from.
	ReplaceIcon(ctx context.Context, userID, id vo.Id, from, to string) (bool, error)
	// ReviseAccounts changes accounts' currency and balance through
	// update-account, resending each account's own name and icon; an unchanged
	// balance writes no correction.
	ReviseAccounts(ctx context.Context, userID vo.Id, changes []model.OnboardingAccountRevision) error
}

// CurrencySteps is the currency feature: the code of a currency the caller may
// use (a global or their own live custom; a NotFound or validation error
// otherwise) and the caller's visible set.
type CurrencySteps interface {
	UsableCurrencyCode(ctx context.Context, userID vo.Id, currencyID string) (string, error)
	HideAllCurrencies(ctx context.Context, userID vo.Id) (*model.HideAllCurrenciesResult, error)
	ShowCurrency(ctx context.Context, userID vo.Id, req model.ShowCurrencyRequest) (*model.ShowCurrencyResult, error)
	FractionDigits(ctx context.Context, currencyID string) (int, error)
}

// BudgetSteps is the budget feature's create, update, add-account,
// remove-account and set-limit use cases, plus the reads that pick the budget, its current period
// and the categories' recent history.
type BudgetSteps interface {
	CreateBudget(ctx context.Context, userID vo.Id, req model.CreateBudgetRequest) (*model.CreateBudgetResult, error)
	UpdateBudget(ctx context.Context, userID vo.Id, req model.UpdateBudgetRequest) (*model.UpdateBudgetResult, error)
	// UnplannedOwnBudget reports the budget's name and currency, with ok set
	// only when the caller owns it alone (no other participant, invited or
	// accepted) and it has no limit in any period.
	UnplannedOwnBudget(ctx context.Context, userID, budgetID vo.Id) (name, currencyID string, ok bool, err error)
	AddAccount(ctx context.Context, userID vo.Id, req model.AddAccountRequest) (*model.AddAccountResult, error)
	RemoveAccount(ctx context.Context, userID vo.Id, req model.RemoveAccountRequest) (*model.RemoveAccountResult, error)
	CategoryHistory(ctx context.Context, userID, budgetID vo.Id, months int) ([]model.BudgetCategoryHistory, error)
	SetLimit(ctx context.Context, userID vo.Id, req model.SetLimitRequest) (*model.SetLimitResult, error)
	DefaultBudget(ctx context.Context, userID vo.Id, preferred string) (id vo.Id, writable, found bool, err error)
	CurrentPeriod(ctx context.Context, userID, budgetID vo.Id) (time.Time, error)
}
