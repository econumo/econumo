package oidc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
)

func discoveryServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func issuerFor(url string) oidc.Issuer {
	return oidc.Issuer{IssuerURL: url, ClientID: "cid", ClientSecret: oidc.StaticSecret("secret"), Scopes: []string{"openid"}, UsePKCE: true}
}

func TestDiscover_Errors(t *testing.T) {
	ctx := context.Background()

	badStatus := discoveryServer(t, `{}`, http.StatusInternalServerError)
	if _, err := oidc.NewClient(issuerFor(badStatus.URL), nil).Discover(ctx); err == nil {
		t.Error("non-200 discovery must fail")
	}

	badJSON := discoveryServer(t, `not json`, http.StatusOK)
	if _, err := oidc.NewClient(issuerFor(badJSON.URL), nil).Discover(ctx); err == nil {
		t.Error("malformed discovery JSON must fail")
	}

	missingFields := discoveryServer(t, `{"issuer":"x"}`, http.StatusOK)
	if _, err := oidc.NewClient(issuerFor(missingFields.URL), nil).Discover(ctx); err == nil {
		t.Error("discovery missing endpoints must fail")
	}

	unreachable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachable.Close()
	if _, err := oidc.NewClient(issuerFor(unreachable.URL), nil).Discover(ctx); err == nil {
		t.Error("unreachable issuer must fail")
	}

	if _, err := oidc.NewClient(issuerFor("http://example.test/\ninvalid"), nil).Discover(ctx); err == nil {
		t.Error("invalid issuer URL must fail")
	}
}

func TestAuthURL_DiscoverErrorAndQuerySeparator(t *testing.T) {
	ctx := context.Background()
	badStatus := discoveryServer(t, `{}`, http.StatusInternalServerError)
	if _, err := oidc.NewClient(issuerFor(badStatus.URL), nil).AuthURL(ctx, "s", "n", "c", "https://a/cb"); err == nil {
		t.Error("discover error must propagate")
	}

	withQuery := discoveryServer(t, "", http.StatusOK)
	// Replace with a body carrying an authorization_endpoint that already has a query string.
	withQuery.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"` + withQuery.URL + `","authorization_endpoint":"` + withQuery.URL + `/authorize?foo=bar","token_endpoint":"` + withQuery.URL + `/token","jwks_uri":"` + withQuery.URL + `/jwks"}`))
	})
	raw, err := oidc.NewClient(issuerFor(withQuery.URL), nil).AuthURL(ctx, "s", "n", "c", "https://a/cb")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "?foo=bar&") {
		t.Fatalf("expected '&' separator, got %s", raw)
	}
}

func TestExchange_Errors(t *testing.T) {
	ctx := context.Background()

	badStatus := discoveryServer(t, `{}`, http.StatusInternalServerError)
	if _, err := oidc.NewClient(issuerFor(badStatus.URL), nil).Exchange(ctx, "code", "v", "https://a/cb", time.Now()); err == nil {
		t.Error("discover error must propagate")
	}

	good := discoveryServer(t, "", http.StatusOK)
	good.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_, _ = w.Write([]byte(`{"issuer":"` + good.URL + `","authorization_endpoint":"` + good.URL + `/authorize","token_endpoint":"` + good.URL + `/token","jwks_uri":"` + good.URL + `/jwks"}`))
		case "/token":
			_, _ = w.Write([]byte(`not json`))
		}
	})
	iss := issuerFor(good.URL)
	if _, err := oidc.NewClient(iss, nil).Exchange(ctx, "code", "v", "https://a/cb", time.Now()); err == nil {
		t.Error("malformed token response must fail")
	}

	failingSecret := issuerFor(good.URL)
	failingSecret.ClientSecret = func(time.Time) (string, error) { return "", errInjected }
	if _, err := oidc.NewClient(failingSecret, nil).Exchange(ctx, "code", "v", "https://a/cb", time.Now()); err == nil {
		t.Error("client secret error must propagate")
	}

	noIDToken := discoveryServer(t, "", http.StatusOK)
	noIDToken.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_, _ = w.Write([]byte(`{"issuer":"` + noIDToken.URL + `","authorization_endpoint":"` + noIDToken.URL + `/authorize","token_endpoint":"` + noIDToken.URL + `/token","jwks_uri":"` + noIDToken.URL + `/jwks"}`))
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"at"}`))
		}
	})
	if _, err := oidc.NewClient(issuerFor(noIDToken.URL), nil).Exchange(ctx, "code", "v", "https://a/cb", time.Now()); err == nil {
		t.Error("missing id_token must fail")
	}

	badTokenURL := discoveryServer(t, "", http.StatusOK)
	badTokenURL.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"` + badTokenURL.URL + `","authorization_endpoint":"` + badTokenURL.URL + `/authorize","token_endpoint":"http://example.test/\ninvalid","jwks_uri":"` + badTokenURL.URL + `/jwks"}`))
	})
	if _, err := oidc.NewClient(issuerFor(badTokenURL.URL), nil).Exchange(ctx, "code", "v", "https://a/cb", time.Now()); err == nil {
		t.Error("invalid token endpoint URL must fail")
	}
}

var errInjected = errTest("injected")

type errTest string

func (e errTest) Error() string { return string(e) }

func TestUserInfo_Errors(t *testing.T) {
	ctx := context.Background()

	badStatus := discoveryServer(t, `{}`, http.StatusInternalServerError)
	if _, err := oidc.NewClient(issuerFor(badStatus.URL), nil).UserInfo(ctx, "at"); err == nil {
		t.Error("discover error must propagate")
	}

	noUserInfo := discoveryServer(t, "", http.StatusOK)
	noUserInfo.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"` + noUserInfo.URL + `","authorization_endpoint":"` + noUserInfo.URL + `/authorize","token_endpoint":"` + noUserInfo.URL + `/token","jwks_uri":"` + noUserInfo.URL + `/jwks"}`))
	})
	if _, err := oidc.NewClient(issuerFor(noUserInfo.URL), nil).UserInfo(ctx, "at"); err == nil {
		t.Error("missing userinfo endpoint must fail")
	}

	unauthorized := discoveryServer(t, "", http.StatusOK)
	unauthorized.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_, _ = w.Write([]byte(`{"issuer":"` + unauthorized.URL + `","authorization_endpoint":"` + unauthorized.URL + `/authorize","token_endpoint":"` + unauthorized.URL + `/token","jwks_uri":"` + unauthorized.URL + `/jwks","userinfo_endpoint":"` + unauthorized.URL + `/userinfo"}`))
		case "/userinfo":
			w.WriteHeader(http.StatusUnauthorized)
		}
	})
	if _, err := oidc.NewClient(issuerFor(unauthorized.URL), nil).UserInfo(ctx, "at"); err == nil {
		t.Error("non-200 userinfo must fail")
	}

	badJSON := discoveryServer(t, "", http.StatusOK)
	badJSON.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_, _ = w.Write([]byte(`{"issuer":"` + badJSON.URL + `","authorization_endpoint":"` + badJSON.URL + `/authorize","token_endpoint":"` + badJSON.URL + `/token","jwks_uri":"` + badJSON.URL + `/jwks","userinfo_endpoint":"` + badJSON.URL + `/userinfo"}`))
		case "/userinfo":
			_, _ = w.Write([]byte(`not json`))
		}
	})
	if _, err := oidc.NewClient(issuerFor(badJSON.URL), nil).UserInfo(ctx, "at"); err == nil {
		t.Error("malformed userinfo JSON must fail")
	}
}

func TestEndSessionURL_Errors(t *testing.T) {
	ctx := context.Background()
	badStatus := discoveryServer(t, `{}`, http.StatusInternalServerError)
	if _, err := oidc.NewClient(issuerFor(badStatus.URL), nil).EndSessionURL(ctx, "idt", "https://a/"); err == nil {
		t.Error("discover error must propagate")
	}

	withQuery := discoveryServer(t, "", http.StatusOK)
	withQuery.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"` + withQuery.URL + `","authorization_endpoint":"` + withQuery.URL + `/authorize","token_endpoint":"` + withQuery.URL + `/token","jwks_uri":"` + withQuery.URL + `/jwks","end_session_endpoint":"` + withQuery.URL + `/end-session?foo=bar"}`))
	})
	raw, err := oidc.NewClient(issuerFor(withQuery.URL), nil).EndSessionURL(ctx, "idt", "https://a/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "?foo=bar&") {
		t.Fatalf("expected '&' separator, got %s", raw)
	}
}

func TestClient_IssuerGetter(t *testing.T) {
	c := oidc.NewClient(issuerFor("https://issuer.example.test/"), nil)
	if c.Issuer().IssuerURL != "https://issuer.example.test" {
		t.Fatalf("Issuer() = %+v", c.Issuer())
	}
}
