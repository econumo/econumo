// Package ratelimit is the in-memory brute-force limiter for the public auth
// endpoints: per-key sliding-window attempt counters plus a per-scope global
// backstop. State is process-local and resets on restart by design (single
// binary; see the 2026-07-09 auth-rate-limiting spec).
package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
)

// Message is the frozen 429 envelope message.
const Message = "Too many attempts. Try again later."

const (
	defaultMaxKeys = 10000
	defaultWindow  = 15 * time.Minute
	// The global backstop counts every request in its own fixed window so a
	// spray of unique keys is caught quickly, independent of the per-key window.
	globalWindow = time.Minute
)

// Config holds the limiter policy. A limit of 0 (or an absent scope) disables
// that check.
type Config struct {
	Limits  map[string]int // per-scope max attempts per key per Window
	Window  time.Duration  // per-key sliding window (default 15m)
	Global  int            // per-scope cap per minute across ALL keys; 0 disables
	MaxKeys int            // per-key map size cap (default 10000)
}

type Limiter struct {
	mu       sync.Mutex
	cfg      Config
	clock    port.Clock
	attempts map[string][]time.Time
}

func New(cfg Config, clk port.Clock) *Limiter {
	if cfg.MaxKeys <= 0 {
		cfg.MaxKeys = defaultMaxKeys
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultWindow
	}
	return &Limiter{cfg: cfg, clock: clk, attempts: map[string][]time.Time{}}
}

// Key layouts: per-key entries are "k\x00<scope>\x00<key>", global counters are
// "g\x00<scope>". The prefixes keep a caller-supplied key from ever colliding
// with a global counter.
//
// maxKeyLen bounds the bytes retained per attempt key: an attacker submitting
// unique multi-megabyte usernames must not turn the failure map into a memory
// sink. Longer keys are replaced by their sha256 so bucketing is preserved.
const maxKeyLen = 128

func attemptKey(scope, key string) string {
	if len(key) > maxKeyLen {
		sum := sha256.Sum256([]byte(key))
		key = hex.EncodeToString(sum[:])
	}
	return "k\x00" + scope + "\x00" + key
}
func globalKey(scope string) string { return "g\x00" + scope }

// Allow reports whether another attempt may proceed. The per-key check runs
// first and rejects without consuming the global budget, so an attack on one
// key cannot starve every other caller of the endpoint.
func (l *Limiter) Allow(scope, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()

	if limit := l.cfg.Limits[scope]; limit > 0 {
		k := attemptKey(scope, key)
		hits := prune(l.attempts[k], now, l.cfg.Window)
		l.store(k, hits)
		if len(hits) >= limit {
			return errs.NewTooManyRequestsRetryAfter(Message, retryAfterSeconds(hits, now, l.cfg.Window))
		}
	}
	if l.cfg.Global > 0 {
		k := globalKey(scope)
		hits := prune(l.attempts[k], now, globalWindow)
		if len(hits) >= l.cfg.Global {
			l.attempts[k] = hits
			return errs.NewTooManyRequestsRetryAfter(Message, retryAfterSeconds(hits, now, globalWindow))
		}
		l.attempts[k] = append(hits, now)
	}
	return nil
}

// retryAfterSeconds reports when the caller may try again: the oldest hit still
// inside the window ages out at hits[0]+window, freeing exactly one slot. Hits
// are pruned and sorted ascending, so hits[0] is that oldest survivor. Rounds
// UP to whole seconds and to a floor of 1, so a caller that obeys the value
// never retries early and is never told to "wait 0 seconds" while blocked.
func retryAfterSeconds(hits []time.Time, now time.Time, window time.Duration) int {
	if len(hits) == 0 {
		return 0
	}
	remaining := hits[0].Add(window).Sub(now)
	if remaining < time.Second {
		return 1
	}
	return int((remaining + time.Second - 1) / time.Second)
}

// Fail records a failed attempt against the key.
func (l *Limiter) Fail(scope, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cfg.Limits[scope] <= 0 {
		return
	}
	now := l.clock.Now()
	k := attemptKey(scope, key)
	if _, exists := l.attempts[k]; !exists && len(l.attempts) >= l.cfg.MaxKeys {
		l.evict(now)
	}
	l.attempts[k] = append(prune(l.attempts[k], now, l.cfg.Window), now)
}

// Mark records an event timestamp for the key WITHOUT any cap check, for
// scopes used purely as a per-key clock (e.g. "when was a code last emailed").
// Fail is not usable for that: it no-ops on a scope with no configured limit.
// Marked keys share the per-key map and its eviction, so the memory bound is
// the same as for counted scopes.
func (l *Limiter) Mark(scope, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	k := attemptKey(scope, key)
	if _, exists := l.attempts[k]; !exists && len(l.attempts) >= l.cfg.MaxKeys {
		l.evict(now)
	}
	l.attempts[k] = append(prune(l.attempts[k], now, l.cfg.Window), now)
}

// LastAttempt reports when the key last recorded an attempt in this scope, and
// whether any is on record. It reads the same per-key counter Fail writes, so
// the answer is identical for every caller — including usernames that do not
// exist. Endpoints that report a cooldown use this rather than their own
// storage, which would only ever have a row for a real account and would turn
// the reported time into an account-existence oracle.
func (l *Limiter) LastAttempt(scope, key string) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hits := l.attempts[attemptKey(scope, key)]
	if len(hits) == 0 {
		return time.Time{}, false
	}
	return hits[len(hits)-1], true
}

// Clear wipes the key's counter (called after a successful attempt).
func (l *Limiter) Clear(scope, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, attemptKey(scope, key))
}

func (l *Limiter) store(k string, hits []time.Time) {
	if len(hits) == 0 {
		delete(l.attempts, k)
		return
	}
	l.attempts[k] = hits
}

// prune drops timestamps older than the window. Slices are append-only, so
// they are already sorted ascending.
func prune(hits []time.Time, now time.Time, window time.Duration) []time.Time {
	cutoff := now.Add(-window)
	i := 0
	for ; i < len(hits); i++ {
		if hits[i].After(cutoff) {
			break
		}
	}
	return hits[i:]
}

// evict makes room under MaxKeys: drop fully-expired per-key entries; if the
// map is still full, drop the entry with the oldest most-recent attempt.
// Global counters (a handful, one per scope) are never evicted.
func (l *Limiter) evict(now time.Time) {
	var stalest string
	var stalestAt time.Time
	for k, hits := range l.attempts {
		if strings.HasPrefix(k, "g\x00") {
			continue
		}
		pruned := prune(hits, now, l.cfg.Window)
		if len(pruned) == 0 {
			delete(l.attempts, k)
			continue
		}
		l.attempts[k] = pruned
		newest := pruned[len(pruned)-1]
		if stalest == "" || newest.Before(stalestAt) {
			stalest, stalestAt = k, newest
		}
	}
	if len(l.attempts) >= l.cfg.MaxKeys && stalest != "" {
		delete(l.attempts, stalest)
	}
}
