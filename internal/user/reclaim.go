package user

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// reclaimCredentials takes away every way into the account that never proved
// the mailbox, in the caller's transaction, after the caller has written the
// new password hash. It is the ONE definition of "reclaim": the reset flow
// (mailbox proven by the code) and the operator's user:change-password both
// use it, so a credential class added later (a pending grant, a linked
// identity) is swept from both paths or from neither.
func (s *Service) reclaimCredentials(ctx context.Context, u *model.User, provenEmail string) error {
	// The fence: every flow that read its evidence before this point is now
	// invalid, whatever it does next (see AccessTokens.InsertIfGeneration).
	if err := s.repo.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		return err
	}
	// Every outstanding code, not just the one presented.
	if err := s.passwordRequests.DeleteByUser(ctx, u.ID); err != nil {
		return err
	}
	if err := s.emailChangeRequests.DeleteByUser(ctx, u.ID); err != nil {
		return err
	}
	if err := s.revokeTokens(ctx, u.ID, vo.Id{}, s.clock.Now(), model.TokenKindSession, model.TokenKindPersonal); err != nil {
		return err
	}
	if s.oauthGrants == nil {
		return nil
	}
	identities, grants, err := s.oauthGrants.ReclaimAccount(ctx, u.ID, strings.ToLower(strings.TrimSpace(provenEmail)))
	if err != nil {
		return err
	}
	if identities > 0 {
		reqctx.AddLogAttr(ctx, "identities_unlinked", identities)
	}
	if grants > 0 {
		reqctx.AddLogAttr(ctx, "oauth_grants_revoked", grants)
	}
	return nil
}
