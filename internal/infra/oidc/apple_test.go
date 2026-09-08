package oidc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestAppleClientSecret(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	parsed, err := ParseApplePrivateKey(pemText)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	jwt, err := AppleClientSecret("TEAM123456", "com.example.web", "KEY1234567", parsed, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("not a compact JWS: %s", jwt)
	}
	var header map[string]any
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(hb, &header)
	if header["alg"] != "ES256" || header["kid"] != "KEY1234567" {
		t.Fatalf("header %v", header)
	}
	var claims map[string]any
	pb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(pb, &claims)
	if claims["iss"] != "TEAM123456" || claims["sub"] != "com.example.web" || claims["aud"] != "https://appleid.apple.com" ||
		claims["exp"].(float64) != float64(now.Add(5*time.Minute).Unix()) {
		t.Fatalf("claims %v", claims)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if len(sig) != 64 {
		t.Fatalf("ES256 signature must be raw r||s (64 bytes), got %d", len(sig))
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key.PublicKey, h[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature does not verify")
	}
	if _, err := ParseApplePrivateKey("garbage"); err == nil {
		t.Fatal("garbage PEM must fail")
	}

	badDER := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("not a valid DER payload")})
	if _, err := ParseApplePrivateKey(string(badDER)); err == nil {
		t.Fatal("PEM with invalid DER must fail")
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaDER, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	rsaPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaDER})
	if _, err := ParseApplePrivateKey(string(rsaPEM)); err == nil {
		t.Fatal("a non-ECDSA key must fail")
	}
}

func TestParseApplePrivateKey_RejectsNonP256Curve(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	_, err := ParseApplePrivateKey(pemText)
	if err == nil || !strings.Contains(err.Error(), "P-256") {
		t.Fatalf("a P-384 key must be rejected, got %v", err)
	}
}
