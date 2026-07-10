// Package ratelimit is the in-memory brute-force limiter for the public auth
// endpoints: per-key sliding-window attempt counters plus a per-scope global
// backstop. State is process-local and resets on restart by design (single
// binary; see the 2026-07-09 auth-rate-limiting spec).
package ratelimit

import (
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
func attemptKey(scope, key string) string { return "k\x00" + scope + "\x00" + key }
func globalKey(scope string) string       { return "g\x00" + scope }

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
			return errs.NewTooManyRequests(Message)
		}
	}
	if l.cfg.Global > 0 {
		k := globalKey(scope)
		hits := prune(l.attempts[k], now, globalWindow)
		if len(hits) >= l.cfg.Global {
			l.attempts[k] = hits
			return errs.NewTooManyRequests(Message)
		}
		l.attempts[k] = append(hits, now)
	}
	return nil
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
