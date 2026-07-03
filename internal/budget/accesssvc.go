package budget

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GrantAccess grants/updates a user's access to a budget (canShare). Returns the
// requester's budget list. New grants are pending (not accepted).
func (s *Service) GrantAccess(ctx context.Context, userID vo.Id, req GrantAccessRequest) (*GrantAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	invitedID, err := vo.ParseId(req.UserId)
	if err != nil {
		return nil, validateBlank(map[string]string{"userId": ""})
	}
	role, err := RoleFromAlias(req.Role)
	if err != nil {
		return nil, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canShare(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		existing, gerr := s.repo.GetAccess(txCtx, budgetID, invitedID)
		if gerr != nil {
			var nf *errs.NotFoundError
			if !errors.As(gerr, &nf) {
				return gerr
			}
			grant := NewBudgetAccess(s.repo.NextIdentity(), budgetID, invitedID, role, now)
			return s.repo.SaveAccess(txCtx, grant)
		}
		existing.UpdateRole(role, now)
		return s.repo.SaveAccess(txCtx, existing)
	})
	if err != nil {
		return nil, err
	}
	list, err := s.GetBudgetList(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &GrantAccessResult{Items: list.Items}, nil
}

// AcceptAccess accepts a pending invite + seeds the invited user's elements
// (canAccept). Returns the user's budget list.
func (s *Service) AcceptAccess(ctx context.Context, userID vo.Id, req AcceptAccessRequest) (*AcceptAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canAccept(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		grant, gerr := s.repo.GetAccess(txCtx, budgetID, userID)
		if gerr != nil {
			return gerr
		}
		grant.Accept(now)
		if serr := s.repo.SaveAccess(txCtx, grant); serr != nil {
			return serr
		}
		// Seed the newly-accepted user's category + tag elements.
		pos, serr := s.seedCategoryElements(txCtx, userID, budgetID, nextElementPosition(b), now)
		if serr != nil {
			return serr
		}
		return s.seedTagElements(txCtx, userID, budgetID, pos, now)
	})
	if err != nil {
		return nil, err
	}
	list, err := s.GetBudgetList(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AcceptAccessResult{Items: list.Items}, nil
}

// RevokeAccess removes a user's access (canShare).
func (s *Service) RevokeAccess(ctx context.Context, userID vo.Id, req RevokeAccessRequest) (*RevokeAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	invitedID, err := vo.ParseId(req.UserId)
	if err != nil {
		return nil, validateBlank(map[string]string{"userId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canShare(b, userID) {
		return nil, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		return s.repo.DeleteAccess(txCtx, budgetID, invitedID)
	}); err != nil {
		return nil, err
	}
	return &RevokeAccessResult{}, nil
}

// DeclineAccess declines an invite (the requester removes their own access).
func (s *Service) DeclineAccess(ctx context.Context, userID vo.Id, req DeclineAccessRequest) (*DeclineAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canDecline(b, userID) {
		return nil, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		return s.repo.DeleteAccess(txCtx, budgetID, userID)
	}); err != nil {
		return nil, err
	}
	return &DeclineAccessResult{}, nil
}
