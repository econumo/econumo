package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) StartLogin(ctx context.Context, req model.StartOAuthRequest) (*model.StartOAuthResult, error) {
	return s.start(ctx, req, model.OAuthIntentLogin, vo.Id{})
}

func (s *Service) StartLink(ctx context.Context, userID vo.Id, req model.StartOAuthRequest) (*model.StartOAuthResult, error) {
	return s.start(ctx, req, model.OAuthIntentLink, userID)
}

// start creates the state row and returns the provider's authorization URL.
// Expired states are purged opportunistically here, the cheapest moment.
func (s *Service) start(ctx context.Context, req model.StartOAuthRequest, intent string, linkUser vo.Id) (*model.StartOAuthResult, error) {
	p, err := s.provider(req.Provider)
	if err != nil {
		return nil, err
	}
	state, err := oidc.RandomToken()
	if err != nil {
		return nil, err
	}
	nonce, err := oidc.RandomToken()
	if err != nil {
		return nil, err
	}
	verifier, challenge := "", ""
	if p.Client.Issuer().UsePKCE {
		if verifier, err = oidc.RandomToken(); err != nil {
			return nil, err
		}
		challenge = oidc.PKCEChallenge(verifier)
	}
	now := s.clock.Now()
	if _, err := s.states.DeleteExpired(ctx, now); err != nil {
		return nil, err
	}
	if _, err := s.handoffs.DeleteExpired(ctx, now); err != nil {
		return nil, err
	}
	if err := s.states.Insert(ctx, &model.OAuthState{
		StateHash: oidc.Sha256Hex(state), Provider: req.Provider, Nonce: nonce, CodeVerifier: verifier,
		Client: req.Client, Intent: intent, LinkUserID: linkUser, CreatedAt: now, ExpiresAt: now.Add(model.OAuthStateTTL),
	}); err != nil {
		return nil, err
	}
	u, err := p.Client.AuthURL(ctx, state, nonce, challenge, s.RedirectURI(req.Provider))
	if err != nil {
		return nil, err
	}
	return &model.StartOAuthResult{Url: u}, nil
}
