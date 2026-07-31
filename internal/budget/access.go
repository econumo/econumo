package budget

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// budgetRole returns the requesting user's role on a budget: owner if they own
// it; the accepted access role otherwise; AccessDenied if no accepted access.
func (s *Service) budgetRole(b *budgetAggregate, userID vo.Id) (model.BudgetRole, error) {
	if b.budget.UserID.Equal(userID) {
		return model.BudgetRoleOwner, nil
	}
	for _, a := range b.access {
		if a.UserID.Equal(userID) {
			if !a.IsAccepted {
				return 0, errs.NewAccessDenied("Access denied")
			}
			return a.Role, nil
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
	return err == nil && (r == model.BudgetRoleOwner || r == model.BudgetRoleAdmin)
}

// canUpdate/canEdit = owner|admin|user.
func (s *Service) canUpdate(b *budgetAggregate, userID vo.Id) bool {
	r, err := s.budgetRole(b, userID)
	return err == nil && (r == model.BudgetRoleOwner || r == model.BudgetRoleAdmin || r == model.BudgetRoleUser)
}

// canShare = owner|admin. Fails closed: any error (no access, pending invite,
// stranger) denies sharing. Only accepted owner/admin may grant or revoke.
func (s *Service) canShare(b *budgetAggregate, userID vo.Id) bool {
	r, err := s.budgetRole(b, userID)
	return err == nil && (r == model.BudgetRoleOwner || r == model.BudgetRoleAdmin)
}

// canAccept = has an UNaccepted access row for the user.
func (s *Service) canAccept(b *budgetAggregate, userID vo.Id) bool {
	for _, a := range b.access {
		if a.UserID.Equal(userID) && !a.IsAccepted {
			return true
		}
	}
	return false
}

// canDecline = has any access row for the user.
func (s *Service) canDecline(b *budgetAggregate, userID vo.Id) bool {
	for _, a := range b.access {
		if a.UserID.Equal(userID) {
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
