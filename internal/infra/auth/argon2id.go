package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP recommendation: 19 MiB memory, 2 passes, 1 lane).
// They are baked into each PHC hash string, so tuning them later only affects
// new hashes — verifyArgon2id always uses the parameters stored in the hash.
const (
	argonMemoryKiB uint32 = 19456
	argonTime      uint32 = 2
	argonThreads   uint8  = 1
	argonSaltLen          = 16
	argonKeyLen    uint32 = 32
)

// Verification caps: a crafted stored string must not drive unbounded
// allocation (a huge m would OOM the process); the caps leave ample headroom
// for future parameter tuning.
const (
	argonMaxMemoryKiB uint32 = 4194304 // 4 GiB
	argonMaxTime      uint32 = 64
	argonMaxThreads   uint8  = 16
)

func hashArgon2id(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	return encodeArgon2id(plain, salt, argonMemoryKiB, argonTime, argonThreads, argonKeyLen), nil
}

// encodeArgon2id derives the key and renders the standard PHC string:
// $argon2id$v=19$m=<KiB>,t=<passes>,p=<lanes>$<b64 salt>$<b64 key>
// (raw std base64, no padding).
func encodeArgon2id(plain string, salt []byte, memoryKiB, time uint32, threads uint8, keyLen uint32) string {
	key := argon2.IDKey([]byte(plain), salt, time, memoryKiB, threads, keyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, time, threads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

// verifyArgon2id recomputes the key with the parameters stored in the hash and
// compares constant-time. Anything malformed fails closed.
func verifyArgon2id(stored, plain string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var m, t uint32
	var p uint8
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || n != 3 || m == 0 || t == 0 || p == 0 {
		return false
	}
	if m > argonMaxMemoryKiB || t > argonMaxTime || p > argonMaxThreads {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return false
	}
	computed := argon2.IDKey([]byte(plain), salt, t, m, p, uint32(len(key)))
	return subtle.ConstantTimeCompare(computed, key) == 1
}
