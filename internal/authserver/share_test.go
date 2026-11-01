package authserver

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	authrepo "github.com/econumo/econumo/internal/authserver/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	shareRedirect = "https://share.example.test/auth/callback"
	shareSecret   = "share-secret-0123456789abcdef0123456789"
)

var shareID = vo.MustParseId("e1000000-0000-0000-0000-0000000000a1")

type shareEnv struct {
	db    *dbtest.DB
	s     *Service
	creds *fakeCreds
	clock *fixedClock
	user  vo.Id
}

func newShareEnv(t *testing.T) *shareEnv {
	t.Helper()
	db := dbtest.New(t)
	e := &shareEnv{db: db, user: vo.MustParseId(fixture.New(t, db).User(fixture.User{}))}
	e.s, e.creds, e.clock = e.service(testURL)
	if err := e.s.ConfigureShareClient(ctx, ShareClient{ID: shareID, Secret: shareSecret, RedirectURI: shareRedirect}); err != nil {
		t.Fatal(err)
	}
	return e
}

// service builds another Service over the same database, with no share client.
func (e *shareEnv) service(appURL string) (*Service, *fakeCreds, *fixedClock) {
	creds := &fakeCreds{}
	clock := &fixedClock{t: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	return NewService(authrepo.NewRepo(e.db.Engine, e.db.TX), creds, e.db.TX, clock, nil, appURL), creds, clock
}

func shareAuthReq(clientID string) model.AuthorizationRequest {
	return model.AuthorizationRequest{
		ClientID: clientID, RedirectURI: shareRedirect, ResponseType: "code",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		Scope: "mcp", State: "s1",
	}
}

func (e *shareEnv) approveShare(t *testing.T, s *Service) string {
	t.Helper()
	res, err := s.ApproveAuthorization(ctx, e.user, session, true, shareAuthReq(shareID.String()))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	code := u.Query().Get("code")
	if code == "" {
		t.Fatalf("no code in %s", res.RedirectURL)
	}
	return code
}

func shareExchange(s *Service, code, secret string) (model.TokenResponse, error) {
	return s.Token(ctx, model.TokenRequest{GrantType: "authorization_code", ClientID: shareID.String(), ClientSecret: secret,
		Code: code, RedirectURI: shareRedirect, CodeVerifier: verifier})
}

func TestConfigureShareClient_RequiresEnabledServer(t *testing.T) {
	e := newShareEnv(t)
	s, _, _ := e.service("http://app.example.test")
	err := s.ConfigureShareClient(ctx, ShareClient{ID: shareID, Secret: shareSecret, RedirectURI: shareRedirect})
	if err == nil || !strings.Contains(err.Error(), "ECONUMO_URL") {
		t.Fatalf("err = %v", err)
	}
	if s.ShareClientEnabled() {
		t.Fatal("a refused share client must stay disabled")
	}
}

func TestConfigureShareClient_RejectsBadRedirect(t *testing.T) {
	e := newShareEnv(t)
	s, _, _ := e.service(testURL)
	if err := s.ConfigureShareClient(ctx, ShareClient{ID: shareID, Secret: shareSecret, RedirectURI: "http://share.example.test/cb"}); err == nil {
		t.Fatal("plain-http redirect must be refused")
	}
	if s.ShareClientEnabled() {
		t.Fatal("a refused share client must stay disabled")
	}
}

func TestConfigureShareClient_Upserts(t *testing.T) {
	e := newShareEnv(t)
	if !e.s.ShareClientEnabled() {
		t.Fatal("configured share client must be enabled")
	}
	const newRedirect = "https://share2.example.test/auth/callback"
	const newSecret = "another-share-secret-0123456789abcdef"
	if err := e.s.ConfigureShareClient(ctx, ShareClient{ID: shareID, Secret: newSecret, RedirectURI: newRedirect}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT COUNT(*) FROM oauth_clients WHERE first_party = ?`), true).Scan(&n); err != nil || n != 1 {
		t.Fatalf("first-party rows = %d, %v", n, err)
	}
	c, err := e.s.repo.GetClient(ctx, shareID)
	if err != nil {
		t.Fatal(err)
	}
	if !c.FirstParty || c.Name != ShareClientName || c.SecretHash == nil || *c.SecretHash != hashSecret(newSecret) ||
		len(c.RedirectURIs) != 1 || c.RedirectURIs[0] != newRedirect || c.LastUsedAt == nil {
		t.Fatalf("client = %+v", c)
	}
}

func TestShareClientFlow_GrantsRead(t *testing.T) {
	e := newShareEnv(t)
	tr, err := shareExchange(e.s, e.approveShare(t, e.s), shareSecret)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Scope != "read" || e.creds.scopes[tr.AccessToken] != model.TokenScopeRead {
		t.Fatalf("scope = %q, token scope %q", tr.Scope, e.creds.scopes[tr.AccessToken])
	}
	g, err := e.s.repo.GetGrantByRefreshHash(ctx, hashSecret(tr.RefreshToken))
	if err != nil || g.Scope != model.TokenScopeRead {
		t.Fatalf("grant = %+v, %v", g, err)
	}
}

func TestShareClientRequiresSecret(t *testing.T) {
	e := newShareEnv(t)
	for _, secret := range []string{"", "wrong-secret"} {
		_, err := shareExchange(e.s, e.approveShare(t, e.s), secret)
		wantOAuth(t, err, "invalid_client")
		if oe := err.(*OAuthError); oe.Status != 401 {
			t.Fatalf("status = %d", oe.Status)
		}
	}
}

func TestShareClientRefreshKeepsReadScope(t *testing.T) {
	e := newShareEnv(t)
	tr, err := shareExchange(e.s, e.approveShare(t, e.s), shareSecret)
	if err != nil {
		t.Fatal(err)
	}
	refresh := tr.RefreshToken
	for i := range 3 {
		e.clock.Advance(time.Minute)
		tr, err = e.s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: shareID.String(), ClientSecret: shareSecret, RefreshToken: refresh})
		if err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
		if tr.Scope != "read" || e.creds.scopes[tr.AccessToken] != model.TokenScopeRead {
			t.Fatalf("refresh %d: scope = %q, token scope %q", i, tr.Scope, e.creds.scopes[tr.AccessToken])
		}
		refresh = tr.RefreshToken
	}
}

func TestDynamicClientAskingReadGetsMCP(t *testing.T) {
	e := newShareEnv(t)
	c, err := e.s.Register(ctx, model.ClientRegistrationRequest{ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}})
	if err != nil {
		t.Fatal(err)
	}
	req := authReq(c.ClientID)
	req.Scope = "read"
	res, err := e.s.ApproveAuthorization(ctx, e.user, session, true, req)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	tr, err := e.s.Token(ctx, model.TokenRequest{GrantType: "authorization_code", ClientID: c.ClientID, Code: u.Query().Get("code"),
		RedirectURI: req.RedirectURI, CodeVerifier: verifier, Scope: "read"})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Scope != "mcp" || e.creds.scopes[tr.AccessToken] != model.TokenScopeMCP {
		t.Fatalf("scope = %q, token scope %q", tr.Scope, e.creds.scopes[tr.AccessToken])
	}
}

// A first-party row left behind after the share config is removed or its id
// changes must not authorize, exchange or refresh: its secret would otherwise
// buy an mcp grant.
func TestStaleFirstPartyClientIsUnknown(t *testing.T) {
	e := newShareEnv(t)
	// A live code and refresh token for the share client, minted while it was configured.
	code := e.approveShare(t, e.s)
	tr, err := shareExchange(e.s, e.approveShare(t, e.s), shareSecret)
	if err != nil {
		t.Fatal(err)
	}

	other, _, _ := e.service(testURL)
	if err := other.ConfigureShareClient(ctx, ShareClient{ID: vo.MustParseId("e1000000-0000-0000-0000-0000000000b2"), Secret: shareSecret, RedirectURI: shareRedirect}); err != nil {
		t.Fatal(err)
	}
	none, _, _ := e.service(testURL)
	for name, s := range map[string]*Service{"other share client": other, "no share client": none} {
		if _, err := s.DescribeAuthorization(ctx, e.user, shareAuthReq(shareID.String())); !hasCode(err, errs.CodeAuthServerClientNotFound) {
			t.Errorf("%s: describe err = %v", name, err)
		}
		if _, err := s.ApproveAuthorization(ctx, e.user, session, true, shareAuthReq(shareID.String())); !hasCode(err, errs.CodeAuthServerClientNotFound) {
			t.Errorf("%s: approve err = %v", name, err)
		}
		_, err := shareExchange(s, code, shareSecret)
		wantOAuth(t, err, "invalid_client")
		_, err = s.Token(ctx, model.TokenRequest{GrantType: "refresh_token", ClientID: shareID.String(), ClientSecret: shareSecret, RefreshToken: tr.RefreshToken})
		wantOAuth(t, err, "invalid_client")
	}
}

func TestDescribeAuthorization_ReportsScope(t *testing.T) {
	e := newShareEnv(t)
	res, err := e.s.DescribeAuthorization(ctx, e.user, shareAuthReq(shareID.String()))
	if err != nil || res.Scope != "read" || !res.IsFirstParty {
		t.Fatalf("share client: %+v %v", res, err)
	}
	c := register(t, e.s, "Claude", "https://claude.ai/api/mcp/auth_callback")
	res, err = e.s.DescribeAuthorization(ctx, e.user, authReq(c.ClientID))
	if err != nil || res.Scope != "mcp" || res.IsFirstParty {
		t.Fatalf("dynamic client: %+v %v", res, err)
	}
}

func TestApprove_ReadonlyMayApproveRead(t *testing.T) {
	e := newShareEnv(t)
	res, err := e.s.ApproveAuthorization(ctx, e.user, session, false, shareAuthReq(shareID.String()))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.RedirectURL)
	if u.Query().Get("code") == "" {
		t.Fatalf("no code in %s", res.RedirectURL)
	}
}

func TestApprove_ReadonlyRefusedForMCP(t *testing.T) {
	e := newShareEnv(t)
	c := register(t, e.s, "Claude", "https://claude.ai/api/mcp/auth_callback")
	_, err := e.s.ApproveAuthorization(ctx, e.user, session, false, authReq(c.ClientID))
	var pe *errs.PaymentRequiredError
	if !errors.As(err, &pe) || pe.Code != errs.CodeReadonlyAccess {
		t.Fatalf("err = %v", err)
	}
	var n int
	if err := e.db.Raw.QueryRow(`SELECT COUNT(*) FROM oauth_authorization_codes`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("codes = %d %v", n, err)
	}
}
