// Ports: the consumer-side interfaces the onboarding feature declares for
// capabilities other features and infrastructure provide. Implementations are
// wired in internal/server — features never import each other.
package onboarding

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BudgetWriter is the budget feature's write path, run as the calling user with
// the budget feature's own validation, idempotency and access checks.
type BudgetWriter interface {
	CreateBudget(ctx context.Context, userID vo.Id, req model.CreateBudgetRequest) (*model.CreateBudgetResult, error)
	AddAccount(ctx context.Context, userID vo.Id, req model.AddAccountRequest) (*model.AddAccountResult, error)
	// MemberAccountIDs lists the budget's members for a caller who may update it.
	MemberAccountIDs(ctx context.Context, userID, budgetID vo.Id) ([]vo.Id, error)
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
