package auth

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/econumo/econumo/internal/model"
)

// vectors holds the golden fixtures from testdata/vectors.json that lock the
// password hasher and EncodeService to the stored data format of existing
// accounts — changing either would make live account rows unreadable.
type vectors struct {
	PasswordHasher []struct {
		Password string `json:"password"`
		Salt     string `json:"salt"`
		Hash     string `json:"hash"`
	} `json:"password_hasher"`
	IdentifierHash []struct {
		Value string `json:"value"`
		Salt  string `json:"salt"`
		Hash  string `json:"hash"`
	} `json:"identifier_hash"`
	Encode []struct {
		Plaintext  string `json:"plaintext"`
		Salt       string `json:"salt"`
		IV         string `json:"iv"`
		Ciphertext string `json:"ciphertext"`
	} `json:"encode"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	b, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	return v
}

func TestPasswordHasher_MatchesGoldenVectors(t *testing.T) {
	v := loadVectors(t)
	h := NewPasswordHasher()
	if len(v.PasswordHasher) == 0 {
		t.Fatal("no password vectors")
	}
	for _, tc := range v.PasswordHasher {
		got := h.HashSHA512(tc.Password, tc.Salt)
		if got != tc.Hash {
			t.Errorf("HashSHA512(%q,%q)\n got=%s\nwant=%s", tc.Password, tc.Salt, got, tc.Hash)
		}
		if !h.Verify(model.AlgorithmSHA512, tc.Hash, tc.Password, tc.Salt) {
			t.Errorf("Verify failed for password %q", tc.Password)
		}
		if h.Verify(model.AlgorithmSHA512, tc.Hash, tc.Password+"x", tc.Salt) {
			t.Errorf("Verify wrongly accepted bad password for %q", tc.Password)
		}
	}
}

func TestIdentifierHash_MatchesGoldenVectors(t *testing.T) {
	v := loadVectors(t)
	for _, tc := range v.IdentifierHash {
		svc := NewEncodeService(tc.Salt)
		if got := svc.Hash(tc.Value); got != tc.Hash {
			t.Errorf("Hash(%q) got=%s want=%s", tc.Value, got, tc.Hash)
		}
	}
}

func TestEncodeDecode_MatchesGoldenVectors(t *testing.T) {
	v := loadVectors(t)
	for _, tc := range v.Encode {
		svc := NewEncodeService(tc.Salt)
		// Decode the golden ciphertext -> must recover the plaintext.
		got, err := svc.Decode(tc.Ciphertext)
		if err != nil {
			t.Errorf("Decode(%q) error: %v", tc.Plaintext, err)
			continue
		}
		if got != tc.Plaintext {
			t.Errorf("Decode -> %q, want %q", got, tc.Plaintext)
		}
		// Round-trip: our Encode -> our Decode recovers plaintext (IV is random,
		// so we can't match the golden ciphertext byte-for-byte, only round-trip).
		enc, err := svc.Encode(tc.Plaintext)
		if err != nil {
			t.Fatalf("Encode error: %v", err)
		}
		back, err := svc.Decode(enc)
		if err != nil || back != tc.Plaintext {
			t.Errorf("round-trip failed: back=%q err=%v", back, err)
		}
	}
}

// TestEncode_FixedIV proves our CBC+HMAC layout is byte-identical to the golden
// vectors when the IV is held fixed (the only nondeterministic input).
func TestEncode_FixedIV(t *testing.T) {
	v := loadVectors(t)
	for _, tc := range v.Encode {
		iv, err := hex.DecodeString(tc.IV)
		if err != nil {
			t.Fatalf("bad iv hex: %v", err)
		}
		svc := NewEncodeService(tc.Salt)
		got, err := svc.encodeWithIV([]byte(tc.Plaintext), iv)
		if err != nil {
			t.Fatalf("encodeWithIV: %v", err)
		}
		if got != tc.Ciphertext {
			t.Errorf("encodeWithIV(%q)\n got=%s\nwant=%s", tc.Plaintext, got, tc.Ciphertext)
		}
	}
}
