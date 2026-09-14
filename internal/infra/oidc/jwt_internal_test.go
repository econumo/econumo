package oidc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func TestParseCompactMalformed(t *testing.T) {
	cases := []string{
		"only-one-part",
		"two.parts",
		"!!!.eyJhIjoxfQ.sig",
		"eyJhIjoxfQ.!!!.sig",
		"eyJhIjoxfQ.eyJhIjoxfQ.!!!",
		b64([]byte("not json")) + "." + b64([]byte(`{"sub":"x"}`)) + ".sig",
		b64([]byte(`{"alg":"RS256"}`)) + "." + b64([]byte("not json")) + ".sig",
	}
	for _, raw := range cases {
		if _, _, _, _, err := parseCompact(raw); err == nil {
			t.Errorf("expected error for %q", raw)
		}
	}
}

func TestVerifySignatureBranches(t *testing.T) {
	if err := verifySignature("HS256", nil, []byte("x"), []byte("y")); err == nil {
		t.Fatal("unsupported alg must fail")
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySignature("ES256", &rsaKey.PublicKey, []byte("x"), make([]byte, 64)); err == nil {
		t.Fatal("ES256 with an RSA key must fail")
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySignature("ES256", &ecKey.PublicKey, []byte("x"), make([]byte, 10)); err == nil {
		t.Fatal("wrong-length ES256 signature must fail")
	}
	if err := verifySignature("RS256", &ecKey.PublicKey, []byte("x"), make([]byte, 10)); err == nil {
		t.Fatal("RS256 with an EC key must fail")
	}
}

func TestAudienceContainsInvalidJSON(t *testing.T) {
	if audienceContains(json.RawMessage(`123`), "client") {
		t.Fatal("a bare number must not match")
	}
}

func TestParseEmailVerifiedBranches(t *testing.T) {
	if parseEmailVerified(nil) {
		t.Fatal("nil raw must be false")
	}
	if parseEmailVerified(json.RawMessage(``)) {
		t.Fatal("empty raw must be false")
	}
	if parseEmailVerified(json.RawMessage(`123`)) {
		t.Fatal("a bare number must be false")
	}
	if !parseEmailVerified(json.RawMessage(`true`)) {
		t.Fatal("bool true must be true")
	}
	if parseEmailVerified(json.RawMessage(`"false"`)) {
		t.Fatal(`string "false" must be false`)
	}
}
