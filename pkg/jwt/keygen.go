package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/youmark/pkcs8"
)

// keyBits is the RSA key size for generated JWT keypairs. 4096 matches the
// Symfony/lexik default and the keys this repo ships.
const keyBits = 4096

// GenerateKeypair writes a fresh RS256 JWT keypair in the exact formats NewJWT
// reads: the public key as an SPKI ("PUBLIC KEY") PEM and the private key as an
// encrypted PKCS#8 ("ENCRYPTED PRIVATE KEY", PBES2) PEM sealed with passphrase.
// This is the Go equivalent of `bin/console lexik:jwt:generate-keypair`.
//
// A non-empty passphrase is required (it is the ECONUMO_JWT_PASSPHRASE the server uses to
// decrypt the key). Existing files are left untouched unless force is true, so a
// stray run cannot silently invalidate every issued token. Parent directories are
// created; the private key is written 0600, the public key 0644.
func GenerateKeypair(privatePath, publicPath, passphrase string, force bool) error {
	if passphrase == "" {
		return errors.New("a passphrase is required (set ECONUMO_JWT_PASSPHRASE) to encrypt the private key")
	}
	if !force {
		for _, p := range []string{privatePath, publicPath} {
			if _, err := os.Stat(p); err == nil {
				return fmt.Errorf("%s already exists (use --force to overwrite)", p)
			}
		}
	}

	priv, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return fmt.Errorf("generate rsa key: %w", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}
	// Encrypted PKCS#8 (PBES2) — youmark/pkcs8 uses AES-256-CBC + PBKDF2-SHA256 by
	// default, which Go's crypto/x509 cannot produce but NewJWT (also youmark) reads.
	privDER, err := pkcs8.MarshalPrivateKey(priv, []byte(passphrase), nil)
	if err != nil {
		return fmt.Errorf("marshal encrypted private key: %w", err)
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: privDER})

	if err := writeKeyFile(publicPath, pubPEM, 0o644); err != nil {
		return err
	}
	if err := writeKeyFile(privatePath, privPEM, 0o600); err != nil {
		return err
	}
	return nil
}

// writeKeyFile creates the parent directory (if any) and writes data with mode.
func writeKeyFile(path string, data []byte, mode os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
