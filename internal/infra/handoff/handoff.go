// Package handoff mints and verifies the short-lived identity assertion the SPA
// carries to the payment portal. It lives in infra rather than in a feature
// because its only minter is the user feature, and features may not import
// each other.
package handoff

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// domain separates these signatures from any other HMAC taken under the same
// key, so a handoff signature cannot be replayed as something else.
const domain = "billing-handoff:v1"

const TTL = 10 * time.Minute

// aiDomain separates the AI-gateway identity token (one per completion call,
// verified by the AI gateway) from the handoff assertion.
const aiDomain = "econumo-ai:v1"

// AITTL is short: the token rides one completion call (≤ 90 s) plus skew.
const AITTL = 5 * time.Minute

var (
	ErrInvalid = errors.New("handoff: invalid token")
	ErrExpired = errors.New("handoff: token expired")
)

type payload struct {
	UID string `json:"uid"`
	Exp int64  `json:"exp"`
}

type Signer struct{ key []byte }

func NewSigner(key string) *Signer { return &Signer{key: []byte(key)} }

func (s *Signer) Sign(uid string, now time.Time) (string, error) {
	return sign(s.key, domain, uid, now.Add(TTL))
}

// SignAI mints the bearer the econumo:// AI dialect sends to the AI gateway.
func (s *Signer) SignAI(uid string, now time.Time) (string, error) {
	return sign(s.key, aiDomain, uid, now.Add(AITTL))
}

func sign(key []byte, dom, uid string, exp time.Time) (string, error) {
	body, err := json.Marshal(payload{UID: uid, Exp: exp.Unix()})
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(body)
	return enc + "." + base64.RawURLEncoding.EncodeToString(mac(key, dom, enc)), nil
}

// Verify has no production caller here — the portal verifies. It exists so the
// scheme is executable from one definition, letting the tests catch a
// spec-level mistake instead of only proving Sign is self-consistent.
func Verify(token, key string, now time.Time) (string, error) {
	return verify(domain, token, key, now)
}

// VerifyAI has no production caller here either; see Verify.
func VerifyAI(token, key string, now time.Time) (string, error) {
	return verify(aiDomain, token, key, now)
}

func verify(dom, token, key string, now time.Time) (string, error) {
	enc, encSig, ok := strings.Cut(token, ".")
	if !ok || strings.Contains(encSig, ".") {
		return "", ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return "", ErrInvalid
	}
	if !hmac.Equal(sig, mac([]byte(key), dom, enc)) {
		return "", ErrInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", ErrInvalid
	}
	var p payload
	if err := json.Unmarshal(body, &p); err != nil || p.UID == "" {
		return "", ErrInvalid
	}
	if !now.Before(time.Unix(p.Exp, 0)) {
		return "", ErrExpired
	}
	return p.UID, nil
}

// mac signs the ENCODED payload, not the struct: signing before serialization
// invites a verify-side mismatch when JSON key order or escaping differs
// between implementations, and the portal is a separate codebase.
func mac(key []byte, dom, encPayload string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(dom))
	m.Write([]byte(encPayload))
	return m.Sum(nil)
}
