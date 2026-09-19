package oidc_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
)

func TestVerifyIDToken_HappyAndRejections(t *testing.T) {
	f := oidctest.New(t)
	now := time.Now()
	c := oidc.NewClient(f.Issuer("oidc", false), nil)
	base := func() map[string]any {
		return map[string]any{
			"iss": f.IssuerURL(), "aud": f.ClientID, "sub": "user-1", "email": "a@example.test",
			"email_verified": true, "name": "Alice", "nonce": "n1",
			"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		}
	}
	claims, err := c.VerifyIDToken(context.Background(), f.SignIDToken(base()), "n1", now)
	if err != nil || claims.Subject != "user-1" || claims.Email != "a@example.test" || !claims.EmailVerified || claims.Name != "Alice" {
		t.Fatalf("claims %+v err %v", claims, err)
	}

	cases := map[string]func(m map[string]any){
		"wrong issuer":   func(m map[string]any) { m["iss"] = "https://evil.example" },
		"wrong audience": func(m map[string]any) { m["aud"] = "other-client" },
		"expired":        func(m map[string]any) { m["exp"] = now.Add(-2 * time.Minute).Unix() },
		"future iat":     func(m map[string]any) { m["iat"] = now.Add(5 * time.Minute).Unix() },
		"bad nonce":      func(m map[string]any) { m["nonce"] = "other" },
	}
	for name, mut := range cases {
		m := base()
		mut(m)
		if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	// Within skew: exp 30s ago is accepted.
	m := base()
	m["exp"] = now.Add(-30 * time.Second).Unix()
	if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now); err != nil {
		t.Fatalf("30s skew must pass: %v", err)
	}
	// aud as an array containing the client id is accepted only when azp
	// names this client (OIDC Core 3.1.3.7 rule 4).
	m = base()
	m["aud"] = []string{"x", f.ClientID}
	m["azp"] = f.ClientID
	if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now); err != nil {
		t.Fatalf("aud array with matching azp must pass: %v", err)
	}
	// The same multi-audience aud without azp must be rejected: a token
	// minted for several clients that does not say which one is authorized
	// could be replayed here.
	m = base()
	m["aud"] = []string{"x", f.ClientID}
	if _, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now); err == nil {
		t.Fatal("aud array without azp must be rejected")
	}
	// email_verified as the string "true" (Apple) is accepted.
	m = base()
	m["email_verified"] = "true"
	cl, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now)
	if err != nil || !cl.EmailVerified {
		t.Fatalf("string email_verified: %+v %v", cl, err)
	}
}

func TestVerifyIDToken_SignatureAndKeyRotation(t *testing.T) {
	f := oidctest.New(t)
	now := time.Now()
	c := oidc.NewClient(f.Issuer("oidc", false), nil)
	claims := map[string]any{"iss": f.IssuerURL(), "aud": f.ClientID, "sub": "s", "nonce": "n", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}
	tok := f.SignIDToken(claims)
	if _, err := c.VerifyIDToken(context.Background(), tok, "n", now); err != nil {
		t.Fatal(err)
	}
	// Tamper with the payload: signature no longer matches.
	if _, err := c.VerifyIDToken(context.Background(), tok[:len(tok)-4]+"AAAA", "n", now); err == nil {
		t.Fatal("tampered signature must fail")
	}
	// Rotate the key: unknown kid triggers one JWKS refresh and succeeds.
	f.RotateKey(t)
	tok2 := f.SignIDToken(claims)
	if _, err := c.VerifyIDToken(context.Background(), tok2, "n", now); err != nil {
		t.Fatalf("after rotation: %v", err)
	}
	// An unsupported alg is rejected even if a key matches.
	if _, err := c.VerifyIDToken(context.Background(), f.SignWithAlg("none", claims), "n", now); err == nil {
		t.Fatal("alg none must fail")
	}
}

// TestVerifyIDToken_AudienceAzpRules covers OIDC Core 3.1.3.7 rules 3-5: the
// client id must be an audience, a multi-audience token must carry azp, and
// any azp present must equal the client id.
func TestVerifyIDToken_AudienceAzpRules(t *testing.T) {
	f := oidctest.New(t)
	now := time.Now()
	c := oidc.NewClient(f.Issuer("oidc", false), nil)
	base := func() map[string]any {
		return map[string]any{
			"iss": f.IssuerURL(), "sub": "user-1", "nonce": "n1",
			"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		}
	}

	cases := []struct {
		name    string
		aud     any
		azp     string
		wantErr bool
	}{
		{"single aud == client", f.ClientID, "", false},
		{"single aud, no azp", f.ClientID, "", false},
		{"single aud, azp == client", f.ClientID, f.ClientID, false},
		{"single aud, azp == other", f.ClientID, "other-client", true},
		{"multi aud containing client, no azp", []string{"x", f.ClientID}, "", true},
		{"multi aud containing client, azp == client", []string{"x", f.ClientID}, f.ClientID, false},
		{"multi aud containing client, azp == other", []string{"x", f.ClientID}, "other-client", true},
		{"aud not containing client", []string{"x", "y"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			m["aud"] = tc.aud
			if tc.azp != "" {
				m["azp"] = tc.azp
			}
			_, err := c.VerifyIDToken(context.Background(), f.SignIDToken(m), "n1", now)
			if tc.wantErr && err == nil {
				t.Fatal("expected rejection, got nil error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected acceptance, got %v", err)
			}
		})
	}
}
