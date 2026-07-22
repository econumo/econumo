# Email Verification: Dedicated Confirm/Resend Endpoints — Revision Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `code`/`resend` fields folded into `login-user` with two dedicated public endpoints — `confirm-email` and `resend-verification-code` — mirroring the remind-password/reset-password pair.

**Architecture:** Login reverts to its frozen `{username, password}` request and keeps only the 403 "verification required" signal plus the auto-send of the first code. `POST /api/v1/user/confirm-email` takes `{username, code}` (no password; failed attempts rate-limited under a new `confirm-email` scope, generic anti-enumeration errors, empty success envelope). `POST /api/v1/user/resend-verification-code` takes `{username}` and always returns success (silent no-op for unknown/verified users; sends count under the existing `verify-email` scope). The SPA dialog confirms, then silently re-submits login.

**Tech Stack:** unchanged (Go stdlib backend, React SPA, apiparity golden catalogue).

**Spec:** `docs/superpowers/specs/2026-07-21-email-verification-design.md` (revised 2026-07-22 — decision 4 and the Behavior/SPA/Testing sections).

## Global Constraints

- `LoginRequest` reverts to exactly `{username, password}` — the `Code`/`Resend` fields and all their handling are REMOVED from the login path.
- Anti-enumeration on `confirm-email`: unknown user, missing code row, and wrong code are indistinguishable — all return the generic `ValidationError` with code `user.verification_code_invalid` ("The confirmation code is not valid."). Expired code → `user.verification_code_expired` ("The code is expired"). Exact mirror of `ResetPassword`'s error discipline.
- Anti-enumeration on `resend-verification-code`: ALWAYS returns the empty success envelope for a well-formed request (unknown user, verified user, and sent-email cases are indistinguishable), except 429 when the send cap is hit — exactly like `remind-password`.
- New rate scope `confirm-email`: `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL`, default `5`, counts FAILED attempts only, cleared on success (mirror `RateScopeReset` semantics).
- The `en` catalogue text equals the Go literal `Msg` strings exactly.
- New routes are PUBLIC (mounted bare in `routes.go`'s public group) and each needs an apiparity catalogue scenario (route guard enforces). Existing goldens must stay byte-identical; only NEW golden files may appear.
- Frontend done-gate: `pnpm test -- --run`, `pnpm lint`, `pnpm exec tsc -b`. Go done-gate: `gofmt -l .` clean, `go vet`, targeted tests; full `make test` in the final task.
- Comments: why-not-what. Commit per task on `feature/email-verification`.

---

### Task R1: Service layer — ConfirmEmail / ResendVerificationCode, login revert, config knob

**Files:**
- Modify: `internal/model/user_dto.go` (revert `LoginRequest`; add 4 DTOs)
- Modify: `internal/shared/errs/codes.go` (+`CodeUserVerificationCodeExpired`, +AllCodes)
- Modify: `locales/en.json`, `locales/ru.json` (`errors.user.verification_code_expired`)
- Modify: `internal/user/ports.go` (+`RateScopeConfirmEmail`)
- Modify: `internal/user/verify_email.go` (rework)
- Modify: `internal/user/login.go` (simplified call)
- Modify: `internal/config/config.go` + `internal/config/config_test.go` (`ECONUMO_RATE_LIMIT_CONFIRM_EMAIL`, default 5)
- Modify: `internal/server/server.go` (limiter map + nothing else — `NewService` signature is unchanged)
- Modify: `.env.example`, `CLAUDE.md` (rate-limit doc line)
- Modify: `internal/user/verify_email_test.go` (rework tests to the new API)

**Interfaces:**
- Produces (Task R2 consumes): `Service.ConfirmEmail(ctx context.Context, req model.ConfirmEmailRequest) (*model.ConfirmEmailResult, error)` and `Service.ResendVerificationCode(ctx context.Context, req model.ResendVerificationCodeRequest) (*model.ResendVerificationCodeResult, error)`; DTOs `ConfirmEmailRequest{Username, Code string}` / `ResendVerificationCodeRequest{Username string}` with empty-struct results.
- `NewService` signature: UNCHANGED (no new dependencies — everything needed is already on the Service).

- [ ] **Step 1: Write the failing tests (rework `verify_email_test.go`)**

Rewrite the login-folded tests against the new API. Keep `captureMailer`, `codeFrom`, `newVerifySvcFlag`/`newVerifySvc`, `TestAdminVerifyEmail`, and `TestFlagOffKeepsLoginUnchanged` as-is (flag-off test has no code fields to remove). Replace `TestLoginBlockedUntilEmailVerified` and `TestLoginResendForcesFreshCode`; `TestResetPasswordMarksEmailVerified` is unchanged. New/updated tests:

```go
func TestLoginBlockedUntilEmailConfirmed(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Verify Me", Email: "verify@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(cap.msgs) != 0 {
		t.Fatal("registration must not send any email")
	}

	// First login: correct password, unverified -> 403-coded error + one email.
	_, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserEmailVerificationRequired) {
		t.Fatalf("want email_verification_required, got %v", err)
	}
	if len(cap.msgs) != 1 {
		t.Fatalf("want exactly 1 verification email, got %d", len(cap.msgs))
	}

	// Second code-less login while the code is outstanding: NO new email.
	_, err = svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserEmailVerificationRequired) {
		t.Fatalf("want email_verification_required again, got %v", err)
	}
	if len(cap.msgs) != 1 {
		t.Fatalf("outstanding code must not be re-sent, got %d emails", len(cap.msgs))
	}

	// Wrong code -> generic invalid-code validation error.
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "verify@econumo.test", Code: "000000000000"}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("want verification_code_invalid, got %v", err)
	}

	// Unknown user -> the SAME generic error (anti-enumeration).
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "nobody@econumo.test", Code: "000000000000"}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("unknown user must be indistinguishable from a bad code, got %v", err)
	}

	// Correct code -> empty success; login then succeeds without any code.
	code := codeFrom(t, cap.msgs[0].Text)
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "verify@econumo.test", Code: code}); err != nil {
		t.Fatalf("ConfirmEmail: %v", err)
	}
	res, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if err != nil {
		t.Fatalf("login after confirm: %v", err)
	}
	if res.Token == "" {
		t.Fatal("login after confirm must mint a session token")
	}

	// The consumed code is dead.
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "verify@econumo.test", Code: code}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("consumed code must be invalid, got %v", err)
	}
}

func TestResendVerificationCode(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Resend Me", Email: "resend@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if len(cap.msgs) != 1 {
		t.Fatalf("blocked login must have sent the first email, got %d", len(cap.msgs))
	}

	// Resend: success envelope + a FRESH code that replaces the old one.
	if _, err := svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "resend@econumo.test"}); err != nil {
		t.Fatalf("ResendVerificationCode: %v", err)
	}
	if len(cap.msgs) != 2 {
		t.Fatalf("resend must send a fresh email, got %d", len(cap.msgs))
	}
	oldCode := codeFrom(t, cap.msgs[0].Text)
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "resend@econumo.test", Code: oldCode}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("replaced code must be invalid, got %v", err)
	}
	newCode := codeFrom(t, cap.msgs[1].Text)
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "resend@econumo.test", Code: newCode}); err != nil {
		t.Fatalf("fresh code must confirm: %v", err)
	}

	// Anti-enumeration: unknown user and now-verified user both silently succeed, no email.
	sent := len(cap.msgs)
	if _, err := svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "nobody@econumo.test"}); err != nil {
		t.Fatalf("unknown user must silently succeed, got %v", err)
	}
	if _, err := svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "resend@econumo.test"}); err != nil {
		t.Fatalf("verified user must silently succeed, got %v", err)
	}
	if len(cap.msgs) != sent {
		t.Fatalf("no-op resends must not send email, got %d new", len(cap.msgs)-sent)
	}
}
```

Add the helper next to `isVerificationDenied`:

```go
func isValidationCode(err error, code string) bool {
	v, ok := errs.AsValidation(err)
	return ok && v.MsgCode == code
}
```

Also update `TestAdminVerifyEmail` if it passed a `Code` field to `LoginRequest` (it does not — it uses code-less logins; leave it).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run 'TestLoginBlocked|TestResendVerification' -v`
Expected: FAIL to COMPILE — `ConfirmEmail` undefined, `ConfirmEmailRequest` undefined.

- [ ] **Step 3: DTOs, error code, catalogue, rate scope, config**

(a) `internal/model/user_dto.go` — revert `LoginRequest` to:

```go
// LoginRequest is the login request body (username and password both NotBlank).
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
```

and add, in a new section after the remind/reset block:

```go
// ---------------------------------------------------------------------------
// confirm-email / resend-verification-code
// ---------------------------------------------------------------------------

// ConfirmEmailRequest is the confirm-email request body. No password: the
// emailed code is the proof of ownership, so this route mirrors
// reset-password's error and rate-limit discipline instead.
type ConfirmEmailRequest struct {
	Username string `json:"username"`
	Code     string `json:"code"`
}

// Validate enforces username NotBlank+Email, code NotBlank.
func (r ConfirmEmailRequest) Validate() error {
	var fields []errs.FieldError
	fields = append(fields, validateEmailField("username", r.Username, 0)...)
	if strings.TrimSpace(r.Code) == "" {
		fields = append(fields, errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// ConfirmEmailResult is the confirm-email response (empty object).
type ConfirmEmailResult struct{}

// ResendVerificationCodeRequest is the resend-verification-code request body.
type ResendVerificationCodeRequest struct {
	Username string `json:"username"`
}

// Validate enforces NotBlank + Email.
func (r ResendVerificationCodeRequest) Validate() error {
	if fields := validateEmailField("username", r.Username, 0); len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// ResendVerificationCodeResult is the resend-verification-code response (empty object).
type ResendVerificationCodeResult struct{}
```

(b) `internal/shared/errs/codes.go` — add after `CodeUserVerificationCodeInvalid`:

```go
	CodeUserVerificationCodeExpired = "user.verification_code_expired"
```

plus the `AllCodes` entry.

(c) Catalogue — `locales/en.json` `errors.user`: `"verification_code_expired": "The code is expired"`. `locales/ru.json`: `"verification_code_expired": "Срок действия кода истёк."`.

(d) `internal/user/ports.go` — extend the const block:

```go
	RateScopeConfirmEmail = "confirm-email"
```

(e) `internal/config/config.go` — struct field after `RateLimitVerifyEmail`:

```go
	RateLimitConfirmEmail int // ECONUMO_RATE_LIMIT_CONFIRM_EMAIL: failed confirm-email attempts per username
```

table row after the verify-email row: `{&c.RateLimitConfirmEmail, "ECONUMO_RATE_LIMIT_CONFIRM_EMAIL", 5},`. Extend `TestLoad_EmailVerification` in `config_test.go` with `if cfg.RateLimitConfirmEmail != 5 { t.Errorf(...) }` on the default-check block.

(f) `internal/server/server.go` — limiter map gains `appuser.RateScopeConfirmEmail: cfg.RateLimitConfirmEmail,`.

(g) `.env.example`: `#ECONUMO_RATE_LIMIT_CONFIRM_EMAIL=5` with a one-line comment next to the verify-email entry. `CLAUDE.md`: extend the rate-limit bullet with `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL` — failed confirm-email attempts per username per window (default `5`, cleared on success).

- [ ] **Step 4: Rework `internal/user/verify_email.go`**

Replace the file's use-case surface: `verifyEmailOnLogin` shrinks to a code-less gate (rename to `requireVerifiedEmail`), and the two new use cases land here. `ensureVerificationCode` stays as-is. Full new file content (imports: drop `strings` only if unused — `ConfirmEmail` uses it; keep `model`, `errs`, `vo`, `context`, `strings`, `time`):

```go
// Email-verification use cases (ECONUMO_EMAIL_VERIFICATION): an unverified
// user's login is denied with a 403 signal (sending the code email as a side
// effect), and the public confirm/resend endpoints mirror the reset-password
// pair's anti-enumeration and rate-limit discipline — the emailed code is the
// sole secret on the confirm route, so failures are generic and counted.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// requireVerifiedEmail gates an unverified user's login: it ensures an
// outstanding verification email exists (sending one when none is live) and
// denies with email_verification_required. Confirming happens on the
// dedicated confirm-email endpoint, never in login.
func (s *Service) requireVerifiedEmail(ctx context.Context, u *model.User, email string, limitKey string) error {
	if err := s.ensureVerificationCode(ctx, u, email, false, limitKey, s.clock.Now()); err != nil {
		return err
	}
	return &errs.AccessDeniedError{Msg: "Please verify your email address.", Code: errs.CodeUserEmailVerificationRequired}
}

// ConfirmEmail validates the (email, code) pair and marks the email verified.
// Unknown user, missing row, and wrong code are indistinguishable (generic
// invalid-code error) so the route cannot be used for account enumeration;
// failed attempts count toward the confirm-email cap and clear on success.
func (s *Service) ConfirmEmail(ctx context.Context, req model.ConfirmEmailRequest) (*model.ConfirmEmailResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeConfirmEmail, lowered); err != nil {
		return nil, err
	}
	invalid := &errs.ValidationError{Msg: "The confirmation code is not valid.", MsgCode: errs.CodeUserVerificationCodeInvalid}

	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeConfirmEmail, lowered)
			return nil, invalid
		}
		return nil, err
	}
	ev, err := s.emailVerifications.GetByUser(ctx, u.ID)
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeConfirmEmail, lowered)
			return nil, invalid
		}
		return nil, err
	}
	if HashResetCode(strings.TrimSpace(req.Code)) != ev.Code {
		s.failAttempt(RateScopeConfirmEmail, lowered)
		return nil, invalid
	}
	if ev.IsExpired(s.clock.Now()) {
		s.failAttempt(RateScopeConfirmEmail, lowered)
		return nil, &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserVerificationCodeExpired}
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.MarkEmailVerified(s.clock.Now())
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		return s.emailVerifications.DeleteByUser(ctx, u.ID)
	}); err != nil {
		return nil, err
	}
	s.clearAttempt(RateScopeConfirmEmail, lowered)
	return &model.ConfirmEmailResult{}, nil
}

// ResendVerificationCode force-sends a fresh code to an unverified user.
// It always reports success — an unknown or already-verified username is a
// silent no-op — so the route cannot be used for account enumeration; actual
// sends are capped by the verify-email scope (over-limit -> 429, like remind).
func (s *Service) ResendVerificationCode(ctx context.Context, req model.ResendVerificationCodeRequest) (*model.ResendVerificationCodeResult, error) {
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			return &model.ResendVerificationCodeResult{}, nil // anti-enumeration
		}
		return nil, err
	}
	if u.EmailVerified {
		return &model.ResendVerificationCodeResult{}, nil
	}
	email, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return nil, derr
	}
	if err := s.ensureVerificationCode(ctx, u, email, true, lowered, s.clock.Now()); err != nil {
		return nil, err
	}
	return &model.ResendVerificationCodeResult{}, nil
}
```

(`ensureVerificationCode` remains below, unchanged.)

- [ ] **Step 5: Simplify `internal/user/login.go`**

Replace the verification block with:

```go
	if s.emailVerification && !u.EmailVerified {
		if err := s.requireVerifiedEmail(ctx, u, email, limitKey); err != nil {
			return nil, err
		}
	}
```

- [ ] **Step 6: Run the reworked tests**

Run: `go test ./internal/user/... -v -run 'TestLoginBlocked|TestResendVerification|TestFlagOff|TestResetPasswordMarks|TestAdminVerifyEmail'`
Expected: PASS (all).
Run: `gofmt -l . && go vet ./... && go build ./... && go test ./internal/user/... ./internal/config/ ./internal/test/i18ntest/`
Expected: clean/PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ locales/ .env.example CLAUDE.md
git commit -m "refactor: dedicated ConfirmEmail/ResendVerificationCode use cases; login keeps only the 403 signal"
```

---

### Task R2: HTTP edge — handlers, routes, swagger, apiparity, docs

**Files:**
- Modify: `internal/user/api/user.go` (2 handlers; LoginUser swag description touch-up)
- Modify: `internal/user/api/routes.go` (public group + route-count doc comment)
- Modify: `internal/test/apiparity/catalogue_user.go` (2 scenarios)
- Modify: `CLAUDE.md` (public-routes lists — the "API conventions" and "Encodings, messages, routes" sections both enumerate public routes)
- Generated: OpenAPI docs (`make swagger`), new apiparity goldens (`UPDATE_GOLDEN=1`)

**Interfaces:**
- Consumes: R1's service methods and DTOs.
- Produces: `POST /api/v1/user/confirm-email`, `POST /api/v1/user/resend-verification-code` (public, standard envelopes) — consumed by Task R3.

- [ ] **Step 1: Handlers** (append near RemindPassword/ResetPassword in `internal/user/api/user.go`)

```go
// ConfirmEmail handles POST /api/v1/user/confirm-email (public). It validates
// the (email, code) pair and marks the email verified; unknown users and bad
// codes yield the same generic error (anti-enumeration).
//
// @Summary     Confirm email
// @Description Confirms a user's email with the emailed verification code (ECONUMO_EMAIL_VERIFICATION). Returns an empty success envelope.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.ConfirmEmailRequest true "Confirm email request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ConfirmEmailResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/confirm-email [post]
func (h *Handlers) ConfirmEmail(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.ConfirmEmail)
}

// ResendVerificationCode handles POST /api/v1/user/resend-verification-code
// (public). It re-sends the verification code to an unverified user, always
// returning success (anti-enumeration).
//
// @Summary     Resend verification code
// @Description Re-sends the email verification code. Always returns success (anti-enumeration).
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.ResendVerificationCodeRequest true "Resend verification code request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ResendVerificationCodeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/user/resend-verification-code [post]
func (h *Handlers) ResendVerificationCode(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.ResendVerificationCode)
}
```

Also update `LoginUser`'s `@Failure 403` description to drop the "retry with the emailed code in the request body" wording — it now reads: `"Email verification required (ECONUMO_EMAIL_VERIFICATION): confirm via /api/v1/user/confirm-email, then log in again."`

- [ ] **Step 2: Routes** — in `internal/user/api/routes.go`, public group gains:

```go
		mux.HandleFunc("POST /api/v1/user/confirm-email", h.ConfirmEmail)
		mux.HandleFunc("POST /api/v1/user/resend-verification-code", h.ResendVerificationCode)
```

Update the file's doc comment endpoint count (21 → 23) and its public-group enumeration.

- [ ] **Step 3: apiparity scenarios** — in `internal/test/apiparity/catalogue_user.go`, append to the scenario containing remind/reset calls:

```go
			// Default config runs flag-off, so the seeded owner is verified:
			// confirm with any code pins the generic invalid-code envelope, and
			// resend pins the silent-success (no email side effect) envelope.
			{Label: "err:confirm-email-bad-code", Method: "POST", Path: "/api/v1/user/confirm-email", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "code": "000000000000"}},
			{Label: "resend-verification-code", Method: "POST", Path: "/api/v1/user/resend-verification-code", Auth: "",
				Body: map[string]any{"username": OwnerEmail}},
```

- [ ] **Step 4: Regenerate + inspect**

Run: `make swagger`
Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` then `git status --porcelain internal/test/apiparity/testdata/golden/ && git diff internal/test/apiparity/testdata/golden/`
Expected: ONLY new golden files (the two new labels); ZERO diffs to existing goldens. If an existing golden changed, STOP — that means default behavior moved.
Run: `go test ./internal/test/apiparity/ ./internal/test/mcpparity/`
Expected: PASS (guards see the new routes covered).

- [ ] **Step 5: CLAUDE.md** — extend both public-route enumerations ("Public routes (login, register, remind-password, reset-password, ...)" in the API-conventions Authentication bullet AND the wire-contract "Encodings, messages, routes" section) with `confirm-email` and `resend-verification-code`.

- [ ] **Step 6: Verify and commit**

Run: `gofmt -l . && go vet ./... && go test ./internal/user/... ./internal/test/apiparity/`
Expected: clean/PASS.

```bash
git add internal/ docs/ CLAUDE.md
git commit -m "feat: public confirm-email + resend-verification-code endpoints"
```

---

### Task R3: SPA rework — confirm-then-login dialog flow

**Files:**
- Modify: `web/src/api/user.ts` (revert `login`; add `confirmEmail`, `resendVerificationCode`)
- Modify: `web/src/features/auth/queries.ts` (revert `useLogin`; add `useConfirmEmail`; rework `useResendVerification`)
- Modify: `web/src/features/auth/VerifyEmailDialog.tsx` (confirm → silent login)
- Modify: `web/src/features/auth/queries.test.tsx`, `web/src/features/auth/VerifyEmailDialog.test.tsx`
- Unchanged: `web/src/lib/metrics.ts` (same two keys), `isForbidden`, `LoginPage.tsx`

**Interfaces:**
- Consumes: R2's endpoints.
- Produces: dialog behavior — Verify calls confirm-email then login; Resend calls resend-verification-code with username only.

- [ ] **Step 1: Rework the tests first**

`queries.test.tsx`: replace the two Task-8 tests with:

```tsx
it('confirmEmail posts username+code and fires the completed metric', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/user/confirm-email', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { result } = renderHook(() => useConfirmEmail(), { wrapper })
  await result.current.mutateAsync({ username: 'a@b.test', code: '123456789012' })
  expect(bodies[0]).toMatchObject({ username: 'a@b.test', code: '123456789012' })
})

it('resend posts the username and succeeds on the success envelope', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/user/resend-verification-code', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { result } = renderHook(() => useResendVerification(), { wrapper })
  await expect(result.current.mutateAsync({ username: 'a@b.test' })).resolves.not.toThrow()
  expect(bodies[0]).toMatchObject({ username: 'a@b.test' })
})
```

(Adapt to the file's existing local harness exactly as before. If the old tests asserted metric calls via a spy, mirror that mechanism for `EMAIL_VERIFICATION_COMPLETED` on confirm success and `EMAIL_VERIFICATION_RESENT` on resend success.)

`VerifyEmailDialog.test.tsx`: rework the verify test — msw now stubs BOTH `confirm-email` (success) and `login-user` (success `{token, user}`); assert the confirm body `{username, code}` AND that login was called afterwards with `{username, password}`. The invalid-code test stubs `confirm-email` → 400 `{success:false, message:'The confirmation code is not valid.', code:400, errors:{}}` and asserts the inline message. The resend test stubs `resend-verification-code` → success envelope and asserts the "new code has been sent" copy. Dismissibility test unchanged.

Run: `cd web && pnpm exec vitest run queries VerifyEmailDialog`
Expected: FAIL (new hooks/endpoints missing).

- [ ] **Step 2: API client** (`web/src/api/user.ts`)

Revert `login` to the two-arg form (delete `LoginOptions` and the spread; restore the original comment) and add:

```ts
export async function confirmEmail(username: string, code: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/confirm-email'), { username, code })
}

export async function resendVerificationCode(username: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/resend-verification-code'), { username })
}
```

- [ ] **Step 3: Hooks** (`web/src/features/auth/queries.ts`)

Revert `useLogin` to its pre-feature body (vars `{ username, password }`, unconditional `USER_LOGIN` metric only). Replace `useResendVerification` and add `useConfirmEmail`:

```ts
export function useConfirmEmail() {
  return useMutation({
    mutationFn: ({ username, code }: { username: string; code: string }) =>
      userApi.confirmEmail(username, code),
    onSuccess: () => trackEvent(METRICS.EMAIL_VERIFICATION_COMPLETED),
  })
}

export function useResendVerification() {
  return useMutation({
    mutationFn: ({ username }: { username: string }) => userApi.resendVerificationCode(username),
    onSuccess: () => trackEvent(METRICS.EMAIL_VERIFICATION_RESENT),
  })
}
```

(Remove the now-unused `isForbidden` import from `queries.ts` — `LoginPage.tsx` still uses it.)

- [ ] **Step 4: Dialog** (`web/src/features/auth/VerifyEmailDialog.tsx`)

`onVerify` becomes confirm-then-silent-login:

```tsx
  const onVerify = handleSubmit(async ({ code }) => {
    setServerError('')
    setResent(false)
    try {
      await confirm.mutateAsync({ username, code: code.trim() })
      // The code proved ownership; the silent re-login uses the credentials
      // still held by the login form, so the user lands in the app in one step.
      await login.mutateAsync({ username, password })
      window.location.assign('/')
    } catch (err) {
      setServerError(apiErrorMessage(err))
    }
  })

  const onResend = async () => {
    setServerError('')
    setResent(false)
    try {
      await resend.mutateAsync({ username })
      setResent(true)
    } catch (err) {
      setServerError(apiErrorMessage(err))
    }
  }
```

with `const confirm = useConfirmEmail()` added and the submit button's `disabled` becoming `confirm.isPending || login.isPending`. Imports updated accordingly. Everything else (layout, i18n keys) unchanged.

- [ ] **Step 5: Full frontend gate**

Run: `cd web && pnpm test -- --run && pnpm lint && pnpm exec tsc -b`
Expected: ALL PASS, including metrics-coverage (both keys still fired at hook choke points).

- [ ] **Step 6: Commit**

```bash
git add web/src/
git commit -m "refactor: dialog confirms via confirm-email endpoint, then silently re-logs-in"
```

---

### Task R4: Full-suite gate + smoke

- [ ] **Step 1:** `make go-test` — PASS, `git status --porcelain` clean (no golden/doc drift beyond what R2 committed).
- [ ] **Step 2:** `make web-test && make web-lint && cd web && pnpm exec tsc -b` — PASS.
- [ ] **Step 3:** `make test` (engine comparison on real PostgreSQL) — PASS.
- [ ] **Step 4:** Scripted smoke against a scratch sqlite instance with `ECONUMO_EMAIL_VERIFICATION=true` (same recipe as the previous smoke, new flow): register → login → 403 + email in log → `POST /api/v1/user/confirm-email {username, code}` → `{"success":true,...,"data":{}}` → login → 200 token. Also: `confirm-email` with a wrong code → 400 generic; `resend-verification-code` for an unknown user → 200 success; resend for the (now unverified? use a second registered user) → fresh code in log, old code rejected.
- [ ] **Step 5:** Commit stragglers if any; report evidence.

---

## Self-review notes (applied)

- `NewService` signature unchanged — no call-site churn this time; the new use cases live on the existing Service.
- The `strings`/`time` imports in `verify_email.go` remain used (`ConfirmEmail` trims; `ensureVerificationCode` takes `time.Time`).
- `isNotFound` (package-level helper in `password.go`) is reused rather than `errs.AsNotFound` inline, matching `ResetPassword`'s idiom.
- apiparity scenario labels follow the catalogue's conventions (`err:` prefix for error-envelope pins).
- `verification_code_invalid` keeps its existing en/ru catalogue text; only `verification_code_expired` is new. The invalid-code error changes transport (403 AccessDenied → 400 ValidationError) — that is intentional and only observable flag-on (no golden impact).
