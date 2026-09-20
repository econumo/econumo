package oidc

import (
	"crypto/sha256"
	"encoding/base64"
)

// PKCEChallenge is the S256 transform of RFC 7636.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
