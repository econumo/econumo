# Email Verification After Registration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Users on instances with `ECONUMO_EMAIL_VERIFICATION=true` must confirm ownership of their email (typed code, sent at the first blocked login) before they can sign in.

**Architecture:** Verification is folded into the existing login endpoint: the login request gains optional `code`/`resend` fields; an unverified user with correct credentials gets HTTP 403 (a status unused on this route, which is how the SPA distinguishes the state) and the code email is sent; re-submitting login with a valid code marks the email verified and returns the normal `{token,user}` in one round trip. Storage mirrors the password-reset pattern exactly (`users_email_verifications` ≙ `users_password_requests`).

**Tech Stack:** Go (stdlib-first backend, sqlc dual-engine persistence), React 19 + Vite SPA, vitest + msw, locales/{en,ru}.json shared i18n.

**Spec:** `docs/superpowers/specs/2026-07-21-email-verification-design.md` (approved).

## Global Constraints

- Feature is a no-op unless `ECONUMO_EMAIL_VERIFICATION=true` (default `false`, strict boolean parse — malformed fails at boot).
- Existing users are backfilled **verified** (`email_verified` column `DEFAULT '1'/'true' NOT NULL`); CLI/admin-created users are always verified.
- No new HTTP route. Login's frozen `{token,user}` success shape and all existing goldens must not change (default config = flag off).
- SQL files (queries and migrations) must be **ASCII-only** in comments — an em dash in a query `.sql` mangles sqlc v1.30 sqlite codegen.
- Comments follow CLAUDE.md: why-not-what, no scaffolding comments; swag `@` blocks are exempt.
- Error catalogue: every new `errs.Code*` const goes into `errs.AllCodes` AND gets `errors.*` entries in BOTH `locales/en.json` and `locales/ru.json` (i18ntest enforces two-way). The `en` catalogue text must equal the Go literal `Msg` strings exactly.
- Every new user-facing SPA action fires a `METRICS` event (frozen `app`-prefixed camelCase; coverage test enforces).
- Frontend done-gate: `pnpm test`, `pnpm lint`, AND `pnpm exec tsc -b` (vitest/oxlint do not type-check).
- Go done-gate per task: `gofmt -l .` clean, `go vet ./...`, targeted `go test`; full `make go-test` in the final task.
- Commit after every task on branch `feature/email-verification`.

---

### Task 1: Config flag, rate-limit knob, boot warning

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/econumo/main.go` (serve boot warning, next to the DATA_SALT warn at ~line 161)
- Modify: `.env.example`
- Modify: `CLAUDE.md` (Configuration section)

**Interfaces:**
- Produces: `config.Config.EmailVerification bool` and `config.Config.RateLimitVerifyEmail int` — consumed by Task 6 (server wiring).

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go` (uses the same `t.Setenv` style as `TestLoad_CheckUpdates`; every `config.Load` test must also set `DATABASE_URL`, mirroring the neighboring tests — copy the exact env preamble used by `TestLoad_CheckUpdates`):

```go
func TestLoad_EmailVerification(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/test.sqlite")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EmailVerification {
		t.Error("EmailVerification should default to false")
	}
	if cfg.RateLimitVerifyEmail != 3 {
		t.Errorf("RateLimitVerifyEmail = %d, want default 3", cfg.RateLimitVerifyEmail)
	}

	t.Setenv("ECONUMO_EMAIL_VERIFICATION", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load with flag: %v", err)
	}
	if !cfg.EmailVerification {
		t.Error("EmailVerification should be true")
	}

	t.Setenv("ECONUMO_EMAIL_VERIFICATION", "banana")
	if _, err := Load(); err == nil {
		t.Error("malformed ECONUMO_EMAIL_VERIFICATION must fail at boot")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_EmailVerification -v`
Expected: FAIL — `cfg.EmailVerification undefined`.

- [ ] **Step 3: Implement**

In `internal/config/config.go`:

(a) Add to the `Config` struct, after the `Trial` field:

```go
	EmailVerification bool // ECONUMO_EMAIL_VERIFICATION: unverified users must confirm an emailed code at login (default false)
```

and in the rate-limit block, after `RateLimitAccept`:

```go
	RateLimitVerifyEmail int // ECONUMO_RATE_LIMIT_VERIFY_EMAIL: verification-code emails per username (every send counts)
```

(b) In `Load()`, after the `c.Trial` validation block:

```go
	// Strict parse for the same reason as ECONUMO_ANALYTICS: a typo must fail
	// at boot, not silently disable (or enable) the verification gate.
	emailVerification, err := getBoolStrict("ECONUMO_EMAIL_VERIFICATION", false)
	if err != nil {
		return Config{}, err
	}
	c.EmailVerification = emailVerification
```

(c) Add to the rate-limit `for` loop table (after the `RateLimitAccept` row):

```go
		{&c.RateLimitVerifyEmail, "ECONUMO_RATE_LIMIT_VERIFY_EMAIL", 3},
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/config/ -v -run TestLoad_EmailVerification`
Expected: PASS. Also run `go test ./internal/config/` (whole package) — PASS.

- [ ] **Step 5: Boot warning in serve**

In `cmd/econumo/main.go`, directly after the existing `cfg.DataSalt` warn block (~line 167):

```go
	// Verification emails through the console transport only reach stdout, so
	// enabling the gate without a real mailer would strand new users.
	if cfg.EmailVerification && cfg.MailProvider == "console" {
		slog.Warn("ECONUMO_EMAIL_VERIFICATION is enabled but MAILER_DSN is the console transport; " +
			"verification codes will only be printed to the server log")
	}
```

- [ ] **Step 6: Document**

`.env.example` — add next to `ECONUMO_TRIAL` / the rate-limit block (match the file's existing comment style):

```
# Require new users to confirm their email with a code at first login (default false).
#ECONUMO_EMAIL_VERIFICATION=false
# Verification-code emails per username per window (default 3).
#ECONUMO_RATE_LIMIT_VERIFY_EMAIL=3
```

`CLAUDE.md` — in the Configuration section add a bullet after `ECONUMO_TRIAL`:

```
- `ECONUMO_EMAIL_VERIFICATION` — require newly registered users to confirm an emailed
  code at login before their first session (default `false`; strict boolean, malformed
  fails at boot). The code email is sent at the first blocked login attempt, not at
  registration; `serve` WARNs at boot when enabled with the console mail transport.
  Existing rows and CLI/admin-created users are always verified.
```

and extend the rate-limit bullet with: `ECONUMO_RATE_LIMIT_VERIFY_EMAIL` — verification-code emails per username per window (default `3`; every send counts).

- [ ] **Step 7: Verify and commit**

Run: `gofmt -l . && go vet ./internal/config/ ./cmd/... && go test ./internal/config/`
Expected: no gofmt output, tests PASS.

```bash
git add internal/config/ cmd/econumo/main.go .env.example CLAUDE.md
git commit -m "feat: ECONUMO_EMAIL_VERIFICATION config flag + verify-email rate-limit knob"
```

---

### Task 2: `users.email_verified` column, end to end

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260721000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260721000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql`
- Modify: `internal/infra/storage/sqlc/query/pgsql/users.sql`
- Modify: `internal/model/user.go`
- Modify: `internal/user/repo/repo.go`
- Modify: `internal/user/repo/repo_integration_test.go` (or a new test in that package)
- Generated: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/**` (via `sqlc generate`)

**Interfaces:**
- Produces: `model.User.EmailVerified bool` (default `true` from `NewUser`), mutators `(*model.User).MarkEmailVerified(now time.Time)` and `(*model.User).RequireEmailVerification()`; column persisted through `Repository.Save` / hydrated by `GetByID`/`GetByIdentifier`. Consumed by Tasks 6 and 7.

- [ ] **Step 1: Migrations**

`internal/infra/storage/migrations/sqlite/20260721000000.sql` (ASCII only — mirrors the is_active ALTER form exactly):

```sql
ALTER TABLE users ADD COLUMN email_verified BOOLEAN DEFAULT '1' NOT NULL;
```

`internal/infra/storage/migrations/pgsql/20260721000000.sql`:

```sql
ALTER TABLE users ADD COLUMN email_verified BOOLEAN DEFAULT true NOT NULL;
```

(The verification-codes table is Task 3 and will be APPENDED to these same two files.)

- [ ] **Step 2: Queries**

In `internal/infra/storage/sqlc/query/sqlite/users.sql`:
- `GetUserByID` and `GetUserByIdentifier`: append `, email_verified` at the END of the select list (after `timezone`) — field order determines the generated struct order, and the repo's `userRow(row)` type conversions require it.
- `UpsertUser`: add `email_verified` as the LAST insert column with a matching `?`, and add to the `DO UPDATE SET` list:

```sql
-- name: UpsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active, access_level, access_until, email_verified)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    identifier = excluded.identifier,
    email      = excluded.email,
    name       = excluded.name,
    avatar = excluded.avatar,
    password   = excluded.password,
    salt       = excluded.salt,
    algorithm  = excluded.algorithm,
    updated_at = excluded.updated_at,
    is_active  = excluded.is_active,
    access_level = excluded.access_level,
    access_until = excluded.access_until,
    email_verified = excluded.email_verified;
```

Mirror all three changes in `internal/infra/storage/sqlc/query/pgsql/users.sql` with `$N` placeholders (renumber: `UpsertUser` gains `$14`).

- [ ] **Step 3: Regenerate sqlc**

Run: `go generate ./internal/infra/storage/sqlc/`
Expected: exit 0; `git status` shows changes only under `internal/infra/storage/sqlc/gen/`.

- [ ] **Step 4: Model**

In `internal/model/user.go`:
- Add to the `User` struct after `IsActive bool`: `EmailVerified bool`.
- In `NewUser`, add `EmailVerified: true,` after `IsActive: true,` (verification is opt-in per instance; the registration path downgrades explicitly).
- Add mutators after `Deactivate`:

```go
// RequireEmailVerification marks a freshly created user as needing email
// verification before the first login (ECONUMO_EMAIL_VERIFICATION). Creation
// time only; no UpdatedAt bump.
func (u *User) RequireEmailVerification() { u.EmailVerified = false }

// MarkEmailVerified records proof of mailbox ownership, bumping UpdatedAt only
// on a real state change.
func (u *User) MarkEmailVerified(now time.Time) {
	if u.EmailVerified {
		return
	}
	u.EmailVerified = true
	u.UpdatedAt = now
}
```

- [ ] **Step 5: Repo**

In `internal/user/repo/repo.go`:
- `userRow` struct: append `EmailVerified bool` as the LAST field (after `Timezone string`) — must match the generated row order from Step 2.
- `Save`: add `EmailVerified: u.EmailVerified,` to the `userParams{...}` literal.
- `hydrate`: add `EmailVerified: row.EmailVerified,` to the `model.User{...}` literal (next to `IsActive`).

The `sqlite.go`/`pgsql.go` adapters need no edits (whole-struct conversions).

- [ ] **Step 6: Write the round-trip test**

Append to `internal/user/repo/repo_integration_test.go` (mirror its existing imports/setup helpers; it already uses `dbtest`):

```go
func TestUserEmailVerifiedRoundTrip(t *testing.T) {
	db := dbtest.New(t)
	r := NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	u := model.NewUser(r.NextIdentity(), "email-verified-rt", "cipher", "RT", "face:blue", "hash", "salt", now)
	if !u.EmailVerified {
		t.Fatal("NewUser must default EmailVerified to true")
	}
	u.RequireEmailVerification()
	if err := r.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := r.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EmailVerified {
		t.Error("persisted EmailVerified=false must survive the round trip")
	}

	got.MarkEmailVerified(now.Add(time.Minute))
	if err := r.Save(ctx, got); err != nil {
		t.Fatalf("Save verified: %v", err)
	}
	again, err := r.GetByIdentifier(ctx, "email-verified-rt")
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if !again.EmailVerified {
		t.Error("MarkEmailVerified must persist")
	}
}
```

Note: if the package's test file uses different constructor helpers, keep the body and adapt only the setup lines to the file's local idiom.

- [ ] **Step 7: Run and verify the backfill default**

Run: `go test ./internal/user/repo/ -run TestUserEmailVerifiedRoundTrip -v`
Expected: PASS.
Run: `go build ./... && go test ./internal/user/... ./internal/test/apiparity/`
Expected: PASS — goldens untouched (the column is in no response DTO).

- [ ] **Step 8: Commit**

```bash
git add internal/infra/storage/ internal/model/user.go internal/user/repo/
git commit -m "feat: users.email_verified column (backfilled verified) through model and repo"
```

---

### Task 3: `users_email_verifications` table, model, port, repo

**Files:**
- Modify: `internal/infra/storage/migrations/sqlite/20260721000000.sql` (append)
- Modify: `internal/infra/storage/migrations/pgsql/20260721000000.sql` (append)
- Create: `internal/infra/storage/sqlc/query/sqlite/email_verification.sql`
- Create: `internal/infra/storage/sqlc/query/pgsql/email_verification.sql`
- Create: `internal/model/user_email_verification.go`
- Modify: `internal/user/repository.go` (new port)
- Create: `internal/user/repo/emailverification.go`
- Create: `internal/user/repo/emailverification_sqlite.go`
- Create: `internal/user/repo/emailverification_pgsql.go`
- Create: `internal/user/repo/emailverification_integration_test.go`

**Interfaces:**
- Produces: `model.EmailVerification` (`NewEmailVerification(id, userID vo.Id, code string, now time.Time) *EmailVerification`, `IsExpired(now) bool`, 10-min TTL), port `user.EmailVerifications { GetByUser(ctx, userID vo.Id) (*model.EmailVerification, error); Save(ctx, *model.EmailVerification) error; DeleteByUser(ctx, userID vo.Id) error }`, constructor `userrepo.NewEmailVerificationRepo(driver string, tx *backend.TxManager) *EmailVerificationRepo`. Consumed by Tasks 6 and 7.

- [ ] **Step 1: Append the table to both migration files**

sqlite (`20260721000000.sql`, after the ALTER; ASCII only — code is CHAR(64) because only the sha256 hex is stored):

```sql
CREATE TABLE users_email_verifications
(
    id         CHAR(36) NOT NULL
    , user_id    CHAR(36) NOT NULL
    , code       CHAR(64) NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , expired_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);
```

pgsql (same file name, after the ALTER):

```sql
CREATE TABLE users_email_verifications
(
    id         UUID NOT NULL
    , user_id    UUID NOT NULL
    , code       CHAR(64) NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expired_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);
```

- [ ] **Step 2: Query files**

`internal/infra/storage/sqlc/query/sqlite/email_verification.sql` (ASCII-only comments):

```sql
-- Login email-verification codes (users_email_verifications). The login flow
-- replaces the user's old code with a fresh one, reads it back by user, and
-- deletes it once verified. Expiry is compared in the app layer (Go time),
-- not in SQL, to avoid engine date-format differences.

-- name: DeleteUserEmailVerificationsByUser :exec
DELETE FROM users_email_verifications WHERE user_id = ?;

-- name: InsertUserEmailVerification :exec
INSERT INTO users_email_verifications (id, user_id, code, created_at, updated_at, expired_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetUserEmailVerificationByUser :one
SELECT id, user_id, code, created_at, updated_at, expired_at
FROM users_email_verifications
WHERE user_id = ?;
```

pgsql variant: same three queries with `$1..$6` placeholders and the comment
"See the sqlite sibling for the flow; expiry is compared in the app layer, not SQL."

Run: `go generate ./internal/infra/storage/sqlc/` — exit 0.

- [ ] **Step 3: Model**

`internal/model/user_email_verification.go`:

```go
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// emailVerificationTTL is how long a login verification code stays valid.
const emailVerificationTTL = 10 * time.Minute

// EmailVerification is a pending email-verification code for a user, mirroring
// PasswordRequest: the code is generated (and hashed) by the application layer
// and passed in; the request is replaced, never updated.
type EmailVerification struct {
	ID        vo.Id
	UserID    vo.Id
	Code      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiredAt time.Time
}

// NewEmailVerification builds a fresh verification expiring 10 minutes from now.
func NewEmailVerification(id, userID vo.Id, code string, now time.Time) *EmailVerification {
	return &EmailVerification{
		ID:        id,
		UserID:    userID,
		Code:      code,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiredAt: now.Add(emailVerificationTTL),
	}
}

// IsExpired reports whether the code is no longer valid at the given time.
func (v *EmailVerification) IsExpired(now time.Time) bool { return now.After(v.ExpiredAt) }
```

- [ ] **Step 4: Port**

Append to `internal/user/repository.go`:

```go
// EmailVerifications persists login email-verification codes
// (users_email_verifications) for the ECONUMO_EMAIL_VERIFICATION flow. One
// outstanding row per user; GetByUser on a missing row returns
// *errs.NotFoundError.
type EmailVerifications interface {
	GetByUser(ctx context.Context, userID vo.Id) (*model.EmailVerification, error)
	Save(ctx context.Context, v *model.EmailVerification) error
	DeleteByUser(ctx context.Context, userID vo.Id) error
}
```

- [ ] **Step 5: Repo (three files, mirroring `passwordrequest*.go` exactly)**

`internal/user/repo/emailverification.go`:

```go
// EmailVerificationRepo persists login email-verification codes
// (users_email_verifications). Expiry is evaluated in the domain
// (EmailVerification.IsExpired), not in SQL.
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
	emailVerificationRow          = sqlitegen.UsersEmailVerification
	emailVerificationInsertParams = sqlitegen.InsertUserEmailVerificationParams
)

type emailVerificationQuerier interface {
	DeleteUserEmailVerificationsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserEmailVerification(ctx context.Context, db backend.DBTX, p emailVerificationInsertParams) error
	GetUserEmailVerificationByUser(ctx context.Context, db backend.DBTX, userID string) (emailVerificationRow, error)
}

type EmailVerificationRepo struct {
	tx *backend.TxManager
	q  emailVerificationQuerier
}

var _ user.EmailVerifications = (*EmailVerificationRepo)(nil)

func NewEmailVerificationRepo(driver string, tx *backend.TxManager) *EmailVerificationRepo {
	switch driver {
	case "sqlite":
		return &EmailVerificationRepo{tx: tx, q: emailVerificationSqliteQuerier{}}
	case "postgresql":
		return &EmailVerificationRepo{tx: tx, q: emailVerificationPgsqlQuerier{}}
	default:
		panic("emailverificationrepo: unknown database driver " + driver)
	}
}

func (r *EmailVerificationRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *EmailVerificationRepo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserEmailVerificationsByUser(ctx, r.db(ctx), userID.String())
}

func (r *EmailVerificationRepo) Save(ctx context.Context, v *model.EmailVerification) error {
	return r.q.InsertUserEmailVerification(ctx, r.db(ctx), emailVerificationInsertParams{
		ID:        v.ID.String(),
		UserID:    v.UserID.String(),
		Code:      v.Code,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
		ExpiredAt: v.ExpiredAt,
	})
}

func (r *EmailVerificationRepo) GetByUser(ctx context.Context, userID vo.Id) (*model.EmailVerification, error) {
	row, err := r.q.GetUserEmailVerificationByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Email verification not found")
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
	return &model.EmailVerification{ID: id, UserID: uid, Code: row.Code,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ExpiredAt: row.ExpiredAt}, nil
}
```

(Adjust `sqlitegen.UsersEmailVerification` to the actual generated type name from Step 2 — check `internal/infra/storage/sqlc/gen/sqlite/models.go`; sqlc singularizes table names, so it should be `UsersEmailVerification`.)

`internal/user/repo/emailverification_sqlite.go`:

```go
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type emailVerificationSqliteQuerier struct{}

var _ emailVerificationQuerier = emailVerificationSqliteQuerier{}

func (emailVerificationSqliteQuerier) DeleteUserEmailVerificationsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).DeleteUserEmailVerificationsByUser(ctx, userID)
}

func (emailVerificationSqliteQuerier) InsertUserEmailVerification(ctx context.Context, db backend.DBTX, p emailVerificationInsertParams) error {
	return sqlitegen.New(db).InsertUserEmailVerification(ctx, p)
}

func (emailVerificationSqliteQuerier) GetUserEmailVerificationByUser(ctx context.Context, db backend.DBTX, userID string) (emailVerificationRow, error) {
	return sqlitegen.New(db).GetUserEmailVerificationByUser(ctx, userID)
}
```

`internal/user/repo/emailverification_pgsql.go`:

```go
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type emailVerificationPgsqlQuerier struct{}

var _ emailVerificationQuerier = emailVerificationPgsqlQuerier{}

func (emailVerificationPgsqlQuerier) DeleteUserEmailVerificationsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteUserEmailVerificationsByUser(ctx, userID)
}

func (emailVerificationPgsqlQuerier) InsertUserEmailVerification(ctx context.Context, db backend.DBTX, p emailVerificationInsertParams) error {
	return pgsqlgen.New(db).InsertUserEmailVerification(ctx, pgsqlgen.InsertUserEmailVerificationParams(p))
}

func (emailVerificationPgsqlQuerier) GetUserEmailVerificationByUser(ctx context.Context, db backend.DBTX, userID string) (emailVerificationRow, error) {
	row, err := pgsqlgen.New(db).GetUserEmailVerificationByUser(ctx, userID)
	return emailVerificationRow(row), err
}
```

- [ ] **Step 6: Write the integration test**

`internal/user/repo/emailverification_integration_test.go`:

```go
package repo

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestEmailVerificationRepoLifecycle(t *testing.T) {
	db := dbtest.New(t)
	users := NewRepo(db.Engine, db.TX)
	r := NewEmailVerificationRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	u := model.NewUser(users.NextIdentity(), "ev-lifecycle", "cipher", "EV", "face:blue", "hash", "salt", now)
	if err := users.Save(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("GetByUser on empty table must return NotFound")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFoundError, got %v", err)
	}

	ev := model.NewEmailVerification(vo.NewId(), u.ID, "hash-one", now)
	if err := r.Save(ctx, ev); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := r.GetByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if got.Code != "hash-one" || !got.ExpiredAt.Equal(now.Add(10*time.Minute)) {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.IsExpired(now.Add(9 * time.Minute)) {
		t.Error("code must be valid inside the TTL")
	}
	if !got.IsExpired(now.Add(11 * time.Minute)) {
		t.Error("code must expire after the TTL")
	}

	// Replace pattern: delete old, insert fresh (unique user_id).
	if err := r.DeleteByUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if err := r.Save(ctx, model.NewEmailVerification(vo.NewId(), u.ID, "hash-two", now)); err != nil {
		t.Fatalf("Save replacement: %v", err)
	}
	got, err = r.GetByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByUser after replace: %v", err)
	}
	if got.Code != "hash-two" {
		t.Errorf("Code = %q, want hash-two", got.Code)
	}
}
```

- [ ] **Step 7: Run and commit**

Run: `go test ./internal/user/repo/ -run TestEmailVerification -v && go build ./...`
Expected: PASS.

```bash
git add internal/infra/storage/ internal/model/user_email_verification.go internal/user/repository.go internal/user/repo/
git commit -m "feat: users_email_verifications store mirroring the password-request pattern"
```

---

### Task 4: Verification email sender + `emails.verify.*` catalogue keys

**Files:**
- Create: `internal/infra/mailer/verify.go`
- Modify: `internal/infra/mailer/reset.go` (the `EmailKeys` var)
- Modify: `locales/en.json`, `locales/ru.json` (the `emails` object)

**Interfaces:**
- Produces: `mailer.NewVerifySender(m Mailer, from, replyTo string) *VerifySender` with `SendVerificationCode(ctx context.Context, to, name, code string) error`. Consumed by Task 6.

- [ ] **Step 1: Catalogue keys (both languages)**

`locales/en.json`, inside `"emails"` after the `"reset"` object:

```json
"verify": {
 "subject": "Email verification code",
 "body": "Hi {name},\nYour confirmation code is: {code}.\n\nEnter this code on the sign-in screen to verify your email address.\n\nIf you didn't create an Econumo account, please ignore this email.\n\n--\nEconumo — Manage money. Together.\n"
}
```

`locales/ru.json`, same position:

```json
"verify": {
 "subject": "Код подтверждения email",
 "body": "Здравствуйте, {name}!\nВаш код подтверждения: {code}.\n\nВведите этот код на экране входа, чтобы подтвердить адрес электронной почты.\n\nЕсли вы не создавали аккаунт Econumo, просто проигнорируйте это письмо.\n\n--\nEconumo — Управляйте деньгами. Вместе.\n"
}
```

- [ ] **Step 2: Register the keys with the i18n guard, then watch it fail**

In `internal/infra/mailer/reset.go` change:

```go
var EmailKeys = []string{"emails.reset.subject", "emails.reset.body", "emails.verify.subject", "emails.verify.body"}
```

Run: `go test ./internal/test/i18ntest/ -v`
Expected: PASS (keys exist in both catalogues with matching `{name}`/`{code}` placeholder sets). If you add the keys to only one language or with mismatched placeholders, this test FAILS — that is the guard working.

- [ ] **Step 3: The sender**

`internal/infra/mailer/verify.go`:

```go
package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// VerifySender builds and sends the login email-verification code email,
// mirroring ResetSender so the app layer stays free of any mail dependency.
type VerifySender struct {
	m       Mailer
	from    string
	replyTo string
}

// NewVerifySender wires the verification email sender over a Mailer with the
// configured From / Reply-To addresses (the from / reply_to query params of
// MAILER_DSN).
func NewVerifySender(m Mailer, from, replyTo string) *VerifySender {
	return &VerifySender{m: m, from: from, replyTo: replyTo}
}

// SendVerificationCode emails the verification code to the user in the
// caller's language.
func (s *VerifySender) SendVerificationCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.verify.subject", nil)
	body := i18n.T(lang, "emails.verify.body", map[string]any{"name": name, "code": code})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}
```

- [ ] **Step 4: Verify and commit**

Run: `go build ./... && go test ./internal/test/i18ntest/ ./internal/infra/mailer/...`
Expected: PASS.

```bash
git add internal/infra/mailer/ locales/
git commit -m "feat: verification-code email sender + emails.verify catalogue keys"
```

---

### Task 5: Error codes + translated 403

**Files:**
- Modify: `internal/shared/errs/codes.go`
- Modify: `internal/shared/errs/errs.go` (`AccessDeniedError` gains an optional `Code`)
- Modify: `internal/web/httpx/errors.go` (translate the 403 message)
- Modify: `locales/en.json`, `locales/ru.json` (the `errors.user` object)
- Test: `internal/web/httpx/` (append to the package's existing `*_test.go`, or create `errors_403_test.go`)

**Interfaces:**
- Produces: `errs.CodeUserEmailVerificationRequired = "user.email_verification_required"`, `errs.CodeUserVerificationCodeInvalid = "user.verification_code_invalid"`; `errs.AccessDeniedError{Msg, Code}` now renders `Msg` through the `errors.*` catalogue when `Code` is set. Consumed by Task 6.

- [ ] **Step 1: Write the failing httpx test**

In `internal/web/httpx` (mirror the package's existing test style; the essential assertions):

```go
func TestWriteErrorAccessDeniedTranslatesCodedMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx := reqctx.WithLanguage(context.Background(), "ru")
	WriteError(ctx, rec, &errs.AccessDeniedError{
		Msg:  "Please verify your email address.",
		Code: errs.CodeUserEmailVerificationRequired,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Подтвердите адрес электронной почты.") {
		t.Errorf("message not translated: %s", body)
	}
	if !strings.Contains(body, `"errors":[]`) {
		t.Errorf("403 envelope must keep errors as an empty ARRAY: %s", body)
	}
}

func TestWriteErrorAccessDeniedWithoutCodeKeepsLiteral(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, errs.NewAccessDenied("You are not allowed"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "You are not allowed") {
		t.Errorf("code-less 403 must keep its literal message: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/web/httpx/ -run TestWriteErrorAccessDenied -v`
Expected: FAIL — `errs.CodeUserEmailVerificationRequired` undefined / `Code` field unknown.

- [ ] **Step 3: Implement**

(a) `internal/shared/errs/codes.go` — add to the user const block (after `CodeUserResetCodeExpired`):

```go
	CodeUserEmailVerificationRequired = "user.email_verification_required"
	CodeUserVerificationCodeInvalid   = "user.verification_code_invalid"
```

and add both strings to `AllCodes`.

(b) `internal/shared/errs/errs.go` — extend the struct:

```go
// AccessDeniedError maps to HTTP 403.
type AccessDeniedError struct {
	Msg string
	// Code is an optional errors.* catalogue key; when set the HTTP edge
	// renders Msg in the caller's language, otherwise the literal text is kept.
	Code string
}
```

(`NewAccessDenied` stays as-is — it builds a code-less error.)

(c) `internal/web/httpx/errors.go` — in `WriteError`, change the AccessDenied branch to:

```go
	if v, ok := errs.AsAccessDenied(err); ok {
		// 403 with errors:[] (empty ARRAY, not {}); a coded message renders in
		// the caller's language, a code-less one keeps its literal text.
		AccessDenied(w, translated(lang, v.Msg, v.Code, nil))
		return
	}
```

(d) Catalogue entries — `locales/en.json` `errors.user`, after `"reset_code_expired"`:

```json
"email_verification_required": "Please verify your email address.",
"verification_code_invalid": "The confirmation code is not valid."
```

`locales/ru.json` `errors.user`:

```json
"email_verification_required": "Подтвердите адрес электронной почты.",
"verification_code_invalid": "Неверный код подтверждения."
```

- [ ] **Step 4: Run and commit**

Run: `go test ./internal/web/httpx/ ./internal/test/i18ntest/ ./internal/shared/errs/`
Expected: PASS (i18ntest confirms the two-way code↔catalogue match).

```bash
git add internal/shared/errs/ internal/web/httpx/ locales/
git commit -m "feat: coded, translatable 403 for the email-verification handshake"
```

---

### Task 6: Login verification flow, registration gate, reset-verifies, wiring, swagger

This is the core task. All server-side behavior lands here, test-first.

**Files:**
- Modify: `internal/model/user_dto.go` (`LoginRequest` gains `Code`/`Resend`)
- Modify: `internal/user/ports.go` (`RateScopeVerifyEmail`)
- Modify: `internal/user/usecase.go` (Service fields + `NewService` signature)
- Create: `internal/user/verify_email.go` (the new use-case logic)
- Modify: `internal/user/login.go` (hook the check in)
- Modify: `internal/user/register.go` (born-unverified; `createUser` param rename)
- Modify: `internal/user/password.go` (`ResetPassword` marks verified)
- Modify: `internal/user/api/user.go` (swag `@Failure 403` on LoginUser)
- Modify: `internal/server/server.go` (wiring + limiter map)
- Modify: `internal/cli/container.go` (wiring)
- Modify: every test constructing `appuser.NewService` (compiler-driven; see Step 5)
- Create: `internal/user/verify_email_test.go`
- Generated: committed OpenAPI docs via `make swagger`

**Interfaces:**
- Consumes: Task 2 (`EmailVerified`, mutators), Task 3 (`EmailVerifications` port, `NewEmailVerificationRepo`), Task 4 (`*mailer.VerifySender`), Task 5 (codes + `AccessDeniedError.Code`), Task 1 (`cfg.EmailVerification`, `cfg.RateLimitVerifyEmail`).
- Produces: final `NewService` signature (all later tasks and tests use it):

```go
func NewService(
	repo Repository,
	tx port.TxRunner,
	encode *auth.EncodeService,
	hasher *auth.PasswordHasher,
	tokens AccessTokens,
	currency CurrencyLookup,
	budgets BudgetAccess,
	passwordRequests PasswordRequests,
	mailer *mailer.ResetSender,
	emailVerifications EmailVerifications,
	verifyMailer *mailer.VerifySender,
	avatars AvatarPicker,
	clock port.Clock,
	limiter AttemptLimiter,
	allowRegistration bool,
	trial string,
	emailVerification bool,
) *Service
```

- [ ] **Step 1: Write the failing integration tests**

`internal/user/verify_email_test.go` (package `user_test`, same imports as `admin_integration_test.go` plus `internal/infra/mailer`):

```go
package user_test

import (
	"context"
	"strings"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// captureMailer records outgoing messages so tests can read the emailed code.
type captureMailer struct{ msgs []mailer.Message }

func (c *captureMailer) Send(_ context.Context, m mailer.Message) error {
	c.msgs = append(c.msgs, m)
	return nil
}

// codeFrom extracts the 12-char hex code from the rendered email body
// ("Your confirmation code is: <code>.").
func codeFrom(t *testing.T, body string) string {
	t.Helper()
	const marker = "code is: "
	i := strings.Index(body, marker)
	if i < 0 || len(body) < i+len(marker)+12 {
		t.Fatalf("no code in email body: %q", body)
	}
	return body[i+len(marker) : i+len(marker)+12]
}

// newVerifySvcFlag builds the user service with registration enabled, a
// capture mailer, and the email-verification gate set by enabled, mirroring
// server.Build's wiring.
func newVerifySvcFlag(t *testing.T, db *dbtest.DB, cap *captureMailer, enabled bool) *appuser.Service {
	t.Helper()
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	prRepo := userrepo.NewPasswordRequestRepo(db.Engine, db.TX)
	evRepo := userrepo.NewEmailVerificationRepo(db.Engine, db.TX)
	return appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets,
		prRepo, mailer.NewResetSender(cap, "noreply@econumo.test", ""),
		evRepo, mailer.NewVerifySender(cap, "noreply@econumo.test", ""),
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, true, "", enabled)
}

func newVerifySvc(t *testing.T, db *dbtest.DB, cap *captureMailer) *appuser.Service {
	t.Helper()
	return newVerifySvcFlag(t, db, cap, true)
}

func isVerificationDenied(err error, code string) bool {
	v, ok := errs.AsAccessDenied(err)
	return ok && v.Code == code
}

func TestLoginBlockedUntilEmailVerified(t *testing.T) {
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

	// Wrong password NEVER reaches the verification layer.
	_, err = svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "wrong"}, "ua", time.Now())
	if _, ok := errs.AsUnauthorized(err); !ok {
		t.Fatalf("bad password must stay a 401, got %v", err)
	}

	// Wrong code -> invalid-code 403.
	_, err = svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1", Code: "000000000000"}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("want verification_code_invalid, got %v", err)
	}

	// Correct code -> full login result in ONE call, user persisted verified.
	code := codeFrom(t, cap.msgs[0].Text)
	res, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1", Code: code}, "ua", time.Now())
	if err != nil {
		t.Fatalf("verified login: %v", err)
	}
	if res.Token == "" {
		t.Fatal("verified login must mint a session token")
	}

	// Subsequent logins skip the gate entirely.
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now()); err != nil {
		t.Fatalf("post-verification login: %v", err)
	}
}

func TestLoginResendForcesFreshCode(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Resend Me", Email: "resend@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	_, err := svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1", Resend: true}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserEmailVerificationRequired) {
		t.Fatalf("resend still answers verification_required, got %v", err)
	}
	if len(cap.msgs) != 2 {
		t.Fatalf("resend must send a fresh email, got %d", len(cap.msgs))
	}
	// The OLD code is dead, the NEW one works.
	oldCode := codeFrom(t, cap.msgs[0].Text)
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1", Code: oldCode}, "ua", time.Now()); !isVerificationDenied(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("replaced code must be invalid, got %v", err)
	}
	newCode := codeFrom(t, cap.msgs[1].Text)
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1", Code: newCode}, "ua", time.Now()); err != nil {
		t.Fatalf("fresh code must verify: %v", err)
	}
}

func TestFlagOffKeepsLoginUnchanged(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvcFlag(t, db, cap, false) // gate disabled, registration enabled
	ctx := context.Background()
	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Legacy Flow", Email: "legacy@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	// Flag off: users are born verified and login needs no code (and sends no email).
	u, err := userrepo.NewRepo(db.Engine, db.TX).GetByIdentifier(ctx, auth.NewEncodeService("").Hash("legacy@econumo.test"))
	if err != nil {
		t.Fatalf("load registered user: %v", err)
	}
	if !u.EmailVerified {
		t.Error("flag off: registration must create a VERIFIED user")
	}
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "legacy@econumo.test", Password: "secretpass1"}, "ua", time.Now()); err != nil {
		t.Fatalf("flag off: login must succeed without verification, got %v", err)
	}
	if len(cap.msgs) != 0 {
		t.Errorf("flag off must never send verification email, got %d", len(cap.msgs))
	}
}

func TestResetPasswordMarksEmailVerified(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Reset Me", Email: "reset-verify@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemindPassword(ctx, model.RemindPasswordRequest{Username: "reset-verify@econumo.test"}); err != nil {
		t.Fatal(err)
	}
	resetCode := codeFrom(t, cap.msgs[len(cap.msgs)-1].Text)
	if _, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
		Username: "reset-verify@econumo.test", Code: resetCode, Password: "newsecret1",
	}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	// A completed reset proved mailbox ownership: login needs no code now.
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "reset-verify@econumo.test", Password: "newsecret1"}, "ua", time.Now()); err != nil {
		t.Fatalf("login after reset must skip verification, got %v", err)
	}
}
```

Note on `RemindPassword`'s email body: it uses the marker `"code is: "` too (`emails.reset.body`), so `codeFrom` works for both. Note the reset flow requires `passwordRequests` + reset mailer — `newVerifySvc` wires both.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run 'TestLoginBlocked|TestLoginResend|TestFlagOff|TestResetPasswordMarks' -v`
Expected: FAIL to COMPILE — `model.LoginRequest` has no `Code`, `NewService` arity mismatch.

- [ ] **Step 3: DTO + rate scope + service fields**

(a) `internal/model/user_dto.go`, `LoginRequest`:

```go
// LoginRequest is the login request body (username and password both NotBlank).
// Code and Resend drive the email-verification handshake: both optional and
// additive, absent for verified users (see the 403 flow in login).
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
	Resend   bool   `json:"resend"`
}
```

(`Validate()` unchanged — the new fields are optional.)

(b) `internal/user/ports.go`, rate-scope consts:

```go
const (
	RateScopeLogin       = "login"
	RateScopeReset       = "reset"
	RateScopeRemind      = "remind"
	RateScopeRegister    = "register"
	RateScopeVerifyEmail = "verify-email"
)
```

(c) `internal/user/usecase.go` — add fields to `Service` (after `mailer`):

```go
	emailVerifications EmailVerifications
	verifyMailer       *mailer.VerifySender
```

and after `trial string`:

```go
	emailVerification bool
```

Extend `NewService` to the exact signature in this task's **Interfaces** block (new params `emailVerifications`, `verifyMailer` immediately after `mailer`; `emailVerification bool` last), assigning all three.

- [ ] **Step 4: The use-case logic**

(a) `internal/user/verify_email.go` (new file):

```go
// Email-verification-at-login use case (ECONUMO_EMAIL_VERIFICATION): an
// unverified user with correct credentials must present a code that was
// emailed to them. The code is only ever processed AFTER the password check,
// so this surface cannot be probed without valid credentials.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// verifyEmailOnLogin gates an unverified user's login. Without a code it
// ensures an outstanding verification email exists (sending one when none is
// live, or always on resend) and denies with email_verification_required.
// With a code it verifies and consumes it, letting the login proceed.
func (s *Service) verifyEmailOnLogin(ctx context.Context, u *model.User, email string, req model.LoginRequest, limitKey string) error {
	now := s.clock.Now()
	code := strings.TrimSpace(req.Code)
	if code == "" {
		if err := s.ensureVerificationCode(ctx, u, email, req.Resend, limitKey, now); err != nil {
			return err
		}
		return &errs.AccessDeniedError{Msg: "Please verify your email address.", Code: errs.CodeUserEmailVerificationRequired}
	}

	invalid := &errs.AccessDeniedError{Msg: "The confirmation code is not valid.", Code: errs.CodeUserVerificationCodeInvalid}
	ev, err := s.emailVerifications.GetByUser(ctx, u.ID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			s.failAttempt(RateScopeLogin, limitKey)
			return invalid
		}
		return err
	}
	if HashResetCode(code) != ev.Code || ev.IsExpired(now) {
		s.failAttempt(RateScopeLogin, limitKey)
		return invalid
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.MarkEmailVerified(s.clock.Now())
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		return s.emailVerifications.DeleteByUser(ctx, u.ID)
	})
}

// ensureVerificationCode sends a fresh code when none is outstanding, the
// outstanding one expired, or the caller forced a resend. Every send counts
// toward the verify-email rate cap; a live code is otherwise reused silently
// so repeated code-less logins cannot spam the mailbox.
func (s *Service) ensureVerificationCode(ctx context.Context, u *model.User, email string, force bool, limitKey string, now time.Time) error {
	send := force
	if !send {
		ev, err := s.emailVerifications.GetByUser(ctx, u.ID)
		switch {
		case err != nil:
			if _, ok := errs.AsNotFound(err); !ok {
				return err
			}
			send = true
		case ev.IsExpired(now):
			send = true
		}
	}
	if !send {
		return nil
	}
	if err := s.allowAttempt(RateScopeVerifyEmail, limitKey); err != nil {
		return err
	}
	s.failAttempt(RateScopeVerifyEmail, limitKey)
	code, err := generatePasswordCode()
	if err != nil {
		return err
	}
	ev := model.NewEmailVerification(vo.NewId(), u.ID, HashResetCode(code), now)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if derr := s.emailVerifications.DeleteByUser(ctx, u.ID); derr != nil {
			return derr
		}
		return s.emailVerifications.Save(ctx, ev)
	}); err != nil {
		return err
	}
	if s.verifyMailer != nil {
		return s.verifyMailer.SendVerificationCode(ctx, email, u.Name, code)
	}
	return nil
}
```

(b) `internal/user/login.go` — insert between the email decode and `purgeDeadTokens`:

```go
	if s.emailVerification && !u.EmailVerified {
		if err := s.verifyEmailOnLogin(ctx, u, email, req, limitKey); err != nil {
			return nil, err
		}
	}
```

(c) `internal/user/register.go` — rename `createUser`'s `grantTrial` param to `selfService` (update the doc comment accordingly: self-service registration grants a trial AND is subject to the verification gate; operator-provisioned accounts get neither) and after `u.SeedDefaultOptions(...)`:

```go
	if selfService && s.emailVerification {
		u.RequireEmailVerification()
	}
```

(the trial condition becomes `if selfService && s.trial == "end-of-next-month"`).

(d) `internal/user/password.go` — in `ResetPassword`'s transaction, before `s.repo.Save(ctx, u)`:

```go
		// Completing a reset proves mailbox ownership, so it also satisfies the
		// email-verification gate.
		u.MarkEmailVerified(s.clock.Now())
```

- [ ] **Step 5: Fix every call site (compiler-driven)**

Run `go build ./... && go vet ./...` and fix each `NewService` call the compiler reports:

- `internal/server/server.go`: pass `userrepo.NewEmailVerificationRepo(cfg.DatabaseDriver, txm)` and `mailer.NewVerifySender(mailTransport, cfg.MailFrom, cfg.MailReplyTo)` after `resetMailer`, and `cfg.EmailVerification` after `cfg.Trial`. Also add to the limiter map: `appuser.RateScopeVerifyEmail: cfg.RateLimitVerifyEmail,`.
- `internal/cli/container.go`: pass `userrepo.NewEmailVerificationRepo(cfg.DatabaseDriver, txm)` (the CLI's `user:verify-email` needs it, Task 7), `nil` for the verify mailer, and `cfg.EmailVerification`.
- Test call sites (`internal/user/{migrate,admin_integration,authenticate,trial_integration}_test.go`, `internal/user/api/harness_test.go`, `internal/server/glue_timezone_test.go`): insert `userrepo.NewEmailVerificationRepo(db.Engine, db.TX), nil,` after the reset-mailer argument (or `nil, nil,` where the file has no `db`/repo in scope and doesn't exercise verification), and append `false` after the trial argument. Use the real repo wherever a `*dbtest.DB` is already in scope — it costs nothing and keeps `AdminVerifyEmail` (Task 7) testable.

- [ ] **Step 6: Run the new tests**

Run: `go test ./internal/user/... -v -run 'TestLoginBlocked|TestLoginResend|TestFlagOff|TestResetPasswordMarks'`
Expected: PASS (all four).
Run: `go test ./internal/user/... ./internal/server/...`
Expected: PASS (no regression).

- [ ] **Step 7: Swagger + parity**

In `internal/user/api/user.go`, add to `LoginUser`'s swag block after the 401 line:

```go
// @Failure     403     {object} apidoc.JsonResponseError "Email verification required (ECONUMO_EMAIL_VERIFICATION): retry with the emailed code in the request body."
```

Run: `make swagger` — commit the regenerated docs.
Run: `go test ./internal/test/apiparity/ ./internal/test/mcpparity/`
Expected: PASS with NO golden diffs (default config keeps the flag off). If a golden changes, STOP and investigate — observable default behavior must not move.

- [ ] **Step 8: Commit**

```bash
git add internal/ docs/ && git status   # inspect: no stray files
git commit -m "feat: email verification folded into login (403 handshake, resend, reset-verifies)"
```

---

### Task 7: CLI — `user:verify-email`, `user:show` line

**Files:**
- Modify: `internal/user/admin.go` (`AdminVerifyEmail`)
- Modify: `internal/cli/user_commands.go`
- Modify: `internal/cli/commands_test.go`
- Modify: `CLAUDE.md` (CLI command list)

**Interfaces:**
- Consumes: `Service.userByEmail`, `MarkEmailVerified`, `EmailVerifications.DeleteByUser`.
- Produces: `Service.AdminVerifyEmail(ctx context.Context, email string) error`; CLI command `user:verify-email <email>`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/cli/commands_test.go`'s `TestUserSetAccessAndShowExitCodes`-style table (mirror how that test seeds its user) — new cases:

```go
		{"verify-email", []string{"user:verify-email", "access@example.test"}, 0},
		{"verify-email-unknown", []string{"user:verify-email", "nobody@example.test"}, 1},
		{"verify-email-too-few-args", []string{"user:verify-email"}, 1},
```

And a service-level test appended to `internal/user/verify_email_test.go`:

```go
func TestAdminVerifyEmail(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Admin Verify", Email: "admin-verify@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	// Trigger a pending code so the command also has a row to clean up.
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "admin-verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())

	if err := svc.AdminVerifyEmail(ctx, "admin-verify@econumo.test"); err != nil {
		t.Fatalf("AdminVerifyEmail: %v", err)
	}
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "admin-verify@econumo.test", Password: "secretpass1"}, "ua", time.Now()); err != nil {
		t.Fatalf("login after admin verify: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run TestAdminVerifyEmail -v`
Expected: FAIL — `AdminVerifyEmail` undefined.

- [ ] **Step 3: Implement**

(a) `internal/user/admin.go`, after `AdminDeactivate`:

```go
// AdminVerifyEmail marks a user's email verified (support/rescue hatch for
// the ECONUMO_EMAIL_VERIFICATION gate) and drops any pending code.
func (s *Service) AdminVerifyEmail(ctx context.Context, email string) error {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.MarkEmailVerified(s.clock.Now())
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		return s.emailVerifications.DeleteByUser(ctx, u.ID)
	})
}
```

(b) `internal/cli/user_commands.go`, after the `user:deactivate` entry:

```go
		{
			name:    "user:verify-email",
			summary: "Mark a user's email verified: user:verify-email <email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 1 {
					return usageErr("user:verify-email <email>")
				}
				email := strings.TrimSpace(args[0])
				if err := c.user.AdminVerifyEmail(ctx, email); err != nil {
					return err
				}
				fmt.Printf("Email verified for %s\n", email)
				return nil
			},
		},
```

(c) `user:show` output — after the `Active:` line:

```go
				verified := "no"
				if u.EmailVerified {
					verified = "yes"
				}
				fmt.Printf("Email verified:  %s\n", verified)
```

(d) `CLAUDE.md` CLI list — add `user:verify-email <email>` after `user:deactivate <email>`.

- [ ] **Step 4: Run and commit**

Run: `go test ./internal/cli/ ./internal/user/ -run 'TestUserSetAccess|TestAdminVerifyEmail'`
Expected: PASS.

```bash
git add internal/user/admin.go internal/user/verify_email_test.go internal/cli/ CLAUDE.md
git commit -m "feat: user:verify-email CLI rescue hatch + verified state in user:show"
```

---

### Task 8: SPA — API client, mutations, metrics

**Files:**
- Modify: `web/src/api/user.ts`
- Modify: `web/src/lib/apiError.ts`
- Modify: `web/src/features/auth/queries.ts`
- Modify: `web/src/lib/metrics.ts`
- Modify: `web/src/features/auth/queries.test.tsx`

**Interfaces:**
- Consumes: backend 403 handshake from Task 6.
- Produces (used by Task 9): `userApi.login(username, password, options?: { code?: string; resend?: boolean })`; `isForbidden(err: unknown): boolean` in `@/lib/apiError`; hooks `useLogin` (mutation vars `{ username, password, code? }`) and `useResendVerification` (vars `{ username, password }`); metrics keys `METRICS.EMAIL_VERIFICATION_COMPLETED` / `METRICS.EMAIL_VERIFICATION_RESENT`.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/features/auth/queries.test.tsx` (mirror its existing msw + renderHook harness — the file already tests `useLogin`):

```tsx
it('login passes the verification code through and fires the completed metric', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/user/login-user', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ token: 'tok', user: { id: 'u1', accessLevel: 'full', accessUntil: '' } })
    }),
  )
  const { result } = renderHook(() => useLogin(), { wrapper })
  await result.current.mutateAsync({ username: 'a@b.test', password: 'pw', code: '123456789012' })
  expect(bodies[0]).toMatchObject({ username: 'a@b.test', password: 'pw', code: '123456789012' })
})

it('resend treats the 403 reply as success', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Please verify your email address.', code: 403, errors: {} }, { status: 403 }),
    ),
  )
  const { result } = renderHook(() => useResendVerification(), { wrapper })
  await expect(result.current.mutateAsync({ username: 'a@b.test', password: 'pw' })).resolves.toBeUndefined()
})
```

(Adapt `wrapper`/`renderHook` imports to the file's existing local helpers — it already renders hooks against msw.)

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm test -- --run queries`
Expected: FAIL — `useResendVerification` not exported / login arity.

- [ ] **Step 3: Implement**

(a) `web/src/api/user.ts` — extend `login`:

```ts
export interface LoginOptions {
  code?: string
  resend?: boolean
}

// login-user is the one endpoint that responds with a bare {token, user}
// body instead of the standard {success, message, data} envelope. code/resend
// drive the email-verification handshake: an unverified user gets HTTP 403
// until a valid code is supplied alongside the credentials.
export async function login(username: string, password: string, options: LoginOptions = {}): Promise<UserLoginItemDto> {
  const response = await api.post<UserLoginItemDto>(apiUrl('/api/v1/user/login-user'), { username, password, ...options })
  const { user } = response.data
  setAnalyticsAccessState(deriveAccessState(user.accessLevel, user.accessUntil))
  return response.data
}
```

(b) `web/src/lib/apiError.ts` — append:

```ts
// The login 403 is the email-verification signal (no other 403 exists on that
// route); the dialog flow keys off the status alone.
export function isForbidden(err: unknown): boolean {
  return isAxiosError(err) && err.response?.status === 403
}
```

(c) `web/src/lib/metrics.ts` — add after `USER_RESET_PASSWORD`:

```ts
  EMAIL_VERIFICATION_COMPLETED: 'appEmailVerificationCompleted',
  EMAIL_VERIFICATION_RESENT: 'appEmailVerificationResent',
```

(d) `web/src/features/auth/queries.ts`:

```ts
import { isForbidden } from '@/lib/apiError'

export function useLogin() {
  return useMutation({
    mutationFn: ({ username, password, code }: { username: string; password: string; code?: string }) =>
      userApi.login(username, password, code ? { code } : {}),
    onSuccess: (data, variables) => {
      // the new session may belong to a different user — never restore the
      // previous user's persisted finances
      clearPersistedQueryCache()
      setToken(data.token)
      trackEvent(METRICS.USER_LOGIN)
      if (variables.code) {
        trackEvent(METRICS.EMAIL_VERIFICATION_COMPLETED)
      }
    },
  })
}

export function useResendVerification() {
  return useMutation({
    mutationFn: async ({ username, password }: { username: string; password: string }) => {
      // The server answers a resend with the verification-required 403 after
      // sending the fresh code — that IS the success case here.
      try {
        await userApi.login(username, password, { resend: true })
      } catch (err) {
        if (isForbidden(err)) return
        throw err
      }
    },
    onSuccess: () => trackEvent(METRICS.EMAIL_VERIFICATION_RESENT),
  })
}
```

- [ ] **Step 4: Run and commit**

Run: `cd web && pnpm test -- --run queries metrics-coverage && pnpm exec tsc -b && pnpm lint`
Expected: queries tests PASS; **metrics-coverage FAILS is acceptable ONLY if it flags the two new keys as unfired — Task 9 wires the dialog that fires them through these hooks; if the coverage test statically detects the `trackEvent` calls above it will already PASS.** If it fails, note it and confirm it passes at the end of Task 9. `tsc -b` and lint must PASS now.

```bash
git add web/src/api/user.ts web/src/lib/apiError.ts web/src/lib/metrics.ts web/src/features/auth/queries.ts web/src/features/auth/queries.test.tsx
git commit -m "feat: login code/resend plumbing, verification metrics, resend mutation"
```

---

### Task 9: SPA — VerifyEmailDialog + LoginPage wiring + catalogue keys

**Files:**
- Create: `web/src/features/auth/VerifyEmailDialog.tsx`
- Create: `web/src/features/auth/VerifyEmailDialog.test.tsx`
- Modify: `web/src/features/auth/LoginPage.tsx`
- Modify: `locales/en.json`, `locales/ru.json` (the `auth` object)

**Interfaces:**
- Consumes: Task 8's hooks and `isForbidden`.
- Produces: `<VerifyEmailDialog open onClose={...} username={...} password={...} />`.

- [ ] **Step 1: Catalogue keys**

`locales/en.json`, inside `"auth"` (sibling of `access_recovery_modal`):

```json
"verify_email": {
 "header": "Verify your email",
 "information": "We've sent a confirmation code to {email}. Enter it below to finish signing in.",
 "resent": "A new code has been sent.",
 "action": {
  "verify": "Verify and sign in",
  "resend": "Resend code"
 }
}
```

`locales/ru.json`:

```json
"verify_email": {
 "header": "Подтвердите ваш email",
 "information": "Мы отправили код подтверждения на {email}. Введите его ниже, чтобы завершить вход.",
 "resent": "Новый код отправлен.",
 "action": {
  "verify": "Подтвердить и войти",
  "resend": "Отправить код ещё раз"
 }
}
```

- [ ] **Step 2: Write the failing component test**

`web/src/features/auth/VerifyEmailDialog.test.tsx` (same harness as `RecoveryDialog.test.tsx`):

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { VerifyEmailDialog } from './VerifyEmailDialog'

function renderDialog(onClose = vi.fn()) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <VerifyEmailDialog open onClose={onClose} username="ada@example.test" password="pw12345678" />
    </QueryClientProvider>,
  )
  return onClose
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('verifies by re-submitting login with the code', async () => {
  const user = userEvent.setup()
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/user/login-user', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ token: 'tok-verified', user: { id: 'u1', accessLevel: 'full', accessUntil: '' } })
    }),
  )
  renderDialog()
  await user.type(screen.getByLabelText(/code/i), '123456789012')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  await vi.waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ username: 'ada@example.test', password: 'pw12345678', code: '123456789012' })
})

it('shows the server message inline on an invalid code', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'The confirmation code is not valid.', code: 403, errors: {} }, { status: 403 }),
    ),
  )
  renderDialog()
  await user.type(screen.getByLabelText(/code/i), '999999999999')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  expect(await screen.findByText(/confirmation code is not valid/i)).toBeInTheDocument()
})

it('resend confirms and stays open', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Please verify your email address.', code: 403, errors: {} }, { status: 403 }),
    ),
  )
  renderDialog()
  await user.click(screen.getByRole('button', { name: /resend code/i }))
  expect(await screen.findByText(/new code has been sent/i)).toBeInTheDocument()
})

it('is dismissible via Cancel', async () => {
  const user = userEvent.setup()
  const onClose = renderDialog()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onClose).toHaveBeenCalled()
})
```

Run: `cd web && pnpm test -- --run VerifyEmailDialog`
Expected: FAIL — module not found.

- [ ] **Step 3: The dialog**

`web/src/features/auth/VerifyEmailDialog.tsx`:

```tsx
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { apiErrorMessage } from '@/lib/apiError'
import { isNotEmpty, isValidRecoveryCode } from '@/lib/validation'
import { useLogin, useResendVerification } from './queries'

interface VerifyEmailForm {
  code: string
}

export function VerifyEmailDialog({ open, onClose, username, password }: {
  open: boolean
  onClose: () => void
  username: string
  password: string
}) {
  const { t } = useTranslation()
  const login = useLogin()
  const resend = useResendVerification()
  const [serverError, setServerError] = useState('')
  const [resent, setResent] = useState(false)
  const { register, handleSubmit, formState: { errors } } = useForm<VerifyEmailForm>({ mode: 'onTouched', defaultValues: { code: '' } })

  const onVerify = handleSubmit(async ({ code }) => {
    setServerError('')
    setResent(false)
    try {
      await login.mutateAsync({ username, password, code: code.trim() })
      window.location.assign('/')
    } catch (err) {
      setServerError(apiErrorMessage(err))
    }
  })

  const onResend = async () => {
    setServerError('')
    setResent(false)
    try {
      await resend.mutateAsync({ username, password })
      setResent(true)
    } catch (err) {
      setServerError(apiErrorMessage(err))
    }
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('auth.verify_email.header')}
      description={t('auth.verify_email.information', { email: username })}
    >
      <form onSubmit={onVerify} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="verify-email-code">{t('user.form.code.placeholder')}</Label>
          <Input
            className="h-11"
            id="verify-email-code"
            autoFocus
            {...register('code', {
              validate: {
                required: (v) => isNotEmpty(v) || t('user.form.code.validation.required_field'),
                code: (v) => isValidRecoveryCode(v.trim()) || t('user.form.code.validation.invalid_code'),
              },
            })}
          />
          {errors.code ? <p className="text-sm text-destructive">{errors.code.message}</p> : null}
        </div>

        {serverError ? <p className="text-sm text-destructive">{serverError}</p> : null}
        {resent ? <p className="text-sm text-muted-foreground">{t('auth.verify_email.resent')}</p> : null}

        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="button" variant="secondary" onClick={onResend} disabled={resend.isPending}>
            {t('auth.verify_email.action.resend')}
          </Button>
          <Button type="submit" disabled={login.isPending}>
            {t('auth.verify_email.action.verify')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
```

- [ ] **Step 4: LoginPage wiring**

In `web/src/features/auth/LoginPage.tsx`:
- Import: `import { isForbidden } from '@/lib/apiError'` and `import { VerifyEmailDialog } from './VerifyEmailDialog'`.
- Add state next to `failOpen`: `const [verifyOpen, setVerifyOpen] = useState(false)`.
- Change the submit catch:

```tsx
    } catch (err) {
      // 403 = correct credentials, unverified email: the server just sent (or
      // reused) a code — collect it instead of showing the generic failure.
      if (isForbidden(err)) {
        setVerifyOpen(true)
        return
      }
      setFailOpen(true)
    }
```

- Render next to `RecoveryDialog`:

```tsx
        {verifyOpen ? (
          <VerifyEmailDialog
            open
            onClose={() => setVerifyOpen(false)}
            username={getValues('username')}
            password={getValues('password')}
          />
        ) : null}
```

- Append to `web/src/features/auth/LoginPage.test.tsx` (mirror its harness):

```tsx
it('opens the verification dialog on a 403 login', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Please verify your email address.', code: 403, errors: {} }, { status: 403 }),
    ),
  )
  renderPage()   // the file's existing render helper
  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'pw12345678')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByText(/verify your email/i)).toBeInTheDocument()
  expect(screen.queryByText(/sign-in failed/i)).not.toBeInTheDocument()
})
```

- [ ] **Step 5: Run the full frontend gate**

Run: `cd web && pnpm test -- --run && pnpm lint && pnpm exec tsc -b`
Expected: ALL PASS — including `metrics-coverage.test.ts` (both new keys now fired via the dialog's hooks) and the i18n key-coverage guard (`go test ./internal/test/i18ntest/` — run it too, since the new `t()` keys must exist in both catalogues).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/auth/ locales/
git commit -m "feat: VerifyEmailDialog on the 403 login handshake"
```

---

### Task 10: Full-suite verification

**Files:** none new — this is the gate.

- [ ] **Step 1: Backend full smoke tier**

Run from the repo root: `make go-test`
Expected: PASS — build, vet, gofmt, OpenAPI-docs freshness, all unit/integration suites, coverage ≥ the gate. Golden diffs: NONE (`git status` clean of `testdata/golden`).

- [ ] **Step 2: Frontend**

Run: `make web-test && make web-lint && cd web && pnpm exec tsc -b`
Expected: PASS.

- [ ] **Step 3: Engine comparison (if a local Postgres is available)**

Run: `make test`
Expected: PASS — the pgsql adapters and the new migration/queries run against a real PostgreSQL. If no Postgres is available locally, state that explicitly in the task report; CI runs it regardless.

- [ ] **Step 4: Manual smoke (optional but recommended)**

```bash
ECONUMO_EMAIL_VERIFICATION=true make go-run
```

Register a user via the SPA, attempt login, read the code off the server stdout (console mailer), verify, land in the app. Also confirm the boot WARN line about the console transport appears.

- [ ] **Step 5: Final commit & wrap-up**

Any stragglers (`git status`) committed with an appropriate message. Then summarize: what shipped, test evidence, and that the branch `feature/email-verification` (PR #137) is ready for the implementation to be pushed.

---

## Self-review notes (already applied)

- **Spec coverage:** flag+strict parse (T1), migration/backfill (T2), codes table (T3), email+i18n (T4), coded 403 (T5), login flow/resend/rate-limit/reset-verifies/registration-gate/swagger (T6), CLI (T7), SPA plumbing+metrics (T8), dialog+copy (T9), guards & parity (T10). The spec's "flag-on apiparity scenarios if supported" is deliberately resolved as: rely on the Task 6 service-level integration tests (the parity harness builds one fixed default config; adding per-scenario config plumbing is out of scope).
- **Type consistency:** `NewService` final signature is stated once in Task 6 and referenced by Tasks 6–7 test helpers; `EmailVerifications` port methods match the repo implementation and all call sites; `isForbidden` lives in `@/lib/apiError` for both Task 8 hooks and Task 9 pages.
- **Known adaptation points (not placeholders):** exact local test-harness helper names (`renderPage`, `wrapper`) in the two existing SPA test files, and the sqlc-generated type name `UsersEmailVerification` — both are verified in-place by the implementer against files that already exist, with the expected shape stated here.
