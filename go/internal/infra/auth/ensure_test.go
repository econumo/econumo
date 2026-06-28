package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEnsureKeypair_GeneratesOnFirstBoot covers the zero-config path: with no
// keys and no passphrase, EnsureKeypair generates a usable RS256 keypair plus a
// persisted passphrase, and the result round-trips through NewJWT.
func TestEnsureKeypair_GeneratesOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "jwt", "private.pem")
	pub := filepath.Join(dir, "jwt", "public.pem")

	pass, err := EnsureKeypair(priv, pub, "")
	if err != nil {
		t.Fatalf("EnsureKeypair: %v", err)
	}
	if pass == "" {
		t.Fatal("expected a generated passphrase")
	}
	for _, p := range []string{priv, pub, filepath.Join(dir, "jwt", ".jwt-passphrase")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}

	// The generated keypair must be in the exact format NewJWT reads, and usable.
	j, err := NewJWT(priv, pub, pass)
	if err != nil {
		t.Fatalf("NewJWT on generated keypair: %v", err)
	}
	tok, err := j.Issue("11111111-1111-1111-1111-111111111111", "u@x.test", time.Now())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := j.Verify(tok); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

// TestEnsureKeypair_Idempotent: a second call must not regenerate (same passphrase,
// same key bytes) so restarts never invalidate issued tokens.
func TestEnsureKeypair_Idempotent(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")

	pass1, err := EnsureKeypair(priv, pub, "")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(priv)

	pass2, err := EnsureKeypair(priv, pub, "")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(priv)

	if pass1 != pass2 {
		t.Errorf("passphrase changed across calls: %q vs %q", pass1, pass2)
	}
	if string(before) != string(after) {
		t.Error("private key was regenerated on the second call")
	}
}

// TestEnsureKeypair_ExplicitPassphrase: when a passphrase is supplied, it is used
// as-is and no passphrase file is written.
func TestEnsureKeypair_ExplicitPassphrase(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")

	pass, err := EnsureKeypair(priv, pub, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if pass != "s3cret" {
		t.Errorf("passphrase = %q, want s3cret", pass)
	}
	if _, err := os.Stat(filepath.Join(dir, ".jwt-passphrase")); !os.IsNotExist(err) {
		t.Error("a passphrase file must NOT be written when JWT_PASSPHRASE is set")
	}
	if _, err := NewJWT(priv, pub, "s3cret"); err != nil {
		t.Fatalf("NewJWT: %v", err)
	}
}
