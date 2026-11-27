package budget

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/econumo/econumo/internal/shared/vo"
)

// WipeBudgetIDs returns the budgets a wipe of the user touches: the ones they
// own plus the ones they have an access row on. Distinct, sorted.
func (s *Service) WipeBudgetIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	owned, err := s.budgets.ListOwnedIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	member, err := s.access.ListMemberIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	return sortedIDs(append(owned, member...)), nil
}

type pointerClear struct{ userID, budgetID vo.Id }

// UnwindUser removes the user from every budget they are a member of and
// clears the active-budget pointer of every member of every budget the user
// owns. Each clear takes a user row lock, so every budget-side change goes
// first and the clears follow in ascending user id order (ties by budget id),
// the one global order a concurrent wipe or revoke also follows.
func (s *Service) UnwindUser(ctx context.Context, userID vo.Id) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		memberOf, err := s.access.ListMemberIDs(txCtx, userID)
		if err != nil {
			return err
		}
		owned, err := s.budgets.ListOwnedIDs(txCtx, userID)
		if err != nil {
			return err
		}
		var clears []pointerClear
		for _, budgetID := range sortedIDs(memberOf) {
			if err := s.RemoveMemberKeepPointer(txCtx, budgetID, userID); err != nil {
				return err
			}
			clears = append(clears, pointerClear{userID: userID, budgetID: budgetID})
		}
		for _, budgetID := range owned {
			access, err := s.access.ListAccess(txCtx, budgetID)
			if err != nil {
				return err
			}
			for _, a := range access {
				clears = append(clears, pointerClear{userID: a.UserID, budgetID: budgetID})
			}
		}
		slices.SortFunc(clears, func(x, y pointerClear) int {
			return cmp.Or(
				strings.Compare(x.userID.String(), y.userID.String()),
				strings.Compare(x.budgetID.String(), y.budgetID.String()),
			)
		})
		clears = slices.CompactFunc(clears, func(x, y pointerClear) bool {
			return x.userID.Equal(y.userID) && x.budgetID.Equal(y.budgetID)
		})
		for _, c := range clears {
			if err := s.users.ClearActiveBudget(txCtx, c.userID, c.budgetID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteOwned deletes every budget the user owns; their children cascade.
func (s *Service) DeleteOwned(ctx context.Context, userID vo.Id) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		owned, err := s.budgets.ListOwnedIDs(txCtx, userID)
		if err != nil {
			return err
		}
		for _, budgetID := range sortedIDs(owned) {
			if err := s.budgets.Delete(txCtx, budgetID); err != nil {
				return err
			}
		}
		return nil
	})
}

func sortedIDs(ids []vo.Id) []vo.Id {
	slices.SortFunc(ids, func(x, y vo.Id) int { return strings.Compare(x.String(), y.String()) })
	return slices.CompactFunc(ids, func(x, y vo.Id) bool { return x.String() == y.String() })
}
