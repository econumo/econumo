package auth

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// TestCoerceAESKey pins the AES-128 key derivation: over-length salts use only
// their first 16 bytes, shorter salts zero-pad to 16, and an empty salt yields no
// key (passthrough path). Regression for a 33-byte ECONUMO_DATA_SALT (one with a
// multibyte char) that otherwise produced "crypto/aes: invalid key size 33".
func TestCoerceAESKey(t *testing.T) {
	long := []byte("SYNTHETIC_TEST_SALT_pad_xyz789£!") // 33 bytes (£ is 2)
	if len(long) != 33 {
		t.Fatalf("precondition: salt is %d bytes, want 33", len(long))
	}
	key := coerceAESKey(long)
	if len(key) != 16 {
		t.Fatalf("coerceAESKey(33-byte) len = %d, want 16", len(key))
	}
	if !bytes.Equal(key, long[:16]) {
		t.Errorf("AES key must be the first 16 salt bytes")
	}

	short := coerceAESKey([]byte("abc"))
	if len(short) != 16 || !bytes.Equal(short[:3], []byte("abc")) || short[3] != 0 {
		t.Errorf("short salt must zero-pad to 16: %v", short)
	}

	if coerceAESKey(nil) != nil {
		t.Errorf("empty salt must yield nil key (passthrough)")
	}
}

// TestEncodeDecode_NonSixteenByteSalt proves the EncodeService works with a salt
// that is not exactly 16 bytes: it round-trips, and ciphertext produced with the
// full 33-byte salt is identical to ciphertext produced with just its first 16
// bytes as the AES key (HMAC still uses the full salt, so the salts must differ
// only past byte 16 for the ciphertext bodies to match — here we assert the
// round-trip, which is the user-visible contract).
func TestEncodeDecode_NonSixteenByteSalt(t *testing.T) {
	salt := "SYNTHETIC_TEST_SALT_pad_xyz789£!"
	svc := NewEncodeService(salt)

	const plain = "user@example.test"
	enc, err := svc.Encode(plain)
	if err != nil {
		t.Fatalf("Encode with 33-byte salt: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(enc); err != nil {
		t.Fatalf("ciphertext not valid base64: %v", err)
	}
	back, err := svc.Decode(enc)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if back != plain {
		t.Errorf("round-trip = %q, want %q", back, plain)
	}
}
