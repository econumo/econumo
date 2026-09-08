package oidc_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
)

func TestClient_AuthURLExchangeUserInfoEndSession(t *testing.T) {
	f := oidctest.New(t)
	f.RequirePKCE = true
	f.EndSession = true
	iss := f.Issuer("oidc", false)
	iss.ExtraAuthParams = map[string]string{"prompt": "select_account"}
	c := oidc.NewClient(iss, nil)

	verifier, _ := oidc.RandomToken()
	raw, err := c.AuthURL(context.Background(), "st", "no", oidc.PKCEChallenge(verifier), "https://app.example.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	if !strings.HasPrefix(raw, f.IssuerURL()+"/authorize?") || q.Get("response_type") != "code" || q.Get("client_id") != f.ClientID ||
		q.Get("redirect_uri") != "https://app.example.test/cb" || q.Get("scope") != "openid profile email" || q.Get("state") != "st" ||
		q.Get("nonce") != "no" || q.Get("code_challenge") != oidc.PKCEChallenge(verifier) || q.Get("code_challenge_method") != "S256" ||
		q.Get("prompt") != "select_account" {
		t.Fatalf("auth url %s", raw)
	}

	code := f.IssueCode("no", q.Get("code_challenge"))
	toks, err := c.Exchange(context.Background(), code, verifier, "https://app.example.test/cb", time.Now())
	if err != nil || toks.IDToken == "" || toks.AccessToken == "" {
		t.Fatalf("exchange %+v %v", toks, err)
	}
	if _, err := c.Exchange(context.Background(), code, "wrong-verifier", "https://app.example.test/cb", time.Now()); err == nil {
		t.Fatal("wrong verifier must fail")
	}

	f.UserInfoEmail = "info@example.test"
	info, err := c.UserInfo(context.Background(), toks.AccessToken)
	if err != nil || info.Email != "info@example.test" || info.Subject != f.Subject {
		t.Fatalf("userinfo %+v %v", info, err)
	}

	end, err := c.EndSessionURL(context.Background(), toks.IDToken, "https://app.example.test/login")
	if err != nil {
		t.Fatal(err)
	}
	eu, _ := url.Parse(end)
	if !strings.HasPrefix(end, f.IssuerURL()+"/end-session?") || eu.Query().Get("id_token_hint") != toks.IDToken ||
		eu.Query().Get("client_id") != f.ClientID || eu.Query().Get("post_logout_redirect_uri") != "https://app.example.test/login" {
		t.Fatalf("end session url %s", end)
	}
}

func TestClient_EndSessionEmptyWhenUnsupported(t *testing.T) {
	f := oidctest.New(t) // EndSession false by default
	c := oidc.NewClient(f.Issuer("google", true), nil)
	if u, err := c.EndSessionURL(context.Background(), "x", "https://a/"); err != nil || u != "" {
		t.Fatalf("want empty, got %q %v", u, err)
	}
}

func TestClient_FormPostAndNoPKCE(t *testing.T) {
	f := oidctest.New(t)
	iss := f.Issuer("apple", true)
	iss.UsePKCE = false
	iss.ResponseMode = "form_post"
	iss.Scopes = []string{"name", "email"}
	c := oidc.NewClient(iss, nil)
	raw, _ := c.AuthURL(context.Background(), "s", "n", "", "https://a/cb")
	u, _ := url.Parse(raw)
	if u.Query().Get("response_mode") != "form_post" || u.Query().Has("code_challenge") || u.Query().Get("scope") != "name email" {
		t.Fatalf("apple auth url %s", raw)
	}
}
