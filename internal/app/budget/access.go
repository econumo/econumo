package budget

import (
	"context"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// budgetRole returns the requesting user's role on a budget: owner if they own
// it; the accepted access role otherwise; AccessDenied if no accepted access.
// Mirrors BudgetAccessService::getBudgetRole.
func (s *Service) budgetRole(b *budgetAggregate, userID vo.Id) (dombudget.UserRole, error) {
	if b.budget.UserId().Equal(userID) {
		return dombudget.RoleOwner, nil
	}
	for _, a := range b.access {
		if a.UserId().Equal(userID) {
			if !a.IsAccepted() {
				return 0, errs.NewAccessDenied("Access denied")
			}
			return a.Role(), nil
		}
	}
	return 0, errs.NewAccessDenied("Access denied")
}

func (s *Service) canRead(b *budgetAggregate, userID vo.Id) bool {
	_, err := s.budgetRole(b, userID)
	return err == nil
}

// canDelete/canReset = owner|admin.
func (s *Service) canDelete(b *budgetAggregate, userID vo.Id) bool {
	r, err := s.budgetRole(b, userID)
	return err == nil && (r == dombudget.RoleOwner || r == dombudget.RoleAdmin)
}

// canUpdate/canEdit = owner|admin|user.
func (s *Service) canUpdate(b *budgetAggregate, userID vo.Id) bool {
	r, err := s.budgetRole(b, userID)
	return err == nil && (r == dombudget.RoleOwner || r == dombudget.RoleAdmin || r == dombudget.RoleUser)
}

// canShare = owner|admin; PHP's AccessDenied fallback returns true (replicated).
func (s *Service) canShare(b *budgetAggregate, userID vo.Id) bool {
	r, err := s.budgetRole(b, userID)
	if err != nil {
		return true // matches PHP canShareBudget's catch branch
	}
	return r == dombudget.RoleOwner || r == dombudget.RoleAdmin
}

// canAccept = has an UNaccepted access row for the user.
func (s *Service) canAccept(b *budgetAggregate, userID vo.Id) bool {
	for _, a := range b.access {
		if a.UserId().Equal(userID) && !a.IsAccepted() {
			return true
		}
	}
	return false
}

// canDecline = has any access row for the user.
func (s *Service) canDecline(b *budgetAggregate, userID vo.Id) bool {
	for _, a := range b.access {
		if a.UserId().Equal(userID) {
			return true
		}
	}
	return false
}

// accessDenied is the shared error.
func accessDenied() error { return errs.NewAccessDenied("Access denied") }

// requireBudget loads a budget and checks at least read access.
func (s *Service) requireBudget(ctx context.Context, userID, budgetID vo.Id) (*budgetAggregate, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canRead(b, userID) {
		return nil, accessDenied()
	}
	return b, nil
}
