package auth

import (
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
)

func TestArgon2id_HashAndVerify(t *testing.T) {
	h, err := hashArgon2id("s3cret-password")
	if err != nil {
		t.Fatalf("hashArgon2id: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("unexpected PHC prefix: %s", h)
	}
	if !verifyArgon2id(h, "s3cret-password") {
		t.Error("verify rejected the correct password")
	}
	if verifyArgon2id(h, "s3cret-passwordX") {
		t.Error("verify accepted a wrong password")
	}
	// Two hashes of the same password differ (random salt).
	h2, err := hashArgon2id("s3cret-password")
	if err != nil {
		t.Fatalf("hashArgon2id: %v", err)
	}
	if h == h2 {
		t.Error("two hashes are identical — salt is not random")
	}
}

// Parameters are read from the stored string, so a hash produced with different
// (e.g. future-tuned) parameters still verifies.
func TestArgon2id_VerifyUsesStoredParams(t *testing.T) {
	h := encodeArgon2id("pw", []byte("0123456789abcdef"), 8, 1, 1, 16)
	if !verifyArgon2id(h, "pw") {
		t.Error("verify rejected a hash with non-default params")
	}
	if verifyArgon2id(h, "pwX") {
		t.Error("verify accepted a wrong password with non-default params")
	}
}

func TestArgon2id_VerifyFailsClosed(t *testing.T) {
	valid := encodeArgon2id("pw", []byte("0123456789abcdef"), 8, 1, 1, 16)
	for name, stored := range map[string]string{
		"empty":            "",
		"not phc":          "abcdef",
		"wrong algorithm":  strings.Replace(valid, "argon2id", "argon2i", 1),
		"wrong version":    strings.Replace(valid, "v=19", "v=18", 1),
		"bad params":       "$argon2id$v=19$m=x,t=y,p=z$c2FsdA$aGFzaA",
		"bad salt b64":     "$argon2id$v=19$m=8,t=1,p=1$!!!$aGFzaA",
		"bad key b64":      "$argon2id$v=19$m=8,t=1,p=1$c2FsdA$!!!",
		"empty key":        "$argon2id$v=19$m=8,t=1,p=1$c2FsdA$",
		"missing sections": "$argon2id$v=19$m=8,t=1,p=1$c2FsdA",
		"legacy sha512":    "gLhkr35ZvvKPZeqspxjnEXharQf+bkzBhecvcxITY0IHalyVKQCVBUB1LGKcO0/EbCcbfsFhZBqEQ54rHhOohw==",
		"m over cap":       "$argon2id$v=19$m=4194305,t=1,p=1$c2FsdA$aGFzaA",
		"t over cap":       "$argon2id$v=19$m=8,t=65,p=1$c2FsdA$aGFzaA",
		"p over cap":       "$argon2id$v=19$m=8,t=1,p=17$c2FsdA$aGFzaA",
	} {
		if verifyArgon2id(stored, "pw") {
			t.Errorf("%s: verify accepted malformed input %q", name, stored)
		}
	}
}

// TestArgon2id_GoldenVector freezes the exact PHC output for a fixed salt so an
// accidental change to the encoding or parameters is caught here, not by locked
// out users.
func TestArgon2id_GoldenVector(t *testing.T) {
	const golden = "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$LRLzSqkezQbwHbXjJeh4xRkwvIgY0fnaPORiZsKh/OU"
	got := encodeArgon2id("s3cret-password", []byte("0123456789abcdef"), argonMemoryKiB, argonTime, argonThreads, argonKeyLen)
	if got != golden {
		t.Errorf("encodeArgon2id drifted:\n got=%s\nwant=%s", got, golden)
	}
	if !verifyArgon2id(golden, "s3cret-password") {
		t.Error("golden vector does not verify")
	}
}

func TestPasswordHasher_VerifyDispatch(t *testing.T) {
	h := NewPasswordHasher()
	legacy := h.HashSHA512("pw", "somesalt")
	modern, err := h.Hash("pw")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !h.Verify(model.AlgorithmSHA512, legacy, "pw", "somesalt") {
		t.Error("sha512 dispatch failed")
	}
	if !h.Verify(model.AlgorithmArgon2id, modern, "pw", "") {
		t.Error("argon2id dispatch failed")
	}
	if h.Verify(model.AlgorithmArgon2id, legacy, "pw", "somesalt") {
		t.Error("argon2id verifier accepted a sha512 hash")
	}
	if h.Verify(model.AlgorithmSHA512, modern, "pw", "") {
		t.Error("sha512 verifier accepted an argon2id hash")
	}
	if h.Verify("", legacy, "pw", "somesalt") || h.Verify("md5", legacy, "pw", "somesalt") {
		t.Error("unknown algorithm must fail closed")
	}
}
