package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

const (
	strangerIP = "198.51.100.1"
	ownerIP    = "203.0.113.7"
)

type limitHarness struct {
	t        *testing.T
	ts       *httptest.Server
	rawToken string
}

// The test server's peer is loopback and trusted, so X-Forwarded-For names the
// caller the way it does behind a reverse proxy.
func (h limitHarness) post(path, ip, token, body string) int {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.ts.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func newLimitHarness(t *testing.T, mutate func(*config.Config)) limitHarness {
	t.Helper()
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	h := limitHarness{t: t, rawToken: "eco_ses_ratelimit-wiring-token"}
	userID := f.User(fixture.User{})
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{
		UserID:    userID,
		Kind:      "session",
		TokenHash: appuser.HashAccessToken(h.rawToken),
		UserAgent: "ratelimit-test",
		ExpiresAt: &exp,
	})
	const perKey = 100
	cfg := config.Config{
		DatabaseDriver:              db.Engine,
		CurrencyBase:                "USD",
		AllowRegistration:           true,
		ImportSimpleFIN:             true,
		AppURL:                      "https://app.example.test",
		RateLimitLogin:              perKey,
		RateLimitReset:              perKey,
		RateLimitRemind:             perKey,
		RateLimitRegister:           perKey,
		RateLimitRequestEmailChange: perKey,
		RateLimitConfirmEmailChange: perKey,
		RateLimitAccept:             perKey,
		RateLimitIngest:             perKey,
		RateLimitClaimSetupToken:    perKey,
		RateLimitSync:               perKey,
		RateLimitPreviewRule:        perKey,
		RateLimitWindow:             15 * time.Minute,
		RateLimitIP:                 1,
		ClientIPHeader:              "X-Forwarded-For",
		ClientIPTrustedProxies:      []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	h.ts = httptest.NewServer(server.BuildAPI(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}))
	t.Cleanup(h.ts.Close)
	return h
}

func TestIPRateCap_OneCallerNeverThrottlesAnother(t *testing.T) {
	h := newLimitHarness(t, nil)
	capped := []struct{ name, path, token, bodyFormat string }{
		{"login", "/api/v1/user/login-user", "", `{"username":"anon%d@example.test","password":"wrong"}`},
		{"remind", "/api/v1/user/remind-password", "", `{"username":"anon%d@example.test"}`},
		{"register", "/api/v1/user/register-user", "", `{"email":"anon%d@example.test","password":"long-enough-password","name":"Anon"}`},
		{"reset", "/api/v1/user/reset-password", "", `{"username":"anon%d@example.test","code":"000000","password":"irrelevant-pw"}`},
		{"oauth-start", "/api/v1/oauth/start-login", "", `{"provider":"google","client":"web","attempt":%d}`},
		{"oauth-register", "/oauth/register", "", `{"redirect_uris":["https://client%d.example.test/cb"],"client_name":"x","token_endpoint_auth_method":"none"}`},
		{"accept-invite", "/api/v1/connection/accept-invite", h.rawToken, `{"code":"00000%d"}`},
		{"request-email-change", "/api/v1/user/request-email-change", h.rawToken, `{"newEmail":"new%d@example.test","password":"wrong-password"}`},
		{"claim-setup-token", "/api/v1/import/claim-setup-token", h.rawToken, `{"setupToken":"aGVsbG8%d"}`},
		{"sync-source", "/api/v1/import/sync-source", h.rawToken, `{"sourceId":"0190a000-0000-7000-8000-00000000000%d","accessUrl":"https://bridge.invalid/simplefin","startDate":"2026-10-01"}`},
	}
	for _, tc := range capped {
		t.Run(tc.name, func(t *testing.T) {
			h.post(tc.path, strangerIP, tc.token, fmt.Sprintf(tc.bodyFormat, 1))
			if got := h.post(tc.path, strangerIP, tc.token, fmt.Sprintf(tc.bodyFormat, 2)); got != http.StatusTooManyRequests {
				t.Fatalf("same IP's second call = %d, want 429", got)
			}
			if got := h.post(tc.path, ownerIP, tc.token, fmt.Sprintf(tc.bodyFormat, 3)); got == http.StatusTooManyRequests {
				t.Fatalf("another IP was throttled by the first one")
			}
		})
	}
}

func TestIPRateCap_AnUntrustedPeerCannotForgeItsWayPast(t *testing.T) {
	h := newLimitHarness(t, func(c *config.Config) {
		c.ClientIPTrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	})
	const path, body = "/api/v1/user/remind-password", `{"username":"anon@example.test"}`
	h.post(path, strangerIP, "", body)
	if got := h.post(path, ownerIP, "", body); got != http.StatusTooManyRequests {
		t.Fatalf("second call with a different forged IP = %d, want 429", got)
	}
}

func TestIPRateCap_ExemptsEndpointsAHouseholdCanFillAlone(t *testing.T) {
	h := newLimitHarness(t, nil)
	exempt := []struct{ name, path, body string }{
		{"ingest", "/api/v1/import/ingest-apple-wallet-event", `{"account":"x","amount":"1","currency":"USD"}`},
		{"preview-rule", "/api/v1/import/preview-rule", `{"action":"skip","matchField":"external_payee","matchType":"contains","matchValue":"Blue Bottle","scope":"all"}`},
	}
	for _, tc := range exempt {
		t.Run(tc.name, func(t *testing.T) {
			h.post(tc.path, ownerIP, h.rawToken, tc.body)
			if got := h.post(tc.path, ownerIP, h.rawToken, tc.body); got == http.StatusTooManyRequests {
				t.Fatalf("second call from one IP = 429, want it past the cap")
			}
		})
	}
}

func TestEmailLimits_AStrangerCannotLockOutTheOwner(t *testing.T) {
	const password = "correct-horse-battery"
	h := newLimitHarness(t, func(c *config.Config) {
		c.RateLimitLogin = 2
		c.RateLimitIP = 0
	})
	if got := h.post("/api/v1/user/register-user", ownerIP, "",
		fmt.Sprintf(`{"email":"victim@example.test","password":%q,"name":"Victim"}`, password)); got != http.StatusOK {
		t.Fatalf("register = %d", got)
	}
	login := func(ip, pw string) int {
		body, _ := json.Marshal(map[string]string{"username": "victim@example.test", "password": pw})
		return h.post("/api/v1/user/login-user", ip, "", string(body))
	}

	for range 2 {
		login(strangerIP, "wrong")
	}
	if got := login(strangerIP, password); got != http.StatusTooManyRequests {
		t.Fatalf("stranger's IP after its limit = %d, want 429", got)
	}
	if got := login(ownerIP, password); got != http.StatusOK {
		t.Fatalf("owner from their own IP = %d, want 200", got)
	}

	// The ceiling across IPs (10x login) still stops guessing from many of them.
	for i := range 20 {
		login(fmt.Sprintf("198.51.100.%d", i+10), "wrong")
	}
	if got := login("198.51.100.200", "wrong"); got != http.StatusTooManyRequests {
		t.Fatalf("past the ceiling = %d, want 429", got)
	}
}

func TestEmailLimits_RetryingALockedAddressSpendsNoIPBudget(t *testing.T) {
	h := newLimitHarness(t, func(c *config.Config) {
		c.RateLimitLogin = 1
		c.RateLimitIP = 2
	})
	login := func(ip, email string) int {
		return h.post("/api/v1/user/login-user", ip, "", fmt.Sprintf(`{"username":%q,"password":"wrong"}`, email))
	}
	for i := range 10 { // the ceiling, 10x login
		login(fmt.Sprintf("198.51.100.%d", i+10), "locked@example.test")
	}
	for range 5 {
		if got := login(ownerIP, "locked@example.test"); got != http.StatusTooManyRequests {
			t.Fatalf("locked address = %d, want 429", got)
		}
	}
	if got := login(ownerIP, "someone-else@example.test"); got == http.StatusTooManyRequests {
		t.Fatal("retries against a locked address used up the IP's budget")
	}
}

func TestEmailLimits_ACeilingDoesNotRevealWhichAccountsExist(t *testing.T) {
	h := newLimitHarness(t, func(c *config.Config) {
		c.RateLimitReset = 2
		c.RateLimitIP = 0
	})
	if got := h.post("/api/v1/user/register-user", ownerIP, "", `{"email":"real@example.test","password":"long-enough-password","name":"Real"}`); got != http.StatusOK {
		t.Fatalf("register = %d", got)
	}
	for _, email := range []string{"real@example.test", "nobody@example.test"} {
		reset := func(ip string) int {
			return h.post("/api/v1/user/reset-password", ip, "", fmt.Sprintf(`{"username":%q,"code":"000000","password":"irrelevant-pw"}`, email))
		}
		for i := range 4 { // 2x the limit
			if got := reset(fmt.Sprintf("198.51.100.%d", i+10)); got == http.StatusTooManyRequests {
				t.Fatalf("%s: guess %d = 429 before the ceiling", email, i)
			}
		}
		if got := reset("198.51.100.200"); got != http.StatusTooManyRequests {
			t.Fatalf("%s: past the ceiling = %d, want 429", email, got)
		}
	}
}

// burst fires every request at once and counts those that were not refused with
// a 429, each of which got its guess tested.
func (h limitHarness) burst(path, token, body string, ips []string) int {
	h.t.Helper()
	var wg sync.WaitGroup
	var admitted atomic.Int64
	start := make(chan struct{})
	for _, ip := range ips {
		wg.Go(func() {
			<-start
			req, err := http.NewRequest(http.MethodPost, h.ts.URL+path, strings.NewReader(body))
			if err != nil {
				h.t.Error(err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", ip)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				h.t.Error(err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusTooManyRequests {
				admitted.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	return int(admitted.Load())
}

func manyIPs(n int) []string {
	ips := make([]string, n)
	for i := range ips {
		ips[i] = fmt.Sprintf("198.51.%d.%d", 100+i/250, i%250+1)
	}
	return ips
}

func TestEmailLimits_ABurstAcrossIPsGetsNoMoreGuessesThanTheCeiling(t *testing.T) {
	const victim = "victim@example.test"
	cases := []struct {
		name, path, body string
		mutate           func(*config.Config)
		ceiling          int
	}{
		{"login", "/api/v1/user/login-user", `{"username":"victim@example.test","password":"wrong"}`,
			func(c *config.Config) { c.RateLimitLogin = 2 }, 20},
		{"reset", "/api/v1/user/reset-password", `{"username":"victim@example.test","code":"000000","password":"irrelevant-pw"}`,
			func(c *config.Config) { c.RateLimitReset = 2 }, 4},
		{"confirm-email", "/api/v1/user/confirm-email", `{"username":"victim@example.test","code":"000000"}`,
			func(c *config.Config) { c.RateLimitConfirmEmail = 2 }, 4},
		// An address with no account hits the same limit at the same point.
		{"reset, unknown address", "/api/v1/user/reset-password", `{"username":"nobody@example.test","code":"000000","password":"irrelevant-pw"}`,
			func(c *config.Config) { c.RateLimitReset = 2 }, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newLimitHarness(t, func(c *config.Config) {
				c.RateLimitIP = 0
				tc.mutate(c)
			})
			if got := h.post("/api/v1/user/register-user", ownerIP, "",
				fmt.Sprintf(`{"email":%q,"password":"long-enough-password","name":"Victim"}`, victim)); got != http.StatusOK {
				t.Fatalf("register = %d", got)
			}
			if got := h.burst(tc.path, "", tc.body, manyIPs(tc.ceiling*8)); got != tc.ceiling {
				t.Fatalf("guesses admitted = %d, want the ceiling %d", got, tc.ceiling)
			}
		})
	}
}

func TestConfirmEmailChange_ABurstAcrossIPsGetsNoMoreGuessesThanItsLimit(t *testing.T) {
	const limit = 3
	h := newLimitHarness(t, func(c *config.Config) {
		c.RateLimitIP = 0
		c.RateLimitConfirmEmailChange = limit
	})
	if got := h.burst("/api/v1/user/confirm-email-change", h.rawToken, `{"code":"000000"}`, manyIPs(limit*8)); got != limit {
		t.Fatalf("guesses admitted = %d, want the limit %d", got, limit)
	}
}

func TestEmailLimits_ABurstFromOneIPCannotLockOutTheOwner(t *testing.T) {
	const password = "correct-horse-battery"
	h := newLimitHarness(t, func(c *config.Config) {
		c.RateLimitLogin = 2
		c.RateLimitIP = 0
	})
	if got := h.post("/api/v1/user/register-user", ownerIP, "",
		fmt.Sprintf(`{"email":"victim@example.test","password":%q,"name":"Victim"}`, password)); got != http.StatusOK {
		t.Fatalf("register = %d", got)
	}
	ips := make([]string, 64)
	for i := range ips {
		ips[i] = strangerIP
	}
	if got := h.burst("/api/v1/user/login-user", "", `{"username":"victim@example.test","password":"wrong"}`, ips); got != 2 {
		t.Fatalf("guesses admitted from one IP = %d, want its limit 2", got)
	}
	body := fmt.Sprintf(`{"username":"victim@example.test","password":%q}`, password)
	if got := h.post("/api/v1/user/login-user", ownerIP, "", body); got != http.StatusOK {
		t.Fatalf("owner from their own IP = %d, want 200", got)
	}
}

// A correct password held at the email-verification gate is no failed guess, so
// retrying it must not use up the attempt it reserved.
func TestEmailLimits_AnAttemptThatIsNotAFailureGivesItsCountBack(t *testing.T) {
	const password = "correct-horse-battery"
	h := newLimitHarness(t, func(c *config.Config) {
		c.RateLimitLogin = 1
		c.RateLimitVerifyEmail = 100
		c.RateLimitIP = 0
		c.EmailVerification = true
	})
	if got := h.post("/api/v1/user/register-user", ownerIP, "",
		fmt.Sprintf(`{"email":"unverified@example.test","password":%q,"name":"Unverified"}`, password)); got != http.StatusOK {
		t.Fatalf("register = %d", got)
	}
	body := fmt.Sprintf(`{"username":"unverified@example.test","password":%q}`, password)
	for i := range 3 {
		if got := h.post("/api/v1/user/login-user", ownerIP, "", body); got == http.StatusTooManyRequests || got == http.StatusOK {
			t.Fatalf("login %d = %d, want the verification denial", i, got)
		}
	}
}

func TestEmailLimits_ResetCodeGuessesStayNearTheirLimitAcrossIPs(t *testing.T) {
	h := newLimitHarness(t, func(c *config.Config) {
		c.RateLimitReset = 2
		c.RateLimitIP = 0
	})
	if got := h.post("/api/v1/user/register-user", ownerIP, "", `{"email":"victim@example.test","password":"long-enough-password","name":"Victim"}`); got != http.StatusOK {
		t.Fatalf("register = %d", got)
	}
	reset := func(ip string) int {
		return h.post("/api/v1/user/reset-password", ip, "", `{"username":"victim@example.test","code":"000000","password":"irrelevant-pw"}`)
	}
	for i := range 4 { // 2x the limit
		if got := reset(fmt.Sprintf("198.51.100.%d", i+10)); got == http.StatusTooManyRequests {
			t.Fatalf("guess %d = 429 before the ceiling", i)
		}
	}
	if got := reset("198.51.100.200"); got != http.StatusTooManyRequests {
		t.Fatalf("past the ceiling = %d, want 429", got)
	}
}
