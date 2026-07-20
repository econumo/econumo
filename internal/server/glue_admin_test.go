package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	adminToken  = "0123456789abcdef0123456789abcdef"
	adminUserA  = "aa000000-0000-0000-0000-0000000000a1"
	adminUserB  = "bb000000-0000-0000-0000-0000000000b1"
	adminUSDID  = "dffc2a06-6f29-4704-8575-31709adee926"
	adminNoSuch = "cc000000-0000-0000-0000-0000000000c1"
)

// newAdminHandler seeds two connected users and returns the private admin
// handler from the real composition root, so these tests exercise middleware,
// handlers, service, glue, and repositories together.
func newAdminHandler(t *testing.T) http.Handler {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: adminUserA, Email: "a@example.test", Name: "Alex"})
	f.User(fixture.User{ID: adminUserB, Email: "b@example.test", Name: "Sam"})
	f.Connect(adminUserA, adminUserB)

	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AdminToken: adminToken}
	_, adminHandler := server.Build(cfg, db.Raw, server.Seams{})
	return adminHandler
}

func adminDo(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func adminData(t *testing.T, rec *httptest.ResponseRecorder) model.AdminUserContextResult {
	t.Helper()
	var env struct {
		Success bool                         `json:"success"`
		Data    model.AdminUserContextResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal %s: %v", rec.Body.String(), err)
	}
	if !env.Success {
		t.Fatalf("envelope not successful: %s", rec.Body.String())
	}
	return env.Data
}

func TestAdminSetAccessRoundTrip(t *testing.T) {
	h := newAdminHandler(t)

	rec := adminDo(t, h, http.MethodPost, "/admin/set-access", adminToken,
		`{"userId":"`+adminUserA+`","level":"readonly","until":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set-access status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+adminUserA, adminToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("user-context status = %d: %s", rec.Code, rec.Body.String())
	}
	got := adminData(t, rec)
	if got.User.AccessLevel != "readonly" || got.User.EffectiveAccessLevel != "readonly" {
		t.Fatalf("user = %+v", got.User)
	}
}

func TestAdminSetAccessWritesExpiry(t *testing.T) {
	h := newAdminHandler(t)
	until := "2027-01-01 00:00:00"

	rec := adminDo(t, h, http.MethodPost, "/admin/set-access", adminToken,
		`{"userId":"`+adminUserA+`","level":"full","until":"`+until+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	got := adminData(t, adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+adminUserA, adminToken, ""))
	if got.User.AccessUntil != until {
		t.Fatalf("accessUntil = %q, want %q", got.User.AccessUntil, until)
	}
	// The expiry is in the future, so the effective level still allows writes.
	if got.User.EffectiveAccessLevel != "full" {
		t.Fatalf("effective = %q, want full", got.User.EffectiveAccessLevel)
	}
}

func TestAdminUserContextIncludesConnections(t *testing.T) {
	h := newAdminHandler(t)
	got := adminData(t, adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+adminUserA, adminToken, ""))

	if got.User.Id != adminUserA || got.User.Email != "a@example.test" {
		t.Fatalf("user = %+v", got.User)
	}
	if len(got.Connections) != 1 {
		t.Fatalf("connections = %+v, want exactly one", got.Connections)
	}
	// The portal prefills a non-editable Stripe checkout from these addresses,
	// so an empty email here would silently break the purchase flow.
	if got.Connections[0].Id != adminUserB || got.Connections[0].Email != "b@example.test" {
		t.Fatalf("connection = %+v", got.Connections[0])
	}
}

func TestAdminRejectsWrongBearer(t *testing.T) {
	h := newAdminHandler(t)

	rec := adminDo(t, h, http.MethodPost, "/admin/set-access", strings.Repeat("x", 32),
		`{"userId":"`+adminUserA+`","level":"readonly"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	// And nothing was written.
	got := adminData(t, adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+adminUserA, adminToken, ""))
	if got.User.AccessLevel != "full" {
		t.Fatalf("accessLevel = %q — a rejected call must not mutate state", got.User.AccessLevel)
	}
}

func TestAdminRejectsMissingBearer(t *testing.T) {
	h := newAdminHandler(t)
	rec := adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+adminUserA, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAdminUnknownUserIs404(t *testing.T) {
	h := newAdminHandler(t)
	rec := adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+adminNoSuch, adminToken, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

// Stripe retries webhooks, so the same call must be safe to apply twice.
func TestAdminSetAccessIsIdempotentOverHTTP(t *testing.T) {
	h := newAdminHandler(t)
	body := `{"userId":"` + adminUserA + `","level":"readonly","until":null}`

	first := adminDo(t, h, http.MethodPost, "/admin/set-access", adminToken, body)
	second := adminDo(t, h, http.MethodPost, "/admin/set-access", adminToken, body)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses %d / %d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("not idempotent:\n%s\n%s", first.Body.String(), second.Body.String())
	}
}
