// Package oidctest is an in-process OpenID provider for tests: discovery,
// authorize (recorded, not rendered), token, JWKS, userinfo, end-session.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
)

type pending struct {
	nonce     string
	challenge string
}

type Fake struct {
	Server   *httptest.Server
	ClientID string
	Secret   string

	// Claims placed in every ID token / userinfo response.
	Subject       string
	Email         string
	EmailVerified any // nil = omit the claim; bool; or the string "true"/"false"
	Name          string
	// UserInfoEmail, when set, is returned by /userinfo (the ID token then
	// omits email when OmitEmailInIDToken is true).
	UserInfoEmail      string
	OmitEmailInIDToken bool
	NoUserInfo         bool
	EndSession         bool
	RequirePKCE        bool
	// DiscoveryIssuer overrides the `issuer` field of the discovery document,
	// for testing the client's issuer-consistency check.
	DiscoveryIssuer string
	// PublicURL, when set, is the issuer and endpoint base this fake presents
	// instead of its (per-run, port-dependent) loopback URL. Reach it through
	// Transport().
	PublicURL string
	// The four *Override fields, when non-empty, replace the corresponding
	// discovery document field verbatim (no base-URL prefixing), for testing
	// the client's endpoint-URL validation.
	AuthorizationEndpointOverride string
	TokenEndpointOverride         string
	JWKSOverride                  string
	UserInfoOverride              string

	mu       sync.Mutex
	key      *rsa.PrivateKey
	kid      string
	codes    map[string]pending
	tokens   map[string]bool // issued access tokens
	requests []string
}

func New(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{ClientID: "test-client", Secret: "fake-secret", Subject: "sub-1", Email: "user@example.test",
		EmailVerified: true, Name: "Test User", codes: map[string]pending{}, tokens: map[string]bool{}}
	f.RotateKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.discovery)
	mux.HandleFunc("/authorize", f.record)
	mux.HandleFunc("/token", f.token)
	mux.HandleFunc("/jwks", f.jwks)
	mux.HandleFunc("/userinfo", f.userinfo)
	mux.HandleFunc("/end-session", f.record)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

func (f *Fake) IssuerURL() string {
	if f.PublicURL != "" {
		return f.PublicURL
	}
	return f.Server.URL
}

// Transport routes requests for PublicURL to the in-process server, so a
// client configured with a fixed, deterministic issuer (goldens compare
// byte-for-byte across runs and engines) still talks to this fake.
func (f *Fake) Transport() http.RoundTripper {
	target := f.Server.Listener.Addr().String()
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r2 := r.Clone(r.Context())
		r2.URL.Scheme, r2.URL.Host, r2.Host = "http", target, target
		return http.DefaultTransport.RoundTrip(r2)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (f *Fake) Issuer(id string, trust bool) oidc.Issuer {
	return oidc.Issuer{ID: id, IssuerURL: f.IssuerURL(), ClientID: f.ClientID, ClientSecret: oidc.StaticSecret(f.Secret),
		Scopes: []string{"openid", "profile", "email"}, UsePKCE: true, TrustEmail: trust}
}

func (f *Fake) RotateKey(t testing.TB) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.key = key
	f.kid = "k" + big.NewInt(time.Now().UnixNano()).Text(36)
	f.mu.Unlock()
}

// IssueCode simulates the user consenting: the returned code is bound to the
// nonce and PKCE challenge the authorization request carried.
func (f *Fake) IssueCode(nonce, challenge string) string {
	code, _ := oidc.RandomToken()
	f.mu.Lock()
	f.codes[code] = pending{nonce: nonce, challenge: challenge}
	f.mu.Unlock()
	return code
}

func (f *Fake) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// Claims exposes the claim set a real token/userinfo response would carry,
// for tests that need to hand-craft and re-sign a token (e.g. issuer aliasing).
func (f *Fake) Claims(nonce string) map[string]any { return f.claims(nonce) }

func (f *Fake) claims(nonce string) map[string]any {
	now := time.Now()
	m := map[string]any{"iss": f.IssuerURL(), "aud": f.ClientID, "sub": f.Subject, "nonce": nonce,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()}
	if f.Name != "" {
		m["name"] = f.Name
	}
	if f.Email != "" && !f.OmitEmailInIDToken {
		m["email"] = f.Email
		if f.EmailVerified != nil {
			m["email_verified"] = f.EmailVerified
		}
	}
	return m
}

func (f *Fake) SignIDToken(claims map[string]any) string { return f.SignWithAlg("RS256", claims) }

func (f *Fake) SignWithAlg(alg string, claims map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, _ := json.Marshal(map[string]string{"alg": alg, "kid": f.kid})
	p, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	if alg == "none" {
		return input + "."
	}
	digest := sha256.Sum256([]byte(input))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (f *Fake) note(r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.Path)
	f.mu.Unlock()
}

func (f *Fake) record(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	w.WriteHeader(http.StatusOK)
}

func (f *Fake) discovery(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	base := f.IssuerURL()
	issuer := base
	if f.DiscoveryIssuer != "" {
		issuer = f.DiscoveryIssuer
	}
	d := map[string]string{
		"issuer": issuer, "authorization_endpoint": base + "/authorize",
		"token_endpoint": base + "/token", "jwks_uri": base + "/jwks",
	}
	if f.AuthorizationEndpointOverride != "" {
		d["authorization_endpoint"] = f.AuthorizationEndpointOverride
	}
	if f.TokenEndpointOverride != "" {
		d["token_endpoint"] = f.TokenEndpointOverride
	}
	if f.JWKSOverride != "" {
		d["jwks_uri"] = f.JWKSOverride
	}
	if !f.NoUserInfo {
		d["userinfo_endpoint"] = base + "/userinfo"
	}
	if f.UserInfoOverride != "" {
		d["userinfo_endpoint"] = f.UserInfoOverride
	}
	if f.EndSession {
		d["end_session_endpoint"] = base + "/end-session"
	}
	_ = json.NewEncoder(w).Encode(d)
}

func (f *Fake) jwks(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	f.mu.Lock()
	pub := f.key.PublicKey
	kid := f.kid
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (f *Fake) token(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	_ = r.ParseForm()
	fail := func(code string) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.PostForm.Get("client_id") != f.ClientID || r.PostForm.Get("client_secret") != f.Secret {
		fail("invalid_client")
		return
	}
	f.mu.Lock()
	p, ok := f.codes[r.PostForm.Get("code")]
	delete(f.codes, r.PostForm.Get("code"))
	f.mu.Unlock()
	if !ok {
		fail("invalid_grant")
		return
	}
	if f.RequirePKCE || p.challenge != "" {
		if oidc.PKCEChallenge(r.PostForm.Get("code_verifier")) != p.challenge {
			fail("invalid_grant")
			return
		}
	}
	access, _ := oidc.RandomToken()
	f.mu.Lock()
	f.tokens[access] = true
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": access, "token_type": "Bearer", "id_token": f.SignIDToken(f.claims(p.nonce)),
	})
}

func (f *Fake) userinfo(w http.ResponseWriter, r *http.Request) {
	f.note(r)
	f.mu.Lock()
	ok := f.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	m := map[string]any{"sub": f.Subject, "name": f.Name}
	email := f.UserInfoEmail
	if email == "" {
		email = f.Email
	}
	if email != "" {
		m["email"] = email
		if f.EmailVerified != nil {
			m["email_verified"] = f.EmailVerified
		}
	}
	_ = json.NewEncoder(w).Encode(m)
}
