package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const testReferralCookie = "affonso_referral"

func affiliateOptionValue(t *testing.T, db *dbtest.DB, email string) (string, bool) {
	t.Helper()
	var v *string
	err := db.Raw.QueryRow(db.Rebind(`SELECT o.value FROM users_options o JOIN users u ON u.id = o.user_id WHERE u.email = ? AND o.name = ?`),
		model.NormalizeEmail(email), model.OptionAffiliate).Scan(&v)
	if err != nil {
		return "", false
	}
	if v == nil {
		return "", true
	}
	return *v, true
}

func affiliateConfig(db *dbtest.DB, cookie, billing string) config.Config {
	return config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AllowRegistration: true,
		AppURL: "https://app.example.test", RateLimitWindow: 15 * time.Minute, RateLimitIP: 60,
		ReferralCookie: cookie, BillingURL: billing}
}

func registerWithCookie(t *testing.T, srv *httptest.Server, email, cookieValue string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/user/register-user",
		strings.NewReader(`{"name":"Aff","email":"`+email+`","password":"secretpass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: testReferralCookie, Value: cookieValue})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("register: %d %s", resp.StatusCode, body)
	}
}

func TestRegisterThroughRouterStoresAffiliate(t *testing.T) {
	db := dbtest.NewSQLite(t)
	srv := httptest.NewServer(BuildAPI(affiliateConfig(db, testReferralCookie, "https://billing.example.test"), db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	registerWithCookie(t, srv, "aff-ok@example.test", "abc123")
	if v, ok := affiliateOptionValue(t, db, "aff-ok@example.test"); !ok || v != "abc123" {
		t.Fatalf("affiliate = %q (present %v), want abc123", v, ok)
	}
}

func TestRegisterWithMalformedAffiliateCookie(t *testing.T) {
	db := dbtest.NewSQLite(t)
	srv := httptest.NewServer(BuildAPI(affiliateConfig(db, testReferralCookie, "https://billing.example.test"), db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	registerWithCookie(t, srv, "aff-bad@example.test", "a b")
	if _, ok := affiliateOptionValue(t, db, "aff-bad@example.test"); ok {
		t.Fatal("a malformed cookie must store nothing")
	}
}

func TestRouterAffiliateGate(t *testing.T) {
	for name, c := range map[string][2]string{
		"cookie only":  {testReferralCookie, ""},
		"billing only": {"", "https://billing.example.test"},
	} {
		t.Run(name, func(t *testing.T) {
			db := dbtest.NewSQLite(t)
			srv := httptest.NewServer(BuildAPI(affiliateConfig(db, c[0], c[1]), db.Raw, Seams{}))
			t.Cleanup(srv.Close)
			registerWithCookie(t, srv, "aff-gate@example.test", "abc123")
			if _, ok := affiliateOptionValue(t, db, "aff-gate@example.test"); ok {
				t.Fatal("the cookie must be ignored unless both settings are present")
			}
		})
	}
}

// signInWithCookie drives one OIDC sign-in whose callback request carries the
// referral cookie, as the SameSite=None cookie does on a cross-site POST.
func signInWithCookie(t *testing.T, db *dbtest.DB, email string) {
	t.Helper()
	f := oidctest.New(t)
	f.Email, f.EmailVerified = email, true
	injected := []appoauth.Provider{{Name: "SSO", Client: oidc.NewClient(f.Issuer(model.OAuthProviderOIDC, false), f.Server.Client())}}
	srv := httptest.NewServer(BuildAPI(affiliateConfig(db, testReferralCookie, "https://billing.example.test"), db.Raw, Seams{OAuthProviders: injected}))
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/v1/oauth/start-login", "application/json", strings.NewReader(`{"provider":"oidc","client":"web"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env struct {
		Data struct{ Url string } `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(env.Data.Url)
	if err != nil {
		t.Fatal(err)
	}
	q := authorize.Query()
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/oauth/callback-oidc?code="+url.QueryEscape(f.IssueCode(q.Get("nonce"), q.Get("code_challenge")))+"&state="+url.QueryEscape(q.Get("state")), nil)
	req.AddCookie(&http.Cookie{Name: testReferralCookie, Value: "abc123"})
	cb, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cb.Body.Close()
	if loc := cb.Header.Get("Location"); cb.StatusCode != http.StatusFound || !strings.Contains(loc, "#handoff=") {
		t.Fatalf("callback: %d %s", cb.StatusCode, loc)
	}
}

func TestOAuthCallbackStoresAffiliateOnNewUser(t *testing.T) {
	db := dbtest.NewSQLite(t)
	signInWithCookie(t, db, "sso-new@example.test")
	if v, ok := affiliateOptionValue(t, db, "sso-new@example.test"); !ok || v != "abc123" {
		t.Fatalf("affiliate = %q (present %v), want abc123", v, ok)
	}
}

func TestAffiliateNotWrittenOnExistingUserSignIn(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fixture.New(t, db).User(fixture.User{Email: "sso-old@example.test", Algorithm: "none", Password: "-"})
	signInWithCookie(t, db, "sso-old@example.test")
	if _, ok := affiliateOptionValue(t, db, "sso-old@example.test"); ok {
		t.Fatal("signing in to an existing account must not attribute it")
	}
}
