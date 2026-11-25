package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/handoff"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

const billingAccessPath = "/api/v1/user/get-billing-access"

type billingAccessEnv struct {
	api    http.Handler
	admin  http.Handler
	tokenA string
}

// newBillingAccessEnv seeds user A connected to user B and builds the real
// composition root against billingURL.
func newBillingAccessEnv(t *testing.T, billingURL string, hc *http.Client) billingAccessEnv {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: adminUserA, Email: "a@example.test", Name: "Alex"})
	f.User(fixture.User{ID: adminUserB, Email: "b@example.test", Name: "Sam"})
	f.DefaultOptions(adminUserA)
	f.Connect(adminUserA, adminUserB)
	token := "eco_ses_" + uuid.NewString()
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{UserID: adminUserA, Kind: "session", TokenHash: appuser.HashAccessToken(token), ExpiresAt: &exp})
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AdminToken: adminToken, BillingURL: billingURL}
	api, admin, _, err := server.Build(cfg, db.Raw, server.Seams{BillingHTTPClient: hc})
	if err != nil {
		t.Fatal(err)
	}
	return billingAccessEnv{api: api, admin: admin, tokenA: token}
}

func (e billingAccessEnv) get(t *testing.T) string {
	t.Helper()
	rec := adminDo(t, e.api, http.MethodGet, billingAccessPath, e.tokenA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

type billingCalls struct {
	mu     sync.Mutex
	bodies []string
	uids   []string
}

func (c *billingCalls) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func TestGetBillingAccessAsksBillingForTheUserAndTheirConnections(t *testing.T) {
	calls := &billingCalls{}
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/portal/api/v1/econumo/get-access" {
			http.NotFound(w, r)
			return
		}
		uid, err := handoff.VerifyAccess(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), adminToken, time.Now())
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		calls.mu.Lock()
		calls.bodies = append(calls.bodies, string(raw))
		calls.uids = append(calls.uids, uid)
		calls.mu.Unlock()
		_, _ = io.WriteString(w, `{"success":true,"message":"","data":{`+
			`"user":{"paid":true,"renewing":true,"renewsAt":"2026-12-24 10:00:00","endsAt":"","paymentFailed":false,"discountUntil":"2027-11-24 10:00:00"},`+
			`"connections":[{"id":"`+adminUserB+`","paid":true,"renewing":false},{"id":"`+adminNoSuch+`","paid":true,"renewing":true}]}}`)
	}))
	defer billing.Close()
	env := newBillingAccessEnv(t, billing.URL+"/portal/", nil)

	want := `{"success":true,"message":"","data":{"available":true,` +
		`"user":{"paid":true,"renewing":true,"renewsAt":"2026-12-24 10:00:00","endsAt":"","paymentFailed":false,"discountUntil":"2027-11-24 10:00:00"},` +
		`"connections":[{"id":"` + adminUserB + `","paid":true,"renewing":false}]}}`
	if got := env.get(t); strings.TrimSpace(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if calls.count() != 1 || calls.bodies[0] != `{"connections":["`+adminUserB+`"]}` || calls.uids[0] != adminUserA {
		t.Fatalf("billing saw %v from %v", calls.bodies, calls.uids)
	}

	env.get(t)
	if calls.count() != 1 {
		t.Fatalf("billing called %d times, want the second read cached", calls.count())
	}

	// A purchase reaches Econumo through set-access; the next read asks again.
	rec := adminDo(t, env.admin, http.MethodPost, "/admin/set-access", adminToken,
		`{"userId":"`+adminUserA+`","level":"full","until":"2027-01-31 00:00:00"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set-access status = %d: %s", rec.Code, rec.Body.String())
	}
	env.get(t)
	if calls.count() != 2 {
		t.Fatalf("billing called %d times, want a refetch after the access changed", calls.count())
	}
}

const unavailableBillingAccess = `{"success":true,"message":"","data":{"available":false,"user":null,"connections":[]}}`

func TestGetBillingAccessIsUnavailableWhenBillingFails(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"older billing without the route": http.NotFound,
		"error envelope": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"success":false,"message":"Access token not found","code":0,"errors":{}}`)
		},
		"bad json": func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"success":true,`) },
	} {
		t.Run(name, func(t *testing.T) {
			billing := httptest.NewServer(handler)
			defer billing.Close()
			env := newBillingAccessEnv(t, billing.URL, nil)
			if got := strings.TrimSpace(env.get(t)); got != unavailableBillingAccess {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestGetBillingAccessIsUnavailableWhenBillingTimesOut(t *testing.T) {
	release := make(chan struct{})
	billing := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer billing.Close()
	defer close(release)
	env := newBillingAccessEnv(t, billing.URL, &http.Client{Timeout: 50 * time.Millisecond})
	if got := strings.TrimSpace(env.get(t)); got != unavailableBillingAccess {
		t.Fatalf("got %s", got)
	}
}

type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("billing called at %s without ECONUMO_BILLING_URL", r.URL)
	return nil, http.ErrHandlerTimeout
}

func TestGetBillingAccessWithoutBillingMakesNoCall(t *testing.T) {
	env := newBillingAccessEnv(t, "", &http.Client{Transport: failingTransport{t}})
	if got := strings.TrimSpace(env.get(t)); got != unavailableBillingAccess {
		t.Fatalf("got %s", got)
	}
}
