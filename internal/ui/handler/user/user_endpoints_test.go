package user_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// loginResult is the {token, user} login response data.
type loginResult struct {
	Token string      `json:"token"`
	User  currentUser `json:"user"`
}

func TestLoginUser_Success(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail,
		"password": seedPassword,
	})

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	// Login is the one endpoint that does NOT use the {success,message,data}
	// envelope: PHP returns `new JsonResponse($result)` and the SPA reads
	// response.token off the TOP LEVEL. So the body is the raw {token,user}, with
	// no "success"/"data" keys — assert against env.raw, not env.Data.
	if env.Success || env.Data != nil {
		t.Fatalf("login must NOT be enveloped (no success/data keys); body: %s", env.raw)
	}
	res := mustUnmarshal[loginResult](t, env.raw)
	if res.Token == "" {
		t.Fatal("expected a non-empty token")
	}
	// The token must verify against the real public key and carry the user id.
	claims, err := h.jwt.Verify(res.Token)
	if err != nil {
		t.Fatalf("verify issued token: %v", err)
	}
	if claims.ID != seedUserID {
		t.Fatalf("token id = %q, want %q", claims.ID, seedUserID)
	}
	if claims.Username != seedEmail {
		t.Fatalf("token username = %q, want %q", claims.Username, seedEmail)
	}
	// User shape.
	if res.User.ID != seedUserID {
		t.Fatalf("user.id = %q, want %q", res.User.ID, seedUserID)
	}
	if res.User.Email != seedEmail {
		t.Fatalf("user.email = %q (want decoded plaintext %q)", res.User.Email, seedEmail)
	}
	// currency_id synthetic option resolved to the seeded USD currency.
	cid, ok := res.User.optionValue("currency_id")
	if !ok {
		t.Fatal("expected synthetic currency_id option")
	}
	if cid == nil || *cid != usdCurrencyID {
		t.Fatalf("currency_id = %v, want %q", cid, usdCurrencyID)
	}
}

func TestLoginUser_BadPassword_401(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail,
		"password": "wrong-password",
	})

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
	// Error envelope always carries an errors object (possibly empty).
	if env.Errors == nil {
		t.Fatalf("expected errors object present (even if empty); body: %s", env.raw)
	}
}

func TestLoginUser_BlankFields_400(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": "",
		"password": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if _, ok := env.errorsMap()["username"]; !ok {
		t.Fatalf("expected a username field error; body: %s", env.raw)
	}
	if _, ok := env.errorsMap()["password"]; !ok {
		t.Fatalf("expected a password field error; body: %s", env.raw)
	}
}

func TestGetUserData_NoToken_401(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
}

func TestGetUserData_InvalidToken_401(t *testing.T) {
	h := newHarness(t)

	status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", "not-a-jwt", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestGetUserData_WithToken_200(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	if !env.Success {
		t.Fatalf("success=false; body: %s", env.raw)
	}
	wrapper := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)
	u := wrapper.User
	if u.ID != seedUserID {
		t.Fatalf("user.id = %q, want %q", u.ID, seedUserID)
	}
	if u.Email != seedEmail {
		t.Fatalf("user.email = %q, want %q", u.Email, seedEmail)
	}
	cid, ok := u.optionValue("currency_id")
	if !ok || cid == nil || *cid != usdCurrencyID {
		t.Fatalf("currency_id option = %v (ok=%v), want %q", cid, ok, usdCurrencyID)
	}
}

func TestRegisterUser_NoToken_CreatesUser(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/register-user", "", map[string]string{
		"email":    "fresh@example.test",
		"password": "hunter2",
		"name":     "Fresh",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	// Register returns the user WITHOUT a token: the data object must have a
	// "user" key and must NOT have a "token" key (distinct from login).
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &probe); err != nil {
		t.Fatalf("decode data: %v; body: %s", err, env.raw)
	}
	if _, hasToken := probe["token"]; hasToken {
		t.Fatalf("register response must NOT include a token; body: %s", env.raw)
	}
	if _, hasUser := probe["user"]; !hasUser {
		t.Fatalf("register response must include a user; body: %s", env.raw)
	}

	res := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)
	if res.User.Email != "fresh@example.test" {
		t.Fatalf("user.email = %q, want %q", res.User.Email, "fresh@example.test")
	}
	if res.User.Name != "Fresh" {
		t.Fatalf("user.name = %q, want Fresh", res.User.Name)
	}

	// The new user can subsequently log in (proves the row + hashed password
	// + encrypted email + identifier were all written correctly).
	st2, env2 := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": "fresh@example.test",
		"password": "hunter2",
	})
	if st2 != http.StatusOK {
		t.Fatalf("login after register: status = %d; body: %s", st2, env2.raw)
	}
}

// TestRegisterUser_DoesNotAutoConnect locks the behaviour after removing
// ECONUMO_CONNECT_USERS: a newly registered user is never auto-connected to any
// existing user. The harness already seeds one user; after registering another,
// the users_connections join table must stay empty (connections are created only
// by accepting an invite).
func TestRegisterUser_DoesNotAutoConnect(t *testing.T) {
	h := newHarness(t)

	if st, env := h.do(t, http.MethodPost, "/api/v1/user/register-user", "", map[string]string{
		"email":    "second@example.test",
		"password": "hunter2",
		"name":     "Second",
	}); st != http.StatusOK {
		t.Fatalf("register status = %d; body: %s", st, env.raw)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM users_connections`).Scan(&n); err != nil {
		t.Fatalf("count users_connections: %v", err)
	}
	if n != 0 {
		t.Fatalf("users_connections has %d row(s) after registration; registration must not auto-connect", n)
	}
}

func TestUpdateName_ChangesName(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-name", token, map[string]string{
		"name": "Renamed",
	})
	if status != http.StatusOK {
		t.Fatalf("update-name status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)
	if res.User.Name != "Renamed" {
		t.Fatalf("returned name = %q, want Renamed", res.User.Name)
	}

	// Verify persistence via get-user-data.
	_, env2 := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", token, nil)
	got := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env2.Data)
	if got.User.Name != "Renamed" {
		t.Fatalf("persisted name = %q, want Renamed", got.User.Name)
	}
}

func TestUpdateName_TooShort_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-name", token, map[string]string{
		"name": "ab", // < 3 chars
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if _, ok := env.errorsMap()["name"]; !ok {
		t.Fatalf("expected a name field error; body: %s", env.raw)
	}
}

func TestLogoutUser_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/logout-user", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	if !env.Success {
		t.Fatalf("success=false; body: %s", env.raw)
	}
	// PHP's LogoutUserV1ResultAssembler hard-codes result = 'test', so the data
	// payload must be {"result":"test"} (NOT {}). Byte-match the reference.
	res := mustUnmarshal[struct {
		Result string `json:"result"`
	}](t, env.Data)
	if res.Result != "test" {
		t.Fatalf("logout data.result = %q, want %q; body: %s", res.Result, "test", env.raw)
	}
}

func TestUpdateBudget_SetsBudgetOption(t *testing.T) {
	h := newHarness(t)
	h.seedBudget(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-budget", token, map[string]string{
		"value": seedBudgetID,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)
	b, ok := res.User.optionValue("budget")
	if !ok || b == nil || *b != seedBudgetID {
		t.Fatalf("budget option = %v (ok=%v), want %q", b, ok, seedBudgetID)
	}
}

func TestUpdateBudget_NotFound_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// A well-formed but non-existent budget id -> "Plan not found" (HTTP 400).
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-budget", token, map[string]string{
		"value": "44444444-4444-4444-4444-4444444444ff",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if env.Message != "Plan not found" {
		t.Fatalf("message = %q, want %q; body: %s", env.Message, "Plan not found", env.raw)
	}
}

func TestUpdateBudget_BadUUID_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-budget", token, map[string]string{
		"value": "not-a-uuid",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if msgs := env.errorsMap()["value"]; len(msgs) == 0 || msgs[0] != "This value is not a valid UUID." {
		t.Fatalf("errors.value = %v, want [\"This value is not a valid UUID.\"]; body: %s", msgs, env.raw)
	}
}

// TestUpdateReportPeriod_OverwritesCurrencyOption pins PHP's long-standing bug:
// User::updateReportPeriod writes the period value onto the CURRENCY option (not
// report_period). As a drop-in replacement Go replicates it: after the call the
// currency option holds "monthly", the currency_id falls back to USD, and the
// deprecated reportPeriod field still reads "monthly".
func TestUpdateReportPeriod_OverwritesCurrencyOption(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-report-period", token, map[string]string{
		"value": "monthly",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)

	// The currency option must now hold the period string (the replicated bug).
	cur, ok := res.User.optionValue("currency")
	if !ok || cur == nil || *cur != "monthly" {
		t.Fatalf("currency option = %v (ok=%v), want %q (PHP bug); body: %s", cur, ok, "monthly", env.raw)
	}
	// "monthly" is not a currency code -> currency_id falls back to USD and the
	// deprecated top-level currency field reads back "USD".
	cid, ok := res.User.optionValue("currency_id")
	if !ok || cid == nil || *cid != usdCurrencyID {
		t.Fatalf("currency_id = %v (ok=%v), want USD fallback %q; body: %s", cid, ok, usdCurrencyID, env.raw)
	}
	if res.User.Currency != "USD" {
		t.Fatalf("deprecated currency field = %q, want USD; body: %s", res.User.Currency, env.raw)
	}
}
