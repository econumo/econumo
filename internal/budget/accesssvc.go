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
	// A budget may only be shared with a connected user (never with yourself).
	connected, err := s.connections.AreConnected(ctx, userID, invitedID)
	if err != nil {
		return nil, err
	}
	if !connected {
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
		// Seed the newly-accepted user's category + tag elements. Elements may
		// already exist from an earlier membership (revoke keeps them, and
		// pre-handshake budgets carry them for pending members) — skip those.
		existing := make(map[vo.Id]bool, len(b.elements))
		for _, el := range b.elements {
			existing[el.ExternalID] = true
		}
		pos, serr := s.seedCategoryElements(txCtx, userID, budgetID, nextElementPosition(b), now, existing)
		if serr != nil {
			return serr
		}
		return s.seedTagElements(txCtx, userID, budgetID, pos, now, existing)
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

// removeMemberRecords deletes a departing member's seeded records from the
// budget: their category/tag elements (limits cascade via FK) and their
// categories' envelope assignments. Runs in the caller's transaction; revoke
// is deliberate, so the member's limit history goes with them.
func (s *Service) removeMemberRecords(ctx context.Context, b *budgetAggregate, memberID vo.Id) error {
	owned := map[vo.Id]bool{}
	cats, err := s.metadata.CategoriesByOwners(ctx, []vo.Id{memberID})
	if err != nil {
		return err
	}
	for _, c := range cats {
		id, perr := vo.ParseId(c.ID)
		if perr != nil {
			return perr
		}
		owned[id] = true
	}
	tags, err := s.metadata.TagsByOwners(ctx, []vo.Id{memberID})
	if err != nil {
		return err
	}
	for _, t := range tags {
		id, perr := vo.ParseId(t.ID)
		if perr != nil {
			return perr
		}
		owned[id] = true
	}
	for _, el := range b.elements {
		if el.Type == model.ElementEnvelope || !owned[el.ExternalID] {
			continue
		}
		if derr := s.elements.DeleteElement(ctx, el.ID); derr != nil {
			return derr
		}
	}
	for _, env := range b.envelopes {
		catIDs, cerr := s.envelopes.EnvelopeCategoryIDs(ctx, env.ID)
		if cerr != nil {
			return cerr
		}
		for _, catID := range catIDs {
			if !owned[catID] {
				continue
			}
			if rerr := s.envelopes.RemoveEnvelopeCategory(ctx, env.ID, catID); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

// RemoveMember drops a user's access to a budget together with their seeded
// records, without a permission check — for the composition root (the
// delete-connection unwind), which has already established authority.
func (s *Service) RemoveMember(ctx context.Context, budgetID, memberID vo.Id) error {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := s.access.DeleteAccess(txCtx, budgetID, memberID); err != nil {
			return err
		}
		if err := s.removeMemberRecords(txCtx, b, memberID); err != nil {
			return err
		}
		return s.users.ClearActiveBudget(txCtx, memberID, budgetID)
	})
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
		if rerr := s.removeMemberRecords(txCtx, b, invitedID); rerr != nil {
			return rerr
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
		// Pre-handshake budgets may carry seeded elements for a still-pending
		// member; declining sheds them the same way revoke does.
		if rerr := s.removeMemberRecords(txCtx, b, userID); rerr != nil {
			return rerr
		}
		return s.users.ClearActiveBudget(txCtx, userID, budgetID)
	}); err != nil {
		return nil, err
	}
	return &model.DeclineAccessResult{}, nil
}
