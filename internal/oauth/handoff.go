package oauth

import (
	"context"
	"crypto/subtle"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ExchangeHandoff redeems a one-shot code for a session.
func (s *Service) ExchangeHandoff(ctx context.Context, req model.ExchangeHandoffRequest, userAgent string) (*model.LoginResult, error) {
	invalid := &errs.UnauthorizedError{Msg: "Sign-in link is invalid or has expired", Code: errs.CodeOAuthHandoffInvalid}
	h, err := s.consumeHandoff(ctx, oidc.Sha256Hex(req.Code), model.OAuthHandoffKindLogin, req.Flow)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, invalid
		}
		return nil, err
	}
	if h == nil {
		return nil, invalid
	}
	res, err := s.users.MintSession(ctx, h.UserID, userAgent, h.Provider, h.IDToken, h.Generation)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "user_id", h.UserID.String())
	reqctx.AddLogAttr(ctx, "provider", h.Provider)
	return res, nil
}

// consumeHandoff deletes the row and validates it: kind, expiry, and the flow
// secret of the client that started the flow. A nil handoff with a nil error
// means "rejected" — the caller decides which error its endpoint reports. The
// delete's affected-row count decides single use: two concurrent redemptions
// of one code can both SELECT the row, but only one DELETE removes it, on
// either engine.
func (s *Service) consumeHandoff(ctx context.Context, hash, kind, flow string) (*model.OAuthHandoff, error) {
	h, err := s.handoffs.Get(ctx, hash)
	if err != nil {
		return nil, err
	}
	n, err := s.handoffs.Delete(ctx, hash)
	if err != nil {
		return nil, err
	}
	if n != 1 || h.Kind != kind || h.IsExpired(s.clock.Now()) {
		return nil, nil
	}
	// The row is already gone, so a wrong secret costs the attacker the code too.
	if subtle.ConstantTimeCompare([]byte(oidc.Sha256Hex(flow)), []byte(h.FlowHash)) != 1 {
		return nil, nil
	}
	return h, nil
}

// CompleteLink performs the identity write the link callback deferred. Three
// things must line up: the one-shot code, the flow secret held only by the
// client that started the link, and an authenticated session for the account
// the link was started from. That last check is what a public callback can
// never make — without it, an attacker's start-link URL opened by a victim
// would bind the victim's provider identity to the attacker's account.
func (s *Service) CompleteLink(ctx context.Context, userID vo.Id, req model.CompleteLinkRequest) (*model.CompleteLinkResult, error) {
	invalid := &errs.ValidationError{Msg: "Linking session is invalid or has expired", MsgCode: errs.CodeOAuthLinkInvalid}
	h, err := s.consumeHandoff(ctx, oidc.Sha256Hex(req.Code), model.OAuthHandoffKindLink, req.Flow)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, invalid
		}
		return nil, err
	}
	if h == nil || !h.UserID.Equal(userID) {
		return nil, invalid
	}
	now := s.clock.Now()
	// The caller's session was checked by the middleware, which is already in
	// the past; the fence is what makes the write itself lose to a reclaim.
	gen, gerr := s.users.CredentialsGeneration(ctx, userID)
	if gerr != nil {
		return nil, gerr
	}
	existing, err := s.identities.GetByProviderSubject(ctx, h.Provider, h.Issuer, h.Subject)
	switch {
	case err == nil && !existing.UserID.Equal(userID):
		return nil, &errs.ValidationError{Msg: "This external account is already linked to another Econumo account", MsgCode: errs.CodeOAuthIdentityTaken}
	case err == nil:
		existing.UpdateEmail(h.Email, now)
		if n, serr := s.identities.SaveIfCurrent(ctx, existing, gen); serr != nil {
			return nil, serr
		} else if n != 1 {
			return nil, invalid
		}
	default:
		if _, ok := errs.AsNotFound(err); !ok {
			return nil, err
		}
		if _, gerr := s.identities.GetByUserProvider(ctx, userID, h.Provider); gerr == nil {
			return nil, &errs.ValidationError{Msg: "Your account already has a different account linked for this provider", MsgCode: errs.CodeOAuthProviderAlreadyLinked}
		} else if _, ok := errs.AsNotFound(gerr); !ok {
			return nil, gerr
		}
		if n, serr := s.identities.SaveIfCurrent(ctx, model.NewIdentity(s.identities.NextIdentity(), userID, h.Provider, h.Issuer, h.Subject, h.Email, now), gen); serr != nil {
			return nil, serr
		} else if n != 1 {
			return nil, invalid
		}
	}
	reqctx.AddLogAttr(ctx, "provider", h.Provider)
	return &model.CompleteLinkResult{Provider: h.Provider}, nil
}
