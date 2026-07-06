package api_test

import (
	"context"
	"net/http"
	"testing"
)

func TestUpdateCurrency_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// USD is seeded by the baseline migration and resolves, so the update succeeds.
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-currency", token, map[string]string{
		"currency": "USD",
	})
	if status != http.StatusOK {
		t.Fatalf("update-currency=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)
	cur, ok := res.User.optionValue("currency")
	if !ok || cur == nil || *cur != "USD" {
		t.Fatalf("currency option=%v (ok=%v) want USD", cur, ok)
	}
}

func TestUpdateCurrency_UnknownCode_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// A well-formed 3-char code that does not resolve to a seeded currency ->
	// NotFound -> HTTP 400 "Currency ZZZ not found".
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-currency", token, map[string]string{
		"currency": "ZZZ",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("update-currency unknown=%d want 400 body=%s", status, env.raw)
	}
	if env.Message != "Currency ZZZ not found" {
		t.Fatalf("message=%q want %q", env.Message, "Currency ZZZ not found")
	}
}

func TestUpdateCurrency_Blank_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-currency", token, map[string]string{
		"currency": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("update-currency blank=%d want 400", status)
	}
	if _, ok := env.errorsMap()["currency"]; !ok {
		t.Fatalf("expected currency field error; body=%s", env.raw)
	}
}

func TestUpdateCurrency_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/user/update-currency", "", map[string]string{"currency": "USD"})
	if status != http.StatusUnauthorized {
		t.Fatalf("update-currency no token=%d want 401", status)
	}
}

func TestUpdatePassword_Success_ThenLoginWithNew(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-password", token, map[string]string{
		"oldPassword": seedPassword, "newPassword": "brand-new-pw",
	})
	if status != http.StatusOK {
		t.Fatalf("update-password=%d body=%s", status, env.raw)
	}
	// The new password authenticates.
	if st, e := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": "brand-new-pw",
	}); st != http.StatusOK {
		t.Fatalf("login with new password=%d body=%s", st, e.raw)
	}
	// The old password no longer authenticates.
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": seedPassword,
	}); st != http.StatusUnauthorized {
		t.Fatalf("login with old password=%d want 401", st)
	}
}

func TestUpdatePassword_WrongOld_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-password", token, map[string]string{
		"oldPassword": "definitely-wrong", "newPassword": "brand-new-pw",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("update-password wrong old=%d want 400 body=%s", status, env.raw)
	}
	// A fieldless *ValidationError surfaces its own message on the wire (no field
	// errors to carry the detail), so the client sees the actual reason.
	if env.Message != "Password is not correct" {
		t.Fatalf("message=%q want %q; body=%s", env.Message, "Password is not correct", env.raw)
	}
}

func TestUpdatePassword_ShortNew_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-password", token, map[string]string{
		"oldPassword": seedPassword, "newPassword": "ab", // < 4
	})
	if status != http.StatusBadRequest {
		t.Fatalf("update-password short new=%d want 400", status)
	}
	if _, ok := env.errorsMap()["newPassword"]; !ok {
		t.Fatalf("expected newPassword field error; body=%s", env.raw)
	}
}

func TestCompleteOnboarding_SetsOption(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/complete-onboarding", token, nil)
	if status != http.StatusOK {
		t.Fatalf("complete-onboarding=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)
	v, ok := res.User.optionValue("onboarding")
	if !ok || v == nil {
		t.Fatalf("onboarding option=%v (ok=%v) want a value", v, ok)
	}
}

func TestGetOptionList_ReturnsRawOptions(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/user/get-option-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("get-option-list=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Items []struct {
			Name  string  `json:"name"`
			Value *string `json:"value"`
		} `json:"items"`
	}](t, env.Data)
	names := map[string]bool{}
	for _, it := range res.Items {
		names[it.Name] = true
	}
	for _, want := range []string{"currency", "report_period", "onboarding", "budget"} {
		if !names[want] {
			t.Fatalf("get-option-list missing %q; got %v", want, names)
		}
	}
	// The raw list must NOT include the synthetic currency_id (that is
	// CurrentUserResult-only).
	if names["currency_id"] {
		t.Fatalf("get-option-list must not contain synthetic currency_id; got %v", names)
	}
}

func TestGetOptionList_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodGet, "/api/v1/user/get-option-list", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("get-option-list no token=%d want 401", status)
	}
}

func TestRemindPassword_AlwaysSuccess(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/remind-password", "", map[string]string{
		"username": "anyone@example.test",
	})
	if status != http.StatusOK {
		t.Fatalf("remind-password=%d body=%s", status, env.raw)
	}
	if !env.Success {
		t.Fatalf("remind-password success=false body=%s", env.raw)
	}
}

func TestRemindPassword_InvalidEmail_400(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/remind-password", "", map[string]string{
		"username": "not-an-email",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("remind-password invalid email=%d want 400", status)
	}
	if _, ok := env.errorsMap()["username"]; !ok {
		t.Fatalf("expected username field error; body=%s", env.raw)
	}
}

// A reset with a code that was never issued is rejected (400). The full happy
// path is covered by TestRemindAndResetPassword.
func TestResetPassword_UnknownCode_400(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
		"username": seedEmail, "code": "deadbeef0000", "password": "newpassword",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("reset-password unknown code=%d want 400; body=%s", status, env.raw)
	}
}

func TestResetPassword_BlankCode_400(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
		"username": "user@example.test", "code": "", "password": "newpassword",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("reset-password blank code=%d want 400", status)
	}
	if _, ok := env.errorsMap()["code"]; !ok {
		t.Fatalf("expected code field error; body=%s", env.raw)
	}
}

func TestRegisterUser_DuplicateEmail_400(t *testing.T) {
	h := newHarness(t)
	// The seed user already exists with seedEmail.
	status, env := h.do(t, http.MethodPost, "/api/v1/user/register-user", "", map[string]string{
		"email": seedEmail, "password": "hunter2", "name": "Dup",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("register dup=%d want 400 body=%s", status, env.raw)
	}
	// The fieldless "User already exists" *ValidationError surfaces its own message
	// on the wire, so a duplicate registration reports the real reason.
	if env.Message != "User already exists" {
		t.Fatalf("message=%q want %q; body=%s", env.Message, "User already exists", env.raw)
	}
}

func TestRegisterUser_BlankFields_400(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/register-user", "", map[string]string{
		"email": "", "password": "", "name": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("register blank=%d want 400", status)
	}
	for _, f := range []string{"email", "password", "name"} {
		if _, ok := env.errorsMap()[f]; !ok {
			t.Fatalf("expected %q field error; body=%s", f, env.raw)
		}
	}
}

func TestRegisterUser_BadEmail_400(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/register-user", "", map[string]string{
		"email": "no-at-sign", "password": "hunter2", "name": "Bob",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("register bad email=%d want 400", status)
	}
	msgs := env.errorsMap()["email"]
	if len(msgs) == 0 || msgs[0] != "This value is not a valid email address." {
		t.Fatalf("errors.email=%v want invalid-email message; body=%s", msgs, env.raw)
	}
}

func TestLoginUser_InactiveUser_401(t *testing.T) {
	h := newHarness(t)
	// Deactivate the seed user.
	if _, err := h.db.ExecContext(context.Background(),
		`UPDATE users SET is_active = 0 WHERE id = ?`, seedUserID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	status, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": seedPassword,
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("login inactive=%d want 401", status)
	}
}

func TestLoginUser_UnknownUser_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": "nobody@example.test", "password": "whatever",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("login unknown user=%d want 401", status)
	}
}

func TestUpdateReportPeriod_Blank_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-report-period", token, map[string]string{
		"value": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("update-report-period blank=%d want 400", status)
	}
	if _, ok := env.errorsMap()["value"]; !ok {
		t.Fatalf("expected value field error; body=%s", env.raw)
	}
}
