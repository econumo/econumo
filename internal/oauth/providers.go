package oauth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/model"
)

const googleIssuerURL = "https://accounts.google.com"

// Provider is one enabled slot: the protocol client plus the button label.
type Provider struct {
	Client *oidc.Client
	Name   string
}

// ProvidersFromConfig builds the enabled slots in display order. Google and
// Apple are fixed issuers with their quirks preconfigured; the custom slot
// follows the operator's variables.
func ProvidersFromConfig(cfg config.Config, hc *http.Client) ([]Provider, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	var out []Provider
	if cfg.OAuthGoogleEnabled() {
		out = append(out, Provider{Name: "Google", Client: oidc.NewClient(oidc.Issuer{
			ID: model.OAuthProviderGoogle, IssuerURL: googleIssuerURL, ClientID: cfg.OAuthGoogleClientID,
			ClientSecret: oidc.StaticSecret(cfg.OAuthGoogleClientSecret),
			Scopes:       []string{"openid", "email", "profile"}, UsePKCE: true, TrustEmail: true,
			ExtraAuthParams: map[string]string{"prompt": "select_account"},
		}, hc)})
	}
	if cfg.OAuthAppleEnabled() {
		key, err := oidc.ParseApplePrivateKey(cfg.OAuthApplePrivateKey)
		if err != nil {
			return nil, fmt.Errorf("ECONUMO_OAUTH_APPLE_PRIVATE_KEY: %w", err)
		}
		teamID, clientID, keyID := cfg.OAuthAppleTeamID, cfg.OAuthAppleClientID, cfg.OAuthAppleKeyID
		out = append(out, Provider{Name: "Apple", Client: oidc.NewClient(oidc.Issuer{
			ID: model.OAuthProviderApple, IssuerURL: oidc.AppleIssuerURL, ClientID: clientID,
			ClientSecret: func(now time.Time) (string, error) { return oidc.AppleClientSecret(teamID, clientID, keyID, key, now) },
			// Apple returns name/email only with form_post; PKCE is undocumented there,
			// the confidential-client secret plus nonce bind the exchange.
			Scopes: []string{"name", "email"}, UsePKCE: false, ResponseMode: "form_post", TrustEmail: true,
		}, hc)})
	}
	if cfg.OIDCEnabled() {
		out = append(out, Provider{Name: cfg.OIDCName, Client: oidc.NewClient(oidc.Issuer{
			ID: model.OAuthProviderOIDC, IssuerURL: cfg.OIDCIssuerURL, ClientID: cfg.OIDCClientID,
			ClientSecret: oidc.StaticSecret(cfg.OIDCClientSecret),
			Scopes:       cfg.OIDCScopes, UsePKCE: true, TrustEmail: cfg.OIDCTrustEmail,
		}, hc)})
	}
	return out, nil
}
