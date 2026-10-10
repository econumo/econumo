package authserver

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type stubLimiter struct{ err error }

func (l stubLimiter) Allow(string, string) error { return l.err }

func TestRegister(t *testing.T) {
	s, _, _, _ := newTestService(t)
	res, err := s.Register(ctx, model.ClientRegistrationRequest{ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}})
	if err != nil || res.ClientID == "" || res.ClientSecret != "" || res.TokenEndpointAuthMethod != "none" {
		t.Fatalf("%+v %v", res, err)
	}
	if len(res.GrantTypes) != 2 || res.ResponseTypes[0] != "code" {
		t.Fatalf("defaults: %+v", res)
	}
	conf, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://a.test/cb"}, TokenEndpointAuthMethod: "client_secret_post"})
	if err != nil || conf.ClientSecret == "" || conf.ClientName != "MCP client" || conf.ClientSecretExpiresAt == nil || *conf.ClientSecretExpiresAt != 0 {
		t.Fatalf("confidential: %+v %v", conf, err)
	}
	for _, bad := range []model.ClientRegistrationRequest{
		{},
		{RedirectURIs: []string{"http://evil.test/cb"}},
		{RedirectURIs: []string{"https://a.test/cb"}, TokenEndpointAuthMethod: "private_key_jwt"},
		{RedirectURIs: []string{"https://a.test/cb"}, GrantTypes: []string{"client_credentials"}},
		{RedirectURIs: []string{"https://a.test/cb"}, ResponseTypes: []string{"token"}},
		{RedirectURIs: []string{"https://a.test/cb"}, ClientName: strings.Repeat("x", 101)},
		{RedirectURIs: []string{"https://a/1", "https://a/2", "https://a/3", "https://a/4", "https://a/5", "https://a/6", "https://a/7", "https://a/8", "https://a/9", "https://a/10", "https://a/11"}},
	} {
		var oe *OAuthError
		if _, err := s.Register(ctx, bad); !errors.As(err, &oe) || oe.Status != 400 {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestRegisterDisabledAndRateLimited(t *testing.T) {
	s, creds, _, _ := newTestService(t)
	req := model.ClientRegistrationRequest{RedirectURIs: []string{"https://a.test/cb"}}

	off := NewService(s.repo, creds, s.tx, s.clock, nil, "")
	var oe *OAuthError
	if _, err := off.Register(ctx, req); !errors.As(err, &oe) || oe.Status != 404 {
		t.Fatalf("disabled: %v", err)
	}

	limited := NewService(s.repo, creds, s.tx, s.clock, stubLimiter{errs.NewTooManyRequests("slow down")}, testURL)
	if _, err := limited.Register(ctx, req); !errors.As(err, &oe) || oe.Status != 429 || oe.Code != "temporarily_unavailable" {
		t.Fatalf("limited: %v", err)
	}
	other := errors.New("boom")
	broken := NewService(s.repo, creds, s.tx, s.clock, stubLimiter{other}, testURL)
	if _, err := broken.Register(ctx, req); !errors.Is(err, other) {
		t.Fatalf("limiter failure must surface: %v", err)
	}
}

func TestRegisterPurgesUnusedClients(t *testing.T) {
	s, _, clock, _ := newTestService(t)
	old, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://a.test/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	// A client that registers and only reaches the consent page days later
	// (a laptop asleep over a weekend) must still be known.
	clock.Advance(7 * 24 * time.Hour)
	if _, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://a.test/cb"}}); err != nil {
		t.Fatal(err)
	}
	q := authReq(old.ClientID)
	q.RedirectURI = "https://a.test/cb"
	if _, err := s.DescribeAuthorization(ctx, vo.NewId(), q); err != nil {
		t.Fatalf("a week-old unused client must survive the purge: %v", err)
	}
	clock.Advance(UnusedClientTTL)
	if _, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://a.test/cb"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DescribeAuthorization(ctx, vo.NewId(), authReq(old.ClientID)); !hasCode(err, errs.CodeAuthServerClientNotFound) {
		t.Fatalf("unused client must be purged: %v", err)
	}
}

func TestRegisterSanitizesAndCaps(t *testing.T) {
	s, _, _, _ := newTestService(t)
	res, err := s.Register(ctx, model.ClientRegistrationRequest{ClientName: "Cla\u202eude\n", RedirectURIs: []string{"https://a.test/cb"}})
	if err != nil || res.ClientName != "Claude" {
		t.Fatalf("name must lose control and bidi runes: %q %v", res.ClientName, err)
	}
	only, err := s.Register(ctx, model.ClientRegistrationRequest{ClientName: "\u202e\n", RedirectURIs: []string{"https://a.test/cb"}})
	if err != nil || only.ClientName != "MCP client" {
		t.Fatalf("name that is only invisible runes falls back to the default: %q %v", only.ClientName, err)
	}
	var oe *OAuthError
	long := "https://a.test/" + strings.Repeat("x", 2048)
	if _, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{long}}); !errors.As(err, &oe) || oe.Code != "invalid_redirect_uri" {
		t.Fatalf("long redirect URI: %v", err)
	}
	if _, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://cl\u0430ude.ai/cb"}}); !errors.As(err, &oe) || oe.Code != "invalid_redirect_uri" {
		t.Fatalf("homograph redirect URI: %v", err)
	}
}
