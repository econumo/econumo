package authserver

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestGrantedScope(t *testing.T) {
	for requested, want := range map[string]model.TokenScope{
		"":                model.TokenScopeMCP,
		"   ":             model.TokenScopeMCP,
		"mcp":             model.TokenScopeMCP,
		"read mcp":        model.TokenScopeMCP,
		"mcp read":        model.TokenScopeMCP,
		"profile":         model.TokenScopeMCP,
		"readonly":        model.TokenScopeMCP,
		"read":            model.TokenScopeRead,
		" read ":          model.TokenScopeRead,
		"read profile":    model.TokenScopeRead,
		"openid read foo": model.TokenScopeRead,
	} {
		if got := grantedScope(requested); got != want {
			t.Errorf("grantedScope(%q) = %q, want %q", requested, got, want)
		}
	}
}

// Through authorize, exchange and three refreshes, whatever the refresh asks for:
// the scope is fixed at approval.
func TestRegisteredClient_ScopeSurvivesExchangeAndRefresh(t *testing.T) {
	for _, tc := range []struct {
		name, requested string
		want            model.TokenScope
	}{
		{"read", "read", model.TokenScopeRead},
		{"nothing", "", model.TokenScopeMCP},
		{"mcp", "mcp", model.TokenScopeMCP},
		{"both", "mcp read", model.TokenScopeMCP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, creds, clock, user := newTestService(t)
			c := register(t, s, "Reports", "https://claude.ai/api/mcp/auth_callback")
			req := authReq(c.ClientID)
			req.Scope = tc.requested

			desc, err := s.DescribeAuthorization(ctx, user, req)
			if err != nil || desc.Scope != string(tc.want) {
				t.Fatalf("describe = %+v %v", desc, err)
			}
			res, err := s.ApproveAuthorization(ctx, user, session, true, req)
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(res.RedirectURL)
			tr, err := s.Token(ctx, model.TokenRequest{GrantType: "authorization_code", ClientID: c.ClientID, Code: u.Query().Get("code"),
				RedirectURI: req.RedirectURI, CodeVerifier: verifier})
			if err != nil {
				t.Fatal(err)
			}
			check := func(step string) {
				t.Helper()
				if tr.Scope != string(tc.want) || creds.scopes[tr.AccessToken] != tc.want {
					t.Fatalf("%s: scope = %q, token scope %q, want %q", step, tr.Scope, creds.scopes[tr.AccessToken], tc.want)
				}
			}
			check("exchange")
			g, err := s.repo.GetGrantByRefreshHash(ctx, hashSecret(tr.RefreshToken))
			if err != nil || g.Scope != tc.want {
				t.Fatalf("grant = %+v, %v", g, err)
			}
			for _, asked := range []string{"", "mcp", "read"} {
				clock.Advance(time.Minute)
				tr, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: c.ClientID, RefreshToken: tr.RefreshToken, Scope: asked})
				if err != nil {
					t.Fatalf("refresh asking %q: %v", asked, err)
				}
				check("refresh asking " + asked)
			}
		})
	}
}

func TestApprove_ReadonlyMayApproveReadOnly(t *testing.T) {
	s, _, _, user := newTestService(t)
	c := register(t, s, "Reports", "https://claude.ai/api/mcp/auth_callback")

	req := authReq(c.ClientID)
	req.Scope = "read"
	res, err := s.ApproveAuthorization(ctx, user, session, false, req)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	if u.Query().Get("code") == "" {
		t.Fatalf("no code in %s", res.RedirectURL)
	}

	for _, scope := range []string{"", "mcp", "mcp read"} {
		req.Scope = scope
		_, err := s.ApproveAuthorization(ctx, user, session, false, req)
		var pe *errs.PaymentRequiredError
		if !errors.As(err, &pe) || pe.Code != errs.CodeReadonlyAccess {
			t.Fatalf("scope %q: err = %v", scope, err)
		}
	}
}
