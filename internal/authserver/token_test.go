package authserver

import (
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
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
	res, err := s.ApproveAuthorization(ctx, user, session, authReq(c.ClientID))
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

func TestRefresh_ReplayOfOlderTokenRevokesAtAnyAge(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after time.Duration
	}{{"inside grace", 10 * time.Second}, {"after grace", RefreshGrace + time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			s, creds, clock, user := newTestService(t)
			cid, code := approve(t, s, user)
			r0, _ := exchange(s, cid, code)
			refresh := func(rt string) (model.TokenResponse, error) {
				return s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: rt})
			}
			r1, err := refresh(r0.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			r2, err := refresh(r1.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			list, _ := s.ListConnectedApps(ctx, user)
			clock.Advance(tc.after)
			creds.locked = nil
			_, err = refresh(r0.RefreshToken)
			wantOAuth(t, err, "invalid_grant")
			if l, _ := s.ListConnectedApps(ctx, user); len(l) != 0 {
				t.Fatal("replay of a token two rotations old must revoke the grant")
			}
			if len(creds.revoked) != 1 || creds.revoked[0].String() != list[0].ID {
				t.Fatalf("revoked tokens of %v", creds.revoked)
			}
			if len(creds.locked) != 1 || !creds.locked[0].Equal(user) {
				t.Fatalf("revoke must take the user row lock first: %v", creds.locked)
			}
			_, err = refresh(r2.RefreshToken)
			wantOAuth(t, err, "invalid_grant")
		})
	}
}

func TestRefresh_GraceOnlyForTheLatestRotatedToken(t *testing.T) {
	s, creds, clock, user := newTestService(t)
	cid, code := approve(t, s, user)
	r0, _ := exchange(s, cid, code)
	refresh := func(rt string) (model.TokenResponse, error) {
		return s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: rt})
	}
	r1, err := refresh(r0.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(10 * time.Second)
	_, err = refresh(r0.RefreshToken)
	wantOAuth(t, err, "invalid_grant")
	if len(creds.revoked) != 0 {
		t.Fatal("a grace-window replay of the latest rotated token must not revoke")
	}
	if _, err := refresh(r1.RefreshToken); err != nil {
		t.Fatalf("grant must survive a grace-window replay: %v", err)
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
	res, _ := s.ApproveAuthorization(ctx, user, session, authReq(c.ClientID))
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
	res, _ := s.ApproveAuthorization(ctx, user, session, authReq(c.ClientID))
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
	// The refusal above leaves the token usable; any requested scope is
	// tolerated and the grant stays mcp.
	tr, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken, Resource: testURL + "/mcp/", Scope: "admin"})
	if err != nil || tr.Scope != "mcp" {
		t.Fatalf("%+v %v", tr, err)
	}
	if _, err := s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: tr.RefreshToken, Scope: "mcp"}); err != nil {
		t.Fatal(err)
	}
}

func TestExchange_ReplacesEarlierGrantForSameClient(t *testing.T) {
	s, creds, _, user := newTestService(t)
	cid, code := approve(t, s, user)
	first, err := exchange(s, cid, code)
	if err != nil {
		t.Fatal(err)
	}
	other, otherCode := approve(t, s, user)
	if _, err := exchange(s, other, otherCode); err != nil {
		t.Fatal(err)
	}
	before, _ := s.repo.ListUnrevokedGrants(ctx, user)
	if len(before) != 2 {
		t.Fatalf("two clients, two grants: %d", len(before))
	}
	firstGrant := before[0].Grant.ID
	if !before[0].Grant.ClientID.Equal(vo.MustParseId(cid)) {
		firstGrant = before[1].Grant.ID
	}

	res, err := s.ApproveAuthorization(ctx, user, session, authReq(cid))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	if _, err := exchange(s, cid, u.Query().Get("code")); err != nil {
		t.Fatal(err)
	}
	after, _ := s.repo.ListUnrevokedGrants(ctx, user)
	if len(after) != 2 {
		t.Fatalf("re-authorizing must replace, not add: %d grants", len(after))
	}
	for _, g := range after {
		if g.Grant.ID.Equal(firstGrant) {
			t.Fatal("the earlier grant for the same client must be revoked")
		}
	}
	if len(creds.revoked) != 1 || !creds.revoked[0].Equal(firstGrant) {
		t.Fatalf("the replaced grant's access tokens must be revoked: %v", creds.revoked)
	}
	_, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: cid, RefreshToken: first.RefreshToken})
	wantOAuth(t, err, "invalid_grant")
}

func TestExchange_HousekeepingPurgesDeadOAuthTokens(t *testing.T) {
	s, creds, clock, user := newTestService(t)
	cid, code := approve(t, s, user)
	if _, err := exchange(s, cid, code); err != nil {
		t.Fatal(err)
	}
	if len(creds.purged) != 1 || !creds.purged[0].Equal(clock.Now().Add(-DeadRetention)) {
		t.Fatalf("purge cutoff = %v", creds.purged)
	}
}

func refreshWith(s *Service, clientID, refresh string) (model.TokenResponse, error) {
	return s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: clientID, RefreshToken: refresh})
}

func TestRefresh_HousekeepingPurgesDeadRows(t *testing.T) {
	s, creds, clock, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, err := exchange(s, cid, code)
	if err != nil {
		t.Fatal(err)
	}
	clientID := vo.MustParseId(cid)
	old := clock.Now().Add(-60 * 24 * time.Hour)
	dead := &model.OAuthGrant{ID: vo.NewId(), UserID: user, ClientID: clientID, RefreshTokenHash: "dead-hash",
		CreatedAt: old, LastUsedAt: old, ExpiresAt: old, RevokedAt: &old}
	if err := s.repo.InsertGrant(ctx, dead); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.InsertCode(ctx, &model.OAuthAuthorizationCode{CodeHash: "stale-code", ClientID: clientID, UserID: user,
		RedirectURI: "x", CodeChallenge: "c", Resource: "r", CreatedAt: old, ExpiresAt: old}); err != nil {
		t.Fatal(err)
	}
	clock.Advance(HousekeepingInterval)
	purged := len(creds.purged)
	if _, err := refreshWith(s, cid, tr.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if len(creds.purged) != purged+1 || !creds.purged[purged].Equal(clock.Now().Add(-DeadRetention)) {
		t.Fatalf("a refresh must purge dead oauth tokens: %v", creds.purged)
	}
	if _, err := s.repo.GetGrant(ctx, dead.ID); err == nil {
		t.Fatal("a refresh must purge long-dead grants")
	}
	if _, err := s.repo.ConsumeCode(ctx, "stale-code"); err == nil {
		t.Fatal("a refresh must purge expired codes")
	}
}

func TestHousekeeping_ThrottledToOncePerInterval(t *testing.T) {
	s, creds, clock, user := newTestService(t)
	cid, code := approve(t, s, user)
	tr, err := exchange(s, cid, code)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds.purged) != 1 {
		t.Fatalf("the first call always runs: %d", len(creds.purged))
	}
	clock.Advance(HousekeepingInterval - time.Second)
	tr, err = refreshWith(s, cid, tr.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds.purged) != 1 {
		t.Fatalf("a refresh inside the interval must not run housekeeping again: %d", len(creds.purged))
	}
	clock.Advance(time.Second)
	if _, err := refreshWith(s, cid, tr.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if len(creds.purged) != 2 {
		t.Fatalf("a refresh after the interval must run housekeeping: %d", len(creds.purged))
	}
}

func TestHousekeepingDue_RaceFree(t *testing.T) {
	s, _, clock, _ := newTestService(t)
	var wg sync.WaitGroup
	var ran atomic.Int64
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.housekeepingDue(clock.Now()) {
				ran.Add(1)
			}
		}()
	}
	wg.Wait()
	if ran.Load() != 1 {
		t.Fatalf("concurrent callers inside one interval must elect exactly one runner, got %d", ran.Load())
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
