package ai

import (
	"log/slog"
	"strconv"
	"strings"
)

// Keys hands out short per-request keys ("a1", "a2", ...) that stand in for
// entity ids in a prompt: models mis-copy UUIDs, and a mis-copied id silently
// loses its answer. A reply's keys are mapped back with ID.
type Keys struct {
	prefix string
	byKey  map[string]string
	byID   map[string]string
}

func NewKeys(prefix string) *Keys {
	return &Keys{prefix: prefix, byKey: map[string]string{}, byID: map[string]string{}}
}

// Key is the id's key, the same one every time the id is asked for.
func (k *Keys) Key(id string) string {
	if key, ok := k.byID[id]; ok {
		return key
	}
	key := k.prefix + strconv.Itoa(len(k.byKey)+1)
	k.byKey[key], k.byID[id] = id, key
	return key
}

// ID maps a key from a reply back to its id; a model may change its case or
// pad it with spaces.
func (k *Keys) ID(key string) (string, bool) {
	id, ok := k.byKey[strings.ToLower(strings.TrimSpace(key))]
	return id, ok
}

// LogUnknownKeys records that a reply named keys its request never issued, so
// a model that copies badly shows up in the logs; only the count is logged.
func LogUnknownKeys(userID, part string, n int) {
	if n > 0 {
		slog.Warn("ai reply named unknown keys", "user_id", userID, "part", part, "count", n)
	}
}
