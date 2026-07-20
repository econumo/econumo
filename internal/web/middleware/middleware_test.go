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

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
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

func TestSecurityHeaders_SetOnResponse(t *testing.T) {
	var ran bool
	h := SecurityHeaders(okHandler(&ran))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if !ran {
		t.Fatal("downstream handler did not run")
	}
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Content-Security-Policy": "frame-ancestors 'none'",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
}

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
	if got := hdr.Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-Timezone, X-Request-Id" {
		t.Fatalf("Allow-Headers=%q", got)
	}
	if got := hdr.Get("Access-Control-Expose-Headers"); got != "X-Request-Id" {
		t.Fatalf("Expose-Headers=%q want X-Request-Id", got)
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

// stubAuthn returns fixed ids / error for the auth middleware tests.
type stubAuthn struct {
	userID  vo.Id
	tokenID vo.Id
	level   model.AccessLevel
	err     error
}

func (s stubAuthn) Authenticate(_ context.Context, token string) (vo.Id, vo.Id, model.AccessLevel, error) {
	level := s.level
	if level == "" {
		level = model.AccessLevelFull
	}
	return s.userID, s.tokenID, level, s.err
}

var (
	authTestUserID  = vo.MustParseId("11111111-1111-1111-1111-111111111111")
	authTestTokenID = vo.MustParseId("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
)

func authMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v\n%s", err, rec.Body.String())
	}
	msg, _ := env["message"].(string)
	return msg
}

func TestAuth_MissingHeader_401(t *testing.T) {
	var ran bool
	h := Auth(stubAuthn{}, false)(okHandler(&ran))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
	if got := authMessage(t, rec); got != "Access token not found" {
		t.Fatalf("message=%q want %q", got, "Access token not found")
	}
	if ran {
		t.Fatal("downstream handler must not run on missing token")
	}
}

func TestAuth_NonBearerScheme_401(t *testing.T) {
	h := Auth(stubAuthn{}, false)(okHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic abc123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (non-bearer)", rec.Code)
	}
}

func TestAuth_AuthenticateUnauthorized_401(t *testing.T) {
	var ran bool
	h := Auth(stubAuthn{err: errs.NewUnauthorized("Invalid access token")}, false)(okHandler(&ran))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer eco_ses_dead")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (authenticate error)", rec.Code)
	}
	if got := authMessage(t, rec); got != "Invalid access token" {
		t.Fatalf("message=%q want %q", got, "Invalid access token")
	}
	if ran {
		t.Fatal("downstream handler must not run when authentication fails")
	}
}

// A non-Unauthorized authenticator error (e.g. the DB being down) must not
// leak internals: it maps to the generic 401 message.
func TestAuth_InternalError_Generic401(t *testing.T) {
	h := Auth(stubAuthn{err: errors.New("db is down: secret dsn")}, false)(okHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer eco_ses_x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (internal error)", rec.Code)
	}
	if got := authMessage(t, rec); got != "Invalid access token" {
		t.Fatalf("message=%q want %q (no internals leaked)", got, "Invalid access token")
	}
}

func TestAuth_Valid_PutsIdsInContext(t *testing.T) {
	var gotUser, gotToken vo.Id
	var userPresent, tokenPresent bool
	h := Auth(stubAuthn{userID: authTestUserID, tokenID: authTestTokenID}, false)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser, userPresent = UserIDFromCtx(r.Context())
			gotToken, tokenPresent = TokenIDFromCtx(r.Context())
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
	if !userPresent || !gotUser.Equal(authTestUserID) {
		t.Fatalf("ctx user id=%v present=%v want %v", gotUser, userPresent, authTestUserID)
	}
	if !tokenPresent || !gotToken.Equal(authTestTokenID) {
		t.Fatalf("ctx token id=%v present=%v want %v", gotToken, tokenPresent, authTestTokenID)
	}
}

func TestAuth_EmptyBearerToken_401(t *testing.T) {
	h := Auth(stubAuthn{}, false)(okHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer    ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (empty bearer token)", rec.Code)
	}
}

func TestTokenIDFromCtx_Absent(t *testing.T) {
	if _, ok := TokenIDFromCtx(context.Background()); ok {
		t.Fatal("TokenIDFromCtx(empty) reported present")
	}
}

func TestUserIDFromCtx_Absent(t *testing.T) {
	if _, ok := UserIDFromCtx(context.Background()); ok {
		t.Fatal("UserIDFromCtx(empty) reported present")
	}
}

func TestRequireUser_NoUserInContext_401(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)

	id, ok := RequireUser(rec, req)

	if ok {
		t.Fatal("ok=true want false (no user in context)")
	}
	if id != (vo.Id{}) {
		t.Fatalf("id=%v want zero value", id)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v\n%s", err, rec.Body.String())
	}
	if env["success"] != false {
		t.Fatalf("success=%v want false", env["success"])
	}
	if env["message"] != "Access token not found" {
		t.Fatalf("message=%q want %q", env["message"], "Access token not found")
	}
}

func TestRequireUser_UserInContext_ReturnsID(t *testing.T) {
	want := authTestUserID
	ctx := context.WithValue(context.Background(), ctxKeyUserID, want)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)

	id, ok := RequireUser(rec, req)

	if !ok {
		t.Fatal("ok=false want true (user present in context)")
	}
	if id != want {
		t.Fatalf("id=%v want %v", id, want)
	}
	// httptest.ResponseRecorder defaults Code to 200 until WriteHeader is
	// called; an empty body is the reliable signal that RequireUser wrote
	// nothing.
	if rec.Body.Len() != 0 {
		t.Fatalf("body=%q want empty (nothing written)", rec.Body.String())
	}
}

func TestLanguageMiddleware(t *testing.T) {
	cases := []struct{ header, want string }{
		{"", "en"},
		{"ru", "ru"},
		{"ru-RU,ru;q=0.9,en;q=0.8", "ru"},
		{"de-DE,de;q=0.9", "en"},
		{"EN-us", "en"},
	}
	for _, tc := range cases {
		var got string
		h := Language([]string{"en", "ru"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = reqctx.Language(r.Context())
		}))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			r.Header.Set("Accept-Language", tc.header)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if got != tc.want {
			t.Errorf("header %q: language = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func readonlyStub() stubAuthn {
	return stubAuthn{userID: authTestUserID, tokenID: authTestTokenID, level: model.AccessLevelReadonly}
}

func fullStub() stubAuthn {
	return stubAuthn{userID: authTestUserID, tokenID: authTestTokenID, level: model.AccessLevelFull}
}

func authRequest(t *testing.T, method, path string, authn stubAuthn) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	ran := false
	h := Auth(authn, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer the.token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, ran
}

func TestAuth_ReadonlyBlocksWrites(t *testing.T) {
	rec, ran := authRequest(t, http.MethodPost, "/api/v1/category/create-category", readonlyStub())
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", rec.Code)
	}
	if ran {
		t.Fatal("handler ran despite read-only access")
	}
	if msg := authMessage(t, rec); msg != "Read-only access. Write operations are disabled." {
		t.Fatalf("message = %q", msg)
	}
}

func TestAuth_ReadonlyAllowsReads(t *testing.T) {
	rec, ran := authRequest(t, http.MethodGet, "/api/v1/account/get-account-list", readonlyStub())
	if rec.Code != http.StatusOK || !ran {
		t.Fatalf("GET should pass: status %d ran %v", rec.Code, ran)
	}
}

func TestAuth_ReadonlyAllowlistedWritesPass(t *testing.T) {
	for _, path := range []string{
		"/api/v1/user/logout-user",
		"/api/v1/user/revoke-session",
		"/api/v1/user/revoke-other-sessions",
		"/api/v1/user/revoke-personal-token",
		"/api/v1/user/update-password",
	} {
		t.Run(path, func(t *testing.T) {
			rec, ran := authRequest(t, http.MethodPost, path, readonlyStub())
			if rec.Code != http.StatusOK || !ran {
				t.Fatalf("allowlisted path blocked: status %d ran %v", rec.Code, ran)
			}
		})
	}
}

// A read-only user is exactly the person who needs the payment link; a 402
// here would be a dead end with no way to restore access.
func TestReadonlyReachesBillingLink(t *testing.T) {
	rec, ran := authRequest(t, http.MethodPost, "/api/v1/user/create-billing-link", readonlyStub())
	if !ran || rec.Code == http.StatusPaymentRequired {
		t.Fatalf("billing link blocked for a read-only user: status %d ran %v", rec.Code, ran)
	}
}

func TestAuth_CreatePersonalTokenIsNotAllowlisted(t *testing.T) {
	rec, ran := authRequest(t, http.MethodPost, "/api/v1/user/create-personal-token", readonlyStub())
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 (a PAT mints new write-capable credentials)", rec.Code)
	}
	if ran {
		t.Fatal("handler ran")
	}
}

func TestAuth_FullUserWritesPass(t *testing.T) {
	rec, ran := authRequest(t, http.MethodPost, "/api/v1/category/create-category", fullStub())
	if rec.Code != http.StatusOK || !ran {
		t.Fatalf("full user blocked: status %d ran %v", rec.Code, ran)
	}
}

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
