package oidc

import (
	"regexp"
	"testing"
)

func TestRandomTokenShape(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		s, err := RandomToken()
		if err != nil || !re.MatchString(s) {
			t.Fatalf("token %q err %v", s, err)
		}
		if seen[s] {
			t.Fatal("duplicate token")
		}
		seen[s] = true
	}
}

func TestPKCEChallengeRFCVector(t *testing.T) {
	// RFC 7636 appendix B.
	got := PKCEChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge = %s", got)
	}
}

func TestSha256Hex(t *testing.T) {
	got := Sha256Hex("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("Sha256Hex = %s, want %s", got, want)
	}
	if Sha256Hex("hello") != Sha256Hex("hello") {
		t.Fatal("Sha256Hex must be deterministic")
	}
}
