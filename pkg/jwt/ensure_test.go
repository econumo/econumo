package jwt

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEnsureKeypair_GeneratesOnFirstBoot covers the zero-config path: with no
// keys and no passphrase, EnsureKeypair generates a usable RS256 keypair plus a
// persisted passphrase, reports generated=true, and the result round-trips
// through New.
func TestEnsureKeypair_GeneratesOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "jwt", "private.pem")
	pub := filepath.Join(dir, "jwt", "public.pem")

	pass, generated, err := EnsureKeypair(priv, pub, "", false)
	if err != nil {
		t.Fatalf("EnsureKeypair: %v", err)
	}
	if !generated {
		t.Fatal("expected generated=true on first boot")
	}
	if pass == "" {
		t.Fatal("expected a generated passphrase")
	}
	for _, p := range []string{priv, pub, filepath.Join(dir, "jwt", ".jwt-passphrase")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}

	// The generated keypair must be in the exact format New reads, and usable.
	j, err := New(priv, pub, pass)
	if err != nil {
		t.Fatalf("New on generated keypair: %v", err)
	}
	tok, err := j.Issue("11111111-1111-1111-1111-111111111111", "u@x.test", time.Now())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := j.Verify(tok); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

// TestEnsureKeypair_Idempotent: a second call (force=false) must not regenerate
// (same passphrase, same key bytes, generated=false) so restarts never invalidate
// issued tokens.
func TestEnsureKeypair_Idempotent(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")

	pass1, gen1, err := EnsureKeypair(priv, pub, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !gen1 {
		t.Fatal("expected generated=true on first call")
	}
	before, _ := os.ReadFile(priv)

	pass2, gen2, err := EnsureKeypair(priv, pub, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if gen2 {
		t.Error("expected generated=false when keys already exist")
	}
	after, _ := os.ReadFile(priv)

	if pass1 != pass2 {
		t.Errorf("passphrase changed across calls: %q vs %q", pass1, pass2)
	}
	if string(before) != string(after) {
		t.Error("private key was regenerated on the second call")
	}
}

// TestEnsureKeypair_ForceRegenerates: force=true overwrites an existing keypair
// (new key bytes, generated=true), reusing the persisted passphrase.
func TestEnsureKeypair_ForceRegenerates(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")

	pass1, _, err := EnsureKeypair(priv, pub, "", false)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(priv)

	pass2, generated, err := EnsureKeypair(priv, pub, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Error("expected generated=true with force")
	}
	after, _ := os.ReadFile(priv)

	if pass1 != pass2 {
		t.Errorf("passphrase changed across force regenerate: %q vs %q", pass1, pass2)
	}
	if string(before) == string(after) {
		t.Error("private key was NOT regenerated despite force=true")
	}
	// The regenerated keypair must still load with the (reused) passphrase.
	if _, err := New(priv, pub, pass2); err != nil {
		t.Fatalf("New on force-regenerated keypair: %v", err)
	}
}

// TestEnsureKeypair_ExplicitPassphrase: when a passphrase is supplied, it is used
// as-is and no passphrase file is written.
func TestEnsureKeypair_ExplicitPassphrase(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")

	pass, generated, err := EnsureKeypair(priv, pub, "s3cret", false)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("expected generated=true on first boot")
	}
	if pass != "s3cret" {
		t.Errorf("passphrase = %q, want s3cret", pass)
	}
	if _, err := os.Stat(filepath.Join(dir, ".jwt-passphrase")); !os.IsNotExist(err) {
		t.Error("a passphrase file must NOT be written when JWT_PASSPHRASE is set")
	}
	if _, err := New(priv, pub, "s3cret"); err != nil {
		t.Fatalf("New: %v", err)
	}
}
