package authserver

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

func approve(t *testing.T, s *Service, user vo.Id) (clientID, code string) {
	t.Helper()
	c, err := s.Register(ctx, model.ClientRegistrationRequest{ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.ApproveAuthorization(ctx, user, authReq(c.ClientID))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	return c.ClientID, u.Query().Get("code")
}

func exchange(s *Service, clientID, code string) (model.TokenResponse, error) {
	return s.Token(ctx, model.TokenRequest{GrantType: "authorization_code", ClientID: clientID, Code: code,
		RedirectURI: "https://claude.ai/api/mcp/auth_callback", CodeVerifier: verifier, Resource: testURL + "/mcp"})
}

func wantOAuth(t *testing.T, err error, code string) {
	t.Helper()
	var oe *OAuthError
	if !errors.As(err, &oe) || oe.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func TestExchange(t *testing.T) {
	s, creds, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, err := exchange(s, cid, code)
	if err != nil || tr.AccessToken == "" || tr.RefreshToken == "" || tr.TokenType != "Bearer" || tr.ExpiresIn != 3600 || tr.Scope != "mcp" {
		t.Fatalf("%+v %v", tr, err)
	}
	_, err = exchange(s, cid, code)
	wantOAuth(t, err, "invalid_grant") // single use
	if creds.issued != 1 {
		t.Fatal("reuse must not mint")
	}
}

func TestExchange_Rejections(t *testing.T) {
	s, creds, clock, user := newTestService(t)
	cases := []struct {
		name string
		mut  func(*model.TokenRequest)
		want string
	}{
		{"wrong verifier", func(r *model.TokenRequest) { r.CodeVerifier = strings.Repeat("a", 43) }, "invalid_grant"},
		{"wrong redirect", func(r *model.TokenRequest) { r.RedirectURI = "http://localhost/cb" }, "invalid_grant"},
		{"wrong resource", func(r *model.TokenRequest) { r.Resource = "https://other.test/mcp" }, "invalid_target"},
		{"wrong client", func(r *model.TokenRequest) {
			// Registered, so it authenticates and consumes the code; an unknown id
			// would fail before the code is touched.
			c, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}})
			if err != nil {
				t.Fatal(err)
			}
			r.ClientID = c.ClientID
		}, "invalid_grant"},
	}
	for _, tc := range cases {
		cid, code := approve(t, s, user)
		req := model.TokenRequest{GrantType: "authorization_code", ClientID: cid, Code: code, RedirectURI: "https://claude.ai/api/mcp/auth_callback", CodeVerifier: verifier}
		tc.mut(&req)
		_, err := s.Token(ctx, req)
		if oe := new(OAuthError); !errors.As(err, &oe) || oe.Code != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.want)
		}
		if _, err := exchange(s, cid, code); err == nil {
			t.Errorf("%s: a failed attempt must burn the code", tc.name)
		}
	}
	cid, code := approve(t, s, user)
	clock.Advance(CodeTTL + time.Second)
	_, err := exchange(s, cid, code)
	wantOAuth(t, err, "invalid_grant") // expired
	clock.Advance(-CodeTTL - time.Second)
	cid, code = approve(t, s, user)
	creds.gen++ // a reclaim landed between approval and exchange
	_, err = exchange(s, cid, code)
	wantOAuth(t, err, "invalid_grant")
	if list, _ := s.ListConnectedApps(ctx, user); len(list) != 0 {
		t.Fatal("a fenced exchange must not leave a grant")
	}
}

func TestRefresh_RotationGraceAndTheft(t *testing.T) {
	s, creds, clock, user := newTestService(t)
	cid, code := approve(t, s, user)
	first, _ := exchange(s, cid, code)
	refresh := func(rt string) (model.TokenResponse, error) {
		return s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: rt})
	}
	second, err := refresh(first.RefreshToken)
	if err != nil || second.RefreshToken == first.RefreshToken || second.AccessToken == "" {
		t.Fatalf("%+v %v", second, err)
	}
	// concurrent loser within the grace window: rejected, grant survives
	clock.Advance(10 * time.Second)
	_, err = refresh(first.RefreshToken)
	wantOAuth(t, err, "invalid_grant")
	if _, err := refresh(second.RefreshToken); err != nil {
		t.Fatalf("grant must survive a grace-window replay: %v", err)
	}
	// replay after the grace window: theft -> grant revoked with its tokens
	list, _ := s.ListConnectedApps(ctx, user)
	clock.Advance(RefreshGrace + time.Second)
	creds.locked = nil
	_, err = refresh(second.RefreshToken) // second was rotated away by the refresh above
	wantOAuth(t, err, "invalid_grant")
	if l, _ := s.ListConnectedApps(ctx, user); len(l) != 0 {
		t.Fatal("theft must revoke the grant")
	}
	if len(creds.revoked) != 1 || creds.revoked[0].String() != list[0].ID {
		t.Fatalf("revoked tokens of %v", creds.revoked)
	}
	if len(creds.locked) != 1 || !creds.locked[0].Equal(user) {
		t.Fatalf("theft revoke must take the user row lock first: %v", creds.locked)
	}
}

func TestRefresh_ExpiryAndClientBinding(t *testing.T) {
	s, _, clock, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, _ := exchange(s, cid, code)
	other, _ := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://a.test/cb"}})
	_, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: other.ClientID, RefreshToken: tr.RefreshToken})
	wantOAuth(t, err, "invalid_grant")
	clock.Advance(GrantIdleTTL + time.Hour)
	_, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken})
	wantOAuth(t, err, "invalid_grant")
}

func TestToken_ClientAuth(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, _ := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}, TokenEndpointAuthMethod: "client_secret_post"})
	res, _ := s.ApproveAuthorization(ctx, user, authReq(c.ClientID))
	u, _ := url.Parse(res.RedirectURL)
	req := model.TokenRequest{GrantType: "authorization_code", ClientID: c.ClientID, Code: u.Query().Get("code"),
		RedirectURI: "https://claude.ai/api/mcp/auth_callback", CodeVerifier: verifier, ClientSecret: "nope"}
	_, err := s.Token(ctx, req)
	wantOAuth(t, err, "invalid_client")
	var oe *OAuthError
	errors.As(err, &oe)
	if oe.Status != 401 {
		t.Fatal("invalid_client is 401")
	}
	_, err = s.Token(ctx, model.TokenRequest{GrantType: "password"})
	wantOAuth(t, err, "unsupported_grant_type")
}

func TestToken_ConfidentialClientSucceeds(t *testing.T) {
	s, _, _, user := newTestService(t)
	c, err := s.Register(ctx, model.ClientRegistrationRequest{RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}, TokenEndpointAuthMethod: "client_secret_post"})
	if err != nil || c.ClientSecret == "" {
		t.Fatalf("%+v %v", c, err)
	}
	res, _ := s.ApproveAuthorization(ctx, user, authReq(c.ClientID))
	u, _ := url.Parse(res.RedirectURL)
	tr, err := s.Token(ctx, model.TokenRequest{GrantType: "authorization_code", ClientID: c.ClientID, ClientSecret: c.ClientSecret,
		Code: u.Query().Get("code"), RedirectURI: "https://claude.ai/api/mcp/auth_callback", CodeVerifier: verifier})
	if err != nil || tr.RefreshToken == "" {
		t.Fatalf("%+v %v", tr, err)
	}
	_, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: c.ClientID, RefreshToken: tr.RefreshToken})
	wantOAuth(t, err, "invalid_client") // refresh needs the secret too
	if _, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: c.ClientID, ClientSecret: c.ClientSecret, RefreshToken: tr.RefreshToken}); err != nil {
		t.Fatal(err)
	}
}

func TestToken_ParameterErrors(t *testing.T) {
	s, _, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	full := model.TokenRequest{GrantType: "authorization_code", ClientID: cid, Code: code, RedirectURI: "https://claude.ai/api/mcp/auth_callback", CodeVerifier: verifier}
	for name, m := range map[string]func(*model.TokenRequest){
		"code":          func(r *model.TokenRequest) { r.Code = "" },
		"redirect_uri":  func(r *model.TokenRequest) { r.RedirectURI = "" },
		"code_verifier": func(r *model.TokenRequest) { r.CodeVerifier = "" },
	} {
		req := full
		m(&req)
		_, err := s.Token(ctx, req)
		wantOAuth(t, err, "invalid_request")
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// None of the rejections above touched the store: the code is still good.
	if _, err := s.Token(ctx, full); err != nil {
		t.Fatalf("a malformed request must not burn the code: %v", err)
	}
	req := full
	req.ClientID = ""
	_, err := s.Token(ctx, req)
	wantOAuth(t, err, "invalid_client")
	_, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid})
	wantOAuth(t, err, "invalid_request")
}

func TestRefresh_ResourceAndScope(t *testing.T) {
	s, _, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, _ := exchange(s, cid, code)
	_, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken, Resource: "https://other.test/mcp"})
	wantOAuth(t, err, "invalid_target")
	_, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken, Scope: "admin"})
	wantOAuth(t, err, "invalid_scope")
	// The refusals above leave the token usable, and a canonical resource/scope passes.
	if _, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken, Resource: testURL + "/mcp/", Scope: "mcp"}); err != nil {
		t.Fatal(err)
	}
}

func TestRefresh_RevokedByReclaim(t *testing.T) {
	s, _, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, _ := exchange(s, cid, code)
	if n, err := s.RevokeAllForUser(ctx, user); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	_, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken})
	wantOAuth(t, err, "invalid_grant")
}

func TestRefresh_FenceMissLeavesGrantUnrotated(t *testing.T) {
	s, creds, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, _ := exchange(s, cid, code)
	issued := creds.issued
	creds.failFence = true // a reclaim landed after the grant was read
	_, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken})
	wantOAuth(t, err, "invalid_grant")
	if creds.issued != issued {
		t.Fatal("a fenced refresh must not mint")
	}
	creds.failFence = false
	// The failed attempt rolled back its rotation: the presented token is still current.
	if _, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken}); err != nil {
		t.Fatalf("grant must not have been rotated: %v", err)
	}
}
