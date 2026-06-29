package connectionrepo

import (
	"context"

	appconnection "github.com/econumo/econumo/internal/app/connection"
	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// budgetRepoPort is the slice of the budget repository the revoker needs. The
// budget *Repo satisfies it; declaring it here (consumer side) avoids importing
// the whole budget repo type and keeps the dependency one-directional.
type budgetRepoPort interface {
	ListForUser(ctx context.Context, userID vo.Id) ([]*dombudget.Budget, error)
	ListAccess(ctx context.Context, budgetID vo.Id) ([]*dombudget.BudgetAccess, error)
	DeleteAccess(ctx context.Context, budgetID, userID vo.Id) error
}

// BudgetAccessRevoker drops budget-sharing between two users in both directions.
// It implements app/connection.BudgetAccessRevoker, used by delete-connection to
// tear down the budget-sharing side effects of a connection.
type BudgetAccessRevoker struct {
	budgets budgetRepoPort
}

var _ appconnection.BudgetAccessRevoker = (*BudgetAccessRevoker)(nil)

// NewBudgetAccessRevoker wires the revoker over the budget repository.
func NewBudgetAccessRevoker(budgets budgetRepoPort) *BudgetAccessRevoker {
	return &BudgetAccessRevoker{budgets: budgets}
}

// RevokeBetween removes any budget-access grants shared between users a and b:
// for each budget A owns where B has access, revoke B; and symmetrically for
// budgets B owns where A has access.
func (r *BudgetAccessRevoker) RevokeBetween(ctx context.Context, a, b vo.Id) error {
	if err := r.revokeDirection(ctx, a, b); err != nil {
		return err
	}
	return r.revokeDirection(ctx, b, a)
}

// revokeDirection: for each budget in `owner`'s list, if `other` holds access,
// delete it. ListForUser returns owned + accessible budgets; the DeleteAccess is
// a no-op when `other` has no row, so this is safe over the full list.
func (r *BudgetAccessRevoker) revokeDirection(ctx context.Context, owner, other vo.Id) error {
	budgets, err := r.budgets.ListForUser(ctx, owner)
	if err != nil {
		return err
	}
	for _, bud := range budgets {
		access, aerr := r.budgets.ListAccess(ctx, bud.Id())
		if aerr != nil {
			return aerr
		}
		for _, acc := range access {
			if acc.UserId().Equal(other) {
				if derr := r.budgets.DeleteAccess(ctx, bud.Id(), other); derr != nil {
					return derr
				}
				break
			}
		}
	}
	return nil
}
