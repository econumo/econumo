package auth

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"github.com/econumo/econumo/internal/model"
)

// PasswordHasher computes stored-password hashes: sha512, 500 iterations,
// base64-std encoded. Wire/data-compatible with existing accounts (see
// CLAUDE.md).
//
// Algorithm:
//
//	salted = mergePasswordAndSalt(password, salt)
//	digest = sha512(salted)                        // raw bytes
//	for i := 1; i < iterations; i++ {              // 499 extra rounds (500 total)
//	    digest = sha512(digest ++ salted)
//	}
//	return base64_std(digest)
//
// mergePasswordAndSalt: empty salt -> password unchanged; otherwise
// password + "{" + salt + "}" (salt may not contain { or }).
type PasswordHasher struct {
	iterations int
}

func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{iterations: 500}
}

// expectedHashLen is the base64-encoded length of a raw sha512 digest (64 bytes
// -> 88 base64 chars incl. padding). Used by the verify() guard.
const expectedHashLen = 88

func mergePasswordAndSalt(password, salt string) string {
	if salt == "" {
		return password
	}
	// The contract forbids { or } in the salt; stored salts are sha1 hex so this
	// never triggers, and we do not special-case it here.
	return password + "{" + salt + "}"
}

// HashSHA512 hashes a password with the legacy sha512 scheme. Only fixtures
// call this directly; new passwords use Hash.
func (h *PasswordHasher) HashSHA512(plainPassword, salt string) string {
	salted := []byte(mergePasswordAndSalt(plainPassword, salt))

	d := sha512.Sum512(salted)
	digest := d[:]
	for i := 1; i < h.iterations; i++ {
		next := sha512.Sum512(append(append([]byte{}, digest...), salted...))
		digest = next[:]
	}
	return base64.StdEncoding.EncodeToString(digest)
}

// verifySHA512 reports whether plainPassword (with salt) matches
// hashedPassword, including the length / '$' guard and a constant-time
// comparison.
func (h *PasswordHasher) verifySHA512(hashedPassword, plainPassword, salt string) bool {
	if len(hashedPassword) != expectedHashLen || strings.Contains(hashedPassword, "$") {
		return false
	}
	computed := h.HashSHA512(plainPassword, salt)
	return subtle.ConstantTimeCompare([]byte(hashedPassword), []byte(computed)) == 1
}

// Hash hashes a NEW plaintext password — always with the current algorithm
// (argon2id). Legacy sha512 hashes are only ever verified, never produced,
// except through HashSHA512 (fixtures).
func (h *PasswordHasher) Hash(plainPassword string) (string, error) {
	return hashArgon2id(plainPassword)
}

// Verify dispatches on the algorithm recorded next to the stored hash.
// Unknown algorithm values fail closed.
func (h *PasswordHasher) Verify(algorithm, hashedPassword, plainPassword, salt string) bool {
	switch algorithm {
	case model.AlgorithmSHA512:
		return h.verifySHA512(hashedPassword, plainPassword, salt)
	case model.AlgorithmArgon2id:
		return verifyArgon2id(hashedPassword, plainPassword)
	default:
		return false
	}
}
