package authserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// RFC 7636 bounds the verifier at 43-128 characters.
func VerifyPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}
