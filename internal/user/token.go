// Opaque access-token generation and hashing. The raw token is
// "<prefix><base64url of 32 random bytes>" (43 chars of payload, 256-bit
// entropy); only its sha256 hex ever reaches the database. A plain sha256 (not
// argon2/bcrypt) is deliberate: with 256 bits of randomness brute-force is
// infeasible, and verification must stay one cheap indexed lookup per request.
package user

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"time"

	"github.com/econumo/econumo/internal/model"
)

const (
	sessionTokenPrefix  = "eco_ses_"
	personalTokenPrefix = "eco_pat_"
	tokenRandomBytes    = 32

	// SessionTTL is the sliding window: a session dies 30 days after its last
	// use. Exported for the test harness to compute seed expiries.
	SessionTTL = 30 * 24 * time.Hour
	// touchInterval throttles last-used persistence so single-writer SQLite
	// stays off the hot path ("last active" is accurate to ±5 minutes).
	touchInterval = 5 * time.Minute
	// deadTokenRetention is how long an expired/revoked row is kept before the
	// opportunistic purge at login deletes it.
	deadTokenRetention = 30 * 24 * time.Hour
)

func generateAccessToken(kind string) (string, string, error) {
	b := make([]byte, tokenRandomBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", err
	}
	prefix := sessionTokenPrefix
	if kind == model.TokenKindPersonal {
		prefix = personalTokenPrefix
	}
	raw := prefix + base64.RawURLEncoding.EncodeToString(b)
	return raw, HashAccessToken(raw), nil
}

// HashAccessToken maps a raw bearer token to its storage/lookup key.
func HashAccessToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
