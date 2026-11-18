package server

import (
	"database/sql"
	"encoding/json"
	"errors"
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

var testAffiliates = []model.AffiliateCookie{{Program: "affonso", Cookie: testReferralCookie}}

func optionValue(t *testing.T, db *dbtest.DB, email, name string) (string, bool) {
	t.Helper()
	var v *string
	err := db.Raw.QueryRow(db.Rebind(`SELECT o.value FROM users_options o JOIN users u ON u.id = o.user_id WHERE u.email = ? AND o.name = ?`),
		model.NormalizeEmail(email), name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		return "", true
	}
	return *v, true
}

func affiliateConfig(db *dbtest.DB, cookies []model.AffiliateCookie, billing string) config.Config {
	return config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AllowRegistration: true,
		AppURL: "https://app.example.test", RateLimitWindow: 15 * time.Minute, RateLimitIP: 60,
		Affiliates: cookies, BillingURL: billing}
}

func registerWithCookie(t *testing.T, srv *httptest.Server, email, cookieValue string) {
	t.Helper()
	registerWithCookies(t, srv, email, &http.Cookie{Name: testReferralCookie, Value: cookieValue})
}

func registerWithCookies(t *testing.T, srv *httptest.Server, email string, cookies ...*http.Cookie) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/user/register-user",
		strings.NewReader(`{"name":"Aff","email":"`+email+`","password":"secretpass"}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
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

func assertAffiliate(t *testing.T, db *dbtest.DB, email string) {
	t.Helper()
	if v, ok := optionValue(t, db, email, model.OptionAffiliate); !ok || v != "affonso" {
		t.Fatalf("affiliate = %q (present %v), want affonso", v, ok)
	}
	if v, ok := optionValue(t, db, email, model.OptionAffiliateCode); !ok || v != "abc123" {
		t.Fatalf("affiliate_code = %q (present %v), want abc123", v, ok)
	}
}

func assertNoAffiliate(t *testing.T, db *dbtest.DB, email string) {
	t.Helper()
	for _, name := range []string{model.OptionAffiliate, model.OptionAffiliateCode} {
		if _, ok := optionValue(t, db, email, name); ok {
			t.Fatalf("option %s must not exist", name)
		}
	}
}

func TestRegisterThroughRouterStoresAffiliate(t *testing.T) {
	db := dbtest.NewSQLite(t)
	srv := httptest.NewServer(BuildAPI(affiliateConfig(db, testAffiliates, "https://billing.example.test"), db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	registerWithCookie(t, srv, "aff-ok@example.test", "abc123")
	assertAffiliate(t, db, "aff-ok@example.test")
}

func TestRegisterPicksTheFirstConfiguredProgram(t *testing.T) {
	db := dbtest.NewSQLite(t)
	two := []model.AffiliateCookie{{Program: "affonso", Cookie: testReferralCookie}, {Program: "other", Cookie: "other_ref"}}
	srv := httptest.NewServer(BuildAPI(affiliateConfig(db, two, "https://billing.example.test"), db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	registerWithCookies(t, srv, "aff-two@example.test", &http.Cookie{Name: "other_ref", Value: "two"}, &http.Cookie{Name: testReferralCookie, Value: "abc123"})
	assertAffiliate(t, db, "aff-two@example.test")
	registerWithCookies(t, srv, "aff-second@example.test", &http.Cookie{Name: "other_ref", Value: "two"})
	if v, _ := optionValue(t, db, "aff-second@example.test", model.OptionAffiliate); v != "other" {
		t.Fatalf("affiliate = %q, want other", v)
	}
}

func TestRegisterWithMalformedAffiliateCookie(t *testing.T) {
	db := dbtest.NewSQLite(t)
	srv := httptest.NewServer(BuildAPI(affiliateConfig(db, testAffiliates, "https://billing.example.test"), db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	registerWithCookie(t, srv, "aff-bad@example.test", "a b")
	assertNoAffiliate(t, db, "aff-bad@example.test")
}

func TestRouterAffiliateGate(t *testing.T) {
	for name, c := range map[string]struct {
		programs []model.AffiliateCookie
		billing  string
	}{
		"cookie only":  {testAffiliates, ""},
		"billing only": {nil, "https://billing.example.test"},
	} {
		t.Run(name, func(t *testing.T) {
			db := dbtest.NewSQLite(t)
			srv := httptest.NewServer(BuildAPI(affiliateConfig(db, c.programs, c.billing), db.Raw, Seams{}))
			t.Cleanup(srv.Close)
			registerWithCookie(t, srv, "aff-gate@example.test", "abc123")
			assertNoAffiliate(t, db, "aff-gate@example.test")
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
	srv := httptest.NewServer(BuildAPI(affiliateConfig(db, testAffiliates, "https://billing.example.test"), db.Raw, Seams{OAuthProviders: injected}))
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
	assertAffiliate(t, db, "sso-new@example.test")
}

func TestAffiliateNotWrittenOnExistingUserSignIn(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fixture.New(t, db).User(fixture.User{Email: "sso-old@example.test", Algorithm: "none", Password: "-"})
	signInWithCookie(t, db, "sso-old@example.test")
	assertNoAffiliate(t, db, "sso-old@example.test")
}
