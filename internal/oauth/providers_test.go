package oauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
)

func TestProvidersFromConfig(t *testing.T) {
	cfg := config.Config{OAuthGoogleClientID: "g", OAuthGoogleClientSecret: "gs",
		OIDCIssuerURL: "https://auth.example.test", OIDCClientID: "c", OIDCClientSecret: "s", OIDCName: "Authentik",
		OIDCScopes: []string{"openid", "email"}, OIDCTrustEmail: true}
	ps, err := ProvidersFromConfig(cfg, nil)
	if err != nil || len(ps) != 2 || ps[0].Client.Issuer().ID != "google" || ps[1].Client.Issuer().ID != "oidc" || ps[1].Name != "Authentik" {
		t.Fatalf("%+v %v", ps, err)
	}
	g := ps[0].Client.Issuer()
	if g.IssuerURL != "https://accounts.google.com" || !g.TrustEmail || !g.UsePKCE || g.ExtraAuthParams["prompt"] != "select_account" {
		t.Fatalf("google issuer %+v", g)
	}
	o := ps[1].Client.Issuer()
	if !o.TrustEmail || len(o.Scopes) != 2 {
		t.Fatalf("oidc issuer %+v", o)
	}
	cfg.OAuthAppleClientID, cfg.OAuthAppleTeamID, cfg.OAuthAppleKeyID, cfg.OAuthApplePrivateKey = "com.example.web", "TEAM", "KEY", "not a pem"
	if _, err := ProvidersFromConfig(cfg, nil); err == nil {
		t.Fatal("a bad apple key must fail")
	}
}

func TestProvidersFromConfig_AppleEnabled(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	cfg := config.Config{OAuthAppleClientID: "com.example.web", OAuthAppleTeamID: "TEAM", OAuthAppleKeyID: "KEY", OAuthApplePrivateKey: string(pemKey)}
	ps, err := ProvidersFromConfig(cfg, nil)
	if err != nil || len(ps) != 1 || ps[0].Client.Issuer().ID != "apple" || ps[0].Name != "Apple" {
		t.Fatalf("%+v %v", ps, err)
	}
	secret, err := ps[0].Client.Issuer().ClientSecret(time.Now())
	if err != nil || strings.Count(secret, ".") != 2 {
		t.Fatalf("apple client secret %q %v", secret, err)
	}
}
