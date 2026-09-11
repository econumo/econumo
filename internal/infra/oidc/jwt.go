package oidc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var ErrInvalidToken = errors.New("oidc: invalid id token")

const clockSkew = 60 * time.Second

type jwsHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type idTokenClaims struct {
	Iss           string          `json:"iss"`
	Sub           string          `json:"sub"`
	Aud           json.RawMessage `json:"aud"`
	Exp           int64           `json:"exp"`
	Iat           int64           `json:"iat"`
	Nonce         string          `json:"nonce"`
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"`
	Name          string          `json:"name"`
}

// parseCompact splits a JWS, decodes header and claims, and returns the
// signing input and signature for verification.
func parseCompact(raw string) (jwsHeader, idTokenClaims, []byte, []byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	var h jwsHeader
	if err := json.Unmarshal(hb, &h); err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	var c idTokenClaims
	if err := json.Unmarshal(pb, &c); err != nil {
		return jwsHeader{}, idTokenClaims{}, nil, nil, ErrInvalidToken
	}
	return h, c, []byte(parts[0] + "." + parts[1]), sig, nil
}

func verifySignature(alg string, key crypto.PublicKey, signingInput, sig []byte) error {
	digest := sha256.Sum256(signingInput)
	switch alg {
	case "RS256":
		pub, ok := key.(*rsa.PublicKey)
		if !ok {
			return ErrInvalidToken
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
			return ErrInvalidToken
		}
		return nil
	case "ES256":
		pub, ok := key.(*ecdsa.PublicKey)
		if !ok || len(sig) != 64 {
			return ErrInvalidToken
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(pub, digest[:], r, s) {
			return ErrInvalidToken
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, alg)
	}
}

// audienceContains accepts both the string and the array form of aud.
func audienceContains(raw json.RawMessage, clientID string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == clientID
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if a == clientID {
				return true
			}
		}
	}
	return false
}

// parseEmailVerified accepts a JSON bool or the strings "true"/"false" (Apple).
func parseEmailVerified(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == "true"
	}
	return false
}

func (c idTokenClaims) toClaims() Claims {
	return Claims{Subject: c.Sub, Email: c.Email, EmailVerified: parseEmailVerified(c.EmailVerified), Name: c.Name}
}
