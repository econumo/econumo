# Access Model and Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every user an access level and an optional expiry, grant a trial at registration, and reject writes from restricted users with HTTP 402 — without touching any feature package.

**Architecture:** Two columns on `users` (`access_level`, `access_until`) collapse to an effective level via a pure function of `(level, until, now)`. `Authenticate` grows a join onto `users` so `middleware.Auth` can reject `POST` requests from restricted callers against a path allowlist. Trials are granted at the single `createUser` choke point. Operators read and write the state through two new CLI commands.

**Tech Stack:** Go (stdlib `net/http`, `log/slog`), sqlc (per-engine generated code), SQLite + PostgreSQL, `go test`.

This is **Plan A of three** for `docs/superpowers/specs/2026-07-19-cloud-monetization-trial-access-design.md`. Plan B covers the admin listener, handoff token, and `create-billing-link`. Plan C covers the SPA and analytics. Nothing here depends on B or C; both depend on this.

## Global Constraints

- **Branch:** `feat/trial-access`. Work in the worktree at `.claude/worktrees/bridge-cse_01Ct1uZgHUM447ZMp4BPSLKZ`, never the main checkout.
- **Datetime layouts** live in `internal/shared/datetime` and there are exactly two: `datetime.Layout = "2006-01-02 15:04:05"` and `datetime.DateLayout = "2006-01-02"`. There is no `DateTimeLayout`.
- **Comments: write sparingly.** Only exceptional scenarios and non-obvious business/frozen-contract rationale. No godoc restating a signature, no section dividers, no references to the removed PHP implementation.
- **No feature package may be edited by the enforcement rule.** The 402 check lives inside `internal/web/middleware/auth.go`.
- **Migrations must exist in BOTH** `internal/infra/storage/migrations/sqlite/` and `internal/infra/storage/migrations/pgsql/` under the identical filename — the runner sorts by filename and the version sets must match.
- **sqlc:** after editing `internal/infra/storage/sqlc/query/{sqlite,pgsql}/*.sql`, run `sqlc generate` from `internal/infra/storage/sqlc/`. Query `.sql` files must be **ASCII-only** — an em dash in a comment corrupts sqlite codegen (byte-offset bug in sqlc v1.30).
- **`emit_pointers_for_null_types: true`** — a nullable `DATETIME`/`TIMESTAMP` surfaces as `*time.Time` on both engines.
- **Naming:** identifiers describe access, never money. `full_access`, never `paid`.
- **Error envelope shapes are frozen.** A 402 serializes as `{"success":false,"message":"...","code":402,"errors":{}}`.
- **Coverage gate:** `make go-test` enforces `GO_COVER_MIN` (default 72).
- Run `gofmt -w` on every file touched; `make go-lint` must pass before each commit.

### Harness facts (verified — do not re-derive)

- `internal/user/register_test.go`, `internal/user/read_test.go` and `internal/web/middleware/auth_test.go` **do not exist**. Registration and `get-user-data` are covered at the API layer in `internal/user/api/`; auth middleware tests live in `internal/web/middleware/middleware_test.go`.
- `newUserSvc`, `isValidation`, `isNotFound`, `isActive`, `testSalt` are already declared in `internal/user/admin_integration_test.go` and shared across the whole `user_test` package. **Do not redeclare them.**
- `appuser.NewService` argument order today: `(repo, tx, encode, hasher, tokens, currencyLookup, budgets, passwordReqs, resetMailer, avatarPicker, clock, limiter, allowRegistration)`.
- `internal/test/authstub.Authenticator` is a field-less struct whose `Authenticate` returns `(id, id, error)` parsed from the bearer token.
- CLI tests (`internal/cli/commands_test.go`, package `cli`) assert **only the integer exit code** from `Run(args)`. There is no stdout capture. `cliEnv(t)` migrates a temp sqlite file and sets `DATABASE_URL`.

---

### Task 1: Migration — add the two columns

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260719000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260719000000.sql`
- Test: `internal/user/repo/repo_integration_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `users.access_level TEXT NOT NULL DEFAULT 'full'` and nullable `users.access_until` on both engines.

- [ ] **Step 1: Write the failing test**

Append to `internal/user/repo/repo_integration_test.go`:

```go
func TestUserSchema_HasAccessColumns(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()

	var level string
	var until *time.Time
	err := db.Raw.QueryRowContext(ctx,
		db.Rebind(`SELECT access_level, access_until FROM users WHERE id = ?`),
		"00000000-0000-0000-0000-000000000000",
	).Scan(&level, &until)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("access columns not queryable: %v", err)
	}
}
```

Ensure `"database/sql"`, `"errors"` and `"time"` are in that file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/user/repo/ -run TestUserSchema_HasAccessColumns -v`
Expected: FAIL — `no such column: access_level`.

- [ ] **Step 3: Write the migrations**

`internal/infra/storage/migrations/sqlite/20260719000000.sql`:

```sql
ALTER TABLE users ADD COLUMN access_level TEXT NOT NULL DEFAULT 'full';
ALTER TABLE users ADD COLUMN access_until DATETIME;
```

`internal/infra/storage/migrations/pgsql/20260719000000.sql`:

```sql
ALTER TABLE users ADD COLUMN access_level TEXT NOT NULL DEFAULT 'full';
ALTER TABLE users ADD COLUMN access_until TIMESTAMP;
```

`NOT NULL DEFAULT 'full'` backfills every existing row in place: current users are grandfathered into permanent access and self-hosted instances see no behavior change.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/user/repo/ -run TestUserSchema_HasAccessColumns -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/storage/migrations/sqlite/20260719000000.sql \
        internal/infra/storage/migrations/pgsql/20260719000000.sql \
        internal/user/repo/repo_integration_test.go
git commit -m "feat: add access_level and access_until columns to users"
```

---

### Task 2: Effective access level on the entity

**Files:**
- Modify: `internal/model/user.go` (struct `:93-106`, `NewUser` `:111-125`, append the rest)
- Test: Create `internal/model/user_access_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type AccessLevel string`; `AccessLevelFull AccessLevel = "full"`; `AccessLevelReadonly AccessLevel = "readonly"`
  - fields `User.AccessLevel AccessLevel`, `User.AccessUntil *time.Time`
  - `func (u *User) EffectiveAccessLevel(now time.Time) AccessLevel`
  - `func (u *User) SetAccess(level AccessLevel, until *time.Time, now time.Time)`
  - `func ParseAccessLevel(s string) (AccessLevel, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/model/user_access_test.go`:

```go
package model

import (
	"testing"
	"time"
)

func accessAt(t time.Time) *time.Time { return &t }

func TestEffectiveAccessLevel(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		level AccessLevel
		until *time.Time
		want  AccessLevel
	}{
		{"no expiry keeps the level", AccessLevelFull, nil, AccessLevelFull},
		{"future expiry keeps the level", AccessLevelFull, accessAt(now.Add(time.Hour)), AccessLevelFull},
		{"past expiry restricts", AccessLevelFull, accessAt(now.Add(-time.Hour)), AccessLevelReadonly},
		{"expiry exactly now restricts", AccessLevelFull, accessAt(now), AccessLevelReadonly},
		{"readonly with future expiry stays readonly", AccessLevelReadonly, accessAt(now.Add(time.Hour)), AccessLevelReadonly},
		{"readonly with no expiry stays readonly", AccessLevelReadonly, nil, AccessLevelReadonly},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := &User{AccessLevel: tc.level, AccessUntil: tc.until}
			if got := u.EffectiveAccessLevel(now); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseAccessLevel(t *testing.T) {
	if got, err := ParseAccessLevel("full"); err != nil || got != AccessLevelFull {
		t.Fatalf("full: got %q err %v", got, err)
	}
	if got, err := ParseAccessLevel("readonly"); err != nil || got != AccessLevelReadonly {
		t.Fatalf("readonly: got %q err %v", got, err)
	}
	if _, err := ParseAccessLevel("pro"); err == nil {
		t.Fatal("expected an error for an unknown level")
	}
}

func TestSetAccessBumpsUpdatedAt(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	u := &User{AccessLevel: AccessLevelFull, UpdatedAt: now.Add(-time.Hour)}

	u.SetAccess(AccessLevelReadonly, nil, now)

	if u.AccessLevel != AccessLevelReadonly {
		t.Fatalf("level: got %q", u.AccessLevel)
	}
	if u.AccessUntil != nil {
		t.Fatalf("until: got %v want nil", u.AccessUntil)
	}
	if !u.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt: got %v want %v", u.UpdatedAt, now)
	}
}

func TestNewUserDefaultsToFullAccess(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	u := NewUser(vo.NewId(), "ident", "email", "Name", "face:sky", "hash", "salt", now)
	if u.AccessLevel != AccessLevelFull {
		t.Fatalf("level: got %q want full", u.AccessLevel)
	}
	if u.AccessUntil != nil {
		t.Fatalf("until: got %v want nil", u.AccessUntil)
	}
}
```

Add the `vo` import (`github.com/econumo/econumo/internal/shared/vo`) to the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run 'TestEffectiveAccessLevel|TestParseAccessLevel|TestSetAccess|TestNewUserDefaults' -v`
Expected: FAIL — `undefined: AccessLevel`.

- [ ] **Step 3: Write minimal implementation**

In `internal/model/user.go`, add two fields to `User` immediately after `IsActive bool`:

```go
	AccessLevel AccessLevel
	AccessUntil *time.Time
```

In `NewUser`, set the default beside `IsActive: true`:

```go
		AccessLevel: AccessLevelFull,
```

Append to the file:

```go
type AccessLevel string

const (
	AccessLevelFull     AccessLevel = "full"
	AccessLevelReadonly AccessLevel = "readonly"
)

func ParseAccessLevel(s string) (AccessLevel, error) {
	switch AccessLevel(s) {
	case AccessLevelFull:
		return AccessLevelFull, nil
	case AccessLevelReadonly:
		return AccessLevelReadonly, nil
	default:
		return "", fmt.Errorf("unknown access level %q (want full or readonly)", s)
	}
}

// EffectiveAccessLevel collapses the stored level and expiry against the clock.
// No job "expires" users: an elapsed access_until IS read-only, so no row can be
// left stale by a run that did not happen.
func (u *User) EffectiveAccessLevel(now time.Time) AccessLevel {
	if u.AccessUntil != nil && !now.Before(*u.AccessUntil) {
		return AccessLevelReadonly
	}
	return u.AccessLevel
}

func (u *User) SetAccess(level AccessLevel, until *time.Time, now time.Time) {
	u.AccessLevel = level
	u.AccessUntil = until
	u.UpdatedAt = now
}
```

Add `"fmt"` to the file's imports if absent.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/ -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/model/user.go internal/model/user_access_test.go
git add internal/model/user.go internal/model/user_access_test.go
git commit -m "feat: add access level and expiry to the user entity"
```

---

### Task 3: Persist the columns through the repo

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql`, `.../query/pgsql/users.sql`
- Modify: `internal/user/repo/repo.go` (`userRow` `:25-43`, `Save` `:152-182`, `hydrate` `:193-217`)
- Modify: `internal/test/fixture/entities.go:22-77`
- Test: `internal/user/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: Task 1 columns; Task 2 entity fields.
- Produces: `Repo.Save` persists both columns; `GetByID`/`GetByIdentifier` hydrate them. `fixture.User` gains `AccessLevel string` (empty seeds `'full'`) and `AccessUntil *time.Time`.

- [ ] **Step 1: Write the failing test**

Append to `internal/user/repo/repo_integration_test.go`. Note `newRepos` returns `(*userrepo.Repo, *userrepo.ReadRepo, *dbtest.DB)` and `newTestUser` builds `&model.User{...}` directly, bypassing `NewUser`:

```go
func TestUserRepo_AccessRoundTrip(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()

	until := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	u := newTestUser(t)
	u.SetAccess(model.AccessLevelReadonly, &until, u.UpdatedAt)

	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AccessLevel != model.AccessLevelReadonly {
		t.Fatalf("level: got %q want readonly", got.AccessLevel)
	}
	if got.AccessUntil == nil || !got.AccessUntil.Equal(until) {
		t.Fatalf("until: got %v want %v", got.AccessUntil, until)
	}
}

func TestUserRepo_AccessDefaultsToFullWithNoExpiry(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()

	u := newTestUser(t)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AccessLevel != model.AccessLevelFull {
		t.Fatalf("level: got %q want full", got.AccessLevel)
	}
	if got.AccessUntil != nil {
		t.Fatalf("until: got %v want nil", got.AccessUntil)
	}
}
```

Add `AccessLevel: model.AccessLevelFull` to the literal inside `newTestUser`, or the second test fails on an empty level rather than the missing column.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/repo/ -run TestUserRepo_Access -v`
Expected: FAIL — the round-trip returns an empty level.

- [ ] **Step 3: Update the queries and regenerate**

In `internal/infra/storage/sqlc/query/sqlite/users.sql`, add `access_level, access_until` to the SELECT column lists of `GetUserByID` and `GetUserByIdentifier`, and to both halves of `UpsertUser` (the INSERT column list and its `ON CONFLICT (id) DO UPDATE SET` assignments). Mirror in `internal/infra/storage/sqlc/query/pgsql/users.sql`, keeping that file's `$N` placeholders. ASCII only.

```bash
cd internal/infra/storage/sqlc && sqlc generate && cd -
```

- [ ] **Step 4: Wire the repo and fixture**

`internal/user/repo/repo.go` — add to `userRow`:

```go
		AccessLevel string
		AccessUntil *time.Time
```

in `Save`, add to the `userParams` literal:

```go
		AccessLevel: string(u.AccessLevel),
		AccessUntil: u.AccessUntil,
```

in `hydrate`, add to the `&model.User{...}` literal:

```go
		AccessLevel: model.AccessLevel(row.AccessLevel),
		AccessUntil: row.AccessUntil,
```

`internal/test/fixture/entities.go` — add to the `User` struct:

```go
	AccessLevel string     // default "full"
	AccessUntil *time.Time // default nil (no expiry)
```

and in `func (b *Builder) User(u User) string`, default the level then extend the insert (booleans stay literal in the query text per that package's contract; the new columns are ordinary bound parameters):

```go
	level := u.AccessLevel
	if level == "" {
		level = "full"
	}
	b.insert(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active, access_level, access_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'sha512', ?, ?, `+active+`, ?, ?)`,
		id, identifier, email, u.Name, u.Avatar, password, u.Salt, now, now, level, u.AccessUntil)
```

Add `"time"` to `entities.go` imports if absent.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/user/... ./internal/test/... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/user/repo/repo.go internal/test/fixture/entities.go internal/user/repo/repo_integration_test.go
git add internal/infra/storage/sqlc internal/user/repo internal/test/fixture/entities.go
git commit -m "feat: persist access level and expiry in the user repo"
```

---

### Task 4: Grant a trial at registration

**Files:**
- Create: `internal/model/trial.go`, `internal/model/trial_test.go`, `internal/user/trial_integration_test.go`
- Modify: `internal/config/config.go`; `internal/user/usecase.go:26-56`; `internal/user/register.go:15,45-79`; `internal/user/admin.go:18`; `internal/server/server.go`; `internal/cli/container.go:75-78`; `internal/user/api/harness_test.go:~120`; `internal/user/admin_integration_test.go:22-37`; `internal/user/authenticate_test.go:~40`

**Interfaces:**
- Consumes: `User.SetAccess` (Task 2).
- Produces:
  - `func model.TrialEnd(registeredAt time.Time) time.Time`
  - `config.Config.Trial string` — `"none"` (default) or `"end-of-next-month"`
  - `appuser.NewService(..., allowRegistration bool, trial string)` — `trial` appended as the **final** parameter

- [ ] **Step 1: Write the failing test**

Create `internal/model/trial_test.go`:

```go
package model

import (
	"testing"
	"time"
)

func TestTrialEnd(t *testing.T) {
	cases := []struct {
		name         string
		registeredAt time.Time
		want         time.Time
	}{
		{"first of the month", time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"second of the month", time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"last day of a 31-day month", time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"february", time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"across the year boundary", time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC), time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"november wraps to january", time.Date(2026, 11, 30, 12, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrialEnd(tc.registeredAt); !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTrialEndAlwaysSpansAFullCalendarMonth(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 400; i++ {
		day := start.AddDate(0, 0, i)
		end := TrialEnd(day)
		nextMonth := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		lastInstant := nextMonth.AddDate(0, 1, 0).Add(-time.Nanosecond)
		if end.Before(lastInstant) {
			t.Fatalf("registered %v: trial ends %v, before the full month ending %v", day, end, lastInstant)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestTrialEnd -v`
Expected: FAIL — `undefined: TrialEnd`.

- [ ] **Step 3: Write the implementation**

Create `internal/model/trial.go`:

```go
package model

import "time"

// TrialEnd returns the first instant of the month after the registration month.
// The product's moment of value is a closed calendar month (plan against
// actual), so the trial must span one whole month whatever day it starts on: a
// fixed day count delivers that to nobody registering early in a month. Taking
// the start of the following month rather than the last second of the previous
// one avoids end-of-day arithmetic and leaves timezone slack.
func TrialEnd(registeredAt time.Time) time.Time {
	utc := registeredAt.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 2, 0)
}
```

In `internal/config/config.go`, add `Trial string` to `Config` and parse it inside `Load` — strictly, so a typo cannot silently disable trials in production:

```go
	c.Trial = getEnv("ECONUMO_TRIAL", "none")
	if c.Trial != "none" && c.Trial != "end-of-next-month" {
		return Config{}, fmt.Errorf("ECONUMO_TRIAL: invalid value %q (want none or end-of-next-month)", c.Trial)
	}
```

In `internal/user/usecase.go`, add a `trial string` field to `Service` and a final `trial string` parameter to `NewService`, assigning it.

In `internal/user/register.go`, change `createUser` to take a `grantTrial bool` and set the expiry after the entity is built:

```go
func (s *Service) createUser(ctx context.Context, name, email, password string, grantTrial bool) (*model.User, error) {
	...
	u := model.NewUser(s.repo.NextIdentity(), identifier, encryptedEmail, name, avatar, passwordHash, salt, now)
	u.SeedDefaultOptions(s.repo.NextIdentity, now)
	if grantTrial && s.trial == "end-of-next-month" {
		until := model.TrialEnd(now)
		u.SetAccess(model.AccessLevelFull, &until, now)
	}
	...
}
```

Pass `true` from `Register` (`register.go:15`) and `false` from `AdminCreateUser` (`admin.go:18`): an operator-created account is a deliberate grant, not a trial.

Update every construction site to pass the new argument:
- `internal/server/server.go` — pass `cfg.Trial`
- `internal/cli/container.go:75-78` — pass `cfg.Trial`
- `internal/user/api/harness_test.go` — pass `""`
- `internal/user/admin_integration_test.go` `newUserSvc` — pass `""`
- `internal/user/authenticate_test.go` `newAuthEnvFull` — pass `""`

- [ ] **Step 4: Write the registration test**

Create `internal/user/trial_integration_test.go`:

```go
package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// trialNow is fixed so the assertion cannot straddle a month boundary between
// the service's clock and the test's own time.Now().
var trialNow = time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)

type trialClock struct{}

func (trialClock) Now() time.Time { return trialNow }

func newTrialSvc(t *testing.T, db *dbtest.DB, trial string) (*appuser.Service, *userrepo.Repo, *auth.EncodeService) {
	t.Helper()
	enc := auth.NewEncodeService(testSalt)
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, nil, nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), trialClock{}, nil, true, trial)
	return svc, repo, enc
}

func TestRegister_GrantsTrialWhenEnabled(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, enc := newTrialSvc(t, db, "end-of-next-month")
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Trial User", Email: "trial@econumo.test", Password: "secretpass",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	u, err := repo.GetByIdentifier(ctx, enc.Hash("trial@econumo.test"))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if u.AccessLevel != model.AccessLevelFull {
		t.Fatalf("level: got %q want full", u.AccessLevel)
	}
	if u.AccessUntil == nil {
		t.Fatal("access_until: got nil, want the start of the month after next")
	}
	want := model.TrialEnd(trialNow) // 2026-09-01 00:00:00 UTC
	if !u.AccessUntil.Equal(want) {
		t.Fatalf("access_until: got %v want %v", *u.AccessUntil, want)
	}
}

func TestRegister_NoTrialByDefault(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, enc := newTrialSvc(t, db, "none")
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Plain User", Email: "plain@econumo.test", Password: "secretpass",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	u, err := repo.GetByIdentifier(ctx, enc.Hash("plain@econumo.test"))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if u.AccessUntil != nil {
		t.Fatalf("access_until: got %v want nil", *u.AccessUntil)
	}
}

func TestAdminCreateUser_NeverGrantsTrial(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, enc := newTrialSvc(t, db, "end-of-next-month")
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Ops User", "ops@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	u, err := repo.GetByIdentifier(ctx, enc.Hash("ops@econumo.test"))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if u.AccessUntil != nil {
		t.Fatalf("access_until: got %v want nil (operator grants are not trials)", *u.AccessUntil)
	}
}
```

Confirm `model.RegisterRequest`'s exact field names before running; adjust if they differ.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/model/ ./internal/user/... ./internal/config/ 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/model internal/config internal/user internal/server internal/cli
git add -A internal/model internal/config internal/user internal/server internal/cli
git commit -m "feat: grant a trial until the end of the next calendar month"
```

---

### Task 5: Return the effective level from Authenticate

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/access_tokens.sql`
- Modify: `internal/user/repo/accesstoken.go`, `accesstoken_sqlite.go`, `accesstoken_pgsql.go`
- Modify: `internal/user/authenticate.go:12-31`
- Modify: `internal/web/middleware/auth.go:19-21,45-68`
- Modify: `internal/test/authstub/authstub.go`
- Modify: `internal/web/middleware/middleware_test.go:278-287` (`stubAuthn`)
- Test: `internal/user/authenticate_test.go`

**Interfaces:**
- Consumes: Task 2 entity, Task 3 persistence.
- Produces: `Authenticate(ctx, token string) (vo.Id, vo.Id, model.AccessLevel, error)` on `user.Service`, `middleware.TokenAuthenticator` and `authstub.Authenticator`.

**Why a join:** `GetAccessTokenByHash` reads `access_tokens` only today, and `internal/user/admin.go:90-93` records that as deliberate — per-request auth needs no `is_active` join because deactivation revokes tokens. Access level cannot reuse that trick: an expired trial must **not** revoke sessions, or a lapsed user could not log in to pay. So the query grows a join. It stays one round trip.

- [ ] **Step 1: Write the failing test**

Append to `internal/user/authenticate_test.go` (`newAuthEnv` returns `(svc, tokens, clk, uid)`; `seedToken(t, tokens, userID, kind, raw, exp)`; `authT0` is the fixed clock base):

```go
func TestAuthenticate_ReturnsReadonlyWhenAccessLapsed(t *testing.T) {
	db := dbtest.New(t)
	svc, tokens, _, uid := newAuthEnvOn(t, db)
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_lapsed", &exp)

	past := authT0.Add(-24 * time.Hour)
	if _, err := db.Raw.ExecContext(context.Background(),
		db.Rebind("UPDATE users SET access_until = ? WHERE id = ?"),
		past.Format(datetime.Layout), uid.String()); err != nil {
		t.Fatalf("lapse access: %v", err)
	}

	_, _, level, err := svc.Authenticate(context.Background(), "eco_ses_lapsed")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if level != model.AccessLevelReadonly {
		t.Fatalf("level: got %q want readonly", level)
	}
}

func TestAuthenticate_ReturnsFullForUnexpiredAccess(t *testing.T) {
	db := dbtest.New(t)
	svc, tokens, _, uid := newAuthEnvOn(t, db)
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_live", &exp)

	_, _, level, err := svc.Authenticate(context.Background(), "eco_ses_live")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if level != model.AccessLevelFull {
		t.Fatalf("level: got %q want full", level)
	}
}
```

`newAuthEnvOn(t, db)` does not exist yet — refactor `newAuthEnvFull` so the `*dbtest.DB` it creates is injectable, and add:

```go
func newAuthEnvOn(t *testing.T, db *dbtest.DB) (*appuser.Service, *userrepo.AccessTokenRepo, *testClock, vo.Id) {
	// same body as newAuthEnvFull, but using the passed db instead of dbtest.New(t)
}
```

Keep `newAuthEnv` and `newAuthEnvFull` working — the existing tests in that file call them.

Add `dbtest` and `datetime` imports to the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/ -run TestAuthenticate_Returns -v`
Expected: FAIL — `Authenticate` returns three values, not four.

- [ ] **Step 3: Join the query and regenerate**

`internal/infra/storage/sqlc/query/sqlite/access_tokens.sql`:

```sql
-- name: GetAccessTokenByHash :one
SELECT t.id, t.user_id, t.kind, t.token_hash, t.name, t.user_agent,
       t.created_at, t.last_used_at, t.expires_at, t.revoked_at,
       u.access_level, u.access_until
FROM access_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = ?;
```

Mirror in the pgsql file with `$1`. Regenerate with `sqlc generate`.

- [ ] **Step 4: Thread the level through**

Carry the two new columns out of the token repo alongside the token — extend the repo's `GetByHash` to return them (a small struct, or a second return value). In `internal/user/authenticate.go`, compute the level from those columns via a `model.User` value and return `u.EffectiveAccessLevel(now)` as the third value; every existing early return now yields `""` for the level beside its error.

`internal/web/middleware/auth.go`:

```go
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, token string) (userID vo.Id, tokenID vo.Id, level model.AccessLevel, err error)
}
```

and update the call site at `:54` to receive four values.

`internal/test/authstub/authstub.go` — give the stub a settable level defaulting to full:

```go
type Authenticator struct {
	Level model.AccessLevel
}

func (a Authenticator) Authenticate(_ context.Context, token string) (vo.Id, vo.Id, model.AccessLevel, error) {
	id, err := vo.ParseId(token)
	if err != nil {
		return vo.Id{}, vo.Id{}, "", errs.NewUnauthorized("Invalid access token")
	}
	level := a.Level
	if level == "" {
		level = model.AccessLevelFull
	}
	return id, id, level, nil
}
```

The zero value stays valid, so the ten feature harnesses constructing `authstub.Authenticator{}` keep compiling unchanged.

`internal/web/middleware/middleware_test.go` — add a `level model.AccessLevel` field to `stubAuthn` and return it, defaulting `""` to full the same way.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./internal/... 2>&1 | tail -30`
Expected: PASS — compilation across every feature's api tests confirms the stub updates are complete.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/user internal/web/middleware internal/test/authstub
git add -A internal/infra/storage/sqlc internal/user internal/web/middleware internal/test/authstub
git commit -m "feat: return the caller's effective access level from Authenticate"
```

---

### Task 6: The 402 error type

**Files:**
- Modify: `internal/shared/errs/errs.go`
- Modify: `internal/web/httpx/errors.go:20-52`
- Test: `internal/web/httpx/errors_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `errs.NewPaymentRequired(msg string) *errs.PaymentRequiredError`, `errs.AsPaymentRequired(err error) (*PaymentRequiredError, bool)`, mapped to HTTP 402.

- [ ] **Step 1: Write the failing test**

Append to `internal/web/httpx/errors_test.go` (package `httpx`; `decodeEnvelope` already exists in that file):

```go
func TestWriteError_PaymentRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errs.NewPaymentRequired("Read-only access. Write operations are disabled."), false)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("HTTP status = %d, want 402", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	want := `{"success":false,"message":"Read-only access. Write operations are disabled.","code":402,"errors":{}}`
	if body != want {
		t.Fatalf("body:\n got %s\nwant %s", body, want)
	}
	if env := decodeEnvelope(t, rec); env.Success {
		t.Fatalf("success = true, want false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web/httpx/ -run TestWriteError_PaymentRequired -v`
Expected: FAIL — `undefined: errs.NewPaymentRequired`.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/shared/errs/errs.go`, mirroring the `UnauthorizedError` shape:

```go
// PaymentRequiredError maps to HTTP 402: the caller is authenticated but their
// access is read-only. 402 rather than 403 lets a client tell this apart from
// validation and auth failures with a single status comparison.
type PaymentRequiredError struct {
	Msg string
}

func (e *PaymentRequiredError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "payment required"
}

func NewPaymentRequired(msg string) *PaymentRequiredError { return &PaymentRequiredError{Msg: msg} }

func AsPaymentRequired(err error) (*PaymentRequiredError, bool) {
	var v *PaymentRequiredError
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}
```

In `internal/web/httpx/errors.go`, add a branch to the if-chain after `AsUnauthorized` and before `AsTooManyRequests`:

```go
	if v, ok := errs.AsPaymentRequired(err); ok {
		errCoded(w, v.Error(), http.StatusPaymentRequired, nil, nil, "", nil, http.StatusPaymentRequired)
		return
	}
```

Passing `402` as the envelope `code` (not `0`) matches the 429 branch and the frozen shape.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/web/httpx/ -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/shared/errs/errs.go internal/web/httpx/errors.go internal/web/httpx/errors_test.go
git add internal/shared/errs internal/web/httpx
git commit -m "feat: add a 402 payment-required error type"
```

---

### Task 7: Enforce read-only in the auth middleware

**Files:**
- Modify: `internal/web/middleware/auth.go:45-68`
- Test: `internal/web/middleware/middleware_test.go` (append; there is no `auth_test.go`)

**Interfaces:**
- Consumes: `model.AccessLevelReadonly` (Task 2), four-value `Authenticate` (Task 5), `errs.NewPaymentRequired` (Task 6).
- Produces: no new exported symbols. Behavior: a `POST` from a read-only caller to a non-allowlisted path yields 402 and the handler does not run.

- [ ] **Step 1: Write the failing test**

Append to `internal/web/middleware/middleware_test.go`. `stubAuthn` (`:278-287`) gains a `level` field in Task 5; `authTestUserID` and `authTestTokenID` already exist at `:290-293`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/middleware/ -run TestAuth_Readonly -v`
Expected: FAIL — writes are not blocked (200, handler ran).

- [ ] **Step 3: Write minimal implementation**

In `internal/web/middleware/auth.go`, add the allowlist at package level:

```go
// readonlyAllowedPaths are the POST endpoints a restricted caller may still
// reach. The principle: a restricted user may always secure their account and
// leave it, but may not add data. update-password is a security operation, so
// locking someone out of rotating a compromised password would be indefensible;
// create-personal-token is excluded because it mints new write-capable
// credentials. Account deletion joins this list when it exists.
var readonlyAllowedPaths = map[string]bool{
	"/api/v1/user/logout-user":           true,
	"/api/v1/user/revoke-session":        true,
	"/api/v1/user/revoke-other-sessions": true,
	"/api/v1/user/revoke-personal-token": true,
	"/api/v1/user/update-password":       true,
}
```

and inside `Auth`, after `Authenticate` succeeds and before the handler runs:

```go
			if level == model.AccessLevelReadonly && r.Method == http.MethodPost && !readonlyAllowedPaths[r.URL.Path] {
				httpx.WriteError(w, errs.NewPaymentRequired("Read-only access. Write operations are disabled."), dev)
				return
			}
```

One rule covers every write because the API convention is frozen: GET reads, POST writes, no PUT/PATCH/DELETE. CSV export is a GET and keeps working; CSV import is a POST and is correctly blocked.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/web/middleware/ -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/web/middleware
git add internal/web/middleware
git commit -m "feat: reject writes from read-only callers with 402"
```

---

### Task 8: Expose access state on the wire

**Files:**
- Modify: `internal/model/user_dto.go:27-35`, `internal/model/connection_dto.go:20-24`
- Modify: the use cases building those DTOs — locate with `grep -rn 'CurrentUserResult{' 'ConnectionResult{' internal/`
- Test: `internal/user/api/user_endpoints_test.go`, `internal/connection/api/connection_endpoints_test.go`, regenerated goldens

**Interfaces:**
- Consumes: `User.EffectiveAccessLevel` (Task 2), persistence (Task 3).
- Produces: JSON keys `accessLevel` (string) and `accessUntil` (string, `""` when NULL) on `CurrentUserResult` and `ConnectionResult`.

- [ ] **Step 1: Write the failing test**

Append to `internal/user/api/user_endpoints_test.go`. Add `AccessLevel`/`AccessUntil` string fields to the `currentUser` struct in `internal/user/api/harness_test.go:261-272` first:

```go
func TestGetUserData_CarriesAccessState(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	_, env := h.do(t, http.MethodGet, "/api/v1/user/get-user-data", token, nil)
	wrapper := mustUnmarshal[struct {
		User currentUser `json:"user"`
	}](t, env.Data)

	if wrapper.User.AccessLevel != "full" {
		t.Fatalf("accessLevel = %q, want full", wrapper.User.AccessLevel)
	}
	if wrapper.User.AccessUntil != "" {
		t.Fatalf("accessUntil = %q, want empty for a user with no expiry", wrapper.User.AccessUntil)
	}
}
```

Append to `internal/connection/api/connection_endpoints_test.go`. `listResult` is already declared there; add `AccessLevel`/`AccessUntil` to its item struct:

```go
func TestGetConnectionList_CarriesPartnerAccessState(t *testing.T) {
	h := newHarness(t)
	if _, err := h.db.Exec("UPDATE users SET access_until = ? WHERE id = ?",
		"2020-01-01 00:00:00", guestUserID); err != nil {
		t.Fatalf("lapse guest access: %v", err)
	}

	ownerTok := h.token(t, ownerUserID, ownerEmail)
	_, env := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", ownerTok, nil)
	owl := mustUnmarshal[listResult](t, env.Data)

	if len(owl.Items) != 1 || owl.Items[0].User.ID != guestUserID {
		t.Fatalf("owner list = %+v, want one connection to guest", owl.Items)
	}
	if owl.Items[0].AccessLevel != "readonly" {
		t.Fatalf("partner accessLevel = %q, want readonly", owl.Items[0].AccessLevel)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/api/ ./internal/connection/api/ -run 'AccessState' -v`
Expected: FAIL — the fields do not exist.

- [ ] **Step 3: Add the fields**

`internal/model/user_dto.go`, on `CurrentUserResult`:

```go
	AccessLevel  string `json:"accessLevel"`
	AccessUntil  string `json:"accessUntil"`
```

`internal/model/connection_dto.go`, on `ConnectionResult`:

```go
	AccessLevel    string `json:"accessLevel"`
	AccessUntil    string `json:"accessUntil"`
```

`ConnectionResult` carries these rather than the shared `UserResult` embed on purpose: `UserResult` rides along on every transaction author (`internal/model/transaction_dto.go:20`, `internal/model/account_dto.go:129`), and transaction lists are the heaviest responses in the product.

Populate both from `EffectiveAccessLevel(now)`, formatting `AccessUntil` with `datetime.Layout` and emitting `""` when nil. The connection read path must load each connected user's access columns — check whether its query already selects from `users` and extend it if not.

- [ ] **Step 4: Run tests, then regenerate goldens**

```bash
go test ./internal/user/... ./internal/connection/... 2>&1 | tail -20
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
git diff --stat internal/test/apiparity/testdata/golden/
git diff internal/test/apiparity/testdata/golden/ | head -60
```

**Inspect the diff before staging.** Every changed golden should show only the added keys. Anything else means behavior changed unintentionally — stop and investigate rather than accepting.

- [ ] **Step 5: Run the full suite**

Run: `make go-test`
Expected: PASS, coverage at or above `GO_COVER_MIN`.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/model internal/user internal/connection
git add -A internal/model internal/user internal/connection internal/test/apiparity
git commit -m "feat: expose access level and expiry on user and connection payloads"
```

---

### Task 9: CLI — user:set-access and user:show

**Files:**
- Modify: `internal/user/admin.go` (two use-case methods)
- Modify: `internal/cli/user_commands.go:11` (register two commands)
- Test: `internal/user/admin_integration_test.go` (service behavior), `internal/cli/commands_test.go` (exit codes)

**Interfaces:**
- Consumes: `model.ParseAccessLevel`, `User.SetAccess`, `User.EffectiveAccessLevel` (Task 2); persistence (Task 3).
- Produces:
  - `func (s *Service) AdminSetAccess(ctx context.Context, email string, level model.AccessLevel, until *time.Time) error`
  - `func (s *Service) AdminShowUser(ctx context.Context, email string) (*model.User, model.AccessLevel, error)`

**Test split rationale:** the CLI harness asserts only exit codes and has no stdout capture, so `user:show`'s rendering is verified through `AdminShowUser` at the service level, and the CLI tests cover dispatch, arity and argument parsing.

- [ ] **Step 1: Write the failing service test**

Append to `internal/user/admin_integration_test.go` (`newUserSvc`, `testSalt`, `isNotFound` already exist there):

```go
func TestAdminSetAccessAndShow(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Access User", "access@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := svc.AdminSetAccess(ctx, "access@econumo.test", model.AccessLevelFull, &until); err != nil {
		t.Fatalf("AdminSetAccess: %v", err)
	}

	u, effective, err := svc.AdminShowUser(ctx, "access@econumo.test")
	if err != nil {
		t.Fatalf("AdminShowUser: %v", err)
	}
	if u.AccessLevel != model.AccessLevelFull {
		t.Fatalf("level: got %q want full", u.AccessLevel)
	}
	if u.AccessUntil == nil || !u.AccessUntil.Equal(until) {
		t.Fatalf("until: got %v want %v", u.AccessUntil, until)
	}
	if effective != model.AccessLevelFull {
		t.Fatalf("effective: got %q want full (expiry is in the future)", effective)
	}

	if err := svc.AdminSetAccess(ctx, "access@econumo.test", model.AccessLevelFull, nil); err != nil {
		t.Fatalf("AdminSetAccess clearing expiry: %v", err)
	}
	u2, _, err := svc.AdminShowUser(ctx, "access@econumo.test")
	if err != nil {
		t.Fatalf("AdminShowUser after clear: %v", err)
	}
	if u2.AccessUntil != nil {
		t.Fatalf("until after clear: got %v want nil", u2.AccessUntil)
	}
}

func TestAdminShowUser_EffectiveDiffersOnceExpired(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Lapsed", "lapsed@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := svc.AdminSetAccess(ctx, "lapsed@econumo.test", model.AccessLevelFull, &past); err != nil {
		t.Fatalf("AdminSetAccess: %v", err)
	}

	u, effective, err := svc.AdminShowUser(ctx, "lapsed@econumo.test")
	if err != nil {
		t.Fatalf("AdminShowUser: %v", err)
	}
	if u.AccessLevel != model.AccessLevelFull {
		t.Fatalf("raw level: got %q want full", u.AccessLevel)
	}
	if effective != model.AccessLevelReadonly {
		t.Fatalf("effective: got %q want readonly", effective)
	}
}

func TestAdminSetAccess_UnknownEmail(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	err := svc.AdminSetAccess(context.Background(), "nobody@econumo.test", model.AccessLevelReadonly, nil)
	if !isNotFound(err) {
		t.Fatalf("err = %v, want NotFound", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/ -run 'TestAdminSetAccess|TestAdminShowUser' -v`
Expected: FAIL — `svc.AdminSetAccess undefined`.

- [ ] **Step 3: Write the use cases**

Append to `internal/user/admin.go`, following `AdminActivate` (`:78-89`) exactly — `userByEmail` resolves the identifier, mutate, save inside `s.tx.WithTx`:

```go
func (s *Service) AdminSetAccess(ctx context.Context, email string, level model.AccessLevel, until *time.Time) error {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.SetAccess(level, until, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}

func (s *Service) AdminShowUser(ctx context.Context, email string) (*model.User, model.AccessLevel, error) {
	u, err := s.userByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}
	return u, u.EffectiveAccessLevel(s.clock.Now()), nil
}
```

- [ ] **Step 4: Register the commands**

Add to the slice returned by `userCommands()` in `internal/cli/user_commands.go`:

```go
		{
			name:    "user:set-access",
			summary: "Set a user's access: user:set-access <email> <full|readonly> [YYYY-MM-DD]",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) < 2 || len(args) > 3 {
					return usageErr("user:set-access <email> <full|readonly> [YYYY-MM-DD]")
				}
				email := strings.TrimSpace(args[0])
				level, err := model.ParseAccessLevel(strings.TrimSpace(args[1]))
				if err != nil {
					return err
				}
				var until *time.Time
				if len(args) == 3 && strings.TrimSpace(args[2]) != "" {
					d, err := time.Parse(datetime.DateLayout, strings.TrimSpace(args[2]))
					if err != nil {
						return fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", args[2], err)
					}
					until = &d
				}
				if err := c.user.AdminSetAccess(ctx, email, level, until); err != nil {
					return err
				}
				if until == nil {
					fmt.Printf("Access for %s set to %s with no expiry\n", email, level)
				} else {
					fmt.Printf("Access for %s set to %s until %s\n", email, level, until.Format(datetime.DateLayout))
				}
				return nil
			},
		},
		{
			name:    "user:show",
			summary: "Show a user's profile and access: user:show <email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 1 {
					return usageErr("user:show <email>")
				}
				u, effective, err := c.user.AdminShowUser(ctx, strings.TrimSpace(args[0]))
				if err != nil {
					return err
				}
				active := "no"
				if u.IsActive {
					active = "yes"
				}
				until := ""
				if u.AccessUntil != nil {
					until = u.AccessUntil.Format(datetime.Layout)
				}
				fmt.Printf("Id:              %s\n", u.ID.String())
				fmt.Printf("Name:            %s\n", u.Name)
				fmt.Printf("Email:           %s\n", u.Email)
				fmt.Printf("Active:          %s\n", active)
				fmt.Printf("Access level:    %s\n", u.AccessLevel)
				fmt.Printf("Access until:    %s\n", until)
				fmt.Printf("Effective:       %s\n", effective)
				return nil
			},
		},
```

Add `"fmt"`, `"time"`, `"github.com/econumo/econumo/internal/model"` and `"github.com/econumo/econumo/internal/shared/datetime"` to that file's imports.

`Effective` prints beside the raw column deliberately: the model turns on `(access_level, access_until, now)` collapsing to one value, and an operator debugging "why can this user not write" needs the answer, not the inputs.

`u.Email` is plaintext on the API/CLI path — the container builds `EncodeService` with an empty salt, so no decode is needed.

- [ ] **Step 5: Write the CLI exit-code tests**

Append to `internal/cli/commands_test.go`, matching the existing style (`cliEnv(t)` then assert `Run(...)`):

```go
func TestUserSetAccessAndShowExitCodes(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"user:create", "Access User", "access@example.test", "secret-pw"}); got != 0 {
		t.Fatalf("user:create = %d, want 0", got)
	}

	steps := []struct {
		name string
		args []string
		want int
	}{
		{"set-readonly", []string{"user:set-access", "access@example.test", "readonly"}, 0},
		{"set-full-no-date", []string{"user:set-access", "access@example.test", "full"}, 0},
		{"set-full-with-date", []string{"user:set-access", "access@example.test", "full", "2027-01-01"}, 0},
		{"show", []string{"user:show", "access@example.test"}, 0},
		{"unknown-level", []string{"user:set-access", "access@example.test", "pro"}, 1},
		{"bad-date", []string{"user:set-access", "access@example.test", "full", "01-01-2027"}, 1},
		{"unknown-email", []string{"user:set-access", "nobody@example.test", "full"}, 1},
		{"show-unknown-email", []string{"user:show", "nobody@example.test"}, 1},
		{"set-access-too-few-args", []string{"user:set-access", "access@example.test"}, 1},
		{"show-too-many-args", []string{"user:show", "a@example.test", "b"}, 1},
	}
	for _, s := range steps {
		if got := Run(s.args); got != s.want {
			t.Fatalf("%s: Run(%v) = %d, want %d", s.name, s.args, got, s.want)
		}
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/user/ ./internal/cli/ -v 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/cli/user_commands.go internal/cli/commands_test.go internal/user/admin.go internal/user/admin_integration_test.go
git add internal/cli internal/user
git commit -m "feat: add user:set-access and user:show CLI commands"
```

---

### Task 10: Document and verify end to end

**Files:**
- Modify: `.env.example`, `CLAUDE.md`, `README.md` (if it lists env vars)

**Interfaces:**
- Consumes: everything above.
- Produces: no code.

- [ ] **Step 1: Add the env var**

In `.env.example`, beside the other `ECONUMO_*` entries:

```
# Trial granted at registration: none (default) | end-of-next-month
ECONUMO_TRIAL=none
```

- [ ] **Step 2: Update CLAUDE.md**

Add `ECONUMO_TRIAL` to the configuration bullet list; add `user:set-access <email> <full|readonly> [YYYY-MM-DD]` and `user:show <email>` to the CLI command block; note in the API-conventions section that a read-only caller receives 402 on non-allowlisted POSTs.

- [ ] **Step 3: Verify the whole suite**

Run: `make go-test`
Expected: PASS, coverage at or above `GO_COVER_MIN`.

- [ ] **Step 4: Verify by hand**

```bash
rm -f /tmp/trial.sqlite
DATABASE_URL="sqlite:///tmp/trial.sqlite" PORT=8181 ECONUMO_TRIAL=end-of-next-month \
  ECONUMO_ALLOW_REGISTRATION=true go run ./cmd/econumo serve &
sleep 4
curl -s -X POST localhost:8181/api/v1/user/register-user \
  -H 'Content-Type: application/json' \
  -d '{"name":"Trial","email":"trial@example.test","password":"secret123"}' | head -c 500
```

Expected: the created user carries a non-empty `accessUntil` on the first of the month after next.

```bash
DATABASE_URL="sqlite:///tmp/trial.sqlite" go run ./cmd/econumo user:show trial@example.test
DATABASE_URL="sqlite:///tmp/trial.sqlite" go run ./cmd/econumo user:set-access trial@example.test readonly
TOKEN=$(curl -s -X POST localhost:8181/api/v1/user/login-user -H 'Content-Type: application/json' \
  -d '{"username":"trial@example.test","password":"secret123"}' | grep -o '"token":"[^"]*' | cut -d'"' -f4)
echo "write:"; curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8181/api/v1/category/create-category \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"name":"Blocked"}'
echo "read:"; curl -s -o /dev/null -w '%{http_code}\n' localhost:8181/api/v1/account/get-account-list \
  -H "Authorization: Bearer $TOKEN"
```

Expected: `402` for the write, `200` for the read. Kill the server and delete `/tmp/trial.sqlite` afterwards.

- [ ] **Step 5: Commit**

```bash
git add .env.example CLAUDE.md README.md
git commit -m "docs: document ECONUMO_TRIAL and the access CLI commands"
```

---

## What this plan does NOT cover

Deliberately deferred so each plan stays reviewable:

- **Plan B:** the admin listener (`ECONUMO_ADMIN_PORT` / `ECONUMO_ADMIN_TOKEN`), `POST /admin/set-access`, `GET /admin/expiring-users`, `GET /admin/user-context`, HMAC handoff-token minting and verification, and `POST /api/v1/user/create-billing-link`.
- **Plan C:** the SPA banner, settings entry, per-connection CTA, 402 client handling, FAB hiding, `PAYWALL_ENABLED` / `paywallUrl` cleanup, `BILLING_URL` config, the eight `METRICS` keys, and the `accessState` super property.

After Plan A the product grants trials, restricts lapsed users, and is fully operable by CLI. Nothing is user-visible in the SPA yet, which is intentional: cloud runs with `ECONUMO_TRIAL=none` until Plan C lands.
