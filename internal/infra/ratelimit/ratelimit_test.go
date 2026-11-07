package ratelimit_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

var ctx = reqctx.WithClientIP(context.Background(), "203.0.113.1")

func fromIP(ip string) context.Context { return reqctx.WithClientIP(context.Background(), ip) }

// fakeClock is a mutable clock so tests can slide the window.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func newLimiter(cfg ratelimit.Config) (*ratelimit.Limiter, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)}
	return ratelimit.New(cfg, clk), clk
}

func cfgLogin(limit int) ratelimit.Config {
	return ratelimit.Config{Limits: map[string]int{"login": limit}, Window: 15 * time.Minute}
}

func count(l *ratelimit.Limiter, scope, key string) { _ = l.Reserve(scope, key) }

func mustAllowed(t *testing.T, l *ratelimit.Limiter, scope, key string) {
	t.Helper()
	if err := l.Allow(ctx, scope, key); err != nil {
		t.Fatalf("Allow(%s,%s) = %v, want nil", scope, key, err)
	}
}

func mustBlocked(t *testing.T, l *ratelimit.Limiter, scope, key string) {
	t.Helper()
	err := l.Allow(ctx, scope, key)
	if err == nil {
		t.Fatalf("Allow(%s,%s) = nil, want TooManyRequestsError", scope, key)
	}
	if _, ok := errs.AsTooManyRequests(err); !ok {
		t.Fatalf("Allow(%s,%s) = %T, want *errs.TooManyRequestsError", scope, key, err)
	}
	if err.Error() != ratelimit.Message {
		t.Fatalf("message = %q, want %q", err.Error(), ratelimit.Message)
	}
}

// retryAfterOf asserts the call was blocked and returns the reported wait.
func retryAfterOf(t *testing.T, l *ratelimit.Limiter, scope, key string) int {
	t.Helper()
	v, ok := errs.AsTooManyRequests(l.Allow(ctx, scope, key))
	if !ok {
		t.Fatalf("Allow(%s,%s) did not block", scope, key)
	}
	return v.RetryAfter
}

// A 429 must always say how long to wait: the caller's oldest counted attempt
// ages out at hits[0]+window, freeing exactly one slot.
func TestAllow_BlockedCarriesRetryAfter(t *testing.T) {
	l, clk := newLimiter(cfgLogin(2))
	const key = "u@example.test"

	count(l, "login", key) // t0
	clk.t = clk.t.Add(4 * time.Minute)
	count(l, "login", key) // t0+4m -> at the cap

	// The oldest hit (t0) expires 15m after it was recorded, i.e. in 11 more min.
	if got := retryAfterOf(t, l, "login", key); got != 11*60 {
		t.Errorf("RetryAfter = %ds, want %ds (the oldest hit ageing out)", got, 11*60)
	}

	// As real time passes the wait shrinks in step.
	clk.t = clk.t.Add(6 * time.Minute)
	if got := retryAfterOf(t, l, "login", key); got != 5*60 {
		t.Errorf("RetryAfter = %ds, want %ds", got, 5*60)
	}

	// Once it elapses the caller is let through, exactly as advertised.
	clk.t = clk.t.Add(5 * time.Minute)
	mustAllowed(t, l, "login", key)
}

// A blocked caller must never be told to wait 0 seconds — that reads as
// "retry now" and would spin.
func TestAllow_RetryAfterIsNeverZeroWhileBlocked(t *testing.T) {
	l, clk := newLimiter(cfgLogin(1))
	const key = "u@example.test"

	count(l, "login", key)
	// A hair before the window closes: the true remainder is a sub-second
	// fraction, which must round up to 1 rather than down to 0.
	clk.t = clk.t.Add(15*time.Minute - 100*time.Millisecond)
	if got := retryAfterOf(t, l, "login", key); got != 1 {
		t.Errorf("RetryAfter = %d, want 1", got)
	}
}

// The global backstop rejects on its own one-minute window, so its advertised
// wait must come from that window, not the per-key one.
func TestAllow_IPCapCarriesItsOwnRetryAfter(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, IP: 2,
	})
	mustAllowed(t, l, "login", "a@example.test")
	clk.t = clk.t.Add(20 * time.Second)
	mustAllowed(t, l, "login", "b@example.test")

	// Global window is 1m; the oldest global hit was 20s ago, so 40s remain —
	// far short of the 15m per-key window.
	if got := retryAfterOf(t, l, "login", "c@example.test"); got != 40 {
		t.Errorf("RetryAfter = %ds, want 40 (the global minute window)", got)
	}
}

func TestAllow_UnderLimit(t *testing.T) {
	l, _ := newLimiter(cfgLogin(3))
	for i := 0; i < 2; i++ {
		mustAllowed(t, l, "login", "user@example.test")
		count(l, "login", "user@example.test")
	}
	mustAllowed(t, l, "login", "user@example.test") // 2 failures < 3
}

func TestAllow_BlocksAtLimit(t *testing.T) {
	l, _ := newLimiter(cfgLogin(3))
	for i := 0; i < 3; i++ {
		mustAllowed(t, l, "login", "user@example.test")
		count(l, "login", "user@example.test")
	}
	mustBlocked(t, l, "login", "user@example.test")
}

func TestAllow_KeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	count(l, "login", "a@example.test")
	mustBlocked(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
}

func TestAllow_ScopesAreIndependent(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 1, "reset": 1}, Window: 15 * time.Minute,
	})
	count(l, "login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	mustAllowed(t, l, "reset", "u@example.test")
}

func TestAllow_WindowSlides(t *testing.T) {
	l, clk := newLimiter(cfgLogin(2))
	count(l, "login", "u@example.test")
	clk.t = clk.t.Add(10 * time.Minute)
	count(l, "login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	clk.t = clk.t.Add(6 * time.Minute) // first failure now 16m old -> expired
	mustAllowed(t, l, "login", "u@example.test")
	clk.t = clk.t.Add(10 * time.Minute) // second failure now expired too
	mustAllowed(t, l, "login", "u@example.test")
}

func TestClear_ResetsCounter(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	count(l, "login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	l.Clear("login", "u@example.test")
	mustAllowed(t, l, "login", "u@example.test")
}

func TestZeroLimit_Disables(t *testing.T) {
	l, _ := newLimiter(cfgLogin(0))
	for i := 0; i < 100; i++ {
		count(l, "login", "u@example.test")
		mustAllowed(t, l, "login", "u@example.test")
	}
}

func TestUnknownScope_Disabled(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	for i := 0; i < 10; i++ {
		count(l, "nope", "u@example.test")
		mustAllowed(t, l, "nope", "u@example.test")
	}
}

// Mark is the per-key clock used by the email-verification resend cooldown: it
// must record on a scope with NO configured limit (where Fail deliberately
// no-ops) and must never reject anything.
func TestMark_RecordsOnAnUnlimitedScopeAndNeverBlocks(t *testing.T) {
	l, clk := newLimiter(cfgLogin(1))
	const scope, key = "verify-email-sent", "u@example.test"

	if _, ok := l.LastAttempt(scope, key); ok {
		t.Fatal("LastAttempt on a fresh key must report no record")
	}
	// Fail cannot serve this role — the scope has no limit, so it stores nothing.
	count(l, scope, key)
	if _, ok := l.LastAttempt(scope, key); ok {
		t.Fatal("Fail must no-op on an unlimited scope (Mark exists precisely because of this)")
	}

	l.Mark(scope, key)
	at, ok := l.LastAttempt(scope, key)
	if !ok || !at.Equal(clk.t) {
		t.Fatalf("LastAttempt = (%v, %v), want (%v, true)", at, ok, clk.t)
	}
	// Marking never rejects, however often it is called.
	for i := 0; i < 50; i++ {
		l.Mark(scope, key)
		mustAllowed(t, l, scope, key)
	}
	// The newest mark wins, so a cooldown always measures from the latest send.
	clk.t = clk.t.Add(30 * time.Second)
	l.Mark(scope, key)
	if at, _ := l.LastAttempt(scope, key); !at.Equal(clk.t) {
		t.Errorf("LastAttempt = %v, want the most recent mark %v", at, clk.t)
	}
}

// A marked key prunes its own stale timestamps on the next write, so a scope
// used purely as a clock keeps at most the current window per key rather than
// growing with every send.
func TestMark_PrunesItsOwnStaleEntries(t *testing.T) {
	l, clk := newLimiter(cfgLogin(1))
	const scope, key = "verify-email-sent", "u@example.test"

	for i := 0; i < 10; i++ {
		l.Mark(scope, key)
	}
	clk.t = clk.t.Add(2 * time.Hour) // well past the configured window
	l.Mark(scope, key)
	at, ok := l.LastAttempt(scope, key)
	if !ok || !at.Equal(clk.t) {
		t.Fatalf("LastAttempt = (%v, %v), want the fresh mark %v", at, ok, clk.t)
	}
}

func TestIPCap_BlocksAcrossKeysFromOneIP(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, IP: 3,
	})
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustAllowed(t, l, "login", "c@example.test")
	mustBlocked(t, l, "login", "d@example.test") // 4th request, distinct key
}

func TestIPCap_OneIPFillingItLeavesEveryOtherCallerAlone(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, IP: 1,
	})
	mustAllowed(t, l, "login", "a@example.test")
	mustBlocked(t, l, "login", "b@example.test")
	if err := l.Allow(fromIP("198.51.100.7"), "login", "c@example.test"); err != nil {
		t.Fatalf("another IP = %v, want allowed", err)
	}
}

func TestIPCap_ScopesAreIndependent(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{Window: 15 * time.Minute, IP: 1})
	mustAllowed(t, l, "oauth-start", "")
	mustBlocked(t, l, "oauth-start", "")
	mustAllowed(t, l, "register", "")
}

func TestIPCap_ExemptScopeKeepsItsPerKeyLimitOnly(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits:   map[string]int{"sync": 1},
		Window:   15 * time.Minute,
		IP:       1,
		IPExempt: map[string]bool{"sync": true},
	})
	mustAllowed(t, l, "sync", "user-a")
	mustAllowed(t, l, "sync", "user-b") // past IP, distinct key
	count(l, "sync", "user-a")
	mustBlocked(t, l, "sync", "user-a")
	mustAllowed(t, l, "sync", "user-b")
}

func TestIPCap_NoResolvedIPIsNotCounted(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{Window: 15 * time.Minute, IP: 1})
	for range 3 {
		if err := l.Allow(context.Background(), "login", "a@example.test"); err != nil {
			t.Fatalf("no client IP = %v, want allowed", err)
		}
	}
}

func TestIPCap_CountersAreEvictedLikeAnyOther(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{Window: 15 * time.Minute, IP: 1, MaxKeys: 2})
	mustAllowed(t, l, "login", "")
	clk.t = clk.t.Add(time.Second)
	for _, ip := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
		if err := l.Allow(fromIP(ip), "login", ""); err != nil {
			t.Fatalf("Allow from %s = %v", ip, err)
		}
		clk.t = clk.t.Add(time.Second)
	}
	// The stalest counter, ctx's IP, made room: it starts over rather than block.
	mustAllowed(t, l, "login", "")
}

func TestPinnedScope_SurvivesASprayOfUniqueKeys(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits:  map[string]int{"reset": 2, "reset-ceiling": 1},
		Window:  15 * time.Minute,
		MaxKeys: 5,
		Pinned:  map[string]bool{"reset-ceiling": true},
	})
	count(l, "reset-ceiling", "victim@example.test")
	for i := range 50 {
		clk.t = clk.t.Add(time.Second)
		count(l, "reset", "spray"+strings.Repeat("x", i)+"@example.test")
	}
	mustBlocked(t, l, "reset-ceiling", "victim@example.test")

	clk.t = clk.t.Add(15 * time.Minute)
	mustAllowed(t, l, "reset-ceiling", "victim@example.test")
}

func TestIPCap_SlidesOnItsOwnMinuteWindow(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, IP: 2,
	})
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustBlocked(t, l, "login", "c@example.test")
	clk.t = clk.t.Add(61 * time.Second)
	mustAllowed(t, l, "login", "c@example.test")
}

func TestIPCap_ZeroDisables(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, IP: 0,
	})
	for i := 0; i < 200; i++ {
		mustAllowed(t, l, "login", "u@example.test")
	}
}

// A key already over its per-key limit must be rejected WITHOUT consuming the
// global budget — an attack on one account must not starve everyone else.
func TestBlockedKey_DoesNotConsumeGlobal(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 1}, Window: 15 * time.Minute, IP: 3,
	})
	count(l, "login", "victim@example.test")
	for i := 0; i < 50; i++ {
		mustBlocked(t, l, "login", "victim@example.test")
	}
	// Global budget (3) untouched by the blocked calls above.
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustAllowed(t, l, "login", "c@example.test")
	mustBlocked(t, l, "login", "d@example.test")
}

func TestEviction_CapsKeyCount(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 5}, Window: 15 * time.Minute, MaxKeys: 3,
	})
	count(l, "login", "a@example.test")
	clk.t = clk.t.Add(time.Second)
	count(l, "login", "b@example.test")
	clk.t = clk.t.Add(time.Second)
	count(l, "login", "c@example.test")
	clk.t = clk.t.Add(time.Second)
	// Map full (3 keys); inserting a 4th evicts the stalest key (a).
	count(l, "login", "d@example.test")
	// a's counter was evicted: 5 fresh failures needed again to block it.
	mustAllowed(t, l, "login", "a@example.test")
	// d's failure was recorded.
	for i := 0; i < 4; i++ {
		count(l, "login", "d@example.test")
	}
	mustBlocked(t, l, "login", "d@example.test")
}

func TestEviction_PrefersExpiredEntries(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 2}, Window: time.Minute, MaxKeys: 2,
	})
	count(l, "login", "old@example.test")
	clk.t = clk.t.Add(2 * time.Minute) // old@ expires
	count(l, "login", "fresh@example.test")
	count(l, "login", "fresh@example.test")
	// Map at cap; the expired old@ entry is dropped, fresh@ keeps its 2 failures.
	count(l, "login", "new@example.test")
	mustBlocked(t, l, "login", "fresh@example.test")
	mustAllowed(t, l, "login", "new@example.test")
}

// A key longer than the internal hashing threshold must be limited exactly
// like a short one, proving Allow/Fail/Clear agree on the hashed form.
func TestAllow_HugeKeyIsLimitedLikeShortKey(t *testing.T) {
	l, _ := newLimiter(cfgLogin(3))
	huge := strings.Repeat("a", 5*1024*1024) + "@example.test"
	for i := 0; i < 3; i++ {
		mustAllowed(t, l, "login", huge)
		count(l, "login", huge)
	}
	mustBlocked(t, l, "login", huge)
	l.Clear("login", huge)
	mustAllowed(t, l, "login", huge)
}

// Two distinct huge keys must not collapse into the same bucket after hashing.
func TestAllow_TwoHugeKeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	hugeA := strings.Repeat("a", 300*1024)
	hugeB := strings.Repeat("b", 300*1024)
	count(l, "login", hugeA)
	mustBlocked(t, l, "login", hugeA)
	mustAllowed(t, l, "login", hugeB)
}

// go test -race exercises the mutex.
func TestConcurrentAccess(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 3}, Window: 15 * time.Minute, IP: 1000,
	})
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			keys := []string{"a@example.test", "b@example.test", "c@example.test"}
			for i := 0; i < 200; i++ {
				k := keys[(g+i)%len(keys)]
				_ = l.Allow(ctx, "login", k)
				count(l, "login", k)
				if i%10 == 0 {
					l.Clear("login", k)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestReserve_ConcurrentAttemptsCannotOverrunTheLimit(t *testing.T) {
	const limit = 5
	l, _ := newLimiter(cfgLogin(limit))
	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	for range limit * 20 {
		wg.Go(func() {
			if l.Reserve("login", "alice") == nil {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if admitted != limit {
		t.Fatalf("admitted = %d, want %d", admitted, limit)
	}
	mustBlocked(t, l, "login", "alice")
}

func TestReserve_ReleaseGivesTheAttemptBack(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	if err := l.Reserve("login", "alice"); err != nil {
		t.Fatalf("Reserve = %v", err)
	}
	if err := l.Reserve("login", "alice"); err == nil {
		t.Fatal("second Reserve = nil, want 429")
	}
	l.Release("login", "alice")
	mustAllowed(t, l, "login", "alice")
	l.Release("login", "alice") // nothing left to give back
	if err := l.Reserve("login", "alice"); err != nil {
		t.Fatalf("Reserve after release = %v", err)
	}
}

func TestReserve_SpendsNoIPBudgetAndNoUnconfiguredScope(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{Limits: map[string]int{"login": 5}, IP: 1, Window: 15 * time.Minute})
	for range 3 {
		if err := l.Reserve("login", "alice"); err != nil {
			t.Fatalf("Reserve = %v", err)
		}
		if err := l.Reserve("no-limit", "alice"); err != nil {
			t.Fatalf("Reserve on a scope with no limit = %v", err)
		}
	}
	mustAllowed(t, l, "login", "bob")
	if _, ok := l.LastAttempt("no-limit", "alice"); ok {
		t.Fatal("a scope with no limit recorded an attempt")
	}
}

func TestAllowThenReserve_AStressedBurstStopsAtTheLimit(t *testing.T) {
	const limit = 5
	for range 50 {
		l, _ := newLimiter(cfgLogin(limit))
		var wg sync.WaitGroup
		var mu sync.Mutex
		admitted := 0
		start := make(chan struct{})
		for range 1000 {
			wg.Go(func() {
				<-start
				if l.Allow(ctx, "login", "alice") == nil && l.Reserve("login", "alice") == nil {
					mu.Lock()
					admitted++
					mu.Unlock()
				}
			})
		}
		close(start)
		wg.Wait()
		if admitted != limit {
			t.Fatalf("admitted = %d, want %d", admitted, limit)
		}
	}
}
