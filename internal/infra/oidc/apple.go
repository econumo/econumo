package oidc

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"time"
)

const AppleIssuerURL = "https://appleid.apple.com"

func ParseApplePrivateKey(pemText string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("oidc: apple private key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("oidc: apple private key is not an ECDSA key")
	}
	return ec, nil
}

// AppleClientSecret is the ES256 JWT Apple requires in place of a static
// client secret: iss = team id, sub = services id, aud = Apple, 5-minute life.
func AppleClientSecret(teamID, clientID, keyID string, key *ecdsa.PrivateKey, now time.Time) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID})
	claims, _ := json.Marshal(map[string]any{
		"iss": teamID, "sub": clientID, "aud": AppleIssuerURL,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
