# Transactions in IndexedDB with per-account revision sync — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep transactions in one IndexedDB database grouped by account and re-download only accounts whose server-side `transactions_revision` moved, instead of the whole list on every boot/refresh.

**Architecture:** The backend adds `accounts.transactions_revision`, bumped by repository methods through a commit-time flush on `backend.TxManager` (exactly +1 per account per DB transaction), exposed on the account DTO, plus a batch read endpoint `get-account-transaction-list`. A behavioural guard suite replays every API/MCP scenario and fails when an account's transaction data changes without a bump. The SPA keeps `['transactions']` as one `TransactionDto[]` but its `queryFn` becomes a revision-driven sync over an IndexedDB store; the sync button's tap stays `invalidateQueries()` (now incremental), a long-press does a full reload.

**Tech Stack:** Go 1.27 (stdlib, sqlc, modernc sqlite, pgx), React 19, TanStack Query 5, vitest + msw + fake-indexeddb.

**Spec:** `docs/superpowers/specs/2026-10-11-transactions-indexeddb-sync-design.md` — read it before any task.

## Global Constraints

- Go toolchain: `export PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1` before any `go`/`make` command.
- Frozen contracts: never change `get-transaction-list`'s request or response; the new field `transactionsRevision` on the account DTO is additive.
- Features never import features; `internal/infra/txrevision` may import only `internal/infra/...`, `internal/shared/...`.
- `.sql` query files ASCII-only; a `;` must end the statement on the same line (never alone on a line).
- After any `.sql` query/migration change: `cd internal/infra/storage/sqlc && sqlc generate` (or `go generate ./...` if that is how the repo wires it — check `Makefile` target `sqlc`/`generate` first).
- Goldens: regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/` and INSPECT the diff; never hand-edit.
- Comments: only non-obvious *why*; no restating names; no PHP references.
- Frontend tests use `fake-indexeddb` (already a devDependency, auto-loaded in `web/src/test/setup.ts`); reset per test with `globalThis.indexedDB = new IDBFactory()`.
- New user-facing action → `METRICS` key fired via literal `METRICS.KEY` in non-test source.
- Every new i18n key goes into all 11 `locales/*.json` (de, en, es, fr, it, nl, pl, pt, ru, uk, zh) with real translations.
- Branch `feature/transactions-indexeddb-sync`; commit after each task with a `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` trailer.

## Review Focus

- A request that writes the same account several times in one DB transaction (create transfer with labels, update moving accounts, account update + correction) must move each affected account by exactly 1 — pinned in Task 3 tests.
- Two concurrent PostgreSQL transactions bumping overlapping accounts must not deadlock — sorted flush, pinned in Task 1 (order assertion) and Task 2 (pgsql run).
- An account shared then revoked, or deleted, between the accounts fetch and the transaction fetch must vanish locally, not linger — pinned in Task 9 (absent from `revisions` → bucket deleted).
- A different user signing in on the same browser must never see the previous user's transactions, even through a 401 path that skips the logout cleanup — pinned in Task 8 (userId mismatch wipes) and Task 11.
- Pending-invite accounts must not be re-requested on every sync — pinned in Task 9.

---

## File Structure

Backend
- `internal/infra/storage/backend/tx.go` — add before-commit hook + per-tx values.
- `internal/infra/txrevision/txrevision.go` (+ `_test.go`) — `Bumper`.
- `internal/infra/storage/migrations/{sqlite,pgsql}/20260816000001.sql` — column.
- `internal/infra/storage/sqlc/query/{sqlite,pgsql}/accounts.sql` — select the column; new `GetAccountTransactionsRevisions`-style read lives in the transaction repo (hand-built IN list).
- `internal/model/account.go`, `internal/model/account_dto.go` — field.
- `internal/account/{repo/repo.go,usecase.go,create.go,update.go}` — hydrate/DTO/fresh revision.
- Bump sites: `internal/{transaction,account,category,payee,tag,label,recurring,imports,user}/repo/repo.go`.
- `internal/model/transaction_dto.go`, `internal/transaction/read.go`, `internal/transaction/repository.go`, `internal/transaction/repo/repo.go`, `internal/transaction/api/{accounttransactionlist.go,routes.go}` — endpoint.
- `internal/test/apiparity/{harness.go,catalogue_transaction*.go,guard_test.go}`, `internal/test/mcpparity/mcpparity.go` — hook + scenarios.
- `internal/test/revisionguard/{guard.go,smoke_test.go,enginecompare_test.go}` — guard suite.

Frontend (`web/src/`)
- `lib/db.ts` (+ test) — the one `econumo` IndexedDB.
- `lib/importCrypto.ts` — switch to `db.ts`.
- `lib/transactionStore.ts` (+ test) — buckets + meta.
- `api/dto/transaction.ts`, `api/dto/account.ts`, `api/transaction.ts` — DTOs + `getAccountTransactionList`.
- `features/transactions/sync.ts` (+ test) — pure sync engine.
- `features/transactions/queries.ts`, `features/transactions/useAccountTransactions.ts`, `features/transactions/revisionTracking.ts` (+ test) — wiring, accounts subscription, write-through, +1 shortcut.
- `features/accounts/queries.ts`, `features/recurring/queries.ts` — +1 shortcut call sites.
- `lib/queryPersist.ts`, `main.tsx`, `features/auth/{queries.ts,LogoutPage.tsx,oauthQueries.ts}` — boot + clear.
- `app/layouts/ApplicationLayout.tsx`, `hooks/useLongPress.ts` (+ test), `lib/metrics.ts`, `locales/*.json` — sync button.
- `docs/regression-test-plan.md`, `CLAUDE.md`.

---

### Task 1: Commit-time hook on TxManager + `txrevision.Bumper`

**Files:**
- Modify: `internal/infra/storage/backend/tx.go`
- Test: `internal/infra/storage/backend/backend_test.go` (or new `tx_hook_test.go`)
- Create: `internal/infra/txrevision/txrevision.go`, `internal/infra/txrevision/txrevision_test.go`

**Interfaces:**
- Produces:
  - `func (m *TxManager) TxValue(ctx context.Context, key any, init func() any) (v any, inTx bool)` — returns the value attached to the active transaction under `key` (creating it with `init` on first use); `inTx=false` and `v=nil` when no transaction is active.
  - `func (m *TxManager) BeforeCommit(ctx context.Context, key any, fn func(ctx context.Context) error) (inTx bool)` — registers `fn` once per transaction per `key`; it runs inside the tx right before the outermost COMMIT (with the tx ctx). An error aborts the commit (rollback, error returned from `WithTx`). Returns false when no tx is active (caller must act immediately).
  - `package txrevision`: `type Bumper struct{...}`; `func New(driver string, tm *backend.TxManager) *Bumper`; `func (b *Bumper) Bump(ctx context.Context, accountIDs ...string) error`; `func (b *Bumper) BumpWhere(ctx context.Context, cond string, args ...any) error`.

The migration that creates `accounts.transactions_revision` is part of this task (Step 1) so the bumper's tests have the column; Task 2 does the rest of the account plumbing.

- [ ] **Step 1: Add the migration files**

`internal/infra/storage/migrations/sqlite/20260816000001.sql`:
```sql
ALTER TABLE accounts ADD COLUMN transactions_revision INTEGER DEFAULT 0 NOT NULL;
```
`internal/infra/storage/migrations/pgsql/20260816000001.sql`:
```sql
ALTER TABLE accounts ADD COLUMN transactions_revision BIGINT NOT NULL DEFAULT 0;
```
(Version `20260816000001` sorts before the `20260817000001` command step `migration:zero-deleted-accounts`, which reads accounts through the repo — the column must exist by then, on fresh installs and old upgrades alike.)

- [ ] **Step 2: Write failing TxManager hook tests**

```go
func TestBeforeCommit_RunsOnceBeforeOutermostCommit(t *testing.T) {
	db := dbtest.NewSQLite(t) // or open a raw sqlite like the other backend tests do
	m := backend.NewTxManager(db.Raw)
	calls := 0
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		for i := 0; i < 3; i++ {
			if !m.BeforeCommit(ctx, "k", func(context.Context) error { calls++; return nil }) {
				t.Fatal("expected inTx")
			}
		}
		return m.WithTx(ctx, func(ctx context.Context) error { // savepoint shares the hook set
			m.BeforeCommit(ctx, "k", func(context.Context) error { calls++; return nil })
			if calls != 0 {
				t.Fatal("hook ran before commit")
			}
			return nil
		})
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestBeforeCommit_ErrorRollsBack(t *testing.T)   // fn returns error -> WithTx returns it, writes in fn's tx are gone
func TestBeforeCommit_NotInTx(t *testing.T)          // returns false, fn not stored
func TestBeforeCommit_SkippedOnRollback(t *testing.T) // WithTx fn returns error -> hook never runs
func TestTxValue_SharedAcrossSavepoints(t *testing.T) // same pointer inside nested WithTx; nil,false outside
```
If `backend` tests cannot import `dbtest` (import cycle), open SQLite via `sql.Open("sqlite", "file::memory:?cache=shared")` with the modernc driver blank-imported, as existing `backend_test.go` does — check it first.

- [ ] **Step 3: Run** `go test ./internal/infra/storage/backend/ -run 'BeforeCommit|TxValue' -v` → FAIL (undefined).

- [ ] **Step 4: Implement in `tx.go`**

```go
type txState struct {
	tx      *sql.Tx
	savepct atomic.Int64
	mu      sync.Mutex
	values  map[any]any
	hooks   []txHook
}

type txHook struct {
	key any
	fn  func(ctx context.Context) error
}

func (m *TxManager) TxValue(ctx context.Context, key any, init func() any) (any, bool) {
	st, ok := dbtxFromCtx(ctx)
	if !ok {
		return nil, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.values == nil {
		st.values = map[any]any{}
	}
	v, found := st.values[key]
	if !found {
		v = init()
		st.values[key] = v
	}
	return v, true
}

// BeforeCommit hooks run inside the transaction so their writes commit
// atomically with it; a hook error turns the commit into a rollback.
func (m *TxManager) BeforeCommit(ctx context.Context, key any, fn func(ctx context.Context) error) bool {
	st, ok := dbtxFromCtx(ctx)
	if !ok {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, h := range st.hooks {
		if h.key == key {
			return true
		}
	}
	st.hooks = append(st.hooks, txHook{key: key, fn: fn})
	return true
}
```
In `top()`, after `err = fn(txCtx)` and before the deferred commit runs, when `err == nil` run the hooks in registration order with `txCtx` (copy the slice under the lock first); assign the first hook error to `err` so the deferred block rolls back. `ContextWithTx` states (test harness outer tx) never commit, so their hooks never run — document that in one comment line on `ContextWithTx`.

- [ ] **Step 5: Run hook tests** → PASS.

- [ ] **Step 6: Write failing txrevision tests** (`dbtest.New(t)` so they also run under `DBTEST_ENGINE=pgsql`; seed with `internal/test/fixture` — read `fixture/entities.go` for the account/transaction builders)

Cases:
- `Bump` in a `WithTx` touching account A three times and B once → after commit A=+1, B=+1, C (untouched) = 0.
- `Bump` outside a tx → +1 immediately per call.
- `BumpWhere(ctx, "category_id = ?", catID)` bumps the account and the recipient account of matching transfers, nothing else.
- A failing `WithTx` (fn returns error after `Bump`) → no change.
- Flush order: register a spy via a test-only seam? Not needed — assert by reading code; instead test that bumping `["b","a"]` and `["a","b"]` both succeed (smoke). Sorting is enforced in code (`sort.Strings`).
- Empty / duplicate ids are ignored.

Use `db.Rebind`-aware raw SQL in the test to read `SELECT transactions_revision FROM accounts WHERE id = ?` (see CLAUDE.md "Raw SQL in a test must use `db.Rebind(query)`").

- [ ] **Step 7: Run** `go test ./internal/infra/txrevision/ -v` → FAIL.

- [ ] **Step 8: Implement `txrevision.go`**

```go
// Package txrevision moves accounts.transactions_revision, the counter clients
// compare to decide which accounts' transactions to download again.
package txrevision

type Bumper struct {
	driver string
	tm     *backend.TxManager
}

func New(driver string, tm *backend.TxManager) *Bumper { return &Bumper{driver: driver, tm: tm} }

type pendingKey struct{}

type pending struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

// Ids are collected for the whole DB transaction and flushed once before
// COMMIT, so a request moves each account by exactly one however many writes
// touched it; the client's "+1 means only my edit" shortcut depends on that.
func (b *Bumper) Bump(ctx context.Context, accountIDs ...string) error {
	v, inTx := b.tm.TxValue(ctx, pendingKey{}, func() any { return &pending{ids: map[string]struct{}{}} })
	if !inTx {
		return b.flush(ctx, accountIDs)
	}
	p := v.(*pending)
	p.mu.Lock()
	for _, id := range accountIDs {
		if id != "" {
			p.ids[id] = struct{}{}
		}
	}
	p.mu.Unlock()
	b.tm.BeforeCommit(ctx, pendingKey{}, func(ctx context.Context) error {
		p.mu.Lock()
		ids := make([]string, 0, len(p.ids))
		for id := range p.ids {
			ids = append(ids, id)
		}
		p.mu.Unlock()
		return b.flush(ctx, ids)
	})
	return nil
}

// cond is always a fixed string from the calling repo; values travel in args.
func (b *Bumper) BumpWhere(ctx context.Context, cond string, args ...any) error {
	q := "SELECT account_id FROM transactions WHERE " + cond +
		" UNION SELECT account_recipient_id FROM transactions WHERE (" + cond + ") AND account_recipient_id IS NOT NULL"
	all := append(append([]any{}, args...), args...)
	rows, err := b.tm.Querier(ctx).QueryContext(ctx, b.rebind(q), all...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.Bump(ctx, ids...)
}

// One UPDATE per account in sorted order: a consistent row-lock order, so two
// PostgreSQL transactions bumping overlapping accounts cannot deadlock.
func (b *Bumper) flush(ctx context.Context, ids []string) error {
	ids = dedupeSorted(ids)
	q := b.rebind("UPDATE accounts SET transactions_revision = transactions_revision + 1 WHERE id = ?")
	for _, id := range ids {
		if _, err := b.tm.Querier(ctx).ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}
```
`rebind` converts each `?` to `$1..$n` when `driver == "postgresql"` (write a 10-line helper; the condition strings never contain literal `?`). `dedupeSorted` drops empty strings, dedupes, `sort.Strings`.

- [ ] **Step 9: Run** txrevision tests (sqlite), then with Postgres if available: `DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/infra/txrevision/` (skip if no `DATABASE_TEST_PGSQL_URL`). → PASS.

- [ ] **Step 10:** `gofmt -l internal/ && go vet ./internal/infra/...` clean. Commit `feat(storage): Commit-time hook and account transactions revision bumper`.

---

### Task 2: Revision on the account DTO

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/accounts.sql` (`GetAccountByID`, `ListAvailableAccounts`, `ListDeletedAccounts` select lists — all three, both engines, or sqlc emits a separate Row type and the `accountRow` alias breaks; NOT in `UpsertAccount`'s SET), regenerate sqlc.
- Modify: `internal/model/account.go` (`TransactionsRevision int64` on `Account`), `internal/model/account_dto.go` (`TransactionsRevision int64 \`json:"transactionsRevision"\`` on `AccountResult`, placed after `SharedAccess`), `internal/account/repo/repo.go` (`hydrateAccount`, pgsql conversion), `internal/account/usecase.go:158` (`buildAccountResult` literal).
- Modify: `internal/account/create.go`, `internal/account/update.go` — after the `WithTx` that may write a correction, re-read the revision (add repo method `TransactionsRevision(ctx, id vo.Id) (int64, error)` with a sqlc query `GetAccountTransactionsRevision :one` in both engines) and set it on the returned item.
- Test: `internal/account/api/*_test.go` or `internal/account/*_test.go` (follow existing account tests).

**Interfaces:**
- Consumes: Task 1 migration.
- Produces: `model.AccountResult.TransactionsRevision int64` (JSON `transactionsRevision`) on every account DTO: `get-account-list`, transaction create/update/delete `accounts`, recurring post `accounts`, account create/update `item`.

- [ ] **Step 1: Failing test** — create an account with a non-zero opening balance via the service/API harness; assert the returned `item.transactionsRevision == 1` (the correction bumped it) and `get-account-list` shows 1; update its balance → response shows 2. (Account create/update bumps come from Task 3's `SaveCorrection` bump; until Task 3 lands, assert the field is present and equals the DB column — then tighten to 1/2 in Task 3. To avoid churn, implement this task's test as "item revision == `SELECT transactions_revision` after the request".)
- [ ] **Step 2:** Run → FAIL (field missing).
- [ ] **Step 3:** Implement queries, regenerate sqlc (`make sqlc` or the documented command), model, hydrate, DTO, fresh re-read in create/update.
- [ ] **Step 4:** `go build ./... && go test ./internal/account/...` → PASS.
- [ ] **Step 5:** `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`; inspect `git diff --stat internal/test/*/testdata` — only `"transactionsRevision": N` additions in account objects are acceptable. Re-run without `UPDATE_GOLDEN` → PASS.
- [ ] **Step 6:** `make swagger` (account DTO doc changes). Commit `feat(account): Expose transactionsRevision on the account DTO`.

---

### Task 3: Bump sites — transaction and account repos

**Files:**
- Modify: `internal/transaction/repo/repo.go` (`NewRepo` builds `rev: txrevision.New(driver, tx)`; `Save`, `ReplaceLabels`, `LinkRecurring`, `Delete`), `internal/account/repo/repo.go` (`SaveCorrection`; store `rev` in `NewRepo`).
- Test: `internal/transaction/repo/repo_integration_test.go` (or a new `revision_test.go` next to it), `internal/account/*_test.go`.

**Interfaces:**
- Consumes: `txrevision.Bumper` (Task 1).
- Produces: the invariant "any write through these repos bumps the old and new `account_id`/`account_recipient_id` exactly once per DB transaction".

Implementation per method (all inside the repo, before/after the existing query):
- `Save`: `r.rev.BumpWhere(ctx, "id = ?", t.ID.String())` **before** the upsert (captures the old accounts if the row exists), then upsert, then `r.rev.Bump(ctx, t.AccountID.String(), recipient)` (recipient `""` when nil).
- `ReplaceLabels`: `r.rev.BumpWhere(ctx, "id = ?", transactionID.String())` (the row exists by then).
- `LinkRecurring`: `BumpWhere("id = ?", id)`.
- `Delete`: `BumpWhere("id = ?", id)` **before** the delete.
- `account` `SaveCorrection`: `r.rev.Bump(ctx, c.AccountID.String())` (check the struct's field name).

- [ ] **Step 1: Failing tests** through the services (use the transaction api harness in `internal/transaction/api/harness_test.go` so requests run their real `WithTx`). Read revisions with raw SQL. Exactly-+1 cases:
  - create expense with 2 labels → account +1;
  - create transfer A→B → A +1, B +1;
  - update moving expense from A to C → A +1, C +1, B 0;
  - update a transfer A→B into A→C → A +1, B +1, C +1;
  - delete → +1;
  - link to recurring (create recurring "from transaction") → +1;
  - CSV import of 3 rows into A → A +1 (one request = one tx);
  - account update with balance change (writes correction) → +1;
  - transaction delete on a transfer whose other side is a deleted account (the §5a re-zero correction) → live account +1 and response `accounts` carry it.
  - Response `accounts[].transactionsRevision` equals the DB value after the request (for create/update/delete).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test ./internal/transaction/... ./internal/account/... ./internal/recurring/...` → PASS. Also tighten Task 2's account create/update test to exact values.
- [ ] **Step 5:** Goldens: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`, inspect (revision numbers in account objects change), re-run → PASS. Commit `feat(transaction): Bump account revisions on every transaction write`.

---

### Task 4: Bump sites — classifications, recurring, imports, user

**Files:**
- Modify: `internal/{category,payee,tag}/repo/repo.go` — `ReassignTransactions` (bump `"<fk> = ?"` with the OLD id **before** the update), `Delete` (bump `"<fk> = ?"` **before** delete; `<fk>` = `category_id` / `payee_id` / `tag_id`).
- Modify: `internal/label/repo/repo.go` — `ReassignTransactions` and `Delete`: `BumpWhere("id IN (SELECT transaction_id FROM transactions_labels WHERE label_id = ?)", oldOrDeletedID)` before the write.
- Modify: `internal/recurring/repo/repo.go` — `Delete`: `BumpWhere("recurring_id = ?", id)` before.
- Modify: `internal/imports/repo/repo.go` — `InsertLink`: if `l.TransactionID != nil` → `BumpWhere("id = ?", *l.TransactionID)`; `UpdateLink`: bump the transaction the link pointed to before (read it: `SELECT transaction_id FROM import_transaction_links WHERE id = ?`) and the new one; `DeleteSource`: `BumpWhere("id IN (SELECT transaction_id FROM import_transaction_links WHERE source_id = ?)", id)` before; `DeleteQueuedLinksByExternalAccount`: queued links have no transaction — verify with the SQL; if the query can delete linked rows, bump like `DeleteSource` scoped by external account.
- Modify: `internal/user/repo/repo.go` — `Save`: read the stored `name`, `avatar` for `u.ID` first (one small query, `sql.ErrNoRows` = new user = no bump); after the upsert, if either changed, `BumpWhere("user_id = ?", u.ID.String())`.
- Each `NewRepo` constructs `rev: txrevision.New(driver, tx)` (several repos currently discard `driver` — keep it for the bumper).
- Tests: one per site in each feature's existing test layout (`internal/<feature>/*_test.go` or `api/*_test.go`).

**Interfaces:**
- Consumes: `txrevision.Bumper` (Task 1).
- Produces: bumps for every remaining write that changes a row as returned by `get-account-transaction-list` (fields, labels, `isImported`, author name/avatar).

- [ ] **Step 1: Failing tests** — for each: seed accounts A (has a matching transaction), B (has a transfer *into* it from A matching), C (no match); perform the API call; assert A and B moved (≥1), C did not. Sites: category merge, category delete, payee merge/delete, tag merge/delete, label merge/delete, recurring delete (on a template with posted transactions), import source delete (with linked transactions), import adopt (ingest an Apple Wallet event that matches an existing manual transaction — see `internal/imports` tests for the matcher fixture), user `update-name` (bumps every account where the user authored transactions, incl. a partner's shared account) and `update-avatar`; `update-name` to the **same** name → no bump; login → no bump.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement. **Step 4:** `go test ./internal/...` → PASS.
- [ ] **Step 5:** Goldens regenerate + inspect + re-run. `make go-lint`. Commit `feat: Bump account revisions on classification, recurring, import and author changes`.

---

### Task 5: Endpoint `GET /api/v1/transaction/get-account-transaction-list`

**Files:**
- Modify: `internal/model/transaction_dto.go` — request + result types.
- Modify: `internal/transaction/repository.go` (+ `repo/repo.go`) — `TransactionsRevisions(ctx, accountIDs []vo.Id) (map[string]int64, error)` (hand-built `SELECT id, transactions_revision FROM accounts WHERE id IN (...)`, placeholders like `ListByAccountIDs`).
- Modify: `internal/transaction/read.go` — `GetAccountTransactionList`.
- Create: `internal/transaction/api/accounttransactionlist.go`; Modify: `internal/transaction/api/routes.go`.
- Modify: `internal/test/apiparity/catalogue_*.go` (the transaction catalogue file) + `guard_test.go` (`minRoutes` 133 → 134), goldens; `make swagger`.
- Test: `internal/transaction/api/account_transaction_list_test.go`, `internal/model/transaction_dto_test.go` (if model tests exist).

**Interfaces:**
- Produces (Go):
```go
type AccountTransactionListRequest struct {
	AccountIds string // raw comma-separated query value; "" = all accessible
}

func (r AccountTransactionListRequest) Validate() error // present-but-empty ("accountIds=" with only commas/whitespace), non-UUID, or >200 ids -> ValidationError on field "accountIds"
func (r AccountTransactionListRequest) IDs() []vo.Id   // parsed, deduplicated; nil when absent

type AccountTransactionItem struct { // TransactionResult minus Author, plus AuthorId
	Id                 string   `json:"id"`
	AuthorId           string   `json:"authorId"`
	Type               string   `json:"type"`
	AccountId          string   `json:"accountId"`
	AccountRecipientId *string  `json:"accountRecipientId"`
	Amount             string   `json:"amount"`
	AmountRecipient    *string  `json:"amountRecipient"`
	CategoryId         *string  `json:"categoryId"`
	Description        string   `json:"description"`
	PayeeId            *string  `json:"payeeId"`
	TagId              *string  `json:"tagId"`
	Date               string   `json:"date"`
	RecurringId        *string  `json:"recurringId"`
	LabelIds           []string `json:"labelIds"`
	IsImported         int      `json:"isImported"`
}
// Copy field names/order/encodings EXACTLY from TransactionResult (internal/model/transaction_dto.go:18-41) — the above is the expected shape; if TransactionResult differs, follow it.

type GetAccountTransactionListResult struct {
	Revisions map[string]int64         `json:"revisions"`
	Items     []AccountTransactionItem `json:"items"`
	Authors   []UserResult             `json:"authors"` // sorted by id
}
```
The handler distinguishes "absent" (`!q.Has("accountIds")`) from "present but empty" (400).
- Produces (HTTP): `GET /api/v1/transaction/get-account-transaction-list[?accountIds=a,b]` → `{revisions, items, authors}`.

Service algorithm (`GetAccountTransactionList(ctx, userID, req)`):
1. `available := s.visible.AvailableAccountIDs(ctx, userID)` (owned + accepted shares, not deleted, hidden folders included).
2. Target = `available` when no ids; else `requested ∩ available` (requested-but-inaccessible are silently dropped).
3. `revisions := repo.TransactionsRevisions(ctx, target)` — **before** rows.
4. `rows := repo.ListByAccountIDs(ctx, target, model.TransactionFilter{})` (already `account_id IN OR account_recipient_id IN`, ordered, one row each).
5. Labels (`LabelsByTransactionIDs`) and `isImported` exactly as `GetTransactionList` does (reuse its helpers in `read.go`).
6. Authors: distinct `UserID`s → resolve via the same author lookup `GetTransactionList` uses (`read.go:108-121`), sorted by id.
7. Empty target → `{revisions: {}, items: [], authors: []}` (never `null`).

- [ ] **Step 1: Failing API tests**: owner without ids gets all own + accepted-shared + hidden-folder accounts; with ids returns only those; transfer between two requested accounts appears once; foreign id omitted from `revisions` (200, not 403); deleted account omitted; pending (unaccepted) share omitted; `accountIds=` → 400; `accountIds=nope` → 400; 201 ids → 400; `revisions` value equals DB column; `authors` deduped & sorted; unauthenticated → 401.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement (handler mirrors `transactionlist.go`'s hand-written pattern with swag annotations: `@Param accountIds query string false "Comma-separated account ids (max 200); omit for all accessible accounts"`, `@Success 200 {object} apidoc.JsonResponseOk{data=model.GetAccountTransactionListResult}`, `@Failure 400/401/500`, `@Security Bearer`, `@Router /api/v1/transaction/get-account-transaction-list [get]`). Register `mux.Handle("GET /api/v1/transaction/get-account-transaction-list", auth(h.GetAccountTransactionList))`.
- [ ] **Step 4:** Add apiparity scenario(s) covering: no ids, ids incl. a foreign one, `accountIds=` 400. Raise `minRoutes` to 134 and the catalogue-size floor if a new scenario name pushes it. `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect, re-run → PASS. `make swagger`, `make go-lint`.
- [ ] **Step 5:** Commit `feat(transaction): get-account-transaction-list batch read with revisions`.

---

### Task 6: Revision guard suite

**Files:**
- Modify: `internal/test/apiparity/harness.go` — add `Observe func(t *testing.T, phase string, label string)` field on `Harness`; call it with `"before"` / `"after"` around the HTTP round trip in `do` (label = `method+" "+path`).
- Modify: `internal/test/mcpparity/mcpparity.go` — export a runner the guard can drive (e.g. `func RunSteps(t *testing.T, h *apiparity.Harness, s Scenario)` wrapping `runSteps`), and make `postMCP` call `h.Observe` around the RPC (pass `h` into it).
- Create: `internal/test/revisionguard/guard.go`, `smoke_test.go`, `enginecompare_test.go` (build tag `enginecompare`, `dbtest.NewPostgres`, skipped without `DATABASE_TEST_PGSQL_URL`).

**Interfaces:**
- Consumes: apiparity `Catalogue()`, `NewHarness`, `Replay`; mcpparity `Catalogue()`; Task 2–4 bumps.
- Produces: `func Check(t *testing.T, newDB func(t *testing.T) *dbtest.DB)` running both catalogues.

Fingerprint (raw SQL on `h.db.Raw`, engine-neutral; use `db.Rebind` where params are used):
```sql
SELECT a.id, a.transactions_revision,
       t.id, t.user_id, t.account_id, t.account_recipient_id, t.category_id, t.payee_id, t.tag_id,
       t.description, t.spent_at, t.type, t.amount, t.amount_recipient, t.recurring_id,
       u.name, u.avatar
FROM accounts a
LEFT JOIN transactions t ON t.account_id = a.id OR t.account_recipient_id = a.id
LEFT JOIN users u ON u.id = t.user_id
ORDER BY a.id, t.id
```
plus `SELECT transaction_id, label_id FROM transactions_labels ORDER BY 1,2` and `SELECT DISTINCT transaction_id FROM import_transaction_links WHERE transaction_id IS NOT NULL`. Per account: sha256 over its rows' normalized text (amounts via `vo.Decimal` normalization or `CAST(... AS TEXT)` trimmed — engines differ in decimal text; normalize trailing zeros in Go), its rows' sorted label ids, and their isImported flags. `revision` kept separately.

`Observe("before")` snapshots `map[accountID]{fp, rev}`; `Observe("after")` snapshots again; for every account present in both whose `fp` changed while `rev` did not → `t.Errorf("%s: account %s transactions changed without a revision bump", label, id)`. New accounts and removed accounts are ignored. Extra bumps are fine.

- [ ] **Step 1:** Write `guard.go` + `smoke_test.go` (`revisionguard.Check(t, dbtest.NewSQLite)`, one subtest per scenario).
- [ ] **Step 2: Prove it bites** — temporarily comment out the bump in `category` `ReassignTransactions`, run `go test ./internal/test/revisionguard/` → FAIL naming `POST /api/v1/category/merge-category` (or whatever the merge route is); restore → PASS. Also note in the commit message that this was checked.
- [ ] **Step 3:** Fix any real misses the guard reports (each one is a missing bump site from Task 4 — add it in the owning repo with a unit test).
- [ ] **Step 4:** `enginecompare_test.go`; run `go test -tags enginecompare ./internal/test/revisionguard/` with Postgres if available. Ensure `make go-test` picks up the smoke test (it runs `./...`). Commit `test: Revision guard over the API and MCP catalogues`.

---

### Task 7: One IndexedDB database `econumo` (`web/src/lib/db.ts`) + importCrypto migration

**Files:**
- Create: `web/src/lib/db.ts`, `web/src/lib/db.test.ts`
- Modify: `web/src/lib/importCrypto.ts` (drop its own `openDb`/`tx`; use `db.ts`), keep `importCrypto.test.ts` passing.

**Interfaces:**
- Produces:
```ts
export const STORE_IMPORT_KEYS = 'importKeys'
export const STORE_ACCOUNT_TRANSACTIONS = 'accountTransactions'
export const STORE_META = 'meta'
export function openDb(): Promise<IDBDatabase>           // cached per page; reset on close/versionchange
export function dbRequest<T>(store: string, mode: IDBTransactionMode, run: (s: IDBObjectStore) => IDBRequest<T>): Promise<T>
export function dbTransaction(stores: string[], mode: IDBTransactionMode, run: (t: IDBTransaction) => void): Promise<void> // resolves on complete, rejects on error/abort
export function resetDbForTests(): void                  // drops the cached connection
```

Implementation notes:
- `indexedDB.open('econumo', 1)`; `onupgradeneeded`: create `importKeys` (no keyPath), `accountTransactions` (`keyPath: 'accountId'`), `meta` (no keyPath).
- `db.onversionchange = () => { db.close(); cached = null }`; `onclose` also clears the cache.
- Migration from `econumo-import` inside `openDb()` once per page, after the new db is open: `indexedDB.open('econumo-import')` — if `onupgradeneeded` fires (the old db did not exist), abort that request (`req.transaction?.abort()`) and stop; otherwise read `keys`/`dataKey`, `put` it into `importKeys` under `'dataKey'` only if `importKeys` has none, close, then `indexedDB.deleteDatabase('econumo-import')`. Any migration error is swallowed with one `console.warn` (the user can re-unlock).
- `importCrypto.ts`: `STORE`→`STORE_IMPORT_KEYS`, `RECORD` stays `'dataKey'`; `loadStoredKey`/`storeKey`/`forgetKey` call `dbRequest`.

- [ ] **Step 1: Failing tests** (`beforeEach`: `globalThis.indexedDB = new IDBFactory(); resetDbForTests()`):
  - fresh open creates the three stores;
  - an existing `econumo-import` db with a `{key: <CryptoKey>, tag: 't'}` record → after `openDb()`, `loadStoredKey()` returns it with tag `'t'` and `indexedDB.databases()` no longer lists `econumo-import`;
  - an existing key in `importKeys` is not overwritten by the migration;
  - no `econumo-import` → no `econumo-import` db is left created;
  - `versionchange` (open `econumo` at version 2 from the test) → the cached connection closes and the next `openDb()` reopens without blocking.
- [ ] **Step 2:** `cd web && pnpm vitest run src/lib/db.test.ts src/lib/importCrypto.test.ts` → FAIL.
- [ ] **Step 3:** Implement. **Step 4:** Run → PASS; `pnpm vitest run src/features/imports` → PASS.
- [ ] **Step 5:** Commit `feat(web): One econumo IndexedDB; move the SimpleFIN key into importKeys`.

---

### Task 8: Transaction store (`web/src/lib/transactionStore.ts`)

**Files:** Create `web/src/lib/transactionStore.ts`, `web/src/lib/transactionStore.test.ts`.

**Interfaces:**
- Consumes: `db.ts` (Task 7); `getVersion()` from `@/lib/config`.
- Produces:
```ts
import type { UserDto } from '@/api/dto/user'
import type { StoredTransaction } from '@/api/dto/transaction' // Task 9 adds it; define it here first if Task 9 is later and move it

export interface Bucket {
  accountId: string
  revision: number
  items: StoredTransaction[]
  authors: UserDto[]
}
export const FORMAT_VERSION = 1
export function storeBuster(): string                      // `${FORMAT_VERSION}:${getVersion()}`
export async function loadBuckets(userId: string): Promise<Map<string, Bucket>>   // wipes first if meta.buster/userId differ; writes fresh meta
export async function putBuckets(userId: string, buckets: Bucket[]): Promise<void>
export async function deleteBuckets(accountIds: string[]): Promise<void>
export async function clearTransactions(): Promise<void>  // accountTransactions + meta only; importKeys untouched
export function resetTransactionStoreForTests(): void
```
- Failure policy: every function catches, `console.warn`s once per session (`warned` flag), and flips `disabled = true`; while disabled, `loadBuckets` returns an empty map and writes are no-ops. Callers never see a rejection.

- [ ] **Step 1: Failing tests**: round trip put/load; `loadBuckets('u2')` after data saved for `'u1'` → empty and old data gone; changed `getVersion()` (mock `@/lib/config`) → wiped; `clearTransactions()` leaves an `importKeys` record intact; when `indexedDB.open` throws (stub `globalThis.indexedDB.open = () => { throw new Error('x') }`) → `loadBuckets` resolves to empty map, `putBuckets` resolves, exactly one `console.warn`.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement. **Step 4:** Run → PASS. **Step 5:** Commit `feat(web): IndexedDB transaction buckets store`.

---

### Task 9: API client + pure sync engine

**Files:**
- Modify: `web/src/api/dto/account.ts` (`transactionsRevision?: number` on `AccountDto` — optional because older servers omit it), `web/src/api/dto/transaction.ts` (add `StoredTransaction` = `Omit<TransactionDto, 'author'> & { authorId: string }` and `AccountTransactionListDto`), `web/src/api/transaction.ts` (`getAccountTransactionList(accountIds?: string[])`).
- Create: `web/src/features/transactions/sync.ts`, `web/src/features/transactions/sync.test.ts`.
- Modify: `web/src/test/fixtures.ts` — `coreHandlers` gains a default `get-account-transaction-list` handler deriving from the same fixture transactions as `get-transaction-list` (so existing tests keep their data), and fixture accounts get `transactionsRevision: 1`.

**Interfaces:**
- Produces:
```ts
export interface AccountTransactionListDto {
  revisions: Record<string, number>
  items: StoredTransaction[]
  authors: UserDto[]
}
export async function getAccountTransactionList(accountIds?: string[]): Promise<AccountTransactionListDto>
// GET /api/v1/transaction/get-account-transaction-list, params: accountIds ? { accountIds: accountIds.join(',') } : undefined

// sync.ts
export const MAX_IDS = 200
export interface SyncInput {
  accounts: AccountDto[]            // already filtered of pending-for-me invites
  stored: Map<string, Bucket>
  full: boolean
  fetch: (accountIds?: string[]) => Promise<AccountTransactionListDto>
}
export interface SyncOutput {
  buckets: Map<string, Bucket>      // the complete new state
  changed: Bucket[]                 // to putBuckets
  removed: string[]                 // to deleteBuckets
}
export async function syncBuckets(input: SyncInput): Promise<SyncOutput>
export function assemble(buckets: Map<string, Bucket>): TransactionDto[]   // dedupe by id, author filled from the bucket's authors (fallback {id, name:'', avatar:''})
export function hasRevisions(accounts: AccountDto[]): boolean               // every account has a numeric transactionsRevision
```
`syncBuckets` algorithm:
```ts
const ids = new Set(input.accounts.map((a) => a.id))
const stale = input.full ? [...ids] : [...ids].filter((id) => input.stored.get(id)?.revision !== revOf(id))
let requested: string[] = []
let res: AccountTransactionListDto | null = null
if (stale.length > 0) {
  const all = input.full || input.stored.size === 0 || stale.length > MAX_IDS
  requested = all ? [...ids] : stale
  res = await input.fetch(all ? undefined : stale)
}
// rebuild a bucket for every requested id present in res.revisions:
//   items = res.items.filter(t => t.accountId === id || t.accountRecipientId === id)
//   authors = res.authors whose id appears in those items
// removed = stored ids not in `ids`, plus requested ids absent from res.revisions
// buckets = stored minus removed, overlaid with rebuilt
```

- [ ] **Step 1: Failing tests** (pure, no IDB — `fetch` is a `vi.fn`): missing bucket → fetched; equal revision → `fetch` not called, output equals input; one changed of three → `fetch(['b'])`; first sync (empty stored) → `fetch(undefined)`; 201 stale → `fetch(undefined)`; `full` → `fetch(undefined)` even when all equal; account gone from list → in `removed`; requested but absent from `revisions` → in `removed`; transfer A→B both requested → the item lands in both buckets and `assemble` returns it once; `assemble` fills `author` from authors; `hasRevisions` false when any account lacks the field.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement (+ API client + fixture handler; add an `api/transaction.test.ts` case asserting the comma-joined param and the omitted param). **Step 4:** Run `pnpm vitest run src/features/transactions src/api` → PASS. **Step 5:** Commit `feat(web): Revision-driven transaction sync engine`.

---

### Task 10: Wire the sync into `['transactions']`, follow accounts, write-through, +1 shortcut

**Files:**
- Create: `web/src/features/transactions/revisionTracking.ts` (+ `revisionTracking.test.ts`).
- Modify: `web/src/features/transactions/queries.ts`, `web/src/features/transactions/useAccountTransactions.ts`, `web/src/app/queryKeys.ts` (remove `accountTransactions`), `web/src/features/accounts/queries.ts` (account item effects + share accept + delete/leave: keep existing patches; add the +1 call where a response carries accounts), `web/src/features/recurring/queries.ts` (post), `web/src/app/queryClient.ts` (install tracking once), tests that relied on `get-transaction-list` for hidden-folder accounts (`useAccountTransactions.test.tsx` ~L240-268) — rewrite them to the new behaviour (hidden-folder account's rows come from the global list).

**Interfaces:**
- Consumes: Tasks 8–9.
- Produces:
```ts
// revisionTracking.ts
export function installTransactionTracking(queryClient: QueryClient): () => void
//   1) accounts subscription: on queryCache 'updated' for queryKeys.accounts with new data,
//      compare (pending-filtered) ids + transactionsRevision against the in-memory revision map;
//      if any differs/appeared/disappeared -> queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
//   2) write-through: on 'updated' for queryKeys.transactions (state.data changed, not from sync),
//      debounce 500 ms, regroup rows by account (accountId and accountRecipientId),
//      rebuild buckets for accounts whose row set changed (compare by object identity per account),
//      keep each bucket's revision, putBuckets
export function noteOwnWrite(accounts: AccountDto[], touchedAccountIds: string[]): void
//   for each touched id: if accounts[id].transactionsRevision === memory[id] + 1 -> memory[id] = that, and mark the bucket revision for the next write-through
export function transactionsQueryFn(queryClient: QueryClient, opts: { full: boolean }): Promise<TransactionDto[]>
export function requestFullReload(): void   // sets the one-shot `full` flag consumed by the next queryFn run
```
`transactionsQueryFn`:
1. `const user = await queryClient.fetchQuery({ queryKey: queryKeys.user, queryFn: getUserData, staleTime: TEN_MINUTES })`
2. `const raw = await queryClient.fetchQuery({ queryKey: queryKeys.accounts, queryFn: accountApi.getAccountList, staleTime: TEN_MINUTES })`; `accounts = raw.filter((a) => !isPendingForMe(a, user.id))` (import `isPendingForMe` from where `useAccounts` gets it).
3. `if (!hasRevisions(accounts)) return transactionApi.getTransactionList()` (legacy; no IDB).
4. `stored = await loadBuckets(user.id)`; `out = await syncBuckets({ accounts, stored, full, fetch: getAccountTransactionList })`; `await putBuckets(user.id, out.changed)`; `await deleteBuckets(out.removed)`; update the in-memory revision map; `return assemble(out.buckets)`.
`useTransactions` keeps key, `staleTime: TEN_MINUTES`, `select: byDateDesc`; `queryFn: () => transactionsQueryFn(queryClient, { full: consumeFullFlag() })`.
Remove `useAccountTransactionList`, `queryKeys.accountTransactions`, and the hidden-folder branch in `useAccountTransactions.ts` (always use the global list). `useApplyTransactionItem`: drop the `accountTransactions` invalidation; before `setQueryData`, `await queryClient.cancelQueries({ queryKey: queryKeys.transactions })` is not possible synchronously — call `void queryClient.cancelQueries(...)` first, then patch, then `noteOwnWrite(result.accounts, [item.accountId, item.accountRecipientId, prev?.accountId, prev?.accountRecipientId])` (prev = the row being replaced/deleted from the cache before patching).

- [ ] **Step 1: Failing tests** (`revisionTracking.test.ts` with msw + real `QueryClient`, fake IDB):
  - boot with empty IDB → one `get-account-transaction-list` without `accountIds`; list returned; buckets persisted;
  - second `invalidateQueries` with unchanged revisions → zero transaction requests;
  - accounts refetch shows account B rev+2 → one request `accountIds=B`;
  - a create mutation response with A at stored+1 → no transaction request afterwards; at stored+2 → one request for A;
  - category-merge style `setQueryData` patch → after 500 ms (fake timers) the IDB bucket reflects the patch;
  - hidden-folder account's rows are in `useTransactions` data;
  - accounts without `transactionsRevision` → `get-transaction-list` is called and IDB stays empty;
  - a pending-invite account is never requested.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement. **Step 4:** `pnpm test` (whole suite) → PASS; fix tests that asserted the old hidden-folder fetch. **Step 5:** `pnpm lint && pnpm exec tsc -b` clean. Commit `feat(web): Transactions sync from IndexedDB by account revision`.

---

### Task 11: Boot restore, persistence exclusion, async clear

**Files:**
- Modify: `web/src/lib/queryPersist.ts`, `web/src/main.tsx`, `web/src/features/auth/queries.ts`, `web/src/features/auth/LogoutPage.tsx`, `web/src/features/auth/oauthQueries.ts`, `web/src/app/layouts/ApplicationLayout.test.tsx`.

**Interfaces:**
- Consumes: Tasks 8–10.
- Produces:
  - `shouldDehydrateQuery` also excludes `queryKeys.transactions[0]` (`'transactions'`).
  - `createPersistOptions()` wraps the sync persister. The wrapper's `restoreClient` awaits the inner restore, then seeds transactions **inside the restore** — so `isRestoring` still covers it and the boot loader never flashes (seeding in `onSuccess` would run after `isRestoring` flips false): read `user` and `accounts` from the snapshot's `clientState.queries` (match by `queryKey`), `loadBuckets(user.id)`, keep buckets of snapshot accounts (pending-filtered), and append a dehydrated query for `queryKeys.transactions` (`state.data = assemble(...)`, `status: 'success'`, `dataUpdatedAt` = snapshot `timestamp`) to the returned snapshot. Also call `seedRevisions(buckets)` (export from `revisionTracking.ts`) so the in-memory revision map matches. No user/accounts in the snapshot → no seeding.
  - `clearPersistedQueryCache(): Promise<void>` — removes the localStorage key and awaits `clearTransactions()`; also resets the in-memory revision map.
- Callers await it: `useLogin.onSuccess` becomes `async`; `LogoutPage` awaits before `window.location.assign`; `oauthQueries` awaits before `setToken`.

- [ ] **Step 1: Failing tests** in `ApplicationLayout.test.tsx` (`renderShellPersisted` updated to use `createPersistOptions`):
  - persisted snapshot (accounts + user) + IDB buckets → shell renders the account's transactions with **no** boot loader and before any network response (delay msw handlers);
  - the localStorage snapshot no longer contains a `transactions` query;
  - IDB data for user `u1` + snapshot user `u2` → no transactions seeded;
  - `clearPersistedQueryCache()` empties IDB buckets and the localStorage key.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement. **Step 4:** `pnpm test` → PASS. **Step 5:** Commit `feat(web): Start from IndexedDB transactions; clear them on logout and login`.

---

### Task 12: Sync button — tap incremental, long-press / Shift+click full reload

**Files:**
- Create: `web/src/hooks/useLongPress.ts`, `web/src/hooks/useLongPress.test.tsx`.
- Modify: `web/src/app/layouts/ApplicationLayout.tsx` (both buttons), `web/src/app/layouts/ApplicationLayout.test.tsx`, `web/src/lib/metrics.ts`, `locales/*.json` (11 files).

**Interfaces:**
- Consumes: `requestFullReload`, `clearTransactions` (Tasks 8, 10).
- Produces:
```ts
export function useLongPress(opts: { onClick: (e: React.MouseEvent) => void; onLongPress: () => void; ms?: number }): {
  onPointerDown: React.PointerEventHandler
  onPointerUp: React.PointerEventHandler
  onPointerLeave: React.PointerEventHandler
  onPointerCancel: React.PointerEventHandler
  onClick: React.MouseEventHandler
  onContextMenu: React.MouseEventHandler   // preventDefault while a press is active (iOS/Android callout)
}
// ms default 600; the click that follows a fired long-press is swallowed; Shift+click calls onLongPress instead of onClick.
```
- `METRICS.DATA_FULL_RELOAD: 'appDataFullReload'`.
- i18n `settings.sync.full_reload` ("Reloading all data from the server…"), `settings.sync.hold_hint` ("Hold to reload everything").

Button wiring (both variants):
```tsx
const fullReload = useCallback(async () => {
  trackEvent(METRICS.DATA_FULL_RELOAD)
  toast(t('settings.sync.full_reload'), { id: 'full-reload' })
  await clearTransactions()
  requestFullReload()
  void queryClient.invalidateQueries()
}, [queryClient, t])
const press = useLongPress({ onClick: () => void queryClient.invalidateQueries(), onLongPress: () => void fullReload() })
// <button ... title={`${syncTitle}\n${t('settings.sync.hold_hint')}`} {...press} className={`${syncClass} select-none [-webkit-touch-callout:none]`}>
```

- [ ] **Step 1: Failing tests**: `useLongPress` (fake timers): down→up at 300 ms → `onClick`; down→600 ms → `onLongPress`, then the click is swallowed; down→leave at 300 ms→ neither; Shift+click → `onLongPress`. ApplicationLayout: tap → `invalidateQueries` and no `accountIds`-less request when revisions are equal; long-press → toast text visible, metric fired (spy on `trackEvent` like other tests do), next transactions request has no `accountIds`.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement + add the two keys to all 11 catalogues (real translations). **Step 4:** `pnpm test`, `pnpm lint`, `go test ./internal/test/i18ntest/ ./locales/` → PASS. **Step 5:** Commit `feat(web): Hold the sync button to reload everything`.

---

### Task 13: Docs + full verification

**Files:** `docs/regression-test-plan.md`, `CLAUDE.md`.

- [ ] **Step 1:** Regression plan: add the items listed in the spec's Docs section (offline reload 📱; cross-device edit after refocus; partner edit on a shared account; accept/revoke share adds/removes history; hidden-folder account page shows transactions and global search finds them; logout → other user sees nothing of the previous; sync tap; long-press 📱 / Shift+click full reload with toast). Edit the existing hidden-folder / sync items that describe the old behaviour.
- [ ] **Step 2:** CLAUDE.md: (a) under Architecture/Database — the `transactions_revision` counter, "bump inside the repository method via `internal/infra/txrevision`; flushed once per DB transaction before COMMIT in sorted order", and the `internal/test/revisionguard` suite; (b) API conventions / transaction feature — `get-account-transaction-list` (absent `accountIds` = all, max 200, inaccessible ids omitted); (c) Frontend architecture — the single `econumo` IndexedDB (`importKeys`, `accountTransactions`, `meta`), transactions not in the localStorage cache, sync button tap vs hold; update the `imports` paragraph's mention of `econumo-import` if any.
- [ ] **Step 3: Full verification** (record output): `make go-test`; `make test-repo-pgsql` and `go test -tags enginecompare ./internal/test/...` if a Postgres is reachable (see memory/CLAUDE.md recipe; else state it was not run); `cd web && pnpm test && pnpm lint && pnpm build`.
- [ ] **Step 4:** Commit `docs: Transactions IndexedDB sync in the regression plan and CLAUDE.md`.
