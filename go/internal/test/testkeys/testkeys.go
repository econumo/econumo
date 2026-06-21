// Package testkeys provides the shared RSA keypair used by tests that need a
// real JWT signer/verifier (auth.NewJWT takes key FILE paths).
//
// The keypair is the repo dev keypair, embedded here so it has a SINGLE home and
// is reachable from any package without fragile "../../../infra/auth/testdata"
// relative paths (which broke the moment a test moved directories). Tests call
// Paths(t) to get on-disk paths to the keys, valid for the duration of the test.
//
// Why embed + write to a temp file (rather than expose the bytes): auth.NewJWT's
// contract is file paths (it mirrors the production config, which points at
// config/jwt/*.pem). Embedding keeps the keys with the Go code; writing them to
// the test's temp dir gives NewJWT the paths it wants, CWD-independently.
package testkeys

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

//go:embed private.pem
var privatePEM []byte

//go:embed public.pem
var publicPEM []byte

// Passphrase is the passphrase the embedded private key is encrypted with (the
// repo dev JWT passphrase). Pass it to auth.NewJWT alongside the paths.
const Passphrase = "d78eedcb16c13bd949ede5d1b8b910cd"

// Paths writes the embedded keypair into the test's temp directory and returns
// the private- and public-key file paths. The temp dir is removed automatically
// when the test ends, so callers need no cleanup.
func Paths(t testing.TB) (privateKeyPath, publicKeyPath string) {
	t.Helper()
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")
	if err := os.WriteFile(priv, privatePEM, 0o600); err != nil {
		t.Fatalf("testkeys: write private key: %v", err)
	}
	if err := os.WriteFile(pub, publicPEM, 0o600); err != nil {
		t.Fatalf("testkeys: write public key: %v", err)
	}
	return priv, pub
}
