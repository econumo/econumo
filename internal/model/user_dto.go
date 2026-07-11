// Request/result DTOs (with their tier-1 Validate() methods) for the user
// use-cases. JSON field names are frozen to the existing API wire contract;
// see CLAUDE.md.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ---------------------------------------------------------------------------
// Shared result shapes
// ---------------------------------------------------------------------------

// OptionResult is one user option in the API. Value is a pointer so a SQL NULL
// (e.g. budget) serializes as JSON null.
type OptionResult struct {
	Name  string  `json:"name"`
	Value *string `json:"value"`
}

// CurrentUserResult is the current-user shape. email is the decoded plaintext;
// options always includes a synthetic currency_id entry resolved from the
// currency code. currency/reportPeriod are deprecated duplicate fields.
type CurrentUserResult struct {
	Id           string         `json:"id"`
	Name         string         `json:"name"`
	Email        string         `json:"email"`
	Avatar       string         `json:"avatar"`
	Options      []OptionResult `json:"options"`
	Currency     string         `json:"currency"`
	ReportPeriod string         `json:"reportPeriod"`
}

// ---------------------------------------------------------------------------
// login-user
// ---------------------------------------------------------------------------

// LoginRequest is the login request body (username and password both NotBlank).
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Validate enforces the NotBlank constraints on username and password.
func (r LoginRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Username) == "" {
		fields = append(fields, errs.FieldError{Key: "username", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if r.Password == "" {
		fields = append(fields, errs.FieldError{Key: "password", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// LoginResult is the login response: a JWT plus the current user.
type LoginResult struct {
	Token string            `json:"token"`
	User  CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// register-user
// ---------------------------------------------------------------------------

// RegisterRequest is the register request body.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Validate enforces: email NotBlank+Email+max256, password NotBlank+min4,
// name NotBlank+len 3..20.
func (r RegisterRequest) Validate() error {
	var fields []errs.FieldError
	fields = append(fields, validateEmailField("email", r.Email, 256)...)
	fields = append(fields, validateMinLenField("password", r.Password, 4)...)
	fields = append(fields, validateLenRangeField("name", r.Name, 3, 20)...)
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// RegisterResult is the register response: the user WITHOUT a token.
type RegisterResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// get-user-data
// ---------------------------------------------------------------------------

// GetUserDataResult is the get-user-data response.
type GetUserDataResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// get-option-list
// ---------------------------------------------------------------------------

// GetOptionListResult is the get-option-list response: the raw persisted
// options (no synthetic currency_id, unlike CurrentUserResult).
type GetOptionListResult struct {
	Items []OptionResult `json:"items"`
}

// ---------------------------------------------------------------------------
// update-name
// ---------------------------------------------------------------------------

// UpdateNameRequest is the update-name request body.
type UpdateNameRequest struct {
	Name string `json:"name"`
}

// Validate enforces NotBlank + length 3..20.
func (r UpdateNameRequest) Validate() error {
	if fields := validateLenRangeField("name", r.Name, 3, 20); len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdateNameResult is the update-name response.
type UpdateNameResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// update-currency
// ---------------------------------------------------------------------------

// UpdateCurrencyRequest is the update-currency request body.
type UpdateCurrencyRequest struct {
	Currency string `json:"currency"`
}

// Validate enforces NotBlank (the 3-char CurrencyCode invariant is checked in
// the service via the value-object constructor — tier 2).
func (r UpdateCurrencyRequest) Validate() error {
	if strings.TrimSpace(r.Currency) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "currency", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// UpdateCurrencyResult is the update-currency response.
type UpdateCurrencyResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// update-report-period
// ---------------------------------------------------------------------------

// UpdateReportPeriodRequest is the update-report-period request body (the field
// name is "value").
type UpdateReportPeriodRequest struct {
	Value string `json:"value"`
}

// Validate enforces NotBlank (the ReportPeriod invariant is checked in the
// service via the value-object constructor — tier 2).
func (r UpdateReportPeriodRequest) Validate() error {
	if strings.TrimSpace(r.Value) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "value", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// UpdateReportPeriodResult is the update-report-period response.
type UpdateReportPeriodResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// update-password
// ---------------------------------------------------------------------------

// UpdatePasswordRequest is the update-password request body.
type UpdatePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// Validate enforces oldPassword NotBlank, newPassword NotBlank + min4.
func (r UpdatePasswordRequest) Validate() error {
	var fields []errs.FieldError
	if r.OldPassword == "" {
		fields = append(fields, errs.FieldError{Key: "oldPassword", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	fields = append(fields, validateMinLenField("newPassword", r.NewPassword, 4)...)
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdatePasswordResult is the update-password response (empty object).
type UpdatePasswordResult struct{}

// ---------------------------------------------------------------------------
// complete-onboarding
// ---------------------------------------------------------------------------

// CompleteOnboardingResult is the complete-onboarding response.
type CompleteOnboardingResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// update-budget
// ---------------------------------------------------------------------------

// UpdateActiveBudgetRequest is the user update-budget request body (the
// endpoint sets the user's active budget). The wire field name is "value" (a
// budget id) — frozen, do not rename. Named distinctly from the budget
// feature's own UpdateBudgetRequest (a different endpoint, same-shape name).
type UpdateActiveBudgetRequest struct {
	Value string `json:"value"`
}

// Validate enforces value NotBlank + Uuid. The budget-existence check
// (-> "Plan not found") is tier 2, in the service.
func (r UpdateActiveBudgetRequest) Validate() error {
	if strings.TrimSpace(r.Value) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "value", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if _, err := vo.ParseId(r.Value); err != nil {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "value", Message: "This value is not a valid UUID.", Code: "INVALID_UUID_ERROR"})
	}
	return nil
}

// UpdateActiveBudgetResult is the user update-budget response.
type UpdateActiveBudgetResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// update-avatar
// ---------------------------------------------------------------------------

// UpdateAvatarRequest is the update-avatar request body. Icon is a Material
// ligature name; color must be one of the avatar color slugs (tier-2, in the
// service).
type UpdateAvatarRequest struct {
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// Validate enforces NotBlank on both fields; format/choice checks are tier 2.
func (r UpdateAvatarRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Icon) == "" {
		fields = append(fields, errs.FieldError{Key: "icon", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Color) == "" {
		fields = append(fields, errs.FieldError{Key: "color", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// UpdateAvatarResult is the update-avatar response.
type UpdateAvatarResult struct {
	User CurrentUserResult `json:"user"`
}

// ---------------------------------------------------------------------------
// remind-password / reset-password
// ---------------------------------------------------------------------------

// RemindPasswordRequest is the remind-password request body.
type RemindPasswordRequest struct {
	Username string `json:"username"`
}

// Validate enforces NotBlank + Email.
func (r RemindPasswordRequest) Validate() error {
	if fields := validateEmailField("username", r.Username, 0); len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// RemindPasswordResult is the remind-password response (empty object).
type RemindPasswordResult struct{}

// ResetPasswordRequest is the reset-password request body.
type ResetPasswordRequest struct {
	Username string `json:"username"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

// Validate enforces username NotBlank+Email, code NotBlank, password NotBlank+min4.
func (r ResetPasswordRequest) Validate() error {
	var fields []errs.FieldError
	fields = append(fields, validateEmailField("username", r.Username, 0)...)
	if strings.TrimSpace(r.Code) == "" {
		fields = append(fields, errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	fields = append(fields, validateMinLenField("password", r.Password, 4)...)
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// ResetPasswordResult is the reset-password response (empty object).
type ResetPasswordResult struct{}

// ---------------------------------------------------------------------------
// logout-user
// ---------------------------------------------------------------------------

// LogoutResult is the logout response. The wire shape is the frozen
// {"result":"test"} — NOT {}; the literal "test" constant is a preserved quirk
// clients depend on, do not change it.
type LogoutResult struct {
	Result string `json:"result"`
}

// ---------------------------------------------------------------------------
// tier-1 field validators (shared)
// ---------------------------------------------------------------------------

// validateEmailField checks NotBlank, RFC-ish email, and optional max length.
// maxLen <= 0 disables the length check. The messages are frozen exact strings;
// see CLAUDE.md.
func validateEmailField(key, v string, maxLen int) []errs.FieldError {
	if strings.TrimSpace(v) == "" {
		return []errs.FieldError{{Key: key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"}}
	}
	if !looksLikeEmail(v) {
		return []errs.FieldError{{Key: key, Message: "This value is not a valid email address.", Code: "INVALID_EMAIL_ERROR"}}
	}
	if maxLen > 0 && len([]rune(v)) > maxLen {
		return []errs.FieldError{{Key: key, Message: "This value is too long.", Code: "TOO_LONG_ERROR"}}
	}
	return nil
}

func validateMinLenField(key, v string, minLen int) []errs.FieldError {
	if v == "" {
		return []errs.FieldError{{Key: key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"}}
	}
	if len([]rune(v)) < minLen {
		return []errs.FieldError{{Key: key, Message: "This value is too short.", Code: "TOO_SHORT_ERROR"}}
	}
	return nil
}

func validateLenRangeField(key, v string, minLen, maxLen int) []errs.FieldError {
	if strings.TrimSpace(v) == "" {
		return []errs.FieldError{{Key: key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"}}
	}
	n := len([]rune(v))
	if n < minLen {
		return []errs.FieldError{{Key: key, Message: "This value is too short.", Code: "TOO_SHORT_ERROR"}}
	}
	if n > maxLen {
		return []errs.FieldError{{Key: key, Message: "This value is too long.", Code: "TOO_LONG_ERROR"}}
	}
	return nil
}

// looksLikeEmail is a minimal, dependency-free check: a single @ with non-empty
// local and domain parts and a dot in the domain. This is close enough for
// tier-1; the canonical email invariant is enforced in tier 2. See notes.
func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	if strings.ContainsRune(s[at+1:], '@') {
		return false
	}
	domain := s[at+1:]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}
