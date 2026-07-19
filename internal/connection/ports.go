// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package connection

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccessRevoker unwinds account sharing between two users on
// delete-connection. Backed by the account feature via a server adapter.
// Runs on the caller's transaction context. May be nil in stripped-down test
// harnesses (delete-connection then skips the account unwind).
type AccountAccessRevoker interface {
	RevokeAccessBetween(ctx context.Context, a, b vo.Id) error
}

// BudgetAccessRevoker is the slice of budget-sharing behavior delete-connection
// needs: drop any budget access SHARED BETWEEN the two users (both directions).
// Backed by the budget module via an adapter in main. A nil revoker is tolerated
// (delete-connection then only unwinds account access + the connection link) so
// test harnesses without the budget module still work.
type BudgetAccessRevoker interface {
	// RevokeBetween removes any budget-access grants where one user owns a budget
	// the other can access, in BOTH directions, for the given user pair.
	RevokeBetween(ctx context.Context, a, b vo.Id) error
}

// UserLookup resolves the connected-user embed (id, name, avatar).
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (model.OwnerView, error)
}

// AttemptLimiter is the brute-force-protection seam for accept-invite: the
// invite code is short, so an authenticated caller must not be able to spray
// guesses. Allow reports whether another attempt may proceed (an
// *errs.TooManyRequestsError, HTTP 429, when over the cap); Fail records an
// attempt. A nil limiter disables protection (CLI, tests).
type AttemptLimiter interface {
	Allow(scope, key string) error
	Fail(scope, key string)
}

// RateScopeAcceptInvite keys the accept-invite limiter config in internal/server.
const RateScopeAcceptInvite = "accept-invite"
