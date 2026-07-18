package budget

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GrantAccess grants/updates a user's access to a budget (canShare). Returns the
// requester's budget list. New grants are pending (not accepted).
func (s *Service) GrantAccess(ctx context.Context, userID vo.Id, req model.GrantAccessRequest) (*model.GrantAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	invitedID, err := vo.ParseId(req.UserId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"userId": ""})
	}
	role, err := model.BudgetRoleFromAlias(req.Role)
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
		existing, gerr := s.access.GetAccess(txCtx, budgetID, invitedID)
		if gerr != nil {
			var nf *errs.NotFoundError
			if !errors.As(gerr, &nf) {
				return gerr
			}
			grant := model.NewBudgetAccess(s.access.NextIdentity(), budgetID, invitedID, role, now)
			return s.access.SaveAccess(txCtx, grant)
		}
		existing.UpdateRole(role, now)
		return s.access.SaveAccess(txCtx, existing)
	})
	if err != nil {
		return nil, err
	}
	list, err := s.GetBudgetList(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &model.GrantAccessResult{Items: list.Items}, nil
}

// AcceptAccess accepts a pending invite + seeds the invited user's elements
// (canAccept). Returns the user's budget list.
func (s *Service) AcceptAccess(ctx context.Context, userID vo.Id, req model.AcceptAccessRequest) (*model.AcceptAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
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
		grant, gerr := s.access.GetAccess(txCtx, budgetID, userID)
		if gerr != nil {
			return gerr
		}
		grant.Accept(now)
		if serr := s.access.SaveAccess(txCtx, grant); serr != nil {
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
	return &model.AcceptAccessResult{Items: list.Items}, nil
}

// RevokeAccess removes a user's access (canShare).
func (s *Service) RevokeAccess(ctx context.Context, userID vo.Id, req model.RevokeAccessRequest) (*model.RevokeAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	invitedID, err := vo.ParseId(req.UserId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"userId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canShare(b, userID) {
		return nil, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if derr := s.access.DeleteAccess(txCtx, budgetID, invitedID); derr != nil {
			return derr
		}
		// A stale active-budget option would make the revoked user's client keep
		// requesting a budget that now 403s.
		return s.users.ClearActiveBudget(txCtx, invitedID, budgetID)
	}); err != nil {
		return nil, err
	}
	return &model.RevokeAccessResult{}, nil
}

// DeclineAccess declines an invite (the requester removes their own access).
func (s *Service) DeclineAccess(ctx context.Context, userID vo.Id, req model.DeclineAccessRequest) (*model.DeclineAccessResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canDecline(b, userID) {
		return nil, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if derr := s.access.DeleteAccess(txCtx, budgetID, userID); derr != nil {
			return derr
		}
		return s.users.ClearActiveBudget(txCtx, userID, budgetID)
	}); err != nil {
		return nil, err
	}
	return &model.DeclineAccessResult{}, nil
}
