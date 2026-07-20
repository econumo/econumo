package handoff

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

const testKey = "0123456789abcdef0123456789abcdef"

var now = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func TestSignVerifyRoundTrip(t *testing.T) {
	tok, err := NewSigner(testKey).Sign("user-1", now)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := Verify(tok, testKey, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if uid != "user-1" {
		t.Fatalf("uid = %q, want user-1", uid)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	if _, err := Verify(tok, testKey, now.Add(TTL)); !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired at exactly exp", err)
	}
}

func TestVerifyAcceptsJustBeforeExpiry(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	if _, err := Verify(tok, testKey, now.Add(TTL-time.Second)); err != nil {
		t.Fatalf("err = %v, want valid one second before exp", err)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	_, sig, _ := strings.Cut(tok, ".")
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":"user-2","exp":9999999999}`))
	if _, err := Verify(forged+"."+sig, testKey, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestVerifyRejectsForeignKey(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	if _, err := Verify(tok, "ffffffffffffffffffffffffffffffff", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

// Domain separation: the same payload signed WITHOUT the billing-handoff:v1
// prefix must not verify, or a signature minted for another purpose under the
// same key could be replayed as a handoff.
func TestVerifyRejectsUndomainedSignature(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	encPayload, _, _ := strings.Cut(tok, ".")
	m := hmac.New(sha256.New, []byte(testKey))
	m.Write([]byte(encPayload))
	bare := encPayload + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	if _, err := Verify(bare, testKey, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, tok := range []string{"", "nodot", "!!!.!!!", "a.b.c"} {
		if _, err := Verify(tok, testKey, now); err == nil {
			t.Fatalf("token %q verified", tok)
		}
	}
}

func TestSignedTokenIsURLSafe(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("0198f3c1-aaaa-bbbb-cccc-ddddeeeeffff", now)
	for _, r := range tok {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.'
		if !ok {
			t.Fatalf("token contains %q, which needs escaping in a query string", r)
		}
	}
}
