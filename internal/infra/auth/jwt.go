package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/youmark/pkcs8"
)

// tokenTTL is 2592000 seconds = 30 days. exp = iat + tokenTTL.
const tokenTTL = 2592000 * time.Second

// signingMethod is fixed to RS256. We never accept any other algorithm —
// neither when issuing nor when verifying.
var signingMethod = jwtv5.SigningMethodRS256

// Claims is the exact claim set Econumo emits on login: iat, exp=iat+TTL,
// roles, username (the plaintext email) and id (the user UUID string). There is
// intentionally NO baseCurrency claim.
//
// Claim ordering in the JSON does not affect signature verification
// (verification re-signs the raw, already-encoded payload bytes), so wire
// compatibility does not depend on field order here. See CLAUDE.md.
type Claims struct {
	Iat      int64    `json:"iat"`
	Exp      int64    `json:"exp"`
	Roles    []string `json:"roles"`
	Username string   `json:"username"`
	ID       string   `json:"id"`
}

// GetExpirationTime implements jwtv5.Claims so the library validates exp.
func (c Claims) GetExpirationTime() (*jwtv5.NumericDate, error) {
	return jwtv5.NewNumericDate(time.Unix(c.Exp, 0)), nil
}

// GetIssuedAt implements jwtv5.Claims.
func (c Claims) GetIssuedAt() (*jwtv5.NumericDate, error) {
	return jwtv5.NewNumericDate(time.Unix(c.Iat, 0)), nil
}

// GetNotBefore implements jwtv5.Claims. No nbf claim is set, so we report none
// (nil is valid and means "no constraint").
func (c Claims) GetNotBefore() (*jwtv5.NumericDate, error) { return nil, nil }

// GetIssuer implements jwtv5.Claims. No iss claim is set.
func (c Claims) GetIssuer() (string, error) { return "", nil }

// GetSubject implements jwtv5.Claims. No sub claim is set.
func (c Claims) GetSubject() (string, error) { return "", nil }

// GetAudience implements jwtv5.Claims. No aud claim is set.
func (c Claims) GetAudience() (jwtv5.ClaimStrings, error) { return nil, nil }

// JWT issues and verifies RS256 tokens that are wire-compatible with existing
// API clients (see CLAUDE.md). The public key is always loaded
// (verification is always possible); the private key is loaded only when its
// path is readable, so a verify-only deployment need not ship the signing key.
type JWT struct {
	public  *rsa.PublicKey
	private *rsa.PrivateKey // nil when only verification is configured
}

// NewJWT loads the keys from the configured paths. The public key (SPKI PEM) is
// required. The private key (an encrypted PKCS#8 PEM, PBES2) is decrypted with
// the passphrase when its file is readable; if the file is absent the JWT can
// still verify tokens but Issue returns an error.
func NewJWT(privateKeyPath, publicKeyPath, passphrase string) (*JWT, error) {
	pubPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("jwt: read public key: %w", err)
	}
	pub, err := parsePublicKey(pubPEM)
	if err != nil {
		return nil, fmt.Errorf("jwt: parse public key: %w", err)
	}

	j := &JWT{public: pub}

	// The private key is optional: only needed for issuing. os.ReadFile failing
	// (e.g. file not present in a verify-only deployment) is not fatal.
	if privateKeyPath != "" {
		if privPEM, rerr := os.ReadFile(privateKeyPath); rerr == nil {
			priv, perr := parseEncryptedPrivateKey(privPEM, passphrase)
			if perr != nil {
				return nil, fmt.Errorf("jwt: parse private key: %w", perr)
			}
			j.private = priv
		}
	}

	return j, nil
}

// Issue builds the claim set, signs it RS256 and returns the compact JWT.
// now is taken as a parameter (rather than calling time.Now) so callers and
// tests control issuance time. exp is now+TTL as a whole-second integer Unix
// timestamp.
func (j *JWT) Issue(userID, email string, now time.Time) (string, error) {
	if j.private == nil {
		return "", errors.New("jwt: no private key loaded, cannot issue tokens")
	}

	iat := now.Unix()
	claims := Claims{
		Iat:      iat,
		Exp:      iat + int64(tokenTTL/time.Second),
		Roles:    []string{"ROLE_USER"},
		Username: email,
		ID:       userID,
	}

	token := jwtv5.NewWithClaims(signingMethod, claims)
	signed, err := token.SignedString(j.private)
	if err != nil {
		return "", fmt.Errorf("jwt: sign: %w", err)
	}
	return signed, nil
}

// Verify validates the RS256 signature against the public key and checks exp,
// rejecting any token that does not use RS256 (this defends against alg-confusion
// attacks such as "none" or an HS256 token forged with the public key as the
// HMAC secret). It returns the parsed claims on success.
func (j *JWT) Verify(tokenString string) (Claims, error) {
	var claims Claims

	parser := jwtv5.NewParser(
		// Pin the accepted algorithm. The parser still calls the keyfunc, but
		// this rejects any header alg outside the allowed set up front.
		jwtv5.WithValidMethods([]string{signingMethod.Alg()}),
		// Validate exp (the library does this by default, but be explicit).
		jwtv5.WithExpirationRequired(),
	)

	_, err := parser.ParseWithClaims(tokenString, &claims, func(t *jwtv5.Token) (interface{}, error) {
		// Defense in depth: confirm the concrete signing method is RSA, so a
		// crafted header cannot trick us into using the RSA public key bytes as
		// an HMAC secret.
		if _, ok := t.Method.(*jwtv5.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("jwt: unexpected signing method %q", t.Header["alg"])
		}
		return j.public, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("jwt: verify: %w", err)
	}

	return claims, nil
}

// parsePublicKey decodes an SPKI ("BEGIN PUBLIC KEY") PEM into an *rsa.PublicKey.
func parsePublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("expected RSA public key, got %T", pub)
	}
	return rsaPub, nil
}

// parseEncryptedPrivateKey decrypts an encrypted PKCS#8 PEM
// ("BEGIN ENCRYPTED PRIVATE KEY", PBES2 as written by OpenSSL) into an
// *rsa.PrivateKey. Go's crypto/x509 cannot decrypt PBES2 PKCS#8, so we use
// github.com/youmark/pkcs8 (pure Go, CGO-free) for the decryption step.
func parseEncryptedPrivateKey(pemBytes []byte, passphrase string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}

	key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, []byte(passphrase))
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("expected RSA private key, got %T", key)
	}
	return rsaKey, nil
}
