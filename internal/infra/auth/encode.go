// Package auth implements Econumo's crypto: the EncodeService (identifier
// hashing + reversible email encryption) and the password hasher. These are
// wire/data-compatible with existing accounts (see CLAUDE.md), locked
// down by golden vectors in *_test.go.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

// EncodeService hashes identifiers and reversibly encrypts emails.
//
// AES-128-CBC takes a 16-byte key, but the raw ECONUMO_DATA_SALT may be any
// length. To stay wire-compatible with already-stored emails, an over-long salt
// is truncated to its first 16 bytes for the AES key and a short one is
// zero-padded to 16, while the HMAC key and the md5 identifier deliberately use
// the FULL salt. This asymmetry is part of the frozen data format: existing rows
// were written this way, so deployments whose salt is not exactly 16 bytes must
// keep decrypting.
type EncodeService struct {
	salt   []byte // full salt: HMAC key + md5 identifier salt
	aesKey []byte // salt coerced to 16 bytes: the AES-128 key
}

func NewEncodeService(salt string) *EncodeService {
	s := []byte(salt)
	return &EncodeService{salt: s, aesKey: coerceAESKey(s)}
}

// coerceAESKey forces the salt to the 16-byte AES-128-CBC key length the stored
// data was encrypted under: take the first 16 bytes, zero-padding if shorter. For
// an empty salt it returns nil (Encode/Decode short-circuit to passthrough before
// the key is used, so the value is never consulted).
func coerceAESKey(salt []byte) []byte {
	if len(salt) == 0 {
		return nil
	}
	key := make([]byte, 16)
	copy(key, salt) // copies min(16, len(salt)) bytes; remainder stays zero
	return key
}

// Hash returns hex(md5(value + salt)) — the CHAR(32) user identifier.
// Callers must lowercase the email before hashing; this method does not
// lowercase.
func (e *EncodeService) Hash(value string) string {
	sum := md5.Sum(append([]byte(value), e.salt...))
	return hex.EncodeToString(sum[:])
}

// Encode encrypts value with AES-128-CBC and returns
// base64( iv[16] || hmac_sha256[32] || ciphertext ). Empty salt -> passthrough.
// A fresh random IV is used per call.
func (e *EncodeService) Encode(value string) (string, error) {
	if len(e.salt) == 0 {
		return value, nil
	}
	iv := make([]byte, aes.BlockSize) // 16
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	return e.encodeWithIV([]byte(value), iv)
}

// encodeWithIV is the deterministic core of Encode, factored out so tests can
// pin the IV and assert byte-for-byte output against the golden vectors.
func (e *EncodeService) encodeWithIV(value, iv []byte) (string, error) {
	block, err := aes.NewCipher(e.aesKey)
	if err != nil {
		return "", err
	}
	plaintext := pkcs7Pad(value, aes.BlockSize)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plaintext)

	mac := hmac.New(sha256.New, e.salt)
	mac.Write(ciphertext)
	h := mac.Sum(nil) // 32 bytes

	out := make([]byte, 0, len(iv)+len(h)+len(ciphertext))
	out = append(out, iv...)
	out = append(out, h...)
	out = append(out, ciphertext...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decode reverses Encode, returning an error when the HMAC check fails.
// Empty salt -> passthrough.
func (e *EncodeService) Decode(value string) (string, error) {
	if len(e.salt) == 0 {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	if len(raw) < aes.BlockSize+sha256.Size {
		return "", errors.New("encode: ciphertext too short")
	}
	iv := raw[:aes.BlockSize]
	mac := raw[aes.BlockSize : aes.BlockSize+sha256.Size]
	ciphertext := raw[aes.BlockSize+sha256.Size:]

	expected := hmac.New(sha256.New, e.salt)
	expected.Write(ciphertext)
	if subtle.ConstantTimeCompare(mac, expected.Sum(nil)) != 1 {
		return "", errors.New("encode: hmac mismatch")
	}
	block, err := aes.NewCipher(e.aesKey)
	if err != nil {
		return "", err
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("encode: ciphertext not block-aligned")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	unpadded, err := pkcs7Unpad(plaintext, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return string(unpadded), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("encode: invalid padding length")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > blockSize || pad > len(data) {
		return nil, errors.New("encode: invalid padding")
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, errors.New("encode: invalid padding bytes")
		}
	}
	return data[:len(data)-pad], nil
}
