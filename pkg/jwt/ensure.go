package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureKeypair is the single key-generation entry point shared by the server
// (serve, force=false) and the app:generate-jwt-keypair CLI command. It makes a
// deployment self-sufficient: if the RS256 keypair at privatePath/publicPath does
// not exist it is generated, so no keys are committed to the repo or baked into
// the image. It returns the passphrase the caller must pass to New, and whether
// it actually generated a keypair on this call (false when an existing one was
// left untouched).
//
// Passphrase handling:
//   - If passphrase is non-empty (ECONUMO_JWT_PASSPHRASE) it is used as-is.
//   - If it is empty, a random passphrase is generated once and persisted in
//     ".jwt-passphrase" beside the private key, then reused on later boots — so a
//     deployment works with zero JWT configuration as long as the key directory
//     is on a persistent volume. Set ECONUMO_JWT_PASSPHRASE to avoid writing that file.
//
// Generation rule: keys are written when force is true OR either key file is
// missing. With force=false an existing keypair is left untouched, so a restart
// can't invalidate already-issued tokens; force=true regenerates unconditionally.
func EnsureKeypair(privatePath, publicPath, passphrase string, force bool) (string, bool, error) {
	if passphrase == "" {
		p, err := loadOrCreatePassphrase(filepath.Join(filepath.Dir(privatePath), ".jwt-passphrase"))
		if err != nil {
			return "", false, err
		}
		passphrase = p
	}

	_, errPriv := os.Stat(privatePath)
	_, errPub := os.Stat(publicPath)
	missing := errors.Is(errPriv, os.ErrNotExist) || errors.Is(errPub, os.ErrNotExist)
	if force || missing {
		if err := GenerateKeypair(privatePath, publicPath, passphrase, force); err != nil {
			return "", false, err
		}
		return passphrase, true, nil
	}
	return passphrase, false, nil
}

// loadOrCreatePassphrase returns the passphrase stored at path, creating a fresh
// random one (persisted 0600) if the file is missing or empty.
func loadOrCreatePassphrase(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate jwt passphrase: %w", err)
	}
	pass := hex.EncodeToString(buf)
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create dir for %s: %w", path, err)
		}
	}
	if err := os.WriteFile(path, []byte(pass+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return pass, nil
}
