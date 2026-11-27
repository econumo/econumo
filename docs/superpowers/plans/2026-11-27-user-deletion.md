# User Deletion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user can be marked for deletion (Settings, inactivity, CLI) and later wiped: sharing unwound, partner balances preserved by corrections, the `users` row deleted with the database cascade, and an audit row left behind.

**Architecture:** The `user` feature owns the state (three columns on `users`, `users_audit`, `users_deletion_requests`) and the use cases (mark, cancel, activity, re-grant, wipe, the daily job). Everything the wipe does to other features' data is ONE narrow method on the feature that owns the SQL, and `internal/server/glue_user_wipe.go` sequences those methods behind the `user.DataWiper` port, so features still never import features. The CLI and the tests get the same fully wired user service as the server through a new `server.BuildWithServices`.

**Tech Stack:** Go 1.27 (stdlib `net/http`, sqlc with one generated package per engine, SQLite via modernc, PostgreSQL via pgx stdlib), React 19 + TypeScript + TanStack Query + vitest in `web/`.

**Spec:** `docs/superpowers/specs/2026-11-26-user-deletion-design.md`. Read it before any task: the plan argues from it and does not repeat its reasoning. Step numbers below ("wipe step 6") are the spec's.

## Global Constraints

- **Go toolchain:** `go` is not on PATH. Start every shell with `export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH GOTOOLCHAIN=go1.27.1`.
- **Tests you run locally:** only the package you changed (`go test ./internal/<pkg>/... -run <Name> -count=1`), `go build ./...`, `go vet ./internal/<pkg>/...`, `gofmt -l`. Never `make test`, `make go-test`, the full `go test ./...`, or the whole vitest suite: CI runs those after the push.
- **PostgreSQL tests** (tag `enginecompare`): start a throwaway server once, `docker run -d --rm --name econumo-wipe-pg -e POSTGRES_USER=econumo -e POSTGRES_PASSWORD=econumo -e POSTGRES_DB=econumo_test -p 127.0.0.1:55532:5432 postgres:17-alpine`, then run with `DBTEST_ENGINE=pgsql DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@127.0.0.1:55532/econumo_test?sslmode=disable' go test -tags enginecompare -count=1 ./internal/<pkg>/ -run <Name>`. Stop it with `docker rm -f econumo-wipe-pg` when the task is done.
- **sqlc:** edit BOTH `internal/infra/storage/sqlc/query/sqlite/<file>.sql` and `.../pgsql/<file>.sql`, then `(cd internal/infra/storage/sqlc && sqlc generate)`. Query files are ASCII only, comments included. Never put `;` on a line of its own. Commit the regenerated `gen/` files.
- **Engine-adapter pattern:** a repo method is written once against a `querier` interface in the sqlite types; `sqlite.go` passes through, `pgsql.go` converts. Reference: `internal/tag/repo`. `internal/user/repo` uses a hand-written `userRow` converted positionally: do NOT add columns to `GetUserByID`/`userRow`/`UpsertUser`.
- **Migrations:** the one new version is `20261127000000`, present in both `sqlite/` and `pgsql/` (the newest existing one is `20261116000000`).
- **The three new `users` columns** (`deletion_marked_at`, `deletion_reason`, `last_active_at`) are read and written only by the dedicated statements of Task 4. They never join `UpsertUser`, `model.User` or `userRow`.
- **Goldens:** regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` (or `mcpparity`), then read the diff. Never hand-edit a golden.
- **Comments:** only the non-obvious why. No comments restating a name, no section dividers. Keep swag `// @...` blocks on API handlers and run `make swagger` when a handler or a DTO changes.
- **Logging:** `log/slog`, static message, ids only. Never an email, a request body or the text of a constraint error (`err_type` = `fmt.Sprintf("%T", err)`).
- **i18n:** every key goes into all 11 catalogues `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (`en` is the reference; this plan gives the `en` copy). Keep each catalogue's existing register: German Sie, Dutch u, French vous, European Portuguese. Placeholders are `{name}`. Read 10 neighbouring lines of a catalogue before translating into it.
- **Wire contract:** datetimes are `"2006-01-02 15:04:05"` UTC strings (`datetime.Layout`), an absent one is `""`. New JSON fields are additive.
- **Never write a shell heredoc containing a dollar-brace template** (the Bash tool empties it): create and edit code with the Write and Edit tools.
- **Commits:** one per task, on the current branch, staged by path (never `git add -A`). Message `feat(user-deletion): <task title>` ending with the line `Co-Authored-By: Claude <noreply@anthropic.com>`. Do not push; the coordinator pushes.
- **Stop rule:** if a signature this plan pins does not exist, a test you were told to write cannot be made to pass without changing another task's interface, or the lock order of Task 14 deadlocks, STOP and report what you found. Do not invent a different design.

## Review Focus

Inputs the spec implies but no step exercises by default; each has its test in the task named.

1. A user with no data and no partners is wiped cleanly (Task 14, `TestWipe_EmptyUser`).
2. The user changed their email after being marked: the audit hash is of the address at wipe time (Task 14, `TestWipe_HashIsOfTheCurrentEmail`).
3. A soft-deleted account on either side of a cross-owner transfer still gets its correction, and a deleted partner account still sums to zero (Task 10, `TestRewrite_SoftDeletedAccounts`).
4. One user's failure does not stop the pass: a mailer that fails for the first inactive user still lets the second be marked (Task 17, `TestMarkInactive_MailerFailureIsIsolated`).
5. A wrong code, an expired code and a sixth attempt are all refused without marking (Task 20, `TestConfirmUserDeletion_Refusals`).

## File Structure

| Path | Responsibility |
|---|---|
| `internal/infra/storage/migrations/{sqlite,pgsql}/20261127000000.sql` | columns, `users_audit`, `users_deletion_requests`, backfill |
| `internal/model/user_deletion.go`, `user_deletion_dto.go` | reasons, audit events, state, DTOs |
| `internal/user/repo/deletion*.go`, `audit*.go`, `deletionrequest*.go` | the three stores, engine adapters |
| `internal/user/deletion_mark.go` | mark, cancel-on-session, activity stamp |
| `internal/user/deletion_request.go` | self-service request and confirm |
| `internal/user/deletion_wipe.go` | the wipe use case |
| `internal/user/deletion_regrant.go` | lifetime re-grant |
| `internal/user/deletion_pass.go`, `deletion_job.go` | release / wipe / mark passes, the daily job |
| `internal/server/glue_user_wipe.go` | `UserDataWiper`, the cross-feature sequence |
| `internal/server/services.go` | `BuildWithServices` |
| `internal/{budget,debt,account,transaction,currency,authserver,imports}/wipe.go` | one primitive each |
| `internal/test/wipeguard/` | schema guard, both engines |
| `internal/test/wipetest/` | wipe fixture test, lock-order test |
| `web/src/features/settings/DeleteDataPage.tsx` | the Settings flow |
| `web/src/features/inbox/DeletionNoticeSection.tsx` | the partner row |

---

## Phase 0: prerequisite fixes

### Task 1: delete-connection leaves third-party budgets alone

**Files:**
- Modify: `internal/server/glue_connection.go` (`revokeDirection`)
- Test: `internal/server/glue_connection_test.go`
- Modify: `docs/regression-test-plan.md` (section 11)

**Interfaces:**
- Produces: nothing new. `ConnectionBudgetRevoker.RevokeBetween(ctx, a, b vo.Id) error` keeps its signature.

- [x] **Step 1: Write the failing test** `TestConnectionBudgetRevoker_LeavesThirdPartyBudgets` next to `TestConnectionBudgetRevoker_RevokeBetween`, reusing its service wiring. Fixture: users U, P1, P2; one budget OWNED BY P2; accepted `BudgetRoleUser` access rows for U and P1; one category of P1 with an element in that budget. Call `revoker.RevokeBetween(ctx, U, P1)` and then `RevokeBetween(ctx, P1, U)` on a fresh fixture (both argument orders, as subtests).

```go
if got := len(list); got != 2 { t.Errorf("access rows on P2's budget = %d, want 2", got) }
if got := len(els); got != 1 { t.Errorf("P1's elements on P2's budget = %d, want 1", got) }
```

- [x] **Step 2: Run it.** `go test ./internal/server/ -run TestConnectionBudgetRevoker_LeavesThirdPartyBudgets -count=1`. Expected: FAIL, 1 access row and 0 elements.
- [x] **Step 3: Fix `revokeDirection`:** skip every budget whose `bud.UserID` is not `owner` (`ListForUser` returns owned AND shared budgets; only owned ones belong to this pair). Then make the order global: `RevokeBetween` first COLLECTS the removals of both directions (budget id, member to remove), sorts them by budget id, and only then calls `RemoveMember` for each. Task 8 makes `RemoveMember` take the budget lock, budget locks are taken in id order everywhere, and two passes (A's budgets, then B's) are not one order. Correct the doc comments. Add to the test: A owns the budget with the HIGHER id (B is a member), B owns the one with the lower id (A is a member); `RemoveMember` is called for the lower id first (recording remover).
- [x] **Step 4: Run both revoker tests.** Expected: PASS.
- [x] **Step 5: Regression plan.** In section 11 add: `- [ ] Three users: A and B are both members of C's budget. A removes the connection with B: B stays a member of C's budget with their categories and plans.`
- [x] **Step 6: Commit** `fix: Disconnecting two users no longer removes one of them from a third user's budget`.

### Task 2: `ClearActiveBudget` writes only the option

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users_options.sql`, regenerated `gen/`
- Modify: `internal/user/repo/repo.go`, `sqlite.go`, `pgsql.go`; `internal/user/repository.go`
- Modify: `internal/server/glue_budget.go` (`budgetUserRepo`, `ClearActiveBudget`)
- Test: `internal/server/glue_budget_test.go` (create if absent), `internal/user/repo/repo_integration_test.go`

**Interfaces:**
- Produces: `Repository.ClearOptionIfValue(ctx context.Context, userID vo.Id, name, value string, now time.Time) (int64, error)` on `internal/user`. Used again by Task 8.
- `ClearActiveBudget` runs in the caller's transaction and takes the member's user row lock (`LockRow`) BEFORE the clear: `Repo.Save` upserts every option of the aggregate it read, so without the lock a profile update that read the old pointer could write it back after the clear commits.

- [x] **Step 1: Write the failing tests.**

```go
// repo: only a matching value is cleared
n, _ := repo.ClearOptionIfValue(ctx, uid, model.OptionBudget, budgetID, now)   // n == 1, value NULL
n, _ = repo.ClearOptionIfValue(ctx, uid, model.OptionBudget, "other", now)    // n == 0
// glue: the users row is not rewritten
before := rowOf(t, db, uid)   // SELECT password, email, updated_at FROM users
lookup.ClearActiveBudget(ctx, uid, budgetID)
if rowOf(t, db, uid) != before { t.Error("ClearActiveBudget rewrote the users row") }
// glue: a user that no longer exists is not an error
if err := lookup.ClearActiveBudget(ctx, vo.NewId(), budgetID); err != nil { t.Errorf(...) }
// glue: the row lock comes first (recording repo, pattern internal/user/lock_order_test.go):
//   calls == []string{"LockRow", "ClearOptionIfValue"}
// pgsql, two connections (//go:build enginecompare, pattern mutate_lock_pgsql_test.go):
//   UpdateName on connection 1 is paused between its read and its save; ClearActiveBudget on
//   connection 2 waits for it; afterwards the budget option is NULL, not the old pointer
// user.UpdateBudget (select an active budget): the access check runs INSIDE the locked
//   update. Recording test: calls == ["LockRow", "HasAccess", "GetByID", "Save"], and the
//   context HasAccess receives is the transaction's (the recording fake asserts it is not the
//   context the test passed in); a budget the user has no access to is still refused with the
//   same error as today
```

- [x] **Step 2: Run.** Expected: FAIL (method missing).
- [x] **Step 3: Add the query `ClearUserOptionIfValue :execrows`** in both engines: `UPDATE users_options SET value = NULL, updated_at = ? WHERE user_id = ? AND name = ? AND value = ?`. Regenerate, add to the `querier`, both adapters and `Repo`.
- [x] **Step 4: Rewrite `BudgetUserLookup.ClearActiveBudget`** to call `LockRow(ctx, userID)` and then `ClearOptionIfValue(ctx, userID, model.OptionBudget, budgetID.String(), l.clock.Now())`, nothing else; `budgetUserRepo` gains those two methods and loses `Save` if nothing else in that file uses it.
- [x] **Step 4b: `internal/user/profile.go` `UpdateBudget`:** today `s.budgets.HasAccess` runs before `s.mutate` takes the row lock, so a selection that passed the check can save a budget id after a revoke (or a wipe) cleared it. Rewrite `UpdateBudget` as one explicit `s.tx.WithTx(ctx, func(ctx context.Context) error { ... })`: `LockRow(ctx)`, `s.budgets.HasAccess(ctx, ...)`, `GetByID(ctx)`, the option change, `Save(ctx)`. Do NOT put the check into the `mutate` callback: that callback has no context parameter, the check would run on the outer context, ask the pool for a second connection, and on SQLite (one connection, already held by the transaction) wait on itself forever. The test runs through `dbtest` (the production SQLite opener) with a 5-second test timeout to catch exactly that.
- [x] **Step 5: Run** `go test ./internal/server/ ./internal/user/... ./internal/budget/... -count=1`. Expected: PASS.
- [x] **Step 6: Commit** `fix: Clearing a member's active budget no longer rewrites their user row`.

### Task 2b: the CLI revokes MCP grants

A bug in the released CLI, found while reviewing this plan: `internal/cli/container.go` builds the authserver but never calls `userSvc.SetMCPGrantRevoker`, and `revokeMCPGrants` is a silent no-op without it. `user:deactivate` and `user:change-password` therefore leave an MCP client able to refresh its access. Marking for deletion from the CLI would inherit it.

**Files:**
- Modify: `internal/cli/container.go`
- Test: `internal/cli/commands_test.go`
- Modify: `docs/regression-test-plan.md` (section 12a)

- [x] **Step 1: Failing test** `TestUserDeactivate_RevokesMCPGrants`: seed a user with an OAuth client, a grant and a live `eco_oat_` token (`fixture.OAuthClient`, `OAuthGrant`, `AccessToken`); `Run([]string{"user:deactivate", email})`; assert `oauth_grants.revoked_at` is set for that grant. The same assertion after `user:change-password`.
- [x] **Step 2: Run, see FAIL.** `go test ./internal/cli/ -run TestUserDeactivate_RevokesMCPGrants -count=1`
- [x] **Step 3: Fix:** `userSvc.SetMCPGrantRevoker(apps)` right after `apps` is built in `newContainerFor`.
- [x] **Step 4: Run** `go test ./internal/cli/ -count=1`. PASS.
- [x] **Step 5: Regression plan**, section 12a: `- [ ] `user:deactivate <email>` on the server: an MCP client connected as that user can no longer refresh its access.`
- [x] **Step 6: Commit** `fix: CLI user:deactivate and user:change-password revoke MCP grants`.

---

## Phase 1: schema, configuration, guard

### Task 3: migration, model, configuration

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261127000000.sql`, `.../pgsql/20261127000000.sql`
- Create: `internal/infra/storage/migrations/migration_20261127_test.go`
- Create: `internal/model/user_deletion.go`
- Modify: `internal/infra/storage/sqliteimport/sqliteimport.go`, `import_test.go`
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `docs/full.env.example`
- Regenerate: `internal/infra/storage/sqlc/gen/`

**Interfaces:**
- Produces (`internal/model/user_deletion.go`):

```go
type DeletionReason string
const (
	DeletionReasonSelf     DeletionReason = "self_initiated"
	DeletionReasonInactive DeletionReason = "inactive"
	DeletionReasonCLI      DeletionReason = "cli"
)
const (
	AuditDeletionMarked    = "deletion_marked"
	AuditDeletionCancelled = "deletion_cancelled"
	AuditDeleted           = "deleted"
	AuditLifetimeRegranted = "lifetime_regranted"
)
const (
	CancelCauseSignIn           = "sign_in"
	CancelCauseActiveConnection = "active_connection"
	CancelCauseInactivityOff    = "inactivity_off"
)
type DeletionState struct {
	MarkedAt     *time.Time
	Reason       DeletionReason
	LastActiveAt *time.Time
	CreatedAt    time.Time
}
type UserAuditEvent struct {
	ID        vo.Id
	UserID    vo.Id
	Event     string
	EmailHash *string
	RefID     *vo.Id // lifetime_regranted: the deleted event it consumes (unique)
	Details   string // JSON object
	CreatedAt time.Time // truncated to the second on write
}
type MarkedUser struct { ID vo.Id; Email string; Reason DeletionReason; MarkedAt time.Time }
type InactiveCandidate struct { ID vo.Id; Email, Name, Language string }
type DeletionRequest struct { ID, UserID vo.Id; Email, Code string; CreatedAt, ExpiredAt time.Time } // Email: users.email as stored when the code was sent
const DeletionRequestTTL = 10 * time.Minute
func NewDeletionRequest(id, userID vo.Id, storedEmail, codeHash string, now time.Time) *DeletionRequest
func (r *DeletionRequest) IsExpired(now time.Time) bool
// EmailHash is lowercase hex SHA-256 of NormalizeEmail(address).
func EmailHash(address string) string
```

- Produces (`config.Config`): `InactiveUserDays int`, `UserDeletionDays int`, `InactiveUserExclude []string` (already `model.NormalizeEmail`-ed), `RateLimitRequestUserDeletion int` (default 3), `RateLimitConfirmUserDeletion int` (default 5).

- [x] **Step 1: Write the failing tests.**

```go
// migration test (pattern: migration_20261116000000_test.go), both columns sets on sqlite
// users has deletion_marked_at (NULL), deletion_reason ('' NOT NULL), last_active_at (NULL)
// a user row inserted BEFORE the migration has last_active_at NOT NULL after it
// tables users_audit and users_deletion_requests exist; users_deletion_requests.user_id cascades
// sqliteimport: TestImport_AbortsWhenTargetHasOnlyAuditRowsWithoutForce: a target with no users
//   but one users_audit row refuses the import without --force (today only `users` is counted,
//   and the import truncates every table: lifetime entitlements and unpolled deletions would go)
// model
if got := model.EmailHash("  Anna@Example.TEST "); got != model.EmailHash("anna@example.test") { ... }
if len(model.EmailHash("a@b.test")) != 64 { ... }
// config
t.Setenv("ECONUMO_INACTIVE_USER_DAYS", "-1")            // Load() returns an error naming the variable
t.Setenv("ECONUMO_USER_DELETION_DAYS", "abc")           // error
t.Setenv("ECONUMO_INACTIVE_USER_DAYS", "36501")          // error: above the 36500-day cap
t.Setenv("ECONUMO_USER_DELETION_DAYS", "36500")          // accepted
t.Setenv("ECONUMO_INACTIVE_USER_EXCLUDE", " A@x.test ,b@x.test,") // []string{"a@x.test","b@x.test"}
// unset: all three are 0 / 0 / nil
```

- [x] **Step 2: Run.** `go test ./internal/infra/storage/migrations/ ./internal/infra/storage/sqliteimport/ ./internal/model/ ./internal/config/ -count=1`. Expected: FAIL.
- [x] **Step 3: Write the migration.** sqlite:

```sql
ALTER TABLE users ADD COLUMN deletion_marked_at DATETIME;
ALTER TABLE users ADD COLUMN deletion_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN last_active_at DATETIME;
UPDATE users SET last_active_at = strftime('%Y-%m-%d %H:%M:%S', 'now');
CREATE INDEX IDX_users_deletion_marked_at ON users (deletion_marked_at);
CREATE TABLE users_audit (
    id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL,
    event TEXT NOT NULL,
    email_hash TEXT,
    ref_id TEXT,
    details TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX UNIQ_users_audit_ref_id ON users_audit (ref_id) WHERE ref_id IS NOT NULL;
CREATE INDEX IDX_users_audit_event_created ON users_audit (event, created_at, id);
CREATE INDEX IDX_users_audit_email_hash ON users_audit (email_hash, created_at);
CREATE TABLE users_deletion_requests (
    id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    code TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    expired_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX UNIQ_users_deletion_requests_user ON users_deletion_requests (user_id);
```

  pgsql: the same with `UUID` for `id`/`user_id` where the sibling tables use it (copy the types of `users_email_change_requests` in the pgsql migrations), `TIMESTAMP(0) WITHOUT TIME ZONE` for every datetime (the type the other pgsql tables use: the admin cursor compares whole seconds, and a fractional `created_at` would make a page repeat forever), and the backfill `UPDATE users SET last_active_at = date_trunc('second', NOW() AT TIME ZONE 'UTC');`.
- [x] **Step 4: Write `internal/model/user_deletion.go`** as specified above (`crypto/sha256`, `encoding/hex`). In `sqliteimport.go` extend the non-empty-target check from `count(*) FROM users` to users OR `users_audit`.
- [x] **Step 5: Config.** Read the two day counts with `getIntStrict(key, 0)` and reject a value above 36500 (the pattern of `ECONUMO_CURRENCY_UPDATE_INTERVAL`'s cap): `(days+1) * 24h` overflows `time.Duration` near 106,752 days, and an overflowed cutoff lands in the future. Read the list with `getStringList` mapped through `model.NormalizeEmail`, the two rate limits in the existing table loop (`ECONUMO_RATE_LIMIT_REQUEST_USER_DELETION` 3, `ECONUMO_RATE_LIMIT_CONFIRM_USER_DELETION` 5). Document all five in `docs/full.env.example`, the first three under a new heading with this text:

```
# --- User deletion -----------------------------------------------------------
# Mark users with no activity for this many days for deletion and email them
# once. 0 = never. A user connected to an active user is never marked.
# ECONUMO_INACTIVE_USER_DAYS=0
# Delete a marked user's data this many days after the mark. 0 = never
# automatically: run `user:delete <email>` by hand.
# ECONUMO_USER_DELETION_DAYS=0
# Comma-separated emails the inactivity job never marks.
# ECONUMO_INACTIVE_USER_EXCLUDE=
```

- [x] **Step 6: `sqlc generate`, `go build ./...`, run the tests of step 2.** Expected: PASS. Also run `go test ./internal/infra/storage/... -count=1` (version parity).
- [x] **Step 7: Commit.**

### Task 4: the three user stores

**Files:**
- Create: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/user_deletion.sql`
- Create: `internal/user/repo/deletion.go`, `deletion_sqlite.go`, `deletion_pgsql.go`, `deletion_integration_test.go`
- Modify: `internal/user/repository.go`

**Interfaces:**
- Consumes: Task 3 model types.
- Produces (`internal/user/repository.go`):

```go
type DeletionStore interface {
	GetState(ctx context.Context, userID vo.Id) (model.DeletionState, error) // NotFound when the row is gone
	// Mark sets the mark only on an unmarked row; 0 rows = already marked.
	Mark(ctx context.Context, userID vo.Id, reason model.DeletionReason, now time.Time) (int64, error)
	Unmark(ctx context.Context, userID vo.Id) (int64, error)
	// TouchLastActive is the one write to a user row without the row lock:
	// blind, monotonic, at most once per 24 h.
	TouchLastActive(ctx context.Context, userID vo.Id, now time.Time) error
	ListMarked(ctx context.Context) ([]model.MarkedUser, error) // marked_at, id
	// cutoff = now - (InactiveUserDays+1) days.
	ListInactiveCandidates(ctx context.Context, cutoff time.Time) ([]model.InactiveCandidate, error)
	HasActiveConnection(ctx context.Context, userID vo.Id, cutoff time.Time) (bool, error)
	DeleteUser(ctx context.Context, userID vo.Id) (int64, error)
	// LockRowForWipe is stronger than LockRow: on PostgreSQL it is FOR UPDATE, which
	// also blocks an insert that references users.id (LockRow's FOR NO KEY UPDATE
	// lets those through).
	LockRowForWipe(ctx context.Context, userID vo.Id) error
	// LockWipes serializes wipes for the rest of the transaction (PostgreSQL:
	// pg_advisory_xact_lock; SQLite has one writer anyway). With it, deleted audit
	// rows become visible in the order of their created_at.
	LockWipes(ctx context.Context) error
	// ConnectedUserIDs: users connected to userID, sorted by id.
	ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}
type AuditLog interface {
	// Insert reports inserted=false when RefID is already taken (a lost re-grant race).
	Insert(ctx context.Context, e model.UserAuditEvent) (inserted bool, err error)
	ListByEmailHash(ctx context.Context, hash string) ([]model.UserAuditEvent, error)                               // created_at, id
	ListByEvent(ctx context.Context, event string, since time.Time, afterID string, limit int) ([]model.UserAuditEvent, error) // (created_at, id) cursor
}
type DeletionRequests interface {
	GetByUser(ctx context.Context, userID vo.Id) (*model.DeletionRequest, error)
	Save(ctx context.Context, r *model.DeletionRequest, generation int64) (int64, error)
	Consume(ctx context.Context, id, userID vo.Id) (int64, error)
	DeleteByUser(ctx context.Context, userID vo.Id) error
}
```

- `Insert` returns two values at every call site (Tasks 6, 14, 15, 17): all check `err`; only the re-grant of Task 15 looks at `inserted`.
- Produces (`internal/user/repo`): `NewDeletionRepo(driver string, tx *backend.TxManager) *DeletionRepo`, `NewAuditRepo(...) *AuditRepo`, `NewDeletionRequestRepo(...) *DeletionRequestRepo`.

- [ ] **Step 1: Write the failing integration tests** (`dbtest.New(t)`, `fixture`), one per rule:

```go
// Mark twice: first returns 1, second 0; GetState shows MarkedAt and Reason
// Unmark clears both columns; Unmark on an unmarked row returns 0
// TouchLastActive: stamp NULL -> set; stamp 23h old -> unchanged; stamp 25h old -> moved
// ListInactiveCandidates(cutoff): returns a user whose stamp is older than cutoff;
//   skips: marked, is_active = 0, stamp newer than cutoff, a user with last_active_at NULL
//   and created_at newer than cutoff; includes one with last_active_at NULL and old created_at
//   skips a user connected (fixture.Connect) to a user whose stamp is newer than cutoff;
//   does NOT skip when that connected user is marked
// HasActiveConnection mirrors the last two lines
// DeleteUser removes the row and returns 1; audit rows with that user_id stay
// ListByEvent pages with (since, afterID): 3 rows in one second, limit 2, then the third
// DeletionRequests.Save with a stale generation returns 0
// ConnectedUserIDs: U connected to C and A (fixture.Connect) -> [A, C] by id; nobody -> empty
// LockRowForWipe on a missing row returns nil (like LockRow)
// Insert stores created_at truncated to the second, on both engines (insert 12:00:00.750,
//   read back 12:00:00); a second row with the same RefID returns inserted=false, no error
// pgsql (enginecompare): LockWipes held by one connection makes a second LockWipes wait
// pgsql (enginecompare): while one connection holds LockRowForWipe(U), an INSERT INTO transactions
//   with user_id = U from a second connection waits; with LockRow it does not
```

- [ ] **Step 2: Run.** `go test ./internal/user/repo/ -run 'Deletion|Audit' -count=1`. Expected: FAIL.
- [ ] **Step 3: Write the queries** (both engines). The two that carry a decision:

```sql
-- name: TouchUserLastActive :exec
UPDATE users SET last_active_at = ? WHERE id = ? AND (last_active_at IS NULL OR last_active_at < ?);

-- name: ListInactiveUserCandidates :many
SELECT u.id, u.email, u.name, u.language FROM users u
WHERE u.deletion_marked_at IS NULL AND u.is_active = 1
  AND COALESCE(u.last_active_at, u.created_at) < sqlc.arg(cutoff)
  AND NOT EXISTS (
    SELECT 1 FROM users_connections c JOIN users p ON p.id = c.connected_user_id
    WHERE c.user_id = u.id AND p.deletion_marked_at IS NULL
      AND COALESCE(p.last_active_at, p.created_at) >= sqlc.arg(cutoff))
ORDER BY u.id;
```

  (`is_active = true` on pgsql.) The others are single-table statements whose shape the interface fixes: `GetUserDeletionState`, `MarkUserForDeletion` (`... WHERE id = ? AND deletion_marked_at IS NULL`), `UnmarkUserDeletion` (`SET deletion_marked_at = NULL, deletion_reason = '' WHERE id = ? AND deletion_marked_at IS NOT NULL`), `ListMarkedUsers`, `UserHasActiveConnection`, `ListWipeConnectedUserIDs` (`ORDER BY connected_user_id`; `connection.sql` already has a `ListConnectedUserIDs`, and sqlc generates one package per engine, so the name must differ), `DeleteUserByID`, `InsertUserAudit`, `ListUserAuditByEmailHash`, `ListUserAuditByEvent` (cursor as in `ListAffiliateUsers`), `LockUserRowForWipe` (pgsql `SELECT id FROM users WHERE id = $1 FOR UPDATE`, no-rows mapped to nil; sqlite the same no-op `UPDATE` as `LockUserRow`), and the four `users_deletion_requests` queries modelled on `email_change_request.sql` but naming exactly the six columns of Task 3 (`id, user_id, email, code, created_at, expired_at`: no `new_email`, no `updated_at`), plus `LockDeletionWipes` (pgsql `SELECT pg_advisory_xact_lock(7203251127)`, sqlite a Go no-op) and `InsertUserAudit` with `ON CONFLICT DO NOTHING` returning the affected rows. `TouchLastActive` passes `now` and `now.Add(-24*time.Hour)`.
- [ ] **Step 4: Repos and adapters** following `internal/user/repo/emailchangerequest*.go`. `ListMarked` and `ListInactiveCandidates` return `users.email` as stored; callers decode it.
- [ ] **Step 5: Run the tests of step 2, then with the PostgreSQL command of Global Constraints.** Expected: PASS on both.
- [ ] **Step 6: Commit.**

### Task 5: schema guard

**Files:**
- Create: `internal/test/wipeguard/guard.go`, `guard_test.go` (sqlite), `guard_pgsql_test.go` (`//go:build enginecompare`)

**Interfaces:**
- Produces: `wipeguard.Check(t *testing.T, db *dbtest.DB)`.

- [ ] **Step 1: Write `Check`** as a test helper that reads the foreign keys and columns of every table (`pragma_foreign_key_list` / `pragma_table_info` on sqlite, `pg_constraint` / `information_schema.columns` on pgsql) and fails when:
  1. a foreign key to `users(id)` is not `CASCADE`;
  2. inside the set of tables reachable from `users` by `CASCADE`, a foreign key into that set is `NO ACTION`/`RESTRICT`, or `SET NULL` on a NOT NULL column, and is not in `handledBlockers`;
  3. a column named `*_id` or `external_id` has no foreign key and is not in `handledLooseIDs`.

```go
var handledBlockers = map[string]string{
	"budgets.currency_id": "wipe step 7 moves a shared currency; step 9 deletes own budgets first",
	"debts.currency_id":   "wipe step 7 moves a shared currency; own debts go with own budgets",
}
var handledLooseIDs = map[string]string{
	"users_audit.user_id":                      "kept on purpose",
	"users_audit.ref_id":                       "names another users_audit row, which is never deleted",
	"idempotency_keys.user_id":                 "wipe step 8",
	"idempotency_keys.resource_id":             "wipe step 8",
	"oauth_states.link_user_id":                "wipe step 8",
	"budgets_elements.external_id":             "wipe step 3",
	"debts.element_external_id":                "wipe step 4",
	"import_transaction_links.applied_category_id": "wipe step 8",
	"import_transaction_links.applied_payee_id":    "wipe step 8",
	"import_transaction_links.applied_tag_id":      "wipe step 8",
	"import_transaction_links.applied_rule_id":     "wipe step 8",
	"import_suggestions.category_id":           "the owner's own entity; the row cascades with its owner",
	"import_suggestions.payee_id":              "the owner's own entity; the row cascades with its owner",
	"access_tokens.grant_id":                   "the token cascades with its user",
	"import_account_links.external_account_id":         "provider id, not an Econumo id",
	"import_transaction_links.external_account_id":     "provider id, not an Econumo id",
	"import_transaction_links.external_transaction_id": "provider id, not an Econumo id",
}
```

  Every entry of both maps must still match a real column (a stale entry fails the test).
- [ ] **Step 2: Run** `go test ./internal/test/wipeguard/ -count=1` and the PostgreSQL variant. Expected: PASS on both with exactly these lists. If a column appears that is not listed, STOP and report it (the spec must decide what the wipe does with it).
- [ ] **Step 3: Prove the guard bites:** temporarily delete one map entry, see the test fail naming that column, restore it.
- [ ] **Step 4: Commit.**

---

## Phase 2: marking, cancelling, activity

### Task 6: mark, cancel on session, activity stamp

**Files:**
- Create: `internal/user/deletion_mark.go`, `internal/user/deletion_mark_test.go`
- Modify: `internal/user/usecase.go` (fields, setter), `ports.go`, `session.go`, `oauth_tokens.go`, `authenticate.go`, `register.go`, `internal/shared/errs/codes.go`, `locales/*.json` (`errors.user.*`)

**Interfaces:**
- Consumes: Task 4 stores.
- Produces (`internal/user`):

```go
type DeletionConfig struct {
	InactiveDays int
	DeletionDays int
	Exclude      map[string]bool // model.NormalizeEmail-ed addresses
}
// SetDeletion installs the deletion stores. Until it is called every
// deletion path is a no-op, which keeps unrelated tests untouched.
func (s *Service) SetDeletion(store DeletionStore, audit AuditLog, requests DeletionRequests, cfg DeletionConfig)
// markForDeletion runs inside the caller's transaction, after LockRow.
func (s *Service) markForDeletion(ctx context.Context, userID vo.Id, reason model.DeletionReason) error
func (s *Service) AdminMarkDeleted(ctx context.Context, email string) error
```

- Produces (errors): `errs.CodeUserDeletionAlreadyMarked = "user.deletion_already_marked"` (en: "This user is already marked for deletion."), `errs.CodeUserDeletionNotMarked = "user.deletion_not_marked"` (en: "This user is not marked for deletion.").

- [ ] **Step 1: Write the failing tests** (service over sqlite, the helper that `internal/user/*_test.go` already uses to build a `Service`, plus `SetDeletion`):

```go
// TestMark_RevokesEverything: a session, a PAT and an OAuth token (fixture.AccessToken with each kind)
//   -> after AdminMarkDeleted: Authenticate(raw) is "Invalid access token" for all three;
//      credentials_generation moved by 1; state.MarkedAt set, Reason "cli";
//      one users_audit row event "deletion_marked", details {"reason":"cli"}, email_hash NULL
// TestMark_Twice: second AdminMarkDeleted returns a ValidationError with CodeUserDeletionAlreadyMarked
// TestLogin_ClearsTheMark: mark, then Login -> state.MarkedAt nil; audit row "deletion_cancelled"
//   with details {"reason":"cli","cause":"sign_in"}
// TestLogin_StartedBeforeTheMarkIsRefused: read the user (generation g), mark, then
//   createSession(..., g) -> UnauthorizedError; the mark is still set
// TestActivity_SessionAndRefreshStamp: stamp set 3 days back; Login moves it to now;
//   set it 3 days back again; IssueOAuthAccessToken moves it to now
// TestActivity_TouchIsDaily: stamp 2h old; Authenticate with a token whose last_used_at is 10 min old
//   leaves the stamp; with the stamp 25h old the same call moves it
// TestRegister_StampsNewUser: after Register, state.LastActiveAt is not nil
// TestDeletionCode_DiesWithTheCredential: a users_deletion_requests row exists; after
//   UpdatePassword it is gone; the same after ResetPassword and after AdminChangePassword
//   (the two reclaim paths)
```

- [ ] **Step 2: Run.** `go test ./internal/user/ -run 'TestMark|TestLogin_|TestActivity|TestRegister_Stamps' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - `markForDeletion`: `Mark` (0 rows -> `CodeUserDeletionAlreadyMarked`), `BumpCredentialsGeneration`, `revokeTokens(ctx, id, vo.Id{}, now, model.TokenKindSession, model.TokenKindPersonal, model.TokenKindOAuth)`, `revokeMCPGrants`, `requests.DeleteByUser`, audit insert. `AdminMarkDeleted` wraps it in `mutateByEmail`.
  - `createSession`: inside its existing transaction, after `InsertIfGeneration` returned 1, call `s.cancelMarkOnSession(ctx, userID)` (reads the state; when marked: `Unmark` + audit `deletion_cancelled` with the mark's reason and cause `sign_in`) and `s.stampActive(ctx, userID, now)`.
  - `IssueOAuthAccessToken`: `stampActive` when the insert matched. `Authenticate`: `stampActive` in the branch where the token was touched.
  - `persistNewUser`: `stampActive` in the same transaction as the `Save`.
  - `UpdatePassword` and `reclaimCredentials` delete the user's deletion requests next to `emailChangeRequests.DeleteByUser`: a pending code is a grant, and it must not outlive the credential that asked for it (the code email itself tells the user to change the password).
  - `stampActive`, `cancelMarkOnSession` and that sweep return nil when `SetDeletion` was never called.
- [ ] **Step 4: Run the tests of step 2 and the whole package** `go test ./internal/user/... -count=1`. Expected: PASS.
- [ ] **Step 5: i18n.** Add the two error keys to all 11 catalogues; run `go test ./internal/test/i18ntest/ -count=1`.
- [ ] **Step 6: Commit.**

---

## Phase 3: one primitive per feature

Every method in this phase runs on the caller's transaction context (a nested `WithTx` is a savepoint that joins it) and is tested in its own package on sqlite with `dbtest` + `fixture`.

### Task 7: debt

**Files:** Create `internal/debt/wipe.go`, `internal/debt/wipe_test.go`.

**Interfaces:**
- Consumes: existing `(*Service).LockBudget(ctx, budgetID vo.Id) error`, `(*Service).ReleaseCategory(ctx, id vo.Id) error`, `repo.ListByElement`.
- Produces: `func (s *Service) BudgetIDsOfCategories(ctx context.Context, ids ...vo.Id) ([]vo.Id, error)` (distinct, sorted by id) and `func (s *Service) HoldsBudgetLock(ctx context.Context, budgetID vo.Id) bool` (the repo's existing check, exported for Task 13).

- [ ] **Step 1: Failing test** `TestBudgetIDsOfCategories`: two categories with debts in budgets B2 and B1 (one debt paid off, one active) and a third category without debts -> returns `[B1, B2]` sorted; no categories -> empty slice, nil error. Second test `TestReleaseCategory_AfterExplicitLocks`: inside one `WithTx`, `LockBudget` on each id returned, then `ReleaseCategory(cat)` deletes the debts and returns nil (this is the sequence the wipe uses).
- [ ] **Step 2: Run, see FAIL.** `go test ./internal/debt/ -run 'TestBudgetIDsOfCategories|TestReleaseCategory_AfterExplicitLocks' -count=1`
- [ ] **Step 3: Implement** from the lookup `LockForCategories` already does, without locking.
- [ ] **Step 4: Run, see PASS. Commit.**

### Task 8: budget

**Files:** Create `internal/budget/wipe.go`, `wipe_test.go`; modify `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql`, `internal/budget/repository.go`, `internal/budget/repo/*`.

**Interfaces:**
- Consumes: existing `RemoveMember(ctx, budgetID, memberID vo.Id) error`, `s.users.ClearActiveBudget` (fixed in Task 2), `BudgetStore.Delete`, `s.debts.LockBudget` (the budget service's existing debt port).
- Changes: `RevokeAccess`, `RemoveMember` and `DeclineAccess` (it deletes the access row and clears the active budget too) take `s.debts.LockBudget(txCtx, budgetID)` as the FIRST statement of their transaction (when `s.debts` is set). Today a revoke deletes the access row and only then touches the member's user row; a wipe holds that user's row and wants the access row, so the two would wait on each other. With the budget lock first, on both, the revoke waits for the wipe.
- Produces:

```go
// WipeBudgetIDs: budgets the user owns plus budgets the user has an access row on. Distinct, sorted.
func (s *Service) WipeBudgetIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
// UnwindUser removes the user from every budget they are a member of (RemoveMember)
// and clears the active-budget pointer of every member of every budget the user owns.
func (s *Service) UnwindUser(ctx context.Context, userID vo.Id) error
// DeleteOwned deletes every budget the user owns.
func (s *Service) DeleteOwned(ctx context.Context, userID vo.Id) error
```

- Produces (repo): `ListOwnedIDs(ctx, userID vo.Id) ([]vo.Id, error)`, `ListMemberIDs(ctx, userID vo.Id) ([]vo.Id, error)` (budget ids from `budgets_access` rows of the user, accepted or pending).

- [ ] **Step 1: Failing tests.** Fixture: U owns budget BU (member P, whose `budget` option points at BU, with an element of P's category); P owns BP (member U with an element of U's category, a limit on it, U's account as a member account, U's category inside one of BP's envelopes); a third user T also a member of BP.

```go
// WipeBudgetIDs(U) == sorted {BU, BP}
// after UnwindUser(U): BP has access rows only for T; no element of U's category, its limit gone,
//   U's account is not a member account, the envelope no longer lists U's category;
//   T's access row and elements are untouched; P's "budget" option is NULL
// after DeleteOwned(U): BU is gone; BP still exists
// pending (not accepted) access row of U on another budget is removed by UnwindUser too
// lock order (recording debts port): RevokeAccess, RemoveMember and DeclineAccess call
//   LockBudget before DeleteAccess
```

- [ ] **Step 2: Run, see FAIL.** `go test ./internal/budget/ -run TestWipe -count=1`
- [ ] **Step 3: Add the two queries** (`ListBudgetIDsByOwner`, `ListBudgetIDsByAccessUser`, both `ORDER BY` id), regenerate, extend `BudgetStore`/`AccessStore` and the repo, implement the three methods.
- [ ] **Step 4: Run, see PASS. Commit.**

### Task 9: account

**Files:** Create `internal/account/wipe.go`, `wipe_test.go`; modify `internal/infra/storage/sqlc/query/pgsql/accounts.sql` (or the file that holds `ListAccountIDsByOwner`), `internal/account/repository.go`, `internal/account/repo/*`.

**Interfaces:**
- Consumes: existing `unwindGrant(ctx, accountID, affectedUserID vo.Id) error`, `AccessStore.ListReceived`, `ListPendingReceived`, `ListIssued`, repo `ListIDsByOwner`.
- Produces:

```go
// LockOwnedForWipe locks every account row the user owns, in id order
// (FOR UPDATE on PostgreSQL, nothing on SQLite).
func (s *Service) LockOwnedForWipe(ctx context.Context, userID vo.Id) error
// UnwindGrantsOf unwinds every grant the user holds and every grant on the user's accounts.
func (s *Service) UnwindGrantsOf(ctx context.Context, userID vo.Id) error
```

- [ ] **Step 1: Failing tests.** `TestUnwindGrantsOf`: U holds an accepted grant on P's account (placed in one of U's folders, with an `accounts_options` row) and a pending one on T's; P holds a grant on U's account (in P's folder, with options). After the call no `accounts_access` row names U as grantee or one of U's accounts; P's folder no longer holds U's account; P's own accounts and folders are untouched. `TestLockOwnedForWipe_PgsqlBlocksTransactionWrites` (`//go:build enginecompare`, pattern `dbtest.AssertLockWaitsForKeyShare` in `internal/test/dbtest/lock_pgsql.go`): after `LockOwnedForWipe` in one connection, an `INSERT INTO transactions` on that account from a second connection waits until the first commits.
- [ ] **Step 2: Run, see FAIL.**
- [ ] **Step 3: Implement.** New pgsql query `LockAccountsByOwner :many` = `SELECT id FROM accounts WHERE user_id = $1 ORDER BY id FOR UPDATE`; the sqlite querier is a Go no-op (pattern: `internal/category/repo/sqlite.go` `LockCategoryRow`). Put `ListIDsByOwner` and the lock on the `AccountStore` interface.
- [ ] **Step 4: Run (sqlite, then the PostgreSQL command). PASS. Commit.**

### Task 10: transaction

**Files:** Create `internal/transaction/wipe.go`, `wipe_test.go`; create `internal/infra/storage/sqlc/query/{sqlite,pgsql}/transaction_wipe.sql`; modify `internal/transaction/repository.go`, `internal/transaction/repo/*`.

**Interfaces:**
- Produces:

```go
// RewriteForDeletedUser is wipe steps 5 and 6 for one user.
func (s *Service) RewriteForDeletedUser(ctx context.Context, userID vo.Id) error
```

- Produces (repo): `ReassignAuthorInForeignAccounts(ctx, userID vo.Id) error`, `ListCrossOwnerTransfers(ctx, userID vo.Id) ([]CrossOwnerTransfer, error)`, `ReplaceTransferWithCorrection(ctx, t CrossOwnerTransfer, c *WipeCorrection) error` (a nil correction deletes the transfer and still bumps the surviving account's revision), with

```go
type CrossOwnerTransfer struct {
	ID, SourceAccountID, RecipientAccountID vo.Id
	SourceOwnerID, RecipientOwnerID         vo.Id
	Amount          string
	AmountRecipient *string
	Description     string
	SpentAt         time.Time
	SortKey         string
	RecipientSortKey *string
}
type WipeCorrection struct { // passed by pointer: a NULL amount_recipient leg gets none
	ID, AuthorID, AccountID vo.Id
	Type        model.TransactionType // expense or income
	Amount      string
	Description string
	SpentAt     time.Time
	SortKey     string
}
```

- [ ] **Step 1: Failing tests** (`fixture.Transaction` with explicit `SortKey`/`RecipientSortKey`; balances through the account repo's `Balance`):

```go
// TestRewrite_ReassignsForeignAuthoredRows: U wrote an expense and an income into P's account and a
//   transfer between P's account and T's account, and an expense into EX-partner X's account (no
//   access row left). After: user_id of each is the owner of account_id (P, P, P, X); id, category,
//   payee, tag, labels, description, spent_at, amount, sort keys unchanged.
//   fixture.RequireRevisionsMoved: every one of those accounts moved by exactly 1.
// TestRewrite_TransferFromPartnerToUser: P.acc -> U.acc, amount "10", description "rent".
//   After: the original id is gone; P.acc has ONE new row: type expense, amount "10",
//   user_id P, description "Original transaction deleted: rent", spent_at equal, sort_key equal to
//   the original sort_key, account_recipient_id NULL, category/payee/tag NULL.
//   Balance of P.acc at every date is unchanged.
// TestRewrite_TransferFromUserToPartner: U.acc -> P.acc, amount "10", amount_recipient "9.5",
//   recipient_sort_key "m". After: P.acc has one income "9.5" with sort_key "m"; balance unchanged.
// TestRewrite_NullRecipientAmount: U.acc -> P.acc with amount_recipient NULL: the original is
//   deleted and NO row is added on P.acc; its balance is unchanged.
// TestRewrite_EmptyAndLongDescriptions: "" -> "Original transaction deleted"; a 255-rune
//   description -> the result is exactly 255 runes and starts with the prefix.
// TestRewrite_SoftDeletedAccounts: the same two transfer cases with P.acc soft-deleted and, in a
//   second run, U.acc soft-deleted: the correction is still written and P.acc's balance is unchanged.
// TestRewrite_BumpsAccountsThatLoseReferences: P's own rows in P.acc that point at U's category,
//   U's payee, U's tag, carry U's label, are linked to U's import source, or have a recurring_id
//   of a template the cascade will delete (U's template on P.acc; P's template that transfers
//   into U's account): after the call (the
//   rows themselves are not changed yet, the cascade does that) P.acc's revision moved by exactly 1.
// TestRewrite_OwnTransfersUntouched: U.acc1 -> U.acc2 and P.acc1 -> P.acc2 written by P: untouched.
// TestRewrite_SortKeysStayUnique: after every case, no two rows of an account's day share a key
//   on that account's side.
```

- [ ] **Step 2: Run, see FAIL.** `go test ./internal/transaction/ -run TestRewrite -count=1`
- [ ] **Step 3: Implement.**
  - `ReassignAuthorInForeignAccounts`: first `rev.BumpWhere(ctx, "user_id = ? AND account_id IN (SELECT id FROM accounts WHERE user_id <> ?)", id, id)`, then `UPDATE transactions SET user_id = (SELECT a.user_id FROM accounts a WHERE a.id = transactions.account_id) WHERE user_id = ? AND account_id IN (SELECT id FROM accounts WHERE user_id <> ?)`. `updated_at` is not touched.
  - `ListCrossOwnerTransfers`: `type = 2`, both accounts joined, exactly one of the two owners is the user.
  - `BumpAccountsLosingReferences(ctx, userID)` (repo): six `rev.BumpWhere` calls, BEFORE anything
    changes, for transactions whose `category_id` / `payee_id` / `tag_id` is one of the user's, that
    carry one of the user's labels (`id IN (SELECT transaction_id FROM transactions_labels WHERE
    label_id IN (SELECT id FROM labels WHERE user_id = ?))`), or that are linked to one of the user's
    import sources, or whose `recurring_id` is a template the cascade deletes (`recurring_transactions`
    with `user_id` = the user, or `account_id` / `account_recipient_id` one of the user's accounts).
    The cascade and `SET NULL` of wipe step 9 change what those rows show.
  - `RewriteForDeletedUser`: `BumpAccountsLosingReferences`, then reassign, then for each listed transfer build the correction in Go (prefix `"Original transaction deleted"`, `": "` + description when not empty, cut to 255 runes) and call `ReplaceTransferWithCorrection`, which `rev.Bump`s the surviving account, inserts the correction (when there is one) with the explicit `sort_key` (a dedicated `InsertWipeCorrection` statement: same columns as `InsertCorrectionTransaction` in `account_corrections.sql`) and deletes the original.
- [ ] **Step 4: Run, see PASS, then the PostgreSQL command for the same tests. Commit.**

### Task 11: currency

**Files:** Create `internal/currency/wipe.go`, `wipe_test.go`; modify `internal/infra/storage/sqlc/query/{sqlite,pgsql}/currency_write.sql`, `currency_read.sql`, `internal/currency/repo/manage.go`, `read.go` and their adapters.

**Interfaces:**
- Produces: `func (s *ManageService) MoveSharedCustomCurrencies(ctx context.Context, userID vo.Id) error`.

- [ ] **Step 1: Failing tests.**

```go
// TestMove_ToTheOnlyForeignOwner: U's custom "PTS" is the currency of P's budget.
//   After: currencies.user_id == P, is_deleted unchanged (0).
// TestMove_ElementDebtAndAccountCount: "PTS" used only by an element of P's budget / only by a debt
//   in P's budget / only by P's account (three subtests): moved to P each time.
// TestMove_LowestUserIDWins: used by budgets of P and T -> owner is min(P, T).
// TestMove_CollisionArrivesSoftDeleted: P already has a live custom "PTS" -> the moved one has
//   user_id P and is_deleted = 1; P's own "PTS" untouched.
// TestMove_UnusedStays: a custom currency used only by U's own budget keeps user_id U.
// TestMove_RatesFollow: currencies_rates rows of the moved currency still exist.
// TestCurrencyList_OwnedReferences: after the move to P, T (owner of a budget in that currency,
//   no access rows) still gets the currency from the read service's list;
//   same for a currency referenced by an element, a debt, and an account T owns.
```

- [ ] **Step 2: Run, see FAIL.** `go test ./internal/currency/... -run 'TestMove|TestCurrencyList_Owned' -count=1`
- [ ] **Step 3: Implement.** Queries: `ListLiveAndDeletedCustomCurrencyIDsByOwner`, `FirstForeignOwnerOfCurrency :one` (one statement: `SELECT owner FROM (accounts.user_id UNION budgets.user_id UNION owner of the budget of budgets_elements UNION owner of the budget of debts, each filtered on currency_id) o WHERE owner <> ? ORDER BY owner LIMIT 1`; PostgreSQL 17 has no `MIN(uuid)`; no row = nobody else uses it), `MoveCurrency` (`UPDATE currencies SET user_id = ?, is_deleted = ? WHERE id = ?`), and the existing `OwnerCurrencyCodeExists` for the collision. Extend `GetUserCurrencyListView` with four more `OR c.id IN (...)` arms keyed on the caller owning the account / budget / budget of the element / budget of the debt.
- [ ] **Step 4: Run, PASS, on sqlite AND with the PostgreSQL command** (the owner query is the engine-sensitive one); **run `go test ./internal/test/apiparity/ -count=1`** (the list query changed; goldens must not move). **Commit.**

### Task 12: authserver, imports, idempotency

**Files:** Create `internal/authserver/wipe.go` + test, `internal/imports/repo/wipe.go` + test; modify `internal/infra/idempotency/store.go` + test; queries in `oauth_server.sql`, `imports.sql`, `idempotency.sql` (both engines).

**Interfaces:**
- Consumes: existing `(*authserver.Service).DeleteApp(ctx, id vo.Id) (int, error)`.
- Produces:

```go
func (s *authserver.Service) DeleteAppsOwnedBy(ctx context.Context, userID vo.Id) error
func (r *importsrepo.Repo) ClearAppliedRefsToUser(ctx context.Context, userID vo.Id) error
func (s *idempotency.Store) DeleteByUser(ctx context.Context, userID vo.Id) error
```

- [ ] **Step 1: Failing tests.**

```go
// authserver: U owns app A; U and T both hold grants on A, T has a live eco_oat_ token
//   (fixture.OAuthClient/OAuthGrant/AccessToken with the grant). After DeleteAppsOwnedBy(U):
//   the client row is gone; T's token has revoked_at set; a global app and T's own app are untouched.
//   An app that was U's when listed but is global by the time its lock is taken (hook between
//   the list and the delete runs MakeClientGlobal) is NOT deleted and keeps its grants.
// imports: P's import link whose applied_category_id / applied_payee_id / applied_tag_id point at
//   U's category/payee/tag and whose applied_rule_id points at U's rule -> all four NULL after the
//   call; a link pointing at P's own entities is unchanged; U's own links are unchanged.
// idempotency: two keys of U and one of P -> only P's is left.
```

- [ ] **Step 2: Run, see FAIL.**
- [ ] **Step 3: Implement.** `ListOAuthClientIDsByOwner` (`ORDER BY id`), then per id the existing delete path with one addition: after the client lock is taken, re-read the client and skip it when `owner_user_id` is no longer the user (an operator's `app:make-global` in between). Give `deleteApp` an optional expected owner rather than duplicating its locking. Four `UPDATE import_transaction_links SET applied_x_id = NULL WHERE applied_x_id IN (SELECT id FROM <table> WHERE user_id = ?) AND source_id NOT IN (SELECT id FROM import_sources WHERE user_id = ?)` statements (`categories`, `payees`, `tags`, `import_rules`); the user's own links go with the cascade and are left alone, as the test requires. `DeleteIdempotencyKeysByUser`.
- [ ] **Step 4: Run, PASS. Commit.**

---

## Phase 4: the wipe

### Task 13: one wiring for server, CLI and tests

**Files:** Create `internal/server/services.go`, `internal/server/glue_user_wipe.go`, `internal/server/glue_user_wipe_test.go`; modify `internal/server/server.go`, `internal/user/ports.go`, `internal/cli/container.go`.

**Interfaces:**
- Consumes: Tasks 7-12 methods, existing `categoryrepo.Repo.ListByOwner`, `oauthrepo.StateRepo.DeleteByLinkUser`.
- Produces (`internal/user/ports.go`):

```go
// DataWiper is everything a wipe does outside the user feature.
type DataWiper interface {
	// LockBudgets locks, in id order, every budget the wipe will touch. First in the transaction.
	LockBudgets(ctx context.Context, userID vo.Id) error
	// Erase runs after the user row is locked and leaves only the users row to delete.
	Erase(ctx context.Context, userID vo.Id) error
}
func (s *Service) SetDataWiper(w DataWiper)
```

- Produces (`internal/server`):

```go
type Built struct {
	API   http.Handler
	Admin http.Handler
	Rates *appcurrency.RateUpdater
	Users *appuser.Service
}
func BuildWithServices(cfg config.Config, db *sql.DB, seams Seams) (*Built, error)
// Build keeps its signature and delegates.
func NewUserDataWiper(...) *UserDataWiper // consumer-side interfaces, one per feature
// ErrWipeSetChanged: retry the wipe; nothing was changed.
var ErrWipeSetChanged = errors.New("wipe: the set of budgets changed while locking")
```

- [ ] **Step 1: Failing test** `TestUserDataWiper_Order` with recording fakes for each consumer interface:

```go
// LockBudgets: budgets = sorted distinct union of budget.WipeBudgetIDs(U) and
//   debt.BudgetIDsOfCategories(ids of category.ListByOwner(U)); debt.LockBudget called once per id, ascending
// Erase calls, in this order:
//   account.LockOwnedForWipe, then the lock-set re-check (below), budget.UnwindUser, account.UnwindGrantsOf,
//   debt.ReleaseCategory (once per category of U), transaction.RewriteForDeletedUser,
//   currency.MoveSharedCustomCurrencies, authserver.DeleteAppsOwnedBy,
//   imports.ClearAppliedRefsToUser, idempotency.DeleteByUser, oauthStates.DeleteByLinkUser,
//   budget.DeleteOwned
// an error from any call stops the sequence and is returned unchanged
// lock-set re-check: Erase recomputes the budget set; when it holds an id for which
//   debt.HoldsBudgetLock is false (a budget cloned or shared in the window between reading
//   the set and locking it) it returns server.ErrWipeSetChanged before changing anything
```

- [ ] **Step 2: Run, see FAIL.**
- [ ] **Step 3: Implement** `UserDataWiper`; move the body of `Build` into `BuildWithServices` (`Build` returns its first three fields); in it, after every service exists, call `userSvc.SetDeletion(userrepo.NewDeletionRepo(...), userrepo.NewAuditRepo(...), userrepo.NewDeletionRequestRepo(...), appuser.DeletionConfig{...from cfg})` and `userSvc.SetDataWiper(NewUserDataWiper(...))`. In `internal/cli/container.go` call the same `SetDeletion` (no wiper) and add `func (c *container) fullUser() (*appuser.Service, error)` that lazily runs `server.BuildWithServices(c.cfg, c.db, server.Seams{})` and returns `.Users`.
- [ ] **Step 4: Run** `go test ./internal/server/ ./internal/cli/ -count=1` and `go test ./internal/test/archtest/ -count=1`. PASS. **Commit.**

### Task 14: the wipe use case and its fixture test

**Files:** Create `internal/user/deletion_wipe.go`; create `internal/test/wipetest/fixture.go`, `wipe_test.go`, `lock_pgsql_test.go` (`//go:build enginecompare`).

**Interfaces:**
- Consumes: `DataWiper`, Task 4 stores, `server.BuildWithServices`.
- Produces:

```go
type WipeOutcome string
const (
	WipeDone       WipeOutcome = "deleted"
	WipeNotMarked  WipeOutcome = "not_marked"   // cancelled meanwhile, or a different mark than selected
	WipeReleased   WipeOutcome = "released"     // inactive mark cancelled by the household rule
)
// Wipe deletes one marked user. selectedMark is the deletion_marked_at an age-based
// caller selected (nil for user:delete): a different value under the lock is WipeNotMarked.
func (s *Service) Wipe(ctx context.Context, userID vo.Id, selectedMark *time.Time) (WipeOutcome, error)
```

- [ ] **Step 1: Write `wipetest/fixture.go`**: `func Seed(t *testing.T, db *dbtest.DB) Fixture` building the spec's fixture with `internal/test/fixture`: U; partner P (shared account each way, shared budget each way, cross transfers both directions, one comment by U on P's budget element, debt D1 on U's category in P's budget, debt D2 on one of P's OWN categories in P's budget, debt D3 on an ENVELOPE of P's budget that holds one category of U and one of P, with one expense in U's category on U's member account inside D3's life, so the debt sync materializes a payment split for it); ex-partner X holding one row U wrote; third user T sharing a budget with both U and P (owned by T); a legacy transfer U.acc -> P.acc with NULL `amount_recipient`; an OAuth app U owns with a grant of T; one row P wrote in P's account categorized with U's category; U's custom currency as the currency of U's own budget, of P's budget, of one element of P's budget and of P's debt D2; idempotency keys and an `oauth_states` row of U. `Fixture` exposes the ids.
- [ ] **Step 2: Failing tests** (`dbtest.New(t)`, so the same file runs on PostgreSQL under the tag; services from `server.BuildWithServices(cfg, db.Raw, server.Seams{})`):

```go
// TestWipe_Fixture: mark U (AdminMarkDeleted), record every other user's account balances at each
//   distinct spent_at day (+ all-time) and fixture.Revisions of their accounts, then Wipe(U, nil):
//   outcome WipeDone; users row of U gone; balances identical; every account of P/X/T whose rows
//   changed (steps 5-6, or a row that pointed at U's category) moved its revision by exactly 1 and
//   no other account moved; every remaining row has a sort_key, transfers have a
//   recipient_sort_key, and no two rows of an account's day share a key (the two invariants
//   revisionguard enforces on API routes; the wipe has no route, so this test carries them);
//   P and T still both have access to T's budget; debt D1 is gone; D3 has no debts_payments
//   row left and, read through the debt read path the API uses, reports the balance of its last
//   snapshot with no payment applied (not a stale figure, not an error); P's budget and debt D2
//   still resolve their currency (currencies row exists, user_id == min(P,T) owner as applicable);
//   no table has a row whose user-naming column holds U's id except users_audit
//   (iterate wipeguard's column list); one users_audit row event "deleted" with
//   details {"reason":"cli","lifetimeAccess":true} and email_hash == model.EmailHash(U's email).
// TestWipe_EmptyUser: a user with nothing -> WipeDone.
// TestWipe_HashIsOfTheCurrentEmail: full-access user, mark, change users.email, wipe -> the hash
//   is of the NEW address.
// TestWipe_NotLifetime: user with access_until set -> details lifetimeAccess false, email_hash NULL.
// TestWipe_NotMarked: unmarked user -> WipeNotMarked, row still there.
// TestWipe_SelectedMarkMismatch: Wipe(U, &otherTime) -> WipeNotMarked.
// TestWipe_RollsBackWhole: a DataWiper whose Erase fails after doing part of the work (wrap the
//   real one) -> error; U's row, U's access rows and P's transfers are exactly as before.
```

- [ ] **Step 3: Run, see FAIL.** `go test ./internal/test/wipetest/ -count=1`
- [ ] **Step 4: Implement `Wipe`**, one `s.tx.WithTx`:
  0. `store.LockWipes`; 1. `wiper.LockBudgets`; 2. `store.LockRowForWipe` (not `LockRow`: a request that authenticated before the mark could otherwise still insert a row authored by the user into a partner's account after step 5 scanned it, and the cascade would delete it); 3. `GetState` (NotFound or unmarked or `selectedMark` mismatch -> `WipeNotMarked`); for reason `inactive`, first `repo.LockRow` on every `store.ConnectedUserIDs(userID)` in id order (a partner's sign-in and activity stamp write that row, so they now wait for the wipe or the wipe sees them; without it a partner could become active between the check and the commit), then `s.inactiveMarkStillAllowed(ctx, userID)` (Task 17 fills it in; here it returns true) else `Unmark` + audit and return `WipeReleased`; 4. read the user, decode the email, `lifetime := u.AccessLevel == model.AccessLevelFull && u.AccessUntil == nil`; 5. `wiper.Erase`; 6. `store.DeleteUser` (must be 1); 7. audit `deleted`.
  A nil wiper is an error ("wipe is not wired"), never a silent skip. The audit row is the last statement, written with the clock read at that moment: with wipes serialized by `LockWipes`, rows become visible in timestamp order as long as the server clock does not go back (the overlap in the polling contract of Task 19 covers a clock that does).
- [ ] **Step 5: Run on sqlite, then with the PostgreSQL command.** Expected: PASS on both. If `DELETE FROM users` fails on a constraint, the failing column belongs to Task 5's lists: STOP and report.
- [ ] **Step 6: Lock-order test** `TestWipe_PartnerTransferWaits` (PostgreSQL, two pools as in `internal/user/mutate_lock_pgsql_test.go`): wrap the wiper so that right after `Erase` returns (before the users row is deleted) a second connection starts `transaction.Create` of a transfer from U's shared account to P's account as P; assert that call does not return until the wipe commits, then fails; assert P's balance equals the pre-wipe balance. A second case: at the same point a second connection inserts, as U, an expense into P's shared account (U still has write access in that connection's snapshot); the insert must wait and then fail, never commit and vanish. A third case (reason `inactive`, U connected to P with no shared budget): the wipe is paused right after its household check; P's `Login` on a second connection must wait until the wipe commits (it cannot slip in between the check and the delete). A fourth case: P runs `RevokeAccess` on U from P's budget while the wipe holds its locks; either order is acceptable, a hang longer than 10 s or a wipe that half-applies is not.
- [ ] **Step 7: Commit.**

---

## Phase 5: lifetime re-grant

### Task 15: one-time re-grant with proof of the mailbox

**Files:** Create `internal/user/deletion_regrant.go`, `deletion_regrant_test.go`; modify `register.go` (`persistNewUser`), `external.go` (`ProvisionExternalUser`), `admin.go` (`AdminCreateUser`, `AdminVerifyEmail`), `verify_email.go` (`ConfirmEmail`), `password.go` (`ResetPassword`), `docs/regression-test-plan.md`.

**Interfaces:**
- Produces: `func (s *Service) regrantLifetime(ctx context.Context, u *model.User, plainEmail string, now time.Time) (bool, error)`: inside the caller's transaction; when the hash has an unconsumed lifetime deletion it calls `u.SetAccess(model.AccessLevelFull, nil, now)` (the caller saves `u`) and inserts `lifetime_regranted` with `RefID` = the consumed `deleted` row's id, details `{"fromUserId":"...","deletedEventId":"..."}` and the email hash. When the insert reports `inserted=false` (someone else consumed it first) it grants nothing.

- [ ] **Step 1: Failing tests** (trial days 14 on the service; a `deleted` audit row with `lifetimeAccess:true` for `anna@example.test` seeded directly):

```go
// TestRegrant_ProviderSignUp: ProvisionExternalUser -> AccessLevel full, AccessUntil nil;
//   one lifetime_regranted row whose deletedEventId is the seeded row's id
// TestRegrant_CLI: AdminCreateUser -> same
// TestRegrant_PasswordWithGate: emailVerification on: Register -> AccessUntil is the trial end;
//   ConfirmEmail with the right code -> full, nil
// TestRegrant_PasswordWithoutGate: emailVerification off: Register -> still the trial;
//   AdminVerifyEmail -> full, nil
// TestRegrant_OnlyOnce: after a re-grant, move that user to another address (SetEmail), then
//   ProvisionExternalUser with anna@example.test again -> trial, no second regrant row
// TestRegrant_SameSecond: the deleted row and the regrant row share created_at; a second deletion
//   of the re-granted user (new deleted row, same second) grants again exactly once
// TestRegrant_PasswordReset: gate on, Register, then ResetPassword with the emailed code
//   (it marks the address verified) -> full, nil
// TestRegrant_NotOnEmailChange: a trial user on another address changes their email to
//   anna@example.test (ConfirmEmailChange) -> still the trial, the deletion stays unconsumed;
//   that same (already verified) user then completes ResetPassword at the new address ->
//   STILL the trial: a reset re-grants only an account that was unverified until this reset
// TestRegrant_LostRace: a lifetime_regranted row with that RefID already exists (inserted
//   directly) -> the caller keeps the trial and no error is returned
// TestRegrant_NotLifetime: a deleted row with lifetimeAccess false -> no grant
// TestRegrant_OtherAddress: no row for the hash -> no grant
```

- [ ] **Step 2: Run, see FAIL.** `go test ./internal/user/ -run TestRegrant -count=1`
- [ ] **Step 3: Implement.** Match in Go over `audit.ListByEmailHash(hash)`: the set of `RefID` values of `lifetime_regranted` rows, then the first `deleted` row with `lifetimeAccess` true whose id is not in the set; the unique index on `ref_id` is what makes it once even for two accounts racing. Call sites: the places where a NEWLY REGISTERED account's address is proven, each of which already holds the row lock and saves the whole aggregate afterwards, so the re-grant only sets the access on `u` before that `Save`: `persistNewUser` (it gets a `proven bool` parameter: true from `ProvisionExternalUser` and `AdminCreateUser`, false from `Register`), `ConfirmEmail` and `ResetPassword` (after the code is consumed, next to `MarkEmailVerified`), each ONLY when `u.EmailVerified` was false before the call (the unverified-to-verified transition is what marks a newly registered account; an already verified account resetting its password proves nothing new), and `AdminVerifyEmail` (the operator vouches, whatever the flag says). NOT `ConfirmEmailChange`, `ReplaceVerifiedEmail` or `AdminChangeEmail`: they replace the address with an SQL-only statement and have no aggregate save, so setting access on the aggregate there would either be lost or write the old email back (see Open items).
- [ ] **Step 3b: Regression plan** (section 2): `- [ ] A deleted account that had full access with no end date: registering again with the same email and confirming the address gives full access with no end date again; a second new account on that address later does not.`
- [ ] **Step 4: Run, PASS; `go test ./internal/user/... -count=1`. Commit.**

---

## Phase 6: daily job, CLI, admin

### Task 16: inactivity notice email

**Files:** Create `internal/infra/mailer/deletion.go`, `deletion_test.go`; modify `internal/infra/mailer/reset.go` (`EmailKeys`), `locales/*.json` (`emails.*`), `internal/server/services.go` and `internal/cli/container.go` (construct and pass the sender), `internal/user/usecase.go`.

**Interfaces:**
- Produces:

```go
type DeletionSender struct{ ... }
func NewDeletionSender(m Mailer, from, replyTo, appURL string) *DeletionSender
// lang is explicit: the job has no request.
func (s *DeletionSender) SendInactivityNotice(ctx context.Context, lang, to, name string, days int) error
func (s *DeletionSender) SendDeletionCode(ctx context.Context, to, name, code string) error // lang from reqctx
func (s *user.Service) SetDeletionSender(sender *mailer.DeletionSender)
```

- `en` copy:
  - `emails.inactivity_notice.subject`: `Your Econumo data is scheduled for deletion`
  - `emails.inactivity_notice.body`: `Hi {name},\n\nUnder our policy we delete the data of users inactive for {days} days. If you want to keep your data, sign in.`
  - `emails.delete_data.subject`: `Confirm deleting your data`
  - `emails.delete_data.body`: `Hi {name},\n\nUse this code to confirm that you want to delete your Econumo data: {code}\n\nThe code expires in 10 minutes. If you did not request this, change your password.`

- [ ] **Step 1: Failing test:** with a recording `Mailer`, `SendInactivityNotice(ctx, "de", ...)` sends the German subject, the body contains the name and `365`; the app link wrapper (`mailer.WithAppLink`) appends the URL when set. `SendDeletionCode` body contains the code.
- [ ] **Step 2: Run, FAIL. Step 3: implement, add the four keys to `EmailKeys` and all 11 catalogues. Step 4:** `go test ./internal/infra/mailer/ ./internal/test/i18ntest/ -count=1`. PASS. **Commit.**

### Task 17: release, wipe and mark passes; the job

**Files:** Create `internal/user/deletion_pass.go`, `deletion_pass_test.go`, `deletion_job.go`; modify `internal/user/deletion_wipe.go` (`inactiveMarkStillAllowed`), `cmd/econumo/main.go`.

**Interfaces:**
- Produces:

```go
type DeletionPassResult struct {
	Released, Wiped, Marked int
	Failed   []PassFailure // wipe failures only
	Err      error         // a pass that could not run at all (listing failed)
}
type PassFailure struct{ UserID vo.Id; ErrType string }
// ReleaseInactiveMarks cancels inactive marks the household rule no longer allows,
// and all of them when InactiveDays is 0.
func (s *Service) ReleaseInactiveMarks(ctx context.Context) (int, error)
// WipeExpired wipes users marked at least DeletionDays ago; nobody when DeletionDays is 0.
func (s *Service) WipeExpired(ctx context.Context) (wiped int, failed []PassFailure, err error)
// MarkInactive emails and marks; nobody when InactiveDays is 0.
func (s *Service) MarkInactive(ctx context.Context) (int, error)
// PurgeDeleted = ReleaseInactiveMarks + WipeExpired (user:purge-deleted).
func (s *Service) PurgeDeleted(ctx context.Context) DeletionPassResult
type DeletionJob struct{ ... }
func NewDeletionJob(svc *Service, clock port.Clock) *DeletionJob
func (j *DeletionJob) StartPolling(ctx context.Context) // first run 2 min after boot, then every 24 h
```

- [ ] **Step 1: Failing tests** (a settable clock; `SetDeletion` with the config under test; a recording or failing mailer):

```go
// cutoff rule: InactiveDays 30 -> a stamp 30 days 23 h old is NOT inactive, 31 days 1 h old is
// TestMarkInactive_MarksAndEmailsOnce: reason "inactive", one email to the decoded address in
//   the user's stored language, days == 30
// TestMarkInactive_SkipsExcludedAndConnected: an address in Exclude; a user connected to an
//   active user; both left unmarked, no email
// TestMarkInactive_WholeHousehold: A and B connected, both inactive -> both marked
// TestMarkInactive_MailerFailureIsIsolated: the mailer fails for the first candidate -> that
//   user is NOT marked, the second IS, the returned count is 1, no error
// TestMarkInactive_SignedInWhileTheMailWasSent: the mailer's Send hook runs Login for that
//   candidate -> the user is NOT marked and the new session still authenticates
// TestMarkInactive_EmailChangedWhileTheMailWasSent: the hook changes users.email -> not marked
// TestMarkInactive_Off: InactiveDays 0 -> 0, no email
// TestRelease_PartnerCameBack: A and B both marked inactive; B logs in (mark cleared);
//   ReleaseInactiveMarks -> A unmarked, audit cause "active_connection"
// TestRelease_MarkedPartnerDoesNotCount: B marked self_initiated with a recent stamp -> A stays marked
// TestRelease_InactivityOff: InactiveDays 0 -> every inactive mark released with cause
//   "inactivity_off"; a "cli" and a "self_initiated" mark stay
// TestRelease_RunsWhenDeletionDaysIsZero
// TestRelease_DoesNotCancelAReplacementMark: the pass lists A's inactive mark; before it locks A,
//   A signs in and is marked again from the CLI (hook on the store's ListMarked) -> the cli mark
//   stays
// TestPurgeDeleted_ListingFails: a store whose ListMarked errors -> PurgeDeleted().Err is that
//   error; the CLI test of Task 18 expects exit 1 for it
// TestWipeExpired_AgeRule: DeletionDays 30: a mark 29 days old stays, 31 days old is wiped;
//   DeletionDays 0: nobody
// TestWipeExpired_ContinuesPastAFailure: wiper fails for the first user -> second is wiped,
//   Failed has one entry with the first id and a Go type name, never the error text
// TestWipe_ReleasesInsteadOfWiping: an inactive mark with an active connection, Wipe(U, nil)
//   -> WipeReleased, row still there
```

- [ ] **Step 2: Run, see FAIL.** `go test ./internal/user/ -run 'TestMarkInactive|TestRelease|TestWipeExpired|TestWipe_Releases' -count=1`
- [ ] **Step 3: Implement.** `cutoff = now - (InactiveDays+1)*24h`. `MarkInactive` per candidate: skip `Exclude[model.NormalizeEmail(decoded)]`; send; on a send error `slog.Warn("deletion: inactivity notice failed", "user_id", ..., "err_type", ...)` and continue; else one transaction: `LockRow`, then re-read EVERYTHING the selection relied on, because the mail took time: the user is still active (`is_active`), still unmarked, their own stamp is still older than the cutoff, their address is still the one the notice went to, and `HasActiveConnection` is still false; only then `markForDeletion`. `ReleaseInactiveMarks` does the mirror image per listed mark: `LockRow`, `GetState`, and it releases only when the reason is still `inactive` and `MarkedAt` equals the listed value. A listing error is returned (and lands in `DeletionPassResult.Err`), never swallowed. `WipeExpired` passes each selected `MarkedAt` to `Wipe`; a failure logs `slog.Error("deletion: wipe failed", "user_id", ..., "err_type", ...)`. The job runs release, wipe, mark in that order and ends with `slog.Info("deletion: pass done", "released", n, "wiped", n, "failed", n, "marked", n)`. `StartPolling` follows `RateUpdater.StartPolling` (one goroutine, so runs never overlap) with a 2-minute first delay; it always starts (the release pass matters at any setting).
- [ ] **Step 4: Start it** in `cmd/econumo/main.go` next to `rateUpdater.StartPolling(ctx)` from `Built.Users`.
- [ ] **Step 5: Regression plan** (section 12), four items:

```
- [ ] Inactivity (ECONUMO_INACTIVE_USER_DAYS=1 on a test server, a user idle for 2+ days):
      after the daily pass the user gets ONE email, is signed out everywhere, and
      `user:list-pending-deletion` lists them with reason `inactive`.
- [ ] The same with a connected user who signed in today: the idle user is NOT marked.
- [ ] Two connected users both marked inactive; one signs in: after the next daily pass the
      other is no longer in `user:list-pending-deletion`.
- [ ] ECONUMO_USER_DELETION_DAYS=1: a user marked 2 days ago is gone after the daily pass;
      with the setting at 0 they stay until `user:delete`.
```

- [ ] **Step 6: Run, PASS; `go build ./...`. Commit.**

### Task 18: CLI

**Files:** Modify `internal/cli/user_commands.go`, `internal/cli/commands_test.go`, `internal/model/user_deletion.go`, `internal/user/deletion_wipe.go`; `CLAUDE.md` (the CLI command list).

**Interfaces:**
- Consumes: `AdminMarkDeleted`, `Wipe`, `PurgeDeleted`, `fullUser()`.
- Produces (`internal/user`): `func (s *Service) ListPendingDeletion(ctx context.Context) ([]model.PendingDeletion, error)` with `model.PendingDeletion{ID string; Email string; Reason string; MarkedAt time.Time; WipeAt *time.Time}` (`WipeAt` nil when `DeletionDays` is 0), and `func (s *Service) AdminDelete(ctx context.Context, email string) (WipeOutcome, error)` (`CodeUserDeletionNotMarked` for an unmarked user).

- [ ] **Step 1: Failing tests** in `commands_test.go` (pattern of the existing `Run([]string{...})` tests, capturing stdout):

```go
// user:mark-deleted a@x.test -> 0, "User a@x.test marked for deletion"; again -> 1
// user:list-pending-deletion -> a tab-separated table ID EMAIL REASON MARKED AUTO-WIPE;
//   AUTO-WIPE is "manual" with ECONUMO_USER_DELETION_DAYS unset, a date otherwise
// user:delete a@x.test -> 0, "User a@x.test deleted"; the row is gone
// user:delete unmarked@x.test -> 1, stderr names "not marked"
// user:delete on an inactive mark with an active connection -> 1,
//   "not deleted: a connected user is active; the mark was removed"
// user:purge-deleted -> 0 with "released N, deleted N"; 1 when any wipe failed, one line per
//   failure "failed <user id>: <error type>"; 1 as well when the pass itself errored (Err set)
```

- [ ] **Step 2: Run, FAIL. Step 3: implement** the four commands (summaries: `Mark a user for deletion: user:mark-deleted <email>`, `List users marked for deletion: user:list-pending-deletion`, `Delete a marked user now: user:delete <email>`, `Release and delete marked users whose time has come: user:purge-deleted`); `user:delete` and `user:purge-deleted` use `c.fullUser()`. **Step 4: run** `go test ./internal/cli/ -count=1`, PASS. **Step 5:** add the four names to the `user:` line of the CLI block in `CLAUDE.md`. **Commit.**

### Task 19: admin listener

**Files:** Modify `internal/admin/ports.go`, `service.go`, `context.go`; create `internal/admin/deleted.go`; modify `internal/admin/api/handler.go`, `routes.go`, `handler_test.go`, `internal/model/admin_dto.go`, `internal/server/glue_admin.go`, `internal/test/apiparity/admin_reachability_test.go`.

**Interfaces:**
- Consumes: `AuditLog.ListByEvent`, `DeletionStore.GetState`.
- Produces: `GET /admin/deleted-users?since=<datetime>&afterId=<audit id>&limit=<1..500>` answering `[]model.AdminDeletedUser{Id string "id"; UserId string "userId"; DeletedAt string "deletedAt"; Reason string "reason"; LifetimeAccess bool "lifetimeAccess"}` oldest first; `model.AdminUserView` gains `DeletionMarkedAt string "deletionMarkedAt"` and `DeletionReason string "deletionReason"`.

- [ ] **Step 1: Failing tests** in `handler_test.go` (existing harness): three `deleted` rows in one second, `limit=2` -> two rows; the next call with `since` = that second and `afterId` = the second row's `id` -> the third; missing `since` -> 400 with field `since`; a marked user's `user-context` carries both new fields, an unmarked one carries `""`; a wiped user's `user-context` is the existing 404.
- [ ] **Step 2: Run, FAIL. Step 3: implement** (handler modelled on `AffiliateUsers`; parse `details` JSON in the service). Wipes are serialized (`LockWipes`) and the `deleted` row is the last statement of each, written with whole-second `created_at`, so rows become visible in cursor order and a forward-only cursor is correct on one server. The endpoint's doc comment still states the spec's belt-and-braces contract (re-read a short overlap, skip ids already processed) for app servers whose clocks differ. Add `{http.MethodGet, "/admin/deleted-users?since=2026-01-01+00:00:00"}` to `TestAdminRoutesAreNotOnThePublicMux`. **Step 4:** `go test ./internal/admin/... ./internal/test/apiparity/ -run 'Admin' -count=1`. PASS. **Commit.**

---

## Phase 7: self-service and the SPA

### Task 20: request and confirm

**Files:** Create `internal/user/deletion_request.go`, `deletion_request_test.go`, `internal/model/user_deletion_dto.go`, `internal/user/api/deletion.go`; modify `internal/user/api/routes.go`, `internal/user/ports.go` (scopes), `internal/server/services.go` (limiter map, `Pinned`), `internal/web/middleware/auth.go` (`ReadonlyAllowedPaths`), `internal/shared/errs/codes.go`, `locales/*.json`, `internal/test/apiparity/catalogue_*.go` + goldens + `guard_test.go` (`minRoutes` 152), swagger docs.

**Interfaces:**
- Produces:

```go
type RequestUserDeletionRequest struct { Password string `json:"password"` }
type RequestUserDeletionResult struct{}
type ConfirmUserDeletionRequest struct { Code string `json:"code"` }   // Validate: not blank
type ConfirmUserDeletionResult struct{}
func (s *Service) RequestUserDeletion(ctx context.Context, userID vo.Id, req model.RequestUserDeletionRequest) (*model.RequestUserDeletionResult, error)
func (s *Service) ConfirmUserDeletion(ctx context.Context, userID vo.Id, req model.ConfirmUserDeletionRequest) (*model.ConfirmUserDeletionResult, error)
const RateScopeRequestUserDeletion = "request-user-deletion"
const RateScopeConfirmUserDeletion = "confirm-user-deletion"
```

- Routes: `POST /api/v1/user/request-user-deletion`, `POST /api/v1/user/confirm-user-deletion`, both authenticated, both in `ReadonlyAllowedPaths`.
- Errors reused: `CodeUserPasswordIncorrect`, `CodeUserVerificationCodeInvalid`, `CodeUserVerificationCodeExpired`.

- [ ] **Step 1: Failing tests.**

```go
// TestRequestUserDeletion_PasswordUser: wrong password -> ValidationError CodeUserPasswordIncorrect,
//   no email; right password -> one email whose body holds a 6-digit code; a row in
//   users_deletion_requests holding HashResetCode(code)
// TestRequestUserDeletion_Passwordless: Algorithm none, empty password -> code sent
// TestRequestUserDeletion_PasswordLoginDisabled: DisablePasswordLogin() and a user WITH a
//   password, empty password -> code sent (no password flow exists on that server)
// TestRequestUserDeletion_RateLimited: the 4th request in the window is a TooManyRequestsError
// TestConfirmUserDeletion_Marks: right code -> state.Reason "self_initiated"; the presenting
//   session no longer authenticates; no email was sent by the confirm; the request row is consumed
// TestConfirmUserDeletion_AddressChanged: a code was sent, then the address changed
//   (AdminChangeEmail) -> the code is refused with CodeUserVerificationCodeInvalid, not marked
// TestConfirmUserDeletion_Refusals: wrong code -> CodeUserVerificationCodeInvalid, not marked;
//   a code older than 10 minutes -> CodeUserVerificationCodeExpired, not marked;
//   the 6th attempt in the window -> TooManyRequestsError, not marked
// apiparity scenario "user_deletion": err:request with a wrong password, request, err:confirm
//   with a wrong code (owner); a "readonly" caller's request is NOT a 402
```

- [ ] **Step 2: Run, FAIL.** `go test ./internal/user/ -run 'UserDeletion' -count=1`
- [ ] **Step 3: Implement** on the model of `RequestEmailChange` / `ConfirmEmailChange` (same limiter helpers, `generatePasswordCode`, `HashResetCode`, the request saved under the row lock with the credentials generation). Password required exactly when `u.Algorithm != model.AlgorithmNone && !s.passwordLoginDisabled`. `ConfirmUserDeletion`: in one transaction `LockRow`, re-read the user and refuse when the request's `Email` is not the stored `users.email` any more (a code proves the mailbox it was sent to, and an email change does not bump the credentials generation), `Consume`, `markForDeletion(ctx, userID, model.DeletionReasonSelf)`. Handlers with swag blocks, `make swagger`.
- [ ] **Step 4: Regenerate the apiparity goldens, read the diff** (only the new scenario file appears). Run `go test ./internal/test/apiparity/ ./internal/test/i18ntest/ ./internal/user/... -count=1`. PASS. **Commit.**

### Task 21: `deletionMarkedAt` on connections and the Inbox row

**Files:** Modify `internal/connection/ports.go`, `read.go`, `internal/model/connection_dto.go`, `internal/server/glue_connection.go` (or the file holding `NewUserOwnerLookup`); goldens of apiparity (`connection_reads`, `connection_invite_flows`) and mcpparity (`reference_tools`, `lifecycle`); swagger; `web/src/api/dto/connection.ts`, create `web/src/features/inbox/DeletionNoticeSection.tsx` + `.test.tsx`, modify `useInbox.ts`, `InboxPage.tsx`, `locales/*.json`, `docs/regression-test-plan.md`.

**Interfaces:**
- Produces (Go): `connection.UserLookup` gains `DeletionMarkedAt(ctx context.Context, userID string) (*time.Time, error)`; `model.ConnectionResult` gains `DeletionMarkedAt string` with tag `json:"deletionMarkedAt"` (`""` when not marked).
- Produces (TS): `ConnectionDto.deletionMarkedAt: string`; `Inbox.markedConnections: ConnectionDto[]` (NOT added to `count`: the row has no action and may stay for weeks).
- `en` copy: `inbox.sharing.scheduled_for_deletion`: `{name}'s data is scheduled for deletion. The accounts and budgets they share with you will disappear, and transfers with their accounts will become corrections.`

- [ ] **Step 1: Failing tests.** Go (`internal/connection`): a marked connected user -> `DeletionMarkedAt` is the formatted time; unmarked -> `""`. Vitest `DeletionNoticeSection.test.tsx`: renders one row per connection with a non-empty `deletionMarkedAt`, showing the name and the copy; renders nothing for an empty list. `useInbox.test.tsx`: `markedConnections` holds only marked connections and `count` is unchanged by them. `InboxPage.test.tsx`: with a marked connection as the only item the page shows the notice and NOT the "All caught up" empty state.
- [ ] **Step 2: Run, FAIL** (`go test ./internal/connection/ -count=1`; `cd web && pnpm test src/features/inbox`).
- [ ] **Step 3: Implement.** In `InboxPage.tsx` the Sharing section renders when `invites.length > 0 || markedConnections.length > 0`, notices first, and the empty-state predicate (today `count === 0` with nothing skipped) also requires `markedConnections.length === 0`. The row is a card like the invite card (`UserAvatar`, text), no buttons. `useInbox` reads `useConnections()`.
- [ ] **Step 4: Regenerate apiparity and mcpparity goldens; read the diff** (only the added field). `make swagger`. `cd web && pnpm lint && pnpm exec tsc -b`.
- [ ] **Step 5: i18n + regression plan.** Add the key to all 11 catalogues. In section 11 add: `- [ ] 📱 A connected user is marked for deletion (user:mark-deleted): the partner's Inbox shows "<name>'s data is scheduled for deletion..." under Sharing without changing the Inbox badge count; it disappears when that user signs in.`
- [ ] **Step 6: Commit.**

### Task 22: "Delete my data" in Settings

**Files:** Create `web/src/features/settings/DeleteDataPage.tsx`, `DeleteDataPage.test.tsx`; modify `web/src/api/user.ts`, `web/src/api/client.ts`, `web/src/features/auth/LogoutPage.tsx`, `LoginPage.tsx`, `web/src/features/user/queries.ts`, `web/src/app/routes.tsx`, `web/src/app/router-pages.ts`, `web/src/features/settings/ProfilePage.tsx`, `web/src/lib/metrics.ts`, `locales/*.json`, `docs/regression-test-plan.md`, `CLAUDE.md`.

**Interfaces:**
- Consumes: Task 20 routes; `exportTransactionList(accountIds)` + `downloadBlob` (`web/src/features/transactions/ExportCsvDialog.tsx` shows the pair, and how it picks OWNED accounts); `useAccounts`; `getBillingUrl()`; `hasPassword` on the user DTO; `usePasswordLoginAvailable()` (what `ProfilePage.tsx` uses: it answers for the backend the user is signed in to, which `isPasswordLoginAllowed()` does not on a custom-backend setup); the local clean-up `LogoutPage.tsx` performs.
- Produces: `web/src/features/auth/localSignOut.ts` exporting `async function signOutLocally(opts?: { keepPendingWrites?: boolean; keepAppLock?: boolean }): Promise<void>` (the `removeToken` / `clearConfig` / `resetAnalyticsIdentity` / `clearPersistedQueryCache` / `clearPendingWrites` sequence, moved out of `LogoutPage.tsx`, which now calls it).
- Produces: `requestUserDeletion(password: string): Promise<void>`, `confirmUserDeletion(code: string): Promise<void>`; hooks `useRequestUserDeletion`, `useConfirmUserDeletion` (`meta: { handlesOwnErrors: true }`; confirm's `onSuccess` fires `trackEvent(METRICS.USER_DELETE_DATA)`); `METRICS.USER_DELETE_DATA = 'appUserDeleteData'`; `RouterPage.SETTINGS_DELETE_DATA = '/settings/profile/delete-data'`.
- `en` copy (namespace `user.delete_data`):
  - `menu_item`: `Delete my data`
  - `header`: `Delete my data`
  - `explain.what`: `This deletes your accounts with their transactions, your budgets, categories, payees, tags and settings.`
  - `explain.shared`: `People you share with lose the accounts and budgets you share with them. They keep their own data: transactions you entered in their accounts stay, with their descriptions, and transfers between your accounts and theirs stay as corrections that keep the description.`
  - `explain.cancel`: `Signing in again before your data is deleted cancels the deletion.`
  - `explain.subscription`: `Deleting your data does not cancel a subscription. Cancel it in billing first.`
  - `confirm.pending_writes`: `Some of your changes are still waiting to be sent. Wait until they are sent before you delete your data.`
  - `explain.billing_link`: `Open billing`
  - `form.password.label`: `Password`
  - `form.submit.label`: `Send confirmation code`
  - `confirm.information`: `We sent a confirmation code to your email.`
  - `confirm.export.done`: `Your transactions were downloaded. The file contains the transactions of your current accounts. Accounts you deleted earlier are not in it.`
  - `confirm.export.failed`: `The download of your transactions failed.`
  - `confirm.export.retry`: `Try the download again`
  - `confirm.action`: `Delete my data`
  - `confirm.export.none`: `You have no accounts of your own to export.`
  - `done.text`: `Your data will be deleted within a month. Sign in before then to keep it.` (shown on the login page, key `auth.login.deletion_scheduled`)

- [ ] **Step 1: Failing tests** (`DeleteDataPage.test.tsx`, msw as in `ChangeEmailPage.test.tsx`):

```tsx
// password user: submitting without a password shows "Required field"; with one, POSTs
//   {password} and shows the code phase
// passwordless user (fixture hasPassword false): no password field; the request body is {password: ''}
// password login unavailable on the signed-in backend (usePasswordLoginAvailable false) while the
//   user has a password: no password field either
// entering the code phase triggers ONE GET export-transaction-list whose accountId lists exactly
//   the accounts the user OWNS (a shared account in the fixture is not in it), and only after
//   the account list has loaded; a 500 from it shows the failed text and a retry button, and
//   Confirm is still enabled
// a user who owns no account: NO export request is made (a blank accountId means "every
//   accessible account" to the server) and the "no accounts" text is shown
// confirm POSTs {code}; on success the token and the persisted query cache are gone from
//   localStorage and the browser is sent to /login?reason=deletion-scheduled
// LoginPage with ?reason=deletion-scheduled shows the done text
// with one change in the pendingWrites queue: Confirm is disabled and the pending-writes text
//   is shown; with the queue empty it is enabled
// interrupted confirm: localStorage['econumo.deletionCleanup'] holds the time the confirm was
//   sent; a 400 on the confirm removes it; completed clean-up removes it; a 401 on any request
//   within 10 minutes of that time runs signOutLocally() and lands on
//   /login?reason=deletion-scheduled (client.ts interceptor test); a 401 later than 10 minutes
//   removes the flag and is handled as an ordinary expired session; a successful response to
//   some OTHER request does not touch the flag
// a 400 on confirm shows the server message and stays on the code phase
// offline (onlineManager.setOnline(false)): both submit buttons are disabled
// the billing link is rendered only when window.econumoConfig.BILLING_URL is set
```

- [ ] **Step 2: Run, FAIL.** `cd web && pnpm test src/features/settings/DeleteDataPage.test.tsx`
- [ ] **Step 3: Implement** the page on the skeleton of `ChangeEmailPage.tsx` (`SettingsShell`, phases `request | confirm`, plain `Input type="password"`, the 6-digit code `Input`), and a destructive `Link` row at the end of the Security group of `ProfilePage.tsx` styled like the debts danger zone (`border-destructive/30 bg-destructive/5`). The route joins the settings routes in `routes.tsx`. The download runs once when the confirm phase is shown AND the account list has loaded; no resend button (requesting again from the first phase replaces the code). On a successful confirm the page does not wait for a click: it awaits `signOutLocally()` and then `window.location.assign('/login?reason=deletion-scheduled')`; the server has already revoked the session, and a closed tab must not leave the cached data behind. `LoginPage.tsx` renders the done text for that reason next to its existing `reason=expired` notice. There is no `done` phase on the page. Two guards around the confirm: (1) it is disabled while the offline write queue is not empty (the hook the sync indicator in `ApplicationLayout.tsx` reads): sign-out clears that queue and the CSV holds server rows only, so unsent changes would be lost even if the user later cancels by signing in; (2) the page stores the current time under `localStorage['econumo.deletionCleanup']` before sending the confirm and removes it on a refused confirm and after a completed clean-up; the 401 interceptor in `web/src/api/client.ts` checks it and, when it is younger than 10 minutes (the code's lifetime; a mark cannot commit later than that), runs `signOutLocally()` and goes to `/login?reason=deletion-scheduled` instead of its usual token-only handling, so a tab that died between the server's commit and the response still gets its local data cleared. An older flag is removed and ignored, which is what keeps an ordinary expired session, months later, from being mistaken for a deletion. Other requests' successes do not clear it: a request that authenticated before the mark can still succeed after it.
- [ ] **Step 4: Run** the test file, `pnpm test src/lib/metrics-coverage.test.ts src/lib/offline-coverage.test.ts src/lib/offline-notice-coverage.test.ts`, `pnpm lint`, `pnpm exec tsc -b`. PASS.
- [ ] **Step 5: i18n** in all 11 catalogues; `go test ./internal/test/i18ntest/ -count=1`.
- [ ] **Step 6: Regression plan** (section 12):

```
- [ ] 📱 **Delete my data**: Profile → Delete my data → password → code from the email →
      the CSV of your transactions downloads → Delete my data → the sign-in page says the data
      will be deleted.
      Every other device is signed out; signing in again cancels the deletion and nothing is lost.
- [ ] Delete my data, passwordless account (provider sign-in only): no password is asked.
- [ ] Delete my data: a wrong code is refused; the page stays on the code step.
- [ ] `user:delete <email>` after the above: the partner's shared accounts and budgets are gone,
      transfers with them show as "Original transaction deleted: ..." on the same dates, and the
      partner's balances are unchanged.
```

- [ ] **Step 7: `CLAUDE.md`.** Add a "User deletion" bullet list under Domain behaviours (mark then wipe; the three ways to mark; the household rule; the wipe's lock order and that every cross-feature step is a method on the owning feature sequenced in `glue_user_wipe.go`; `users_audit`; the schema guard `internal/test/wipeguard`), the three env vars under Configuration, and the two routes to the read-only allowlist sentence. Keep it under 20 lines.
- [ ] **Step 8: Commit.**

---

## Open items

Decided against or left for later while the plan was reviewed; none blocks the tasks above.

- **"Your data will be deleted within a month"** on the sign-in page does not follow `ECONUMO_USER_DELETION_DAYS` (0 by default, or any other number). Raised by both reviewers in every round; kept as the product owner decided.
- **Lifetime re-grant on an email change.** An existing account that moves to a deleted lifetime address (`ConfirmEmailChange`, a provider's verified address change, `user:change-email`) is not re-granted. Those paths replace the address without saving the aggregate, and the feature is about registering again. The operator can run `user:verify-email <email>`.
- **Stray rows from a request in flight.** A request that authenticated before the mark can still insert an `idempotency_keys` or `oauth_states` row after the wipe's sweep. Ids only; they expire by age (90 days, 10 minutes). No foreign key was added (a SQLite table rebuild for no exposure).
- **A connection made after an inactive mark** is not handled at that moment; the daily release and the check inside the wipe see it.
- **The billing cursor assumes the server clock does not go back.** Wipes are serialized and timestamps are whole seconds; a clock set back is covered only by the caller re-reading an overlap.
- **The Inbox row about a partner's pending deletion does not add to the Inbox badge.**
- **Elements left by a revoke of an older version.** On an upgraded database a former member can still have category or tag elements in a budget they no longer have an access row on (an earlier `RevokeAccess` did not remove them). The wipe finds budgets by ownership and access rows, so those elements stay, pointing at a category that is gone. They are orphans today already.
- **A partner's import link to a replaced transfer.** If a partner's import had adopted a transfer that the wipe replaces by a correction, the link ends up pointing at nothing, so their import history shows that bank movement as deleted and offers to import it again.
- **Inactivity marking does not lock the connected users' rows** (the wipe does). A partner who signs in during the seconds a candidate is being marked leaves the candidate marked until the next daily release. Locking there was left out: two connected candidates marked at once would lock each other's rows in opposite order.
- **Verification and reset codes are not bound to an address** (the deletion code is). Combined with the re-grant this matters only if an unverified account's address is changed by the operator while a code is outstanding.
- **`DeleteConnection` unwinds account grants before budgets**, the wipe the other way round.
- **The lock order is proven by Task 14's two-connection tests, not on paper.** If one of them deadlocks or hangs, the stop rule applies.
