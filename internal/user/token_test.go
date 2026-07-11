package user

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"testing"

	"github.com/econumo/econumo/internal/model"
)

var tokenRe = regexp.MustCompile(`^eco_(ses|pat)_[A-Za-z0-9_-]{43}$`)

func TestGenerateAccessToken(t *testing.T) {
	rawSes, hashSes, err := generateAccessToken(model.TokenKindSession)
	if err != nil {
		t.Fatalf("generate session: %v", err)
	}
	if !tokenRe.MatchString(rawSes) || rawSes[:8] != "eco_ses_" {
		t.Errorf("session token %q does not match eco_ses_<43 urlsafe chars>", rawSes)
	}
	sum := sha256.Sum256([]byte(rawSes))
	if hashSes != hex.EncodeToString(sum[:]) {
		t.Errorf("hash mismatch: %q", hashSes)
	}

	rawPat, _, err := generateAccessToken(model.TokenKindPersonal)
	if err != nil {
		t.Fatalf("generate pat: %v", err)
	}
	if rawPat[:8] != "eco_pat_" {
		t.Errorf("pat token %q must start with eco_pat_", rawPat)
	}

	raw2, _, _ := generateAccessToken(model.TokenKindSession)
	if raw2 == rawSes {
		t.Error("two generated tokens must differ")
	}
}

func TestHashAccessToken(t *testing.T) {
	sum := sha256.Sum256([]byte("eco_ses_x"))
	if got := HashAccessToken("eco_ses_x"); got != hex.EncodeToString(sum[:]) {
		t.Errorf("HashAccessToken = %q", got)
	}
}
