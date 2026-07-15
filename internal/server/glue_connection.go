// Connection glue: every adapter satisfying a port that the connection
// feature declares (see internal/connection/ports.go). Features must not
// import each other (archtest); the composition root bridges them here.
package server

import (
	"context"
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
	DeleteAccess(ctx context.Context, budgetID, userID vo.Id) error
}

// ConnectionBudgetRevoker drops budget-sharing between two users in both
// directions. It implements connection.BudgetAccessRevoker, used by
// delete-connection to tear down the budget-sharing side effects of a
// connection.
type ConnectionBudgetRevoker struct {
	budgets connectionBudgetRepoPort
}

var _ appconnection.BudgetAccessRevoker = (*ConnectionBudgetRevoker)(nil)

// NewConnectionBudgetRevoker wires the revoker over the budget repository.
func NewConnectionBudgetRevoker(budgets connectionBudgetRepoPort) *ConnectionBudgetRevoker {
	return &ConnectionBudgetRevoker{budgets: budgets}
}

// RevokeBetween removes any budget-access grants shared between users a and b:
// for each budget A owns where B has access, revoke B; and symmetrically for
// budgets B owns where A has access.
func (r *ConnectionBudgetRevoker) RevokeBetween(ctx context.Context, a, b vo.Id) error {
	if err := r.revokeDirection(ctx, a, b); err != nil {
		return err
	}
	return r.revokeDirection(ctx, b, a)
}

// revokeDirection: for each budget in `owner`'s list, if `other` holds access,
// delete it. ListForUser returns owned + accessible budgets; the DeleteAccess is
// a no-op when `other` has no row, so this is safe over the full list.
func (r *ConnectionBudgetRevoker) revokeDirection(ctx context.Context, owner, other vo.Id) error {
	budgets, err := r.budgets.ListForUser(ctx, owner)
	if err != nil {
		return err
	}
	for _, bud := range budgets {
		access, aerr := r.budgets.ListAccess(ctx, bud.ID)
		if aerr != nil {
			return aerr
		}
		for _, acc := range access {
			if acc.UserID.Equal(other) {
				if derr := r.budgets.DeleteAccess(ctx, bud.ID, other); derr != nil {
					return derr
				}
				break
			}
		}
	}
	return nil
}
