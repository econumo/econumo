package imports

import (
	"crypto/sha256"
	"encoding/hex"
)

func HashPayload(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
