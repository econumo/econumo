package authserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	ScopeRead       = "read"
	ShareClientName = "Econumo Share"
)

// ShareClient is the first-party client the share app signs in with; it is
// seeded from config at boot and always granted the read scope.
type ShareClient struct {
	ID          vo.Id
	Secret      string
	RedirectURI string
}

func (s *Service) ConfigureShareClient(ctx context.Context, sc ShareClient) error {
	if !s.Enabled() {
		return errors.New("ECONUMO_SHARE_CLIENT_ID requires the MCP OAuth server: ECONUMO_URL must be an https origin with no path")
	}
	if err := ValidateRedirectURI(sc.RedirectURI); err != nil {
		return fmt.Errorf("ECONUMO_SHARE_REDIRECT_URI: %w", err)
	}
	hash := hashSecret(sc.Secret)
	c := &model.OAuthClient{ID: sc.ID, Name: ShareClientName, RedirectURIs: []string{sc.RedirectURI},
		SecretHash: &hash, CreatedAt: s.clock.Now().UTC(), FirstParty: true}
	if err := s.repo.UpsertFirstPartyClient(ctx, c); err != nil {
		return err
	}
	id := sc.ID
	s.share = &id
	return nil
}

func (s *Service) ShareClientEnabled() bool { return s.share != nil }

func (s *Service) isShareClient(c *model.OAuthClient) bool {
	return c.FirstParty && s.share != nil && c.ID.Equal(*s.share)
}

// A first-party row left behind after the share config was removed or its id
// changed is treated as unknown: otherwise its secret could still buy a grant,
// and with no configured share client that grant would be mcp-scoped.
func (s *Service) usableClient(c *model.OAuthClient) bool {
	return !c.FirstParty || s.isShareClient(c)
}

func (s *Service) grantedScope(c *model.OAuthClient) model.TokenScope {
	if s.isShareClient(c) {
		return model.TokenScopeRead
	}
	return model.TokenScopeMCP
}
