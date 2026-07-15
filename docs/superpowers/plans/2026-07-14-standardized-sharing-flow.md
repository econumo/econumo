# Standardized Sharing Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Account sharing adopts budget's grant → pending → accept/decline handshake (with folder choice on accept), and the SPA gains a "Sharing requests" sidebar button + modal as the single recipient-side surface.

**Architecture:** Add `is_accepted` to `accounts_access` (existing rows grandfathered accepted). Move account-access use cases from the `connection` feature into the `account` feature with four new endpoints mirroring budget's (`grant-access`/`accept-access`/`decline-access`/`revoke-access`); the old `/connection/set-account-access` + `revoke-account-access` routes are removed. Every consumer of `accounts_access` treats an unaccepted row as no-access, except `get-account-list`, which carries the recipient's pending account so the UI can offer accept/decline. The frontend derives a pending-invite count from the cached account/budget lists (no new endpoint, no poll).

**Tech Stack:** Go (stdlib HTTP, sqlc for SQLite+PostgreSQL), React 19 + TypeScript + TanStack Query + vitest/msw.

**Spec:** `docs/superpowers/specs/2026-07-14-standardized-sharing-flow-design.md`

## Global Constraints

- Work from the repo root (a git worktree). All commands run there unless a `cd web` is shown.
- Wire contract is frozen (CLAUDE.md): envelope shapes, datetime layout `"2006-01-02 15:04:05"`, int `0`/`1` flags (`isAccepted` follows `isArchived`), exact validation strings (`"This value should not be blank."` code `IS_BLANK_ERROR`, `"Access denied"`, etc.).
- SQL comments in `internal/infra/storage/sqlc/query/**/*.sql` must be **ASCII only** — an em dash mangles sqlc v1.30's sqlite codegen. Use `-` and `--`.
- After editing any query `.sql`: regenerate with `go generate ./internal/infra/storage/sqlc/...` (runs `sqlc generate`). Never hand-edit `gen/`.
- After changing routes or `@` swag annotations: `make swagger` (also run by `make go-build`). `make go-test` fails if committed docs are stale.
- Golden files: regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, then **inspect the diff** — every change must be an expected behavior change. Never hand-edit a golden.
- `make go-test` enforces the coverage gate (min 72%) — new use cases need tests.
- Comments: only non-obvious rationale (CLAUDE.md "Comments — write sparingly"); swag `// @…` blocks are exempt and required on handlers.
- Commit after each task with a conventional message.
- Known pre-existing failure: `web/src/features/transactions/ImportCsvDialog.test.tsx` fails on main — not yours to fix; run frontend tests on specific files or ignore that one failure.

---

### Task 1: `is_accepted` column — schema, queries, model, fixture

Adds the column (grandfathering existing rows), regenerates sqlc, threads `IsAccepted` through the domain model and the connection repo, and makes test fixtures explicit about acceptance. **No behavior change yet** — nothing reads the flag.

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260714000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260714000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/connection.sql`
- Modify: `internal/infra/storage/sqlc/query/pgsql/connection.sql` (same query names, `$N` placeholders)
- Modify: `internal/model/connection.go`
- Modify: `internal/model/connection_test.go`
- Modify: `internal/connection/repo/repo.go`
- Modify: `internal/test/fixture/entities.go`
- Test: `internal/connection/repo/repo_integration_test.go`

**Interfaces:**
- Produces: `model.AccountAccess.IsAccepted bool` field; `(*model.AccountAccess).Accept(now time.Time)`; generated query `ListPendingReceivedAccountAccess(userID)`; fixture methods `Builder.AccountAccess(accountID, userID, role)` (now writes `is_accepted=1`) and new `Builder.AccountAccessPending(accountID, userID string, role int)`.

- [ ] **Step 1: Write the migrations**

`internal/infra/storage/migrations/sqlite/20260714000000.sql`:

```sql
-- Account sharing handshake: new grants start pending (is_accepted = 0) and
-- become effective on accept. Rows existing before this migration predate the
-- handshake and are grandfathered as accepted.
ALTER TABLE accounts_access ADD COLUMN is_accepted BOOLEAN DEFAULT '0' NOT NULL;
UPDATE accounts_access SET is_accepted = 1;
```

`internal/infra/storage/migrations/pgsql/20260714000000.sql`:

```sql
-- Account sharing handshake: new grants start pending (is_accepted = false) and
-- become effective on accept. Rows existing before this migration predate the
-- handshake and are grandfathered as accepted.
ALTER TABLE accounts_access ADD COLUMN is_accepted BOOLEAN DEFAULT '0' NOT NULL;
UPDATE accounts_access SET is_accepted = true;
```

- [ ] **Step 2: Update the accounts_access queries (both engines)**

In `internal/infra/storage/sqlc/query/sqlite/connection.sql`:
- Add `is_accepted` to the SELECT column list of `GetAccountAccess`, `ListReceivedAccountAccess`, `ListAccountAccessByAccount`, `ListIssuedAccountAccess` (the issued query selects `aa.is_accepted`).
- Replace `UpsertAccountAccess` with:

```sql
-- name: UpsertAccountAccess :exec
INSERT INTO accounts_access (account_id, user_id, role, is_accepted, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id, user_id) DO UPDATE SET
    role        = excluded.role,
    is_accepted = excluded.is_accepted,
    updated_at  = excluded.updated_at;
```

- Append a new query:

```sql
-- name: ListPendingReceivedAccountAccess :many
-- Pending grants TO this user (invites awaiting acceptance). Ordered so both
-- engines return identical row order.
SELECT account_id, user_id, role, is_accepted, created_at, updated_at
FROM accounts_access
WHERE user_id = ? AND is_accepted = 0
ORDER BY created_at, account_id;
```

Mirror all of the above in `internal/infra/storage/sqlc/query/pgsql/connection.sql` with `$1..$6` placeholders and `is_accepted = false` in the pending filter.

- [ ] **Step 3: Regenerate sqlc**

Run: `go generate ./internal/infra/storage/sqlc/...`
Expected: `gen/sqlite/models.go` and `gen/pgsql/models.go` show `AccountsAccess` with `IsAccepted bool`; new `ListPendingReceivedAccountAccess` methods exist. `go build ./...` now FAILS in `internal/connection/repo` (UpsertAccountAccessParams changed) — expected, fixed in Step 5.

- [ ] **Step 4: Extend the domain model + write its failing test**

In `internal/model/connection.go`, change `AccountAccess` and add `Accept` (mirror `BudgetAccess.Accept` at `internal/model/budget.go:93`):

```go
type AccountAccess struct {
	AccountID  vo.Id
	UserID     vo.Id
	Role       Role
	IsAccepted bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewAccountAccess creates a fresh PENDING grant (CreatedAt == UpdatedAt == now).
func NewAccountAccess(accountID, userID vo.Id, role Role, now time.Time) *AccountAccess {
	return &AccountAccess{AccountID: accountID, UserID: userID, Role: role, IsAccepted: false, CreatedAt: now, UpdatedAt: now}
}

// Accept marks a pending grant accepted, bumping UpdatedAt only on the transition.
func (a *AccountAccess) Accept(now time.Time) {
	if !a.IsAccepted {
		a.IsAccepted = true
		a.UpdatedAt = now
	}
}
```

Add to `internal/model/connection_test.go`:

```go
func TestAccountAccess_Accept(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	a := model.NewAccountAccess(vo.MustParseId("aaaa1111-0000-0000-0000-0000000000a1"), vo.MustParseId("22222222-2222-2222-2222-222222222222"), model.RoleUser, now)
	if a.IsAccepted {
		t.Fatal("new grant must be pending")
	}
	a.Accept(later)
	if !a.IsAccepted || !a.UpdatedAt.Equal(later) {
		t.Fatalf("accept: IsAccepted=%v UpdatedAt=%v", a.IsAccepted, a.UpdatedAt)
	}
	a.Accept(later.Add(time.Hour))
	if !a.UpdatedAt.Equal(later) {
		t.Fatal("second accept must not bump UpdatedAt")
	}
}
```

(Match the existing test file's import style/helpers — if it uses a different id-parsing helper, follow it.)

- [ ] **Step 5: Map the column in the connection repo**

In `internal/connection/repo/repo.go`:
- `Save` (`upsertParams` literal): add `IsAccepted: a.IsAccepted,` after `Role:`.
- `hydrate`: add `IsAccepted: row.IsAccepted,` to the returned struct.

- [ ] **Step 6: Fixture — accepted by default, pending variant**

In `internal/test/fixture/entities.go`, update `AccountAccess` (line ~278) to insert `is_accepted = 1` and add a pending variant:

```go
// AccountAccess grants a user ACCEPTED access to an account (accounts_access).
// role is the stored int: admin=0, user=1, guest=2.
func (b *Builder) AccountAccess(accountID, userID string, role int) {
	// keep the existing now/time handling of the current method
	b.insert(`INSERT INTO accounts_access (account_id, user_id, role, is_accepted, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`,
		accountID, userID, role, now, now)
}

// AccountAccessPending grants a user a PENDING (not yet accepted) grant.
func (b *Builder) AccountAccessPending(accountID, userID string, role int) {
	b.insert(`INSERT INTO accounts_access (account_id, user_id, role, is_accepted, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
		accountID, userID, role, now, now)
}
```

(Adapt the body to the file's actual timestamp variables — read the current method first.)

- [ ] **Step 7: Round-trip integration test for the flag**

Add to `internal/connection/repo/repo_integration_test.go` (reuse its `newRepo`/`newAccess` helpers; `newAccess` needs no change — construct via `model.NewAccountAccess` for pending):

```go
func TestConnectionRepo_IsAcceptedRoundTrip(t *testing.T) {
	repo, _, _ := newRepo(t)
	ctx := context.Background()
	pending := model.NewAccountAccess(vo.MustParseId(acctA), vo.MustParseId(userB), model.RoleUser, fixedTime)
	if err := repo.Save(ctx, pending); err != nil {
		t.Fatalf("Save pending: %v", err)
	}
	got, err := repo.Get(ctx, vo.MustParseId(acctA), vo.MustParseId(userB))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.IsAccepted {
		t.Fatal("stored grant must round-trip as pending")
	}
	got.Accept(fixedTime.Add(time.Hour))
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("Save accepted: %v", err)
	}
	got2, _ := repo.Get(ctx, vo.MustParseId(acctA), vo.MustParseId(userB))
	if !got2.IsAccepted {
		t.Fatal("accept must persist")
	}
}
```

- [ ] **Step 8: Verify**

Run: `go build ./... && go test ./internal/model/ ./internal/connection/... ./internal/test/fixture/ && go test ./internal/test/apiparity/`
Expected: all PASS (goldens unchanged — the wire shape hasn't moved; fixture rows are accepted, matching pre-migration behavior).

- [ ] **Step 9: Commit**

```bash
git add -A && git commit -m "feat(account): add is_accepted to accounts_access (grandfathered accepted)"
```

---

### Task 2: `sharedAccess[].isAccepted` on the wire

Expose the flag in the account embed so the sharer sees "awaiting acceptance" and the recipient's client can detect pending. Goldens change — every account payload with a `sharedAccess` entry gains the key.

**Files:**
- Modify: `internal/model/account_view.go` (SharedAccessView)
- Modify: `internal/model/account_dto.go` (SharedAccess)
- Modify: `internal/account/usecase.go:226-243` (sharedAccessFor)
- Modify: `internal/server/glue_connection.go:104-114` (ConnectionSharedAccessLookup.ListByAccount)
- Test: `internal/account/api/shared_access_test.go`

**Interfaces:**
- Produces: `model.SharedAccess{User UserResult; Role string; IsAccepted int}` (JSON `isAccepted`, int 0/1); `model.SharedAccessView{UserID, Role string; IsAccepted bool}`.

- [ ] **Step 1: Write the failing test**

In `internal/account/api/shared_access_test.go`, add a test that seeds an ACCEPTED grant via `f.AccountAccess(...)` and a PENDING grant via `f.AccountAccessPending(...)` (two grantee users), calls `GET /api/v1/account/get-account-list` as the owner, and asserts the two `sharedAccess` entries carry `"isAccepted": 1` and `"isAccepted": 0` respectively. Follow the file's existing harness/decode helpers exactly (read the file first — it already asserts `sharedAccess` shapes).

- [ ] **Step 2: Run it**

Run: `go test ./internal/account/api/ -run SharedAccess -v`
Expected: FAIL (no `isAccepted` key in the response).

- [ ] **Step 3: Implement**

`internal/model/account_view.go`:

```go
type SharedAccessView struct {
	UserID     string
	Role       string
	IsAccepted bool
}
```

`internal/model/account_dto.go`:

```go
// SharedAccess is one accounts_access grant on the account: the granted user
// (id, avatar, name), the role alias (admin/user/guest), and whether the
// grant has been accepted (int 0/1, like isArchived).
type SharedAccess struct {
	User       UserResult `json:"user"`
	Role       string     `json:"role"`
	IsAccepted int        `json:"isAccepted"`
}
```

`internal/account/usecase.go` `sharedAccessFor` loop body:

```go
	for _, g := range grants {
		u, uerr := s.resolveOwner(ctx, cache, g.UserID)
		if uerr != nil {
			return nil, uerr
		}
		accepted := 0
		if g.IsAccepted {
			accepted = 1
		}
		out = append(out, model.SharedAccess{User: u, Role: g.Role, IsAccepted: accepted})
	}
```

`internal/server/glue_connection.go` `ListByAccount`:

```go
		out[i] = model.SharedAccessView{UserID: g.UserID.String(), Role: g.Role.Alias(), IsAccepted: g.IsAccepted}
```

- [ ] **Step 4: Run the test, regenerate goldens, inspect**

Run: `go test ./internal/account/api/ -run SharedAccess -v` → PASS.
Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` then `git diff internal/test/apiparity/testdata/golden/`
Expected diff: ONLY additions of `"isAccepted": 1` inside existing `sharedAccess` entries (the apiparity fixture's grant is accepted). Any other change = stop and investigate.

- [ ] **Step 5: Full check + commit**

Run: `go test ./internal/account/... ./internal/test/apiparity/`
Expected: PASS.

```bash
git add -A && git commit -m "feat(account): expose isAccepted in sharedAccess embed"
```

---

### Task 3: Unaccepted grant = no access (everywhere except the recipient's list)

Tighten every `accounts_access` consumer to accepted-only, and add the one deliberate exception: `get-account-list` (and the shared list builder) appends the recipient's pending accounts as inert entries.

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/accounts.sql` (ListAvailableAccounts, CountAvailableAccounts)
- Modify: `internal/infra/storage/sqlc/query/pgsql/accounts.sql` (same two)
- Modify: `internal/infra/storage/sqlc/query/sqlite/account_balance.sql` (ListAccountBalancesForUser)
- Modify: `internal/infra/storage/sqlc/query/pgsql/account_balance.sql`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/category_read.sql`, `tag_read.sql`, `payee_read.sql` (the shared-owner subquery)
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/transaction_export.sql`
- Modify: `internal/connection/repo/adapters.go` (HasWriteGrant, HasAdminGrant)
- Modify: `internal/server/glue_connection.go` (ConnectionAccessRevoker.HasAccess — pending counts as access here so delete-account keeps working as "decline"; see step 4 note)
- Modify: `internal/account/repository.go` (AccountStore gains ListPendingReceived — see step 5)
- Modify: `internal/account/repo/repo.go` + its engine shims (implement ListPendingReceived)
- Modify: `internal/account/usecase.go` (buildAccountList appends pending)
- Modify: `internal/account/ports.go` (SharedAccessLookup unchanged; no port change here)
- Test: `internal/account/api/shared_access_test.go`, `internal/transaction/api/` permissions tests, `internal/connection/repo/access_matrix_integration_test.go`

**Interfaces:**
- Consumes: `AccountAccessPending` fixture, `IsAccepted` model field (Task 1).
- Produces: SQL-level invariant "pending row behaves like no row" for ListAvailableAccounts/Count/Balances/category/tag/payee sharing/export; `HasWriteGrant`/`HasAdminGrant` return false for pending; pending accounts appear at the END of `get-account-list` items with `folderId: null`, `position: 0`, `balance: "0"`.

- [ ] **Step 1: Write the failing tests**

Backend tests that pin the new semantics, using `AccountAccessPending`:

1. In `internal/account/api/shared_access_test.go`: as the GRANTEE with a pending grant, `get-account-list` DOES include the shared account, with `folderId: null`, `position: 0`, `balance: "0"`, and my `sharedAccess` entry `isAccepted: 0`; after switching the fixture to `AccountAccess` (accepted, seeded into a folder+options like the apiparity fixture does), the account carries real folder/position.
2. In `internal/transaction/api/` (find the existing permissions test file — `ls internal/transaction/api/*_test.go` — and follow its harness): a user with a PENDING `user`-role grant gets the not-available validation error on create-transaction against that account, and cannot list its transactions (the account is absent from the visible set); with an ACCEPTED grant both succeed (that case already exists — extend it).
3. In `internal/connection/repo/access_matrix_integration_test.go`: `HasWriteGrant` and `HasAdminGrant` return false for a pending admin grant, true once accepted.

Write these first; they should fail.

- [ ] **Step 2: SQL filters (both engines)**

Pattern — sqlite (literal `1`), pgsql (literal `true`). Apply to each file:

`accounts.sql` (both queries):

```sql
WHERE a.is_deleted = 0 AND (a.user_id = ? OR (aa.user_id = ? AND aa.is_accepted = 1))
```

pgsql:

```sql
WHERE a.is_deleted = false AND (a.user_id = $1 OR (aa.user_id = $1 AND aa.is_accepted = true))
```

`account_balance.sql` `ListAccountBalancesForUser`: same WHERE rewrite.

`category_read.sql` / `tag_read.sql` / `payee_read.sql` shared-owner subquery (sqlite shown; pgsql analogous):

```sql
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = ? AND aa.is_accepted = 1
```

`transaction_export.sql`:

```sql
WHERE a.is_deleted = 0 AND (a.user_id = ? OR (aa.user_id = ? AND aa.is_accepted = 1));
```

Then: `go generate ./internal/infra/storage/sqlc/...` (param counts are unchanged — the same placeholder is reused — so no Go signatures move).

- [ ] **Step 3: Go-side checks**

`internal/connection/repo/adapters.go` — `HasWriteGrant`:

```go
	role := grant.Role
	return grant.IsAccepted && (role == model.RoleAdmin || role == model.RoleUser), nil
```

`HasAdminGrant` (same file, below): add the `grant.IsAccepted &&` guard to its role comparison.

- [ ] **Step 4: delete-account for a pending recipient stays a self-removal**

`ConnectionAccessRevoker.HasAccess` (glue_connection.go) intentionally keeps counting ANY row (pending included): a pending recipient "deleting" the account from their side is equivalent to declining, and `RevokeOwnAccess` unwinds nothing extra (a pending grant has no folder/options rows). Add one sentence of comment there stating this. No code change.

- [ ] **Step 5: Pending accounts appended to the account list**

`internal/account/repository.go` — add to `AccountStore`:

```go
	// ListPendingReceived returns the user's pending (not yet accepted) received
	// grants, ordered by grant creation.
	ListPendingReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)
```

Implement in `internal/account/repo/repo.go` + `sqlite.go` + `pgsql.go`, following the file's existing querier pattern, calling generated `ListPendingReceivedAccountAccess` and hydrating into `*model.AccountAccess` (copy the hydrate approach from `internal/connection/repo/repo.go:167-179`, including `IsAccepted: row.IsAccepted`).

`internal/account/usecase.go` — at the end of `buildAccountList`, after the `reversed` block, append:

```go
	pending, err := s.accounts.ListPendingReceived(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, g := range pending {
		acct, gerr := s.accounts.GetByID(ctx, g.AccountID)
		if gerr != nil {
			return nil, gerr
		}
		item, berr := s.buildAccountResult(ctx, userID, acct, "0", nil, nil, cache)
		if berr != nil {
			return nil, berr
		}
		items = append(items, item)
	}
	return items, nil
```

Check `buildAccountResult` tolerates `foldersSorted == nil` and `memberships == nil` (it does: the folder loop just doesn't match, `folderID` stays nil; `GetPosition` returns 0 for a missing row). The pending account is invisible to `ListAvailable` (Step 2), so no duplicate.

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/account/... ./internal/transaction/... ./internal/connection/... ./internal/category/... ./internal/tag/... ./internal/payee/...`
Expected: the Step-1 tests PASS; pre-existing tests still PASS (all fixtures write accepted grants).

- [ ] **Step 7: Goldens + commit**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff internal/test/apiparity/testdata/golden/`
Expected: NO golden changes (apiparity fixture grants are accepted). If a golden moved, investigate before proceeding.

```bash
git add -A && git commit -m "feat: unaccepted account grants carry no access; pending accounts ride the list"
```

---

### Task 4: Account feature owns its access persistence

Give the account package an `AccessStore` role interface and implementation so the use cases in Task 5 need no connection imports. The generated queries are shared; only the repo wrapper is new.

**Files:**
- Modify: `internal/account/repository.go` (add AccessStore; move ListPendingReceived from AccountStore into it)
- Create: `internal/account/repo/access.go`
- Test: `internal/account/repo/access_integration_test.go`

**Interfaces:**
- Produces (package `account`):

```go
// AccessStore persists the per-account grants (accounts_access) plus the two
// per-user cleanup reads revocation needs. A missing grant returns an
// *errs.NotFoundError.
type AccessStore interface {
	Get(ctx context.Context, accountID, userID vo.Id) (*model.AccountAccess, error)
	Save(ctx context.Context, a *model.AccountAccess) error
	Delete(ctx context.Context, accountID, userID vo.Id) error
	ListByAccount(ctx context.Context, accountID vo.Id) ([]*model.AccountAccess, error)
	ListReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)
	ListPendingReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)
	ListIssued(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)
	// DeleteOption removes the user's accounts_options row for the account.
	DeleteOption(ctx context.Context, accountID, userID vo.Id) error
}
```

- Produces (package `accountrepo`): `NewAccessRepo(driver string, tx *backend.TxManager) *AccessRepo` implementing `account.AccessStore`.
- Note: `ListPendingReceived` will end up on `AccessStore` only. Task 3 put a copy on `AccountStore` (the service had no `access` field yet); in THIS task just ADD the new interface + repo (both copies coexist and compile); Task 5 switches `buildAccountList` to `s.access.ListPendingReceived` and deletes the `AccountStore` copy.

- [ ] **Step 1: Write the failing integration test**

`internal/account/repo/access_integration_test.go` — mirror `internal/connection/repo/repo_integration_test.go`'s setup (dbtest + fixture, two users, two accounts). Cover: Save→Get round-trip (pending), Accept→Save→Get, Delete→Get NotFound, ListByAccount, ListReceived vs ListPendingReceived (one accepted + one pending row: received returns both, pending returns only the pending one, ordered), ListIssued, DeleteOption removes a seeded `accounts_options` row (seed via `f.AccountOption(...)`, verify with a direct `db.Rebind` query or a second DeleteOption no-op — follow how the connection repo test asserts it).

Run: `go test ./internal/account/repo/ -run Access -v` → FAIL (types don't exist).

- [ ] **Step 2: Implement `internal/account/repo/access.go`**

One self-contained file (interface-per-engine shim, same style as `internal/connection/repo/repo.go`):

```go
// AccessRepo implements account.AccessStore over the shared accounts_access
// queries (generated once, in internal/infra/storage/sqlc/query/*/connection.sql).
package repo

import (
	"context"
	"database/sql"
	"errors"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	accessRow          = sqlitegen.AccountsAccess
	accessUpsertParams = sqlitegen.UpsertAccountAccessParams
)

type accessQuerier interface {
	GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error)
	UpsertAccountAccess(ctx context.Context, db backend.DBTX, p accessUpsertParams) error
	DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error
	ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error)
	ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	ListPendingReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error)
	DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error
}

type AccessRepo struct {
	tx *backend.TxManager
	q  accessQuerier
}

var _ appaccount.AccessStore = (*AccessRepo)(nil)

func NewAccessRepo(driver string, tx *backend.TxManager) *AccessRepo {
	switch driver {
	case "sqlite":
		return &AccessRepo{tx: tx, q: accessSqlite{}}
	case "postgresql":
		return &AccessRepo{tx: tx, q: accessPgsql{}}
	default:
		panic("accountrepo: unknown database driver " + driver)
	}
}

func (r *AccessRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *AccessRepo) Get(ctx context.Context, accountID, userID vo.Id) (*model.AccountAccess, error) {
	row, err := r.q.GetAccountAccess(ctx, r.db(ctx), accountID.String(), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("AccountAccess not found")
		}
		return nil, err
	}
	return hydrateAccess(row)
}

func (r *AccessRepo) Save(ctx context.Context, a *model.AccountAccess) error {
	return r.q.UpsertAccountAccess(ctx, r.db(ctx), accessUpsertParams{
		AccountID: a.AccountID.String(), UserID: a.UserID.String(),
		Role: a.Role.Int16(), IsAccepted: a.IsAccepted,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	})
}

func (r *AccessRepo) Delete(ctx context.Context, accountID, userID vo.Id) error {
	return r.q.DeleteAccountAccess(ctx, r.db(ctx), accountID.String(), userID.String())
}

func (r *AccessRepo) ListByAccount(ctx context.Context, accountID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListByAccount(ctx, r.db(ctx), accountID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

func (r *AccessRepo) ListReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListReceived(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

func (r *AccessRepo) ListPendingReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListPendingReceived(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

func (r *AccessRepo) ListIssued(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	rows, err := r.q.ListIssued(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAccessAll(rows)
}

func (r *AccessRepo) DeleteOption(ctx context.Context, accountID, userID vo.Id) error {
	return r.q.DeleteAccountOptionForUser(ctx, r.db(ctx), accountID.String(), userID.String())
}

func hydrateAccess(row accessRow) (*model.AccountAccess, error) {
	accountID, err := vo.ParseId(row.AccountID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.AccountAccess{AccountID: accountID, UserID: userID, Role: model.Role(row.Role),
		IsAccepted: row.IsAccepted, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func hydrateAccessAll(rows []accessRow) ([]*model.AccountAccess, error) {
	out := make([]*model.AccountAccess, 0, len(rows))
	for _, row := range rows {
		a, err := hydrateAccess(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

type accessSqlite struct{}

func (accessSqlite) GetAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) (accessRow, error) {
	return sqlitegen.New(db).GetAccountAccess(ctx, sqlitegen.GetAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (accessSqlite) UpsertAccountAccess(ctx context.Context, db backend.DBTX, p accessUpsertParams) error {
	return sqlitegen.New(db).UpsertAccountAccess(ctx, p)
}
func (accessSqlite) DeleteAccountAccess(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return sqlitegen.New(db).DeleteAccountAccess(ctx, sqlitegen.DeleteAccountAccessParams{AccountID: accountID, UserID: userID})
}
func (accessSqlite) ListByAccount(ctx context.Context, db backend.DBTX, accountID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListAccountAccessByAccount(ctx, accountID)
}
func (accessSqlite) ListReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListReceivedAccountAccess(ctx, userID)
}
func (accessSqlite) ListPendingReceived(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListPendingReceivedAccountAccess(ctx, userID)
}
func (accessSqlite) ListIssued(ctx context.Context, db backend.DBTX, userID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListIssuedAccountAccess(ctx, userID)
}
func (accessSqlite) DeleteAccountOptionForUser(ctx context.Context, db backend.DBTX, accountID, userID string) error {
	return sqlitegen.New(db).DeleteAccountOptionForUser(ctx, sqlitegen.DeleteAccountOptionForUserParams{AccountID: accountID, UserID: userID})
}

type accessPgsql struct{}
```

For `accessPgsql`, write the same eight methods against `pgsqlgen.New(db)`, converting rows/params by whole-struct literal exactly as `internal/connection/repo/pgsql.go` does for the same queries (open that file and copy its conversion style — the generated shapes are field-identical). If the generated param struct names differ between engines, follow what `go build` tells you.

IMPORTANT — import cycle check: `internal/account/repo` importing `appaccount "internal/account"` must not cycle. Check how `internal/account/repo/repo.go` declares its interface-compliance assertion first; if it does NOT import the parent package (asserts elsewhere), drop the `var _ appaccount.AccessStore` line and the import, and instead add the assertion in `internal/account/repo/access_integration_test.go` (`var _ account.AccessStore = (*accountrepo.AccessRepo)(nil)`).

Also move the `ListPendingReceived` interface method: it stays on `AccountStore` for now (Task 3 put it there and `buildAccountList` uses `s.accounts`); Task 5 rehomes the call to `s.access` and deletes it from `AccountStore`.

- [ ] **Step 3: Run the tests**

Run: `go test ./internal/account/repo/ -run Access -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(account): AccessStore repo for accounts_access"
```

---

### Task 5: Account access use cases (grant / accept / decline / revoke)

The service methods, DTOs, and rewiring: the account feature becomes self-sufficient (no `SharedAccessLookup`/`AccessRevoker` ports, no connection glue for its own data).

**Files:**
- Modify: `internal/model/account_dto.go` (three new request/result DTO pairs; the revoke pair moves here in Task 7)
- Create: `internal/account/access.go`
- Modify: `internal/account/usecase.go` (Service struct + NewService + sharedAccessFor + buildAccountList pending source)
- Modify: `internal/account/repository.go` (drop ListPendingReceived from AccountStore)
- Modify: `internal/account/repo/repo.go` + engine shims (drop the AccountStore ListPendingReceived added in Task 3 — it lives on AccessRepo now)
- Modify: `internal/account/ports.go` (delete SharedAccessLookup + AccessRevoker)
- Modify: `internal/account/delete.go` (native non-owner branch)
- Modify: `internal/server/server.go` (wiring)
- Modify: `internal/server/glue_connection.go` (delete ConnectionSharedAccessLookup + ConnectionAccessRevoker)
- Modify: `internal/account/api/harness_test.go` + any account test constructing the service (follow compile errors)
- Test: `internal/account/access_test.go` (new)

**Interfaces:**
- Consumes: `account.AccessStore` (Task 4), `s.resolveAccountFolder` (`internal/account/create.go:185`), `s.positions.MaxPosition/SavePosition`, `s.memberships.AddAccount/RemoveAccount`, `s.folders`/`s.memberships` role interfaces.
- Produces (all on `*account.Service`; shapes match `endpoint.Handle`):
  - `GrantAccess(ctx, userID vo.Id, req model.GrantAccountAccessRequest) (*model.GrantAccountAccessResult, error)`
  - `AcceptAccess(ctx, userID vo.Id, req model.AcceptAccountAccessRequest) (*model.AcceptAccountAccessResult, error)`
  - `DeclineAccess(ctx, userID vo.Id, req model.DeclineAccountAccessRequest) (*model.DeclineAccountAccessResult, error)`
  - `RevokeAccess(ctx, userID vo.Id, req model.RevokeAccountAccessRequest) (*model.RevokeAccountAccessResult, error)`
  - `RevokeOwnAccess(ctx, userID, accountID vo.Id) error` (used by delete.go; later by nothing else)
  - `RevokeAccessBetween(ctx, a, b vo.Id) error` — runs on the CALLER's tx (no WithTx inside); consumed by connection in Task 7.
  - `NewService(repo Repository, folders FolderRepository, access AccessStore, currency CurrencyLookup, users UserLookup, tx port.TxRunner, ops port.OperationGuard, clock port.Clock) *Service` — note `shared`/`revoker` params GONE, `access` added third.

**Semantics (from the spec, pinned):**
- Grant: gate = owner or ACCEPTED admin grant (`requireOwnerAdmin`); no connectivity check (same as today). New grant → pending row only. Existing grant (pending or accepted) → `UpdateRole`, acceptance untouched. Result `{}`.
- Accept: gate = caller has a PENDING row, else `errs.NewAccessDenied("Access denied")` (a missing row is also AccessDenied). One tx: folder := `s.resolveAccountFolder(txCtx, userID, req.FolderId)` (blank folderId tolerated only when the user has no folders → auto-creates "General"; foreign folder → AccessDenied; unknown/blank with folders → the existing validation errors), `grant.Accept(now)`, save, `MaxPosition`+1 → `SavePosition`, `AddAccount(folder, account)`. Result `{}`.
- Decline: caller removes their OWN row, pending or accepted (mirrors budget `canDecline`); missing row → AccessDenied. Unwinds folders/options (no-ops for pending). Result `{}`.
- Revoke: gate = `requireOwnerAdmin`; missing grant → NotFound (today's behavior); unwind + delete. Result `{}`.

- [ ] **Step 1: DTOs**

In `internal/model/account_dto.go` append (Validate bodies copy the loop style of `SetAccountAccessRequest` at `internal/model/connection_dto.go:41-54`):

```go
// GrantAccountAccessRequest grants/updates a connected user's role on an owned
// account. New grants start pending (isAccepted 0).
type GrantAccountAccessRequest struct {
	AccountId string `json:"accountId"`
	UserId    string `json:"userId"`
	Role      string `json:"role"`
}
```

`Validate()`: NotBlank on accountId/userId/role (exact copy of the old SetAccountAccessRequest.Validate). `GrantAccountAccessResult struct{}`.

```go
// AcceptAccountAccessRequest accepts a pending grant. folderId picks where the
// account lands; blank is tolerated only when the user has no folders (a
// "General" folder is then created), same as create-account.
type AcceptAccountAccessRequest struct {
	AccountId string `json:"accountId"`
	FolderId  string `json:"folderId"`
}
```

`Validate()`: NotBlank on accountId only. `AcceptAccountAccessResult struct{}`.

```go
// DeclineAccountAccessRequest removes the caller's own grant (their side of
// the share), pending or accepted.
type DeclineAccountAccessRequest struct {
	AccountId string `json:"accountId"`
}
```

`Validate()`: NotBlank on accountId. `DeclineAccountAccessResult struct{}`.

In THIS task add only the three new pairs above. `RevokeAccountAccessRequest`/`RevokeAccountAccessResult` already exist in `connection_dto.go` and, being in the shared `model` package, are usable by the account feature as-is; they MOVE to `account_dto.go` (and `SetAccountAccessRequest`/`SetAccountAccessResult` get deleted) in Task 7, when the connection feature stops using them. That keeps every task compiling.

- [ ] **Step 2: Write the failing service tests**

`internal/account/access_test.go` — integration-style (dbtest + fixture + real repos), mirroring how `internal/connection/access_test.go` builds its service (read that file for the pattern). Construct the account service with the NEW constructor. Cover at minimum:

```go
func TestGrantAccess_CreatesPendingWithoutPlacement(t *testing.T)
// owner grants role "user" to userB; assert repo row IsAccepted false; assert
// userB has NO accounts_options row and NO folder membership (query via repos).

func TestGrantAccess_UpdateRoleKeepsAcceptance(t *testing.T)
// accepted grant (fixture.AccountAccess) + re-grant "admin" -> role changed, still accepted.

func TestGrantAccess_RequiresOwnerOrAcceptedAdmin(t *testing.T)
// userB with PENDING admin grant cannot grant to userC (AccessDenied);
// after accept, they can.

func TestAcceptAccess_PlacesIntoChosenFolder(t *testing.T)
// pending grant; userB accepts with their folder id -> row accepted, options
// position = max+1, membership in that folder; account now in userB's
// GetAccountList as a normal (non-pending) entry.

func TestAcceptAccess_ForeignFolderDenied(t *testing.T)
// accept with OWNER's folder id -> AccessDenied, grant still pending.

func TestAcceptAccess_NoFoldersCreatesGeneral(t *testing.T)
// userB has zero folders; accept with blank folderId -> a folder named
// "General" exists and contains the account.

func TestAcceptAccess_NoPendingDenied(t *testing.T)
// no row, and separately an already-accepted row -> AccessDenied both times.

func TestDeclineAccess_RemovesOwnRow(t *testing.T)
// pending -> declined -> row gone; accepted -> declined -> row + options +
// membership gone.

func TestRevokeAccess_OwnerRemovesGrant(t *testing.T)
// pending revoked by owner -> gone; accepted revoked -> unwound (options +
// membership).
```

Use `errs.AsNotFound` / type-assert `*errs.AccessDeniedError` the way neighboring tests do. Run: `go test ./internal/account/ -run 'Grant|Accept|Decline|Revoke' -v` → FAIL (methods missing).

- [ ] **Step 3: Implement `internal/account/access.go`**

```go
// Account-access use cases: the grant -> pending -> accept/decline handshake
// plus revocation. The wire semantics mirror the budget feature's
// grant/accept/decline/revoke contract.
package account

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// parseAccessID converts a primitive id string to a vo.Id, surfacing the
// standard invalid-UUID validation error.
func parseAccessID(field, s string) (vo.Id, error) {
	id, err := vo.ParseId(s)
	if err != nil {
		return vo.Id{}, errs.NewValidation("Validation failed", errs.FieldError{
			Key: field, Message: "This is not a valid UUID.", Code: "INVALID_UUID_ERROR",
		})
	}
	return id, nil
}

// requireOwnerAdmin gates grant/revoke: the caller owns the account or holds
// an ACCEPTED admin grant on it.
func (s *Service) requireOwnerAdmin(ctx context.Context, userID, accountID vo.Id) error {
	acct, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if acct.UserID.Equal(userID) {
		return nil
	}
	grant, err := s.access.Get(ctx, accountID, userID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return errs.NewAccessDenied("Access denied")
		}
		return err
	}
	if grant.IsAccepted && grant.Role == model.RoleAdmin {
		return nil
	}
	return errs.NewAccessDenied("Access denied")
}

func (s *Service) GrantAccess(ctx context.Context, userID vo.Id, req model.GrantAccountAccessRequest) (*model.GrantAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	affectedUserID, err := parseAccessID("userId", req.UserId)
	if err != nil {
		return nil, err
	}
	role, err := model.RoleFromAlias(req.Role)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwnerAdmin(ctx, userID, accountID); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		grant, gerr := s.access.Get(txCtx, accountID, affectedUserID)
		if gerr != nil {
			var nf *errs.NotFoundError
			if !errors.As(gerr, &nf) {
				return gerr
			}
			// New grant: pending, no placement -- the recipient places the
			// account when they accept.
			return s.access.Save(txCtx, model.NewAccountAccess(accountID, affectedUserID, role, now))
		}
		grant.UpdateRole(role, now)
		return s.access.Save(txCtx, grant)
	})
	if err != nil {
		return nil, err
	}
	return &model.GrantAccountAccessResult{}, nil
}

func (s *Service) AcceptAccess(ctx context.Context, userID vo.Id, req model.AcceptAccountAccessRequest) (*model.AcceptAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		grant, gerr := s.access.Get(txCtx, accountID, userID)
		if gerr != nil {
			var nf *errs.NotFoundError
			if errors.As(gerr, &nf) {
				return errs.NewAccessDenied("Access denied")
			}
			return gerr
		}
		if grant.IsAccepted {
			return errs.NewAccessDenied("Access denied")
		}
		folderID, ferr := s.resolveAccountFolder(txCtx, userID, req.FolderId)
		if ferr != nil {
			return ferr
		}
		grant.Accept(now)
		if serr := s.access.Save(txCtx, grant); serr != nil {
			return serr
		}
		max, perr := s.positions.MaxPosition(txCtx, userID)
		if perr != nil {
			return perr
		}
		if perr := s.positions.SavePosition(txCtx, accountID, userID, max+1, now); perr != nil {
			return perr
		}
		return s.memberships.AddAccount(txCtx, folderID, accountID)
	})
	if err != nil {
		return nil, err
	}
	return &model.AcceptAccountAccessResult{}, nil
}

func (s *Service) DeclineAccess(ctx context.Context, userID vo.Id, req model.DeclineAccountAccessRequest) (*model.DeclineAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if _, gerr := s.access.Get(txCtx, accountID, userID); gerr != nil {
			var nf *errs.NotFoundError
			if errors.As(gerr, &nf) {
				return errs.NewAccessDenied("Access denied")
			}
			return gerr
		}
		return s.unwindGrant(txCtx, accountID, userID)
	}); err != nil {
		return nil, err
	}
	return &model.DeclineAccountAccessResult{}, nil
}

func (s *Service) RevokeAccess(ctx context.Context, userID vo.Id, req model.RevokeAccountAccessRequest) (*model.RevokeAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	affectedUserID, err := parseAccessID("userId", req.UserId)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwnerAdmin(ctx, userID, accountID); err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// Load first so a missing grant surfaces NotFound before cleanup.
		if _, gerr := s.access.Get(txCtx, accountID, affectedUserID); gerr != nil {
			return gerr
		}
		return s.unwindGrant(txCtx, accountID, affectedUserID)
	}); err != nil {
		return nil, err
	}
	return &model.RevokeAccountAccessResult{}, nil
}

// RevokeOwnAccess drops the caller's own grant (the delete-account non-owner
// branch).
func (s *Service) RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if _, gerr := s.access.Get(txCtx, accountID, userID); gerr != nil {
			return gerr
		}
		return s.unwindGrant(txCtx, accountID, userID)
	})
}

// RevokeAccessBetween removes every grant shared between the two users, both
// directions. It runs on the CALLER's transaction context (delete-connection
// already holds one; opening another would savepoint).
func (s *Service) RevokeAccessBetween(ctx context.Context, a, b vo.Id) error {
	received, err := s.access.ListReceived(ctx, a)
	if err != nil {
		return err
	}
	for _, g := range received {
		acct, gerr := s.accounts.GetByID(ctx, g.AccountID)
		if gerr != nil {
			return gerr
		}
		if acct.UserID.Equal(b) {
			if uerr := s.unwindGrant(ctx, g.AccountID, g.UserID); uerr != nil {
				return uerr
			}
		}
	}
	issued, err := s.access.ListIssued(ctx, a)
	if err != nil {
		return err
	}
	for _, g := range issued {
		if g.UserID.Equal(b) {
			if uerr := s.unwindGrant(ctx, g.AccountID, g.UserID); uerr != nil {
				return uerr
			}
		}
	}
	return nil
}

// unwindGrant removes affectedUserID's view of the account (folder memberships
// + accounts_options) and the grant row, on the current (tx) context.
func (s *Service) unwindGrant(ctx context.Context, accountID, affectedUserID vo.Id) error {
	memberships, err := s.memberships.MembershipsByUser(ctx, affectedUserID)
	if err != nil {
		return err
	}
	for folderID, accountIDs := range memberships {
		for _, aid := range accountIDs {
			if aid == accountID.String() {
				fid, perr := vo.ParseId(folderID)
				if perr != nil {
					return perr
				}
				if rerr := s.memberships.RemoveAccount(ctx, fid, accountID); rerr != nil {
					return rerr
				}
				break
			}
		}
	}
	if oerr := s.access.DeleteOption(ctx, accountID, affectedUserID); oerr != nil {
		return oerr
	}
	return s.access.Delete(ctx, accountID, affectedUserID)
}
```

NOTE: `MembershipsByUser` returns folderID -> account ids for the USER'S OWN folders — for the affected user that is exactly the set to scrub (same data the old `FoldersContaining` glue derived from it).

- [ ] **Step 4: Rewire the service**

`internal/account/usecase.go`:
- Struct: delete `shared SharedAccessLookup` and `revoker AccessRevoker`; add `access AccessStore`.
- `NewService(repo Repository, folders FolderRepository, access AccessStore, currency CurrencyLookup, users UserLookup, tx port.TxRunner, ops port.OperationGuard, clock port.Clock)` — set `access: access`, drop the removed fields.
- `sharedAccessFor`: replace the `s.shared == nil` guard + `s.shared.ListByAccount` with `s.access.ListByAccount` (grants are `[]*model.AccountAccess` now):

```go
func (s *Service) sharedAccessFor(ctx context.Context, accountID vo.Id, cache *accountEmbedCache) ([]model.SharedAccess, error) {
	out := []model.SharedAccess{}
	grants, err := s.access.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		u, uerr := s.resolveOwner(ctx, cache, g.UserID.String())
		if uerr != nil {
			return nil, uerr
		}
		accepted := 0
		if g.IsAccepted {
			accepted = 1
		}
		out = append(out, model.SharedAccess{User: u, Role: g.Role.Alias(), IsAccepted: accepted})
	}
	return out, nil
}
```

- `buildAccountList`: the pending loop switches `s.accounts.ListPendingReceived` → `s.access.ListPendingReceived`.
- `internal/account/repository.go`: remove `ListPendingReceived` from `AccountStore` (it lives on `AccessStore`); remove its implementation from the ACCOUNT store side of `internal/account/repo/` (repo.go + shims) added in Task 3.
- `internal/account/ports.go`: delete `SharedAccessLookup` and `AccessRevoker` (and now-unused imports).
- `internal/account/delete.go` non-owner branch becomes:

```go
	// Non-owner: must hold a grant (pending counts -- deleting from their side
	// is a decline), then drop their own access. AccessStore.Get takes
	// (accountID, userID).
	if _, gerr := s.access.Get(ctx, id, userID); gerr != nil {
		var nf *errs.NotFoundError
		if errors.As(gerr, &nf) {
			return nil, errs.NewAccessDenied("Access denied")
		}
		return nil, gerr
	}
	if rerr := s.RevokeOwnAccess(ctx, userID, id); rerr != nil {
		return nil, rerr
	}
	return &model.DeleteAccountResult{}, nil
```

- `internal/server/server.go`: in the account wiring block add `accountAccessRepo := accountrepo.NewAccessRepo(cfg.DatabaseDriver, txm)` and pass it: `appaccount.NewService(accountRepo, folderRepo, accountAccessRepo, accountCurrencyLookup, userOwnerLookup, txm, opGuard, clk)`. Delete `accountSharedLookup` and `accountRevoker` lines and their glue types in `glue_connection.go` (`ConnectionSharedAccessLookup`, `connectionAccountAccessLister`, `ConnectionAccessRevoker`, `connectionAccessRevokerDeps`, `connectionOwnAccessRevoker`). The connection service wiring stays as-is for now.
- Fix every test harness that calls `appaccount.NewService` (`internal/account/api/harness_test.go`, and grep: `grep -rln "appaccount.NewService\|account.NewService" internal/ | grep _test`) — follow compile errors; they now pass `accountrepo.NewAccessRepo(db.Engine, db.TX)` and drop the two glue params.

- [ ] **Step 5: Run the tests**

Run: `go build ./... && go test ./internal/account/... ./internal/server/... ./internal/test/apiparity/`
Expected: Step-2 tests PASS; everything else still PASS; goldens unchanged.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(account): grant/accept/decline/revoke access use cases"
```

---

### Task 6: Account access API endpoints

**Files:**
- Create: `internal/account/api/access.go`
- Modify: `internal/account/api/routes.go`
- Modify: `internal/test/apiparity/catalogue_account.go` (new scenario)
- Test: `internal/account/api/access_endpoints_test.go` (new)
- Regenerated: `internal/web/apidoc/` docs (`make swagger`), goldens

**Interfaces:**
- Consumes: the four Service methods from Task 5; `endpoint.Handle` combinator; handler pattern from `internal/connection/api/connection.go:30-66`.
- Produces routes: `POST /api/v1/account/grant-access`, `POST /api/v1/account/accept-access`, `POST /api/v1/account/decline-access`, `POST /api/v1/account/revoke-access`.

- [ ] **Step 1: Write the failing endpoint tests**

`internal/account/api/access_endpoints_test.go`, using the package's harness (`newHarness`, fixture builder, bearer = user id via authstub). Cover the happy path end to end plus the envelope-level contracts:

- grant as owner → 200 `{"success":true,...,"data":{}}`; grantee's `get-account-list` now contains the account with `folderId: null` and own entry `isAccepted: 0`.
- accept as grantee with a folderId → 200; list shows real folder + position, `isAccepted: 1`.
- decline as grantee (fresh pending) → 200; row gone from list.
- revoke as owner → 200.
- negatives: grant by a non-owner stranger → 403 envelope `"Access denied"`; accept with no pending → 403; blank body fields → 400 with `IS_BLANK_ERROR`; unknown role → 400 with `"AccountRole with alias xyz not exists"`.

Run: `go test ./internal/account/api/ -run Access -v` → FAIL (404s).

- [ ] **Step 2: Handlers**

`internal/account/api/access.go` — four thin handlers, each with a full swag block (copy the annotation structure from `internal/connection/api/connection.go:30-46`, adjusting `@Tags Account`, paths, request/result types):

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// GrantAccess handles POST /api/v1/account/grant-access (auth).
//
// @Summary     Grant account access
// @Description Grants or updates a connected user's role on an account you own or administer. New grants are pending until accepted.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       request body     model.GrantAccountAccessRequest true "Grant account access request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GrantAccountAccessResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/account/grant-access [post]
func (h *Handlers) GrantAccess(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.GrantAccess)
}
```

…and `AcceptAccess`, `DeclineAccess`, `RevokeAccess` identically (adjust names/types/descriptions; accept's description mentions the folder choice). Check whether the existing `handler.go` forces the `model` import for swag (`var _ = model...`); mirror what `internal/account/api/account.go` does at its top.

- [ ] **Step 3: Routes**

`internal/account/api/routes.go`, after the account block, before the folder block:

```go
		mux.Handle("POST /api/v1/account/grant-access", auth(h.GrantAccess))
		mux.Handle("POST /api/v1/account/accept-access", auth(h.AcceptAccess))
		mux.Handle("POST /api/v1/account/decline-access", auth(h.DeclineAccess))
		mux.Handle("POST /api/v1/account/revoke-access", auth(h.RevokeAccess))
```

- [ ] **Step 4: Run endpoint tests**

Run: `go test ./internal/account/api/ -run Access -v` → PASS.

- [ ] **Step 5: apiparity scenario + swagger + goldens**

In `internal/test/apiparity/catalogue_account.go`, register a new scenario (study the file's existing scenarios for the fixture ids; `OwnerAccount`, `GuestID`, and the guest's folder id constant are in `internal/test/apiparity/fixture.go`):

```go
	register(Scenario{Name: "account_access_writes", Calls: func() []Call {
		return []Call{
			{Label: "grant-access", Method: "POST", Path: "/api/v1/account/grant-access", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID, "role": "user"}},
			{Label: "get-account-list-pending", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "guest"},
			{Label: "accept-access", Method: "POST", Path: "/api/v1/account/accept-access", Auth: "guest",
				Body: map[string]any{"accountId": OwnerAccount, "folderId": GuestFolder}},
			{Label: "get-account-list-accepted", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "guest"},
			{Label: "revoke-access", Method: "POST", Path: "/api/v1/account/revoke-access", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID}},
			{Label: "grant-access-again", Method: "POST", Path: "/api/v1/account/grant-access", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID, "role": "guest"}},
			{Label: "decline-access", Method: "POST", Path: "/api/v1/account/decline-access", Auth: "guest",
				Body: map[string]any{"accountId": OwnerAccount}},
			{Label: "err:accept-access-no-pending", Method: "POST", Path: "/api/v1/account/accept-access", Auth: "guest",
				Body: map[string]any{"accountId": OwnerAccount}},
		}
	}})
```

(Use the actual exported fixture constant for the guest folder — check `fixture.go`; if none exists, export one.)

Run: `make swagger`, then `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect `git diff` (new golden file for the scenario; no changes to others), then `go test ./internal/test/apiparity/` → PASS (route-coverage guard is satisfied for the 4 new routes).

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(account): grant/accept/decline/revoke access endpoints"
```

---

### Task 7: Slim the connection feature down to what it owns

Remove the old account-access endpoints and use cases; delete-connection unwinds account access through a port to the account feature.

**Files:**
- Delete: `internal/connection/setaccess.go`, `internal/connection/revoke.go`
- Modify: `internal/connection/ports.go` (drop FolderPort/OptionPort; add AccountAccessRevoker)
- Modify: `internal/connection/usecase.go` (Service struct/constructor; requireOwnerAdmin deleted)
- Modify: `internal/connection/invite_usecase.go` (DeleteConnection uses the port; revokeGrantTx deleted)
- Modify: `internal/connection/repository.go` (shrink AccountAccessRepository to what the service still uses)
- Modify: `internal/connection/api/connection.go` + `routes.go` (drop the two handlers/routes)
- Modify: `internal/model/connection_dto.go` (delete SetAccountAccessRequest/Result; MOVE RevokeAccountAccessRequest/Result to `internal/model/account_dto.go`)
- Modify: `internal/server/glue_connection.go` (+ `server.go`): new revoker adapter; wiring order flips (account before connection)
- Modify: `internal/connection/access_test.go`, `internal/connection/api/*_test.go`, `internal/test/apiparity/catalogue_connection.go`
- Regenerated: swagger docs, goldens

**Interfaces:**
- Produces (`internal/connection/ports.go`):

```go
// AccountAccessRevoker unwinds account sharing between two users on
// delete-connection. Backed by the account feature via a server adapter.
// Runs on the caller's transaction context. May be nil in stripped-down test
// harnesses (delete-connection then skips the account unwind).
type AccountAccessRevoker interface {
	RevokeAccessBetween(ctx context.Context, a, b vo.Id) error
}
```

- New connection constructor: `NewService(access AccountAccessRepository, invites InviteRepository, users UserLookup, accountAccess AccountAccessRevoker, budgetAccess BudgetAccessRevoker, tx port.TxRunner, clock port.Clock) *Service`.
- Shrunk repository interface (repo methods themselves stay implemented — `AccountAccessResolver` and its tests still use `Get`):

```go
type AccountAccessRepository interface {
	ListReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)
	ListIssued(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
	DeleteConnection(ctx context.Context, a, b vo.Id) error
	ConnectUsers(ctx context.Context, a, b vo.Id) error
}
```

- [ ] **Step 1: Update DeleteConnection + write/adjust its test first**

In `internal/connection/access_test.go` (and/or wherever DeleteConnection is covered — grep `DeleteConnection` in `internal/connection/*_test.go`), the test service construction changes to the new signature; add an assertion that a PENDING grant between the pair is also removed by delete-connection. The account-unwind assertions now go through a real account service adapter — in feature-level connection tests you may use a small inline stub implementing `AccountAccessRevoker` and assert it was called with the right pair; the REAL end-to-end unwind is covered by the server-level apiparity scenario (`delete-connection` already in the catalogue) plus a server test if one exists (`internal/server/glue_connection_test.go` — update it to cover the new adapter).

- [ ] **Step 2: Implement the feature changes**

`internal/connection/invite_usecase.go` — DeleteConnection's tx body becomes:

```go
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if s.accountAccess != nil {
			if aerr := s.accountAccess.RevokeAccessBetween(txCtx, userID, connectedID); aerr != nil {
				return aerr
			}
		}
		if s.budgetAccess != nil {
			if berr := s.budgetAccess.RevokeBetween(txCtx, userID, connectedID); berr != nil {
				return berr
			}
		}
		return s.access.DeleteConnection(txCtx, userID, connectedID)
	}); err != nil {
		return nil, err
	}
```

Delete `revokeGrantTx`. Delete `setaccess.go` + `revoke.go`. In `usecase.go`: drop `folders`, `options` fields; add `accountAccess AccountAccessRevoker`; delete `requireOwnerAdmin`; update `NewService` (keep `parseID`). Shrink `repository.go` as above.

`internal/connection/api/`: remove the `SetAccountAccess`/`RevokeAccountAccess` handlers and their two routes. `internal/model/connection_dto.go`: delete `SetAccountAccessRequest`/`SetAccountAccessResult`; move `RevokeAccountAccessRequest`/`RevokeAccountAccessResult` (with their Validate) into `internal/model/account_dto.go`.

- [ ] **Step 3: Server wiring**

`internal/server/glue_connection.go`: delete `ConnectionFolderPort` + `connectionFolderRepo` (nothing uses them now). Add:

```go
// connectionAccountRevoker is the slice of the account service that
// delete-connection's unwind needs.
type connectionAccountRevoker interface {
	RevokeAccessBetween(ctx context.Context, a, b vo.Id) error
}

// ConnectionAccountAccessRevoker adapts the account service to
// connection.AccountAccessRevoker.
type ConnectionAccountAccessRevoker struct{ accounts connectionAccountRevoker }

var _ appconnection.AccountAccessRevoker = (*ConnectionAccountAccessRevoker)(nil)

func NewConnectionAccountAccessRevoker(accounts connectionAccountRevoker) *ConnectionAccountAccessRevoker {
	return &ConnectionAccountAccessRevoker{accounts: accounts}
}

func (r *ConnectionAccountAccessRevoker) RevokeAccessBetween(ctx context.Context, a, b vo.Id) error {
	return r.accounts.RevokeAccessBetween(ctx, a, b)
}
```

`internal/server/server.go`: build `accountSvc` BEFORE `connectionSvc` (move the account block up; account no longer references connection), then:

```go
	connectionSvc := appconnection.NewService(
		connectionRepo, connectionInviteRepo, userOwnerLookup,
		NewConnectionAccountAccessRevoker(accountSvc), connectionBudgetRevoker, txm, clk,
	)
```

Update the stale "Connection service is built first" comment. Fix `internal/connection/api/harness_test.go` and any other harness for the new constructor (grep `appconnection.NewService`).

- [ ] **Step 4: apiparity + swagger + goldens**

`internal/test/apiparity/catalogue_connection.go`: delete the `connection_access_writes` scenario (its routes are gone; total scenario count grew in Task 6, so no guard trips). The orphaned-golden guard will flag `connection_access_writes.golden` — delete it. Any golden that replayed those routes regenerates away.

Run: `make swagger && UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git status internal/test/apiparity/testdata/golden/`
Expected diff: `connection_access_writes.golden` deleted; nothing else.

- [ ] **Step 5: Verify + commit**

Run: `go build ./... && go test ./... && make go-lint`
Expected: PASS (archtest included — connection no longer reaches account data except through its port).

```bash
git add -A && git commit -m "refactor(connection): account-access moves out; delete-connection unwinds via port"
```

---

### Task 8: Full backend verification

- [ ] **Step 1: Smoke tier**

Run: `make go-test`
Expected: build + vet + gofmt + docs-fresh + tests + coverage gate all green. If coverage dips below the gate, add missing unit tests around `internal/account/access.go` branches (parse errors, repo error paths).

- [ ] **Step 2: Full tier (PostgreSQL + engine compare + frontend)**

Run: `make test`
Expected: PASS. This exercises the pgsql adapters for the new/changed queries and asserts byte-identical responses across engines for the new scenario (pending ordering is pinned by the `ORDER BY created_at, account_id`).

- [ ] **Step 3: Commit anything the run regenerated; otherwise no-op**

```bash
git status --short   # expect clean
```

---

### Task 9: Frontend — API layer, hooks, and list hygiene

Move account-access calls to the account API module with the new endpoints, expose `isAccepted`, filter pending items out of the normal lists, show "invitation pending" on the sharer side, and drop the old inline budget affordance.

**Files:**
- Modify: `web/src/api/dto/account.ts` (AccountAccessDto gains `isAccepted: 0 | 1`)
- Modify: `web/src/api/account.ts` (four new functions)
- Modify: `web/src/api/connection.ts` (delete setAccountAccess/revokeAccountAccess)
- Modify: `web/src/features/accounts/queries.ts` (four new mutation hooks; useAccounts filters pending)
- Modify: `web/src/features/connections/queries.ts` (delete useSetAccountAccess/useRevokeAccountAccess)
- Modify: `web/src/features/connections/shared.ts` (delete applyAccountAccess/removeAccountAccess; add isPendingForMe; buildShareEntries passes account isAccepted through — verify it already does via the `isAccepted?: 0|1` param)
- Modify: `web/src/features/connections/ShareEntryList.tsx` (pending text for accounts too)
- Modify: callers of the old hooks: `web/src/features/accounts/AccountDialog.tsx`, `web/src/features/accounts/AccountsSettingsPage.tsx` (grep `useSetAccountAccess\|useRevokeAccountAccess` to find all)
- Modify: `web/src/features/budgets/queries.ts` (useBudgets filters pending) + `web/src/features/budgets/BudgetsPage.tsx` (remove accept item + pending banner)
- Modify: `web/src/test/fixtures.ts` (sharedAccess fixtures gain isAccepted where present)
- Test: `web/src/features/accounts/queries.test.tsx`, `web/src/features/connections/ShareEntryList.test.tsx`, `web/src/features/budgets/BudgetsPage.test.tsx`

**Interfaces:**
- Produces (`web/src/api/account.ts`):

```ts
export async function grantAccess(form: { accountId: Id; userId: Id; role: AccountRole }): Promise<void> {
  await api.post(apiUrl('/api/v1/account/grant-access'), form)
}
export async function acceptAccess(form: { accountId: Id; folderId?: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/account/accept-access'), { accountId: form.accountId, folderId: form.folderId ?? '' })
}
export async function declineAccess(accountId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/account/decline-access'), { accountId })
}
export async function revokeAccess(form: { accountId: Id; userId: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/account/revoke-access'), form)
}
```

(Import `AccountRole` from `./dto/account`.)

- Produces (`web/src/features/accounts/queries.ts`) — all four invalidate instead of cache surgery:

```ts
export function useGrantAccountAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { accountId: Id; userId: Id; role: AccountRole }) => accountApi.grantAccess(form),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
    },
  })
}
export function useAcceptAccountAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { accountId: Id; folderId?: Id }) => accountApi.acceptAccess(form),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.folders })
    },
  })
}
export function useDeclineAccountAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (accountId: Id) => accountApi.declineAccess(accountId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
    },
  })
}
export function useRevokeAccountAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { accountId: Id; userId: Id }) => accountApi.revokeAccess(form),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
    },
  })
}
```

(Keep the `trackEvent(METRICS.CONNECTION_UPDATE_ACCOUNT_ACCESS)` / `CONNECTION_REVOKE_ACCOUNT_ACCESS` calls in grant/revoke onSuccess if METRICS is easy to import — preserve existing analytics.)

- Produces (`web/src/features/connections/shared.ts`):

```ts
/** True when the account is shared TO me and I have not accepted yet. */
export function isPendingForMe(account: AccountDto, meId: Id | undefined): boolean {
  if (!meId || account.owner.id === meId) return false
  return account.sharedAccess.some((a) => a.user.id === meId && a.isAccepted === 0)
}
```

- [ ] **Step 1: DTO + fixtures**

`web/src/api/dto/account.ts`:

```ts
export interface AccountAccessDto {
  user: UserDto
  role: AccountRole
  isAccepted: 0 | 1
}
```

Update `web/src/test/fixtures.ts`: any fixture account with a non-empty `sharedAccess` gains `isAccepted: 1` on its entries (grep `sharedAccess` in the file). TypeScript errors point at every other literal to fix.

- [ ] **Step 2: Write failing tests**

1. `web/src/features/accounts/queries.test.tsx`: `useAccounts` hides an account whose `sharedAccess` marks ME (fixtureUser id `'u1'`) pending (`isAccepted: 0`), and shows it once `isAccepted: 1`. Follow the existing test file's provider/msw setup.
2. `web/src/features/connections/ShareEntryList.test.tsx`: an ACCOUNTS entry with `isAccepted: false` renders the "invitation pending" suffix (today only budgets do).
3. `web/src/features/budgets/BudgetsPage.test.tsx`: a budget whose myAccess `isAccepted: 0` does NOT render in the list (today it renders with an Accept menu item — invert that assertion).

Run: `cd web && pnpm test -- features/accounts/queries.test.tsx features/connections/ShareEntryList.test.tsx features/budgets/BudgetsPage.test.tsx` → new tests FAIL.

- [ ] **Step 3: Implement**

- API functions + hooks as in Interfaces above; delete `setAccountAccess`/`revokeAccountAccess` from `api/connection.ts` and `useSetAccountAccess`/`useRevokeAccountAccess` from `features/connections/queries.ts`; delete `applyAccountAccess`/`removeAccountAccess` from `shared.ts`.
- Update callers (`AccountDialog.tsx`, `AccountsSettingsPage.tsx`, any other grep hit) to import the new hooks from `@/features/accounts/queries` — mutate payload shapes are unchanged (`{accountId, userId, role}` / `{accountId, userId}`).
- `useAccounts` filter (in `web/src/features/accounts/queries.ts`):

```ts
export function useAccounts() {
  const { data: user } = useUserData()
  return useQuery({
    queryKey: queryKeys.accounts,
    queryFn: accountApi.getAccountList,
    staleTime: TEN_MINUTES,
    // get-account-list response order differs from order-account-list; position is authoritative.
    // Invites I have not accepted are surfaced by the sharing-requests modal, not the lists.
    select: (items) =>
      items
        .filter((a) => !isPendingForMe(a, user?.id))
        .sort((a, b) => a.position - b.position),
  })
}
```

(Import `useUserData` from `@/features/user/queries` and `isPendingForMe` from `@/features/connections/shared`. Note `select` must not mutate: `.filter` already copies, so the previous `[...items]` spread is redundant.)
- `useBudgets` in `web/src/features/budgets/queries.ts`: add the analogous select filter keeping budgets where I'm the owner (`budget.ownerUserId === user?.id`) or my access entry has `isAccepted === 1` (read the hook first; if it already has a select, compose).
- `ShareEntryList.tsx` `roleText`: change

```ts
if (kind === 'budgets' && entry.isAccepted === false) {
```

to

```ts
if (entry.isAccepted === false) {
```

- `BudgetsPage.tsx`: remove the `!accepted` Accept `DropdownMenuItem` (lines ~124-129) and the pending banner span (lines ~102-106); the `accepted`/`myAccess` helpers may simplify or go if unused (decline stays — it is "leave shared budget" for accepted non-owners). Keep `useAcceptBudgetAccess`/`useDeclineBudgetAccess` exported from queries — the modal consumes accept in Task 10; decline is still used by BudgetsPage.

- [ ] **Step 4: Run tests + lint**

Run: `cd web && pnpm test -- features/accounts features/connections features/budgets && pnpm lint`
Expected: PASS (ImportCsvDialog failure is pre-existing and out of scope if the full suite is run).

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(web): account access via account endpoints; pending items leave the lists"
```

---

### Task 10: Sharing-requests modal + pending-invites hook

**Files:**
- Create: `web/src/features/connections/pendingInvites.ts` (hook)
- Create: `web/src/features/connections/SharingRequestsDialog.tsx`
- Modify: `web/src/locales/en-US.ts` (strings under `modules.connections.sharing_requests` and `blocks.main.sharing_requests`)
- Test: `web/src/features/connections/pendingInvites.test.tsx`, `web/src/features/connections/SharingRequestsDialog.test.tsx`

**Interfaces:**
- Produces:

```ts
export interface PendingInvite {
  kind: 'account' | 'budget'
  id: Id            // account id or budget id
  name: string      // entity name
  owner: UserDto
  role: string      // role alias granted to me
}
export function usePendingInvites(): { invites: PendingInvite[]; count: number }
```

- `SharingRequestsDialog({ open, onClose }: { open: boolean; onClose: () => void })` — lists `usePendingInvites()`, accept/decline per row; account accept opens an inline folder-picker step.
- Consumes: `useUserData`, `useAcceptAccountAccess`/`useDeclineAccountAccess` (Task 9), `useAcceptBudgetAccess`/`useDeclineBudgetAccess` (existing), folder list hook (grep `queryKeys.folders` in `web/src/features/accounts/queries.ts` — a `useFolders` hook exists or add one wrapping `accountApi.getFolderList` with `staleTime: TEN_MINUTES`), `ResponsiveDialog`, `ConfirmDialog`, `UserAvatar` components (all used by existing connections dialogs — copy their import paths).

- [ ] **Step 1: Locale strings**

In `web/src/locales/en-US.ts`, under `blocks.main` add `sharing_requests: 'Sharing requests'`; under `modules.connections` add:

```ts
sharing_requests: {
  title: 'Sharing requests',
  empty: 'No pending requests',
  invited_you: '{name} invited you',
  account: 'Account',
  budget: 'Budget',
  choose_folder: 'Choose a folder for this account',
  general_folder_hint: 'General (will be created)',
  decline_question: 'Decline access to "{name}"?',
},
```

(Interpolation uses single braces — `{name}` — per `app/i18n.ts`.)

- [ ] **Step 2: The hook + its failing test**

`web/src/features/connections/pendingInvites.ts`:

```ts
import { useQuery } from '@tanstack/react-query'
import * as accountApi from '@/api/account'
import * as budgetApi from '@/api/budget'
import type { AccountDto } from '@/api/dto/account'
import type { BudgetMetaDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import type { UserDto } from '@/api/dto/user'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { useUserData } from '@/features/user/queries'

export interface PendingInvite {
  kind: 'account' | 'budget'
  id: Id
  name: string
  owner: UserDto
  role: string
}

/** Pending share invites for the current user, derived from the account and
 *  budget list caches (raw, unfiltered queries on the same keys). */
export function usePendingInvites(): { invites: PendingInvite[]; count: number } {
  const { data: user } = useUserData()
  const meId = user?.id
  const { data: accounts } = useQuery({
    queryKey: queryKeys.accounts,
    queryFn: accountApi.getAccountList,
    staleTime: TEN_MINUTES,
    enabled: !!meId,
  })
  const { data: budgets } = useQuery({
    queryKey: queryKeys.budgets,
    queryFn: budgetApi.getBudgetList,
    staleTime: TEN_MINUTES,
    enabled: !!meId,
  })
  const invites: PendingInvite[] = []
  for (const a of accounts ?? []) {
    const mine = a.sharedAccess.find((s) => s.user.id === meId && s.isAccepted === 0)
    if (mine && a.owner.id !== meId) {
      invites.push({ kind: 'account', id: a.id, name: a.name, owner: a.owner, role: mine.role })
    }
  }
  for (const b of budgets ?? []) {
    const mine = b.access.find((s) => s.user.id === meId && s.isAccepted === 0)
    if (mine && b.ownerUserId !== meId) {
      const owner = b.access.find((s) => s.user.id === b.ownerUserId)?.user
      invites.push({ kind: 'budget', id: b.id, name: b.name, owner: owner ?? { id: b.ownerUserId, name: '', avatar: '' }, role: mine.role })
    }
  }
  return { invites, count: invites.length }
}
```

(Check the budget list API function name in `web/src/api/budget.ts` — adjust `getBudgetList` and the owner-resolution to the real `BudgetMetaDto` shape; the owner may need resolving from `useConnections` if not embedded in `access` — inspect `fixtureBudgets` first and follow reality.)

Test `pendingInvites.test.tsx`: renderHook with QueryClientProvider + msw `coreHandlers` overridden so one account and one budget carry a pending entry for `u1`; assert `count === 2` and the invite fields. Then zero-pending → `count === 0`. Run → FAIL until implemented, then PASS.

- [ ] **Step 3: The dialog + its failing test**

`SharingRequestsDialog.tsx` — follow `ShareAccessDialog.tsx`'s `ResponsiveDialog` skeleton. Behavior:

- Rows: `UserAvatar owner` + `t('modules.connections.sharing_requests.invited_you', { name: owner.name })`, entity kind label + entity name, role text (reuse the `modules.connections.${kind}s.roles.${role}` keys), and two buttons: Accept (`t('elements.button.accept.label')`) and Decline (`t('elements.button.decline.label')`, destructive).
- Accept on a BUDGET row: `acceptBudget.mutate(invite.id)`.
- Accept on an ACCOUNT row: reveal an inline folder `<Select>` (shadcn select — copy the pattern from `AccountDialog.tsx`'s folder picker) listing `useFolders()` items, preselected to the LAST folder; when the user has zero folders show a disabled option `t('...general_folder_hint')`; confirm button calls `acceptAccount.mutate({ accountId: invite.id, folderId: selected || undefined })`.
- Decline: `ConfirmDialog` with `t('...decline_question', { name: invite.name })`; on confirm, `declineAccount.mutate(invite.id)` or `declineBudget.mutate(invite.id)`.
- When `invites.length === 0` render `t('...empty')`; auto-close is the parent's concern (Task 11 hides the button; keep the dialog dumb).

Test: render with msw fixtures carrying one pending account (owner `u2`, role `user`) and one pending budget; assert rows render; click account Accept → folder select appears with fixture folders → confirm → assert POST body `{accountId, folderId}` captured via `server.use(http.post('*/api/v1/account/accept-access', ...))`; click budget Accept → assert `*/api/v1/budget/accept-access` called; Decline flows confirm-then-POST. Follow `ConnectionsPage.test.tsx`'s request-capturing style.

- [ ] **Step 4: Run + commit**

Run: `cd web && pnpm test -- features/connections && pnpm lint`
Expected: PASS.

```bash
git add -A && git commit -m "feat(web): sharing-requests modal + pending-invites hook"
```

---

### Task 11: Sidebar button

**Files:**
- Modify: `web/src/app/layouts/ApplicationLayout.tsx`
- Test: extend the layout's existing test file if present (grep `ApplicationLayout.test`), else add `web/src/app/layouts/ApplicationLayout.sharing.test.tsx`

**Interfaces:**
- Consumes: `usePendingInvites`, `SharingRequestsDialog` (Task 10).

- [ ] **Step 1: Failing test**

Render the layout (copy provider/msw scaffolding from an existing layout or page test) with fixtures producing 2 pending invites: assert a button labeled `Sharing requests` with visible count `2` renders ABOVE the Budget link; click → the dialog opens (assert its title). With zero pending: the button is absent.

- [ ] **Step 2: Implement**

In `ApplicationLayout.tsx`:

- `const { count } = usePendingInvites()` and `const [sharingOpen, setSharingOpen] = useState(false)`.
- Full-width nav (before the Budget `<Link>` at ~line 166):

```tsx
{count > 0 ? (
  <button
    type="button"
    onClick={() => setSharingOpen(true)}
    className={`flex items-center justify-between rounded-md px-2 py-2 text-left hover:bg-accent ${isCompact ? 'text-lg' : 'text-[15px]'}`}
  >
    <span>{t('blocks.main.sharing_requests')}</span>
    <span className="ml-2 rounded-full bg-primary px-2 py-0.5 text-xs text-primary-foreground">{count}</span>
  </button>
) : null}
```

- Collapsed rail (before the Wallet icon at ~line 150): same conditional with an icon button — `UserPlus` from `lucide-react` (add to the existing import), `title={t('blocks.main.sharing_requests')}`, and a tiny count bubble (absolutely positioned span, same badge classes at `text-[10px]`).
- Mount `<SharingRequestsDialog open={sharingOpen} onClose={() => setSharingOpen(false)} />` next to the layout's other dialogs.
- When `count` drops to 0 while open, keep the dialog open showing the empty state (no forced close — simpler and predictable).

- [ ] **Step 3: Run + commit**

Run: `cd web && pnpm test -- app/layouts features/connections && pnpm lint`
Expected: PASS.

```bash
git add -A && git commit -m "feat(web): sharing-requests sidebar button with pending count"
```

---

### Task 12: End-to-end verification

- [ ] **Step 1: Full suite**

Run: `make test`
Expected: PASS (backend both engines + frontend; ImportCsvDialog is the only tolerated pre-existing failure — confirm nothing NEW fails).

- [ ] **Step 2: Golden sanity sweep**

Run: `git log --oneline main..HEAD -- internal/test/apiparity/testdata/golden/ && git diff main -- internal/test/apiparity/testdata/golden/ | head -200`
Re-read the cumulative golden diff against expectations: (a) `isAccepted` added to `sharedAccess` entries, (b) new `account_access_writes` golden, (c) `connection_access_writes` golden deleted. Nothing else.

- [ ] **Step 3: Live smoke (recommended)**

Run the app (`make go-run` + `make web-run`, or the /verify skill) with two users: connect them, share an account, confirm the sidebar button + modal + folder pick + the account appearing after accept, and that BEFORE accepting the recipient sees no transactions of that account.

- [ ] **Step 4: Wrap up**

Use superpowers:finishing-a-development-branch — the work merges toward `main` via PR (the repo's convention).
