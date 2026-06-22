package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGenerateKeypair_RoundTrip generates a keypair and proves NewJWT can load it
// and issue/verify a token — i.e. the generated formats match what the server reads.
func TestGenerateKeypair_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "jwt", "private.pem") // nested dir: also tests MkdirAll
	pub := filepath.Join(dir, "jwt", "public.pem")
	const pass = "s3cret-pass"

	if err := GenerateKeypair(priv, pub, pass, false); err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	// Private key must not be world/group readable.
	if fi, err := os.Stat(priv); err != nil {
		t.Fatal(err)
	} else if fi.Mode().Perm() != 0o600 {
		t.Errorf("private key mode = %v, want 0600", fi.Mode().Perm())
	}

	j, err := NewJWT(priv, pub, pass)
	if err != nil {
		t.Fatalf("NewJWT on generated keys: %v", err)
	}
	tok, err := j.Issue("user-1", "e@example.test", time.Now())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := j.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.ID != "user-1" || claims.Username != "e@example.test" {
		t.Errorf("claims = %+v, want id=user-1 username=e@example.test", claims)
	}
}

func TestGenerateKeypair_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")

	if err := GenerateKeypair(priv, pub, "pw", false); err != nil {
		t.Fatal(err)
	}
	// Without force -> refuse (don't clobber keys that may be signing live tokens).
	if err := GenerateKeypair(priv, pub, "pw", false); err == nil {
		t.Error("expected refusal when files exist and force=false")
	}
	// With force -> overwrite succeeds.
	if err := GenerateKeypair(priv, pub, "pw", true); err != nil {
		t.Errorf("force overwrite: %v", err)
	}
}

func TestGenerateKeypair_RequiresPassphrase(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateKeypair(filepath.Join(dir, "p.pem"), filepath.Join(dir, "P.pem"), "", true); err == nil {
		t.Error("expected error when passphrase is empty")
	}
}
