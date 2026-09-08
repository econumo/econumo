package oidc_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
)

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	h, err := json.Marshal(map[string]string{"alg": "RS256", "kid": kid})
	if err != nil {
		t.Fatal(err)
	}
	p, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func countJWKSRequests(paths []string) int {
	n := 0
	for _, p := range paths {
		if p == "/jwks" {
			n++
		}
	}
	return n
}

func TestVerifyIDToken_ParseErrorAndMissingSub(t *testing.T) {
	f := oidctest.New(t)
	c := oidc.NewClient(f.Issuer("oidc", false), nil)
	now := time.Now()

	if _, err := c.VerifyIDToken(context.Background(), "not-a-jwt", "n", now); err == nil {
		t.Error("malformed compact token must fail")
	}

	claims := map[string]any{"iss": f.IssuerURL(), "aud": f.ClientID, "nonce": "n", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}
	tok := f.SignIDToken(claims)
	if _, err := c.VerifyIDToken(context.Background(), tok, "n", now); err == nil {
		t.Error("missing sub must fail")
	}
}

func TestVerifyIDToken_UnknownKidThrottled(t *testing.T) {
	f := oidctest.New(t)
	c := oidc.NewClient(f.Issuer("oidc", false), nil)
	now := time.Now()

	forged, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]any{"iss": f.IssuerURL(), "aud": f.ClientID, "sub": "s", "nonce": "n", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}
	tok := signToken(t, forged, "never-registered", claims)

	if _, err := c.VerifyIDToken(context.Background(), tok, "n", now); err == nil {
		t.Fatal("a kid that is never in the JWKS must fail")
	}
	afterFirst := countJWKSRequests(f.Requests())
	if afterFirst == 0 {
		t.Fatal("the first unknown kid must trigger a JWKS fetch")
	}

	if _, err := c.VerifyIDToken(context.Background(), tok, "n", now); err == nil {
		t.Fatal("still-unknown kid must keep failing")
	}
	afterSecond := countJWKSRequests(f.Requests())
	if afterSecond != afterFirst {
		t.Fatalf("the same unknown kid within the throttle window must not refetch: %d -> %d", afterFirst, afterSecond)
	}

	// A flood of DISTINCT forged kids must not each trigger their own refetch:
	// the throttle is global to the client (per issuer), not keyed by kid.
	for i := 0; i < 20; i++ {
		flood := signToken(t, forged, fmt.Sprintf("forged-kid-%d", i), claims)
		if _, err := c.VerifyIDToken(context.Background(), flood, "n", now); err == nil {
			t.Fatalf("forged kid %d must fail", i)
		}
	}
	afterFlood := countJWKSRequests(f.Requests())
	if afterFlood != afterFirst {
		t.Fatalf("a flood of distinct unknown kids within the throttle window must not refetch: %d -> %d", afterFirst, afterFlood)
	}
}
