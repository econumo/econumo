# Self-service change-email — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a signed-in user change their own email — verify the new address with a 6-digit code, gate the request on the current password, notify the old address, and revoke other sessions on confirm.

**Architecture:** Reuses the registration email-verification machinery almost verbatim, keyed by **user id** (the flow is fully authenticated). New pending-change table `users_email_change_requests`; three authenticated endpoints (`request-email-change`, `confirm-email-change`, `resend-email-change-code`); a new `ChangeEmailSender` mailer; a `ChangeEmailPage` mirroring `ChangePasswordPage` + the `VerifyEmailDialog` code UX. Builds on the merged email-only model (`lower(email)` is the key; `GetByEmail`/`ExistsByEmail`).

**Tech Stack:** Go hexagonal + sqlc (per-engine sqlite/pgsql), SQLite + PostgreSQL, React 19 + Vite + TanStack Query, i18n (`locales/{en,ru}.json`).

## Global Constraints

- **Both engines, byte-identical.** Every migration/query change lands in BOTH `sqlite` and `pgsql`; `make test-repo-pgsql` + `enginecompare` must pass.
- **sqlc regen after any `.sql` edit:** `cd internal/infra/storage/sqlc && sqlc generate && cd -`. ASCII-only in `.sql` (no non-ASCII — v1.30 sqlite codegen mangles byte offsets).
- **Next migration timestamp is `20260723000000`** (`20260722000000` already exists on `main`).
- **Frozen wire/format:** datetimes `2006-01-02 15:04:05`; error envelope unchanged; validation strings are exact per language and asserted; codes render from the `errors.*` catalogue.
- **apiparity/mcpparity goldens are regenerated only via `UPDATE_GOLDEN=1`, then INSPECTED** — a golden change must reflect exactly the new endpoints, nothing else. No MCP surface for this feature (matches update-password), so `mcpparity` goldens must NOT change.
- **i18n guards (in `make go-test`):** en/ru key parity, `{var}` parity per key, `errs.AllCodes` ↔ `errors.*` two-way, `EmailKeys` ↔ `emails.*`, and frontend `t()`-key coverage. Any new catalogue key must be added to BOTH `en.json` and `ru.json` with matching `{placeholders}`.
- **Analytics:** every new user-facing action fires a `METRICS` event; `metrics-coverage.test.ts` fails if a `METRICS` key is never referenced in non-test source.
- **Frontend type-check:** run `pnpm exec tsc -b` (vitest/oxlint do not type-check).
- **Comments:** only non-obvious *why*; no name-restating godoc; no PHP references.
- **Analytics/product rule:** confirm-success fires the change-email metric.

---

### Task 1: Data layer — model, migration, sqlc, repo, port

Add the pending-change store, mirroring the email-verification store exactly, plus a `new_email` column. Purely additive; nothing consumes it yet.

**Files:**
- Create: `internal/model/user_email_change_request.go`
- Create: `internal/infra/storage/migrations/sqlite/20260723000000.sql`, `internal/infra/storage/migrations/pgsql/20260723000000.sql`
- Create: `internal/infra/storage/sqlc/query/sqlite/email_change_request.sql`, `internal/infra/storage/sqlc/query/pgsql/email_change_request.sql`
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}`
- Create: `internal/user/repo/emailchangerequest.go`, `emailchangerequest_sqlite.go`, `emailchangerequest_pgsql.go`
- Modify: `internal/user/repository.go` (add `EmailChangeRequests` port)
- Test: `internal/user/repo/emailchangerequest_integration_test.go`

**Interfaces:**
- Produces: `model.EmailChangeRequest{ID, UserID, NewEmail, Code, CreatedAt, UpdatedAt, ExpiredAt}`, `model.NewEmailChangeRequest(id, userID vo.Id, newEmail, code string, now time.Time) *EmailChangeRequest`, `(*EmailChangeRequest).IsExpired(now)`, `.RetryAfter(now)`; the `EmailChangeRequests` port (`GetByUser`/`Save`/`DeleteByUser`) and `repo.NewEmailChangeRequestRepo(driver, tx)`.

- [ ] **Step 1: Model** — create `internal/model/user_email_change_request.go` (mirror `user_email_change_request` on `user_email_verification.go`, adding `NewEmail`; reuse the same 10-min TTL and 60s resend gap constants — reference `EmailVerificationResendGap` to avoid a second constant):

```go
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// emailChangeTTL is how long a pending change-email code stays valid.
const emailChangeTTL = 10 * time.Minute

// EmailChangeRequest is a pending email change for a user: the proposed new
// email plus the emailed code (hashed by the app layer). One outstanding row
// per user; the request is replaced, never updated.
type EmailChangeRequest struct {
	ID        vo.Id
	UserID    vo.Id
	NewEmail  string
	Code      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiredAt time.Time
}

// NewEmailChangeRequest builds a fresh pending change expiring 10 minutes from now.
func NewEmailChangeRequest(id, userID vo.Id, newEmail, code string, now time.Time) *EmailChangeRequest {
	return &EmailChangeRequest{
		ID:        id,
		UserID:    userID,
		NewEmail:  newEmail,
		Code:      code,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiredAt: now.Add(emailChangeTTL),
	}
}

// IsExpired reports whether the code is no longer valid at the given time.
func (r *EmailChangeRequest) IsExpired(now time.Time) bool { return now.After(r.ExpiredAt) }

// RetryAfter reports how long until another code may be sent (see
// EmailVerification.RetryAfter — same semantics, same resend gap).
func (r *EmailChangeRequest) RetryAfter(now time.Time) time.Duration {
	remaining := r.CreatedAt.Add(EmailVerificationResendGap).Sub(now)
	if remaining <= 0 {
		return 0
	}
	if remaining > EmailVerificationResendGap {
		return EmailVerificationResendGap
	}
	if rem := remaining % time.Second; rem > 0 {
		remaining += time.Second - rem
	}
	return remaining
}
```

- [ ] **Step 2: Migration (both engines)** — create `…/sqlite/20260723000000.sql`:

```sql
CREATE TABLE users_email_change_requests
(
    id         TEXT NOT NULL
    , user_id    TEXT NOT NULL
    , new_email  VARCHAR(255) NOT NULL
    , code       TEXT NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , expired_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);
```

and `…/pgsql/20260723000000.sql`:

```sql
CREATE TABLE users_email_change_requests
(
    id         UUID NOT NULL
    , user_id    UUID NOT NULL
    , new_email  VARCHAR(255) NOT NULL
    , code       VARCHAR(64) NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expired_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);
```

- [ ] **Step 3: sqlc queries (both engines)** — create `query/sqlite/email_change_request.sql`:

```sql
-- Pending change-email requests (users_email_change_requests). The request is
-- replaced with a fresh one, read back by user, and deleted once confirmed.
-- Expiry is compared in the app layer (Go time), not in SQL.

-- name: DeleteUserEmailChangeRequestsByUser :exec
DELETE FROM users_email_change_requests WHERE user_id = ?;

-- name: InsertUserEmailChangeRequest :exec
INSERT INTO users_email_change_requests (id, user_id, new_email, code, created_at, updated_at, expired_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetUserEmailChangeRequestByUser :one
SELECT id, user_id, new_email, code, created_at, updated_at, expired_at
FROM users_email_change_requests
WHERE user_id = ?;
```

and `query/pgsql/email_change_request.sql` (same, `$1..$7`):

```sql
-- See the sqlite sibling for the flow; expiry is compared in the app layer, not SQL.

-- name: DeleteUserEmailChangeRequestsByUser :exec
DELETE FROM users_email_change_requests WHERE user_id = $1;

-- name: InsertUserEmailChangeRequest :exec
INSERT INTO users_email_change_requests (id, user_id, new_email, code, created_at, updated_at, expired_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetUserEmailChangeRequestByUser :one
SELECT id, user_id, new_email, code, created_at, updated_at, expired_at
FROM users_email_change_requests
WHERE user_id = $1;
```

- [ ] **Step 4: Regenerate sqlc** — Run: `cd internal/infra/storage/sqlc && sqlc generate && cd -`. Expected exit 0; `gen/{sqlite,pgsql}` now have `UsersEmailChangeRequest`, `InsertUserEmailChangeRequestParams`, and the three methods.

- [ ] **Step 5: Port interface** — in `internal/user/repository.go`, after the `EmailVerifications` interface, add:

```go
// EmailChangeRequests persists pending self-service email changes
// (users_email_change_requests). One outstanding row per user; GetByUser on a
// missing row returns *errs.NotFoundError.
type EmailChangeRequests interface {
	GetByUser(ctx context.Context, userID vo.Id) (*model.EmailChangeRequest, error)
	Save(ctx context.Context, r *model.EmailChangeRequest) error
	DeleteByUser(ctx context.Context, userID vo.Id) error
}
```

- [ ] **Step 6: Repo (mirror EmailVerificationRepo)** — create `internal/user/repo/emailchangerequest.go`:

```go
// EmailChangeRequestRepo persists pending self-service email changes
// (users_email_change_requests). Expiry is evaluated in the domain
// (EmailChangeRequest.IsExpired), not in SQL.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

type (
	emailChangeRow          = sqlitegen.UsersEmailChangeRequest
	emailChangeInsertParams = sqlitegen.InsertUserEmailChangeRequestParams
)

type emailChangeQuerier interface {
	DeleteUserEmailChangeRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserEmailChangeRequest(ctx context.Context, db backend.DBTX, p emailChangeInsertParams) error
	GetUserEmailChangeRequestByUser(ctx context.Context, db backend.DBTX, userID string) (emailChangeRow, error)
}

type EmailChangeRequestRepo struct {
	tx *backend.TxManager
	q  emailChangeQuerier
}

var _ user.EmailChangeRequests = (*EmailChangeRequestRepo)(nil)

func NewEmailChangeRequestRepo(driver string, tx *backend.TxManager) *EmailChangeRequestRepo {
	switch driver {
	case "sqlite":
		return &EmailChangeRequestRepo{tx: tx, q: emailChangeSqliteQuerier{}}
	case "postgresql":
		return &EmailChangeRequestRepo{tx: tx, q: emailChangePgsqlQuerier{}}
	default:
		panic("emailchangerequestrepo: unknown database driver " + driver)
	}
}

func (r *EmailChangeRequestRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *EmailChangeRequestRepo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserEmailChangeRequestsByUser(ctx, r.db(ctx), userID.String())
}

func (r *EmailChangeRequestRepo) Save(ctx context.Context, cr *model.EmailChangeRequest) error {
	return r.q.InsertUserEmailChangeRequest(ctx, r.db(ctx), emailChangeInsertParams{
		ID:        cr.ID.String(),
		UserID:    cr.UserID.String(),
		NewEmail:  cr.NewEmail,
		Code:      cr.Code,
		CreatedAt: cr.CreatedAt,
		UpdatedAt: cr.UpdatedAt,
		ExpiredAt: cr.ExpiredAt,
	})
}

func (r *EmailChangeRequestRepo) GetByUser(ctx context.Context, userID vo.Id) (*model.EmailChangeRequest, error) {
	row, err := r.q.GetUserEmailChangeRequestByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Email change request not found")
		}
		return nil, err
	}
	id, perr := vo.ParseId(row.ID)
	if perr != nil {
		return nil, perr
	}
	uid, perr := vo.ParseId(row.UserID)
	if perr != nil {
		return nil, perr
	}
	return &model.EmailChangeRequest{ID: id, UserID: uid, NewEmail: row.NewEmail, Code: row.Code,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ExpiredAt: row.ExpiredAt}, nil
}
```

Create `internal/user/repo/emailchangerequest_sqlite.go` (mirror `emailverification_sqlite.go`):

```go
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type emailChangeSqliteQuerier struct{}

var _ emailChangeQuerier = emailChangeSqliteQuerier{}

func (emailChangeSqliteQuerier) DeleteUserEmailChangeRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).DeleteUserEmailChangeRequestsByUser(ctx, userID)
}

func (emailChangeSqliteQuerier) InsertUserEmailChangeRequest(ctx context.Context, db backend.DBTX, p emailChangeInsertParams) error {
	return sqlitegen.New(db).InsertUserEmailChangeRequest(ctx, p)
}

func (emailChangeSqliteQuerier) GetUserEmailChangeRequestByUser(ctx context.Context, db backend.DBTX, userID string) (emailChangeRow, error) {
	return sqlitegen.New(db).GetUserEmailChangeRequestByUser(ctx, userID)
}
```

Create `internal/user/repo/emailchangerequest_pgsql.go`:

```go
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type emailChangePgsqlQuerier struct{}

var _ emailChangeQuerier = emailChangePgsqlQuerier{}

func (emailChangePgsqlQuerier) DeleteUserEmailChangeRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteUserEmailChangeRequestsByUser(ctx, userID)
}

func (emailChangePgsqlQuerier) InsertUserEmailChangeRequest(ctx context.Context, db backend.DBTX, p emailChangeInsertParams) error {
	return pgsqlgen.New(db).InsertUserEmailChangeRequest(ctx, pgsqlgen.InsertUserEmailChangeRequestParams(p))
}

func (emailChangePgsqlQuerier) GetUserEmailChangeRequestByUser(ctx context.Context, db backend.DBTX, userID string) (emailChangeRow, error) {
	row, err := pgsqlgen.New(db).GetUserEmailChangeRequestByUser(ctx, userID)
	return emailChangeRow(row), err
}
```

- [ ] **Step 7: Repo integration test** — create `internal/user/repo/emailchangerequest_integration_test.go` mirroring the email-verification repo test (find it in the repo test files and copy its structure). Assert: `Save` then `GetByUser` round-trips `NewEmail`+`Code`; `GetByUser` on a missing user returns `*errs.NotFoundError`; `Save` twice for the same user violates the `UNIQUE(user_id)` unless `DeleteByUser` is called first (mirror how the verification test handles replace — the app layer deletes-then-saves in one tx). Use the existing repo test harness (`newRepos(t)` / `dbtest`).

- [ ] **Step 8: Build + test** — Run: `go build ./... && go test ./internal/user/repo/ -run EmailChange`. Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/model/user_email_change_request.go internal/infra/storage/migrations internal/infra/storage/sqlc internal/user/repo/emailchangerequest*.go internal/user/repository.go
git commit -m "feat(user): add users_email_change_requests store"
```

---

### Task 2: Mailer, error code, rate scopes, config, i18n catalogue

Add the supporting infra the use cases will need. All additive; the mailer/config are constructed/wired in Task 3.

**Files:**
- Create: `internal/infra/mailer/change_email.go`
- Modify: `internal/infra/mailer/reset.go` (extend `EmailKeys`)
- Modify: `internal/shared/errs/codes.go` (new code)
- Modify: `internal/user/ports.go` (new rate scopes)
- Modify: `internal/config/config.go` (new rate-limit fields + parsing)
- Modify: `locales/en.json`, `locales/ru.json` (email + error catalogue keys)
- Modify: `.env.example`, `CLAUDE.md` (document the new env vars)

**Interfaces:**
- Produces: `mailer.NewChangeEmailSender(m, from, replyTo) *mailer.ChangeEmailSender` with `SendEmailChangeCode(ctx, to, name, code)` and `SendEmailChangeNotice(ctx, to, name, newEmail)`; `errs.CodeUserEmailUnchanged`; `user.RateScopeRequestEmailChange`, `user.RateScopeConfirmEmailChange`, `user.RateScopeEmailChangeSent`; `config.Config.RateLimitRequestEmailChange`, `.RateLimitConfirmEmailChange`.

- [ ] **Step 1: Mailer** — create `internal/infra/mailer/change_email.go` (mirror `verify.go`):

```go
package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// ChangeEmailSender sends the two self-service change-email messages: the code
// to the NEW address, and a heads-up notice to the OLD address.
type ChangeEmailSender struct {
	m       Mailer
	from    string
	replyTo string
}

func NewChangeEmailSender(m Mailer, from, replyTo string) *ChangeEmailSender {
	return &ChangeEmailSender{m: m, from: from, replyTo: replyTo}
}

// SendEmailChangeCode emails the confirmation code to the proposed NEW address.
func (s *ChangeEmailSender) SendEmailChangeCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.change_email.subject", nil)
	body := i18n.T(lang, "emails.change_email.body", map[string]any{"name": name, "code": code})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}

// SendEmailChangeNotice emails the OLD address that a change was requested,
// naming the proposed new address so an unwanted change is noticeable.
func (s *ChangeEmailSender) SendEmailChangeNotice(ctx context.Context, to, name, newEmail string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.change_email_notice.subject", nil)
	body := i18n.T(lang, "emails.change_email_notice.body", map[string]any{"name": name, "email": newEmail})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}
```

- [ ] **Step 2: Extend `EmailKeys`** — in `internal/infra/mailer/reset.go`, append the four keys:

```go
var EmailKeys = []string{"emails.reset.subject", "emails.reset.body", "emails.verify.subject", "emails.verify.body",
	"emails.change_email.subject", "emails.change_email.body",
	"emails.change_email_notice.subject", "emails.change_email_notice.body"}
```

- [ ] **Step 3: Error code** — in `internal/shared/errs/codes.go`, add to the const block (after `CodeUserVerificationCodeExpired`):

```go
	CodeUserEmailUnchanged = "user.email_unchanged"
```

and to the `AllCodes` slice (in the same user section):

```go
	CodeUserEmailUnchanged,
```

- [ ] **Step 4: Rate scopes** — in `internal/user/ports.go`, add to the rate-scope const block:

```go
	RateScopeRequestEmailChange = "request-email-change"
	RateScopeConfirmEmailChange = "confirm-email-change"
	// RateScopeEmailChangeSent is a timestamp channel (like RateScopeVerifySent),
	// not a cap: it records the last change-email code send per user for the
	// resend cooldown. It carries no configured limit.
	RateScopeEmailChangeSent = "email-change-sent"
```

- [ ] **Step 5: Config** — in `internal/config/config.go`, add fields (after `RateLimitConfirmEmail`):

```go
	RateLimitRequestEmailChange int           // ECONUMO_RATE_LIMIT_REQUEST_EMAIL_CHANGE: change-email code sends per user (every send counts)
	RateLimitConfirmEmailChange int           // ECONUMO_RATE_LIMIT_CONFIRM_EMAIL_CHANGE: failed confirm-email-change attempts per user
```

and add to the `for _, p := range []struct{...}` parse list:

```go
		{&c.RateLimitRequestEmailChange, "ECONUMO_RATE_LIMIT_REQUEST_EMAIL_CHANGE", 3},
		{&c.RateLimitConfirmEmailChange, "ECONUMO_RATE_LIMIT_CONFIRM_EMAIL_CHANGE", 5},
```

- [ ] **Step 6: i18n catalogue** — add to `locales/en.json` (under the existing `emails` object and `errors.user` object; match the surrounding style):

```
"emails": {
  ...existing keys...,
  "change_email": {
    "subject": "Confirm your new email",
    "body": "Hi {name},\n\nUse this code to confirm your new email address: {code}\n\nThe code expires in 10 minutes. If you did not request this, you can ignore this email."
  },
  "change_email_notice": {
    "subject": "Email change requested",
    "body": "Hi {name},\n\nSomeone requested to change the email on your account to {email}. If this was not you, please change your password immediately."
  }
}
```
```
"errors": { "user": { ..., "email_unchanged": "The new email is the same as your current email." } }
```

Add the exact-parallel Russian strings to `locales/ru.json` with the SAME `{name}`/`{code}`/`{email}` placeholders (translate the copy). Keep both files valid JSON and alphabetically consistent with neighbors.

- [ ] **Step 7: Docs** — in `.env.example` add `ECONUMO_RATE_LIMIT_REQUEST_EMAIL_CHANGE` and `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL_CHANGE` (with the defaults, next to the other `ECONUMO_RATE_LIMIT_*`); add one line each to the CLAUDE.md rate-limit list.

- [ ] **Step 8: Build + i18n guards** — Run: `go build ./... && go test ./internal/test/i18ntest/... ./internal/shared/errs/...`. Expected: PASS (EmailKeys ↔ emails.* and AllCodes ↔ errors.* two-way guards satisfied). If a guard fails, a key is missing in one language or one registry — fix and re-run.

- [ ] **Step 9: Commit**

```bash
git add internal/infra/mailer internal/shared/errs/codes.go internal/user/ports.go internal/config/config.go locales .env.example CLAUDE.md
git commit -m "feat(user): change-email mailer, error code, rate scopes, config, i18n"
```

---

### Task 3: Service use cases + DTOs + wiring

The core: `RequestEmailChange`, `ConfirmEmailChange`, `ResendEmailChangeCode`, plus the DTOs, the Service struct/constructor growth (new repo + mailer fields), and the `server.go` wiring (rate limits + mailer + repo). Update every `NewService` caller.

**Files:**
- Modify: `internal/model/user_dto.go` (three DTOs + results)
- Create: `internal/user/change_email.go`
- Modify: `internal/user/usecase.go` (Service struct fields + `NewService` params)
- Modify: `internal/server/server.go` (construct repo + mailer; add rate-limit entries; pass to `NewService`)
- Modify: every other `NewService` caller — `internal/user/admin_integration_test.go` (`newUserSvc`), `internal/user/api/harness_test.go` (`newHarness…`), and any others found by `grep -rn "appuser.NewService\|user.NewService\|NewService(" internal/`
- Test: `internal/user/change_email_integration_test.go`

**Interfaces:**
- Consumes: Task 1 (`EmailChangeRequests`, model), Task 2 (mailer, rate scopes, errs code).
- Produces: `Service.RequestEmailChange(ctx, userID vo.Id, req model.RequestEmailChangeRequest) (*model.RequestEmailChangeResult, error)`; `Service.ConfirmEmailChange(ctx, userID, currentTokenID vo.Id, req model.ConfirmEmailChangeRequest) (*model.CurrentUserResult, error)`; `Service.ResendEmailChangeCode(ctx, userID vo.Id) (*model.ResendEmailChangeCodeResult, time.Duration, error)`.

- [ ] **Step 1: DTOs** — in `internal/model/user_dto.go` add:

```go
// RequestEmailChangeRequest is the request-email-change body.
type RequestEmailChangeRequest struct {
	NewEmail string `json:"newEmail"`
	Password string `json:"password"`
}

// Validate enforces newEmail NotBlank+Email and password NotBlank.
func (r RequestEmailChangeRequest) Validate() error {
	fields := validateEmailField("newEmail", r.NewEmail, 0)
	if r.Password == "" {
		fields = append(fields, errs.FieldError{Key: "password", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// RequestEmailChangeResult is the request-email-change response (empty object).
type RequestEmailChangeResult struct{}

// ConfirmEmailChangeRequest is the confirm-email-change body. No password: the
// emailed code (to the new address) is the proof of ownership.
type ConfirmEmailChangeRequest struct {
	Code string `json:"code"`
}

// Validate enforces code NotBlank.
func (r ConfirmEmailChangeRequest) Validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// ResendEmailChangeCodeResult is the resend-email-change-code response (empty
// object). The wait travels on the Retry-After header, not in the body.
type ResendEmailChangeCodeResult struct{}
```

- [ ] **Step 2: Service struct + constructor** — in `internal/user/usecase.go`, add two fields to the `Service` struct (next to `emailVerifications` / `verifyMailer`):

```go
	emailChangeRequests EmailChangeRequests
	changeMailer        *mailer.ChangeEmailSender
```

Add the two matching params to `NewService` (place them right after `emailVerifications EmailVerifications, verifyMailer *mailer.VerifySender`), and assign them in the struct literal. Keep parameter order stable; note the exact new signature so callers can be updated:

```
NewService(..., emailVerifications EmailVerifications, verifyMailer *mailer.VerifySender,
	emailChangeRequests EmailChangeRequests, changeMailer *mailer.ChangeEmailSender,
	avatars ..., clk ..., limiter AttemptLimiter, allowRegistration bool, trial string, emailVerification bool)
```

- [ ] **Step 3: Use cases** — create `internal/user/change_email.go`:

```go
// Self-service change-email use cases. The flow is fully authenticated (keyed
// by user id), verifies the NEW address with an emailed code, gates the request
// on the current password, and notifies the OLD address. On confirm it revokes
// every OTHER session (the presenting one survives); PATs are untouched.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// RequestEmailChange verifies the current password, checks the new email is
// free and different, stores a pending change (replacing any prior), emails the
// code to the new address, and notifies the old address.
func (s *Service) RequestEmailChange(ctx context.Context, userID vo.Id, req model.RequestEmailChangeRequest) (*model.RequestEmailChangeResult, error) {
	key := userID.String()
	if err := s.allowAttempt(RateScopeRequestEmailChange, key); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeRequestEmailChange, key) // every send counts toward the cap

	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !s.hasher.Verify(u.Algorithm, u.Password, req.Password, u.Salt) {
		return nil, &errs.ValidationError{Msg: "Password is not correct", MsgCode: errs.CodeUserPasswordIncorrect}
	}

	newEmail := strings.TrimSpace(req.NewEmail)
	currentEmail, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return nil, derr
	}
	if strings.EqualFold(newEmail, strings.TrimSpace(currentEmail)) {
		return nil, &errs.ValidationError{Msg: "The new email is the same as your current email.", MsgCode: errs.CodeUserEmailUnchanged}
	}
	exists, err := s.repo.ExistsByEmail(ctx, newEmail)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
	}

	now := s.clock.Now()
	if err := s.issueEmailChangeCode(ctx, u.ID, newEmail, u.Name, now); err != nil {
		return nil, err
	}
	s.markEmailChangeSent(key)
	if s.changeMailer != nil {
		if nerr := s.changeMailer.SendEmailChangeNotice(ctx, strings.TrimSpace(currentEmail), u.Name, newEmail); nerr != nil {
			return nil, nerr
		}
	}
	return &model.RequestEmailChangeResult{}, nil
}

// ConfirmEmailChange validates the code, commits the new email, marks it
// verified, deletes the pending row, and revokes other sessions. A
// missing/wrong/expired code is a generic invalid-code error (anti-enumeration,
// though this is authenticated); failed attempts count toward the cap.
func (s *Service) ConfirmEmailChange(ctx context.Context, userID, currentTokenID vo.Id, req model.ConfirmEmailChangeRequest) (*model.CurrentUserResult, error) {
	key := userID.String()
	if err := s.allowAttempt(RateScopeConfirmEmailChange, key); err != nil {
		return nil, err
	}
	invalid := &errs.ValidationError{Msg: "The confirmation code is not valid.", MsgCode: errs.CodeUserVerificationCodeInvalid}

	cr, err := s.emailChangeRequests.GetByUser(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeConfirmEmailChange, key)
			return nil, invalid
		}
		return nil, err
	}
	if HashResetCode(strings.TrimSpace(req.Code)) != cr.Code {
		s.failAttempt(RateScopeConfirmEmailChange, key)
		return nil, invalid
	}
	now := s.clock.Now()
	if cr.IsExpired(now) {
		s.failAttempt(RateScopeConfirmEmailChange, key)
		return nil, &errs.ValidationError{Msg: "The code is expired", MsgCode: errs.CodeUserVerificationCodeExpired}
	}
	// Commit-time race guard: the target could have been taken since the request.
	exists, eerr := s.repo.ExistsByEmail(ctx, cr.NewEmail)
	if eerr != nil {
		return nil, eerr
	}
	if exists {
		return nil, &errs.ValidationError{Msg: "User already exists", MsgCode: errs.CodeUserAlreadyExists}
	}

	encrypted, eerr := s.encode.Encode(cr.NewEmail)
	if eerr != nil {
		return nil, eerr
	}
	var updated *model.User
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u, gerr := s.repo.GetByID(ctx, userID)
		if gerr != nil {
			return gerr
		}
		u.UpdateEmail(encrypted, now)
		u.MarkEmailVerified(now)
		if serr := s.repo.Save(ctx, u); serr != nil {
			return serr
		}
		if derr := s.emailChangeRequests.DeleteByUser(ctx, userID); derr != nil {
			return derr
		}
		updated = u
		return nil
	}); err != nil {
		return nil, err
	}
	s.clearAttempt(RateScopeConfirmEmailChange, key)
	if err := s.revokeSessions(ctx, userID, currentTokenID, now); err != nil {
		return nil, err
	}
	return s.toCurrentUser(ctx, updated)
}

// ResendEmailChangeCode re-sends the code to the pending new address, at most
// once per resend gap. Silent no-op if there is no pending change. Returns the
// seconds-until-next as a Duration (the edge emits Retry-After).
func (s *Service) ResendEmailChangeCode(ctx context.Context, userID vo.Id) (*model.ResendEmailChangeCodeResult, time.Duration, error) {
	key := userID.String()
	now := s.clock.Now()
	wait := s.emailChangeSentCooldown(key, now)

	if err := s.allowAttempt(RateScopeRequestEmailChange, key); err != nil {
		return nil, 0, err
	}
	s.failAttempt(RateScopeRequestEmailChange, key)

	result := &model.ResendEmailChangeCodeResult{}
	fullGap := model.EmailVerificationResendGap
	if wait > 0 {
		return result, wait, nil
	}
	s.markEmailChangeSent(key)

	cr, err := s.emailChangeRequests.GetByUser(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return result, fullGap, nil // no pending change: silent no-op
		}
		return nil, 0, err
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	if err := s.issueEmailChangeCode(ctx, userID, cr.NewEmail, u.Name, now); err != nil {
		return nil, 0, err
	}
	return result, fullGap, nil
}

// issueEmailChangeCode generates a fresh code, replaces any pending row for the
// user (preserving newEmail), and emails the code to the new address. It does
// NOT rate-limit — callers own that.
func (s *Service) issueEmailChangeCode(ctx context.Context, userID vo.Id, newEmail, name string, now time.Time) error {
	code, err := generatePasswordCode()
	if err != nil {
		return err
	}
	cr := model.NewEmailChangeRequest(vo.NewId(), userID, newEmail, HashResetCode(code), now)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if derr := s.emailChangeRequests.DeleteByUser(ctx, userID); derr != nil {
			return derr
		}
		return s.emailChangeRequests.Save(ctx, cr)
	}); err != nil {
		return err
	}
	if s.changeMailer != nil {
		return s.changeMailer.SendEmailChangeCode(ctx, newEmail, name, code)
	}
	return nil
}

func (s *Service) markEmailChangeSent(key string) {
	if s.limiter != nil {
		s.limiter.Mark(RateScopeEmailChangeSent, key)
	}
}

func (s *Service) emailChangeSentCooldown(key string, now time.Time) time.Duration {
	if s.limiter == nil {
		return 0
	}
	last, ok := s.limiter.LastAttempt(RateScopeEmailChangeSent, key)
	if !ok {
		return 0
	}
	remaining := last.Add(model.EmailVerificationResendGap).Sub(now)
	if remaining <= 0 {
		return 0
	}
	if remaining > model.EmailVerificationResendGap {
		return model.EmailVerificationResendGap
	}
	if rem := remaining % time.Second; rem > 0 {
		remaining += time.Second - rem
	}
	return remaining
}
```

NOTE: verify `s.toCurrentUser(ctx, *model.User) (*model.CurrentUserResult, error)` exists on the Service (login uses `toCurrentUserWithEmail`; there is a `toCurrentUser` used by Register). If the exact name/signature differs, use the same helper Register uses to build `CurrentUserResult`. Confirm by reading `internal/user/usecase.go` / `register.go` before writing.

- [ ] **Step 4: Wire in `server.go`** — after `emailVerificationRepo := …`, add:

```go
	emailChangeRepo := userrepo.NewEmailChangeRequestRepo(cfg.DatabaseDriver, txm)
```
after `verifyMailer := …`, add:
```go
	changeMailer := mailer.NewChangeEmailSender(mailTransport, cfg.MailFrom, cfg.MailReplyTo)
```
add two entries to the `ratelimit.New` `Limits` map:
```go
			appuser.RateScopeRequestEmailChange: cfg.RateLimitRequestEmailChange,
			appuser.RateScopeConfirmEmailChange: cfg.RateLimitConfirmEmailChange,
```
and pass the two new args to `appuser.NewService(...)` in the correct position (right after `emailVerificationRepo, verifyMailer`):
```go
		passwordReqRepo, resetMailer, emailVerificationRepo, verifyMailer,
		emailChangeRepo, changeMailer,
		avatars, clk, authLimiter, cfg.AllowRegistration, cfg.Trial, cfg.EmailVerification,
```

- [ ] **Step 5: Update the other `NewService` callers** — Run `grep -rn "NewService(" internal/user internal/server internal/cli` to find every construction. Update each (the api harness `newHarness…`, `admin_integration_test.go`'s `newUserSvc`, the CLI container if it builds the service, and any trial/other test constructor) to pass the two new args. For test constructors that don't exercise change-email, pass a real `repo.NewEmailChangeRequestRepo(db.Engine, db.TX)` and a `nil` `*mailer.ChangeEmailSender` (the use cases guard `s.changeMailer != nil`, exactly like the existing nil-mailer pattern). Keep salt-free construction as-is.

- [ ] **Step 6: Service integration test** — create `internal/user/change_email_integration_test.go` using the existing salt-free `newUserSvc`-style harness (build the Service with a real `EmailChangeRequestRepo`, a real in-memory limiter where cooldown is tested, and a capturing mailer or `nil`). Cover:
  - happy path: seed a user; `RequestEmailChange` with correct password + a free new email → a pending row exists (assert via the repo) and (if a capturing mailer is wired) a code was sent to the new address + a notice to the old; then `ConfirmEmailChange` with the right code → `GetByEmail(newEmail)` finds the user, `GetByEmail(oldEmail)` does not, `email_verified` is true, the pending row is gone.
  - wrong password → `CodeUserPasswordIncorrect`, no pending row.
  - same-as-current email → `CodeUserEmailUnchanged`.
  - duplicate email (another user already has it) at request → `CodeUserAlreadyExists`; and at confirm (create the collision after request) → `CodeUserAlreadyExists`.
  - wrong code and expired code (advance the clock past 10m) → invalid / expired coded errors; failed attempts counted.
  - a second `RequestEmailChange` supersedes the first (only one pending row; the latest code wins).
  - session revocation: seed two sessions; after confirm, the presenting token survives and the other is revoked; a PAT survives.
  - resend cooldown: two `ResendEmailChangeCode` calls within the gap → the second returns a non-zero `time.Duration` and sends nothing.

  Mirror assertion helpers/patterns from `internal/user/verify_email_test.go` and `internal/user/admin_integration_test.go`.

- [ ] **Step 7: Build + test** — Run: `go build ./... && go vet ./internal/... && go test ./internal/user/...`. Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/model/user_dto.go internal/user internal/server/server.go
git commit -m "feat(user): change-email use cases, DTOs, and wiring"
```

---

### Task 4: HTTP edge — handlers, routes, apiparity

**Files:**
- Create: `internal/user/api/change_email.go`
- Modify: `internal/user/api/routes.go` (three authenticated routes)
- Modify: apiparity scenario catalogue (add scenarios) + regenerate goldens
- Regenerate: OpenAPI docs (`make swagger`)

**Interfaces:**
- Consumes: `Service.RequestEmailChange` / `ConfirmEmailChange` / `ResendEmailChangeCode`.

- [ ] **Step 1: Handlers** — create `internal/user/api/change_email.go` (mirror `password.go` for `endpoint.Handle` and `user.go`'s `ResendVerificationCode` for the hand-written Retry-After one). Include the swag `// @…` annotation blocks (copy the shape from `UpdatePassword` and `ResendVerificationCode`, adjusting summary/description/router path). Bodies:

```go
func (h *Handlers) RequestEmailChange(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.RequestEmailChangeRequest) (*model.RequestEmailChangeResult, error) {
		return h.svc.RequestEmailChange(ctx, userID, req)
	})
}

func (h *Handlers) ConfirmEmailChange(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.ConfirmEmailChangeRequest) (*model.CurrentUserResult, error) {
		tokenID, _ := middleware.TokenIDFromCtx(ctx)
		return h.svc.ConfirmEmailChange(ctx, userID, tokenID, req)
	})
}

// Hand-written (not a combinator): the cooldown travels on Retry-After, and the
// combinators cannot set response headers. Mirrors ResendVerificationCode.
func (h *Handlers) ResendEmailChangeCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, retryAfter, err := h.svc.ResendEmailChangeCode(r.Context(), userID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter/time.Second)))
	}
	httpx.OK(w, res)
}
```

Match the import set of the sibling handler files (`context`, `net/http`, `strconv`, `time`, `endpoint`, `httpx`, `middleware`, `model`, `vo`).

- [ ] **Step 2: Routes** — in `internal/user/api/routes.go`, add to the authenticated group:

```go
		mux.Handle("POST /api/v1/user/request-email-change", auth(h.RequestEmailChange))
		mux.Handle("POST /api/v1/user/confirm-email-change", auth(h.ConfirmEmailChange))
		mux.Handle("POST /api/v1/user/resend-email-change-code", auth(h.ResendEmailChangeCode))
```

Update the route-count comment in `RegisterAPI`'s doc (it says "mounts the 24 user endpoints" → now 27).

- [ ] **Step 3: apiparity scenarios** — add scenarios for the three routes to the apiparity catalogue (find where user POST scenarios are registered, e.g. `internal/test/apiparity/…`; mirror the `update-password` and `confirm-email` scenarios). For `request-email-change`, a scenario needs an authed user + a valid password + a free email; for `confirm-email-change`, the deterministic code is unknowable across a black-box replay — follow how the existing `confirm-email` scenario handles this (it asserts the generic invalid-code path with a wrong code, which is deterministic). Add at least: request success (200 empty envelope), request with wrong password (400), request same-as-current (400), confirm with wrong code (400 invalid), resend (200 with Retry-After). Respect the guard tests (scenario count and scanned-route count must not shrink; every route needs a scenario).

- [ ] **Step 4: Regenerate goldens + docs** — Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` then `make swagger`. INSPECT the golden diff: it must contain ONLY the new endpoints' scenarios (and the OpenAPI additions). Confirm `mcpparity` goldens are unchanged (`git status`). Never hand-edit a golden.

- [ ] **Step 5: Full gate** — Run: `make go-test`. Expected: PASS (build, vet, gofmt, OpenAPI-fresh, sqlite unit/integration, apiparity + mcpparity goldens, i18n guards, coverage). Then `make test-repo-pgsql` — PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/user/api internal/test/apiparity internal/web/apidoc docs
git commit -m "feat(user): change-email HTTP endpoints"
```

---

### Task 5: Frontend — API client, hooks, routing, metrics

**Files:**
- Modify: `web/src/api/user.ts` (three functions)
- Modify: `web/src/features/user/queries.ts` (three hooks)
- Modify: `web/src/lib/metrics.ts` (one metric key)
- Modify: `web/src/app/router-pages.ts` (route constant), `web/src/app/routes.tsx` (register route + import)

**Interfaces:**
- Produces: `userApi.requestEmailChange(newEmail, password)`, `userApi.confirmEmailChange(code): Promise<CurrentUserDto>`, `userApi.resendEmailChangeCode(): Promise<number>`; hooks `useRequestEmailChange`, `useConfirmEmailChange`, `useResendEmailChangeCode`; `RouterPage.SETTINGS_CHANGE_EMAIL`; `METRICS.USER_CHANGE_EMAIL`.

- [ ] **Step 1: API client** — in `web/src/api/user.ts` add (mirroring `updatePassword` / `resendVerificationCode`; `confirmEmailChange` returns the refreshed user from the envelope like `getUserData`):

```ts
export async function requestEmailChange(newEmail: string, password: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/request-email-change'), { newEmail, password })
}

export async function confirmEmailChange(code: string): Promise<CurrentUserDto> {
  const response = await api.post<{ data: CurrentUserDto }>(apiUrl('/api/v1/user/confirm-email-change'), { code })
  return response.data.data
}

export async function resendEmailChangeCode(): Promise<number> {
  const response = await api.post(apiUrl('/api/v1/user/resend-email-change-code'), {})
  const seconds = Number(response.headers?.['retry-after'])
  return Number.isFinite(seconds) && seconds > 0 ? seconds : 0
}
```

(Confirm the confirm-endpoint envelope shape against the backend `CurrentUserResult` — it is returned as `httpx.OK(w, *CurrentUserResult)`, i.e. `{ data: <user> }`, so `response.data.data` is the user.)

- [ ] **Step 2: Metric** — in `web/src/lib/metrics.ts`, add to `METRICS` (in the user block):

```ts
  USER_CHANGE_EMAIL: 'appUserChangeEmail',
```

- [ ] **Step 3: Hooks** — in `web/src/features/user/queries.ts` add (mirror `useUpdatePassword` + the auth resend hook; confirm updates the cached user and fires the metric):

```ts
export function useRequestEmailChange() {
  return useMutation({
    mutationFn: ({ newEmail, password }: { newEmail: string; password: string }) =>
      userApi.requestEmailChange(newEmail, password),
  })
}

export function useConfirmEmailChange() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ code }: { code: string }) => userApi.confirmEmailChange(code),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      trackEvent(METRICS.USER_CHANGE_EMAIL)
    },
  })
}

export function useResendEmailChangeCode() {
  return useMutation({
    mutationFn: () => userApi.resendEmailChangeCode(),
  })
}
```

Add `requestEmailChange`, `confirmEmailChange`, `resendEmailChangeCode` to the `import * as userApi` usage (already a namespace import).

- [ ] **Step 4: Route constant only** — in `web/src/app/router-pages.ts` add (next to `SETTINGS_CHANGE_PASSWORD`):

```ts
  SETTINGS_CHANGE_EMAIL: '/settings/profile/change-email',
```

Do NOT register the route in `routes.tsx` yet — that import (`ChangeEmailPage`) doesn't exist until Task 6, and each task must build green. The `routes.tsx` import + registration lands in Task 6 alongside the page. Adding an unused constant is fine (it's a plain object literal).

- [ ] **Step 5: Build + tests** — Run (from `web/`): `pnpm exec tsc -b && pnpm test`. Expected: PASS. `metrics-coverage.test.ts` passes because `USER_CHANGE_EMAIL` is referenced by `useConfirmEmailChange`.

- [ ] **Step 6: Commit**

```bash
git add web/src/api/user.ts web/src/features/user/queries.ts web/src/lib/metrics.ts web/src/app/router-pages.ts
git commit -m "feat(web): change-email api client, hooks, metric, route constant"
```

---

### Task 6: Frontend — ChangeEmailPage, ProfilePage link, UI i18n

**Files:**
- Create: `web/src/features/settings/ChangeEmailPage.tsx`
- Modify: `web/src/app/routes.tsx` (the route import from Task 5 Step 4, if deferred)
- Modify: `web/src/features/settings/ProfilePage.tsx` (a "Change email" affordance)
- Modify: `locales/en.json`, `locales/ru.json` (UI strings under `user`/`settings`)
- Test: `web/src/features/settings/ChangeEmailPage.test.tsx`

**Interfaces:**
- Consumes: Task 5 hooks + `RouterPage.SETTINGS_CHANGE_EMAIL`.

- [ ] **Step 1: Page** — create `web/src/features/settings/ChangeEmailPage.tsx`, a two-phase form (request → code) inside `SettingsShell`, mirroring `ChangePasswordPage` for structure and `VerifyEmailDialog` for the code step + resend cooldown. Behavior:
  - Phase 1 (`'request'`): inputs new email + current password; `validate()` (non-empty email via existing `isNotEmpty`/an email check, non-empty password); on submit call `useRequestEmailChange().mutate({ newEmail, password })`; on success switch to phase `'confirm'` and seed a 60s resend cooldown; on error render `apiErrorMessage(err)` (and `apiFieldErrors` for inline field messages).
  - Phase 2 (`'confirm'`): a 6-digit code input (mirror `VerifyEmailDialog`'s `inputMode="numeric"`, `autoComplete="one-time-code"`, `maxLength={6}`, `isValidRecoveryCode`); a resend button using the deadline/cooldown pattern from `VerifyEmailDialog` (seed from `useResendEmailChangeCode().mutateAsync()` return + `retryAfterSeconds(err)` on error); on submit call `useConfirmEmailChange().mutate({ code })`; on success show a success dialog and navigate back to `RouterPage.SETTINGS_PROFILE`.
  - Use `SettingsShell` with `title`, `backTo={RouterPage.SETTINGS_PROFILE}`, and the same `crumbs` shape as `ChangePasswordPage`.
  - All copy via `t('user.change_email.*')` keys (added in Step 4).

- [ ] **Step 2: Route import** — ensure `web/src/app/routes.tsx` imports and registers `ChangeEmailPage` at `/settings/profile/change-email` (from Task 5 Step 4).

- [ ] **Step 3: ProfilePage affordance** — in `web/src/features/settings/ProfilePage.tsx`, add a "Change email" entry. Simplest and consistent: add a `<Link to={RouterPage.SETTINGS_CHANGE_EMAIL}>` row in the "Security" group stack (mirroring the change-password `<Link>`), labelled `t('user.page.settings.profile.change_email.menu_item')`. (Leave the read-only email field as the display; the link is the action.)

- [ ] **Step 4: UI i18n** — add to `locales/en.json` under `user` a `change_email` block with all the strings the page references (header, menu_item, form labels/placeholders/validation for email + password + code, resend/resend_in, loading, success text, error header/text) — model the key shape on the existing `user.change_password.*` and `auth.verify_email.*` blocks. Add exact-parallel Russian strings to `locales/ru.json` with matching placeholders (e.g. `{seconds}`, `{email}`). The i18ntest frontend-`t()`-coverage guard requires every `t('user.change_email.*')` key used in the page to exist in the catalogue (and vice-versa), so add exactly the keys the page uses.

- [ ] **Step 5: Component test** — create `ChangeEmailPage.test.tsx` mirroring the existing settings-page tests (and any `VerifyEmailDialog`/verification test) with msw-mocked endpoints: request → phase switches to code entry; confirm success → success UI + `queryClient` user updated; wrong-code error surfaces `apiErrorMessage`; resend disabled during cooldown. Follow the repo's vitest + msw patterns (`web/src/test`).

- [ ] **Step 6: Build + lint + tests** — Run (from `web/`): `pnpm exec tsc -b && pnpm lint && pnpm test`. Expected: PASS. Then from repo root run the i18n guards: `go test ./internal/test/i18ntest/...` — PASS (frontend `t()` key coverage).

- [ ] **Step 7: Commit**

```bash
git add web/src/features/settings/ChangeEmailPage.tsx web/src/features/settings/ProfilePage.tsx web/src/app/routes.tsx web/src/features/settings/ChangeEmailPage.test.tsx locales
git commit -m "feat(web): change-email page and profile entry"
```

---

## Final verification (after all tasks)

- `make go-test` (smoke) — PASS, goldens as expected, coverage ≥ gate.
- `make test-repo-pgsql` + `enginecompare` — PASS (both engines identical).
- `web/`: `pnpm exec tsc -b && pnpm lint && pnpm test` — PASS.
- Drive the flow end to end against a scratch instance (see the `econumo-e2e-verify-recipe` memory): register/login, request an email change, read the console-transport code + notice, confirm, verify login with the new email and that other sessions were revoked.

## Notes for the implementer

- The Service depends on **concrete** mailer types, not an interface — the change-email mailer is a new `*mailer.ChangeEmailSender` field, guarded `!= nil` exactly like `verifyMailer`.
- Keep the confirm flow authenticated and keyed by **user id** (not email) — do not copy registration's public/username-keyed shape.
- `RateScopeEmailChangeSent` is a timestamp channel: declare the constant but do NOT add it to the `ratelimit.New` `Limits` map (mirrors `RateScopeVerifySent`).
- Before writing `ConfirmEmailChange`'s return, confirm the exact `toCurrentUser`-style helper the Service exposes and reuse it.
