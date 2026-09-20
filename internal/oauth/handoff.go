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

// ExchangeHandoff redeems a one-shot code for a session. The whole redemption
// runs under the account's row lock, taken before the code is consumed: an
// unlink holds that same lock while it removes the identity and this
// provider's pending handoffs, so the two can only run one after the other.
func (s *Service) ExchangeHandoff(ctx context.Context, req model.ExchangeHandoffRequest, userAgent string) (*model.LoginResult, error) {
	invalid := &errs.UnauthorizedError{Msg: "Sign-in link is invalid or has expired", Code: errs.CodeOAuthHandoffInvalid}
	var res *model.LoginResult
	var h *model.OAuthHandoff
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// The row is read once before the lock only to learn whose lock to take;
		// consumeHandoffRow re-decides everything under it.
		peek, err := s.handoffs.Get(ctx, oidc.Sha256Hex(req.Code))
		if err != nil {
			return err
		}
		if err := s.users.LockRow(ctx, peek.UserID); err != nil {
			return err
		}
		// A rejection returns nil, not an error: the consuming DELETE must stay
		// committed, so presenting a wrong flow secret costs the attacker the
		// code. res left nil is what the caller reports on.
		h, err = s.consumeHandoffRow(ctx, peek, model.OAuthHandoffKindLogin, req.Flow)
		if err != nil || h == nil {
			return err
		}
		// The identity the callback authenticated must still be linked to this
		// user: an unlink committed after the callback has already deleted this
		// handoff, and the lookup also covers a handoff minted before handoffs
		// carried the identity at all.
		id, err := s.identities.GetByProviderSubject(ctx, h.Provider, h.Issuer, h.Subject)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return nil
			}
			return err
		}
		if !id.UserID.Equal(h.UserID) {
			return nil
		}
		res, err = s.users.MintSession(ctx, h.UserID, userAgent, h.Provider, h.IDToken, h.Generation)
		return err
	})
	if err != nil {
		// The only NotFound that escapes the closure is the unknown code.
		if _, ok := errs.AsNotFound(err); ok {
			return nil, invalid
		}
		return nil, err
	}
	if res == nil {
		return nil, invalid
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
	return s.consumeHandoffRow(ctx, h, kind, flow)
}

// consumeHandoffRow is consumeHandoff over a row the caller already read.
func (s *Service) consumeHandoffRow(ctx context.Context, h *model.OAuthHandoff, kind, flow string) (*model.OAuthHandoff, error) {
	n, err := s.handoffs.Delete(ctx, h.CodeHash)
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
	// Only a new identity row is a new sign-in method; refreshing the email of
	// one the account already holds grants nothing and must not notify.
	linked := false
	// The fence value was captured when the callback resolved the account
	// (h.Generation); the write is refused if a reclaim bumped it since,
	// whatever this request's session looked like at the middleware. The row
	// lock is what makes that refusal reliable on PostgreSQL: under READ
	// COMMITTED the fence's EXISTS cannot see a reclaim that has not committed
	// yet, so an unlocked insert would slip behind its identity sweep and leave
	// the account with a sign-in method the recovery was supposed to remove.
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if lerr := s.users.LockRow(ctx, userID); lerr != nil {
			return lerr
		}
		existing, gerr := s.identities.GetByProviderSubject(ctx, h.Provider, h.Issuer, h.Subject)
		switch {
		case gerr == nil && !existing.UserID.Equal(userID):
			return &errs.ValidationError{Msg: "This external account is already linked to another Econumo account", MsgCode: errs.CodeOAuthIdentityTaken}
		case gerr == nil:
			existing.UpdateEmail(h.Email, now)
			if n, serr := s.identities.UpdateIfCurrent(ctx, existing, h.Generation); serr != nil {
				return serr
			} else if n != 1 {
				return invalid
			}
		default:
			if _, ok := errs.AsNotFound(gerr); !ok {
				return gerr
			}
			if _, lerr := s.identities.GetByUserProvider(ctx, userID, h.Provider); lerr == nil {
				return &errs.ValidationError{Msg: "Your account already has a different account linked for this provider", MsgCode: errs.CodeOAuthProviderAlreadyLinked}
			} else if _, ok := errs.AsNotFound(lerr); !ok {
				return lerr
			}
			if n, serr := s.identities.InsertIfCurrent(ctx, model.NewIdentity(s.identities.NextIdentity(), userID, h.Provider, h.Issuer, h.Subject, h.Email, now), h.Generation); serr != nil {
				return serr
			} else if n != 1 {
				return invalid
			}
			linked = true
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// Told even though the user asked for this: the notice is how an owner
	// detects a link they did NOT ask for, and a stolen session is exactly what
	// the checks above are guarding against. Best-effort and outside the
	// transaction — the identity is already committed, so a dead mailer must
	// not fail the link.
	if linked && s.notifier != nil {
		if nerr := s.notifier.IdentityLinked(ctx, userID, s.providerName(h.Provider)); nerr != nil {
			logWarn(ctx, "oauth complete-link: identity-linked notice", nerr, "user_id", userID.String(), "provider", h.Provider)
		}
	}
	reqctx.AddLogAttr(ctx, "provider", h.Provider)
	return &model.CompleteLinkResult{Provider: h.Provider}, nil
}
