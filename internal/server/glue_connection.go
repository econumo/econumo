// Connection glue: every adapter satisfying a port that the connection
// feature declares (see internal/connection/ports.go). Features must not
// import each other (archtest); the composition root bridges them here.
package server

import (
	"context"
	"slices"
	"strings"

	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// connectionAccountRevoker is the slice of the account service that
// delete-connection's unwind needs.
type connectionAccountRevoker interface {
	RevokeAccessBetween(ctx context.Context, a, b vo.Id) error
}

// ConnectionAccountAccessRevoker adapts the account service to
// connection.AccountAccessRevoker.
type ConnectionAccountAccessRevoker struct{ accounts connectionAccountRevoker }

var _ appconnection.AccountAccessRevoker = (*ConnectionAccountAccessRevoker)(nil)

// NewConnectionAccountAccessRevoker wraps the account service.
func NewConnectionAccountAccessRevoker(accounts connectionAccountRevoker) *ConnectionAccountAccessRevoker {
	return &ConnectionAccountAccessRevoker{accounts: accounts}
}

// RevokeAccessBetween unwinds account sharing between the two users, both
// directions, via the account feature.
func (r *ConnectionAccountAccessRevoker) RevokeAccessBetween(ctx context.Context, a, b vo.Id) error {
	return r.accounts.RevokeAccessBetween(ctx, a, b)
}

// connectionBudgetRepoPort is the slice of the budget repository the revoker
// needs. The budget *Repo satisfies it; declaring it here (consumer side)
// avoids importing the whole budget repo type and keeps the dependency
// one-directional.
type connectionBudgetRepoPort interface {
	ListForUser(ctx context.Context, userID vo.Id) ([]*model.Budget, error)
	ListAccess(ctx context.Context, budgetID vo.Id) ([]*model.BudgetAccess, error)
}

// budgetMemberRemover is the budget-service slice the revoker needs: drop a
// member's access AND their seeded records (elements, limits, envelope links).
type budgetMemberRemover interface {
	RemoveMember(ctx context.Context, budgetID, memberID vo.Id) error
}

// ConnectionBudgetRevoker drops budget-sharing between two users in both
// directions. It implements connection.BudgetAccessRevoker, used by
// delete-connection to tear down the budget-sharing side effects of a
// connection.
type ConnectionBudgetRevoker struct {
	budgets connectionBudgetRepoPort
	remover budgetMemberRemover
}

var _ appconnection.BudgetAccessRevoker = (*ConnectionBudgetRevoker)(nil)

// NewConnectionBudgetRevoker wires the revoker over the budget repository
// (listing) and the budget service (member removal).
func NewConnectionBudgetRevoker(budgets connectionBudgetRepoPort, remover budgetMemberRemover) *ConnectionBudgetRevoker {
	return &ConnectionBudgetRevoker{budgets: budgets, remover: remover}
}

// RevokeBetween removes any budget-access grants shared between users a and b:
// B from every budget A owns, and A from every budget B owns. Budgets owned by
// anyone else are left alone even when both are members. The removals of both
// directions run in one pass ordered by budget id, the order in which budget
// locks are taken everywhere.
func (r *ConnectionBudgetRevoker) RevokeBetween(ctx context.Context, a, b vo.Id) error {
	removals, err := r.collectRemovals(ctx, a, b)
	if err != nil {
		return err
	}
	more, err := r.collectRemovals(ctx, b, a)
	if err != nil {
		return err
	}
	removals = append(removals, more...)
	slices.SortFunc(removals, func(x, y budgetRemoval) int {
		return strings.Compare(x.budgetID.String(), y.budgetID.String())
	})
	for _, rm := range removals {
		if err := r.remover.RemoveMember(ctx, rm.budgetID, rm.memberID); err != nil {
			return err
		}
	}
	return nil
}

type budgetRemoval struct {
	budgetID vo.Id
	memberID vo.Id
}

// collectRemovals lists the budgets `owner` owns where `other` holds access.
// ListForUser also returns budgets shared with `owner`; those belong to a
// third user and are skipped.
func (r *ConnectionBudgetRevoker) collectRemovals(ctx context.Context, owner, other vo.Id) ([]budgetRemoval, error) {
	budgets, err := r.budgets.ListForUser(ctx, owner)
	if err != nil {
		return nil, err
	}
	var removals []budgetRemoval
	for _, bud := range budgets {
		if !bud.UserID.Equal(owner) {
			continue
		}
		access, err := r.budgets.ListAccess(ctx, bud.ID)
		if err != nil {
			return nil, err
		}
		for _, acc := range access {
			if acc.UserID.Equal(other) {
				removals = append(removals, budgetRemoval{budgetID: bud.ID, memberID: other})
				break
			}
		}
	}
	return removals, nil
}
