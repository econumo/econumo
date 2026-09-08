package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// ExchangeHandoff redeems a one-shot code for a session. The row is deleted
// before anything else so a replay races nothing.
func (s *Service) ExchangeHandoff(ctx context.Context, req model.ExchangeHandoffRequest, userAgent string) (*model.LoginResult, error) {
	invalid := &errs.UnauthorizedError{Msg: "Sign-in link is invalid or has expired", Code: errs.CodeOAuthHandoffInvalid}
	hash := oidc.Sha256Hex(req.Code)
	h, err := s.handoffs.Get(ctx, hash)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, invalid
		}
		return nil, err
	}
	if derr := s.handoffs.Delete(ctx, hash); derr != nil {
		return nil, derr
	}
	if h.IsExpired(s.clock.Now()) {
		return nil, invalid
	}
	res, err := s.users.MintSession(ctx, h.UserID, userAgent, h.Provider, h.IDToken)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "user_id", h.UserID.String())
	reqctx.AddLogAttr(ctx, "provider", h.Provider)
	return res, nil
}
