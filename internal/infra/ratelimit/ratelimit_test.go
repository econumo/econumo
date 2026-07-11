package ratelimit_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/shared/errs"
)

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

func mustAllowed(t *testing.T, l *ratelimit.Limiter, scope, key string) {
	t.Helper()
	if err := l.Allow(scope, key); err != nil {
		t.Fatalf("Allow(%s,%s) = %v, want nil", scope, key, err)
	}
}

func mustBlocked(t *testing.T, l *ratelimit.Limiter, scope, key string) {
	t.Helper()
	err := l.Allow(scope, key)
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

func TestAllow_UnderLimit(t *testing.T) {
	l, _ := newLimiter(cfgLogin(3))
	for i := 0; i < 2; i++ {
		mustAllowed(t, l, "login", "user@example.test")
		l.Fail("login", "user@example.test")
	}
	mustAllowed(t, l, "login", "user@example.test") // 2 failures < 3
}

func TestAllow_BlocksAtLimit(t *testing.T) {
	l, _ := newLimiter(cfgLogin(3))
	for i := 0; i < 3; i++ {
		mustAllowed(t, l, "login", "user@example.test")
		l.Fail("login", "user@example.test")
	}
	mustBlocked(t, l, "login", "user@example.test")
}

func TestAllow_KeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	l.Fail("login", "a@example.test")
	mustBlocked(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
}

func TestAllow_ScopesAreIndependent(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 1, "reset": 1}, Window: 15 * time.Minute,
	})
	l.Fail("login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	mustAllowed(t, l, "reset", "u@example.test")
}

func TestAllow_WindowSlides(t *testing.T) {
	l, clk := newLimiter(cfgLogin(2))
	l.Fail("login", "u@example.test")
	clk.t = clk.t.Add(10 * time.Minute)
	l.Fail("login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	clk.t = clk.t.Add(6 * time.Minute) // first failure now 16m old -> expired
	mustAllowed(t, l, "login", "u@example.test")
	clk.t = clk.t.Add(10 * time.Minute) // second failure now expired too
	mustAllowed(t, l, "login", "u@example.test")
}

func TestClear_ResetsCounter(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	l.Fail("login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	l.Clear("login", "u@example.test")
	mustAllowed(t, l, "login", "u@example.test")
}

func TestZeroLimit_Disables(t *testing.T) {
	l, _ := newLimiter(cfgLogin(0))
	for i := 0; i < 100; i++ {
		l.Fail("login", "u@example.test")
		mustAllowed(t, l, "login", "u@example.test")
	}
}

func TestUnknownScope_Disabled(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	for i := 0; i < 10; i++ {
		l.Fail("nope", "u@example.test")
		mustAllowed(t, l, "nope", "u@example.test")
	}
}

func TestGlobalCap_BlocksAcrossKeys(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, Global: 3,
	})
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustAllowed(t, l, "login", "c@example.test")
	mustBlocked(t, l, "login", "d@example.test") // 4th request, distinct key
}

func TestGlobalCap_SlidesOnItsOwnMinuteWindow(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, Global: 2,
	})
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustBlocked(t, l, "login", "c@example.test")
	clk.t = clk.t.Add(61 * time.Second)
	mustAllowed(t, l, "login", "c@example.test")
}

func TestGlobalCap_ZeroDisables(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, Global: 0,
	})
	for i := 0; i < 200; i++ {
		mustAllowed(t, l, "login", "u@example.test")
	}
}

// A key already over its per-key limit must be rejected WITHOUT consuming the
// global budget — an attack on one account must not starve everyone else.
func TestBlockedKey_DoesNotConsumeGlobal(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 1}, Window: 15 * time.Minute, Global: 3,
	})
	l.Fail("login", "victim@example.test")
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
	l.Fail("login", "a@example.test")
	clk.t = clk.t.Add(time.Second)
	l.Fail("login", "b@example.test")
	clk.t = clk.t.Add(time.Second)
	l.Fail("login", "c@example.test")
	clk.t = clk.t.Add(time.Second)
	// Map full (3 keys); inserting a 4th evicts the stalest key (a).
	l.Fail("login", "d@example.test")
	// a's counter was evicted: 5 fresh failures needed again to block it.
	mustAllowed(t, l, "login", "a@example.test")
	// d's failure was recorded.
	for i := 0; i < 4; i++ {
		l.Fail("login", "d@example.test")
	}
	mustBlocked(t, l, "login", "d@example.test")
}

func TestEviction_PrefersExpiredEntries(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 2}, Window: time.Minute, MaxKeys: 2,
	})
	l.Fail("login", "old@example.test")
	clk.t = clk.t.Add(2 * time.Minute) // old@ expires
	l.Fail("login", "fresh@example.test")
	l.Fail("login", "fresh@example.test")
	// Map at cap; the expired old@ entry is dropped, fresh@ keeps its 2 failures.
	l.Fail("login", "new@example.test")
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
		l.Fail("login", huge)
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
	l.Fail("login", hugeA)
	mustBlocked(t, l, "login", hugeA)
	mustAllowed(t, l, "login", hugeB)
}

// go test -race exercises the mutex.
func TestConcurrentAccess(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 3}, Window: 15 * time.Minute, Global: 1000,
	})
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			keys := []string{"a@example.test", "b@example.test", "c@example.test"}
			for i := 0; i < 200; i++ {
				k := keys[(g+i)%len(keys)]
				_ = l.Allow("login", k)
				l.Fail("login", k)
				if i%10 == 0 {
					l.Clear("login", k)
				}
			}
		}(g)
	}
	wg.Wait()
}
