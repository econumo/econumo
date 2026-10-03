package authserver

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	defaultClientName = "MCP client"
	maxClientName     = 100
	maxRedirectURIs   = 10
	maxRedirectURILen = 2048
)

func (s *Service) Register(ctx context.Context, req model.ClientRegistrationRequest) (model.ClientRegistrationResult, error) {
	if !s.Enabled() {
		return model.ClientRegistrationResult{}, &OAuthError{Status: 404, Code: "invalid_request", Description: "not available"}
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeRegister, ""); err != nil {
			if _, ok := errs.AsTooManyRequests(err); ok {
				return model.ClientRegistrationResult{}, &OAuthError{Status: 429, Code: "temporarily_unavailable", Description: "Too many registrations"}
			}
			return model.ClientRegistrationResult{}, err
		}
	}

	name := strings.TrimSpace(strings.Map(dropInvisible, req.ClientName))
	if name == "" {
		name = defaultClientName
	}
	if utf8.RuneCountInString(name) > maxClientName {
		return model.ClientRegistrationResult{}, invalidMetadata("client_name is too long")
	}
	if n := len(req.RedirectURIs); n < 1 || n > maxRedirectURIs {
		return model.ClientRegistrationResult{}, &OAuthError{Status: 400, Code: "invalid_redirect_uri", Description: "between 1 and 10 redirect_uris are required"}
	}
	for _, u := range req.RedirectURIs {
		if len(u) > maxRedirectURILen {
			return model.ClientRegistrationResult{}, &OAuthError{Status: 400, Code: "invalid_redirect_uri", Description: "redirect URI is too long"}
		}
		if err := ValidateRedirectURI(u); err != nil {
			return model.ClientRegistrationResult{}, &OAuthError{Status: 400, Code: "invalid_redirect_uri", Description: err.Error()}
		}
	}

	grantTypes := req.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code", "refresh_token"}
	}
	for _, g := range grantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			return model.ClientRegistrationResult{}, invalidMetadata("unsupported grant_types")
		}
	}
	responseTypes := req.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{"code"}
	}
	for _, r := range responseTypes {
		if r != "code" {
			return model.ClientRegistrationResult{}, invalidMetadata("unsupported response_types")
		}
	}
	method := req.TokenEndpointAuthMethod
	if method == "" {
		method = "none"
	}
	if !slices.Contains([]string{"none", "client_secret_post", "client_secret_basic"}, method) {
		return model.ClientRegistrationResult{}, invalidMetadata("unsupported token_endpoint_auth_method")
	}

	now := s.clock.Now().UTC()
	if _, err := s.repo.PurgeUnusedClients(ctx, now.Add(-UnusedClientTTL)); err != nil {
		slog.WarnContext(ctx, "purge unused oauth clients failed", "err", err.Error())
	}

	client := &model.OAuthClient{ID: vo.NewId(), Name: name, RedirectURIs: req.RedirectURIs, CreatedAt: now}
	res := model.ClientRegistrationResult{
		ClientID:                client.ID.String(),
		ClientIDIssuedAt:        now.Unix(),
		ClientName:              name,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		TokenEndpointAuthMethod: method,
	}
	if method != "none" {
		raw, hash, err := newSecret()
		if err != nil {
			return model.ClientRegistrationResult{}, err
		}
		client.SecretHash = &hash
		res.ClientSecret = raw
		never := int64(0)
		res.ClientSecretExpiresAt = &never
	}
	if err := s.repo.InsertClient(ctx, client); err != nil {
		return model.ClientRegistrationResult{}, err
	}
	reqctx.AddLogAttr(ctx, "client_id", res.ClientID)
	return res, nil
}

// Control and format runes (newlines, bidi overrides) would let a client name
// spoof or reflow the consent page.
func dropInvisible(r rune) rune {
	if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
		return -1
	}
	return r
}

func invalidMetadata(desc string) *OAuthError {
	return &OAuthError{Status: 400, Code: "invalid_client_metadata", Description: desc}
}
