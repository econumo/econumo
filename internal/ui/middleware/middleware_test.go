package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/econumo/econumo/pkg/jwt"
)

// okHandler is a trivial downstream handler that records that it ran and writes 200.
func okHandler(ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ran != nil {
			*ran = true
		}
		w.WriteHeader(http.StatusOK)
	})
}

// --- RequestID ---

func TestRequestID_SetsHeaderAndContext(t *testing.T) {
	var ctxID string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = RequestIDFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	hdr := rec.Header().Get("X-Request-Id")
	if hdr == "" {
		t.Fatal("X-Request-Id header is empty")
	}
	if ctxID == "" {
		t.Fatal("request id absent from context")
	}
	if ctxID != hdr {
		t.Fatalf("context id %q != header id %q", ctxID, hdr)
	}
}

func TestRequestIDFromCtx_AbsentIsEmpty(t *testing.T) {
	if got := RequestIDFromCtx(context.Background()); got != "" {
		t.Fatalf("RequestIDFromCtx(empty)=%q want \"\"", got)
	}
}

func TestRequestID_IsUUIDv7(t *testing.T) {
	h := RequestID(okHandler(nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	id := rec.Header().Get("X-Request-Id")
	u, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("X-Request-Id %q is not a UUID: %v", id, err)
	}
	if u.Version() != 7 {
		t.Fatalf("X-Request-Id version=%d want 7 (uuidv7)", u.Version())
	}
}

// --- Recover ---

func TestRecover_PanicYields500Exception(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	h := Recover(false)(panicking)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v\n%s", err, rec.Body.String())
	}
	if env["success"] != false {
		t.Fatalf("success=%v want false", env["success"])
	}
	if env["exceptionType"] != "panic" {
		t.Fatalf("exceptionType=%v want panic", env["exceptionType"])
	}
	// Non-dev: no stackTrace key.
	if _, ok := env["stackTrace"]; ok {
		t.Fatalf("stackTrace present in non-dev mode: %s", rec.Body.String())
	}
}

func TestRecover_DevIncludesStackTrace(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	h := Recover(true)(panicking)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	trace, ok := env["stackTrace"].(string)
	if !ok || trace == "" {
		t.Fatalf("dev mode must include a non-empty stackTrace; body: %s", rec.Body.String())
	}
}

func TestRecover_NoPanicPassesThrough(t *testing.T) {
	var ran bool
	h := Recover(false)(okHandler(&ran))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("ran=%v code=%d want true/200", ran, rec.Code)
	}
}

// --- CORS ---

func TestCORS_Preflight_ShortCircuits(t *testing.T) {
	var ran bool
	h := CORS([]string{"https://app.example"})(okHandler(&ran))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/x", nil)
	req.Header.Set("Origin", "https://app.example")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("preflight status=%d want 200", rec.Code)
	}
	if ran {
		t.Fatal("preflight must NOT call the downstream handler")
	}
	hdr := rec.Header()
	if got := hdr.Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("Allow-Origin=%q want https://app.example", got)
	}
	if got := hdr.Get("Access-Control-Allow-Methods"); got != "OPTIONS, POST, GET" {
		t.Fatalf("Allow-Methods=%q", got)
	}
	if got := hdr.Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-Timezone" {
		t.Fatalf("Allow-Headers=%q", got)
	}
	if got := hdr.Get("Access-Control-Max-Age"); got != "3600" {
		t.Fatalf("Max-Age=%q want 3600", got)
	}
}

func TestCORS_NonOptionsPassesThrough(t *testing.T) {
	var ran bool
	h := CORS([]string{"*"})(okHandler(&ran))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if !ran {
		t.Fatal("non-OPTIONS must pass through to the handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin=%q want *", got)
	}
}

func TestCORS_EmptyConfig_NoCORSHeaders(t *testing.T) {
	var ran bool
	h := CORS(nil)(okHandler(&ran))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Origin", "https://other.example")
	h.ServeHTTP(rec, req)
	if !ran {
		t.Fatal("non-OPTIONS must pass through even with no CORS config")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin=%q want empty (same-domain only default)", got)
	}
}

func TestCORS_AllowedOriginReflected(t *testing.T) {
	h := CORS([]string{"https://a.example", "https://b.example"})(okHandler(nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Origin", "https://b.example")
	h.ServeHTTP(rec, req)
	hdr := rec.Header()
	if got := hdr.Get("Access-Control-Allow-Origin"); got != "https://b.example" {
		t.Fatalf("Allow-Origin=%q want https://b.example (reflected)", got)
	}
	if got := hdr.Get("Vary"); got != "Origin" {
		t.Fatalf("Vary=%q want Origin", got)
	}
}

func TestCORS_DisallowedOriginGetsNoHeaders(t *testing.T) {
	h := CORS([]string{"https://a.example"})(okHandler(nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin=%q want empty (origin not in allowlist)", got)
	}
}

// --- Timezone ---

func TestTimezone_ValidHeaderParsedIntoContext(t *testing.T) {
	var loc *time.Location
	h := Timezone(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc = LocationFromCtx(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Timezone", "Europe/Berlin")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if loc == nil || loc.String() != "Europe/Berlin" {
		t.Fatalf("location=%v want Europe/Berlin", loc)
	}
}

func TestTimezone_MissingHeaderDefaultsUTC(t *testing.T) {
	var loc *time.Location
	h := Timezone(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc = LocationFromCtx(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if loc != time.UTC {
		t.Fatalf("location=%v want UTC (missing header)", loc)
	}
}

func TestTimezone_InvalidHeaderDefaultsUTC(t *testing.T) {
	var loc *time.Location
	h := Timezone(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc = LocationFromCtx(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Timezone", "Not/A_Zone")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if loc != time.UTC {
		t.Fatalf("location=%v want UTC (invalid header)", loc)
	}
}

func TestLocationFromCtx_AbsentIsUTC(t *testing.T) {
	if got := LocationFromCtx(context.Background()); got != time.UTC {
		t.Fatalf("LocationFromCtx(empty)=%v want UTC", got)
	}
}

// --- JWT ---

// stubVerifier returns fixed claims / error for the JWT middleware tests.
type stubVerifier struct {
	claims jwt.Claims
	err    error
}

func (s stubVerifier) Verify(token string) (jwt.Claims, error) { return s.claims, s.err }

const jwtTestUserID = "11111111-1111-1111-1111-111111111111"

func TestJWT_MissingHeader_401(t *testing.T) {
	var ran bool
	h := JWT(stubVerifier{}, false)(okHandler(&ran))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
	if ran {
		t.Fatal("downstream handler must not run on missing token")
	}
}

func TestJWT_NonBearerScheme_401(t *testing.T) {
	h := JWT(stubVerifier{}, false)(okHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic abc123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (non-bearer)", rec.Code)
	}
}

func TestJWT_VerifyError_401(t *testing.T) {
	var ran bool
	h := JWT(stubVerifier{err: errors.New("expired")}, false)(okHandler(&ran))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (verify error)", rec.Code)
	}
	if ran {
		t.Fatal("downstream handler must not run when verify fails")
	}
}

func TestJWT_BadClaimID_401(t *testing.T) {
	h := JWT(stubVerifier{claims: jwt.Claims{ID: "not-a-uuid"}}, false)(okHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (unparsable id claim)", rec.Code)
	}
}

func TestJWT_Valid_PutsUserIDInContext(t *testing.T) {
	var gotID string
	var present bool
	h := JWT(stubVerifier{claims: jwt.Claims{ID: jwtTestUserID}}, false)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := UserIDFromCtx(r.Context())
			present = ok
			gotID = id.String()
			w.WriteHeader(http.StatusOK)
		}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	// Scheme match is case-insensitive (RFC 7235).
	req.Header.Set("Authorization", "bEaReR  the.token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	if !present {
		t.Fatal("user id absent from context after valid token")
	}
	if gotID != jwtTestUserID {
		t.Fatalf("ctx user id=%q want %q", gotID, jwtTestUserID)
	}
}

func TestJWT_EmptyBearerToken_401(t *testing.T) {
	h := JWT(stubVerifier{}, false)(okHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer    ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (empty bearer token)", rec.Code)
	}
}

func TestUserIDFromCtx_Absent(t *testing.T) {
	if _, ok := UserIDFromCtx(context.Background()); ok {
		t.Fatal("UserIDFromCtx(empty) reported present")
	}
}

// --- Chain ordering ---

func TestChain_OuterToInnerOrder(t *testing.T) {
	var order []string
	mk := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(mk("a"), mk("b"), mk("c"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "h")
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	want := []string{"a", "b", "c", "h"}
	if len(order) != len(want) {
		t.Fatalf("order=%v want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order=%v want %v", order, want)
		}
	}
}
