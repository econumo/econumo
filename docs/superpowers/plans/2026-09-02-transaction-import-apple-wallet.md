# Transaction Import — Stage 2 (Apple Wallet) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the first end-to-end import provider: an iPhone Shortcut POSTs every Apple Wallet tap to `ingest-apple-wallet-event`, the server normalizes it, maps the card to an account, converts currency, matches or creates the transaction, and queues anything it cannot decide; the SPA gets Settings → Import & export (Apple Wallet setup + card mapping), a queue page with a banner, and an "imported" badge with provenance on transactions.

**Architecture:** The `internal/imports` package (stage 1: persistence + pure matcher) grows an ingest pipeline `Service`, consumer-side ports for account/currency/transaction capabilities (wired by `internal/server/glue_imports.go`), and its first HTTP edge `internal/imports/api` under `/api/v1/import/`. Every ingest runs inside one `TxManager.WithTx` and always persists the raw payload first (the inbox), so the response is `200 {status}` for every parsable-or-not body once stored. The SPA adds a `features/imports` slice (API client, hooks, Data page, queue page, banner) and reuses the existing transaction dialog for per-event triage.

**Tech Stack:** Go 1.2x stdlib (`net/http.ServeMux`, `log/slog`), sqlc (sqlite + pgsql query files, `go generate ./internal/infra/storage/sqlc/`), `modernc.org/sqlite`, `jackc/pgx/v5`; React 19 + Vite + TypeScript, TanStack Query, Zustand, react-i18next, vitest, oxlint; `docs/regression-test-plan.md` for manual QA.

**Spec:** `docs/superpowers/specs/2026-08-15-transaction-import-design.md` — Part 2 (`import_account_links`, card→account flow, link row states), Part 5 (pipeline stages 0–3), Part 6 (ingest payload, parse table, response statuses, rate limit, queue semantics), Part 8 (endpoint table), Part 9 (Settings → Data, Apple Wallet setup, queue page + banner, analytics keys), Testing list, staging item 2, Part 4 (`isImported`). Stage 1 (merged as PR #230) is the base: schema, `model.Import*`, `imports.Repository`, `imports.Match`, `imports.HashPayload`, ingest-scoped PATs.

## Global Constraints

- Features never import features (`internal/test/archtest`). `imports` declares its ports in `internal/imports/ports.go` typed in `model`/`vo`; adapters live in `internal/server/glue_imports.go`.
- Engine-adapter pattern: every new query exists in BOTH `internal/infra/storage/sqlc/query/sqlite/imports.sql` and `.../pgsql/imports.sql`; the `querier` interface is in sqlite-generated types; `sqlite.go` passes through, `pgsql.go` converts whole structs. pgsql files put `;` on its own line; sqlite files end statements inline (a `;` on its own line silently truncates generated sqlite SQL).
- Wire contract is frozen: only `GET` (reads) and `POST` (writes); paths `/api/v1/import/{action}-{subject}`; envelope `{"success","message","data"}`; datetimes `"2006-01-02 15:04:05"` (`datetime.Layout`); `isImported` is int `0`/`1`; ids are UUIDv7 strings; `errors` object always present on handled errors.
- Every new error code: a `Code…` const in `internal/shared/errs/codes.go`, an entry in `AllCodes`, and `errors.import.<key>` in ALL 11 catalogues (`de,en,es,fr,it,nl,pl,pt,ru,uk,zh`). Every new UI key: the `imports.*` namespace in all 11 catalogues (`internal/test/i18ntest` enforces parity; `{var}` placeholder sets must match per key).
- Ingest scope: `middleware.IngestPathPrefix = "/api/v1/import/ingest-"` — the ingest endpoint path MUST start with it (`/api/v1/import/ingest-apple-wallet-event`).
- Logs: static messages, UUIDs only, NEVER the ingest body, payee, or card name (`reqctx.AddLogAttr` values are ids/statuses only).
- Rate limit: `ECONUMO_RATE_LIMIT_INGEST` (default `60`) per user per window, every request counts; 429 uses the frozen envelope.
- Ingest response is always HTTP 200 with `{"status": "created"|"queued"|"skipped"|"duplicate"|"failed", "eventId"}` once the payload is persisted; the only non-200s are 401 (token), 402 (read-only), 429 (rate limit), and the coded 400 `import.source_not_found` (Apple Wallet never connected — nothing to store against).
- Analytics: every new user action fires `trackEvent(METRICS.X)` at its success point; `metrics-coverage.test.ts` must pass without adding to `NOT_WIRED`.
- Comments: only *why*/non-obvious business rules; no references to PHP/Symfony; keep swag `// @…` blocks complete on every handler.
- `docs/regression-test-plan.md` MUST be updated in the same PR for every user-observable change.
- Run Go from the repo root with `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin`; frontend commands run in `web/` via `pnpm`. Never `cd` out of the worktree; use repo-root-relative paths.
- Branch: work on the current worktree branch; the final branch is pushed as `feature/transaction-import-apple-wallet` stacked on `feature/transaction-import`.

---

## File map

| Path | Responsibility |
|---|---|
| `internal/model/imports.go` (modify) | `ImportAccountLink` entity + mode consts |
| `internal/model/imports_dto.go` (create) | every `/api/v1/import/*` request/result DTO + `Validate()` |
| `internal/infra/storage/sqlc/query/{sqlite,pgsql}/imports.sql` (modify) | new queries (sources, account links, events, links) |
| `internal/imports/repository.go`, `internal/imports/repo/{repo,sqlite,pgsql}.go` (modify) | repository methods for the new queries |
| `internal/test/fixture/imports.go` (modify) | `ImportAccountLink`, `ImportEvent` seed builders |
| `internal/imports/applewallet.go` (create) | `ParseAppleWalletEvent` — payload → `model.IngestEvent` |
| `internal/imports/ports.go` (create) | `AccountReader`, `CurrencyConverter`, `TransactionCreator`, `TransactionLister`, `AttemptLimiter` |
| `internal/imports/service.go` (create) | `Service` struct, constructor, shared helpers (source lookup, card lookup, DTO mappers) |
| `internal/imports/ingest.go` (create) | `IngestAppleWallet`, `RetryEvent`, the stage 0–3 pipeline, `processEvent` |
| `internal/imports/source.go` (create) | `CreateSource`, `GetSourceList`, `DeleteSource` |
| `internal/imports/accountlink.go` (create) | `LinkAccount` (+ conversion run), `IgnoreAccount`, `UnlinkAccount` |
| `internal/imports/queue.go` (create) | `GetQueue`, `ImportQueuedEvent`, `SkipQueuedEvent`, `UnskipQueuedEvent`, `DiscardEvent`, `GetTransactionImportList` |
| `internal/imports/api/{handler,source,accountlink,ingest,queue,routes}.go` (create) | HTTP edge |
| `internal/server/glue_imports.go` (create) + `server.go` (modify) | port adapters + composition |
| `internal/config/config.go` (modify) | `RateLimitIngest` |
| `internal/web/router/router.go` (modify) | `IMPORT_MATCHER` in served `econumo-config.js` |
| `internal/shared/errs/codes.go`, `locales/*.json` (modify) | codes + catalogue |
| `internal/test/apiparity/{fixture.go,catalogue_import.go}` (+goldens) | scenario coverage |
| `web/src/api/dto/imports.ts`, `web/src/api/imports.ts`, `web/src/features/imports/{queries.ts,ImportsDataPage.tsx,AppleWalletSetup.tsx,ImportCards.tsx,ImportQueuePage.tsx,ImportQueueBanner.tsx}` (create) | SPA slice |
| `web/src/lib/{config,metrics,platform}.ts`, `web/src/api/user.ts`, `web/src/app/{routes.tsx,router-pages.ts,queryKeys.ts,uiStore.ts}`, `web/src/features/transactions/{TransactionDialog,TransactionRow,ViewTransactionDialog}.tsx`, `web/src/features/settings/SettingsPage.tsx`, `web/src/app/layouts/ApplicationLayout.tsx`, `web/public/econumo-config.js` (modify) | integration points |
| `web/shortcuts/README.md` (modify — exists since stage 1), `web/public/shortcuts/` (user-supplied `.shortcut` files, NOT part of this plan) | shortcut recipe |
| `CLAUDE.md`, `.env.example`, `docs/regression-test-plan.md` (modify) | docs |

---

### Task 1: Model, queries, repository, fixtures

**Files:**
- Modify: `internal/model/imports.go`
- Modify: `internal/infra/storage/sqlc/query/sqlite/imports.sql`, `internal/infra/storage/sqlc/query/pgsql/imports.sql`
- Modify: `internal/imports/repository.go`, `internal/imports/repo/repo.go`, `internal/imports/repo/sqlite.go`, `internal/imports/repo/pgsql.go`
- Modify: `internal/test/fixture/imports.go`
- Test: `internal/imports/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: stage-1 `imports.Repository`, `model.Import*`, `fixture.Builder` helpers (`orNewID`, `now`, `insert`, `nullable`).
- Produces (later tasks rely on these exact names):
  ```go
  // internal/model/imports.go
  const ImportAccountLinkModeImport = "import"
  const ImportAccountLinkModeIgnore = "ignore"
  type ImportAccountLink struct { ID, SourceID vo.Id; ExternalAccountID, ExternalName string; ExternalCurrency *string; AccountID *vo.Id; Mode string; CreatedAt, UpdatedAt time.Time }
  // internal/imports/repository.go additions
  GetSourceByUserProvider(ctx, userID vo.Id, provider string) (*model.ImportSource, error)  // NotFound when absent
  ListSourcesByUser(ctx, userID vo.Id) ([]model.ImportSource, error)                          // ORDER BY created_at, id
  DeleteSource(ctx, id vo.Id) error
  InsertAccountLink(ctx, l *model.ImportAccountLink) error
  UpdateAccountLink(ctx, l *model.ImportAccountLink) error                                    // external_currency, account_id, mode, updated_at
  DeleteAccountLink(ctx, id vo.Id) error
  GetAccountLink(ctx, id vo.Id) (*model.ImportAccountLink, error)
  ListAccountLinksBySource(ctx, sourceID vo.Id) ([]model.ImportAccountLink, error)            // ORDER BY created_at, id
  DeleteEvent(ctx, id vo.Id) error
  ListEventsBySourceStatus(ctx, sourceID vo.Id, status string) ([]model.ImportEvent, error)  // ORDER BY received_at DESC, id
  GetLink(ctx, id vo.Id) (*model.ImportTransactionLink, error)
  UpdateLink(ctx, l *model.ImportTransactionLink) error                                       // transaction_id, status, applied_*, run_id, external_amount, external_currency
  ListLinksBySource(ctx, sourceID vo.Id) ([]model.ImportTransactionLink, error)               // ORDER BY external_posted_at DESC, id
  DeleteQueuedLinksByExternalAccount(ctx, sourceID vo.Id, externalAccountID string) error    // status = 'queued' only
  // internal/test/fixture/imports.go
  type ImportAccountLink struct { ID, SourceID, ExternalAccountID, ExternalName, ExternalCurrency, AccountID, Mode string }  // Mode default "import"; ExternalCurrency/AccountID "" -> NULL
  type ImportEvent struct { ID, SourceID, Payload, PayloadHash, Status, ParseError string; ReceivedAt time.Time }             // Status default "processed"; PayloadHash "" -> sha256 of Payload; ParseError "" -> NULL
  ```

- [ ] **Step 1: Add the entity + mode constants to `internal/model/imports.go`**

Append after the `ImportRunStatus…` consts and after `ImportSource`:

```go
	ImportAccountLinkModeImport = "import"
	ImportAccountLinkModeIgnore = "ignore"
```

```go
// ImportAccountLink is the user's decision for one external account (a
// card name for Apple Wallet): import into AccountID, or ignore. AccountID
// is nil only in ignore mode (schema CHECK). ExternalCurrency is the
// currency the provider reported for the card, recorded when the link is
// created so a currency drift can be refused later.
type ImportAccountLink struct {
	ID                vo.Id
	SourceID          vo.Id
	ExternalAccountID string
	ExternalName      string
	ExternalCurrency  *string
	AccountID         *vo.Id
	Mode              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
```

- [ ] **Step 2: Add the queries — sqlite** (append to `internal/infra/storage/sqlc/query/sqlite/imports.sql`; statements end inline)

```sql
-- name: GetImportSourceByUserProvider :one
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = ? AND provider = ?
ORDER BY created_at, id
LIMIT 1;

-- name: ListImportSourcesByUser :many
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = ?
ORDER BY created_at, id;

-- name: DeleteImportSource :exec
DELETE FROM import_sources WHERE id = ?;

-- name: InsertImportAccountLink :exec
INSERT INTO import_account_links (id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateImportAccountLink :exec
UPDATE import_account_links SET external_currency = ?, account_id = ?, mode = ?, updated_at = ? WHERE id = ?;

-- name: DeleteImportAccountLink :exec
DELETE FROM import_account_links WHERE id = ?;

-- name: GetImportAccountLinkByID :one
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE id = ?;

-- name: ListImportAccountLinksBySource :many
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE source_id = ?
ORDER BY created_at, id;

-- name: DeleteImportEvent :exec
DELETE FROM import_events WHERE id = ?;

-- name: ListImportEventsBySourceStatus :many
SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at
FROM import_events
WHERE source_id = ? AND status = ?
ORDER BY received_at DESC, id;

-- name: GetImportTransactionLinkByID :one
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE id = ?;

-- name: UpdateImportTransactionLink :exec
UPDATE import_transaction_links
SET run_id = ?, transaction_id = ?, status = ?, external_amount = ?, external_currency = ?, applied_category_id = ?, applied_payee_id = ?, applied_tag_id = ?, applied_rule_id = ?
WHERE id = ?;

-- name: ListImportTransactionLinksBySource :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = ?
ORDER BY external_posted_at DESC, id;

-- name: DeleteQueuedImportTransactionLinksByExternalAccount :exec
DELETE FROM import_transaction_links WHERE source_id = ? AND external_account_id = ? AND status = 'queued';
```

- [ ] **Step 3: Add the same queries — pgsql** (append to `internal/infra/storage/sqlc/query/pgsql/imports.sql`; `$N` placeholders, `;` on its own line — copy the file's existing style exactly)

```sql
-- name: GetImportSourceByUserProvider :one
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = $1 AND provider = $2
ORDER BY created_at, id
LIMIT 1
;

-- name: ListImportSourcesByUser :many
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = $1
ORDER BY created_at, id
;

-- name: DeleteImportSource :exec
DELETE FROM import_sources WHERE id = $1
;

-- name: InsertImportAccountLink :exec
INSERT INTO import_account_links (id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
;

-- name: UpdateImportAccountLink :exec
UPDATE import_account_links SET external_currency = $1, account_id = $2, mode = $3, updated_at = $4 WHERE id = $5
;

-- name: DeleteImportAccountLink :exec
DELETE FROM import_account_links WHERE id = $1
;

-- name: GetImportAccountLinkByID :one
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE id = $1
;

-- name: ListImportAccountLinksBySource :many
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE source_id = $1
ORDER BY created_at, id
;

-- name: DeleteImportEvent :exec
DELETE FROM import_events WHERE id = $1
;

-- name: ListImportEventsBySourceStatus :many
SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at
FROM import_events
WHERE source_id = $1 AND status = $2
ORDER BY received_at DESC, id
;

-- name: GetImportTransactionLinkByID :one
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE id = $1
;

-- name: UpdateImportTransactionLink :exec
UPDATE import_transaction_links
SET run_id = $1, transaction_id = $2, status = $3, external_amount = $4, external_currency = $5, applied_category_id = $6, applied_payee_id = $7, applied_tag_id = $8, applied_rule_id = $9
WHERE id = $10
;

-- name: ListImportTransactionLinksBySource :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = $1
ORDER BY external_posted_at DESC, id
;

-- name: DeleteQueuedImportTransactionLinksByExternalAccount :exec
DELETE FROM import_transaction_links WHERE source_id = $1 AND external_account_id = $2 AND status = 'queued'
;
```

- [ ] **Step 4: Regenerate sqlc and confirm both packages compile**

Run: `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin && go generate ./internal/infra/storage/sqlc/ && go build ./internal/infra/storage/sqlc/...`
Expected: no output from build; `gen/sqlite/imports.sql.go` and `gen/pgsql/imports.sql.go` now contain the 13 new methods. If the pgsql `UpdateImportTransactionLink` param types differ from sqlite (e.g. `external_amount` NUMERIC → `string` on both with the stdlib driver; `*string` for nullable columns), note the exact generated types — the adapters below convert whole structs, so the pgsql shim only compiles when field sets line up. If a pgsql param struct has a different field TYPE (not just name), convert field-by-field in `pgsql.go` instead of the whole-struct cast.

- [ ] **Step 5: Write failing repository tests** (append to `internal/imports/repo/repo_integration_test.go`)

```go
func TestRepo_SourceByUserProviderListDelete(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	if _, err := repo.GetSourceByUserProvider(ctx, vo.MustParseId(userA), model.ImportProviderAppleWallet); err == nil {
		t.Fatal("GetSourceByUserProvider(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("miss = %T, want NotFound", err)
	}
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetSourceByUserProvider(ctx, src.UserID, model.ImportProviderAppleWallet)
	if err != nil || got.ID != src.ID {
		t.Fatalf("GetSourceByUserProvider = %+v, %v", got, err)
	}
	list, err := repo.ListSourcesByUser(ctx, src.UserID)
	if err != nil || len(list) != 1 || list[0].ID != src.ID {
		t.Fatalf("ListSourcesByUser = %+v, %v", list, err)
	}
	if err := repo.DeleteSource(ctx, src.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := repo.ListSourcesByUser(ctx, src.UserID); len(list) != 0 {
		t.Fatalf("after delete: %+v", list)
	}
}

func TestRepo_AccountLinkRoundTrip(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	acct := vo.MustParseId(acct1)
	l := &model.ImportAccountLink{
		ID: vo.NewId(), SourceID: src.ID, ExternalAccountID: "Apple Card", ExternalName: "Apple Card",
		Mode: model.ImportAccountLinkModeIgnore, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.InsertAccountLink(ctx, l); err != nil {
		t.Fatalf("InsertAccountLink: %v", err)
	}
	got, err := repo.GetAccountLink(ctx, l.ID)
	if err != nil || got.Mode != model.ImportAccountLinkModeIgnore || got.AccountID != nil || got.ExternalCurrency != nil {
		t.Fatalf("GetAccountLink = %+v, %v", got, err)
	}
	usd := "USD"
	l.Mode, l.AccountID, l.ExternalCurrency = model.ImportAccountLinkModeImport, &acct, &usd
	if err := repo.UpdateAccountLink(ctx, l); err != nil {
		t.Fatalf("UpdateAccountLink: %v", err)
	}
	list, err := repo.ListAccountLinksBySource(ctx, src.ID)
	if err != nil || len(list) != 1 || list[0].AccountID == nil || *list[0].AccountID != acct || *list[0].ExternalCurrency != "USD" {
		t.Fatalf("ListAccountLinksBySource = %+v, %v", list, err)
	}
	// (source_id, external_account_id) is unique
	dup := *l
	dup.ID = vo.NewId()
	if err := repo.InsertAccountLink(ctx, &dup); err == nil {
		t.Fatal("duplicate external account must fail")
	}
	if err := repo.DeleteAccountLink(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetAccountLink(ctx, l.ID); err == nil {
		t.Fatal("GetAccountLink after delete must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("= %T, want NotFound", err)
	}
}

func TestRepo_EventListDelete(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	perr := "bad amount"
	failed := &model.ImportEvent{ID: vo.NewId(), SourceID: src.ID, Payload: `{"a":1}`, PayloadHash: "h1", Status: model.ImportEventStatusFailed, ParseError: &perr, ReceivedAt: fixedTime}
	ok := &model.ImportEvent{ID: vo.NewId(), SourceID: src.ID, Payload: `{"a":2}`, PayloadHash: "h2", Status: model.ImportEventStatusProcessed, ReceivedAt: fixedTime}
	for _, e := range []*model.ImportEvent{failed, ok} {
		if _, err := repo.InsertEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repo.ListEventsBySourceStatus(ctx, src.ID, model.ImportEventStatusFailed)
	if err != nil || len(list) != 1 || list[0].ID != failed.ID || *list[0].ParseError != "bad amount" {
		t.Fatalf("ListEventsBySourceStatus = %+v, %v", list, err)
	}
	if err := repo.DeleteEvent(ctx, failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetEvent(ctx, failed.ID); err == nil {
		t.Fatal("GetEvent after delete must error")
	}
}

func TestRepo_LinkGetUpdateListPurge(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000001")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	mk := func(ext string, status string, posted time.Time) *model.ImportTransactionLink {
		return &model.ImportTransactionLink{
			ID: vo.NewId(), SourceID: src.ID, ExternalAccountID: "Apple Card", ExternalTransactionID: ext, Status: status,
			ExternalPayee: "Shop", ExternalAmount: "12.50000000", ExternalPostedAt: posted, ImportedAt: fixedTime,
		}
	}
	q1 := mk("t1", model.ImportLinkStatusQueued, fixedTime)
	q2 := mk("t2", model.ImportLinkStatusQueued, fixedTime.Add(time.Hour))
	seen := mk("t3", model.ImportLinkStatusSkipped, fixedTime.Add(2*time.Hour))
	for _, l := range []*model.ImportTransactionLink{q1, q2, seen} {
		if err := repo.InsertLink(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.GetLink(ctx, q1.ID)
	if err != nil || got.ExternalTransactionID != "t1" {
		t.Fatalf("GetLink = %+v, %v", got, err)
	}
	list, err := repo.ListLinksBySource(ctx, src.ID)
	if err != nil || len(list) != 3 || list[0].ID != seen.ID || list[2].ID != q1.ID {
		t.Fatalf("ListLinksBySource order = %+v, %v", list, err)
	}
	eur := "EUR"
	q1.Status, q1.ExternalAmount, q1.ExternalCurrency = model.ImportLinkStatusSkipped, "11.00000000", &eur
	if err := repo.UpdateLink(ctx, q1); err != nil {
		t.Fatalf("UpdateLink: %v", err)
	}
	if got, _ := repo.GetLink(ctx, q1.ID); got.Status != model.ImportLinkStatusSkipped || got.ExternalAmount != vo.NewDecimal("11.00000000").String() || *got.ExternalCurrency != "EUR" {
		t.Fatalf("after UpdateLink = %+v", got)
	}
	if err := repo.DeleteQueuedLinksByExternalAccount(ctx, src.ID, "Apple Card"); err != nil {
		t.Fatal(err)
	}
	list, _ = repo.ListLinksBySource(ctx, src.ID)
	if len(list) != 2 { // q2 (queued) purged; q1 (now skipped) and seen survive
		t.Fatalf("after purge = %+v", list)
	}
	if _, err := repo.GetLink(ctx, vo.NewId()); err == nil {
		t.Fatal("GetLink(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("= %T, want NotFound", err)
	}
}
```

- [ ] **Step 6: Run the tests to see them fail to compile**

Run: `go test ./internal/imports/repo/ 2>&1 | head -20`
Expected: build errors — `repo.GetSourceByUserProvider undefined`, etc.

- [ ] **Step 7: Extend the interface** (`internal/imports/repository.go`)

```go
type Repository interface {
	InsertSource(ctx context.Context, s *model.ImportSource) error
	GetSource(ctx context.Context, id vo.Id) (*model.ImportSource, error)
	GetSourceByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.ImportSource, error)
	ListSourcesByUser(ctx context.Context, userID vo.Id) ([]model.ImportSource, error)
	DeleteSource(ctx context.Context, id vo.Id) error

	InsertAccountLink(ctx context.Context, l *model.ImportAccountLink) error
	UpdateAccountLink(ctx context.Context, l *model.ImportAccountLink) error
	DeleteAccountLink(ctx context.Context, id vo.Id) error
	GetAccountLink(ctx context.Context, id vo.Id) (*model.ImportAccountLink, error)
	ListAccountLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportAccountLink, error)

	// InsertEvent is the inbox dedupe point: a payload already stored for the
	// source (same hash) is dropped and inserted=false is returned.
	InsertEvent(ctx context.Context, e *model.ImportEvent) (inserted bool, err error)
	GetEvent(ctx context.Context, id vo.Id) (*model.ImportEvent, error)
	UpdateEventStatus(ctx context.Context, id vo.Id, status string, parseError *string) error
	DeleteEvent(ctx context.Context, id vo.Id) error
	ListEventsBySourceStatus(ctx context.Context, sourceID vo.Id, status string) ([]model.ImportEvent, error)

	InsertRun(ctx context.Context, r *model.ImportRun) error
	GetRun(ctx context.Context, id vo.Id) (*model.ImportRun, error)
	UpdateRun(ctx context.Context, r *model.ImportRun) error

	InsertLink(ctx context.Context, l *model.ImportTransactionLink) error
	GetLink(ctx context.Context, id vo.Id) (*model.ImportTransactionLink, error)
	UpdateLink(ctx context.Context, l *model.ImportTransactionLink) error
	GetLinkByExternalKey(ctx context.Context, sourceID vo.Id, externalAccountID, externalTransactionID string) (*model.ImportTransactionLink, error)
	ListLinksByTransaction(ctx context.Context, transactionID vo.Id) ([]model.ImportTransactionLink, error)
	ListLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportTransactionLink, error)
	// DeleteQueuedLinksByExternalAccount purges only queued rows: seen rows
	// (linked/skipped/tombstone) are the dedupe memory and must survive.
	DeleteQueuedLinksByExternalAccount(ctx context.Context, sourceID vo.Id, externalAccountID string) error
}
```

- [ ] **Step 8: Implement the repo methods** (`internal/imports/repo/repo.go`)

Add type aliases next to the existing ones:

```go
	accountLinkRow            = sqlitegen.ImportAccountLink
	insertAccountLinkParams   = sqlitegen.InsertImportAccountLinkParams
	updateAccountLinkParams   = sqlitegen.UpdateImportAccountLinkParams
	sourceByUserProviderPs    = sqlitegen.GetImportSourceByUserProviderParams
	eventsBySourceStatusPs    = sqlitegen.ListImportEventsBySourceStatusParams
	updateLinkParams          = sqlitegen.UpdateImportTransactionLinkParams
	purgeQueuedLinksParams    = sqlitegen.DeleteQueuedImportTransactionLinksByExternalAccountParams
```

Extend `querier`:

```go
	GetImportSourceByUserProvider(ctx context.Context, db backend.DBTX, p sourceByUserProviderPs) (sourceRow, error)
	ListImportSourcesByUser(ctx context.Context, db backend.DBTX, userID string) ([]sourceRow, error)
	DeleteImportSource(ctx context.Context, db backend.DBTX, id string) error
	InsertImportAccountLink(ctx context.Context, db backend.DBTX, p insertAccountLinkParams) error
	UpdateImportAccountLink(ctx context.Context, db backend.DBTX, p updateAccountLinkParams) error
	DeleteImportAccountLink(ctx context.Context, db backend.DBTX, id string) error
	GetImportAccountLinkByID(ctx context.Context, db backend.DBTX, id string) (accountLinkRow, error)
	ListImportAccountLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]accountLinkRow, error)
	DeleteImportEvent(ctx context.Context, db backend.DBTX, id string) error
	ListImportEventsBySourceStatus(ctx context.Context, db backend.DBTX, p eventsBySourceStatusPs) ([]eventRow, error)
	GetImportTransactionLinkByID(ctx context.Context, db backend.DBTX, id string) (linkRow, error)
	UpdateImportTransactionLink(ctx context.Context, db backend.DBTX, p updateLinkParams) error
	ListImportTransactionLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]linkRow, error)
	DeleteQueuedImportTransactionLinksByExternalAccount(ctx context.Context, db backend.DBTX, p purgeQueuedLinksParams) error
```

Methods (follow the file's existing style — `sql.ErrNoRows` → `errs.NewNotFound(...)`, `optionalID`/`parseOptionalID`, `*FromRow` helpers):

```go
func (r *Repo) GetSourceByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.ImportSource, error) {
	row, err := r.q.GetImportSourceByUserProvider(ctx, r.db(ctx), sourceByUserProviderPs{UserID: userID.String(), Provider: provider})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import source not found")
		}
		return nil, err
	}
	return sourceFromRow(row)
}

func (r *Repo) ListSourcesByUser(ctx context.Context, userID vo.Id) ([]model.ImportSource, error) {
	rows, err := r.q.ListImportSourcesByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportSource, 0, len(rows))
	for _, row := range rows {
		s, err := sourceFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, nil
}

func (r *Repo) DeleteSource(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportSource(ctx, r.db(ctx), id.String())
}

func (r *Repo) InsertAccountLink(ctx context.Context, l *model.ImportAccountLink) error {
	return r.q.InsertImportAccountLink(ctx, r.db(ctx), insertAccountLinkParams{
		ID: l.ID.String(), SourceID: l.SourceID.String(), ExternalAccountID: l.ExternalAccountID, ExternalName: l.ExternalName,
		ExternalCurrency: l.ExternalCurrency, AccountID: optionalID(l.AccountID), Mode: l.Mode, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	})
}

func (r *Repo) UpdateAccountLink(ctx context.Context, l *model.ImportAccountLink) error {
	return r.q.UpdateImportAccountLink(ctx, r.db(ctx), updateAccountLinkParams{
		ExternalCurrency: l.ExternalCurrency, AccountID: optionalID(l.AccountID), Mode: l.Mode, UpdatedAt: l.UpdatedAt, ID: l.ID.String(),
	})
}

func (r *Repo) DeleteAccountLink(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportAccountLink(ctx, r.db(ctx), id.String())
}

func (r *Repo) GetAccountLink(ctx context.Context, id vo.Id) (*model.ImportAccountLink, error) {
	row, err := r.q.GetImportAccountLinkByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import account link not found")
		}
		return nil, err
	}
	return accountLinkFromRow(row)
}

func (r *Repo) ListAccountLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportAccountLink, error) {
	rows, err := r.q.ListImportAccountLinksBySource(ctx, r.db(ctx), sourceID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportAccountLink, 0, len(rows))
	for _, row := range rows {
		l, err := accountLinkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func accountLinkFromRow(row accountLinkRow) (*model.ImportAccountLink, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	accountID, err := parseOptionalID(row.AccountID)
	if err != nil {
		return nil, err
	}
	return &model.ImportAccountLink{
		ID: id, SourceID: sourceID, ExternalAccountID: row.ExternalAccountID, ExternalName: row.ExternalName,
		ExternalCurrency: row.ExternalCurrency, AccountID: accountID, Mode: row.Mode, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *Repo) DeleteEvent(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportEvent(ctx, r.db(ctx), id.String())
}

func (r *Repo) ListEventsBySourceStatus(ctx context.Context, sourceID vo.Id, status string) ([]model.ImportEvent, error) {
	rows, err := r.q.ListImportEventsBySourceStatus(ctx, r.db(ctx), eventsBySourceStatusPs{SourceID: sourceID.String(), Status: status})
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportEvent, 0, len(rows))
	for _, row := range rows {
		e, err := eventFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, nil
}

func (r *Repo) GetLink(ctx context.Context, id vo.Id) (*model.ImportTransactionLink, error) {
	row, err := r.q.GetImportTransactionLinkByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import link not found")
		}
		return nil, err
	}
	return linkFromRow(row)
}

func (r *Repo) UpdateLink(ctx context.Context, l *model.ImportTransactionLink) error {
	return r.q.UpdateImportTransactionLink(ctx, r.db(ctx), updateLinkParams{
		RunID: optionalID(l.RunID), TransactionID: optionalID(l.TransactionID), Status: l.Status,
		ExternalAmount: l.ExternalAmount, ExternalCurrency: l.ExternalCurrency,
		AppliedCategoryID: optionalID(l.AppliedCategoryID), AppliedPayeeID: optionalID(l.AppliedPayeeID),
		AppliedTagID: optionalID(l.AppliedTagID), AppliedRuleID: optionalID(l.AppliedRuleID), ID: l.ID.String(),
	})
}

func (r *Repo) ListLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportTransactionLink, error) {
	rows, err := r.q.ListImportTransactionLinksBySource(ctx, r.db(ctx), sourceID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportTransactionLink, 0, len(rows))
	for _, row := range rows {
		l, err := linkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func (r *Repo) DeleteQueuedLinksByExternalAccount(ctx context.Context, sourceID vo.Id, externalAccountID string) error {
	return r.q.DeleteQueuedImportTransactionLinksByExternalAccount(ctx, r.db(ctx), purgeQueuedLinksParams{SourceID: sourceID.String(), ExternalAccountID: externalAccountID})
}
```

`sqlite.go`: add one passthrough per new querier method (`return sqlitegen.New(db).X(ctx, p)` — copy the existing shape). `pgsql.go`: add one shim per method, converting params with `pgsqlgen.XParams(p)` and rows with `sourceRow(s)` / `accountLinkRow(a)` / `eventRow(e)` / `linkRow(l)` element-wise for slices (copy the existing `ListImportTransactionLinksByTransaction` loop).

- [ ] **Step 9: Run the repo tests on sqlite**

Run: `go test ./internal/imports/repo/ -run 'TestRepo_' -v 2>&1 | tail -20`
Expected: all `TestRepo_*` PASS.

- [ ] **Step 10: Add the fixture builders** (append to `internal/test/fixture/imports.go`)

```go
// ImportAccountLink seeds one import_account_links row. Mode defaults to
// "import"; ExternalCurrency/AccountID "" -> NULL (AccountID must be set
// unless Mode is "ignore" — schema CHECK).
type ImportAccountLink struct {
	ID                string
	SourceID          string
	ExternalAccountID string
	ExternalName      string
	ExternalCurrency  string
	AccountID         string
	Mode              string
}

func (b *Builder) ImportAccountLink(l ImportAccountLink) string {
	b.t.Helper()
	id := b.orNewID(l.ID)
	mode := l.Mode
	if mode == "" {
		mode = "import"
	}
	name := l.ExternalName
	if name == "" {
		name = l.ExternalAccountID
	}
	now := b.now()
	b.insert(`INSERT INTO import_account_links (id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, l.SourceID, l.ExternalAccountID, name, nullable(l.ExternalCurrency), nullable(l.AccountID), mode, now, now)
	return id
}

// ImportEvent seeds one inbox row. Status defaults to "processed";
// PayloadHash "" -> sha256(Payload); ParseError "" -> NULL; ReceivedAt zero -> now.
type ImportEvent struct {
	ID          string
	SourceID    string
	Payload     string
	PayloadHash string
	Status      string
	ParseError  string
	ReceivedAt  time.Time
}

func (b *Builder) ImportEvent(e ImportEvent) string {
	b.t.Helper()
	id := b.orNewID(e.ID)
	status := e.Status
	if status == "" {
		status = "processed"
	}
	hash := e.PayloadHash
	if hash == "" {
		sum := sha256.Sum256([]byte(e.Payload))
		hash = hex.EncodeToString(sum[:])
	}
	received := e.ReceivedAt
	if received.IsZero() {
		received = b.now()
	}
	b.insert(`INSERT INTO import_events (id, source_id, run_id, payload, payload_hash, status, parse_error, received_at)
		VALUES (?, ?, NULL, ?, ?, ?, ?, ?)`,
		id, e.SourceID, e.Payload, hash, status, nullable(e.ParseError), received)
	return id
}
```

Add `"crypto/sha256"` and `"encoding/hex"` to the file's imports. Confirm `nullable` exists in the fixture package (it is used by `ImportTransactionLink`); it maps `""` → `nil`.

- [ ] **Step 11: Full smoke + pgsql repo run**

Run: `make go-test 2>&1 | tail -15`
Expected: PASS (coverage gate holds; no golden changes yet).

Run (PostgreSQL; start a throwaway server if none: `docker run --rm -d --name econumo-pg-test -e POSTGRES_USER=econumo -e POSTGRES_PASSWORD=econumo -e POSTGRES_DB=econumo_test -p 15432:5432 postgres:17-alpine`):
`DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@localhost:15432/econumo_test?sslmode=disable' make test-repo-pgsql 2>&1 | tail -15`
Expected: PASS — the pgsql adapters for all 13 new queries are exercised.

- [ ] **Step 12: Commit**

```bash
git add internal/model/imports.go internal/infra/storage/sqlc internal/imports/repository.go internal/imports/repo internal/test/fixture/imports.go
git commit -m "feat(imports): account links, source/event/link queries, fixtures"
```

---

### Task 2: Apple Wallet payload normalizer

**Files:**
- Create: `internal/imports/applewallet.go`
- Test: `internal/imports/applewallet_test.go`

**Interfaces:**
- Consumes: `model.IngestEvent`, `model.TransactionTypeExpense/Income`, `vo.NewDecimal`.
- Produces:
  ```go
  // ParseAppleWalletEvent normalizes one shortcut payload. receivedAt is the
  // fallback for a missing/unparsable occurredAt. PostedAt keeps the payload's
  // UTC offset (the pipeline converts to wall-clock); ExternalAccountID is the
  // whitespace-collapsed card name in its original case; ExternalTransactionID
  // is eventId or the synthesized sha256.
  func ParseAppleWalletEvent(payload []byte, receivedAt time.Time) (model.IngestEvent, error)
  func normalizeAmount(raw string) (string, error)          // package-private, tested directly
  func normalizeExternalAccountID(s string) string          // strings.Join(strings.Fields(s), " ")
  ```
  Parse errors are plain `error`s whose `Error()` text is stored as the event's `parse_error` and shown in the failed list: `"invalid JSON"`, `"account is required"`, `"amount must be a positive number"`, `"currency must be a 3-letter code"`, `"type must be expense or income"`.

- [ ] **Step 1: Write the failing tests** (`internal/imports/applewallet_test.go`, `package imports`)

```go
package imports

import (
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

var received = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

func TestNormalizeAmount(t *testing.T) {
	cases := []struct{ in, want string }{
		{"12.50", "12.50"}, {"$12.50", "12.50"}, {"12,50 €", "12.50"}, {"1.234,56", "1234.56"},
		{"1,234.56", "1234.56"}, {"-12.50", "12.50"}, {"12", "12"}, {"1,234", "1.234"}, {" 7 ", "7"}, {".5", "0.5"},
	}
	for _, c := range cases {
		got, err := normalizeAmount(c.in)
		if err != nil {
			t.Errorf("normalizeAmount(%q): %v", c.in, err)
			continue
		}
		if !vo.NewDecimal(got).Equals(vo.NewDecimal(c.want)) {
			t.Errorf("normalizeAmount(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"", "abc", "0", "0.00", "$", "1.2.3."} {
		if _, err := normalizeAmount(bad); err == nil {
			t.Errorf("normalizeAmount(%q) must fail", bad)
		}
	}
}

func TestParseAppleWalletEvent_Full(t *testing.T) {
	body := []byte(`{"account":"  Apple   Card ","payee":" Blue Bottle ","amount":"$4.75","currency":"usd","occurredAt":"2026-08-15T10:42:03-07:00","type":"Expense","eventId":"evt-1"}`)
	ev, err := ParseAppleWalletEvent(body, received)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalAccountID != "Apple Card" || ev.Payee != "Blue Bottle" || ev.Currency != "USD" || ev.ExternalTransactionID != "evt-1" || ev.Type != model.TransactionTypeExpense || ev.Description != "" {
		t.Errorf("parsed = %+v", ev)
	}
	if !vo.NewDecimal(ev.Amount).Equals(vo.NewDecimal("4.75")) {
		t.Errorf("amount = %q", ev.Amount)
	}
	want := time.Date(2026, 8, 15, 10, 42, 3, 0, time.FixedZone("", -7*3600))
	if !ev.PostedAt.Equal(want) || ev.PostedAt.Format("15:04") != "10:42" {
		t.Errorf("postedAt = %v (offset must be preserved)", ev.PostedAt)
	}
}

func TestParseAppleWalletEvent_DefaultsAndSynthesizedID(t *testing.T) {
	body := []byte(`{"account":"Apple Card","payee":"Shop","amount":12.5,"currency":"EUR"}`)
	ev, err := ParseAppleWalletEvent(body, received)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != model.TransactionTypeExpense || !ev.PostedAt.Equal(received) {
		t.Errorf("defaults: %+v", ev)
	}
	if len(ev.ExternalTransactionID) != 64 {
		t.Errorf("synthesized id = %q, want sha256 hex", ev.ExternalTransactionID)
	}
	// the synthesized id is a function of the PARSED values, so formatting noise does not fork it
	again, _ := ParseAppleWalletEvent([]byte(`{"account":" Apple  Card","payee":" Shop ","amount":"12,50","currency":"eur"}`), received)
	if again.ExternalTransactionID != ev.ExternalTransactionID {
		t.Errorf("synthesized id must be stable across formatting: %q vs %q", again.ExternalTransactionID, ev.ExternalTransactionID)
	}
	// income flips the type; an unparsable occurredAt falls back to receivedAt
	inc, err := ParseAppleWalletEvent([]byte(`{"account":"Apple Card","amount":"1","currency":"USD","type":"income","occurredAt":"yesterday"}`), received)
	if err != nil || inc.Type != model.TransactionTypeIncome || !inc.PostedAt.Equal(received) {
		t.Errorf("income/fallback: %+v, %v", inc, err)
	}
}

func TestParseAppleWalletEvent_Errors(t *testing.T) {
	cases := map[string]string{
		`not json`:                                                         "invalid JSON",
		`{"amount":"1","currency":"USD"}`:                                  "account is required",
		`{"account":"Apple Card","amount":"zero","currency":"USD"}`:        "amount must be a positive number",
		`{"account":"Apple Card","amount":"1","currency":"$"}`:             "currency must be a 3-letter code",
		`{"account":"Apple Card","amount":"1"}`:                            "currency must be a 3-letter code",
		`{"account":"Apple Card","amount":"1","currency":"USD","type":"x"}`: "type must be expense or income",
	}
	for body, want := range cases {
		_, err := ParseAppleWalletEvent([]byte(body), received)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", body, err, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/imports/ -run 'TestNormalizeAmount|TestParseAppleWallet' 2>&1 | head`
Expected: build error `undefined: normalizeAmount` / `ParseAppleWalletEvent`.

- [ ] **Step 3: Implement** (`internal/imports/applewallet.go`)

```go
package imports

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// appleWalletPayload is what the "Econumo Wallet" shortcut POSTs. amount is
// untyped because Shortcuts serializes numbers as numbers and text as text
// depending on how the automation was built.
type appleWalletPayload struct {
	Account    string `json:"account"`
	Payee      string `json:"payee"`
	Amount     any    `json:"amount"`
	Currency   string `json:"currency"`
	OccurredAt string `json:"occurredAt"`
	Type       string `json:"type"`
	EventID    string `json:"eventId"`
}

var (
	currencyCodeRe = regexp.MustCompile(`^[A-Z]{3}$`)
	plainDecimalRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
)

func ParseAppleWalletEvent(payload []byte, receivedAt time.Time) (model.IngestEvent, error) {
	var p appleWalletPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return model.IngestEvent{}, errors.New("invalid JSON")
	}
	account := normalizeExternalAccountID(p.Account)
	if account == "" {
		return model.IngestEvent{}, errors.New("account is required")
	}
	amount, err := normalizeAmount(amountText(p.Amount))
	if err != nil {
		return model.IngestEvent{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(p.Currency))
	if !currencyCodeRe.MatchString(currency) {
		return model.IngestEvent{}, errors.New("currency must be a 3-letter code")
	}
	var typ model.TransactionType
	switch strings.ToLower(strings.TrimSpace(p.Type)) {
	case "", "expense":
		typ = model.TransactionTypeExpense
	case "income":
		typ = model.TransactionTypeIncome
	default:
		return model.IngestEvent{}, errors.New("type must be expense or income")
	}
	// The shortcut sends RFC3339 with the device offset; anything else (or
	// nothing) is replaced by the server's receipt time — a tap is delivered
	// within seconds, so that is close enough to be useful.
	postedAt := receivedAt
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(p.OccurredAt)); err == nil {
		postedAt = t
	}
	payee := strings.TrimSpace(p.Payee)
	externalID := strings.TrimSpace(p.EventID)
	if externalID == "" {
		// Built from the PARSED values so formatting noise between two
		// deliveries of the same tap cannot fork the id.
		sum := sha256.Sum256([]byte(account + "|" + postedAt.UTC().Format(time.RFC3339) + "|" + amount + "|" + payee))
		externalID = hex.EncodeToString(sum[:])
	}
	return model.IngestEvent{
		ExternalAccountID: account, ExternalTransactionID: externalID, Type: typ,
		Amount: amount, Currency: currency, PostedAt: postedAt, Payee: payee,
	}, nil
}

func amountText(v any) string {
	switch a := v.(type) {
	case string:
		return a
	case float64:
		return strconv.FormatFloat(a, 'f', -1, 64)
	default:
		return ""
	}
}

// normalizeAmount accepts what a phone produces — currency symbols, spaces,
// either separator, a sign — and returns the absolute value as plain decimal
// text. The LAST separator is the decimal point ("1.234,56" -> 1234.56).
func normalizeAmount(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if last := strings.LastIndexAny(s, ".,"); last >= 0 {
		intPart := strings.NewReplacer(".", "", ",", "").Replace(s[:last])
		frac := s[last+1:]
		if frac == "" {
			s = intPart
		} else {
			s = intPart + "." + frac
		}
	}
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	}
	if !plainDecimalRe.MatchString(s) {
		return "", errors.New("amount must be a positive number")
	}
	d := vo.NewDecimal(s)
	if d.IsZero() {
		return "", errors.New("amount must be a positive number")
	}
	return d.String(), nil
}

func normalizeExternalAccountID(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/imports/ -run 'TestNormalizeAmount|TestParseAppleWallet' -v 2>&1 | tail -12`
Expected: PASS. If `"1.2.3."` passes `plainDecimalRe` (it does not: after the last-separator rule it becomes `"123."` → int part `123`, frac `""` → `"123"`, which is valid!) — adjust the test's bad list to `{"", "abc", "0", "0.00", "$"}` and add `{"1.2.3.", "123"}` to the good cases: the tolerant rule is the contract, not strictness.

- [ ] **Step 5: gofmt + vet + commit**

Run: `gofmt -l internal/imports && go vet ./internal/imports/`
Expected: no output.

```bash
git add internal/imports/applewallet.go internal/imports/applewallet_test.go
git commit -m "feat(imports): Apple Wallet payload normalizer"
```

---

### Task 3: Ports, DTOs, and the ingest pipeline service

**Files:**
- Create: `internal/imports/ports.go`, `internal/imports/service.go`, `internal/imports/ingest.go`, `internal/model/imports_dto.go`
- Modify: `internal/shared/errs/codes.go` (codes only; catalogues in Task 6)
- Test: `internal/imports/ingest_test.go` (package `imports_test`, real sqlite repo + in-memory port fakes)

**Interfaces:**
- Consumes: Task 1 repository, Task 2 parser, `imports.Match`, `imports.MatcherConfig`, `imports.HashPayload`.
- Produces:
  ```go
  // ports.go
  type AccountReader interface {
      AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)        // *errs.NotFoundError when missing
      AccountDeleted(ctx context.Context, accountID vo.Id) (bool, error)
      AccountCurrencyCode(ctx context.Context, accountID vo.Id) (string, error) // ISO code, e.g. "USD"
  }
  type CurrencyConverter interface {
      Convert(ctx context.Context, userID vo.Id, from, to, amount string, at time.Time) (converted string, ok bool, err error)
  }
  type TransactionCreator interface {
      CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
  }
  type TransactionLister interface {
      ListByAccount(ctx context.Context, accountID vo.Id, from, to time.Time) ([]*model.Transaction, error)
  }
  type AttemptLimiter interface { Allow(scope, key string) error; Fail(scope, key string) }
  const RateScopeIngest = "ingest"
  // service.go
  func NewService(repo Repository, accounts AccountReader, converter CurrencyConverter, txns TransactionCreator, lister TransactionLister, limiter AttemptLimiter, tx port.TxRunner, clk port.Clock, cfg MatcherConfig) *Service
  // ingest.go
  func (s *Service) IngestAppleWallet(ctx context.Context, userID vo.Id, payload []byte) (*model.IngestEventResult, error)
  func (s *Service) RetryEvent(ctx context.Context, userID vo.Id, req model.RetryImportEventRequest) (*model.IngestEventResult, error)
  // errs codes
  CodeImportSourceNotFound = "import.source_not_found"; CodeImportProviderUnsupported = "import.provider_unsupported"
  CodeImportAccountLinkExists = "import.account_link_exists"; CodeImportCurrencyMismatch = "import.currency_mismatch"
  CodeImportLinkNotQueued = "import.link_not_queued"; CodeImportLinkNotSkipped = "import.link_not_skipped"; CodeImportEventNotFailed = "import.event_not_failed"
  ```
  Ingest statuses (wire, frozen): `"created"`, `"queued"`, `"skipped"`, `"duplicate"`, `"failed"`. A matcher adopt reports `"created"` too — from the shortcut's point of view the tap now has a transaction.
  Wall-clock rule: the parser keeps the payload offset; the pipeline converts once with `wallClock(t)` = `time.Date(y, m, d, h, mi, s, 0, time.UTC)` from `t`'s own components, and uses that for the ledger's `external_posted_at`, the match window, `Match`, and the transaction `date` — the same "local time stored as bare datetime" convention the SPA uses.

- [ ] **Step 1: Error codes** (`internal/shared/errs/codes.go`) — add after the `CodeUser…` group in the const block:

```go
	CodeImportSourceNotFound      = "import.source_not_found"
	CodeImportProviderUnsupported = "import.provider_unsupported"
	CodeImportAccountLinkExists   = "import.account_link_exists"
	CodeImportCurrencyMismatch    = "import.currency_mismatch"
	CodeImportLinkNotQueued       = "import.link_not_queued"
	CodeImportLinkNotSkipped      = "import.link_not_skipped"
	CodeImportEventNotFailed      = "import.event_not_failed"
```

and the same seven names at the end of `AllCodes`. (The i18ntest guard will FAIL until Task 6 adds the catalogue entries — that is expected; run only the packages named in this task until then.)

- [ ] **Step 2: DTOs** (`internal/model/imports_dto.go`)

```go
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

const (
	ImportIngestStatusCreated   = "created"
	ImportIngestStatusQueued    = "queued"
	ImportIngestStatusSkipped   = "skipped"
	ImportIngestStatusDuplicate = "duplicate"
	ImportIngestStatusFailed    = "failed"

	ImportCardStateMapped   = "mapped"
	ImportCardStateIgnored  = "ignored"
	ImportCardStateUnmapped = "unmapped"

	ImportQueueReasonUnmapped       = "unmapped"
	ImportQueueReasonAccountDeleted = "account_deleted"
	ImportQueueReasonNoRate         = "no_rate"
)

func blankField(key string) errs.FieldError {
	return errs.FieldError{Key: key, Message: "This value should not be blank.", Code: errs.CodeIsBlank}
}

func requireNonBlank(pairs ...string) error {
	var fields []errs.FieldError
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			fields = append(fields, blankField(pairs[i]))
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type CreateImportSourceRequest struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

func (r CreateImportSourceRequest) Validate() error {
	if err := requireNonBlank("provider", r.Provider, "name", r.Name); err != nil {
		return err
	}
	if r.Provider != ImportProviderAppleWallet {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "provider", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	return nil
}

type ImportCardResult struct {
	ExternalAccountId string `json:"externalAccountId"`
	ExternalName      string `json:"externalName"`
	ExternalCurrency  string `json:"externalCurrency"`
	State             string `json:"state"`
	AccountId         string `json:"accountId"`
	QueuedCount       int    `json:"queuedCount"`
	TapCount          int    `json:"tapCount"`
	LastSeenAt        string `json:"lastSeenAt"`
}

type ImportSourceResult struct {
	Id        string             `json:"id"`
	Provider  string             `json:"provider"`
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	CreatedAt string             `json:"createdAt"`
	Cards     []ImportCardResult `json:"cards"`
}

type CreateImportSourceResult struct {
	Item ImportSourceResult `json:"item"`
}

type GetImportSourceListResult struct {
	Items []ImportSourceResult `json:"items"`
}

type DeleteImportSourceRequest struct {
	Id string `json:"id"`
}

func (r DeleteImportSourceRequest) Validate() error { return requireNonBlank("id", r.Id) }

type LinkImportAccountRequest struct {
	SourceId          string `json:"sourceId"`
	ExternalAccountId string `json:"externalAccountId"`
	AccountId         string `json:"accountId"`
}

func (r LinkImportAccountRequest) Validate() error {
	return requireNonBlank("sourceId", r.SourceId, "externalAccountId", r.ExternalAccountId, "accountId", r.AccountId)
}

type ImportAccountActionRequest struct {
	SourceId          string `json:"sourceId"`
	ExternalAccountId string `json:"externalAccountId"`
}

func (r ImportAccountActionRequest) Validate() error {
	return requireNonBlank("sourceId", r.SourceId, "externalAccountId", r.ExternalAccountId)
}

type ImportRunResult struct {
	Id            string `json:"id"`
	Status        string `json:"status"`
	ImportedCount int    `json:"importedCount"`
	MatchedCount  int    `json:"matchedCount"`
	SkippedCount  int    `json:"skippedCount"`
	FailedCount   int    `json:"failedCount"`
}

// UpdateImportAccountResult is shared by link/ignore/unlink: the source with
// its refreshed card list, plus the conversion run link-account triggered
// (null for ignore/unlink).
type UpdateImportAccountResult struct {
	Item ImportSourceResult `json:"item"`
	Run  *ImportRunResult   `json:"run"`
}

type IngestEventResult struct {
	Status  string `json:"status"`
	EventId string `json:"eventId"`
}

type ImportQueuedEventResult struct {
	LinkId            string `json:"linkId"`
	SourceId          string `json:"sourceId"`
	ExternalAccountId string `json:"externalAccountId"`
	AccountId         string `json:"accountId"`
	Payee             string `json:"payee"`
	Amount            string `json:"amount"`
	Currency          string `json:"currency"`
	Type              string `json:"type"`
	PostedAt          string `json:"postedAt"`
	Reason            string `json:"reason"`
}

// Payload is the raw body as received (the queue page's "needs attention"
// section shows it so the user can see what the shortcut actually sent).
type ImportFailedEventResult struct {
	EventId    string `json:"eventId"`
	SourceId   string `json:"sourceId"`
	ReceivedAt string `json:"receivedAt"`
	Error      string `json:"error"`
	Payload    string `json:"payload"`
}

type GetImportQueueResult struct {
	Queued  []ImportQueuedEventResult `json:"queued"`
	Skipped []ImportQueuedEventResult `json:"skipped"`
	Failed  []ImportFailedEventResult `json:"failed"`
}

type ImportQueuedEventRequest struct {
	LinkId      string                   `json:"linkId"`
	Transaction CreateTransactionRequest `json:"transaction"`
}

func (r ImportQueuedEventRequest) Validate() error {
	if err := requireNonBlank("linkId", r.LinkId); err != nil {
		return err
	}
	return r.Transaction.Validate()
}

type ImportLinkActionRequest struct {
	LinkId string `json:"linkId"`
}

func (r ImportLinkActionRequest) Validate() error { return requireNonBlank("linkId", r.LinkId) }

type RetryImportEventRequest struct {
	EventId string `json:"eventId"`
}

func (r RetryImportEventRequest) Validate() error { return requireNonBlank("eventId", r.EventId) }

type DiscardImportEventRequest struct {
	EventId string `json:"eventId"`
}

func (r DiscardImportEventRequest) Validate() error { return requireNonBlank("eventId", r.EventId) }

type TransactionImportListRequest struct {
	TransactionId string `json:"transactionId"`
}

func (r TransactionImportListRequest) Validate() error { return requireNonBlank("transactionId", r.TransactionId) }

type TransactionImportLinkResult struct {
	Id                    string `json:"id"`
	SourceId              string `json:"sourceId"`
	Provider              string `json:"provider"`
	SourceName            string `json:"sourceName"`
	ExternalAccountId     string `json:"externalAccountId"`
	ExternalTransactionId string `json:"externalTransactionId"`
	ExternalPayee         string `json:"externalPayee"`
	ExternalAmount        string `json:"externalAmount"`
	ExternalCurrency      string `json:"externalCurrency"`
	ExternalPostedAt      string `json:"externalPostedAt"`
	Status                string `json:"status"`
	ImportedAt            string `json:"importedAt"`
}

type GetTransactionImportListResult struct {
	Items []TransactionImportLinkResult `json:"items"`
}
```

If a helper named `blankField`/`requireNonBlank` already exists in `internal/model`, reuse it and delete the duplicate here.

- [ ] **Step 3: Ports** (`internal/imports/ports.go`)

```go
// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package imports

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountReader is the slice of the account feature the pipeline needs:
// ownership (link-account), dormancy (a deleted account queues instead of
// failing), and the currency to convert into.
type AccountReader interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	AccountDeleted(ctx context.Context, accountID vo.Id) (bool, error)
	AccountCurrencyCode(ctx context.Context, accountID vo.Id) (string, error)
}

// CurrencyConverter re-denominates amount (decimal text) from code `from`
// into `to` at `at`. ok=false means "no rate for that pair and date" (or an
// unknown code) — the event queues; the shared convertor's silent 1:1
// fallback must never reach an imported amount.
type CurrencyConverter interface {
	Convert(ctx context.Context, userID vo.Id, from, to, amount string, at time.Time) (converted string, ok bool, err error)
}

// TransactionCreator is the transaction feature's create use case, so an
// import goes through exactly the checks a hand-entered transaction does
// (deleted account, write access, idempotency on the request id).
type TransactionCreator interface {
	CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
}

// TransactionLister pre-selects the matcher's candidates: the account's
// transactions with spent_at inside [from, to].
type TransactionLister interface {
	ListByAccount(ctx context.Context, accountID vo.Id, from, to time.Time) ([]*model.Transaction, error)
}

// AttemptLimiter caps ingest per user; every request counts (Allow then
// Fail). A nil limiter disables protection (tests).
type AttemptLimiter interface {
	Allow(scope, key string) error
	Fail(scope, key string)
}

const RateScopeIngest = "ingest"
```

- [ ] **Step 4: Service core** (`internal/imports/service.go`)

```go
package imports

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Service struct {
	repo      Repository
	accounts  AccountReader
	converter CurrencyConverter
	txns      TransactionCreator
	lister    TransactionLister
	limiter   AttemptLimiter
	tx        port.TxRunner
	clk       port.Clock
	cfg       MatcherConfig
}

func NewService(repo Repository, accounts AccountReader, converter CurrencyConverter, txns TransactionCreator, lister TransactionLister, limiter AttemptLimiter, tx port.TxRunner, clk port.Clock, cfg MatcherConfig) *Service {
	return &Service{repo: repo, accounts: accounts, converter: converter, txns: txns, lister: lister, limiter: limiter, tx: tx, clk: clk, cfg: cfg}
}

// ownedSource resolves a source the caller owns; a foreign id reads as
// not-found so ids cannot be probed.
func (s *Service) ownedSource(ctx context.Context, userID vo.Id, rawID string) (*model.ImportSource, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, errs.NewNotFound("Import source not found")
	}
	src, err := s.repo.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if src.UserID != userID {
		return nil, errs.NewNotFound("Import source not found")
	}
	return src, nil
}

// ownedLink resolves a ledger row through its source's ownership.
func (s *Service) ownedLink(ctx context.Context, userID vo.Id, rawID string) (*model.ImportTransactionLink, *model.ImportSource, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, nil, errs.NewNotFound("Import link not found")
	}
	link, err := s.repo.GetLink(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	src, err := s.ownedSource(ctx, userID, link.SourceID.String())
	if err != nil {
		return nil, nil, errs.NewNotFound("Import link not found")
	}
	return link, src, nil
}

func (s *Service) ownedEvent(ctx context.Context, userID vo.Id, rawID string) (*model.ImportEvent, *model.ImportSource, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, nil, errs.NewNotFound("Import event not found")
	}
	ev, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	src, err := s.ownedSource(ctx, userID, ev.SourceID.String())
	if err != nil {
		return nil, nil, errs.NewNotFound("Import event not found")
	}
	return ev, src, nil
}

// findAccountLink matches a card name case-insensitively: the shortcut
// delivers whatever Wallet renders, and "Apple Card" vs "apple card" is the
// same card.
func findAccountLink(links []model.ImportAccountLink, externalAccountID string) *model.ImportAccountLink {
	for i := range links {
		if strings.EqualFold(links[i].ExternalAccountID, externalAccountID) {
			return &links[i]
		}
	}
	return nil
}

// wallClock drops the offset and keeps the digits: transactions store the
// user's local time as a bare datetime, so a 10:42 -07:00 tap must become a
// 10:42 transaction, and the matcher compares it against spent_at the same way.
func wallClock(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func idString(p *vo.Id) string {
	if p == nil {
		return ""
	}
	return p.String()
}

// sourceResult assembles the wire view of a source: cards are the union of
// the user's explicit links and every card the ledger has seen, so an
// unmapped card shows up with its queued count before the user acts on it.
func (s *Service) sourceResult(ctx context.Context, src *model.ImportSource) (*model.ImportSourceResult, error) {
	accountLinks, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	cards := make([]model.ImportCardResult, 0, len(accountLinks))
	index := map[string]int{}
	for _, al := range accountLinks {
		state := model.ImportCardStateMapped
		if al.Mode == model.ImportAccountLinkModeIgnore {
			state = model.ImportCardStateIgnored
		}
		index[strings.ToLower(al.ExternalAccountID)] = len(cards)
		cards = append(cards, model.ImportCardResult{
			ExternalAccountId: al.ExternalAccountID, ExternalName: al.ExternalName, ExternalCurrency: derefString(al.ExternalCurrency),
			State: state, AccountId: idString(al.AccountID),
		})
	}
	for _, l := range ledger { // newest first (repo order), so the first hit per card is its last-seen tap
		key := strings.ToLower(l.ExternalAccountID)
		i, ok := index[key]
		if !ok {
			index[key] = len(cards)
			cards = append(cards, model.ImportCardResult{ExternalAccountId: l.ExternalAccountID, ExternalName: l.ExternalAccountID, State: model.ImportCardStateUnmapped})
			i = len(cards) - 1
		}
		cards[i].TapCount++
		if l.Status == model.ImportLinkStatusQueued {
			cards[i].QueuedCount++
		}
		if cards[i].LastSeenAt == "" {
			cards[i].LastSeenAt = l.ExternalPostedAt.Format(datetime.Layout)
		}
	}
	return &model.ImportSourceResult{
		Id: src.ID.String(), Provider: src.Provider, Name: src.Name, Status: src.Status,
		CreatedAt: src.CreatedAt.Format(datetime.Layout), Cards: cards,
	}, nil
}
```

- [ ] **Step 5: Failing pipeline tests** (`internal/imports/ingest_test.go`, `package imports_test`)

```go
package imports_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID  = "dffc2a06-6f29-4704-8575-31709adee926"
	userA  = "0a000000-0000-0000-0000-000000000001"
	userB  = "0a000000-0000-0000-0000-000000000002"
	acct1  = "0a000000-0000-0000-0000-0000000000a1"
	acctB  = "0a000000-0000-0000-0000-0000000000b1"
	source = "0c000000-0000-0000-0000-000000000001"
)

var now = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

type clock struct{ t time.Time }

func (c clock) Now() time.Time { return c.t }

// fakeAccounts: acct1 (userA, USD, live), acctB (userB). deleted toggles acct1.
type fakeAccounts struct{ deleted bool }

func (f *fakeAccounts) AccountOwner(_ context.Context, id vo.Id) (vo.Id, error) {
	switch id.String() {
	case acct1:
		return vo.MustParseId(userA), nil
	case acctB:
		return vo.MustParseId(userB), nil
	}
	return vo.Id{}, errs.NewNotFound("Account not found")
}
func (f *fakeAccounts) AccountDeleted(_ context.Context, id vo.Id) (bool, error) {
	if id.String() != acct1 && id.String() != acctB {
		return false, errs.NewNotFound("Account not found")
	}
	return f.deleted && id.String() == acct1, nil
}
func (f *fakeAccounts) AccountCurrencyCode(_ context.Context, id vo.Id) (string, error) {
	return "USD", nil
}

// fakeConverter knows EUR->USD only.
type fakeConverter struct{ calls int }

func (f *fakeConverter) Convert(_ context.Context, _ vo.Id, from, to, amount string, _ time.Time) (string, bool, error) {
	f.calls++
	if from == "EUR" && to == "USD" {
		return vo.NewDecimal(amount).Mul(vo.NewDecimal("1.10")).String(), true, nil
	}
	return "", false, nil
}

// fakeTxns records creates and serves the candidate list.
type fakeTxns struct {
	created    []model.CreateTransactionRequest
	candidates []*model.Transaction
	fail       error
}

func (f *fakeTxns) CreateTransaction(_ context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	f.created = append(f.created, req)
	return &model.CreateTransactionResult{Item: model.TransactionResult{Id: vo.NewId().String(), AccountId: req.AccountId, Amount: string(req.Amount)}}, nil
}
func (f *fakeTxns) ListByAccount(_ context.Context, _ vo.Id, _, _ time.Time) ([]*model.Transaction, error) {
	return f.candidates, nil
}

type limiter struct{ allow, fail int }

func (l *limiter) Allow(scope, key string) error {
	l.allow++
	if l.allow > 2 {
		return errs.NewTooManyRequests("Too many attempts. Try again later.")
	}
	return nil
}
func (l *limiter) Fail(scope, key string) { l.fail++ }

type harness struct {
	svc      *imports.Service
	repo     *importsrepo.Repo
	accounts *fakeAccounts
	conv     *fakeConverter
	txns     *fakeTxns
	lim      *limiter
	f        *fixture.Builder
}

func setup(t *testing.T) *harness {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db).At(now)
	f.User(fixture.User{ID: userA, Email: "a@example.test", Name: "A"})
	f.User(fixture.User{ID: userB, Email: "b@example.test", Name: "B"})
	f.Account(fixture.Account{ID: acct1, UserID: userA, CurrencyID: usdID, Name: "Card"})
	f.Account(fixture.Account{ID: acctB, UserID: userB, CurrencyID: usdID, Name: "Other"})
	f.ImportSource(fixture.ImportSource{ID: source, UserID: userA, Name: "iPhone"})
	repo := importsrepo.NewRepo(db.Engine, db.TX)
	h := &harness{repo: repo, accounts: &fakeAccounts{}, conv: &fakeConverter{}, txns: &fakeTxns{}, lim: &limiter{}, f: f}
	h.svc = imports.NewService(repo, h.accounts, h.conv, h.txns, h.txns, h.lim, db.TX, clock{now}, imports.DefaultMatcherConfig())
	return h
}

func (h *harness) mapCard(t *testing.T, card string) {
	t.Helper()
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: card, AccountID: acct1})
}

func ingest(t *testing.T, h *harness, body string) *model.IngestEventResult {
	t.Helper()
	res, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userA), []byte(body))
	if err != nil {
		t.Fatalf("IngestAppleWallet: %v", err)
	}
	return res
}

const tap = `{"account":"Apple Card","payee":"Blue Bottle","amount":"4.75","currency":"USD","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"evt-1"}`

func TestIngest_UnmappedCardQueues(t *testing.T) {
	h := setup(t)
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusQueued || res.EventId == "" {
		t.Fatalf("res = %+v", res)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusQueued || links[0].ExternalPayee != "Blue Bottle" || links[0].ExternalCurrency == nil || *links[0].ExternalCurrency != "USD" {
		t.Fatalf("ledger = %+v", links)
	}
	if got := links[0].ExternalPostedAt.Format("2006-01-02 15:04:05"); got != "2026-08-20 10:42:03" {
		t.Errorf("posted_at must be the tap's wall clock, got %s", got)
	}
	if len(h.txns.created) != 0 {
		t.Error("nothing may be created for an unmapped card")
	}
	// a re-fired identical payload is a duplicate at the inbox
	if again := ingest(t, h, tap); again.Status != model.ImportIngestStatusDuplicate {
		t.Errorf("re-fire = %+v", again)
	}
	// same tap, different formatting: same external id -> still queued, no second row
	if again := ingest(t, h, `{"account":" apple  card","payee":"Blue Bottle","amount":"$4.75","currency":"usd","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"evt-1"}`); again.Status != model.ImportIngestStatusQueued {
		t.Errorf("reformatted = %+v", again)
	}
	if links, _ = h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source)); len(links) != 1 {
		t.Errorf("ledger must not fork: %d rows", len(links))
	}
}

func TestIngest_MappedCardCreatesTransaction(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "apple card") // case-insensitive lookup
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("res = %+v", res)
	}
	if len(h.txns.created) != 1 {
		t.Fatalf("created = %+v", h.txns.created)
	}
	req := h.txns.created[0]
	if req.AccountId != acct1 || req.Type != "expense" || string(req.Amount) != vo.NewDecimal("4.75").String() || req.Date != "2026-08-20 10:42:03" || req.Description == nil || *req.Description != "Blue Bottle" || req.Id == "" {
		t.Errorf("create request = %+v", req)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusLinked || links[0].TransactionID == nil {
		t.Fatalf("ledger = %+v", links)
	}
	// the exact key is now seen: re-delivery with a fresh payload hash is a duplicate at the ledger
	if again := ingest(t, h, `{"account":"Apple Card","payee":"Blue Bottle","amount":"4.75","currency":"USD","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"evt-1","type":"expense"}`); again.Status != model.ImportIngestStatusDuplicate {
		t.Errorf("re-delivery = %+v", again)
	}
}

func TestIngest_AdoptsHandEnteredTransaction(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	existing := vo.NewId()
	h.txns.candidates = []*model.Transaction{{ID: existing, AccountID: vo.MustParseId(acct1), Type: model.TransactionTypeExpense, Amount: "4.75000000", SpentAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)}}
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusCreated || len(h.txns.created) != 0 {
		t.Fatalf("res = %+v, created = %d", res, len(h.txns.created))
	}
	links, _ := h.repo.ListLinksByTransaction(context.Background(), existing)
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusLinked {
		t.Fatalf("adopt must link the existing transaction: %+v", links)
	}
}

func TestIngest_IgnoredCardSkips(t *testing.T) {
	h := setup(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", Mode: "ignore"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusSkipped {
		t.Fatalf("res = %+v", res)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusSkipped {
		t.Fatalf("ledger = %+v", links)
	}
}

func TestIngest_DeletedAccountQueues(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	h.accounts.deleted = true
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("res = %+v", res)
	}
}

func TestIngest_CurrencyConversion(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	eur := `{"account":"Apple Card","payee":"Cafe","amount":"10","currency":"EUR","occurredAt":"2026-08-20T10:42:03+02:00","eventId":"e-eur"}`
	if res := ingest(t, h, eur); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("res = %+v", res)
	}
	if got := string(h.txns.created[0].Amount); !vo.NewDecimal(got).Equals(vo.NewDecimal("11")) {
		t.Errorf("converted amount = %s", got)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if !vo.NewDecimal(links[0].ExternalAmount).Equals(vo.NewDecimal("10")) || *links[0].ExternalCurrency != "EUR" {
		t.Errorf("ledger must keep the original amount/currency: %+v", links[0])
	}
	// no rate -> queued, nothing created
	gbp := `{"account":"Apple Card","payee":"Pub","amount":"10","currency":"GBP","occurredAt":"2026-08-20T10:42:03+01:00","eventId":"e-gbp"}`
	if res := ingest(t, h, gbp); res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("no-rate res = %+v", res)
	}
	if len(h.txns.created) != 1 {
		t.Errorf("no-rate tap must not create: %d", len(h.txns.created))
	}
}

func TestIngest_ParseFailureIsStoredAndReported(t *testing.T) {
	h := setup(t)
	res := ingest(t, h, `{"account":"Apple Card","amount":"nope","currency":"USD"}`)
	if res.Status != model.ImportIngestStatusFailed || res.EventId == "" {
		t.Fatalf("res = %+v", res)
	}
	ev, err := h.repo.GetEvent(context.Background(), vo.MustParseId(res.EventId))
	if err != nil || ev.Status != model.ImportEventStatusFailed || ev.ParseError == nil || *ev.ParseError != "amount must be a positive number" {
		t.Fatalf("event = %+v, %v", ev, err)
	}
	// not even JSON: still persisted, still 'failed'
	if res := ingest(t, h, `garbage`); res.Status != model.ImportIngestStatusFailed {
		t.Errorf("garbage = %+v", res)
	}
}

func TestIngest_NoSourceIsCoded400(t *testing.T) {
	h := setup(t)
	_, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userB), []byte(tap))
	verr, ok := errs.AsValidation(err)
	if !ok || verr.MsgCode != errs.CodeImportSourceNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestIngest_RateLimited(t *testing.T) {
	h := setup(t)
	ingest(t, h, tap)
	ingest(t, h, `{"account":"Apple Card","amount":"1","currency":"USD","eventId":"x"}`)
	_, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userA), []byte(tap))
	var tooMany *errs.TooManyRequestsError
	if !errors.As(err, &tooMany) {
		t.Fatalf("third call = %v, want 429", err)
	}
	if h.lim.fail != 2 {
		t.Errorf("every accepted request must count: fail=%d", h.lim.fail)
	}
}

func TestIngest_CreateFailureRollsBack(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	h.txns.fail = errors.New("boom")
	if _, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userA), []byte(tap)); err == nil {
		t.Fatal("create failure must surface")
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	events, _ := h.repo.ListEventsBySourceStatus(context.Background(), vo.MustParseId(source), model.ImportEventStatusProcessed)
	if len(links) != 0 || len(events) != 0 {
		t.Errorf("one transaction: nothing may persist after a failed create (links=%d events=%d)", len(links), len(events))
	}
}

func TestRetryEvent(t *testing.T) {
	h := setup(t)
	failed := ingest(t, h, `{"account":"Apple Card","amount":"nope","currency":"USD"}`)
	// retry re-parses the stored payload; it is still broken, so it stays failed
	res, err := h.svc.RetryEvent(context.Background(), vo.MustParseId(userA), model.RetryImportEventRequest{EventId: failed.EventId})
	if err != nil || res.Status != model.ImportIngestStatusFailed {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	ok := ingest(t, h, tap)
	_, err = h.svc.RetryEvent(context.Background(), vo.MustParseId(userA), model.RetryImportEventRequest{EventId: ok.EventId})
	if verr, isV := errs.AsValidation(err); !isV || verr.MsgCode != errs.CodeImportEventNotFailed {
		t.Fatalf("retry of a processed event = %v", err)
	}
	if _, err := h.svc.RetryEvent(context.Background(), vo.MustParseId(userB), model.RetryImportEventRequest{EventId: failed.EventId}); err == nil {
		t.Fatal("foreign user must not retry")
	}
}
```

Check `fixture.User`'s required fields against `internal/test/fixture/entities.go` (copy what `internal/imports/repo/repo_integration_test.go` passes). If `fixture.New(t, db).At(now)` is not chainable that way, call `f := fixture.New(t, db); f = f.At(now)`.

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/imports/ 2>&1 | head`
Expected: build errors (`imports.NewService` undefined …).

- [ ] **Step 7: Implement the pipeline** (`internal/imports/ingest.go`)

```go
package imports

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// IngestAppleWallet is the push endpoint's use case. The raw payload is
// persisted before anything is interpreted, so a broken body is a 'failed'
// event the user can see and discard — never a lost tap. Everything runs in
// one transaction: a failed create leaves no event, no ledger row.
func (s *Service) IngestAppleWallet(ctx context.Context, userID vo.Id, payload []byte) (*model.IngestEventResult, error) {
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeIngest, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopeIngest, userID.String())
	}
	var res *model.IngestEventResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.repo.GetSourceByUserProvider(ctx, userID, model.ImportProviderAppleWallet)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return &errs.ValidationError{Msg: "Apple Wallet is not connected", MsgCode: errs.CodeImportSourceNotFound}
			}
			return err
		}
		ev := &model.ImportEvent{
			ID: vo.NewId(), SourceID: src.ID, Payload: string(payload), PayloadHash: HashPayload(payload),
			Status: model.ImportEventStatusProcessed, ReceivedAt: s.clk.Now().UTC(),
		}
		inserted, err := s.repo.InsertEvent(ctx, ev)
		if err != nil {
			return err
		}
		if !inserted {
			res = &model.IngestEventResult{Status: model.ImportIngestStatusDuplicate}
			return nil
		}
		status, err := s.processEvent(ctx, src, ev)
		if err != nil {
			return err
		}
		res = &model.IngestEventResult{Status: status, EventId: ev.ID.String()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// RetryEvent re-runs a failed event's parse + pipeline. The payload is what
// it was; a retry is useful after the user mapped the card or the server was
// upgraded with a more tolerant parser.
func (s *Service) RetryEvent(ctx context.Context, userID vo.Id, req model.RetryImportEventRequest) (*model.IngestEventResult, error) {
	var res *model.IngestEventResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		ev, src, err := s.ownedEvent(ctx, userID, req.EventId)
		if err != nil {
			return err
		}
		if ev.Status != model.ImportEventStatusFailed {
			return &errs.ValidationError{Msg: "This event did not fail", MsgCode: errs.CodeImportEventNotFailed}
		}
		status, err := s.processEvent(ctx, src, ev)
		if err != nil {
			return err
		}
		res = &model.IngestEventResult{Status: status, EventId: ev.ID.String()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// processEvent runs stages 0-3 for one stored event and records the outcome
// on the event row. It returns the ingest status; only infrastructure
// failures are errors.
func (s *Service) processEvent(ctx context.Context, src *model.ImportSource, ev *model.ImportEvent) (string, error) {
	parsed, perr := s.parse(src, ev)
	if perr != nil {
		msg := perr.Error()
		return model.ImportIngestStatusFailed, s.repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusFailed, &msg)
	}
	status, err := s.applyEvent(ctx, src, ev.ID, parsed)
	if err != nil {
		return "", err
	}
	return status, s.repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusProcessed, nil)
}

func (s *Service) parse(src *model.ImportSource, ev *model.ImportEvent) (model.IngestEvent, error) {
	switch src.Provider {
	case model.ImportProviderAppleWallet:
		return ParseAppleWalletEvent([]byte(ev.Payload), ev.ReceivedAt)
	default:
		return model.IngestEvent{}, errors.New("unsupported provider " + src.Provider)
	}
}

// resolution is stage 1+2's verdict for an event: where it goes and in what
// amount, or why it cannot go anywhere yet.
type resolution struct {
	accountID vo.Id
	amount    string
	status    string // "" = import; ImportIngestStatusQueued / ImportIngestStatusSkipped otherwise
}

func (s *Service) resolve(ctx context.Context, src *model.ImportSource, ev model.IngestEvent) (resolution, error) {
	links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
	if err != nil {
		return resolution{}, err
	}
	al := findAccountLink(links, ev.ExternalAccountID)
	if al == nil {
		return resolution{status: model.ImportIngestStatusQueued}, nil
	}
	if al.Mode == model.ImportAccountLinkModeIgnore || al.AccountID == nil {
		return resolution{status: model.ImportIngestStatusSkipped}, nil
	}
	deleted, err := s.accounts.AccountDeleted(ctx, *al.AccountID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return resolution{status: model.ImportIngestStatusQueued}, nil
		}
		return resolution{}, err
	}
	if deleted { // dormant link: keep the tap for when the user re-maps the card
		return resolution{status: model.ImportIngestStatusQueued}, nil
	}
	code, err := s.accounts.AccountCurrencyCode(ctx, *al.AccountID)
	if err != nil {
		return resolution{}, err
	}
	amount := ev.Amount
	if ev.Currency != "" && ev.Currency != code {
		converted, ok, err := s.converter.Convert(ctx, src.UserID, ev.Currency, code, ev.Amount, wallClock(ev.PostedAt))
		if err != nil {
			return resolution{}, err
		}
		if !ok {
			return resolution{status: model.ImportIngestStatusQueued}, nil
		}
		amount = converted
	}
	return resolution{accountID: *al.AccountID, amount: amount}, nil
}

// applyEvent is stages 1-3 for a parsed event: the ledger check, the account
// resolution, and matching/creation. It always leaves exactly one ledger row
// for a new external key.
func (s *Service) applyEvent(ctx context.Context, src *model.ImportSource, eventID vo.Id, ev model.IngestEvent) (string, error) {
	existing, err := s.repo.GetLinkByExternalKey(ctx, src.ID, ev.ExternalAccountID, ev.ExternalTransactionID)
	if err == nil {
		if existing.IsSeen() {
			return model.ImportIngestStatusDuplicate, nil
		}
		return model.ImportIngestStatusQueued, nil // still waiting on the user; nothing new to record
	} else if _, ok := errs.AsNotFound(err); !ok {
		return "", err
	}
	link := &model.ImportTransactionLink{
		ID: vo.NewId(), SourceID: src.ID, EventID: &eventID,
		ExternalAccountID: ev.ExternalAccountID, ExternalTransactionID: ev.ExternalTransactionID,
		Status: model.ImportLinkStatusQueued, ExternalPayee: ev.Payee, ExternalDescription: ev.Description,
		ExternalAmount: ev.Amount, ExternalCurrency: optionalString(ev.Currency),
		ExternalPostedAt: wallClock(ev.PostedAt), ImportedAt: s.clk.Now().UTC(),
	}
	r, err := s.resolve(ctx, src, ev)
	if err != nil {
		return "", err
	}
	switch r.status {
	case model.ImportIngestStatusQueued:
		return r.status, s.repo.InsertLink(ctx, link)
	case model.ImportIngestStatusSkipped:
		link.Status = model.ImportLinkStatusSkipped
		return r.status, s.repo.InsertLink(ctx, link)
	}
	txID, _, err := s.place(ctx, src, ev, r)
	if err != nil {
		return "", err
	}
	link.Status = model.ImportLinkStatusLinked
	link.TransactionID = &txID
	return model.ImportIngestStatusCreated, s.repo.InsertLink(ctx, link)
}

// place is stage 3: adopt an existing transaction the matcher recognizes
// (adopted=true), else create one through the transaction feature's own use
// case.
func (s *Service) place(ctx context.Context, src *model.ImportSource, ev model.IngestEvent, r resolution) (txID vo.Id, adopted bool, err error) {
	at := wallClock(ev.PostedAt)
	window := s.cfg.MatchDays
	if s.cfg.TipDays > window {
		window = s.cfg.TipDays
	}
	txs, err := s.lister.ListByAccount(ctx, r.accountID, at.AddDate(0, 0, -window), at.AddDate(0, 0, window+1))
	if err != nil {
		return vo.Id{}, false, err
	}
	providers := map[vo.Id]string{}
	candidates := make([]model.ImportCandidate, 0, len(txs))
	for _, t := range txs {
		c := model.ImportCandidate{TransactionID: t.ID, Type: t.Type, Amount: t.Amount, SpentAt: t.SpentAt}
		links, err := s.repo.ListLinksByTransaction(ctx, t.ID)
		if err != nil {
			return vo.Id{}, false, err
		}
		for _, l := range links {
			provider, seen := providers[l.SourceID]
			if !seen {
				if ls, err := s.repo.GetSource(ctx, l.SourceID); err == nil {
					provider = ls.Provider
				}
				providers[l.SourceID] = provider
			}
			c.Links = append(c.Links, model.ImportCandidateLink{SourceID: l.SourceID, Provider: provider, ExternalAmount: l.ExternalAmount, ExternalPayee: l.ExternalPayee})
		}
		candidates = append(candidates, c)
	}
	matchEv := ev
	matchEv.Amount = r.amount // compare in the account's currency
	matchEv.PostedAt = at
	if m := Match(matchEv, src.ID, candidates, s.cfg); m.Kind != MatchCreate {
		return m.TransactionID, true, nil
	}
	res, err := s.txns.CreateTransaction(ctx, src.UserID, model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: ev.Type.Alias(), Amount: vo.FlexString(r.amount), AccountId: r.accountID.String(),
		Date: at.Format(datetime.Layout), Description: optionalString(ev.Payee),
	})
	if err != nil {
		return vo.Id{}, false, err
	}
	txID, err = vo.ParseId(res.Item.Id)
	return txID, false, err
}
```

Check `model.CreateTransactionRequest`'s exact field types in `internal/model/transaction_dto.go` (`Amount vo.FlexString`, `Description *string`, `Date string`) and adjust the literal if they differ. `ev.Type.Alias()` yields `"expense"`/`"income"`.

- [ ] **Step 8: Run the pipeline tests**

Run: `go test ./internal/imports/... 2>&1 | tail -20`
Expected: PASS for `internal/imports` and `internal/imports/repo`. If `TestIngest_AdoptsHandEnteredTransaction` fails because `Match` filters by `c.Type != ev.Type` on `model.TransactionTypeExpense` vs the candidate's type — the fake candidate sets `Type: model.TransactionTypeExpense`; verify `Match`'s adopt path (`matcher.go:46-120`) accepts a same-day equal-amount candidate with no links — it does (stage 1 tests cover it).

- [ ] **Step 9: archtest + vet + commit**

Run: `go vet ./internal/imports/... ./internal/model/... && go test ./internal/test/archtest/`
Expected: PASS (imports imports only `model`, `shared`).

```bash
git add internal/imports/ports.go internal/imports/service.go internal/imports/ingest.go internal/imports/ingest_test.go internal/model/imports_dto.go internal/shared/errs/codes.go
git commit -m "feat(imports): ingest pipeline service with account, currency and transaction ports"
```

---

### Task 4: Sources and card (account-link) management

**Files:**
- Create: `internal/imports/source.go`, `internal/imports/accountlink.go`
- Test: `internal/imports/source_test.go`, `internal/imports/accountlink_test.go` (package `imports_test`, reuse the harness from `ingest_test.go`)

**Interfaces:**
- Consumes: Task 1 repository, Task 3 `Service` (`ownedSource`, `sourceResult`, `findAccountLink`, `resolve`, `place`, `parse`, `wallClock`, `optionalString`), Task 3 DTOs.
- Produces:
  ```go
  func (s *Service) CreateSource(ctx, userID vo.Id, req model.CreateImportSourceRequest) (*model.CreateImportSourceResult, error)  // idempotent per (user, provider)
  func (s *Service) GetSourceList(ctx, userID vo.Id) (*model.GetImportSourceListResult, error)
  func (s *Service) DeleteSource(ctx, userID vo.Id, req model.DeleteImportSourceRequest) (*model.GetImportSourceListResult, error)
  func (s *Service) LinkAccount(ctx, userID vo.Id, req model.LinkImportAccountRequest) (*model.UpdateImportAccountResult, error)
  func (s *Service) IgnoreAccount(ctx, userID vo.Id, req model.ImportAccountActionRequest) (*model.UpdateImportAccountResult, error)
  func (s *Service) UnlinkAccount(ctx, userID vo.Id, req model.ImportAccountActionRequest) (*model.UpdateImportAccountResult, error)
  ```
  Semantics (spec §"Cards"): `LinkAccount` maps a card to an owned, live account whose currency equals the card's currency (the single currency across the card's ledger rows; a card with no rows or mixed rows has no currency and is accepted); it then runs a **conversion run** over the card's queued taps (creates/adopts through the same stage 3 as ingest) and returns the run counts. `IgnoreAccount` records the card as ignored and flips its queued taps to `skipped`. `UnlinkAccount` forgets the card: its mapping row, its queued taps AND their inbox events (so a re-fired tap is treated as new), while seen rows stay as dedupe memory.

- [ ] **Step 1: Failing tests** (`internal/imports/source_test.go`)

```go
package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestCreateSource_IdempotentPerProvider(t *testing.T) {
	h := setup(t) // userA already has `source`
	uB := vo.MustParseId(userB)
	first, err := h.svc.CreateSource(context.Background(), uB, model.CreateImportSourceRequest{Provider: "apple-wallet", Name: "My iPhone"})
	if err != nil || first.Item.Provider != "apple-wallet" || first.Item.Name != "My iPhone" || first.Item.Status != "active" || first.Item.Cards == nil {
		t.Fatalf("create = %+v, %v", first, err)
	}
	second, err := h.svc.CreateSource(context.Background(), uB, model.CreateImportSourceRequest{Provider: "apple-wallet", Name: "Other name"})
	if err != nil || second.Item.Id != first.Item.Id || second.Item.Name != "My iPhone" {
		t.Fatalf("second create must return the existing source: %+v, %v", second, err)
	}
	if err := (model.CreateImportSourceRequest{Provider: "plaid", Name: "x"}).Validate(); err == nil {
		t.Fatal("unknown provider must fail validation")
	}
}

func TestGetSourceList_CardsUnionLedgerAndLinks(t *testing.T) {
	h := setup(t)
	ingest(t, h, tap)                                                                                   // "Apple Card" queued (unmapped)
	ingest(t, h, `{"account":"Apple Card","amount":"2","currency":"USD","eventId":"evt-2"}`)          // second tap on the same card
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Old Card", Mode: "ignore"})
	res, err := h.svc.GetSourceList(context.Background(), vo.MustParseId(userA))
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("list = %+v, %v", res, err)
	}
	cards := map[string]model.ImportCardResult{}
	for _, c := range res.Items[0].Cards {
		cards[c.ExternalAccountId] = c
	}
	if c := cards["Apple Card"]; c.State != "unmapped" || c.QueuedCount != 2 || c.TapCount != 2 || c.LastSeenAt == "" || c.AccountId != "" {
		t.Errorf("unmapped card = %+v", c)
	}
	if c := cards["Old Card"]; c.State != "ignored" || c.TapCount != 0 {
		t.Errorf("ignored card = %+v", c)
	}
	// another user sees nothing
	other, _ := h.svc.GetSourceList(context.Background(), vo.MustParseId(userB))
	if len(other.Items) != 0 {
		t.Errorf("foreign list = %+v", other)
	}
}

func TestDeleteSource(t *testing.T) {
	h := setup(t)
	ingest(t, h, tap)
	if _, err := h.svc.DeleteSource(context.Background(), vo.MustParseId(userB), model.DeleteImportSourceRequest{Id: source}); err == nil {
		t.Fatal("foreign delete must fail")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("foreign delete = %T, want NotFound", err)
	}
	res, err := h.svc.DeleteSource(context.Background(), vo.MustParseId(userA), model.DeleteImportSourceRequest{Id: source})
	if err != nil || len(res.Items) != 0 {
		t.Fatalf("delete = %+v, %v", res, err)
	}
	if _, err := h.repo.GetSource(context.Background(), vo.MustParseId(source)); err == nil {
		t.Error("source row must be gone (cascade removes events/links)")
	}
}
```

Add `"github.com/econumo/econumo/internal/test/fixture"` to the imports.

- [ ] **Step 2: Failing tests** (`internal/imports/accountlink_test.go`)

```go
package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func cardByID(res *model.UpdateImportAccountResult, ext string) model.ImportCardResult {
	for _, c := range res.Item.Cards {
		if c.ExternalAccountId == ext {
			return c
		}
	}
	return model.ImportCardResult{}
}

func TestLinkAccount_RunsConversionOverQueuedTaps(t *testing.T) {
	h := setup(t)
	ingest(t, h, tap) // queued: unmapped
	ingest(t, h, `{"account":"apple card","payee":"Shop","amount":"2","currency":"USD","occurredAt":"2026-08-19T09:00:00Z","eventId":"evt-2"}`)
	res, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if err != nil {
		t.Fatalf("LinkAccount: %v", err)
	}
	if res.Run == nil || res.Run.Status != model.ImportRunStatusCompleted || res.Run.ImportedCount != 2 || res.Run.MatchedCount != 0 || res.Run.SkippedCount != 0 {
		t.Fatalf("run = %+v", res.Run)
	}
	if len(h.txns.created) != 2 {
		t.Fatalf("conversion must create both queued taps: %d", len(h.txns.created))
	}
	if c := cardByID(res, "Apple Card"); c.State != "mapped" || c.AccountId != acct1 || c.QueuedCount != 0 || c.TapCount != 2 {
		t.Errorf("card after link = %+v", c)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	for _, l := range links {
		if l.Status != model.ImportLinkStatusLinked || l.TransactionID == nil || l.RunID == nil {
			t.Errorf("link must be linked + stamped with the run: %+v", l)
		}
	}
	// the next tap goes straight through
	if r := ingest(t, h, `{"account":"Apple Card","amount":"3","currency":"USD","eventId":"evt-3"}`); r.Status != model.ImportIngestStatusCreated {
		t.Errorf("post-link ingest = %+v", r)
	}
}

func TestLinkAccount_Rejections(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	// foreign account
	_, err := h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB})
	if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("foreign account = %v, want NotFound", err)
	}
	// deleted account
	h.accounts.deleted = true
	_, err = h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeTransactionAccountDeleted {
		t.Errorf("deleted account = %v", err)
	}
	h.accounts.deleted = false
	// currency mismatch: the card's ledger is EUR, the account is USD
	ingest(t, h, `{"account":"Euro Card","amount":"5","currency":"EUR","eventId":"e1"}`)
	_, err = h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Euro Card", AccountId: acct1})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeImportCurrencyMismatch {
		t.Errorf("currency mismatch = %v", err)
	}
	// duplicate (case-insensitive)
	if _, err := h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1}); err != nil {
		t.Fatalf("first link: %v", err)
	}
	_, err = h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "APPLE CARD", AccountId: acct1})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeImportAccountLinkExists {
		t.Errorf("duplicate = %v", err)
	}
	// blank / foreign source
	if _, err := h.svc.LinkAccount(ctx, vo.MustParseId(userB), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "X", AccountId: acctB}); err == nil {
		t.Error("foreign source must fail")
	}
}

func TestLinkAccount_NoRateStaysQueued(t *testing.T) {
	h := setup(t)
	ingest(t, h, `{"account":"GBP Card","amount":"5","currency":"GBP","eventId":"g1"}`)
	// mapping an all-GBP card onto a USD account is refused (mismatch) — so map a mixed card instead
	ingest(t, h, `{"account":"Mixed","amount":"5","currency":"GBP","eventId":"m1"}`)
	ingest(t, h, `{"account":"Mixed","amount":"6","currency":"USD","eventId":"m2"}`)
	res, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Mixed", AccountId: acct1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.ImportedCount != 1 || res.Run.SkippedCount != 1 || res.Run.Status != model.ImportRunStatusPartial {
		t.Errorf("run = %+v", res.Run)
	}
	if c := cardByID(res, "Mixed"); c.QueuedCount != 1 {
		t.Errorf("no-rate tap must stay queued: %+v", c)
	}
}

func TestIgnoreAndUnlinkAccount(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	res, err := h.svc.IgnoreAccount(ctx, uA, model.ImportAccountActionRequest{SourceId: source, ExternalAccountId: "Apple Card"})
	if err != nil || res.Run != nil || cardByID(res, "Apple Card").State != "ignored" || cardByID(res, "Apple Card").QueuedCount != 0 {
		t.Fatalf("ignore = %+v, %v", res, err)
	}
	links, _ := h.repo.ListLinksBySource(ctx, vo.MustParseId(source))
	if links[0].Status != model.ImportLinkStatusSkipped {
		t.Errorf("queued tap must become skipped on ignore: %+v", links[0])
	}
	if r := ingest(t, h, `{"account":"Apple Card","amount":"9","currency":"USD","eventId":"evt-9"}`); r.Status != model.ImportIngestStatusSkipped {
		t.Errorf("ignored card ingest = %+v", r)
	}
	// map-instead: link over an ignored card flips it to import and converts nothing (all rows are skipped now)
	linked, err := h.svc.LinkAccount(ctx, uA, model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if err != nil || cardByID(linked, "Apple Card").State != "mapped" || linked.Run.ImportedCount != 0 {
		t.Fatalf("map-instead = %+v, %v", linked, err)
	}
	// unlink forgets the mapping; queued rows and their events go, seen rows stay
	ingest(t, h, `{"account":"Fresh","amount":"1","currency":"USD","eventId":"f1"}`) // queued on another card
	un, err := h.svc.UnlinkAccount(ctx, uA, model.ImportAccountActionRequest{SourceId: source, ExternalAccountId: "Fresh"})
	if err != nil || cardByID(un, "Fresh").ExternalAccountId != "" {
		t.Fatalf("unlink must drop an unmapped card entirely: %+v, %v", un, err)
	}
	if evs, _ := h.repo.ListEventsBySourceStatus(ctx, vo.MustParseId(source), model.ImportEventStatusProcessed); len(evs) != 2 {
		t.Errorf("only the unlinked card's event may disappear (2 Apple Card events remain): %d", len(evs))
	}
	if r := ingest(t, h, `{"account":"Fresh","amount":"1","currency":"USD","eventId":"f1"}`); r.Status != model.ImportIngestStatusQueued {
		t.Errorf("a re-fired tap on a forgotten card is new again: %+v", r)
	}
	un, _ = h.svc.UnlinkAccount(ctx, uA, model.ImportAccountActionRequest{SourceId: source, ExternalAccountId: "Apple Card"})
	if c := cardByID(un, "Apple Card"); c.State != "unmapped" || c.TapCount != 2 {
		t.Errorf("unlink keeps seen rows: %+v", c)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/imports/ -run 'TestCreateSource|TestGetSourceList|TestDeleteSource|TestLinkAccount|TestIgnoreAndUnlink' 2>&1 | head -5`
Expected: build errors (`h.svc.CreateSource undefined` …).

- [ ] **Step 4: Implement sources** (`internal/imports/source.go`)

```go
package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateSource is idempotent per (user, provider): the shortcut setup can
// be re-run without leaving duplicate sources behind.
func (s *Service) CreateSource(ctx context.Context, userID vo.Id, req model.CreateImportSourceRequest) (*model.CreateImportSourceResult, error) {
	var out *model.CreateImportSourceResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.repo.GetSourceByUserProvider(ctx, userID, req.Provider)
		if err != nil {
			if _, ok := errs.AsNotFound(err); !ok {
				return err
			}
			now := s.clk.Now().UTC()
			src = &model.ImportSource{
				ID: vo.NewId(), UserID: userID, Provider: req.Provider, Name: req.Name,
				Status: model.ImportSourceStatusActive, CreatedAt: now, UpdatedAt: now,
			}
			if err := s.repo.InsertSource(ctx, src); err != nil {
				return err
			}
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.CreateImportSourceResult{Item: *item}
		return nil
	})
	return out, err
}

func (s *Service) GetSourceList(ctx context.Context, userID vo.Id) (*model.GetImportSourceListResult, error) {
	sources, err := s.repo.ListSourcesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportSourceListResult{Items: make([]model.ImportSourceResult, 0, len(sources))}
	for i := range sources {
		item, err := s.sourceResult(ctx, &sources[i])
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, *item)
	}
	return out, nil
}

// DeleteSource removes the source and, by FK cascade, its events, runs,
// links and card mappings; imported transactions stay.
func (s *Service) DeleteSource(ctx context.Context, userID vo.Id, req model.DeleteImportSourceRequest) (*model.GetImportSourceListResult, error) {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.Id)
		if err != nil {
			return err
		}
		return s.repo.DeleteSource(ctx, src.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetSourceList(ctx, userID)
}
```

- [ ] **Step 5: Implement account links** (`internal/imports/accountlink.go`)

```go
package imports

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// LinkAccount maps a card to an account and converts the card's queued taps.
func (s *Service) LinkAccount(ctx context.Context, userID vo.Id, req model.LinkImportAccountRequest) (*model.UpdateImportAccountResult, error) {
	var out *model.UpdateImportAccountResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.SourceId)
		if err != nil {
			return err
		}
		accountID, err := vo.ParseId(strings.TrimSpace(req.AccountId))
		if err != nil {
			return errs.NewNotFound("Account not found")
		}
		owner, err := s.accounts.AccountOwner(ctx, accountID)
		if err != nil {
			return err
		}
		if owner != userID {
			return errs.NewNotFound("Account not found")
		}
		deleted, err := s.accounts.AccountDeleted(ctx, accountID)
		if err != nil {
			return err
		}
		if deleted {
			return &errs.ValidationError{Msg: "Account is deleted", MsgCode: errs.CodeTransactionAccountDeleted}
		}
		ext := normalizeExternalAccountID(req.ExternalAccountId)
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		accountCode, err := s.accounts.AccountCurrencyCode(ctx, accountID)
		if err != nil {
			return err
		}
		// A card that only ever reports one currency is IN that currency; mapping it
		// onto an account in another one is a misconfiguration, not a conversion job.
		if cardCode := uniformCurrency(ledger, ext); cardCode != "" && cardCode != accountCode {
			return &errs.ValidationError{Msg: "Card currency does not match the account", MsgCode: errs.CodeImportCurrencyMismatch}
		}
		links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if existing := findAccountLink(links, ext); existing != nil {
			if existing.Mode == model.ImportAccountLinkModeImport {
				return &errs.ValidationError{Msg: "This card is already linked", MsgCode: errs.CodeImportAccountLinkExists}
			}
			existing.Mode = model.ImportAccountLinkModeImport // map-instead over an ignored card
			existing.AccountID = &accountID
			existing.ExternalCurrency = optionalString(uniformCurrency(ledger, ext))
			existing.UpdatedAt = now
			if err := s.repo.UpdateAccountLink(ctx, existing); err != nil {
				return err
			}
		} else {
			if err := s.repo.InsertAccountLink(ctx, &model.ImportAccountLink{
				ID: vo.NewId(), SourceID: src.ID, ExternalAccountID: ext, ExternalName: ext,
				ExternalCurrency: optionalString(uniformCurrency(ledger, ext)), AccountID: &accountID,
				Mode: model.ImportAccountLinkModeImport, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		run, err := s.convertQueued(ctx, src, ext, ledger)
		if err != nil {
			return err
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.UpdateImportAccountResult{Item: *item, Run: run}
		return nil
	})
	return out, err
}

// uniformCurrency is the card's currency when every ledger row agrees; ""
// for a card with no rows or mixed rows.
func uniformCurrency(ledger []model.ImportTransactionLink, ext string) string {
	code := ""
	for _, l := range ledger {
		if !strings.EqualFold(l.ExternalAccountID, ext) || l.ExternalCurrency == nil {
			continue
		}
		if code == "" {
			code = *l.ExternalCurrency
		} else if code != *l.ExternalCurrency {
			return ""
		}
	}
	return code
}

// convertQueued re-runs stages 2-3 for every queued tap on the card, now
// that it has a destination. Rows that still cannot be placed (no rate)
// stay queued and count as skipped; the run is "partial" then.
func (s *Service) convertQueued(ctx context.Context, src *model.ImportSource, ext string, ledger []model.ImportTransactionLink) (*model.ImportRunResult, error) {
	now := s.clk.Now().UTC()
	run := &model.ImportRun{
		ID: vo.NewId(), UserID: src.UserID, SourceID: src.ID, Provider: src.Provider,
		Params: `{"externalAccountId":` + strconvQuote(ext) + `}`, Status: model.ImportRunStatusRunning, StartedAt: now,
	}
	if err := s.repo.InsertRun(ctx, run); err != nil {
		return nil, err
	}
	for i := range ledger {
		l := &ledger[i]
		if l.Status != model.ImportLinkStatusQueued || !strings.EqualFold(l.ExternalAccountID, ext) {
			continue
		}
		ev, ok, err := s.reparse(ctx, src, l)
		if err != nil {
			return nil, err
		}
		if !ok {
			run.SkippedCount++
			continue
		}
		r, err := s.resolve(ctx, src, ev)
		if err != nil {
			return nil, err
		}
		if r.status != "" {
			run.SkippedCount++
			continue
		}
		txID, adopted, err := s.place(ctx, src, ev, r)
		if err != nil {
			return nil, err
		}
		l.Status = model.ImportLinkStatusLinked
		l.TransactionID = &txID
		l.RunID = &run.ID
		if err := s.repo.UpdateLink(ctx, l); err != nil {
			return nil, err
		}
		if adopted {
			run.MatchedCount++
		} else {
			run.ImportedCount++
		}
	}
	run.Status = model.ImportRunStatusCompleted
	if run.SkippedCount > 0 {
		run.Status = model.ImportRunStatusPartial
	}
	finished := s.clk.Now().UTC()
	run.FinishedAt = &finished
	if err := s.repo.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	return &model.ImportRunResult{
		Id: run.ID.String(), Status: run.Status, ImportedCount: run.ImportedCount,
		MatchedCount: run.MatchedCount, SkippedCount: run.SkippedCount, FailedCount: run.FailedCount,
	}, nil
}

// reparse rebuilds the IngestEvent behind a ledger row from its stored
// inbox event. ok=false when the event is gone or no longer parses.
func (s *Service) reparse(ctx context.Context, src *model.ImportSource, l *model.ImportTransactionLink) (model.IngestEvent, bool, error) {
	if l.EventID == nil {
		return model.IngestEvent{}, false, nil
	}
	stored, err := s.repo.GetEvent(ctx, *l.EventID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return model.IngestEvent{}, false, nil
		}
		return model.IngestEvent{}, false, err
	}
	ev, perr := s.parse(src, stored)
	if perr != nil {
		return model.IngestEvent{}, false, nil
	}
	return ev, true, nil
}

func (s *Service) IgnoreAccount(ctx context.Context, userID vo.Id, req model.ImportAccountActionRequest) (*model.UpdateImportAccountResult, error) {
	var out *model.UpdateImportAccountResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.SourceId)
		if err != nil {
			return err
		}
		ext := normalizeExternalAccountID(req.ExternalAccountId)
		links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if existing := findAccountLink(links, ext); existing != nil {
			existing.Mode = model.ImportAccountLinkModeIgnore
			existing.AccountID = nil
			existing.UpdatedAt = now
			if err := s.repo.UpdateAccountLink(ctx, existing); err != nil {
				return err
			}
		} else if err := s.repo.InsertAccountLink(ctx, &model.ImportAccountLink{
			ID: vo.NewId(), SourceID: src.ID, ExternalAccountID: ext, ExternalName: ext,
			Mode: model.ImportAccountLinkModeIgnore, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		for i := range ledger {
			if ledger[i].Status == model.ImportLinkStatusQueued && strings.EqualFold(ledger[i].ExternalAccountID, ext) {
				ledger[i].Status = model.ImportLinkStatusSkipped
				if err := s.repo.UpdateLink(ctx, &ledger[i]); err != nil {
					return err
				}
			}
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.UpdateImportAccountResult{Item: *item}
		return nil
	})
	return out, err
}

// UnlinkAccount forgets a card: its mapping, its queued taps and their inbox
// events. Seen rows (linked/skipped) stay — they are the dedupe memory.
func (s *Service) UnlinkAccount(ctx context.Context, userID vo.Id, req model.ImportAccountActionRequest) (*model.UpdateImportAccountResult, error) {
	var out *model.UpdateImportAccountResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.SourceId)
		if err != nil {
			return err
		}
		ext := normalizeExternalAccountID(req.ExternalAccountId)
		links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		if existing := findAccountLink(links, ext); existing != nil {
			if err := s.repo.DeleteAccountLink(ctx, existing.ID); err != nil {
				return err
			}
		}
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		var eventIDs []vo.Id
		for _, l := range ledger {
			if l.Status == model.ImportLinkStatusQueued && strings.EqualFold(l.ExternalAccountID, ext) && l.EventID != nil {
				eventIDs = append(eventIDs, *l.EventID)
			}
		}
		if err := s.repo.DeleteQueuedLinksByExternalAccount(ctx, src.ID, ext); err != nil {
			return err
		}
		for _, id := range eventIDs {
			if err := s.repo.DeleteEvent(ctx, id); err != nil {
				return err
			}
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.UpdateImportAccountResult{Item: *item}
		return nil
	})
	return out, err
}
```

`strconvQuote` is `strconv.Quote` — add `"strconv"` to the imports and call `strconv.Quote(ext)` directly (the alias is only to keep the snippet readable; do not define a helper).

`DeleteQueuedLinksByExternalAccount` matches `external_account_id = ?` exactly (Task 1); ledger rows store the parser-normalized card name (whitespace-collapsed, case kept) and `findAccountLink` is case-insensitive, so pass every distinct spelling: collect `seen := map[string]bool{}` over the queued rows' `ExternalAccountID` values that `strings.EqualFold` the requested card and call the purge once per spelling. Write it that way in the implementation (replace the single call above with the loop).

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/imports/ 2>&1 | tail -15`
Expected: PASS. `TestLinkAccount_NoRateStaysQueued` expects `ImportRunStatusPartial` when a tap stays queued — if `model.ImportRunStatusPartial` does not exist, check `internal/model/imports.go` for the exact run-status const names (`ImportRunStatusRunning/Completed/Failed/Partial` were defined in stage 1).

- [ ] **Step 7: vet + commit**

Run: `gofmt -l internal/imports && go vet ./internal/imports/`

```bash
git add internal/imports/source.go internal/imports/accountlink.go internal/imports/source_test.go internal/imports/accountlink_test.go
git commit -m "feat(imports): sources, card mapping and the conversion run"
```

---

### Task 5: Queue, failed events, and transaction provenance

**Files:**
- Create: `internal/imports/queue.go`
- Test: `internal/imports/queue_test.go` (package `imports_test`, same harness)

**Interfaces:**
- Consumes: Task 3 `Service` helpers (`ownedLink`, `ownedEvent`, `reparse` from Task 4, `findAccountLink`), Task 3 DTOs.
- Produces:
  ```go
  func (s *Service) GetQueue(ctx, userID vo.Id) (*model.GetImportQueueResult, error)
  func (s *Service) ImportQueuedEvent(ctx, userID vo.Id, req model.ImportQueuedEventRequest) (*model.CreateTransactionResult, error)
  func (s *Service) SkipQueuedEvent(ctx, userID vo.Id, req model.ImportLinkActionRequest) (*model.GetImportQueueResult, error)
  func (s *Service) UnskipQueuedEvent(ctx, userID vo.Id, req model.ImportLinkActionRequest) (*model.GetImportQueueResult, error)
  func (s *Service) DiscardEvent(ctx, userID vo.Id, req model.DiscardImportEventRequest) (*model.GetImportQueueResult, error)
  func (s *Service) GetTransactionImportList(ctx, userID vo.Id, req model.TransactionImportListRequest) (*model.GetTransactionImportListResult, error)
  ```
  Queue `reason` (wire): `unmapped` (no mapping for the card), `account_deleted` (mapped, account deleted/missing), `no_rate` (mapped, live, currency differs and no rate). `type` is re-derived from the stored inbox event (the ledger has no type column); `"expense"` when the event is gone.

- [ ] **Step 1: Failing tests** (`internal/imports/queue_test.go`)

```go
package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestGetQueue_ReasonsAndSections(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)                                                                                                   // unmapped
	h.mapCard(t, "GBP Card")
	ingest(t, h, `{"account":"GBP Card","amount":"5","currency":"GBP","eventId":"g1","type":"income"}`)              // mapped, no rate
	h.mapCard(t, "Dead Card")
	failed := ingest(t, h, `{"account":"Dead Card","amount":"x","currency":"USD"}`)                                    // failed
	q, err := h.svc.GetQueue(ctx, uA)
	if err != nil || len(q.Queued) != 2 || len(q.Skipped) != 0 || len(q.Failed) != 1 {
		t.Fatalf("queue = %+v, %v", q, err)
	}
	byCard := map[string]model.ImportQueuedEventResult{}
	for _, e := range q.Queued {
		byCard[e.ExternalAccountId] = e
	}
	if e := byCard["Apple Card"]; e.Reason != model.ImportQueueReasonUnmapped || e.Payee != "Blue Bottle" || e.Type != "expense" || e.Currency != "USD" || e.PostedAt != "2026-08-20 10:42:03" || e.AccountId != "" {
		t.Errorf("unmapped entry = %+v", e)
	}
	if e := byCard["GBP Card"]; e.Reason != model.ImportQueueReasonNoRate || e.Type != "income" || e.AccountId != acct1 {
		t.Errorf("no-rate entry = %+v", e)
	}
	if f := q.Failed[0]; f.EventId != failed.EventId || f.Error != "amount must be a positive number" || f.ReceivedAt == "" {
		t.Errorf("failed entry = %+v", f)
	}
	// a foreign user's queue is empty, and the slices are never null
	other, _ := h.svc.GetQueue(ctx, vo.MustParseId(userB))
	if other.Queued == nil || other.Skipped == nil || other.Failed == nil || len(other.Queued) != 0 {
		t.Errorf("foreign queue = %+v", other)
	}
	// account_deleted
	h.accounts.deleted = true
	h.mapCard(t, "Apple Card")
	q, _ = h.svc.GetQueue(ctx, uA)
	for _, e := range q.Queued {
		if e.ExternalAccountId == "Apple Card" && e.Reason != model.ImportQueueReasonAccountDeleted {
			t.Errorf("deleted-account reason = %+v", e)
		}
	}
}

func TestImportQueuedEvent(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	q, _ := h.svc.GetQueue(ctx, uA)
	linkID := q.Queued[0].LinkId
	cat := "0b000000-0000-0000-0000-0000000000c1"
	res, err := h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{LinkId: linkID, Transaction: model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: "4.75", AccountId: acct1, Date: "2026-08-20 10:42:03", CategoryId: &cat,
	}})
	if err != nil || res.Item.Id == "" {
		t.Fatalf("import = %+v, %v", res, err)
	}
	link, _ := h.repo.GetLink(ctx, vo.MustParseId(linkID))
	if link.Status != model.ImportLinkStatusLinked || link.TransactionID == nil || link.TransactionID.String() != res.Item.Id || link.AppliedCategoryID == nil || link.AppliedCategoryID.String() != cat {
		t.Errorf("link after import = %+v", link)
	}
	if q, _ := h.svc.GetQueue(ctx, uA); len(q.Queued) != 0 {
		t.Errorf("queue must drain: %+v", q.Queued)
	}
	// importing it again is refused
	_, err = h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{LinkId: linkID, Transaction: model.CreateTransactionRequest{Id: vo.NewId().String(), Type: "expense", Amount: "4.75", AccountId: acct1, Date: "2026-08-20 10:42:03"}})
	if v, ok := errs.AsValidation(err); !ok || v.MsgCode != errs.CodeImportLinkNotQueued {
		t.Errorf("re-import = %v", err)
	}
	// foreign user: not found
	if _, err := h.svc.ImportQueuedEvent(ctx, vo.MustParseId(userB), model.ImportQueuedEventRequest{LinkId: linkID, Transaction: model.CreateTransactionRequest{Id: vo.NewId().String(), Type: "expense", Amount: "1", AccountId: acctB, Date: "2026-08-20 10:42:03"}}); err == nil {
		t.Error("foreign import must fail")
	}
}

func TestSkipUnskipDiscard(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	ingest(t, h, tap)
	failed := ingest(t, h, `{"account":"Apple Card","amount":"x","currency":"USD"}`)
	q, _ := h.svc.GetQueue(ctx, uA)
	linkID := q.Queued[0].LinkId
	q, err := h.svc.SkipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID})
	if err != nil || len(q.Queued) != 0 || len(q.Skipped) != 1 {
		t.Fatalf("skip = %+v, %v", q, err)
	}
	if _, err := h.svc.SkipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID}); err == nil {
		t.Error("skipping a skipped row must fail (link_not_queued)")
	}
	q, err = h.svc.UnskipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID})
	if err != nil || len(q.Queued) != 1 || len(q.Skipped) != 0 {
		t.Fatalf("unskip = %+v, %v", q, err)
	}
	if _, err := h.svc.UnskipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID}); err == nil {
		t.Error("unskipping a queued row must fail (link_not_skipped)")
	}
	q, err = h.svc.DiscardEvent(ctx, uA, model.DiscardImportEventRequest{EventId: failed.EventId})
	if err != nil || len(q.Failed) != 0 {
		t.Fatalf("discard = %+v, %v", q, err)
	}
	if _, err := h.repo.GetEvent(ctx, vo.MustParseId(failed.EventId)); err == nil {
		t.Error("discarded event row must be deleted")
	}
	// discarding a processed event is refused: it is the dedupe memory
	ok := ingest(t, h, `{"account":"Apple Card","amount":"1","currency":"USD","eventId":"k"}`)
	if _, err := h.svc.DiscardEvent(ctx, uA, model.DiscardImportEventRequest{EventId: ok.EventId}); err == nil {
		t.Error("discard of a processed event must fail (event_not_failed)")
	}
}

func TestGetTransactionImportList(t *testing.T) {
	h := setup(t)
	uA := vo.MustParseId(userA)
	ctx := context.Background()
	h.mapCard(t, "Apple Card")
	ingest(t, h, tap)
	links, _ := h.repo.ListLinksBySource(ctx, vo.MustParseId(source))
	txID := links[0].TransactionID.String()
	res, err := h.svc.GetTransactionImportList(ctx, uA, model.TransactionImportListRequest{TransactionId: txID})
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("list = %+v, %v", res, err)
	}
	it := res.Items[0]
	if it.Provider != "apple-wallet" || it.SourceName != "iPhone" || it.ExternalAccountId != "Apple Card" || it.ExternalPayee != "Blue Bottle" || it.ExternalCurrency != "USD" || it.Status != "linked" || it.ExternalPostedAt != "2026-08-20 10:42:03" || it.ImportedAt == "" {
		t.Errorf("item = %+v", it)
	}
	if other, _ := h.svc.GetTransactionImportList(ctx, vo.MustParseId(userB), model.TransactionImportListRequest{TransactionId: txID}); len(other.Items) != 0 {
		t.Errorf("foreign user sees no provenance: %+v", other)
	}
	if empty, err := h.svc.GetTransactionImportList(ctx, uA, model.TransactionImportListRequest{TransactionId: vo.NewId().String()}); err != nil || empty.Items == nil {
		t.Errorf("unknown transaction = %+v, %v (empty list, never null)", empty, err)
	}
}
```

`Amount: "4.75"` relies on `vo.FlexString` being a string type — write `vo.FlexString("4.75")` if it is a named type without an untyped-constant conversion.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/imports/ -run 'TestGetQueue|TestImportQueued|TestSkipUnskip|TestGetTransactionImportList' 2>&1 | head -5`
Expected: build errors.

- [ ] **Step 3: Implement** (`internal/imports/queue.go`)

```go
package imports

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) GetQueue(ctx context.Context, userID vo.Id) (*model.GetImportQueueResult, error) {
	out := &model.GetImportQueueResult{
		Queued: []model.ImportQueuedEventResult{}, Skipped: []model.ImportQueuedEventResult{}, Failed: []model.ImportFailedEventResult{},
	}
	sources, err := s.repo.ListSourcesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range sources {
		src := &sources[i]
		accountLinks, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return nil, err
		}
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return nil, err
		}
		for j := range ledger {
			l := &ledger[j]
			if l.Status != model.ImportLinkStatusQueued && l.Status != model.ImportLinkStatusSkipped {
				continue
			}
			entry, err := s.queuedEntry(ctx, src, accountLinks, l)
			if err != nil {
				return nil, err
			}
			if l.Status == model.ImportLinkStatusQueued {
				out.Queued = append(out.Queued, entry)
			} else {
				out.Skipped = append(out.Skipped, entry)
			}
		}
		failed, err := s.repo.ListEventsBySourceStatus(ctx, src.ID, model.ImportEventStatusFailed)
		if err != nil {
			return nil, err
		}
		for _, ev := range failed {
			out.Failed = append(out.Failed, model.ImportFailedEventResult{
				EventId: ev.ID.String(), SourceId: src.ID.String(), ReceivedAt: ev.ReceivedAt.Format(datetime.Layout),
				Error: derefString(ev.ParseError), Payload: ev.Payload,
			})
		}
	}
	return out, nil
}

func (s *Service) queuedEntry(ctx context.Context, src *model.ImportSource, accountLinks []model.ImportAccountLink, l *model.ImportTransactionLink) (model.ImportQueuedEventResult, error) {
	entry := model.ImportQueuedEventResult{
		LinkId: l.ID.String(), SourceId: src.ID.String(), ExternalAccountId: l.ExternalAccountID,
		Payee: l.ExternalPayee, Amount: vo.NewDecimal(l.ExternalAmount).String(), Currency: derefString(l.ExternalCurrency),
		Type: model.TransactionTypeExpense.Alias(), PostedAt: l.ExternalPostedAt.Format(datetime.Layout), Reason: model.ImportQueueReasonUnmapped,
	}
	if ev, ok, err := s.reparse(ctx, src, l); err != nil {
		return entry, err
	} else if ok {
		entry.Type = ev.Type.Alias()
	}
	al := findAccountLink(accountLinks, l.ExternalAccountID)
	if al == nil || al.Mode != model.ImportAccountLinkModeImport || al.AccountID == nil {
		return entry, nil
	}
	entry.AccountId = al.AccountID.String()
	deleted, err := s.accounts.AccountDeleted(ctx, *al.AccountID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			entry.Reason = model.ImportQueueReasonAccountDeleted
			return entry, nil
		}
		return entry, err
	}
	if deleted {
		entry.Reason = model.ImportQueueReasonAccountDeleted
		return entry, nil
	}
	entry.Reason = model.ImportQueueReasonNoRate
	return entry, nil
}

// ImportQueuedEvent is the manual path out of the queue: the user reviewed
// the tap and submits the transaction they want (possibly edited). The link
// records what they chose so a later rule engine can learn from it.
func (s *Service) ImportQueuedEvent(ctx context.Context, userID vo.Id, req model.ImportQueuedEventRequest) (*model.CreateTransactionResult, error) {
	var out *model.CreateTransactionResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		link, _, err := s.ownedLink(ctx, userID, req.LinkId)
		if err != nil {
			return err
		}
		if link.Status != model.ImportLinkStatusQueued {
			return &errs.ValidationError{Msg: "This event is not queued", MsgCode: errs.CodeImportLinkNotQueued}
		}
		res, err := s.txns.CreateTransaction(ctx, userID, req.Transaction)
		if err != nil {
			return err
		}
		txID, err := vo.ParseId(res.Item.Id)
		if err != nil {
			return err
		}
		link.Status = model.ImportLinkStatusLinked
		link.TransactionID = &txID
		link.AppliedCategoryID = parseOptionalIDField(req.Transaction.CategoryId)
		link.AppliedPayeeID = parseOptionalIDField(req.Transaction.PayeeId)
		link.AppliedTagID = parseOptionalIDField(req.Transaction.TagId)
		if err := s.repo.UpdateLink(ctx, link); err != nil {
			return err
		}
		out = res
		return nil
	})
	return out, err
}

func parseOptionalIDField(raw *string) *vo.Id {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	id, err := vo.ParseId(strings.TrimSpace(*raw))
	if err != nil {
		return nil
	}
	return &id
}

func (s *Service) SkipQueuedEvent(ctx context.Context, userID vo.Id, req model.ImportLinkActionRequest) (*model.GetImportQueueResult, error) {
	return s.flipLink(ctx, userID, req.LinkId, model.ImportLinkStatusQueued, model.ImportLinkStatusSkipped, errs.CodeImportLinkNotQueued, "This event is not queued")
}

func (s *Service) UnskipQueuedEvent(ctx context.Context, userID vo.Id, req model.ImportLinkActionRequest) (*model.GetImportQueueResult, error) {
	return s.flipLink(ctx, userID, req.LinkId, model.ImportLinkStatusSkipped, model.ImportLinkStatusQueued, errs.CodeImportLinkNotSkipped, "This event is not skipped")
}

func (s *Service) flipLink(ctx context.Context, userID vo.Id, rawID, from, to, code, msg string) (*model.GetImportQueueResult, error) {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		link, _, err := s.ownedLink(ctx, userID, rawID)
		if err != nil {
			return err
		}
		if link.Status != from {
			return &errs.ValidationError{Msg: msg, MsgCode: code}
		}
		link.Status = to
		return s.repo.UpdateLink(ctx, link)
	})
	if err != nil {
		return nil, err
	}
	return s.GetQueue(ctx, userID)
}

// DiscardEvent drops a failed inbox event. Processed events are refused:
// their hash is what makes a re-fired push a duplicate.
func (s *Service) DiscardEvent(ctx context.Context, userID vo.Id, req model.DiscardImportEventRequest) (*model.GetImportQueueResult, error) {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		ev, _, err := s.ownedEvent(ctx, userID, req.EventId)
		if err != nil {
			return err
		}
		if ev.Status != model.ImportEventStatusFailed {
			return &errs.ValidationError{Msg: "This event did not fail", MsgCode: errs.CodeImportEventNotFailed}
		}
		return s.repo.DeleteEvent(ctx, ev.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetQueue(ctx, userID)
}

// GetTransactionImportList is the provenance view of one transaction: every
// ledger row pointing at it whose source the caller owns.
func (s *Service) GetTransactionImportList(ctx context.Context, userID vo.Id, req model.TransactionImportListRequest) (*model.GetTransactionImportListResult, error) {
	out := &model.GetTransactionImportListResult{Items: []model.TransactionImportLinkResult{}}
	txID, err := vo.ParseId(strings.TrimSpace(req.TransactionId))
	if err != nil {
		return out, nil
	}
	links, err := s.repo.ListLinksByTransaction(ctx, txID)
	if err != nil {
		return nil, err
	}
	sources := map[vo.Id]*model.ImportSource{}
	for _, l := range links {
		src, seen := sources[l.SourceID]
		if !seen {
			src, err = s.repo.GetSource(ctx, l.SourceID)
			if err != nil {
				if _, ok := errs.AsNotFound(err); ok {
					continue
				}
				return nil, err
			}
			sources[l.SourceID] = src
		}
		if src.UserID != userID {
			continue
		}
		out.Items = append(out.Items, model.TransactionImportLinkResult{
			Id: l.ID.String(), SourceId: src.ID.String(), Provider: src.Provider, SourceName: src.Name,
			ExternalAccountId: l.ExternalAccountID, ExternalTransactionId: l.ExternalTransactionID,
			ExternalPayee: l.ExternalPayee, ExternalAmount: vo.NewDecimal(l.ExternalAmount).String(), ExternalCurrency: derefString(l.ExternalCurrency),
			ExternalPostedAt: l.ExternalPostedAt.Format(datetime.Layout), Status: l.Status, ImportedAt: l.ImportedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run all imports tests**

Run: `go test ./internal/imports/... 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 5: vet + commit**

Run: `gofmt -l internal/imports && go vet ./internal/imports/`

```bash
git add internal/imports/queue.go internal/imports/queue_test.go
git commit -m "feat(imports): review queue, failed-event handling, transaction provenance"
```

---

### Task 6: Config, error catalogue, cross-feature glue

**Files:**
- Modify: `internal/config/config.go` (struct field + strict table entry), `internal/config/config_test.go`
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` — new `errors.import` block
- Create: `internal/server/glue_imports.go`
- Test: `internal/server/glue_imports_test.go`, `internal/config/config_test.go`, the `i18ntest` guard

**Interfaces:**
- Consumes: Task 3 ports (`imports.AccountReader`, `imports.CurrencyConverter`, `imports.TransactionLister`), Task 3 codes.
- Produces:
  ```go
  // config
  RateLimitIngest int // ECONUMO_RATE_LIMIT_INGEST, default 60, 0 disables
  // internal/server/glue_imports.go
  func NewImportsAccountReader(accounts importsAccountSource, currencies importsCurrencyByID) *ImportsAccountReader
  func NewImportsCurrencyConverter(lookup importsCurrencyCodeLookup, rates importsRateSource, convertor importsConvertor) *ImportsCurrencyConverter
  func NewImportsTransactionLister(txns importsTransactionSource) *ImportsTransactionLister
  ```

- [ ] **Step 1: Config field + default + test**

`internal/config/config.go` — after the `RateLimitConfirmEmailChange` field:

```go
	RateLimitIngest             int           // ECONUMO_RATE_LIMIT_INGEST: ingest pushes per user (every request counts)
```

and after the `{&c.RateLimitConfirmEmailChange, …, 5}` table row:

```go
		{&c.RateLimitIngest, "ECONUMO_RATE_LIMIT_INGEST", 60},
```

`internal/config/config_test.go` — extend `TestLoad_RateLimitDefaults` with

```go
	if c.RateLimitIngest != 60 {
		t.Fatalf("ingest = %d, want 60", c.RateLimitIngest)
	}
```

and add to `TestLoad_RateLimitOverridesAndDisable` a `t.Setenv("ECONUMO_RATE_LIMIT_INGEST", "0")` plus `|| c.RateLimitIngest != 0` in its assertion.

Run: `go test ./internal/config/ 2>&1 | tail -3` — expected PASS.

- [ ] **Step 2: Catalogue entries** — add an `"import"` object inside `"errors"` (after `"currency"`, keeping the existing alphabetical-ish order is not required; the guard checks key sets, not order) in EVERY catalogue:

`en`:
```json
    "import": {
      "source_not_found": "Apple Wallet is not connected. Connect it in Settings → Import & export first.",
      "provider_unsupported": "This import provider is not supported.",
      "account_link_exists": "This card is already linked.",
      "currency_mismatch": "The card's currency does not match the account's currency.",
      "link_not_queued": "This event is not waiting for review.",
      "link_not_skipped": "This event is not skipped.",
      "event_not_failed": "This event did not fail."
    },
```
`de`:
```json
    "import": {
      "source_not_found": "Apple Wallet ist nicht verbunden. Verbinde es zuerst unter Einstellungen → Daten.",
      "provider_unsupported": "Dieser Import-Anbieter wird nicht unterstützt.",
      "account_link_exists": "Diese Karte ist bereits verknüpft.",
      "currency_mismatch": "Die Währung der Karte stimmt nicht mit der Währung des Kontos überein.",
      "link_not_queued": "Dieses Ereignis wartet nicht auf Prüfung.",
      "link_not_skipped": "Dieses Ereignis ist nicht übersprungen.",
      "event_not_failed": "Dieses Ereignis ist nicht fehlgeschlagen."
    },
```
`es`:
```json
    "import": {
      "source_not_found": "Apple Wallet no está conectado. Conéctalo primero en Ajustes → Datos.",
      "provider_unsupported": "Este proveedor de importación no es compatible.",
      "account_link_exists": "Esta tarjeta ya está vinculada.",
      "currency_mismatch": "La moneda de la tarjeta no coincide con la moneda de la cuenta.",
      "link_not_queued": "Este evento no está pendiente de revisión.",
      "link_not_skipped": "Este evento no está omitido.",
      "event_not_failed": "Este evento no ha fallado."
    },
```
`fr`:
```json
    "import": {
      "source_not_found": "Apple Wallet n'est pas connecté. Connectez-le d'abord dans Paramètres → Données.",
      "provider_unsupported": "Ce fournisseur d'import n'est pas pris en charge.",
      "account_link_exists": "Cette carte est déjà liée.",
      "currency_mismatch": "La devise de la carte ne correspond pas à celle du compte.",
      "link_not_queued": "Cet événement n'est pas en attente de vérification.",
      "link_not_skipped": "Cet événement n'est pas ignoré.",
      "event_not_failed": "Cet événement n'a pas échoué."
    },
```
`it`:
```json
    "import": {
      "source_not_found": "Apple Wallet non è collegato. Collegalo prima in Impostazioni → Dati.",
      "provider_unsupported": "Questo provider di importazione non è supportato.",
      "account_link_exists": "Questa carta è già collegata.",
      "currency_mismatch": "La valuta della carta non corrisponde alla valuta del conto.",
      "link_not_queued": "Questo evento non è in attesa di revisione.",
      "link_not_skipped": "Questo evento non è stato saltato.",
      "event_not_failed": "Questo evento non è fallito."
    },
```
`nl`:
```json
    "import": {
      "source_not_found": "Apple Wallet is niet gekoppeld. Koppel het eerst via Instellingen → Gegevens.",
      "provider_unsupported": "Deze importbron wordt niet ondersteund.",
      "account_link_exists": "Deze kaart is al gekoppeld.",
      "currency_mismatch": "De valuta van de kaart komt niet overeen met de valuta van de rekening.",
      "link_not_queued": "Deze gebeurtenis wacht niet op beoordeling.",
      "link_not_skipped": "Deze gebeurtenis is niet overgeslagen.",
      "event_not_failed": "Deze gebeurtenis is niet mislukt."
    },
```
`pl`:
```json
    "import": {
      "source_not_found": "Apple Wallet nie jest połączony. Najpierw połącz go w Ustawienia → Dane.",
      "provider_unsupported": "Ten dostawca importu nie jest obsługiwany.",
      "account_link_exists": "Ta karta jest już powiązana.",
      "currency_mismatch": "Waluta karty nie zgadza się z walutą konta.",
      "link_not_queued": "To zdarzenie nie czeka na sprawdzenie.",
      "link_not_skipped": "To zdarzenie nie jest pominięte.",
      "event_not_failed": "To zdarzenie nie zakończyło się błędem."
    },
```
`pt`:
```json
    "import": {
      "source_not_found": "A Apple Wallet não está ligada. Ligue-a primeiro em Definições → Dados.",
      "provider_unsupported": "Este fornecedor de importação não é suportado.",
      "account_link_exists": "Este cartão já está associado.",
      "currency_mismatch": "A moeda do cartão não corresponde à moeda da conta.",
      "link_not_queued": "Este evento não está a aguardar revisão.",
      "link_not_skipped": "Este evento não está ignorado.",
      "event_not_failed": "Este evento não falhou."
    },
```
`ru`:
```json
    "import": {
      "source_not_found": "Apple Wallet не подключён. Сначала подключите его в Настройки → Данные.",
      "provider_unsupported": "Этот источник импорта не поддерживается.",
      "account_link_exists": "Эта карта уже привязана.",
      "currency_mismatch": "Валюта карты не совпадает с валютой счёта.",
      "link_not_queued": "Это событие не ожидает проверки.",
      "link_not_skipped": "Это событие не пропущено.",
      "event_not_failed": "Это событие не завершилось ошибкой."
    },
```
`uk`:
```json
    "import": {
      "source_not_found": "Apple Wallet не підключено. Спочатку підключіть його в Налаштування → Дані.",
      "provider_unsupported": "Це джерело імпорту не підтримується.",
      "account_link_exists": "Цю картку вже прив'язано.",
      "currency_mismatch": "Валюта картки не збігається з валютою рахунку.",
      "link_not_queued": "Ця подія не очікує на перевірку.",
      "link_not_skipped": "Цю подію не пропущено.",
      "event_not_failed": "Ця подія не завершилася помилкою."
    },
```
`zh`:
```json
    "import": {
      "source_not_found": "尚未连接 Apple Wallet。请先在 设置 → 数据 中连接。",
      "provider_unsupported": "不支持此导入来源。",
      "account_link_exists": "此卡已关联。",
      "currency_mismatch": "卡片货币与账户货币不一致。",
      "link_not_queued": "此事件不在待审核状态。",
      "link_not_skipped": "此事件未被跳过。",
      "event_not_failed": "此事件没有失败。"
    },
```

Insert each block with a small script rather than by hand (the files are ~1600 lines): for each language, `python3 - <<'EOF'` load the JSON with `object_pairs_hook=OrderedDict`, insert the `"import"` key into `data["errors"]` right after `"currency"`, and dump with `json.dump(data, f, ensure_ascii=False, indent=2)` + trailing newline. Then `git diff --stat locales/` must show ONLY additions of ~9 lines per file — if the dump reformatted anything else (indentation, escaping, key order), the file's existing formatting differs from `indent=2`; in that case insert the block textually with `sed`/an editor instead and re-diff.

Run: `go test ./internal/test/i18ntest/ 2>&1 | tail -3` — expected PASS (every code in `errs.AllCodes` has an `errors.*` entry in every language and vice versa).

- [ ] **Step 3: Failing glue tests** (`internal/server/glue_imports_test.go`)

```go
package server

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type stubAccounts struct {
	owner    vo.Id
	deleted  bool
	currency vo.Id
	err      error
}

func (s stubAccounts) AccountOwner(context.Context, vo.Id) (vo.Id, error)    { return s.owner, s.err }
func (s stubAccounts) AccountDeleted(context.Context, vo.Id) (bool, error)  { return s.deleted, s.err }
func (s stubAccounts) AccountCurrency(context.Context, vo.Id) (vo.Id, error) { return s.currency, s.err }

type stubCurrencyByID struct{ views map[string]currencyrepo.CurrencyView }

func (s stubCurrencyByID) GetByID(_ context.Context, id string) (currencyrepo.CurrencyView, error) {
	v, ok := s.views[id]
	if !ok {
		return currencyrepo.CurrencyView{}, errs.NewNotFound("Currency not found")
	}
	return v, nil
}

type stubCodeLookup struct{ ids map[string]string }

func (s stubCodeLookup) GetIDByCodeForUser(_ context.Context, _ string, code string) (string, error) {
	id, ok := s.ids[code]
	if !ok {
		return "", errs.NewNotFound("Currency not found")
	}
	return id, nil
}

type stubRates struct {
	base  vo.Id
	rates []model.FullRate
}

func (s stubRates) BaseCurrencyID(context.Context) (vo.Id, error) { return s.base, nil }
func (s stubRates) AverageRates(context.Context, time.Time, time.Time) ([]model.FullRate, error) {
	return s.rates, nil
}

type stubConvertor struct{ factor string }

func (s stubConvertor) Convert(_ context.Context, _, _ time.Time, _, _ vo.Id, sum vo.DecimalNumber) (vo.DecimalNumber, error) {
	return sum.Mul(vo.NewDecimal(s.factor)), nil
}

func TestImportsAccountReader_CurrencyCode(t *testing.T) {
	usd := vo.NewId()
	r := NewImportsAccountReader(stubAccounts{currency: usd}, stubCurrencyByID{views: map[string]currencyrepo.CurrencyView{usd.String(): {ID: usd.String(), Code: "USD"}}})
	code, err := r.AccountCurrencyCode(context.Background(), vo.NewId())
	if err != nil || code != "USD" {
		t.Fatalf("code = %q, %v", code, err)
	}
}

func TestImportsCurrencyConverter(t *testing.T) {
	usd, eur, gbp := vo.NewId(), vo.NewId(), vo.NewId()
	lookup := stubCodeLookup{ids: map[string]string{"USD": usd.String(), "EUR": eur.String(), "GBP": gbp.String()}}
	rates := stubRates{base: usd, rates: []model.FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("0.9")}}}
	c := NewImportsCurrencyConverter(lookup, rates, stubConvertor{factor: "1.10"})
	at := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	got, ok, err := c.Convert(context.Background(), vo.NewId(), "EUR", "USD", "10", at)
	if err != nil || !ok || !vo.NewDecimal(got).Equals(vo.NewDecimal("11")) {
		t.Fatalf("EUR->USD = %q, %v, %v", got, ok, err)
	}
	// the base currency needs no rate row
	if _, ok, err := c.Convert(context.Background(), vo.NewId(), "USD", "EUR", "10", at); err != nil || !ok {
		t.Errorf("USD->EUR must be ok: %v, %v", ok, err)
	}
	// GBP has no rate: the shared convertor would silently go 1:1, so the adapter must say no
	if _, ok, err := c.Convert(context.Background(), vo.NewId(), "GBP", "USD", "10", at); err != nil || ok {
		t.Errorf("GBP without a rate must be ok=false: %v, %v", ok, err)
	}
	// unknown code
	if _, ok, err := c.Convert(context.Background(), vo.NewId(), "XXX", "USD", "10", at); err != nil || ok {
		t.Errorf("unknown code must be ok=false: %v, %v", ok, err)
	}
}
```

- [ ] **Step 4: Implement the glue** (`internal/server/glue_imports.go`)

```go
// Imports glue: adapters satisfying the ports the imports feature declares
// (internal/imports/ports.go). Features never import each other (archtest);
// the composition root bridges them here.
package server

import (
	"context"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type importsAccountSource interface {
	AccountOwner(ctx context.Context, id vo.Id) (vo.Id, error)
	AccountDeleted(ctx context.Context, id vo.Id) (bool, error)
	AccountCurrency(ctx context.Context, id vo.Id) (vo.Id, error)
}

type importsCurrencyByID interface {
	GetByID(ctx context.Context, id string) (currencyrepo.CurrencyView, error)
}

// ImportsAccountReader answers the pipeline's account questions from the
// account service, translating the currency id into the ISO code the
// Wallet payload speaks.
type ImportsAccountReader struct {
	accounts   importsAccountSource
	currencies importsCurrencyByID
}

func NewImportsAccountReader(accounts importsAccountSource, currencies importsCurrencyByID) *ImportsAccountReader {
	return &ImportsAccountReader{accounts: accounts, currencies: currencies}
}

func (r *ImportsAccountReader) AccountOwner(ctx context.Context, id vo.Id) (vo.Id, error) {
	return r.accounts.AccountOwner(ctx, id)
}

func (r *ImportsAccountReader) AccountDeleted(ctx context.Context, id vo.Id) (bool, error) {
	return r.accounts.AccountDeleted(ctx, id)
}

func (r *ImportsAccountReader) AccountCurrencyCode(ctx context.Context, id vo.Id) (string, error) {
	currencyID, err := r.accounts.AccountCurrency(ctx, id)
	if err != nil {
		return "", err
	}
	view, err := r.currencies.GetByID(ctx, currencyID.String())
	if err != nil {
		return "", err
	}
	return view.Code, nil
}

var _ imports.AccountReader = (*ImportsAccountReader)(nil)

type importsCurrencyCodeLookup interface {
	GetIDByCodeForUser(ctx context.Context, userID, code string) (string, error)
}

type importsRateSource interface {
	BaseCurrencyID(ctx context.Context) (vo.Id, error)
	AverageRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error)
}

type importsConvertor interface {
	Convert(ctx context.Context, periodStart, periodEnd time.Time, from, to vo.Id, sum vo.DecimalNumber) (vo.DecimalNumber, error)
}

// ImportsCurrencyConverter wraps the shared convertor with the presence
// check it lacks: the convertor treats a missing rate as 1:1, which is fine
// for a budget total but would import a foreign tap at the wrong amount.
type ImportsCurrencyConverter struct {
	lookup    importsCurrencyCodeLookup
	rates     importsRateSource
	convertor importsConvertor
}

func NewImportsCurrencyConverter(lookup importsCurrencyCodeLookup, rates importsRateSource, convertor importsConvertor) *ImportsCurrencyConverter {
	return &ImportsCurrencyConverter{lookup: lookup, rates: rates, convertor: convertor}
}

func (c *ImportsCurrencyConverter) Convert(ctx context.Context, userID vo.Id, from, to, amount string, at time.Time) (string, bool, error) {
	fromID, ok, err := c.resolve(ctx, userID, from)
	if err != nil || !ok {
		return "", false, err
	}
	toID, ok, err := c.resolve(ctx, userID, to)
	if err != nil || !ok {
		return "", false, err
	}
	if fromID.Equal(toID) {
		return vo.NewDecimal(amount).String(), true, nil
	}
	start := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	baseID, err := c.rates.BaseCurrencyID(ctx)
	if err != nil {
		return "", false, err
	}
	rates, err := c.rates.AverageRates(ctx, start, end)
	if err != nil {
		return "", false, err
	}
	for _, id := range []vo.Id{fromID, toID} {
		if !id.Equal(baseID) && !hasRate(rates, id) {
			return "", false, nil
		}
	}
	converted, err := c.convertor.Convert(ctx, start, end, fromID, toID, vo.NewDecimal(amount))
	if err != nil {
		return "", false, err
	}
	return converted.String(), true, nil
}

func (c *ImportsCurrencyConverter) resolve(ctx context.Context, userID vo.Id, code string) (vo.Id, bool, error) {
	raw, err := c.lookup.GetIDByCodeForUser(ctx, userID.String(), code)
	if err != nil {
		if _, nf := errs.AsNotFound(err); nf {
			return vo.Id{}, false, nil
		}
		return vo.Id{}, false, err
	}
	id, err := vo.ParseId(raw)
	if err != nil {
		return vo.Id{}, false, err
	}
	return id, true, nil
}

func hasRate(rates []model.FullRate, id vo.Id) bool {
	for _, r := range rates {
		if r.CurrencyID.Equal(id) {
			return true
		}
	}
	return false
}

var _ imports.CurrencyConverter = (*ImportsCurrencyConverter)(nil)

type importsTransactionSource interface {
	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, filter model.TransactionFilter) ([]*model.Transaction, error)
}

// ImportsTransactionLister narrows the transaction repo's account-list read
// to the matcher's window.
type ImportsTransactionLister struct{ txns importsTransactionSource }

func NewImportsTransactionLister(txns importsTransactionSource) *ImportsTransactionLister {
	return &ImportsTransactionLister{txns: txns}
}

func (l *ImportsTransactionLister) ListByAccount(ctx context.Context, accountID vo.Id, from, to time.Time) ([]*model.Transaction, error) {
	return l.txns.ListByAccountIDs(ctx, []vo.Id{accountID}, model.TransactionFilter{PeriodStart: from, PeriodEnd: to})
}

var _ imports.TransactionLister = (*ImportsTransactionLister)(nil)
```

Check `model.TransactionFilter`'s period semantics in `internal/transaction/repo/repo.go:266` (`ListByAccountIDs`): whether `PeriodEnd` is inclusive or exclusive and whether a zero `PeriodStart` means "unbounded". The pipeline passes `[at-window, at+window+1)`; if the filter is inclusive on both ends that is one day of slack, which is harmless. If `vo.Id` has no `Equal` method, use `==` (it is a comparable struct — `internal/shared/vo/id.go`).

- [ ] **Step 5: Run**

Run: `go test ./internal/server/ -run 'TestImports' 2>&1 | tail -5 && go vet ./internal/server/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go locales/ internal/server/glue_imports.go internal/server/glue_imports_test.go
git commit -m "feat(imports): ingest rate limit, import error catalogue, account/currency/transaction glue"
```

---

### Task 7: HTTP edge, server wiring, served matcher config, apiparity

**Files:**
- Create: `internal/imports/api/handler.go`, `internal/imports/api/source.go`, `internal/imports/api/accountlink.go`, `internal/imports/api/ingest.go`, `internal/imports/api/queue.go`, `internal/imports/api/routes.go`
- Create: `internal/imports/api/harness_test.go`, `internal/imports/api/ingest_test.go`
- Modify: `internal/web/router/router.go:155` (the `overrides` map), `internal/web/router/router_test.go:201-226`
- Modify: `internal/server/server.go` (imports block after `transactionSvc`; `authLimiter` limits; `router.Compose`)
- Modify: `internal/test/fixture/imports.go` (`ImportTransactionLink` gains `EventID` + `ExternalCurrency`)
- Modify: `internal/test/apiparity/fixture.go` (new consts + seed rows), `internal/test/apiparity/harness.go:85-105` (cfg)
- Create: `internal/test/apiparity/catalogue_import.go`
- Regenerate: `internal/test/apiparity/testdata/golden/*.golden`, `internal/web/apidoc/swagger.{json,yaml}` (via `make swagger`)

**Interfaces:**
- Consumes (Task 3–6): `imports.NewService(repo, accounts, converter, txns, lister, limiter, tx, clk, cfg)`, `imports.RateScopeIngest`, every `Service` method listed in the routes table below, `server.NewImportsAccountReader(accounts, currencies)`, `server.NewImportsCurrencyConverter(lookup, rates, convertor)`, `server.NewImportsTransactionLister(txns)`, `config.Config.RateLimitIngest`, `config.Config.Import{MatchDays,TipDays,TipTolerancePct,TokenMinLength}`.
- Produces: the 14 routes under `/api/v1/import/` (table below), the served `IMPORT_MATCHER` config key (`{"matchDays","tipDays","tipTolerancePct","tokenMinLength"}` ints), and `fixture.ImportTransactionLink{EventID, ExternalCurrency string}`.

Route table (all under `/api/v1/import/`; every authenticated route documents `@Failure 401` + `500`, every POST also `402`):

| Method | Path | Handler | Service call |
|---|---|---|---|
| POST | `create-source` | `CreateSource` | `svc.CreateSource` |
| GET | `get-source-list` | `GetSourceList` | `svc.GetSourceList` |
| POST | `delete-source` | `DeleteSource` | `svc.DeleteSource` |
| POST | `link-account` | `LinkAccount` | `svc.LinkAccount` |
| POST | `ignore-account` | `IgnoreAccount` | `svc.IgnoreAccount` |
| POST | `unlink-account` | `UnlinkAccount` | `svc.UnlinkAccount` |
| POST | `ingest-apple-wallet-event` | `IngestAppleWalletEvent` (hand-written, raw body) | `svc.IngestAppleWallet` |
| GET | `get-queued-event-list` | `GetQueuedEventList` | `svc.GetQueue` |
| POST | `import-queued-event` | `ImportQueuedEvent` | `svc.ImportQueuedEvent` |
| POST | `skip-queued-event` | `SkipQueuedEvent` | `svc.SkipQueuedEvent` |
| POST | `unskip-queued-event` | `UnskipQueuedEvent` | `svc.UnskipQueuedEvent` |
| POST | `retry-event` | `RetryEvent` | `svc.RetryEvent` |
| POST | `discard-event` | `DiscardEvent` | `svc.DiscardEvent` |
| GET | `get-transaction-import-list?transactionId=` | `GetTransactionImportList` (hand-written, query param) | `svc.GetTransactionImportList` |

The ingest route is the ONLY path an ingest-scoped PAT may reach (`middleware.IngestPathPrefix = "/api/v1/import/ingest-"`, enforced already by the auth middleware from stage 1). It rides the normal authenticated pipeline, so the readonly-402 check, the rate limiter (inside the service) and the access log all apply — but the body is never logged: the handler only adds `ingest_status` to the operation line.

- [ ] **Step 1: Handler scaffolding** (`internal/imports/api/handler.go`)

```go
// Package api wires the imports module's HTTP edge.
package api

import (
	appimports "github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appimports.Service
}

func NewHandlers(svc *appimports.Service) *Handlers {
	return &Handlers{svc: svc}
}
```

- [ ] **Step 2: Source handlers** (`internal/imports/api/source.go`)

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.CreateImportSourceResult{}

// CreateSource handles POST /api/v1/import/create-source (auth).
//
// @Summary     Connect an import source
// @Description Creates an import source (currently only the "apple-wallet" provider) for the authenticated user. One source per provider per user.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateImportSourceRequest true "Create source request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateImportSourceResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/create-source [post]
func (h *Handlers) CreateSource(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateSource)
}

// GetSourceList handles GET /api/v1/import/get-source-list (auth).
//
// @Summary     List import sources
// @Description Returns the caller's import sources with their cards (external accounts seen so far) and each card's mapping state.
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportSourceListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-source-list [get]
func (h *Handlers) GetSourceList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetSourceList)
}

// DeleteSource handles POST /api/v1/import/delete-source (auth).
//
// @Summary     Disconnect an import source
// @Description Deletes a source and cascades its events, runs, account links and ledger rows. Imported transactions stay (they become plain transactions).
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteImportSourceRequest true "Delete source request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportSourceListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/delete-source [post]
func (h *Handlers) DeleteSource(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteSource)
}
```

- [ ] **Step 3: Account-link handlers** (`internal/imports/api/accountlink.go`)

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// LinkAccount handles POST /api/v1/import/link-account (auth).
//
// @Summary     Map a card to an account
// @Description Maps an external account (card) of a source onto one of the caller's accounts and converts every queued event of that card in one run. Refuses a card whose reported currency differs from the account's.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.LinkImportAccountRequest true "Link account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateImportAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/link-account [post]
func (h *Handlers) LinkAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.LinkAccount)
}

// IgnoreAccount handles POST /api/v1/import/ignore-account (auth).
//
// @Summary     Ignore a card
// @Description Marks a card ignored: its queued events are dropped and future events for it are skipped on arrival.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportAccountActionRequest true "Ignore account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateImportAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/ignore-account [post]
func (h *Handlers) IgnoreAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.IgnoreAccount)
}

// UnlinkAccount handles POST /api/v1/import/unlink-account (auth).
//
// @Summary     Unmap a card
// @Description Removes a card's mapping (or its ignore flag). Already-imported transactions are untouched; new events for the card queue again.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportAccountActionRequest true "Unlink account request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateImportAccountResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/unlink-account [post]
func (h *Handlers) UnlinkAccount(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UnlinkAccount)
}
```

- [ ] **Step 4: Ingest handler** (`internal/imports/api/ingest.go`)

The body is the raw shortcut payload — it is NOT decoded here (the service stores it verbatim and parses it inside the transaction), so the generic combinator does not fit. The size cap keeps a misconfigured shortcut from streaming megabytes into `import_events.payload`.

```go
package api

import (
	"io"
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// maxIngestBody caps one shortcut push; a real Apple Wallet event is < 1 KB.
const maxIngestBody = 64 << 10

// IngestAppleWalletEvent handles POST /api/v1/import/ingest-apple-wallet-event (auth; ingest- or full-scope token).
//
// @Summary     Push an Apple Wallet transaction event
// @Description Accepts one raw Apple Wallet event as pushed by the "Econumo Wallet" shortcut, stores it, and processes it synchronously: created (a transaction was written), duplicate (already seen), queued (card unmapped / no rate / account deleted), skipped (card ignored) or failed (unparsable payload). The body is never logged.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     object true "Raw Apple Wallet event: {account, payee, amount, currency, occurredAt?, type?, eventId?}"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.IngestEventResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/ingest-apple-wallet-event [post]
func (h *Handlers) IngestAppleWalletEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxIngestBody))
	if err != nil {
		httpx.WriteError(ctx, w, errs.NewValidation("Request body is too large"))
		return
	}
	res, err := h.svc.IngestAppleWallet(ctx, userID, body)
	if err != nil {
		httpx.WriteError(ctx, w, err)
		return
	}
	reqctx.AddLogAttr(ctx, "ingest_status", res.Status)
	httpx.OK(w, res)
}
```

If `errs.NewValidation` takes a code-less message in this codebase, keep it exactly as written; if it needs a code, use `&errs.ValidationError{Msg: "Request body is too large"}`.

- [ ] **Step 5: Queue + provenance handlers** (`internal/imports/api/queue.go`)

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetQueuedEventList handles GET /api/v1/import/get-queued-event-list (auth).
//
// @Summary     Get the import queue
// @Description Returns the caller's queued events (waiting for a card mapping or a manual import), skipped events, and failed events (unparsable payloads awaiting retry/discard).
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-queued-event-list [get]
func (h *Handlers) GetQueuedEventList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetQueue)
}

// ImportQueuedEvent handles POST /api/v1/import/import-queued-event (auth).
//
// @Summary     Import one queued event by hand
// @Description Creates the given transaction (same body as create-transaction, prefilled by the SPA from the queued event) and links the queued event to it.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportQueuedEventRequest true "Import queued event request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/import-queued-event [post]
func (h *Handlers) ImportQueuedEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ImportQueuedEvent)
}

// SkipQueuedEvent handles POST /api/v1/import/skip-queued-event (auth).
//
// @Summary     Skip a queued event
// @Description Moves a queued event to the skipped list; a later push of the same event stays skipped.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportLinkActionRequest true "Skip request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/skip-queued-event [post]
func (h *Handlers) SkipQueuedEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SkipQueuedEvent)
}

// UnskipQueuedEvent handles POST /api/v1/import/unskip-queued-event (auth).
//
// @Summary     Un-skip an event
// @Description Moves a skipped event back to the queue.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ImportLinkActionRequest true "Un-skip request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/unskip-queued-event [post]
func (h *Handlers) UnskipQueuedEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UnskipQueuedEvent)
}

// RetryEvent handles POST /api/v1/import/retry-event (auth).
//
// @Summary     Retry a failed event
// @Description Re-runs the pipeline on a failed event's stored payload (after a normalizer fix, for instance). Returns the new ingest status.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.RetryImportEventRequest true "Retry request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.IngestEventResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/retry-event [post]
func (h *Handlers) RetryEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.RetryEvent)
}

// DiscardEvent handles POST /api/v1/import/discard-event (auth).
//
// @Summary     Discard a failed event
// @Description Deletes a failed event and its payload for good.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.DiscardImportEventRequest true "Discard request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportQueueResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/discard-event [post]
func (h *Handlers) DiscardEvent(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DiscardEvent)
}

// GetTransactionImportList handles GET /api/v1/import/get-transaction-import-list (auth).
// Required query param: transactionId.
//
// @Summary     Get a transaction's import provenance
// @Description Returns the import ledger rows linked to one of the caller's visible transactions (source, external payee/amount/currency, posted and imported timestamps).
// @Tags        Import
// @Produce     json
// @Param       transactionId query    string true "Transaction id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetTransactionImportListResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-transaction-import-list [get]
func (h *Handlers) GetTransactionImportList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	req := model.TransactionImportListRequest{TransactionId: r.URL.Query().Get("transactionId")}
	if err := req.Validate(); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GetTransactionImportList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
```

- [ ] **Step 6: Routes** (`internal/imports/api/routes.go`)

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/import/create-source", auth(h.CreateSource))
		mux.Handle("GET /api/v1/import/get-source-list", auth(h.GetSourceList))
		mux.Handle("POST /api/v1/import/delete-source", auth(h.DeleteSource))
		mux.Handle("POST /api/v1/import/link-account", auth(h.LinkAccount))
		mux.Handle("POST /api/v1/import/ignore-account", auth(h.IgnoreAccount))
		mux.Handle("POST /api/v1/import/unlink-account", auth(h.UnlinkAccount))
		// The only route an ingest-scoped PAT may call (middleware.IngestPathPrefix).
		mux.Handle("POST /api/v1/import/ingest-apple-wallet-event", auth(h.IngestAppleWalletEvent))
		mux.Handle("GET /api/v1/import/get-queued-event-list", auth(h.GetQueuedEventList))
		mux.Handle("POST /api/v1/import/import-queued-event", auth(h.ImportQueuedEvent))
		mux.Handle("POST /api/v1/import/skip-queued-event", auth(h.SkipQueuedEvent))
		mux.Handle("POST /api/v1/import/unskip-queued-event", auth(h.UnskipQueuedEvent))
		mux.Handle("POST /api/v1/import/retry-event", auth(h.RetryEvent))
		mux.Handle("POST /api/v1/import/discard-event", auth(h.DiscardEvent))
		mux.Handle("GET /api/v1/import/get-transaction-import-list", auth(h.GetTransactionImportList))
	}
}
```

Run: `go build ./... && go vet ./internal/imports/...` — Expected: clean.

- [ ] **Step 7: Body-never-logged regression test** (`internal/imports/api/harness_test.go` + `internal/imports/api/ingest_test.go`)

The harness mounts the real routes behind the real `RequestID` + `AccessLog` middleware with `authstub` (bearer token = user id) and fake ports, so the test exercises exactly the log line production emits.

```go
// internal/imports/api/harness_test.go
package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appimports "github.com/econumo/econumo/internal/imports"
	handlerimports "github.com/econumo/econumo/internal/imports/api"
	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/web/middleware"
)

const (
	usdID  = "dffc2a06-6f29-4704-8575-31709adee926"
	userA  = "0a000000-0000-0000-0000-000000000001"
	acct1  = "0a000000-0000-0000-0000-0000000000a1"
	source = "0c000000-0000-0000-0000-000000000001"
)

var now = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

type clock struct{ t time.Time }

func (c clock) Now() time.Time { return c.t }

type fakeAccounts struct{}

func (fakeAccounts) AccountOwner(_ context.Context, id vo.Id) (vo.Id, error) {
	if id.String() == acct1 {
		return vo.MustParseId(userA), nil
	}
	return vo.Id{}, errs.NewNotFound("Account not found")
}
func (fakeAccounts) AccountDeleted(context.Context, vo.Id) (bool, error)         { return false, nil }
func (fakeAccounts) AccountCurrencyCode(context.Context, vo.Id) (string, error) { return "USD", nil }

type fakeConverter struct{}

func (fakeConverter) Convert(_ context.Context, _ vo.Id, from, to, amount string, _ time.Time) (string, bool, error) {
	if from == to {
		return amount, true, nil
	}
	return "", false, nil
}

type fakeTxns struct{ created int }

func (f *fakeTxns) CreateTransaction(_ context.Context, _ vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error) {
	f.created++
	return &model.CreateTransactionResult{Item: model.TransactionResult{Id: req.Id, AccountId: req.AccountId, Amount: string(req.Amount)}}, nil
}
func (f *fakeTxns) ListByAccount(context.Context, vo.Id, time.Time, time.Time) ([]*model.Transaction, error) {
	return nil, nil
}

type harness struct {
	srv  *httptest.Server
	txns *fakeTxns
	f    *fixture.Builder
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db).At(now)
	f.User(fixture.User{ID: userA, Email: "a@example.test", Name: "A"})
	f.Account(fixture.Account{ID: acct1, UserID: userA, CurrencyID: usdID, Name: "Card"})
	f.ImportSource(fixture.ImportSource{ID: source, UserID: userA, Name: "iPhone"})
	txns := &fakeTxns{}
	svc := appimports.NewService(importsrepo.NewRepo(db.Engine, db.TX), fakeAccounts{}, fakeConverter{}, txns, txns, nil, db.TX, clock{now}, appimports.DefaultMatcherConfig())

	mux := http.NewServeMux()
	handlerimports.RegisterAPI(handlerimports.NewHandlers(svc), authstub.Authenticator{})(mux)
	srv := httptest.NewServer(middleware.Chain(middleware.RequestID, middleware.AccessLog)(mux))
	t.Cleanup(srv.Close)
	return &harness{srv: srv, txns: txns, f: f}
}
```

If `middleware.Chain` composes in a different order than `RequestID` outermost, check its doc comment and order the arguments so `RequestID` wraps `AccessLog`.

```go
// internal/imports/api/ingest_test.go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

func TestIngest_BodyIsNeverLogged(t *testing.T) {
	h := newHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", AccountID: acct1})

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	const secretPayee = "Dr. Very Private Clinic"
	body := `{"account":"Apple Card","payee":"` + secretPayee + `","amount":"120.00","currency":"USD","eventId":"evt-secret"}`
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, h.srv.URL+"/api/v1/import/ingest-apple-wallet-event", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userA)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.StatusCode != 200 || !env.Success || env.Data.Status != "created" {
		t.Fatalf("status=%d success=%v ingest=%q", res.StatusCode, env.Success, env.Data.Status)
	}
	if h.txns.created != 1 {
		t.Fatalf("created=%d, want 1", h.txns.created)
	}

	out := logs.String()
	if strings.Contains(out, secretPayee) || strings.Contains(out, "evt-secret") || strings.Contains(out, "120.00") {
		t.Fatalf("payload leaked into logs:\n%s", out)
	}
	if !strings.Contains(out, "ingest_status=created") {
		t.Fatalf("operation line missing ingest_status:\n%s", out)
	}
}

func TestIngest_BodyTooLarge(t *testing.T) {
	h := newHarness(t)
	body := `{"account":"Apple Card","payee":"` + strings.Repeat("x", 70<<10) + `","amount":"1","currency":"USD"}`
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, h.srv.URL+"/api/v1/import/ingest-apple-wallet-event", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userA)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status=%d, want 400", res.StatusCode)
	}
}
```

Run: `go test ./internal/imports/api/ -run 'TestIngest' -v` — Expected: PASS (both).

- [ ] **Step 8: Served matcher config** (`internal/web/router/router.go`, `router_test.go`)

The SPA needs the matcher thresholds to group cross-provider duplicates client-side (spec: "the matcher config is served to the SPA"). Stage 2 has one provider so the SPA only types the key; the value is served now so stage 3 needs no server change.

In `router.go`, add to the `overrides` map literal (always merged, next to `ANALYTICS`/`ALLOW_REGISTRATION`/`BILLING_URL`):

```go
		"IMPORT_MATCHER": map[string]int{
			"matchDays":       deps.Cfg.ImportMatchDays,
			"tipDays":         deps.Cfg.ImportTipDays,
			"tipTolerancePct": deps.Cfg.ImportTipTolerancePct,
			"tokenMinLength":  deps.Cfg.ImportTokenMinLength,
		},
```

In `router_test.go` `TestRuntimeConfigOverrides` (line ~201-226), the asserted `Object.assign` string gains the key in sorted position, i.e. between `"BILLING_URL":"https://pay.example.test/cloud/",` and `"MIN_APP_VERSION":"v9.9.9"`:

```
"IMPORT_MATCHER":{"matchDays":0,"tipDays":0,"tipTolerancePct":0,"tokenMinLength":0},
```

(the test's config leaves the import fields at their zero values — do not set them there; a second assertion is not needed because `config_test.go` already covers the defaults).

Run: `go test ./internal/web/router/` — Expected: PASS.

- [ ] **Step 9: Server wiring** (`internal/server/server.go`)

Imports (alphabetical among the existing aliases):

```go
	appimports "github.com/econumo/econumo/internal/imports"
	handlerimports "github.com/econumo/econumo/internal/imports/api"
	importsrepo "github.com/econumo/econumo/internal/imports/repo"
```

In the `authLimiter` config's `Limits` map (after `appconnection.RateScopeAcceptInvite: cfg.RateLimitAccept,`):

```go
			appimports.RateScopeIngest:          cfg.RateLimitIngest,
```

After `transactionSvc` is built (and `transactionRepo` exists — both do at ~line 303-324):

```go
	importsRepo := importsrepo.NewRepo(cfg.DatabaseDriver, txm)
	importsSvc := appimports.NewService(
		importsRepo,
		NewImportsAccountReader(accountSvc, currencyLookup),
		NewImportsCurrencyConverter(currencyLookup, rateProvider, convertor),
		transactionSvc,
		NewImportsTransactionLister(transactionRepo),
		authLimiter, txm, clk,
		appimports.MatcherConfig{
			MatchDays:       cfg.ImportMatchDays,
			TipDays:         cfg.ImportTipDays,
			TipTolerancePct: cfg.ImportTipTolerancePct,
			TokenMinLength:  cfg.ImportTokenMinLength,
		},
	)
	importsHandlers := handlerimports.NewHandlers(importsSvc)
```

In `router.Compose(...)`, before `handlersystem.RegisterAPI(systemHandlers, authn),`:

```go
		handlerimports.RegisterAPI(importsHandlers, authn),
```

`transactionSvc` must satisfy `imports.TransactionCreator` (`CreateTransaction(ctx, userID, model.CreateTransactionRequest) (*model.CreateTransactionResult, error)`) — check `internal/transaction/create.go`; if the method has that exact shape (it is what `endpoint.Handle` is given in `internal/transaction/api`), no adapter is needed. If its signature differs, add a one-method `ImportsTransactionCreator` adapter in `glue_imports.go` following the same `New…` + `var _` pattern.

Run: `go build ./... && go test ./internal/server/ ./internal/test/archtest/` — Expected: PASS (archtest: `imports/api` imports only `imports`, `model`, `shared`, `web`).

- [ ] **Step 10: Regenerate OpenAPI docs**

Run: `make swagger && go test ./internal/test/apiparity/ -run 'TestGuard_EveryAuthenticatedRouteDocuments401And500|TestGuard_EveryRestrictedPostDocuments402'` — Expected: PASS. `git status` shows `internal/web/apidoc/swagger.json` + `swagger.yaml` changed with the 14 new paths.

- [ ] **Step 11: Fixture builder additions** (`internal/test/fixture/imports.go`)

Add two fields to `ImportTransactionLink` and write them:

```go
type ImportTransactionLink struct {
	ID                    string
	SourceID              string
	EventID               string // "" -> NULL
	ExternalAccountID     string
	ExternalTransactionID string
	TransactionID         string
	Status                string
	ExternalPayee         string
	ExternalDescription   string
	ExternalAmount        string // decimal text, e.g. "12.50000000"
	ExternalCurrency      string // "" -> NULL
	ExternalPostedAt      time.Time
}
```

and change the INSERT to bind them: `event_id` gets `nullable(l.EventID)` (replacing the literal `NULL` after `run_id`), `external_currency` gets `nullable(l.ExternalCurrency)` (replacing the literal `NULL` after `external_amount`). Keep the column order of the statement; only the two placeholders change from `NULL` to `?`:

```go
	b.insert(`INSERT INTO import_transaction_links (id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at)
		VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, ?)`,
		id, l.SourceID, nullable(l.EventID), l.ExternalAccountID, l.ExternalTransactionID, nullable(l.TransactionID), status,
		l.ExternalPayee, l.ExternalDescription, l.ExternalAmount, nullable(l.ExternalCurrency), posted, b.now())
```

Run: `go test ./internal/imports/... ./internal/test/apiparity/ -count=1` — Expected: PASS (existing callers pass zero values → NULL, unchanged).

- [ ] **Step 12: apiparity seed** (`internal/test/apiparity/fixture.go`)

Add consts next to `ImportLinkTxn2`:

```go
	ImportEventQueued = "0e000000-0000-0000-0000-000000000001" // processed event behind ImportLinkQueued
	ImportLinkQueued  = "0d000000-0000-0000-0000-000000000002" // "wallet" tap-2, queued (card unmapped)
	ImportEventFailed = "0e000000-0000-0000-0000-000000000002" // unparsable payload awaiting retry/discard
	ImportLinkEuro    = "0d000000-0000-0000-0000-000000000003" // "eurocard" tap-9, queued, EUR
```

Append to `Seed` after the existing `f.ImportTransactionLink(...)` call:

```go
	// A queued tap on the same "wallet" card: 12.50 at "Shop" received at
	// ClockTime, i.e. the same day + amount as Txn1, so link-account's
	// conversion run ADOPTS Txn1 (matchedCount 1) rather than creating a row.
	f.ImportEvent(fixture.ImportEvent{ID: ImportEventQueued, SourceID: ImportSourcePhone,
		Payload: `{"account":"wallet","payee":"Shop","amount":"12.50","currency":"USD","eventId":"tap-2"}`,
		ReceivedAt: ClockTime})
	f.ImportTransactionLink(fixture.ImportTransactionLink{ID: ImportLinkQueued, SourceID: ImportSourcePhone, EventID: ImportEventQueued,
		ExternalAccountID: "wallet", ExternalTransactionID: "tap-2", Status: model.ImportLinkStatusQueued,
		ExternalPayee: "Shop", ExternalAmount: "12.50000000", ExternalCurrency: "USD", ExternalPostedAt: ClockTime})
	// A card that only ever reported EUR: mapping it onto the USD OwnerAccount
	// is the currency-mismatch refusal.
	f.ImportTransactionLink(fixture.ImportTransactionLink{ID: ImportLinkEuro, SourceID: ImportSourcePhone,
		ExternalAccountID: "eurocard", ExternalTransactionID: "tap-9", Status: model.ImportLinkStatusQueued,
		ExternalPayee: "Bakery", ExternalAmount: "3.00000000", ExternalCurrency: "EUR", ExternalPostedAt: ClockTime})
	// A failed event (unparsable amount) for the retry/discard scenarios.
	f.ImportEvent(fixture.ImportEvent{ID: ImportEventFailed, SourceID: ImportSourcePhone,
		Payload: `{"account":"wallet","payee":"Broken","amount":"lots","currency":"USD"}`,
		Status: model.ImportEventStatusFailed, ParseError: "amount must be a positive number", ReceivedAt: ClockTime})
```

`ImportEvent` with `Status: ""` defaults to `processed` (Task 1 builder), which is right for the queued one.

- [ ] **Step 13: apiparity harness config** (`internal/test/apiparity/harness.go:85-105`)

Add to the `config.Config{...}` literal, after `RateLimitGlobal: 60,`:

```go
		RateLimitIngest: 60,
		// Production matcher defaults so the seeded queued tap adopts Txn1 by
		// the same rules a real instance applies.
		ImportMatchDays:       3,
		ImportTipDays:         5,
		ImportTipTolerancePct: 20,
		ImportTokenMinLength:  3,
```

- [ ] **Step 14: Scenario catalogue** (`internal/test/apiparity/catalogue_import.go`)

```go
package apiparity

// Apple Wallet import: source lifecycle, card mapping with its conversion
// run, the ingest route under both token scopes, queue triage, and the
// provenance read. Every scenario starts from the fresh seed (ImportSourcePhone
// owned by Owner with one linked tap on Txn2, one queued tap on the "wallet"
// card, one queued EUR tap on "eurocard", one failed event).
func init() {
	register(Scenario{Name: "import_sources", Calls: func() []Call {
		return []Call{
			{Label: "get-source-list", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "owner"},
			// Owner already has an apple-wallet source: one per provider per user.
			{Label: "err:create-source-duplicate", Method: "POST", Path: "/api/v1/import/create-source", Auth: "owner",
				Body: map[string]any{"provider": "apple-wallet", "name": "Second phone"}},
			{Label: "err:create-source-unknown-provider", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "Bank"}},
			{Label: "create-source", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "apple-wallet", "name": "Guest phone"}},
			{Label: "get-source-list-guest", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "guest"},
			// Guest may not delete the owner's source (looked up by owner -> coded 400, no existence leak).
			{Label: "err:delete-source-foreign", Method: "POST", Path: "/api/v1/import/delete-source", Auth: "guest",
				Body: map[string]any{"id": ImportSourcePhone}},
			{Label: "delete-source", Method: "POST", Path: "/api/v1/import/delete-source", Auth: "owner",
				Body: map[string]any{"id": ImportSourcePhone}},
			// The cascade dropped the ledger row behind Txn2, so it reads as hand-entered now.
			{Label: "get-transaction-list-after-delete", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})

	register(Scenario{Name: "import_account_links", Calls: func() []Call {
		return []Call{
			{Label: "err:link-account-currency-mismatch", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "eurocard", "accountId": OwnerAccount}},
			// Guest-owned SharedAccount is shared with Owner but not OWNED by Owner: not mappable.
			{Label: "err:link-account-not-owned", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": SharedAccount}},
			// Mapping "wallet" converts its queued tap: same day + amount as Txn1 -> adopted (run.matchedCount 1).
			{Label: "link-account", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": OwnerAccount}},
			{Label: "err:link-account-again", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": OwnerAccount}},
			{Label: "get-queued-event-list-after-link", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
			{Label: "get-transaction-import-list-adopted", Method: "GET", Path: "/api/v1/import/get-transaction-import-list?transactionId=" + Txn1, Auth: "owner"},
			{Label: "ignore-account", Method: "POST", Path: "/api/v1/import/ignore-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "eurocard"}},
			// "map instead" over an ignored card is allowed; currency still has to agree, so it stays refused for eurocard.
			{Label: "err:link-ignored-card-currency-mismatch", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "eurocard", "accountId": OwnerAccount}},
			{Label: "unlink-account", Method: "POST", Path: "/api/v1/import/unlink-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet"}},
			{Label: "err:unlink-account-unknown-card", Method: "POST", Path: "/api/v1/import/unlink-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "nope"}},
			{Label: "get-source-list-after-unlink", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "owner"},
		}
	}})

	register(Scenario{Name: "import_ingest", Calls: func() []Call {
		return []Call{
			// Unmapped card -> queued, under the ingest-scoped PAT.
			{Label: "ingest-queued", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`{"account":"wallet","payee":"Coffee Corner","amount":"3.25","currency":"USD","eventId":"tap-3"}`), ContentType: "application/json"},
			{Label: "ingest-duplicate", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`{"account":"wallet","payee":"Coffee Corner","amount":"3.25","currency":"USD","eventId":"tap-3"}`), ContentType: "application/json"},
			{Label: "link-account", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": OwnerAccount}},
			// Mapped card, no candidate -> created; a full-scope token may push too.
			{Label: "ingest-created", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "owner",
				RawBody: []byte(`{"account":"Wallet","payee":"Grocer","amount":"$41.10","currency":"usd","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"tap-4"}`), ContentType: "application/json"},
			{Label: "ingest-failed-payload", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`{"account":"wallet","amount":"free","currency":"USD"}`), ContentType: "application/json"},
			{Label: "ingest-not-json", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`not json at all`), ContentType: "text/plain"},
			{Label: "get-queued-event-list", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
			{Label: "get-transaction-list", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
			// Guest has no source: coded 400 (import.source_not_found).
			{Label: "err:ingest-no-source", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "guest",
				RawBody: []byte(`{"account":"x","amount":"1","currency":"USD"}`), ContentType: "application/json"},
			// Scope + access enforcement live in the middleware; both envelopes are frozen.
			{Label: "err:ingest-token-on-management-route", Method: "POST", Path: "/api/v1/import/create-source", Auth: "ingest",
				Body: map[string]any{"provider": "apple-wallet", "name": "x"}},
			{Label: "err:ingest-token-on-read-route", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "ingest"},
			{Label: "err:readonly-ingest", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "readonly",
				RawBody: []byte(`{"account":"x","amount":"1","currency":"USD"}`), ContentType: "application/json"},
		}
	}})

	register(Scenario{Name: "import_queue", Calls: func() []Call {
		var created string
		return []Call{
			{Label: "get-queued-event-list", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
			{Label: "skip-queued-event", Method: "POST", Path: "/api/v1/import/skip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "err:skip-queued-event-again", Method: "POST", Path: "/api/v1/import/skip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "unskip-queued-event", Method: "POST", Path: "/api/v1/import/unskip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "err:unskip-queued-event-again", Method: "POST", Path: "/api/v1/import/unskip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "err:skip-foreign-link", Method: "POST", Path: "/api/v1/import/skip-queued-event", Auth: "guest",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			// Manual import of the queued tap: the SPA prefills create-transaction from the row.
			{Label: "import-queued-event", Method: "POST", Path: "/api/v1/import/import-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued, "transaction": map[string]any{
					"id": "b0000000-0000-0000-0000-0000000000f1", "type": "expense", "accountId": OwnerAccount,
					"amount": "12.50", "categoryId": CatFood, "payeeId": PayeeShop, "date": "2026-08-20 17:42:03", "description": "Shop (wallet)", "labelIds": []string{}}},
				CaptureIDInto: &created},
			{Label: "err:import-queued-event-again", Method: "POST", Path: "/api/v1/import/import-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued, "transaction": map[string]any{
					"id": "b0000000-0000-0000-0000-0000000000f2", "type": "expense", "accountId": OwnerAccount,
					"amount": "12.50", "date": "2026-08-20 17:42:03", "labelIds": []string{}}}},
			{Label: "get-transaction-import-list", Method: "GET", Path: "/api/v1/import/get-transaction-import-list?transactionId=b0000000-0000-0000-0000-0000000000f1", Auth: "owner"},
			{Label: "err:get-transaction-import-list-blank", Method: "GET", Path: "/api/v1/import/get-transaction-import-list", Auth: "owner"},
			{Label: "err:get-transaction-import-list-foreign", Method: "GET", Path: "/api/v1/import/get-transaction-import-list?transactionId=" + Txn2, Auth: "guest"},
			// Failed event: retry re-parses the stored payload (still broken -> failed again), then discard.
			{Label: "retry-event", Method: "POST", Path: "/api/v1/import/retry-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventFailed}},
			{Label: "err:retry-event-not-failed", Method: "POST", Path: "/api/v1/import/retry-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventQueued}},
			{Label: "discard-event", Method: "POST", Path: "/api/v1/import/discard-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventFailed}},
			{Label: "err:discard-event-gone", Method: "POST", Path: "/api/v1/import/discard-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventFailed}},
			{Label: "get-queued-event-list-final", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
		}
	}})
}
```

If `Call` has no `CaptureIDInto` use in this scenario (the id is fixed), drop the `var created` + `CaptureIDInto` lines — they are harmless but unnecessary. If `RawBody`/`ContentType` are named differently in `apiparity.go`, use the existing names (the stage-1 catalogue already has an ingest-token 401 call; copy its field spelling).

Then regenerate and INSPECT:

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ -count=1 && git status --short internal/test/apiparity/testdata/golden/ && git diff --stat internal/test/apiparity/testdata/golden/`

Expected: four NEW goldens (`import_sources`, `import_account_links`, `import_ingest`, `import_queue`); no existing golden changes (the seed additions are all import-table rows; `isImported` on Txn2 was already 1 in stage 1). Open each new golden and check, at minimum: `link-account` → `run.status: "completed"`, `run.matchedCount: 1`, `run.importedCount: 0`; `ingest-queued` → `status: "queued"`; `ingest-duplicate` → `"duplicate"`; `ingest-created` → `"created"`; `ingest-failed-payload` and `ingest-not-json` → `"failed"` (a 200 — the event is stored for review); `err:ingest-token-on-management-route` and `err:ingest-token-on-read-route` → 401 `"Invalid access token"`; `err:readonly-ingest` → 402; `err:ingest-no-source` → 400 with message "Apple Wallet is not connected. Connect it in Settings → Import & export first." and code `import.source_not_found` absent from the body (the envelope carries only the rendered message).

Run: `go test ./internal/test/apiparity/ -count=1` — Expected: PASS including the route/scenario guards (14 new routes, 4 new scenarios).

- [ ] **Step 15: Full smoke tier**

Run: `make go-test` — Expected: PASS (build, vet, gofmt, OpenAPI fresh, i18ntest, coverage ≥ `GO_COVER_MIN`).

- [ ] **Step 16: Commit**

```bash
git add internal/imports/api internal/web/router internal/server/server.go internal/test/fixture/imports.go internal/test/apiparity internal/web/apidoc
git commit -m "feat(imports): Apple Wallet HTTP edge, server wiring, served matcher config, apiparity"
```

---

### Task 8: Frontend API client, hooks, metrics, platform helpers, i18n catalogue

**Files:**
- Create: `web/src/api/dto/imports.ts`
- Create: `web/src/api/imports.ts`
- Create: `web/src/features/imports/queries.ts`
- Create: `web/src/features/imports/queries.test.tsx`
- Create: `web/src/lib/platform.test.ts`
- Create: `scripts/add-imports-locale-keys.py` (throwaway helper, deleted at the end of the task — never committed)
- Modify: `web/src/app/queryKeys.ts`
- Modify: `web/src/lib/metrics.ts:128-129`
- Modify: `web/src/lib/platform.ts`
- Modify: `web/src/lib/config.ts` (the `EconumoConfig` interface)
- Modify: `web/public/econumo-config.js`
- Modify: `web/src/app/router-pages.ts`
- Modify: `web/src/api/user.ts:136-146`
- Modify: `web/src/features/settings/security.ts:37-47`
- Modify: `web/src/features/transactions/queries.ts:17` (export `useApplyTransactionItem`)
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (new `imports` namespace)

**Interfaces:**
- Consumes: the 14 routes under `/api/v1/import/` and the result DTOs from Task 7 (`ImportSourceResult`, `ImportCardResult`, `UpdateImportAccountResult`, `IngestEventResult`, `GetImportQueueResult`, `ImportQueuedEventResult`, `ImportFailedEventResult`, `TransactionImportLinkResult`, `CreateTransactionResult`); `useApplyTransactionItem` from `web/src/features/transactions/queries.ts`.
- Produces (used by Tasks 9 and 10): every TS type in `web/src/api/dto/imports.ts`; hooks `useImportSources`, `useCreateImportSource`, `useDeleteImportSource`, `useLinkImportAccount`, `useIgnoreImportAccount`, `useUnlinkImportAccount`, `useImportQueue`, `useImportQueuedEvent`, `useSkipQueuedEvent`, `useUnskipQueuedEvent`, `useRetryImportEvent`, `useDiscardImportEvent`, `useTransactionImportLinks`; `queryKeys.importSources`, `queryKeys.importQueue`, `queryKeys.transactionImports(id)`; `isIOS()`; `RouterPage.SETTINGS_DATA` (`/settings/data`), `RouterPage.IMPORT_QUEUE` (`/imports/queue`); `METRICS.IMPORT_SOURCE_CONNECT`, `IMPORT_ACCOUNT_LINK`, `IMPORT_ACCOUNT_IGNORE`, `IMPORT_QUEUE_IMPORT`, `IMPORT_QUEUE_SKIP`; `createPersonalToken(name, expiresAt, scope)`; `useCreatePersonalToken` accepting `{ name, expiresAt, scope? }`; the `imports.*` catalogue keys listed in Step 11.

The metrics coverage test (`web/src/lib/metrics-coverage.test.ts`) fails when a `METRICS` key is never referenced, so this task adds only the five keys its hooks fire; Task 9 adds `IMPORT_SHORTCUT_DOWNLOAD` / `IMPORT_SHORTCUT_CONFIGURE` together with the component that fires them.

- [ ] **Step 1: Wire DTOs**

Create `web/src/api/dto/imports.ts` — a field-for-field mirror of the Go DTOs, wire names (camelCase) exactly as the Go `json:` tags:

```ts
import type { Id } from '../types'
import type { AccountDto } from './account'
import type { CreateTransactionDto, TransactionDto, TransactionType } from './transaction'

export type ImportProvider = 'apple-wallet'
export type ImportCardState = 'mapped' | 'ignored' | 'unmapped'
export type ImportQueueReason = 'unmapped' | 'account_deleted' | 'no_rate'
export type IngestStatus = 'created' | 'queued' | 'skipped' | 'duplicate' | 'failed'

export interface ImportCardDto {
  externalAccountId: string
  externalName: string
  externalCurrency: string
  state: ImportCardState
  /** '' unless state === 'mapped' */
  accountId: Id | ''
  queuedCount: number
  tapCount: number
  /** "YYYY-MM-DD HH:mm:ss" of the newest event from this card, '' when none */
  lastSeenAt: string
}

export interface ImportSourceDto {
  id: Id
  provider: ImportProvider
  name: string
  status: string
  createdAt: string
  cards: ImportCardDto[]
}

export interface ImportRunDto {
  id: Id
  status: string
  importedCount: number
  matchedCount: number
  skippedCount: number
  failedCount: number
}

/** link-account / ignore-account / unlink-account: the refreshed source, plus
    the conversion run when link-account drained a queue (null otherwise). */
export interface UpdateImportAccountDto {
  item: ImportSourceDto
  run: ImportRunDto | null
}

export interface IngestEventDto {
  status: IngestStatus
  /** '' for a duplicate (nothing new was stored) */
  eventId: Id | ''
}

export interface ImportQueuedEventDto {
  linkId: Id
  sourceId: Id
  externalAccountId: string
  /** '' while the card is unmapped */
  accountId: Id | ''
  payee: string
  amount: string
  currency: string
  type: TransactionType
  /** "YYYY-MM-DD HH:mm:ss" */
  postedAt: string
  reason: ImportQueueReason
}

export interface ImportFailedEventDto {
  eventId: Id
  sourceId: Id
  receivedAt: string
  error: string
  /** the raw body as received, for the "needs attention" panel */
  payload: string
}

export interface ImportQueueDto {
  queued: ImportQueuedEventDto[]
  skipped: ImportQueuedEventDto[]
  failed: ImportFailedEventDto[]
}

export interface ImportQueuedEventPayload {
  linkId: Id
  transaction: CreateTransactionDto
}

export interface ImportQueuedEventResultDto {
  item: TransactionDto
  accounts: AccountDto[]
}

export interface TransactionImportLinkDto {
  id: Id
  sourceId: Id
  provider: ImportProvider
  sourceName: string
  externalAccountId: string
  externalTransactionId: string
  externalPayee: string
  externalAmount: string
  externalCurrency: string
  externalPostedAt: string
  status: string
  importedAt: string
}
```

- [ ] **Step 2: API client**

Create `web/src/api/imports.ts`, one function per route, in the `recurring.ts` style:

```ts
import { api, apiUrl } from './client'
import type { Id } from './types'
import type {
  ImportQueueDto,
  ImportQueuedEventPayload,
  ImportQueuedEventResultDto,
  ImportSourceDto,
  IngestEventDto,
  TransactionImportLinkDto,
  UpdateImportAccountDto,
} from './dto/imports'

interface Envelope<T> {
  data: T
}

export async function getImportSourceList(): Promise<ImportSourceDto[]> {
  const response = await api.get<Envelope<{ items: ImportSourceDto[] }>>(apiUrl('/api/v1/import/get-source-list'))
  return response.data.data.items
}

export async function createImportSource(provider: 'apple-wallet', name: string): Promise<ImportSourceDto> {
  const response = await api.post<Envelope<{ item: ImportSourceDto }>>(apiUrl('/api/v1/import/create-source'), { provider, name })
  return response.data.data.item
}

export async function deleteImportSource(id: Id): Promise<ImportSourceDto[]> {
  const response = await api.post<Envelope<{ items: ImportSourceDto[] }>>(apiUrl('/api/v1/import/delete-source'), { id })
  return response.data.data.items
}

export async function linkImportAccount(sourceId: Id, externalAccountId: string, accountId: Id): Promise<UpdateImportAccountDto> {
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/link-account'), { sourceId, externalAccountId, accountId })
  return response.data.data
}

export async function ignoreImportAccount(sourceId: Id, externalAccountId: string): Promise<UpdateImportAccountDto> {
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/ignore-account'), { sourceId, externalAccountId })
  return response.data.data
}

export async function unlinkImportAccount(sourceId: Id, externalAccountId: string): Promise<UpdateImportAccountDto> {
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/unlink-account'), { sourceId, externalAccountId })
  return response.data.data
}

export async function getImportQueue(): Promise<ImportQueueDto> {
  const response = await api.get<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/get-queued-event-list'))
  return response.data.data
}

export async function importQueuedEvent(payload: ImportQueuedEventPayload): Promise<ImportQueuedEventResultDto> {
  const response = await api.post<Envelope<ImportQueuedEventResultDto>>(apiUrl('/api/v1/import/import-queued-event'), payload)
  return response.data.data
}

export async function skipQueuedEvent(linkId: Id): Promise<ImportQueueDto> {
  const response = await api.post<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/skip-queued-event'), { linkId })
  return response.data.data
}

export async function unskipQueuedEvent(linkId: Id): Promise<ImportQueueDto> {
  const response = await api.post<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/unskip-queued-event'), { linkId })
  return response.data.data
}

export async function retryImportEvent(eventId: Id): Promise<IngestEventDto> {
  const response = await api.post<Envelope<IngestEventDto>>(apiUrl('/api/v1/import/retry-event'), { eventId })
  return response.data.data
}

export async function discardImportEvent(eventId: Id): Promise<ImportQueueDto> {
  const response = await api.post<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/discard-event'), { eventId })
  return response.data.data
}

export async function getTransactionImportList(transactionId: Id): Promise<TransactionImportLinkDto[]> {
  const response = await api.get<Envelope<{ items: TransactionImportLinkDto[] }>>(
    apiUrl('/api/v1/import/get-transaction-import-list'),
    { params: { transactionId } },
  )
  return response.data.data.items
}
```

- [ ] **Step 3: Query keys, router pages, platform, config**

`web/src/app/queryKeys.ts` — add three entries to the `queryKeys` object (after `updateInfo`):

```ts
  importSources: ['importSources'] as const,
  importQueue: ['importQueue'] as const,
  transactionImports: (transactionId: string) => ['transactionImports', transactionId] as const,
```

`web/src/app/router-pages.ts` — add two members to `RouterPage` (after `SETTINGS_RECURRING`):

```ts
  SETTINGS_DATA: '/settings/data',
  IMPORT_QUEUE: '/imports/queue',
```

`web/src/lib/platform.ts` — append:

```ts
// iPadOS 13+ reports itself as a Mac; the touch-point probe tells them apart.
// Used only to decide whether the "Configure on this iPhone" button (a
// shortcuts:// deep link) can work — never for feature gating.
export function isIOS(): boolean {
  const ua = navigator.userAgent
  return /iPhone|iPad|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
}
```

`web/src/lib/config.ts` — extend the `EconumoConfig` interface with the served matcher thresholds (the router merges `IMPORT_MATCHER` in Task 7; nothing reads it client-side until stage 3's grouping, but the type lands with the server key so the two never drift):

```ts
  IMPORT_MATCHER?: { matchDays: number; tipDays: number; tipTolerancePct: number; tokenMinLength: number }
```

`web/public/econumo-config.js` — add the dist fallback (same defaults as `DefaultMatcherConfig()`), after `VERSION: null,`:

```js
  IMPORT_MATCHER: { matchDays: 3, tipDays: 5, tipTolerancePct: 20, tokenMinLength: 3 },
```

- [ ] **Step 4: Platform test**

Create `web/src/lib/platform.test.ts`:

```ts
import { isIOS } from './platform'

function stubNavigator(overrides: { userAgent?: string; platform?: string; maxTouchPoints?: number }) {
  Object.defineProperty(window.navigator, 'userAgent', { value: overrides.userAgent ?? 'Mozilla/5.0 (X11; Linux x86_64)', configurable: true })
  Object.defineProperty(window.navigator, 'platform', { value: overrides.platform ?? 'Linux x86_64', configurable: true })
  Object.defineProperty(window.navigator, 'maxTouchPoints', { value: overrides.maxTouchPoints ?? 0, configurable: true })
}

it('isIOS matches an iPhone user agent', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15' })
  expect(isIOS()).toBe(true)
})

it('isIOS treats a touch-capable "MacIntel" as an iPad', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 5 })
  expect(isIOS()).toBe(true)
})

it('isIOS is false on a desktop Mac and on Linux', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 0 })
  expect(isIOS()).toBe(false)
  stubNavigator({})
  expect(isIOS()).toBe(false)
})
```

Run: `(cd web && pnpm vitest run src/lib/platform.test.ts)`
Expected: PASS (3 tests).

- [ ] **Step 5: Metrics keys**

`web/src/lib/metrics.ts` — insert before `} as const` (line 129), after `UI_MODAL_RECURRING_CLOSE`:

```ts
  IMPORT_SOURCE_CONNECT: 'appImportSourceConnect',
  IMPORT_ACCOUNT_LINK: 'appImportAccountLink',
  IMPORT_ACCOUNT_IGNORE: 'appImportAccountIgnore',
  IMPORT_QUEUE_IMPORT: 'appImportQueueImport',
  IMPORT_QUEUE_SKIP: 'appImportQueueSkip',
```

- [ ] **Step 6: Ingest-scope PAT creation**

`web/src/api/user.ts` — replace the `createPersonalToken` function and its comment:

```ts
// expiresAt: a "YYYY-MM-DD HH:mm:ss" datetime, or null for a token that never expires.
// The Settings -> Personal tokens UI creates full-scope tokens; the Apple
// Wallet setup on Settings -> Data mints ingest-scope ones.
export async function createPersonalToken(
  name: string,
  expiresAt: string | null,
  scope: 'full' | 'ingest' = 'full',
): Promise<CreatedPersonalTokenDto> {
  const response = await api.post<Envelope<CreatedPersonalTokenDto>>(
    apiUrl('/api/v1/user/create-personal-token'),
    { name, scope, expiresAt: expiresAt ?? '' },
  )
  return response.data.data
}
```

`web/src/features/settings/security.ts` — the `useCreatePersonalToken` mutation accepts an optional scope:

```ts
export function useCreatePersonalToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, expiresAt, scope }: { name: string; expiresAt: string | null; scope?: 'full' | 'ingest' }) =>
      userApi.createPersonalToken(name, expiresAt, scope),
    onSuccess: () => {
      trackEvent(METRICS.PERSONAL_TOKEN_CREATE)
      return queryClient.invalidateQueries({ queryKey: queryKeys.personalTokens })
    },
  })
}
```

Run: `(cd web && pnpm vitest run src/features/settings)`
Expected: PASS — the existing PAT page tests still post `scope: 'full'` (the default).

- [ ] **Step 7: Export the transaction cache applier**

`web/src/features/transactions/queries.ts:17` — change `function useApplyTransactionItem()` to `export function useApplyTransactionItem()`. No other change; the imports hook below reuses it so an imported transaction updates the same caches as a created one.

- [ ] **Step 8: Hooks**

Create `web/src/features/imports/queries.ts`. Every mutation that changes data the user sees fires its metric from the hook (the shared choke point), and the cache updates mirror `usePostRecurring`:

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as importsApi from '@/api/imports'
import type { Id } from '@/api/types'
import type { ImportQueueDto, ImportQueuedEventPayload, ImportSourceDto, UpdateImportAccountDto } from '@/api/dto/imports'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useApplyTransactionItem } from '@/features/transactions/queries'

export function useImportSources() {
  return useQuery({ queryKey: queryKeys.importSources, queryFn: importsApi.getImportSourceList, staleTime: TEN_MINUTES })
}

export function useImportQueue() {
  return useQuery({ queryKey: queryKeys.importQueue, queryFn: importsApi.getImportQueue, staleTime: TEN_MINUTES })
}

export function useCreateImportSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ provider, name }: { provider: 'apple-wallet'; name: string }) => importsApi.createImportSource(provider, name),
    onSuccess: (item) => {
      queryClient.setQueryData<ImportSourceDto[]>(queryKeys.importSources, (prev) => {
        const items = prev ?? []
        return items.some((s) => s.id === item.id) ? items.map((s) => (s.id === item.id ? item : s)) : [...items, item]
      })
      trackEvent(METRICS.IMPORT_SOURCE_CONNECT, { provider: item.provider })
    },
  })
}

export function useDeleteImportSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: importsApi.deleteImportSource,
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.importSources, items)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
    },
  })
}

// link/ignore/unlink all return the refreshed source; a link that drained a
// queue also created transactions, so the ledger caches are invalidated too.
function useApplyImportAccount() {
  const queryClient = useQueryClient()
  return (result: UpdateImportAccountDto) => {
    queryClient.setQueryData<ImportSourceDto[]>(queryKeys.importSources, (prev) =>
      (prev ?? []).map((s) => (s.id === result.item.id ? result.item : s)))
    void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
    if (result.run) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
    }
  }
}

export function useLinkImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    mutationFn: ({ sourceId, externalAccountId, accountId }: { sourceId: Id; externalAccountId: string; accountId: Id }) =>
      importsApi.linkImportAccount(sourceId, externalAccountId, accountId),
    onSuccess: (result) => {
      apply(result)
      trackEvent(METRICS.IMPORT_ACCOUNT_LINK, { imported: result.run?.importedCount ?? 0, matched: result.run?.matchedCount ?? 0 })
    },
  })
}

export function useIgnoreImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    mutationFn: ({ sourceId, externalAccountId }: { sourceId: Id; externalAccountId: string }) =>
      importsApi.ignoreImportAccount(sourceId, externalAccountId),
    onSuccess: (result) => {
      apply(result)
      trackEvent(METRICS.IMPORT_ACCOUNT_IGNORE)
    },
  })
}

export function useUnlinkImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    mutationFn: ({ sourceId, externalAccountId }: { sourceId: Id; externalAccountId: string }) =>
      importsApi.unlinkImportAccount(sourceId, externalAccountId),
    onSuccess: apply,
  })
}

export function useImportQueuedEvent() {
  const queryClient = useQueryClient()
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: (payload: ImportQueuedEventPayload) => importsApi.importQueuedEvent(payload),
    onSuccess: (result, payload) => {
      apply(result, 'add')
      queryClient.setQueryData<ImportQueueDto>(queryKeys.importQueue, (prev) =>
        prev ? { ...prev, queued: prev.queued.filter((q) => q.linkId !== payload.linkId) } : prev)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      trackEvent(METRICS.IMPORT_QUEUE_IMPORT)
    },
  })
}

function useReplaceQueue() {
  const queryClient = useQueryClient()
  return (queue: ImportQueueDto) => {
    queryClient.setQueryData(queryKeys.importQueue, queue)
    void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
  }
}

export function useSkipQueuedEvent() {
  const replace = useReplaceQueue()
  return useMutation({
    mutationFn: importsApi.skipQueuedEvent,
    onSuccess: (queue) => {
      replace(queue)
      trackEvent(METRICS.IMPORT_QUEUE_SKIP)
    },
  })
}

export function useUnskipQueuedEvent() {
  const replace = useReplaceQueue()
  return useMutation({ mutationFn: importsApi.unskipQueuedEvent, onSuccess: replace })
}

export function useDiscardImportEvent() {
  const replace = useReplaceQueue()
  return useMutation({ mutationFn: importsApi.discardImportEvent, onSuccess: replace })
}

// A retry may create a transaction (status "created") or park the event
// somewhere else in the queue, so every cache it could have touched is refetched.
export function useRetryImportEvent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: importsApi.retryImportEvent,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    },
  })
}

export function useTransactionImportLinks(transactionId: Id, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.transactionImports(transactionId),
    queryFn: () => importsApi.getTransactionImportList(transactionId),
    enabled,
    staleTime: TEN_MINUTES,
  })
}
```

- [ ] **Step 9: Hook tests**

Create `web/src/features/imports/queries.test.tsx` (same harness as `web/src/features/recurring/queries.test.tsx`):

```tsx
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { ImportQueueDto, ImportSourceDto } from '@/api/dto/imports'
import type { TransactionDto } from '@/api/dto/transaction'
import { useImportQueuedEvent, useImportSources, useLinkImportAccount, useSkipQueuedEvent } from './queries'

const card = { externalAccountId: 'wallet', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 1, tapCount: 1, lastSeenAt: '2026-08-20 17:42:03' }
const wireSource: ImportSourceDto = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [card] }
const queued = { linkId: 'l1', sourceId: 's1', externalAccountId: 'wallet', accountId: '', payee: 'Blue Bottle', amount: '4.75', currency: 'USD', type: 'expense' as const, postedAt: '2026-08-20 17:42:03', reason: 'unmapped' as const }
const wireQueue: ImportQueueDto = { queued: [queued], skipped: [], failed: [] }

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('useImportSources fetches the source list with its cards', async () => {
  server.use(http.get('*/api/v1/import/get-source-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [wireSource] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useImportSources(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data![0].cards[0].state).toBe('unmapped')
})

it('useLinkImportAccount replaces the source in the cache and refetches the ledger when a run happened', async () => {
  const mapped = { ...wireSource, cards: [{ ...card, state: 'mapped', accountId: 'a1', queuedCount: 0 }] }
  server.use(http.post('*/api/v1/import/link-account', () =>
    HttpResponse.json({ success: true, message: '', data: { item: mapped, run: { id: 'r1', status: 'finished', importedCount: 1, matchedCount: 0, skippedCount: 0, failedCount: 0 } } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportSourceDto[]>(queryKeys.importSources, [wireSource])
  queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, [])
  const { result } = renderHook(() => useLinkImportAccount(), { wrapper })
  result.current.mutate({ sourceId: 's1', externalAccountId: 'wallet', accountId: 'a1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<ImportSourceDto[]>(queryKeys.importSources)![0].cards[0].state).toBe('mapped')
  expect(queryClient.getQueryState(queryKeys.transactions)?.isInvalidated).toBe(true)
})

it('useImportQueuedEvent prepends the transaction and drops the queue row', async () => {
  const wireTx = {
    id: 't1', author: { id: 'u1', name: 'U', avatar: 'face:fuchsia' }, type: 'expense',
    accountId: 'a1', accountRecipientId: null, amount: '4.75', amountRecipient: null,
    categoryId: 'c1', description: 'Blue Bottle', payeeId: null, tagId: null, labelIds: [], recurringId: null, isImported: 1,
    date: '2026-08-20 17:42:03',
  }
  server.use(http.post('*/api/v1/import/import-queued-event', () =>
    HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [] } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportQueueDto>(queryKeys.importQueue, wireQueue)
  queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, [])
  const { result } = renderHook(() => useImportQueuedEvent(), { wrapper })
  result.current.mutate({
    linkId: 'l1',
    transaction: { id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '4.75', amountRecipient: null, categoryId: 'c1', description: 'Blue Bottle', payeeId: null, tagId: null, labelIds: [], date: '2026-08-20 17:42:03' },
  })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions)![0].id).toBe('t1')
  expect(queryClient.getQueryData<ImportQueueDto>(queryKeys.importQueue)!.queued).toHaveLength(0)
})

it('useSkipQueuedEvent replaces the whole queue from the response', async () => {
  server.use(http.post('*/api/v1/import/skip-queued-event', () =>
    HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [queued], failed: [] } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportQueueDto>(queryKeys.importQueue, wireQueue)
  const { result } = renderHook(() => useSkipQueuedEvent(), { wrapper })
  result.current.mutate('l1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<ImportQueueDto>(queryKeys.importQueue)!.skipped).toHaveLength(1)
})
```

If `CreateTransactionDto` in `web/src/api/dto/transaction.ts` has a field set that differs from the literal used above (check the file), adjust the literal — the test must type-check under `pnpm exec tsc -b`.

Run: `(cd web && pnpm vitest run src/features/imports src/lib/metrics-coverage.test.ts)`
Expected: PASS — all four hook tests, and the coverage test sees every new `METRICS` key referenced.

- [ ] **Step 10: The `imports` catalogue namespace, all 11 languages**

The catalogue is shared by both stacks and `internal/test/i18ntest` enforces key parity, `{var}` placeholder parity and frontend `t('…')` coverage across every language, so the namespace lands in all 11 files at once. Write the throwaway helper `scripts/add-imports-locale-keys.py` (delete it after running; never commit it), which inserts the `imports` object after the `recurring` object in each `locales/<lang>.json` with the file's existing 2-space indentation:

```python
#!/usr/bin/env python3
import json, pathlib

EN = {
  "badge": "Imported",
  "banner": {
    "text": "{count} imported transaction is waiting for review | {count} imported transactions are waiting for review",
    "cta": "Review"
  },
  "data_page": {"menu_item": "Import & export", "header": "Import & export", "csv": {"header": "CSV"}},
  "apple_wallet": {
    "header": "Apple Wallet",
    "intro": "Log Apple Pay taps automatically with an iPhone Shortcut. Each tap is sent to Econumo; you map every card to an account once.",
    "connect": "Set up Apple Wallet",
    "connected": "Connected",
    "disconnect": "Disconnect",
    "disconnect_modal": {
      "title": "Disconnect Apple Wallet?",
      "question": "Card mappings and the review queue of this connection will be removed. Transactions you already imported stay."
    },
    "steps": {
      "install": {"title": "1. Get the Shortcuts", "text": "Install both Shortcuts on your iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
      "configure": {
        "title": "2. Configure",
        "button": "Configure on this iPhone",
        "text": "Creates an access token for the Shortcut and hands it over together with the server address.",
        "desktop": "Open Settings → Import & export on your iPhone to configure the Shortcut there.",
        "done": "The Shortcut is configured. Your next Apple Pay payment will show up here."
      },
      "automate": {"title": "3. Automate", "text": "In the Shortcuts app add an automation: Transaction → Run \"Econumo Wallet\" with the transaction as input."}
    },
    "manual": {
      "toggle": "Configure manually",
      "token": "Access token (shown once)",
      "url": "Server",
      "body": "Request body",
      "action_1": "Get Details of Transaction → fill the request body above",
      "action_2": "Get Contents of URL → POST {url}/api/v1/import/ingest-apple-wallet-event",
      "action_3": "Header Authorization: Bearer <token>",
      "action_4": "Header Content-Type: application/json",
      "action_5": "Request Body: JSON, the fields above"
    },
    "same_named_cards": "Cards that share a name in Wallet arrive here as one card.",
    "cards": {
      "header": "Cards",
      "empty": "No cards yet — the first Apple Pay tap adds one.",
      "state": {"mapped": "Mapped", "ignored": "Ignored", "unmapped": "Unmapped · {count} queued"},
      "taps": "{count} tap | {count} taps",
      "last_seen": "Last seen {date}",
      "map": "Map to account",
      "map_instead": "Map instead",
      "ignore": "Ignore",
      "unlink": "Unmap",
      "unlink_modal": {"title": "Unmap {card}?", "question": "New taps from this card will wait for review until you map it again."},
      "map_modal": {"header": "Map {card}", "account": "Account", "submit": "Map"},
      "mapped_toast": "{imported} imported, {matched} matched, {skipped} skipped"
    }
  },
  "queue": {
    "header": "Import queue",
    "empty": "Nothing to review.",
    "queued": "Waiting for review",
    "skipped": "Skipped",
    "failed": "Needs attention",
    "reason": {"unmapped": "Card not mapped", "account_deleted": "Account deleted", "no_rate": "No exchange rate for {currency}"},
    "import": "Import",
    "skip": "Skip",
    "unskip": "Restore",
    "retry": "Retry",
    "discard": "Discard",
    "received": "Received {date}",
    "retry_result": {"created": "Imported", "queued": "Queued for review", "skipped": "Skipped", "duplicate": "Already imported", "failed": "Still failing"}
  },
  "provenance": {"header": "Imported from"}
}

# Per-language overrides: same shape as EN; every leaf present in EN must be present here.
TRANSLATIONS = {
  "de": {
    "badge": "Importiert",
    "banner": {"text": "{count} importierte Transaktion wartet auf Prüfung | {count} importierte Transaktionen warten auf Prüfung", "cta": "Prüfen"},
    "data_page": {"menu_item": "Import & Export", "header": "Import & Export", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Erfasse Apple-Pay-Zahlungen automatisch mit einem iPhone-Kurzbefehl. Jede Zahlung wird an Econumo gesendet; jede Karte ordnest du einmal einem Konto zu.",
      "connect": "Apple Wallet einrichten", "connected": "Verbunden", "disconnect": "Trennen",
      "disconnect_modal": {"title": "Apple Wallet trennen?", "question": "Kartenzuordnungen und die Prüfwarteschlange dieser Verbindung werden entfernt. Bereits importierte Transaktionen bleiben erhalten."},
      "steps": {
        "install": {"title": "1. Kurzbefehle laden", "text": "Installiere beide Kurzbefehle auf deinem iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Konfigurieren", "button": "Auf diesem iPhone konfigurieren", "text": "Erstellt ein Zugriffstoken für den Kurzbefehl und übergibt es zusammen mit der Serveradresse.", "desktop": "Öffne Einstellungen → Import & Export auf deinem iPhone, um den Kurzbefehl dort zu konfigurieren.", "done": "Der Kurzbefehl ist konfiguriert. Deine nächste Apple-Pay-Zahlung erscheint hier."},
        "automate": {"title": "3. Automatisieren", "text": "Füge in der Kurzbefehle-App eine Automation hinzu: Transaktion → „Econumo Wallet“ mit der Transaktion als Eingabe ausführen."}
      },
      "manual": {"toggle": "Manuell konfigurieren", "token": "Zugriffstoken (wird einmal angezeigt)", "url": "Server", "body": "Anfragetext", "action_1": "Details der Transaktion abrufen → den obigen Anfragetext befüllen", "action_2": "Inhalt der URL abrufen → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Header Authorization: Bearer <token>", "action_4": "Header Content-Type: application/json", "action_5": "Anfragetext: JSON, die Felder oben"},
      "same_named_cards": "Karten mit gleichem Namen in Wallet erscheinen hier als eine Karte.",
      "cards": {"header": "Karten", "empty": "Noch keine Karten — die erste Apple-Pay-Zahlung fügt eine hinzu.", "state": {"mapped": "Zugeordnet", "ignored": "Ignoriert", "unmapped": "Nicht zugeordnet · {count} wartend"}, "taps": "{count} Zahlung | {count} Zahlungen", "last_seen": "Zuletzt gesehen {date}", "map": "Konto zuordnen", "map_instead": "Stattdessen zuordnen", "ignore": "Ignorieren", "unlink": "Zuordnung aufheben", "unlink_modal": {"title": "Zuordnung von {card} aufheben?", "question": "Neue Zahlungen dieser Karte warten auf Prüfung, bis du sie erneut zuordnest."}, "map_modal": {"header": "{card} zuordnen", "account": "Konto", "submit": "Zuordnen"}, "mapped_toast": "{imported} importiert, {matched} abgeglichen, {skipped} übersprungen"}
    },
    "queue": {"header": "Importwarteschlange", "empty": "Nichts zu prüfen.", "queued": "Wartet auf Prüfung", "skipped": "Übersprungen", "failed": "Braucht Aufmerksamkeit", "reason": {"unmapped": "Karte nicht zugeordnet", "account_deleted": "Konto gelöscht", "no_rate": "Kein Wechselkurs für {currency}"}, "import": "Importieren", "skip": "Überspringen", "unskip": "Wiederherstellen", "retry": "Erneut versuchen", "discard": "Verwerfen", "received": "Empfangen {date}", "retry_result": {"created": "Importiert", "queued": "Zur Prüfung eingereiht", "skipped": "Übersprungen", "duplicate": "Bereits importiert", "failed": "Schlägt weiterhin fehl"}},
    "provenance": {"header": "Importiert aus"}
  },
  "es": {
    "badge": "Importada",
    "banner": {"text": "{count} transacción importada espera revisión | {count} transacciones importadas esperan revisión", "cta": "Revisar"},
    "data_page": {"menu_item": "Importar y exportar", "header": "Importar y exportar", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Registra los pagos con Apple Pay automáticamente con un atajo de iPhone. Cada pago se envía a Econumo; asignas cada tarjeta a una cuenta una sola vez.",
      "connect": "Configurar Apple Wallet", "connected": "Conectado", "disconnect": "Desconectar",
      "disconnect_modal": {"title": "¿Desconectar Apple Wallet?", "question": "Se eliminarán las asignaciones de tarjetas y la cola de revisión de esta conexión. Las transacciones ya importadas se conservan."},
      "steps": {
        "install": {"title": "1. Obtén los atajos", "text": "Instala ambos atajos en tu iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Configura", "button": "Configurar en este iPhone", "text": "Crea un token de acceso para el atajo y se lo entrega junto con la dirección del servidor.", "desktop": "Abre Ajustes → Importar y exportar en tu iPhone para configurar el atajo allí.", "done": "El atajo está configurado. Tu próximo pago con Apple Pay aparecerá aquí."},
        "automate": {"title": "3. Automatiza", "text": "En la app Atajos añade una automatización: Transacción → Ejecutar \"Econumo Wallet\" con la transacción como entrada."}
      },
      "manual": {"toggle": "Configurar manualmente", "token": "Token de acceso (se muestra una vez)", "url": "Servidor", "body": "Cuerpo de la petición", "action_1": "Obtener detalles de la transacción → rellenar el cuerpo de arriba", "action_2": "Obtener contenido de URL → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Cabecera Authorization: Bearer <token>", "action_4": "Cabecera Content-Type: application/json", "action_5": "Cuerpo de la petición: JSON, los campos de arriba"},
      "same_named_cards": "Las tarjetas con el mismo nombre en Wallet llegan aquí como una sola tarjeta.",
      "cards": {"header": "Tarjetas", "empty": "Aún no hay tarjetas — el primer pago con Apple Pay añadirá una.", "state": {"mapped": "Asignada", "ignored": "Ignorada", "unmapped": "Sin asignar · {count} en cola"}, "taps": "{count} pago | {count} pagos", "last_seen": "Visto por última vez {date}", "map": "Asignar a una cuenta", "map_instead": "Asignar en su lugar", "ignore": "Ignorar", "unlink": "Desasignar", "unlink_modal": {"title": "¿Desasignar {card}?", "question": "Los nuevos pagos de esta tarjeta esperarán revisión hasta que la asignes de nuevo."}, "map_modal": {"header": "Asignar {card}", "account": "Cuenta", "submit": "Asignar"}, "mapped_toast": "{imported} importadas, {matched} emparejadas, {skipped} omitidas"}
    },
    "queue": {"header": "Cola de importación", "empty": "Nada que revisar.", "queued": "Esperando revisión", "skipped": "Omitidas", "failed": "Requieren atención", "reason": {"unmapped": "Tarjeta sin asignar", "account_deleted": "Cuenta eliminada", "no_rate": "Sin tipo de cambio para {currency}"}, "import": "Importar", "skip": "Omitir", "unskip": "Restaurar", "retry": "Reintentar", "discard": "Descartar", "received": "Recibido {date}", "retry_result": {"created": "Importada", "queued": "En cola para revisión", "skipped": "Omitida", "duplicate": "Ya importada", "failed": "Sigue fallando"}},
    "provenance": {"header": "Importada desde"}
  },
  "fr": {
    "badge": "Importée",
    "banner": {"text": "{count} transaction importée attend une vérification | {count} transactions importées attendent une vérification", "cta": "Vérifier"},
    "data_page": {"menu_item": "Import et export", "header": "Import et export", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Enregistrez automatiquement vos paiements Apple Pay avec un raccourci iPhone. Chaque paiement est envoyé à Econumo ; vous associez chaque carte à un compte une seule fois.",
      "connect": "Configurer Apple Wallet", "connected": "Connecté", "disconnect": "Déconnecter",
      "disconnect_modal": {"title": "Déconnecter Apple Wallet ?", "question": "Les associations de cartes et la file de vérification de cette connexion seront supprimées. Les transactions déjà importées sont conservées."},
      "steps": {
        "install": {"title": "1. Récupérer les raccourcis", "text": "Installez les deux raccourcis sur votre iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Configurer", "button": "Configurer sur cet iPhone", "text": "Crée un jeton d'accès pour le raccourci et le lui transmet avec l'adresse du serveur.", "desktop": "Ouvrez Réglages → Import et export sur votre iPhone pour y configurer le raccourci.", "done": "Le raccourci est configuré. Votre prochain paiement Apple Pay apparaîtra ici."},
        "automate": {"title": "3. Automatiser", "text": "Dans l'app Raccourcis, ajoutez une automatisation : Transaction → Exécuter « Econumo Wallet » avec la transaction en entrée."}
      },
      "manual": {"toggle": "Configurer manuellement", "token": "Jeton d'accès (affiché une seule fois)", "url": "Serveur", "body": "Corps de la requête", "action_1": "Obtenir les détails de la transaction → remplir le corps ci-dessus", "action_2": "Obtenir le contenu de l'URL → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "En-tête Authorization: Bearer <token>", "action_4": "En-tête Content-Type: application/json", "action_5": "Corps de la requête : JSON, les champs ci-dessus"},
      "same_named_cards": "Les cartes portant le même nom dans Wallet arrivent ici comme une seule carte.",
      "cards": {"header": "Cartes", "empty": "Pas encore de carte — le premier paiement Apple Pay en ajoutera une.", "state": {"mapped": "Associée", "ignored": "Ignorée", "unmapped": "Non associée · {count} en attente"}, "taps": "{count} paiement | {count} paiements", "last_seen": "Vue pour la dernière fois {date}", "map": "Associer à un compte", "map_instead": "Associer plutôt", "ignore": "Ignorer", "unlink": "Dissocier", "unlink_modal": {"title": "Dissocier {card} ?", "question": "Les nouveaux paiements de cette carte attendront une vérification jusqu'à ce que vous l'associiez à nouveau."}, "map_modal": {"header": "Associer {card}", "account": "Compte", "submit": "Associer"}, "mapped_toast": "{imported} importées, {matched} rapprochées, {skipped} ignorées"}
    },
    "queue": {"header": "File d'importation", "empty": "Rien à vérifier.", "queued": "En attente de vérification", "skipped": "Ignorées", "failed": "Nécessitent votre attention", "reason": {"unmapped": "Carte non associée", "account_deleted": "Compte supprimé", "no_rate": "Aucun taux de change pour {currency}"}, "import": "Importer", "skip": "Ignorer", "unskip": "Restaurer", "retry": "Réessayer", "discard": "Abandonner", "received": "Reçu {date}", "retry_result": {"created": "Importée", "queued": "En file pour vérification", "skipped": "Ignorée", "duplicate": "Déjà importée", "failed": "Échoue toujours"}},
    "provenance": {"header": "Importée depuis"}
  },
  "it": {
    "badge": "Importata",
    "banner": {"text": "{count} transazione importata in attesa di revisione | {count} transazioni importate in attesa di revisione", "cta": "Rivedi"},
    "data_page": {"menu_item": "Importa ed esporta", "header": "Importa ed esporta", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Registra automaticamente i pagamenti Apple Pay con un comando rapido per iPhone. Ogni pagamento viene inviato a Econumo; associ ogni carta a un conto una sola volta.",
      "connect": "Configura Apple Wallet", "connected": "Connesso", "disconnect": "Disconnetti",
      "disconnect_modal": {"title": "Disconnettere Apple Wallet?", "question": "Le associazioni delle carte e la coda di revisione di questa connessione verranno rimosse. Le transazioni già importate restano."},
      "steps": {
        "install": {"title": "1. Scarica i comandi rapidi", "text": "Installa entrambi i comandi rapidi sul tuo iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Configura", "button": "Configura su questo iPhone", "text": "Crea un token di accesso per il comando rapido e glielo consegna insieme all'indirizzo del server.", "desktop": "Apri Impostazioni → Importa ed esporta sul tuo iPhone per configurare lì il comando rapido.", "done": "Il comando rapido è configurato. Il tuo prossimo pagamento Apple Pay comparirà qui."},
        "automate": {"title": "3. Automatizza", "text": "Nell'app Comandi rapidi aggiungi un'automazione: Transazione → Esegui \"Econumo Wallet\" con la transazione come input."}
      },
      "manual": {"toggle": "Configura manualmente", "token": "Token di accesso (mostrato una sola volta)", "url": "Server", "body": "Corpo della richiesta", "action_1": "Ottieni dettagli della transazione → compila il corpo qui sopra", "action_2": "Ottieni contenuto dell'URL → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Intestazione Authorization: Bearer <token>", "action_4": "Intestazione Content-Type: application/json", "action_5": "Corpo della richiesta: JSON, i campi qui sopra"},
      "same_named_cards": "Le carte con lo stesso nome in Wallet arrivano qui come un'unica carta.",
      "cards": {"header": "Carte", "empty": "Ancora nessuna carta — il primo pagamento Apple Pay ne aggiungerà una.", "state": {"mapped": "Associata", "ignored": "Ignorata", "unmapped": "Non associata · {count} in coda"}, "taps": "{count} pagamento | {count} pagamenti", "last_seen": "Ultimo utilizzo {date}", "map": "Associa a un conto", "map_instead": "Associa invece", "ignore": "Ignora", "unlink": "Dissocia", "unlink_modal": {"title": "Dissociare {card}?", "question": "I nuovi pagamenti di questa carta attenderanno la revisione finché non la associ di nuovo."}, "map_modal": {"header": "Associa {card}", "account": "Conto", "submit": "Associa"}, "mapped_toast": "{imported} importate, {matched} abbinate, {skipped} saltate"}
    },
    "queue": {"header": "Coda di importazione", "empty": "Niente da rivedere.", "queued": "In attesa di revisione", "skipped": "Saltate", "failed": "Richiedono attenzione", "reason": {"unmapped": "Carta non associata", "account_deleted": "Conto eliminato", "no_rate": "Nessun tasso di cambio per {currency}"}, "import": "Importa", "skip": "Salta", "unskip": "Ripristina", "retry": "Riprova", "discard": "Scarta", "received": "Ricevuto {date}", "retry_result": {"created": "Importata", "queued": "In coda per la revisione", "skipped": "Saltata", "duplicate": "Già importata", "failed": "Continua a fallire"}},
    "provenance": {"header": "Importata da"}
  },
  "nl": {
    "badge": "Geïmporteerd",
    "banner": {"text": "{count} geïmporteerde transactie wacht op controle | {count} geïmporteerde transacties wachten op controle", "cta": "Controleren"},
    "data_page": {"menu_item": "Import & export", "header": "Import & export", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Leg Apple Pay-betalingen automatisch vast met een iPhone-opdracht. Elke betaling wordt naar Econumo gestuurd; je koppelt elke kaart één keer aan een rekening.",
      "connect": "Apple Wallet instellen", "connected": "Verbonden", "disconnect": "Verbinding verbreken",
      "disconnect_modal": {"title": "Apple Wallet loskoppelen?", "question": "Kaartkoppelingen en de controlewachtrij van deze verbinding worden verwijderd. Al geïmporteerde transacties blijven staan."},
      "steps": {
        "install": {"title": "1. Haal de opdrachten op", "text": "Installeer beide opdrachten op je iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Configureren", "button": "Configureren op deze iPhone", "text": "Maakt een toegangstoken voor de opdracht en geeft het samen met het serveradres door.", "desktop": "Open Instellingen → Import & export op je iPhone om de opdracht daar te configureren.", "done": "De opdracht is geconfigureerd. Je volgende Apple Pay-betaling verschijnt hier."},
        "automate": {"title": "3. Automatiseren", "text": "Voeg in de Opdrachten-app een automatisering toe: Transactie → Voer \"Econumo Wallet\" uit met de transactie als invoer."}
      },
      "manual": {"toggle": "Handmatig configureren", "token": "Toegangstoken (eenmalig getoond)", "url": "Server", "body": "Verzoektekst", "action_1": "Details van transactie ophalen → vul de verzoektekst hierboven in", "action_2": "Inhoud van URL ophalen → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Header Authorization: Bearer <token>", "action_4": "Header Content-Type: application/json", "action_5": "Verzoektekst: JSON, de velden hierboven"},
      "same_named_cards": "Kaarten met dezelfde naam in Wallet komen hier als één kaart binnen.",
      "cards": {"header": "Kaarten", "empty": "Nog geen kaarten — de eerste Apple Pay-betaling voegt er een toe.", "state": {"mapped": "Gekoppeld", "ignored": "Genegeerd", "unmapped": "Niet gekoppeld · {count} in wachtrij"}, "taps": "{count} betaling | {count} betalingen", "last_seen": "Laatst gezien {date}", "map": "Koppelen aan rekening", "map_instead": "Toch koppelen", "ignore": "Negeren", "unlink": "Ontkoppelen", "unlink_modal": {"title": "{card} ontkoppelen?", "question": "Nieuwe betalingen van deze kaart wachten op controle tot je haar opnieuw koppelt."}, "map_modal": {"header": "{card} koppelen", "account": "Rekening", "submit": "Koppelen"}, "mapped_toast": "{imported} geïmporteerd, {matched} gematcht, {skipped} overgeslagen"}
    },
    "queue": {"header": "Importwachtrij", "empty": "Niets te controleren.", "queued": "Wacht op controle", "skipped": "Overgeslagen", "failed": "Aandacht nodig", "reason": {"unmapped": "Kaart niet gekoppeld", "account_deleted": "Rekening verwijderd", "no_rate": "Geen wisselkoers voor {currency}"}, "import": "Importeren", "skip": "Overslaan", "unskip": "Herstellen", "retry": "Opnieuw proberen", "discard": "Verwijderen", "received": "Ontvangen {date}", "retry_result": {"created": "Geïmporteerd", "queued": "In wachtrij voor controle", "skipped": "Overgeslagen", "duplicate": "Al geïmporteerd", "failed": "Mislukt nog steeds"}},
    "provenance": {"header": "Geïmporteerd uit"}
  },
  "pl": {
    "badge": "Zaimportowana",
    "banner": {"text": "{count} zaimportowana transakcja czeka na sprawdzenie | {count} zaimportowane transakcje czekają na sprawdzenie | {count} zaimportowanych transakcji czeka na sprawdzenie", "cta": "Sprawdź"},
    "data_page": {"menu_item": "Import i eksport", "header": "Import i eksport", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Zapisuj płatności Apple Pay automatycznie za pomocą skrótu na iPhonie. Każda płatność trafia do Econumo; każdą kartę przypisujesz do konta tylko raz.",
      "connect": "Skonfiguruj Apple Wallet", "connected": "Połączono", "disconnect": "Rozłącz",
      "disconnect_modal": {"title": "Rozłączyć Apple Wallet?", "question": "Przypisania kart i kolejka do sprawdzenia tego połączenia zostaną usunięte. Zaimportowane już transakcje zostaną."},
      "steps": {
        "install": {"title": "1. Pobierz skróty", "text": "Zainstaluj oba skróty na swoim iPhonie.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Skonfiguruj", "button": "Skonfiguruj na tym iPhonie", "text": "Tworzy token dostępu dla skrótu i przekazuje go razem z adresem serwera.", "desktop": "Otwórz Ustawienia → Import i eksport na swoim iPhonie, aby tam skonfigurować skrót.", "done": "Skrót jest skonfigurowany. Twoja następna płatność Apple Pay pojawi się tutaj."},
        "automate": {"title": "3. Zautomatyzuj", "text": "W aplikacji Skróty dodaj automatyzację: Transakcja → Uruchom „Econumo Wallet” z transakcją jako wejściem."}
      },
      "manual": {"toggle": "Skonfiguruj ręcznie", "token": "Token dostępu (pokazywany raz)", "url": "Serwer", "body": "Treść żądania", "action_1": "Pobierz szczegóły transakcji → wypełnij treść żądania powyżej", "action_2": "Pobierz zawartość URL → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Nagłówek Authorization: Bearer <token>", "action_4": "Nagłówek Content-Type: application/json", "action_5": "Treść żądania: JSON, pola powyżej"},
      "same_named_cards": "Karty o tej samej nazwie w Wallet trafiają tu jako jedna karta.",
      "cards": {"header": "Karty", "empty": "Brak kart — pierwsza płatność Apple Pay doda kartę.", "state": {"mapped": "Przypisana", "ignored": "Ignorowana", "unmapped": "Nieprzypisana · w kolejce: {count}"}, "taps": "{count} płatność | {count} płatności | {count} płatności", "last_seen": "Ostatnio {date}", "map": "Przypisz do konta", "map_instead": "Przypisz mimo to", "ignore": "Ignoruj", "unlink": "Odłącz", "unlink_modal": {"title": "Odłączyć {card}?", "question": "Nowe płatności z tej karty będą czekać na sprawdzenie, dopóki nie przypiszesz jej ponownie."}, "map_modal": {"header": "Przypisz {card}", "account": "Konto", "submit": "Przypisz"}, "mapped_toast": "zaimportowano: {imported}, dopasowano: {matched}, pominięto: {skipped}"}
    },
    "queue": {"header": "Kolejka importu", "empty": "Nic do sprawdzenia.", "queued": "Czeka na sprawdzenie", "skipped": "Pominięte", "failed": "Wymagają uwagi", "reason": {"unmapped": "Karta nieprzypisana", "account_deleted": "Konto usunięte", "no_rate": "Brak kursu dla {currency}"}, "import": "Importuj", "skip": "Pomiń", "unskip": "Przywróć", "retry": "Ponów", "discard": "Odrzuć", "received": "Odebrano {date}", "retry_result": {"created": "Zaimportowano", "queued": "W kolejce do sprawdzenia", "skipped": "Pominięto", "duplicate": "Już zaimportowana", "failed": "Nadal się nie udaje"}},
    "provenance": {"header": "Zaimportowano z"}
  },
  "pt": {
    "badge": "Importada",
    "banner": {"text": "{count} transação importada aguarda revisão | {count} transações importadas aguardam revisão", "cta": "Rever"},
    "data_page": {"menu_item": "Importar e exportar", "header": "Importar e exportar", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Registe pagamentos Apple Pay automaticamente com um atalho do iPhone. Cada pagamento é enviado ao Econumo; associa cada cartão a uma conta uma única vez.",
      "connect": "Configurar Apple Wallet", "connected": "Ligado", "disconnect": "Desligar",
      "disconnect_modal": {"title": "Desligar o Apple Wallet?", "question": "As associações de cartões e a fila de revisão desta ligação serão removidas. As transações já importadas mantêm-se."},
      "steps": {
        "install": {"title": "1. Obter os atalhos", "text": "Instale ambos os atalhos no seu iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Configurar", "button": "Configurar neste iPhone", "text": "Cria um token de acesso para o atalho e entrega-o juntamente com o endereço do servidor.", "desktop": "Abra Definições → Importar e exportar no seu iPhone para configurar o atalho lá.", "done": "O atalho está configurado. O seu próximo pagamento Apple Pay aparecerá aqui."},
        "automate": {"title": "3. Automatizar", "text": "Na app Atalhos adicione uma automatização: Transação → Executar \"Econumo Wallet\" com a transação como entrada."}
      },
      "manual": {"toggle": "Configurar manualmente", "token": "Token de acesso (mostrado uma vez)", "url": "Servidor", "body": "Corpo do pedido", "action_1": "Obter detalhes da transação → preencher o corpo acima", "action_2": "Obter conteúdo do URL → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Cabeçalho Authorization: Bearer <token>", "action_4": "Cabeçalho Content-Type: application/json", "action_5": "Corpo do pedido: JSON, os campos acima"},
      "same_named_cards": "Cartões com o mesmo nome no Wallet chegam aqui como um único cartão.",
      "cards": {"header": "Cartões", "empty": "Ainda sem cartões — o primeiro pagamento Apple Pay adiciona um.", "state": {"mapped": "Associado", "ignored": "Ignorado", "unmapped": "Sem associação · {count} em fila"}, "taps": "{count} pagamento | {count} pagamentos", "last_seen": "Visto pela última vez {date}", "map": "Associar a uma conta", "map_instead": "Associar em vez disso", "ignore": "Ignorar", "unlink": "Desassociar", "unlink_modal": {"title": "Desassociar {card}?", "question": "Novos pagamentos deste cartão aguardarão revisão até o associar novamente."}, "map_modal": {"header": "Associar {card}", "account": "Conta", "submit": "Associar"}, "mapped_toast": "{imported} importadas, {matched} correspondidas, {skipped} ignoradas"}
    },
    "queue": {"header": "Fila de importação", "empty": "Nada para rever.", "queued": "A aguardar revisão", "skipped": "Ignoradas", "failed": "Precisam de atenção", "reason": {"unmapped": "Cartão sem associação", "account_deleted": "Conta eliminada", "no_rate": "Sem taxa de câmbio para {currency}"}, "import": "Importar", "skip": "Ignorar", "unskip": "Repor", "retry": "Tentar novamente", "discard": "Descartar", "received": "Recebido {date}", "retry_result": {"created": "Importada", "queued": "Em fila para revisão", "skipped": "Ignorada", "duplicate": "Já importada", "failed": "Continua a falhar"}},
    "provenance": {"header": "Importada de"}
  },
  "ru": {
    "badge": "Импортирована",
    "banner": {"text": "{count} импортированная транзакция ждёт проверки | {count} импортированные транзакции ждут проверки | {count} импортированных транзакций ждут проверки", "cta": "Проверить"},
    "data_page": {"menu_item": "Импорт и экспорт", "header": "Импорт и экспорт", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Записывайте оплаты Apple Pay автоматически с помощью команды на iPhone. Каждая оплата отправляется в Econumo; каждую карту вы привязываете к счёту один раз.",
      "connect": "Настроить Apple Wallet", "connected": "Подключено", "disconnect": "Отключить",
      "disconnect_modal": {"title": "Отключить Apple Wallet?", "question": "Привязки карт и очередь проверки этого подключения будут удалены. Уже импортированные транзакции останутся."},
      "steps": {
        "install": {"title": "1. Установите команды", "text": "Установите обе команды на iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Настройте", "button": "Настроить на этом iPhone", "text": "Создаёт токен доступа для команды и передаёт его вместе с адресом сервера.", "desktop": "Откройте Настройки → Импорт и экспорт на iPhone, чтобы настроить команду там.", "done": "Команда настроена. Ваша следующая оплата Apple Pay появится здесь."},
        "automate": {"title": "3. Автоматизируйте", "text": "В приложении «Команды» добавьте автоматизацию: Транзакция → Запустить «Econumo Wallet», передав транзакцию на вход."}
      },
      "manual": {"toggle": "Настроить вручную", "token": "Токен доступа (показывается один раз)", "url": "Сервер", "body": "Тело запроса", "action_1": "Получить сведения о транзакции → заполнить тело запроса выше", "action_2": "Получить содержимое URL → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Заголовок Authorization: Bearer <token>", "action_4": "Заголовок Content-Type: application/json", "action_5": "Тело запроса: JSON, поля выше"},
      "same_named_cards": "Карты с одинаковым именем в Wallet приходят сюда как одна карта.",
      "cards": {"header": "Карты", "empty": "Карт пока нет — первая оплата Apple Pay добавит карту.", "state": {"mapped": "Привязана", "ignored": "Игнорируется", "unmapped": "Не привязана · в очереди: {count}"}, "taps": "{count} оплата | {count} оплаты | {count} оплат", "last_seen": "Последняя оплата {date}", "map": "Привязать к счёту", "map_instead": "Всё же привязать", "ignore": "Игнорировать", "unlink": "Отвязать", "unlink_modal": {"title": "Отвязать {card}?", "question": "Новые оплаты с этой карты будут ждать проверки, пока вы не привяжете её снова."}, "map_modal": {"header": "Привязать {card}", "account": "Счёт", "submit": "Привязать"}, "mapped_toast": "импортировано: {imported}, сопоставлено: {matched}, пропущено: {skipped}"}
    },
    "queue": {"header": "Очередь импорта", "empty": "Нечего проверять.", "queued": "Ждут проверки", "skipped": "Пропущенные", "failed": "Требуют внимания", "reason": {"unmapped": "Карта не привязана", "account_deleted": "Счёт удалён", "no_rate": "Нет курса для {currency}"}, "import": "Импортировать", "skip": "Пропустить", "unskip": "Вернуть", "retry": "Повторить", "discard": "Удалить", "received": "Получено {date}", "retry_result": {"created": "Импортирована", "queued": "В очереди на проверку", "skipped": "Пропущена", "duplicate": "Уже импортирована", "failed": "По-прежнему ошибка"}},
    "provenance": {"header": "Импортирована из"}
  },
  "uk": {
    "badge": "Імпортована",
    "banner": {"text": "{count} імпортована транзакція чекає на перевірку | {count} імпортовані транзакції чекають на перевірку | {count} імпортованих транзакцій чекають на перевірку", "cta": "Перевірити"},
    "data_page": {"menu_item": "Імпорт та експорт", "header": "Імпорт та експорт", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "Записуйте оплати Apple Pay автоматично за допомогою швидкої команди на iPhone. Кожна оплата надсилається в Econumo; кожну картку ви прив'язуєте до рахунку один раз.",
      "connect": "Налаштувати Apple Wallet", "connected": "Підключено", "disconnect": "Відключити",
      "disconnect_modal": {"title": "Відключити Apple Wallet?", "question": "Прив'язки карток і чергу перевірки цього підключення буде видалено. Уже імпортовані транзакції залишаться."},
      "steps": {
        "install": {"title": "1. Встановіть команди", "text": "Встановіть обидві команди на iPhone.", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. Налаштуйте", "button": "Налаштувати на цьому iPhone", "text": "Створює токен доступу для команди й передає його разом з адресою сервера.", "desktop": "Відкрийте Налаштування → Імпорт та експорт на iPhone, щоб налаштувати команду там.", "done": "Команду налаштовано. Ваша наступна оплата Apple Pay з'явиться тут."},
        "automate": {"title": "3. Автоматизуйте", "text": "У застосунку «Швидкі команди» додайте автоматизацію: Транзакція → Запустити «Econumo Wallet», передавши транзакцію на вхід."}
      },
      "manual": {"toggle": "Налаштувати вручну", "token": "Токен доступу (показується один раз)", "url": "Сервер", "body": "Тіло запиту", "action_1": "Отримати відомості про транзакцію → заповнити тіло запиту вище", "action_2": "Отримати вміст URL → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "Заголовок Authorization: Bearer <token>", "action_4": "Заголовок Content-Type: application/json", "action_5": "Тіло запиту: JSON, поля вище"},
      "same_named_cards": "Картки з однаковою назвою у Wallet надходять сюди як одна картка.",
      "cards": {"header": "Картки", "empty": "Карток ще немає — перша оплата Apple Pay додасть картку.", "state": {"mapped": "Прив'язана", "ignored": "Ігнорується", "unmapped": "Не прив'язана · у черзі: {count}"}, "taps": "{count} оплата | {count} оплати | {count} оплат", "last_seen": "Остання оплата {date}", "map": "Прив'язати до рахунку", "map_instead": "Усе ж прив'язати", "ignore": "Ігнорувати", "unlink": "Відв'язати", "unlink_modal": {"title": "Відв'язати {card}?", "question": "Нові оплати з цієї картки чекатимуть на перевірку, доки ви не прив'яжете її знову."}, "map_modal": {"header": "Прив'язати {card}", "account": "Рахунок", "submit": "Прив'язати"}, "mapped_toast": "імпортовано: {imported}, зіставлено: {matched}, пропущено: {skipped}"}
    },
    "queue": {"header": "Черга імпорту", "empty": "Нічого перевіряти.", "queued": "Чекають на перевірку", "skipped": "Пропущені", "failed": "Потребують уваги", "reason": {"unmapped": "Картка не прив'язана", "account_deleted": "Рахунок видалено", "no_rate": "Немає курсу для {currency}"}, "import": "Імпортувати", "skip": "Пропустити", "unskip": "Повернути", "retry": "Повторити", "discard": "Видалити", "received": "Отримано {date}", "retry_result": {"created": "Імпортована", "queued": "У черзі на перевірку", "skipped": "Пропущена", "duplicate": "Уже імпортована", "failed": "Досі помилка"}},
    "provenance": {"header": "Імпортована з"}
  },
  "zh": {
    "badge": "已导入",
    "banner": {"text": "{count} 笔导入的交易等待审核 | {count} 笔导入的交易等待审核", "cta": "审核"},
    "data_page": {"menu_item": "导入与导出", "header": "导入与导出", "csv": {"header": "CSV"}},
    "apple_wallet": {
      "header": "Apple Wallet",
      "intro": "通过 iPhone 快捷指令自动记录 Apple Pay 付款。每笔付款都会发送到 Econumo；每张卡只需映射到账户一次。",
      "connect": "设置 Apple Wallet", "connected": "已连接", "disconnect": "断开连接",
      "disconnect_modal": {"title": "断开 Apple Wallet？", "question": "此连接的卡片映射和审核队列将被删除。已导入的交易会保留。"},
      "steps": {
        "install": {"title": "1. 获取快捷指令", "text": "在 iPhone 上安装这两个快捷指令。", "wallet": "Econumo Wallet", "setup": "Econumo Setup"},
        "configure": {"title": "2. 配置", "button": "在此 iPhone 上配置", "text": "为快捷指令创建访问令牌，并连同服务器地址一起交给它。", "desktop": "在 iPhone 上打开 设置 → 导入与导出，在那里配置快捷指令。", "done": "快捷指令已配置。您的下一笔 Apple Pay 付款将显示在这里。"},
        "automate": {"title": "3. 自动化", "text": "在“快捷指令”应用中添加自动化：交易 → 以该交易为输入运行“Econumo Wallet”。"}
      },
      "manual": {"toggle": "手动配置", "token": "访问令牌（仅显示一次）", "url": "服务器", "body": "请求正文", "action_1": "获取交易详情 → 填写上方的请求正文", "action_2": "获取 URL 内容 → POST {url}/api/v1/import/ingest-apple-wallet-event", "action_3": "标头 Authorization: Bearer <token>", "action_4": "标头 Content-Type: application/json", "action_5": "请求正文：JSON，上述字段"},
      "same_named_cards": "Wallet 中同名的卡片在这里会显示为一张卡。",
      "cards": {"header": "卡片", "empty": "还没有卡片 — 第一笔 Apple Pay 付款会添加一张。", "state": {"mapped": "已映射", "ignored": "已忽略", "unmapped": "未映射 · {count} 笔排队中"}, "taps": "{count} 笔付款 | {count} 笔付款", "last_seen": "最近使用 {date}", "map": "映射到账户", "map_instead": "改为映射", "ignore": "忽略", "unlink": "取消映射", "unlink_modal": {"title": "取消映射 {card}？", "question": "在您重新映射之前，这张卡的新付款将等待审核。"}, "map_modal": {"header": "映射 {card}", "account": "账户", "submit": "映射"}, "mapped_toast": "已导入 {imported}，已匹配 {matched}，已跳过 {skipped}"}
    },
    "queue": {"header": "导入队列", "empty": "没有需要审核的内容。", "queued": "等待审核", "skipped": "已跳过", "failed": "需要处理", "reason": {"unmapped": "卡片未映射", "account_deleted": "账户已删除", "no_rate": "没有 {currency} 的汇率"}, "import": "导入", "skip": "跳过", "unskip": "恢复", "retry": "重试", "discard": "丢弃", "received": "接收于 {date}", "retry_result": {"created": "已导入", "queued": "已排队等待审核", "skipped": "已跳过", "duplicate": "已导入过", "failed": "仍然失败"}},
    "provenance": {"header": "导入自"}
  },
}
TRANSLATIONS["en"] = EN


def leaves(tree, prefix=""):
    for k, v in tree.items():
        key = f"{prefix}.{k}" if prefix else k
        if isinstance(v, dict):
            yield from leaves(v, key)
        else:
            yield key, v


root = pathlib.Path(__file__).resolve().parent.parent / "locales"
en_leaves = dict(leaves(EN))
for lang, tree in TRANSLATIONS.items():
    got = dict(leaves(tree))
    missing = sorted(set(en_leaves) - set(got))
    extra = sorted(set(got) - set(en_leaves))
    if missing or extra:
        raise SystemExit(f"{lang}: missing={missing} extra={extra}")
    path = root / f"{lang}.json"
    data = json.loads(path.read_text(encoding="utf-8"))
    if "imports" in data:
        raise SystemExit(f"{lang}: imports namespace already present")
    # keep the namespace right after `recurring` so the diff reads top-down like the UI
    out = {}
    for k, v in data.items():
        out[k] = v
        if k == "recurring":
            out["imports"] = tree
    if "imports" not in out:
        out["imports"] = tree
    path.write_text(json.dumps(out, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"{lang}: +{len(got)} keys")
```

Before running, confirm the existing files serialize the same way the script writes them (2-space indent, `ensure_ascii=False`, trailing newline) so the diff is only the new block:

Run: `python3 -c "import json,pathlib; p=pathlib.Path('locales/en.json'); s=p.read_text(); print(json.dumps(json.loads(s), ensure_ascii=False, indent=2)+'\n' == s)"`
Expected: `True`. If it prints `False`, inspect `git diff --stat locales/` after running the script and, if anything beyond the `imports` block changed, restore the files (`git checkout locales/`) and adapt the writer to the file's actual formatting before rerunning.

Run: `python3 scripts/add-imports-locale-keys.py && git diff --stat locales/ && rm scripts/add-imports-locale-keys.py`
Expected: 11 files changed, each with the same number of added lines and no removed lines.

- [ ] **Step 11: i18n guards**

Run: `go test ./internal/test/i18ntest/`
Expected: PASS — key parity across languages, placeholder parity (`{count}`, `{date}`, `{card}`, `{currency}`, `{url}`, `{imported}`, `{matched}`, `{skipped}` appear in every language's copy of the same key), and the frontend-coverage guard still passes because no `t('imports.…')` call exists yet (Tasks 9 and 10 add them; the guard checks that every `t()` key exists in the catalogue, not the reverse).

- [ ] **Step 12: Lint, type-check, full frontend suite**

Run: `(cd web && pnpm lint && pnpm exec tsc -b && pnpm test)`
Expected: lint clean, type-check clean, tests PASS except the pre-existing `exportTransactionList … resolves a Blob` failure (unrelated, present before this branch).

- [ ] **Step 13: Commit**

```bash
git add web/src/api/dto/imports.ts web/src/api/imports.ts web/src/features/imports web/src/lib/platform.ts web/src/lib/platform.test.ts web/src/lib/config.ts web/public/econumo-config.js web/src/app/queryKeys.ts web/src/app/router-pages.ts web/src/lib/metrics.ts web/src/api/user.ts web/src/features/settings/security.ts web/src/features/transactions/queries.ts locales/
git commit -m "feat(web): import API client, hooks, metrics, i18n"
```

---

### Task 9: Settings → Import & export page: CSV, Apple Wallet setup, card list

**Files:**
- Create: `web/src/features/imports/ImportsDataPage.tsx`
- Create: `web/src/features/imports/AppleWalletSetup.tsx`
- Create: `web/src/features/imports/ImportCards.tsx`
- Create: `web/src/features/imports/ImportsDataPage.test.tsx`
- Create: `web/src/features/imports/AppleWalletSetup.test.tsx`
- Create: `web/src/features/imports/ImportCards.test.tsx`
- Modify: `web/src/features/settings/SettingsPage.tsx` (drop the CSV state/dialogs, the Data group becomes one link row)
- Modify: `web/src/features/settings/SettingsPage.test.tsx:90-96` (the CSV test moves to `ImportsDataPage.test.tsx`)
- Modify: `web/src/app/routes.tsx` (route `/settings/data`)
- Modify: `web/src/lib/metrics.ts` (two keys fired by `AppleWalletSetup`)
- Modify: `web/src/test/fixtures.ts` (`coreHandlers` gains `importSources` + the `get-source-list` handler)

**Interfaces:**
- Consumes (Task 8): `useImportSources`, `useCreateImportSource`, `useDeleteImportSource`, `useLinkImportAccount`, `useIgnoreImportAccount`, `useUnlinkImportAccount`, `useCreatePersonalToken` (`{ name, expiresAt, scope: 'ingest' }`), `isIOS()`, `RouterPage.SETTINGS_DATA`, the `imports.*` catalogue keys, `ImportSourceDto` / `ImportCardDto`.
- Produces: `METRICS.IMPORT_SHORTCUT_DOWNLOAD` (`'appImportShortcutDownload'`), `METRICS.IMPORT_SHORTCUT_CONFIGURE` (`'appImportShortcutConfigure'`); `coreHandlers({ importSources })` for later page tests.

Shortcut files are NOT part of this task: the page links to `/shortcuts/econumo-wallet-v1.shortcut` and `/shortcuts/econumo-setup-v1.shortcut` under `web/public/`, which the maintainer drops in by hand later (signed `.shortcut` bundles cannot be generated here). Until then those links 404 — the recipe in Task 11's `web/shortcuts/README.md` is the contract they must implement.

- [ ] **Step 1: Metrics keys**

`web/src/lib/metrics.ts` — after `IMPORT_QUEUE_SKIP`, add:

```ts
  IMPORT_SHORTCUT_DOWNLOAD: 'appImportShortcutDownload',
  IMPORT_SHORTCUT_CONFIGURE: 'appImportShortcutConfigure',
```

- [ ] **Step 2: Test fixtures**

`web/src/test/fixtures.ts` — in `coreHandlers`, add `importSources: [] as unknown[]` to the `data` object (after `recurring: [],`) and one handler to the returned array:

```ts
    http.get('*/api/v1/import/get-source-list', () => envelope({ items: data.importSources })),
```

- [ ] **Step 3: Page test (red)**

Create `web/src/features/imports/ImportsDataPage.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportsDataPage } from './ImportsDataPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const wireSource = {
  id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00',
  cards: [{ externalAccountId: 'wallet', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 2, tapCount: 3, lastSeenAt: '2026-08-20 17:42:03' }],
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/data', element: <ImportsDataPage /> }], { initialEntries: ['/settings/data'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('renders the CSV rows and opens the import dialog', async () => {
  server.use(...coreHandlers())
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Import & export')).toBeInTheDocument()
  expect(screen.getByText('Export CSV')).toBeInTheDocument()
  await user.click(screen.getByText('Import CSV'))
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
})

it('offers Apple Wallet setup when no source exists', async () => {
  server.use(...coreHandlers())
  renderPage()
  expect(await screen.findByText('Apple Wallet')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Set up Apple Wallet' })).toBeInTheDocument()
  expect(screen.queryByText('Cards')).not.toBeInTheDocument()
})

it('shows the connected state and the card list when a source exists', async () => {
  server.use(...coreHandlers({ importSources: [wireSource] }))
  renderPage()
  expect(await screen.findByText('Connected')).toBeInTheDocument()
  expect(screen.getByText('Apple Card')).toBeInTheDocument()
  expect(screen.getByText('Unmapped · 2 queued')).toBeInTheDocument()
  expect(screen.getByText('3 taps')).toBeInTheDocument()
})
```

Run: `(cd web && pnpm vitest run src/features/imports/ImportsDataPage.test.tsx)`
Expected: FAIL — `ImportsDataPage` module not found.

- [ ] **Step 4: The page**

Create `web/src/features/imports/ImportsDataPage.tsx`. It owns the CSV dialogs that used to live on the settings hub, then the Apple Wallet section:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { ExportCsvDialog } from '@/features/transactions/ExportCsvDialog'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import type { AggregatedImportResult } from '@/features/transactions/importCsv'
import { AppleWalletSetup } from './AppleWalletSetup'
import { ImportCards } from './ImportCards'
import { useImportSources } from './queries'

function ActionRow({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="flex w-full items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-left text-sm hover:bg-econumo-hover">
      <span>{label}</span>
      <ChevronRight className="size-4 text-muted-foreground" />
    </button>
  )
}

export function ImportsDataPage() {
  const { t } = useTranslation()
  const [exportOpen, setExportOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importResult, setImportResult] = useState<AggregatedImportResult | null>(null)
  const { data: sources = [] } = useImportSources()
  const wallet = sources.find((s) => s.provider === 'apple-wallet') ?? null

  return (
    <SettingsShell
      title={t('imports.data_page.header')}
      backTo={RouterPage.SETTINGS}
    >
      <div className="mx-auto flex w-full max-w-xl flex-col gap-6">
        <section className="flex flex-col gap-2">
          <p className="px-1 text-xs uppercase text-muted-foreground">{t('imports.data_page.csv.header')}</p>
          <ActionRow label={t('settings.import_csv.menu_item')} onClick={() => setImportOpen(true)} />
          <ActionRow label={t('settings.export_csv.menu_item')} onClick={() => setExportOpen(true)} />
        </section>

        <section className="flex flex-col gap-2">
          <p className="px-1 text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.header')}</p>
          <AppleWalletSetup source={wallet} />
          {wallet ? <ImportCards source={wallet} /> : null}
        </section>
      </div>

      <ExportCsvDialog open={exportOpen} onClose={() => setExportOpen(false)} />
      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
    </SettingsShell>
  )
}
```

`RecurringSettingsPage.tsx` is the reference for the shell usage (`title` + `backTo={RouterPage.SETTINGS}`, no crumbs); copy whatever extra props it passes (e.g. `heading`) so the two pages look alike.

- [ ] **Step 5: Apple Wallet setup component**

Create `web/src/features/imports/AppleWalletSetup.tsx`:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy } from 'lucide-react'
import type { ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { Button } from '@/components/ui/button'
import { useCreatePersonalToken } from '@/features/settings/security'
import { copyText } from '@/lib/clipboard'
import { backendHost } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isIOS } from '@/lib/platform'
import { useCreateImportSource, useDeleteImportSource } from './queries'

export const WALLET_SHORTCUT_URL = '/shortcuts/econumo-wallet-v1.shortcut'
export const SETUP_SHORTCUT_URL = '/shortcuts/econumo-setup-v1.shortcut'
const INGEST_TOKEN_NAME = 'Apple Wallet'

// The Setup shortcut takes one text input: JSON with the server URL and the
// ingest token. Encoded once here so the deep link and the manual recipe
// never disagree about the shape.
export function setupDeepLink(url: string, token: string): string {
  const input = encodeURIComponent(JSON.stringify({ url, token }))
  return `shortcuts://run-shortcut?name=Econumo%20Setup&input=text&text=${input}`
}

const REQUEST_BODY = `{
  "account": "<Card name>",
  "payee": "<Merchant>",
  "amount": "<Amount>",
  "currency": "<Currency code>",
  "occurredAt": "<Date, ISO 8601>",
  "eventId": "<Transaction identifier>"
}`

export function AppleWalletSetup({ source }: { source: ImportSourceDto | null }) {
  const { t } = useTranslation()
  const createSource = useCreateImportSource()
  const deleteSource = useDeleteImportSource()
  const createToken = useCreatePersonalToken()
  const [disconnectOpen, setDisconnectOpen] = useState(false)
  const [manualOpen, setManualOpen] = useState(false)
  // The raw token is shown exactly once (the API never returns it again), so
  // it lives only in component state and dies with the page.
  const [token, setToken] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [configured, setConfigured] = useState(false)
  const serverUrl = backendHost()

  const mintToken = async (): Promise<string> => {
    const created = await createToken.mutateAsync({ name: INGEST_TOKEN_NAME, expiresAt: null, scope: 'ingest' })
    setToken(created.token)
    return created.token
  }

  const configureHere = async () => {
    const value = token ?? (await mintToken())
    trackEvent(METRICS.IMPORT_SHORTCUT_CONFIGURE)
    setConfigured(true)
    window.location.href = setupDeepLink(serverUrl, value)
  }

  const revealManual = async () => {
    setManualOpen(true)
    if (!token) {
      await mintToken()
    }
  }

  if (!source) {
    return (
      <div className="flex flex-col gap-3 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
        <p className="text-muted-foreground">{t('imports.apple_wallet.intro')}</p>
        <Button type="button" className="h-11" disabled={createSource.isPending} onClick={() => createSource.mutate({ provider: 'apple-wallet', name: 'iPhone' })}>
          {t('imports.apple_wallet.connect')}
        </Button>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{t('imports.apple_wallet.connected')}</span>
        <Button type="button" variant="ghost" size="sm" onClick={() => setDisconnectOpen(true)}>
          {t('imports.apple_wallet.disconnect')}
        </Button>
      </div>

      <ol className="flex flex-col gap-4">
        <li className="flex flex-col gap-1">
          <span className="font-medium">{t('imports.apple_wallet.steps.install.title')}</span>
          <span className="text-muted-foreground">{t('imports.apple_wallet.steps.install.text')}</span>
          <div className="flex flex-wrap gap-2 pt-1">
            <a href={WALLET_SHORTCUT_URL} download onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'wallet' })} className="rounded-md border px-3 py-2">
              {t('imports.apple_wallet.steps.install.wallet')}
            </a>
            <a href={SETUP_SHORTCUT_URL} download onClick={() => trackEvent(METRICS.IMPORT_SHORTCUT_DOWNLOAD, { shortcut: 'setup' })} className="rounded-md border px-3 py-2">
              {t('imports.apple_wallet.steps.install.setup')}
            </a>
          </div>
        </li>

        <li className="flex flex-col gap-1">
          <span className="font-medium">{t('imports.apple_wallet.steps.configure.title')}</span>
          {isIOS() ? (
            <>
              <span className="text-muted-foreground">{t('imports.apple_wallet.steps.configure.text')}</span>
              <Button type="button" className="mt-1 h-11" disabled={createToken.isPending} onClick={() => void configureHere()}>
                {t('imports.apple_wallet.steps.configure.button')}
              </Button>
              {configured ? <InfoBox>{t('imports.apple_wallet.steps.configure.done')}</InfoBox> : null}
            </>
          ) : (
            <span className="text-muted-foreground">{t('imports.apple_wallet.steps.configure.desktop')}</span>
          )}
          <button type="button" className="self-start text-left text-primary underline-offset-2 hover:underline" onClick={() => void revealManual()}>
            {t('imports.apple_wallet.manual.toggle')}
          </button>
          {manualOpen ? (
            <div className="mt-2 flex flex-col gap-2 rounded-md border p-3">
              <label className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.manual.token')}</label>
              <div className="flex items-center gap-2">
                <code className="min-w-0 flex-1 break-all text-xs">{token ?? '…'}</code>
                {token ? (
                  <Button type="button" variant="ghost" size="icon" aria-label={t('user.page.settings.profile.tokens.create.label')} onClick={() => void copyText(token).then(setCopied)}>
                    {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                  </Button>
                ) : null}
              </div>
              <label className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.manual.url')}</label>
              <code className="break-all text-xs">{serverUrl}</code>
              <label className="text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.manual.body')}</label>
              <pre className="overflow-x-auto text-xs">{REQUEST_BODY}</pre>
              <ol className="list-decimal pl-4 text-muted-foreground">
                <li>{t('imports.apple_wallet.manual.action_1')}</li>
                <li>{t('imports.apple_wallet.manual.action_2', { url: serverUrl })}</li>
                <li>{t('imports.apple_wallet.manual.action_3')}</li>
                <li>{t('imports.apple_wallet.manual.action_4')}</li>
                <li>{t('imports.apple_wallet.manual.action_5')}</li>
              </ol>
            </div>
          ) : null}
        </li>

        <li className="flex flex-col gap-1">
          <span className="font-medium">{t('imports.apple_wallet.steps.automate.title')}</span>
          <span className="text-muted-foreground">{t('imports.apple_wallet.steps.automate.text')}</span>
        </li>
      </ol>

      <InfoBox>{t('imports.apple_wallet.same_named_cards')}</InfoBox>

      <ConfirmDialog
        open={disconnectOpen}
        onClose={() => setDisconnectOpen(false)}
        onConfirm={() => {
          setDisconnectOpen(false)
          deleteSource.mutate(source.id)
        }}
        title={t('imports.apple_wallet.disconnect_modal.title')}
        question={t('imports.apple_wallet.disconnect_modal.question')}
        confirmLabel={t('imports.apple_wallet.disconnect')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
```

The copy button reuses the aria-label the PAT page uses for its own copy button (`web/src/features/settings/PersonalTokensPage.tsx:109`); `common.button.cancel.label` exists.

- [ ] **Step 6: Setup component test**

Create `web/src/features/imports/AppleWalletSetup.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { AppleWalletSetup, setupDeepLink } from './AppleWalletSetup'

const mockIsIOS = vi.hoisted(() => ({ value: false }))
vi.mock('@/lib/platform', () => ({ isIOS: () => mockIsIOS.value, isNativeApp: () => false }))

const source = { id: 's1', provider: 'apple-wallet' as const, name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [] }

function renderSetup(src: typeof source | null) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AppleWalletSetup source={src} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockIsIOS.value = false
})

it('setupDeepLink encodes the JSON input for the Setup shortcut', () => {
  expect(setupDeepLink('https://eco.example', 'eco_pat_x')).toBe(
    'shortcuts://run-shortcut?name=Econumo%20Setup&input=text&text=' + encodeURIComponent('{"url":"https://eco.example","token":"eco_pat_x"}'),
  )
})

it('connect posts create-source', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/create-source', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { item: source } })
  }))
  const user = userEvent.setup()
  renderSetup(null)
  await user.click(screen.getByRole('button', { name: 'Set up Apple Wallet' }))
  await waitFor(() => expect(body).toEqual({ provider: 'apple-wallet', name: 'iPhone' }))
})

it('desktop shows the iPhone hint instead of the configure button', () => {
  renderSetup(source)
  expect(screen.getByText('Open Settings → Import & export on your iPhone to configure the Shortcut there.')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Configure on this iPhone' })).not.toBeInTheDocument()
})

it('iOS configure mints an ingest token and opens the shortcuts deep link', async () => {
  mockIsIOS.value = true
  let body: unknown
  server.use(http.post('*/api/v1/user/create-personal-token', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { id: 'p1', name: 'Apple Wallet', token: 'eco_pat_new', createdAt: '2026-08-01 00:00:00', expiresAt: null } })
  }))
  const assigned: string[] = []
  const original = window.location
  Object.defineProperty(window, 'location', { configurable: true, value: { ...original, set href(v: string) { assigned.push(v) } } })
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Configure on this iPhone' }))
  await waitFor(() => expect(assigned).toHaveLength(1))
  expect(body).toEqual({ name: 'Apple Wallet', scope: 'ingest', expiresAt: '' })
  expect(assigned[0].startsWith('shortcuts://run-shortcut?name=Econumo%20Setup&input=text&text=')).toBe(true)
  expect(decodeURIComponent(assigned[0].split('text=')[1])).toContain('"token":"eco_pat_new"')
  Object.defineProperty(window, 'location', { configurable: true, value: original })
})

it('manual recipe reveals the token once, with the server URL and the request body', async () => {
  server.use(http.post('*/api/v1/user/create-personal-token', () =>
    HttpResponse.json({ success: true, message: '', data: { id: 'p1', name: 'Apple Wallet', token: 'eco_pat_manual', createdAt: '2026-08-01 00:00:00', expiresAt: null } })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByText('Configure manually'))
  expect(await screen.findByText('eco_pat_manual')).toBeInTheDocument()
  expect(screen.getByText(/ingest-apple-wallet-event/)).toBeInTheDocument()
  expect(screen.getByText(/"occurredAt"/)).toBeInTheDocument()
})
```

If jsdom refuses the `window.location` redefinition in the iOS test, replace the assignment in the component with a tiny injectable: export `let openDeepLink = (url: string) => { window.location.href = url }` from `AppleWalletSetup.tsx`, call it from `configureHere`, and `vi.spyOn` it in the test instead.

Run: `(cd web && pnpm vitest run src/features/imports/AppleWalletSetup.test.tsx)`
Expected: PASS (5 tests).

- [ ] **Step 7: Card list component**

Create `web/src/features/imports/ImportCards.tsx`:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportCardDto, ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'
import { dayKey, formatDayHeading } from '@/lib/datetime'
import { pluralPick } from '@/lib/plural'
import { useIgnoreImportAccount, useLinkImportAccount, useUnlinkImportAccount } from './queries'

export function ImportCards({ source }: { source: ImportSourceDto }) {
  const { t, i18n } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const { data: user } = useUserData()
  const link = useLinkImportAccount()
  const ignore = useIgnoreImportAccount()
  const unlink = useUnlinkImportAccount()
  const [mapTarget, setMapTarget] = useState<ImportCardDto | null>(null)
  const [unlinkTarget, setUnlinkTarget] = useState<ImportCardDto | null>(null)
  const [accountId, setAccountId] = useState('')
  // link-account requires ownership (the backend rejects shared accounts), so
  // the picker lists only my own accounts, in the card's currency first
  const ownAccounts = accounts.filter((a) => a.owner.id === user?.id)
  const accountName = (id: string) => accounts.find((a) => a.id === id)?.name ?? ''

  const submitMap = () => {
    if (!mapTarget || !accountId) {
      return
    }
    const card = mapTarget
    link.mutate(
      { sourceId: source.id, externalAccountId: card.externalAccountId, accountId },
      {
        onSuccess: (result) => {
          setMapTarget(null)
          setAccountId('')
          if (result.run) {
            toast.success(t('imports.apple_wallet.cards.mapped_toast', {
              imported: result.run.importedCount, matched: result.run.matchedCount, skipped: result.run.skippedCount,
            }))
          }
        },
      },
    )
  }

  const stateLabel = (card: ImportCardDto) =>
    card.state === 'mapped'
      ? `${t('imports.apple_wallet.cards.state.mapped')} · ${accountName(card.accountId)}`
      : card.state === 'ignored'
        ? t('imports.apple_wallet.cards.state.ignored')
        : t('imports.apple_wallet.cards.state.unmapped', { count: card.queuedCount })

  return (
    <div className="flex flex-col gap-2">
      <p className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.cards.header')}</p>
      {source.cards.length === 0 ? (
        <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.apple_wallet.cards.empty')}</p>
      ) : (
        source.cards.map((card) => (
          <div key={card.externalAccountId} className="flex flex-col gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
            <div className="flex items-center justify-between gap-2">
              <div className="min-w-0">
                <div className="truncate font-medium">{card.externalName}</div>
                <div className="text-xs text-muted-foreground">{stateLabel(card)}</div>
              </div>
              <div className="shrink-0 text-right text-xs text-muted-foreground">
                <div>{pluralPick(t('imports.apple_wallet.cards.taps'), card.tapCount, i18n.language)}</div>
                {card.lastSeenAt ? <div>{t('imports.apple_wallet.cards.last_seen', { date: formatDayHeading(dayKey(card.lastSeenAt), i18n.language) })}</div> : null}
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {card.state === 'unmapped' ? (
                <>
                  <Button type="button" size="sm" onClick={() => setMapTarget(card)}>{t('imports.apple_wallet.cards.map')}</Button>
                  <Button type="button" size="sm" variant="secondary" disabled={ignore.isPending} onClick={() => ignore.mutate({ sourceId: source.id, externalAccountId: card.externalAccountId })}>
                    {t('imports.apple_wallet.cards.ignore')}
                  </Button>
                </>
              ) : card.state === 'ignored' ? (
                <Button type="button" size="sm" variant="secondary" onClick={() => setMapTarget(card)}>{t('imports.apple_wallet.cards.map_instead')}</Button>
              ) : (
                <Button type="button" size="sm" variant="secondary" onClick={() => setUnlinkTarget(card)}>{t('imports.apple_wallet.cards.unlink')}</Button>
              )}
            </div>
          </div>
        ))
      )}

      <ResponsiveDialog
        open={mapTarget !== null}
        onOpenChange={(o) => !o && setMapTarget(null)}
        title={t('imports.apple_wallet.cards.map_modal.header', { card: mapTarget?.externalName ?? '' })}
      >
        <div className="flex flex-col gap-3">
          <label className="text-xs uppercase text-muted-foreground" htmlFor="import-map-account">{t('imports.apple_wallet.cards.map_modal.account')}</label>
          <select id="import-map-account" className="h-11 w-full rounded-md border bg-transparent px-2 text-sm" value={accountId} onChange={(e) => setAccountId(e.target.value)}>
            <option value="" />
            {ownAccounts.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={() => setMapTarget(null)}>{t('common.button.cancel.label')}</Button>
            <Button type="button" disabled={!accountId || link.isPending} onClick={submitMap}>{t('imports.apple_wallet.cards.map_modal.submit')}</Button>
          </div>
        </div>
      </ResponsiveDialog>

      <ConfirmDialog
        open={unlinkTarget !== null}
        onClose={() => setUnlinkTarget(null)}
        onConfirm={() => {
          const card = unlinkTarget
          setUnlinkTarget(null)
          if (card) {
            unlink.mutate({ sourceId: source.id, externalAccountId: card.externalAccountId })
          }
        }}
        title={t('imports.apple_wallet.cards.unlink_modal.title', { card: unlinkTarget?.externalName ?? '' })}
        question={t('imports.apple_wallet.cards.unlink_modal.question')}
        confirmLabel={t('imports.apple_wallet.cards.unlink')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
```

A mapping error (e.g. `import.currency_mismatch`, a coded 400) surfaces through the app's global mutation error toast the way every other mutation does — check `web/src/app/providers.tsx` (or wherever the `QueryClient` is built) for the `MutationCache.onError` → `apiErrorMessage` hook; if the project has none, add `onError: (err) => toast.error(apiErrorMessage(err))` to `link.mutate`'s options here.

- [ ] **Step 8: Card list test**

Create `web/src/features/imports/ImportCards.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts } from '@/test/fixtures'
import type { ImportSourceDto } from '@/api/dto/imports'
import { ImportCards } from './ImportCards'

const unmapped = { externalAccountId: 'wallet', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped' as const, accountId: '', queuedCount: 2, tapCount: 3, lastSeenAt: '2026-08-20 17:42:03' }
const source: ImportSourceDto = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [unmapped] }

function renderCards(src: ImportSourceDto) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportCards source={src} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
})

it('map opens the account picker and posts link-account, then toasts the run counts', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/link-account', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: {
      item: { ...source, cards: [{ ...unmapped, state: 'mapped', accountId: fixtureAccounts[0].id, queuedCount: 0 }] },
      run: { id: 'r1', status: 'finished', importedCount: 1, matchedCount: 1, skippedCount: 0, failedCount: 0 },
    } })
  }))
  const user = userEvent.setup()
  renderCards(source)
  await user.click(await screen.findByRole('button', { name: 'Map to account' }))
  await user.selectOptions(await screen.findByLabelText('Account'), fixtureAccounts[0].id)
  await user.click(screen.getByRole('button', { name: 'Map' }))
  await waitFor(() => expect(body).toEqual({ sourceId: 's1', externalAccountId: 'wallet', accountId: fixtureAccounts[0].id }))
})

it('ignore posts ignore-account; an ignored card offers "Map instead"', async () => {
  let called = false
  server.use(http.post('*/api/v1/import/ignore-account', () => {
    called = true
    return HttpResponse.json({ success: true, message: '', data: { item: { ...source, cards: [{ ...unmapped, state: 'ignored' }] }, run: null } })
  }))
  const user = userEvent.setup()
  renderCards(source)
  await user.click(await screen.findByRole('button', { name: 'Ignore' }))
  await waitFor(() => expect(called).toBe(true))
  renderCards({ ...source, cards: [{ ...unmapped, state: 'ignored' }] })
  expect(screen.getByRole('button', { name: 'Map instead' })).toBeInTheDocument()
})

it('a mapped card shows the account name and unmaps after confirmation', async () => {
  let called = false
  server.use(http.post('*/api/v1/import/unlink-account', () => {
    called = true
    return HttpResponse.json({ success: true, message: '', data: { item: source, run: null } })
  }))
  const user = userEvent.setup()
  renderCards({ ...source, cards: [{ ...unmapped, state: 'mapped', accountId: fixtureAccounts[0].id, queuedCount: 0 }] })
  expect(await screen.findByText(`Mapped · ${fixtureAccounts[0].name}`)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Unmap' }))
  await user.click(await screen.findByRole('button', { name: 'Unmap' , hidden: false }))
  await waitFor(() => expect(called).toBe(true))
})
```

The last test clicks "Unmap" twice on purpose: the row button, then the confirm button with the same label; if RTL reports two matches for the second click, scope it with `within(screen.getByRole('dialog'))`. `fixtureAccounts[0]` must be owned by `fixtureUser` — check `web/src/test/fixtures.ts`; if the first account is shared, pick the first with `owner.id === fixtureUser.id`.

Run: `(cd web && pnpm vitest run src/features/imports)`
Expected: PASS.

- [ ] **Step 9: Route, settings hub**

`web/src/app/routes.tsx` — import `ImportsDataPage` from `@/features/imports/ImportsDataPage` next to the other page imports, and add after the recurring route:

```tsx
  { path: '/settings/data', element: <ImportsDataPage /> },
```

`web/src/features/settings/SettingsPage.tsx`:
- Remove the imports of `ExportCsvDialog`, `ImportCsvDialog`, `ImportResultDialog`, `AggregatedImportResult` and the three `useState`s that drove them (`exportOpen`, `importOpen`, `importResult`), and the three `<…Dialog>` elements at the bottom.
- Replace the Data group's two rows with one link row:

```tsx
          <MenuGroup label={t('settings.page.groups.data')}>
            <MenuRow label={t('imports.data_page.menu_item')} to={RouterPage.SETTINGS_DATA} />
          </MenuGroup>
```

`web/src/features/settings/SettingsPage.test.tsx` — replace the test `'Import CSV and Export CSV rows open their dialogs'` with a navigation test (add `{ path: '/settings/data', element: <div>DATA PAGE</div> }` to the `renderPage` router):

```tsx
it('the Data group links to the Import & export page', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Import & export'))
  expect(await screen.findByText('DATA PAGE')).toBeInTheDocument()
})
```

Run: `(cd web && pnpm vitest run src/features/settings src/features/imports src/lib/metrics-coverage.test.ts && pnpm lint && pnpm exec tsc -b)`
Expected: PASS, lint clean, type-check clean.

- [ ] **Step 10: i18n coverage guard**

Run: `go test ./internal/test/i18ntest/`
Expected: PASS — every `t('imports.…')` literal used by the three components resolves in the catalogue. A failure names the missing key; add it to every language via the catalogue (not by changing the component to a key that exists but means something else).

- [ ] **Step 11: Commit**

```bash
git add web/src/features/imports web/src/features/settings/SettingsPage.tsx web/src/features/settings/SettingsPage.test.tsx web/src/app/routes.tsx web/src/lib/metrics.ts web/src/test/fixtures.ts
git commit -m "feat(web): Settings → Import & export page with Apple Wallet setup and card mapping"
```

---

### Task 10: Import queue page, review banner, transaction dialog / row / view integration

**Files:**
- Create: `web/src/features/imports/ImportQueuePage.tsx`
- Create: `web/src/features/imports/ImportQueuePage.test.tsx`
- Create: `web/src/features/imports/ImportQueueBanner.tsx`
- Create: `web/src/features/imports/ImportQueueBanner.test.tsx`
- Modify: `web/src/app/uiStore.ts:9-17` (`OpenTransactionParams.importQueued`)
- Modify: `web/src/features/transactions/useTransactionForm.ts:38-59` (`initialFormState` branch)
- Modify: `web/src/features/transactions/useTransactionForm.test.ts`
- Modify: `web/src/features/transactions/TransactionDialog.tsx:70,199-216` (submit branch + pending)
- Modify: `web/src/features/transactions/TransactionDialog.test.tsx`
- Modify: `web/src/features/transactions/TransactionRow.tsx:1,80-86` (`isImported` badge)
- Modify: `web/src/features/transactions/TransactionRow.test.tsx`
- Modify: `web/src/features/transactions/ViewTransactionDialog.tsx` (provenance card)
- Modify: `web/src/features/transactions/ViewTransactionDialog.test.tsx`
- Modify: `web/src/app/layouts/ApplicationLayout.tsx:143` (banner mount)
- Modify: `web/src/app/routes.tsx` (route `/imports/queue`)
- Modify: `web/src/test/fixtures.ts` (`importQueue` data + `get-queued-event-list` handler)

**Interfaces:**
- Consumes (Task 8): `useImportQueue`, `useImportQueuedEvent` (`{ linkId, transaction: CreateTransactionDto }` → `{ item, accounts }`), `useSkipQueuedEvent(linkId)`, `useUnskipQueuedEvent(linkId)`, `useRetryImportEvent(eventId)` → `IngestEventDto`, `useDiscardImportEvent(eventId)`, `useTransactionImportLinks(transactionId, enabled)`, `useIgnoreImportAccount`, `useImportSources`, `RouterPage.IMPORT_QUEUE`, `ImportQueuedEventDto`, `ImportFailedEventDto`, `TransactionImportLinkDto`.
- Consumes (Task 9): `ImportCards`' map dialog is NOT reused — the queue page's per-card "Map to account" navigates to `RouterPage.SETTINGS_DATA` where the picker already lives (one picker, one place).
- Produces: `OpenTransactionParams.importQueued: ImportQueuedPrefill` (`web/src/app/uiStore.ts`), used by `initialFormState` and `TransactionDialog`.

- [ ] **Step 1: Test fixtures**

`web/src/test/fixtures.ts` — in `coreHandlers`, add `importQueue: { queued: [], skipped: [], failed: [] } as unknown` to `data` (after `importSources`) and one handler:

```ts
    http.get('*/api/v1/import/get-queued-event-list', () => envelope(data.importQueue)),
```

The banner (Step 8) mounts in `ApplicationLayout`, so every layout-level test that already uses `coreHandlers()` gets an empty queue for free.

- [ ] **Step 2: The prefill contract (red)**

`web/src/app/uiStore.ts` — extend `OpenTransactionParams` (the `TransactionType` import already exists at line 6):

```ts
// a queued import row being turned into a transaction: the dialog opens
// prefilled with the bank's data and, on save, posts import-queued-event
// (which links the row) instead of create-transaction
export interface ImportQueuedPrefill {
  linkId: Id
  type: TransactionType
  accountId: Id
  amount: string
  payee: string
  date: string
}

export interface OpenTransactionParams {
  transaction?: TransactionPrefill
  type?: TransactionType
  accountId?: Id
  // present when the caller is posting a due recurring template — switches
  // TransactionDialog into posting mode (prefilled from the template, with a
  // recurringId sent alongside the created transaction)
  postRecurring?: RecurringDto
  importQueued?: ImportQueuedPrefill
}
```

Append to `web/src/features/transactions/useTransactionForm.test.ts`:

```ts
it('a queued import seeds a new transaction from the bank data, payee as description', () => {
  const state = initialFormState(
    { importQueued: { linkId: 'l1', type: 'expense', accountId: 'a1', amount: '12.5', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' } },
    [account({})],
    null,
  )
  expect(state.id).toMatch(UUID_V7)
  expect(state.isNew).toBe(true)
  expect(state.type).toBe('expense')
  expect(state.accountId).toBe('a1')
  expect(state.amount).toBe('12.50')
  expect(state.description).toBe('Blue Bottle')
  expect(state.date).toBe('2026-08-20 10:42:03')
  expect(state.categoryId).toBeNull()
})
```

Run: `(cd web && pnpm vitest run src/features/transactions/useTransactionForm.test.ts)`
Expected: FAIL — `description` is `''` (the generic branch runs).

- [ ] **Step 3: initialFormState branch**

`web/src/features/transactions/useTransactionForm.ts` — insert between the `rt` block and the `tx` block:

```ts
  const iq = params.importQueued
  if (iq) {
    const account = accounts.find((a) => a.id === iq.accountId)
    return {
      id: uuidv7(),
      isNew: true,
      type: iq.type,
      accountId: iq.accountId || null,
      accountRecipientId: null,
      amount: seedAmount(iq.amount, account),
      amountRecipient: '',
      categoryId: null,
      payeeId: null,
      tagId: null,
      labelIds: [],
      // the bank's merchant string is free text; payees are the user's own
      // entities, so it lands in the description for the user to reclassify
      description: iq.payee,
      date: iq.date,
    }
  }
```

Run: `(cd web && pnpm vitest run src/features/transactions/useTransactionForm.test.ts)`
Expected: PASS.

- [ ] **Step 4: Dialog submit branch (red)**

Append to `web/src/features/transactions/TransactionDialog.test.tsx`:

```tsx
it('a queued import posts import-queued-event with the link id and the edited transaction', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/import/import-queued-event', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireTxEcho({ isImported: 1 }), accounts: fixtureAccounts } })
    }),
  )
  const user = userEvent.setup()
  renderDialog('/')
  useUiStore.getState().openTransactionModal({
    importQueued: { linkId: 'l1', type: 'expense', accountId: 'a1', amount: '12.5', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' },
  })

  await screen.findByRole('heading', { name: 'Add transaction' })
  expect(screen.getByLabelText('Amount')).toHaveValue('12.50')
  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  await user.click(await screen.findByText('Food'))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body!.linkId).toBe('l1')
  const tx = body!.transaction as Record<string, unknown>
  expect(tx.accountId).toBe('a1')
  expect(tx.amount).toBe('12.50')
  expect(tx.categoryId).toBe('cat-food')
  expect(tx.description).toBe('Blue Bottle')
  expect(tx.date).toBe('2026-08-20 10:42:03')
})
```

`buildPayload` may normalise the amount (`'12.5'` vs `'12.50'`) — check `buildPayload` in `useTransactionForm.ts` and match the single `tx.amount` assertion to what it produces.

Run: `(cd web && pnpm vitest run src/features/transactions/TransactionDialog.test.tsx -t "queued import")`
Expected: FAIL — the dialog posts to `create-transaction` (msw logs an unhandled request) and `body` stays undefined.

- [ ] **Step 5: Dialog submit branch**

`web/src/features/transactions/TransactionDialog.tsx`:

```tsx
import { useImportQueuedEvent } from '@/features/imports/queries'
…
  const postRecurring = usePostRecurring()
  const importQueued = useImportQueuedEvent()
…
      if (params.importQueued) {
        await importQueued.mutateAsync({ linkId: params.importQueued.linkId, transaction: payload })
      } else if (params.postRecurring) {
        await postRecurring.mutateAsync({ ...payload, recurringId: params.postRecurring.id })
      } else if (form.isNew) {
…
  const pending = createTransaction.isPending || updateTransaction.isPending || postRecurring.isPending || importQueued.isPending
```

Run: `(cd web && pnpm vitest run src/features/transactions/TransactionDialog.test.tsx)`
Expected: PASS.

- [ ] **Step 6: Row badge + provenance card (red)**

Append to `web/src/features/transactions/TransactionRow.test.tsx`:

```tsx
it('marks an imported transaction with the import glyph', () => {
  renderRow({ isImported: 1 })
  expect(screen.getByLabelText('Imported')).toBeInTheDocument()
})

it('shows no import glyph on a hand-entered transaction', () => {
  renderRow({ isImported: 0 })
  expect(screen.queryByLabelText('Imported')).toBeNull()
})
```

Append to `web/src/features/transactions/ViewTransactionDialog.test.tsx` (add `import { http, HttpResponse } from 'msw'` at the top):

```tsx
it('lists the import provenance for an imported transaction', async () => {
  server.use(http.get('*/api/v1/import/get-transaction-import-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [{
      id: 'l1', sourceId: 's1', provider: 'apple-wallet', sourceName: 'iPhone', externalAccountId: 'Apple Card',
      externalTransactionId: 'evt-1', externalPayee: 'Blue Bottle', externalAmount: '12.5', externalCurrency: 'USD',
      externalPostedAt: '2026-08-20 10:42:03', status: 'imported', importedAt: '2026-08-20 10:42:05',
    }] } })))
  renderView({ transaction: { ...fixtureTransaction, isImported: 1 } as ViewTransaction })
  expect(await screen.findByText('Imported from')).toBeInTheDocument()
  expect(screen.getByText(/Apple Card/)).toBeInTheDocument()
  expect(screen.getByText(/Blue Bottle/)).toBeInTheDocument()
})

it('fetches no provenance for a hand-entered transaction', () => {
  renderView({ transaction: { ...fixtureTransaction, isImported: 0 } as ViewTransaction })
  expect(screen.queryByText('Imported from')).toBeNull()
})
```

Run: `(cd web && pnpm vitest run src/features/transactions/TransactionRow.test.tsx src/features/transactions/ViewTransactionDialog.test.tsx)`
Expected: FAIL on the three new positive assertions.

- [ ] **Step 7: Row badge + provenance card**

`web/src/features/transactions/TransactionRow.tsx`:

```tsx
import { ArrowDownToLine, Repeat } from 'lucide-react'
…
          {isRecurring ? ( … existing … ) : null}
          {tx.isImported === 1 ? (
            <span title={t('imports.badge')} className="ml-1 flex shrink-0 items-center">
              <ArrowDownToLine className="size-3 text-muted-foreground" aria-label={t('imports.badge')} />
            </span>
          ) : null}
```

`web/src/features/transactions/ViewTransactionDialog.tsx` — import `useTransactionImportLinks` from `@/features/imports/queries` and `formatDateTime, parseDateTime` from `@/lib/datetime`; fetch only for imported rows and add a card after the tags card:

```tsx
  const { data: importLinks = [] } = useTransactionImportLinks(tx.id, tx.isImported === 1)
…
  if (importLinks.length > 0) {
    cards.push({
      label: t('imports.provenance.header'),
      content: (
        <span className="flex flex-col gap-1 text-sm">
          {importLinks.map((l) => (
            <span key={l.id} className="break-words">
              {l.sourceName} · {l.externalAccountId} · {l.externalPayee} {l.externalAmount} {l.externalCurrency} · {formatDateTime(parseDateTime(l.externalPostedAt))}
            </span>
          ))}
        </span>
      ),
    })
  }
```

Run: `(cd web && pnpm vitest run src/features/transactions)`
Expected: PASS.

- [ ] **Step 8: Banner (red → green)**

Create `web/src/features/imports/ImportQueueBanner.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportQueueBanner } from './ImportQueueBanner'

const queued = (linkId: string) => ({
  linkId, sourceId: 's1', externalAccountId: 'Apple Card', accountId: '', payee: 'Blue Bottle', amount: '12.5',
  currency: 'USD', type: 'expense', postedAt: '2026-08-20 10:42:03', reason: 'unmapped',
})

function renderBanner(path = '/') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/', element: <ImportQueueBanner /> }, { path: '/imports/queue', element: <ImportQueueBanner /> }],
    { initialEntries: [path] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('renders nothing while the queue is empty', async () => {
  server.use(...coreHandlers())
  renderBanner()
  await new Promise((r) => setTimeout(r, 20))
  expect(screen.queryByText(/waiting for review/)).toBeNull()
})

it('counts the queued rows, pluralised, and links to the queue', async () => {
  server.use(...coreHandlers({ importQueue: { queued: [queued('l1'), queued('l2')], skipped: [], failed: [] } }))
  renderBanner()
  expect(await screen.findByText('2 imported transactions are waiting for review')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Review' })).toHaveAttribute('href', '/imports/queue')
})

it('stays hidden on the queue page itself', async () => {
  server.use(...coreHandlers({ importQueue: { queued: [queued('l1')], skipped: [], failed: [] } }))
  renderBanner('/imports/queue')
  await new Promise((r) => setTimeout(r, 20))
  expect(screen.queryByText(/waiting for review/)).toBeNull()
})
```

Create `web/src/features/imports/ImportQueueBanner.tsx` (same markup family as `SubscriptionBanner`; no dismiss — the count is the call to action, and it disappears by itself once the rows are handled):

```tsx
import { Link, useLocation } from 'react-router'
import { useTranslation } from 'react-i18next'
import { RouterPage } from '@/app/router-pages'
import { buttonVariants } from '@/components/ui/button'
import { pluralPick } from '@/lib/plural'
import { useImportQueue } from './queries'

export function ImportQueueBanner() {
  const { t, i18n } = useTranslation()
  const { pathname } = useLocation()
  const { data } = useImportQueue()
  const count = data?.queued.length ?? 0
  if (count === 0 || pathname === RouterPage.IMPORT_QUEUE) {
    return null
  }
  return (
    <div className="flex items-center gap-3 bg-primary/10 px-4 py-2 text-sm text-primary">
      <span className="min-w-0 flex-1">{pluralPick(t('imports.banner.text', { count }), count, i18n.language)}</span>
      <Link to={RouterPage.IMPORT_QUEUE} className={buttonVariants({ size: 'sm', variant: 'outline' })}>
        {t('imports.banner.cta')}
      </Link>
    </div>
  )
}
```

`pluralPick` receives the already-interpolated pipe string (`t` substitutes `{count}` in every variant), exactly like `subscription.banner.connection_trial` in `SubscriptionBanner.tsx:66`. `buttonVariants` is exported by `web/src/components/ui/button.tsx` (shadcn); if it is not, wrap a `<Button asChild>` around the `Link` instead.

`web/src/app/layouts/ApplicationLayout.tsx` — import `ImportQueueBanner` from `@/features/imports/ImportQueueBanner` and mount it right after `<ServerVersionNotice />` (line 143):

```tsx
      <SubscriptionBanner />
      <ServerVersionNotice />
      <ImportQueueBanner />
```

Run: `(cd web && pnpm vitest run src/features/imports/ImportQueueBanner.test.tsx src/app/layouts)`
Expected: PASS.

- [ ] **Step 9: Queue page test (red)**

Create `web/src/features/imports/ImportQueuePage.test.tsx`:

```tsx
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { ImportQueuePage } from './ImportQueuePage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const queued = (over: Record<string, unknown> = {}) => ({
  linkId: 'l1', sourceId: 's1', externalAccountId: 'Apple Card', accountId: '', payee: 'Blue Bottle', amount: '12.5',
  currency: 'USD', type: 'expense', postedAt: '2026-08-20 10:42:03', reason: 'unmapped', ...over,
})
const failed = { eventId: 'e9', sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'amount: This value should not be blank.', payload: '{"account":"Apple Card","amount":""}' }
const source = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [
  { externalAccountId: 'Apple Card', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 1, tapCount: 1, lastSeenAt: '2026-08-20 10:42:03' },
] }

function renderPage(importQueue: unknown) {
  server.use(...coreHandlers({ importSources: [source], importQueue }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/imports/queue', element: <ImportQueuePage /> }, { path: '/settings/data', element: <div>DATA PAGE</div> }],
    { initialEntries: ['/imports/queue'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  useUiStore.setState({ transactionModal: null })
})

it('shows the empty state when nothing is queued', async () => {
  renderPage({ queued: [], skipped: [], failed: [] })
  expect(await screen.findByText('Nothing to review.')).toBeInTheDocument()
})

it('groups queued rows by card and opens the prefilled transaction dialog on tap', async () => {
  renderPage({ queued: [queued(), queued({ linkId: 'l2', payee: 'Whole Foods', amount: '80' })], skipped: [], failed: [] })
  const user = userEvent.setup()
  expect(await screen.findByText('Apple Card')).toBeInTheDocument()
  expect(screen.getAllByText(/Card not mapped/)).toHaveLength(2)
  await user.click(screen.getByText('Blue Bottle'))
  const params = useUiStore.getState().transactionModal
  expect(params?.importQueued).toEqual({ linkId: 'l1', type: 'expense', accountId: '', amount: '12.5', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' })
})

it('an unmapped card header links to the mapping page', async () => {
  renderPage({ queued: [queued()], skipped: [], failed: [] })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('link', { name: 'Map to account' }))
  expect(await screen.findByText('DATA PAGE')).toBeInTheDocument()
})

it('skip posts skip-queued-event; a skipped row offers Restore', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/skip-queued-event', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [queued()], failed: [] } })
  }))
  renderPage({ queued: [queued()], skipped: [], failed: [] })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Skip' }))
  await waitFor(() => expect(body).toEqual({ linkId: 'l1' }))
  expect(await screen.findByRole('button', { name: 'Restore' })).toBeInTheDocument()
})

it('needs-attention rows show the error and payload; retry toasts the outcome; discard removes', async () => {
  let retried: unknown
  let discarded: unknown
  server.use(
    http.post('*/api/v1/import/retry-event', async ({ request }) => {
      retried = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { status: 'failed', eventId: 'e9' } })
    }),
    http.post('*/api/v1/import/discard-event', async ({ request }) => {
      discarded = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [], failed: [] } })
    }),
  )
  renderPage({ queued: [], skipped: [], failed: [failed] })
  const user = userEvent.setup()
  const section = (await screen.findByText('Needs attention')).parentElement as HTMLElement
  expect(within(section).getByText('amount: This value should not be blank.')).toBeInTheDocument()
  expect(within(section).getByText(/"account":"Apple Card"/)).toBeInTheDocument()
  await user.click(within(section).getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(retried).toEqual({ eventId: 'e9' }))
  await user.click(within(section).getByRole('button', { name: 'Discard' }))
  await waitFor(() => expect(discarded).toEqual({ eventId: 'e9' }))
  expect(await screen.findByText('Nothing to review.')).toBeInTheDocument()
})
```

Run: `(cd web && pnpm vitest run src/features/imports/ImportQueuePage.test.tsx)`
Expected: FAIL — module not found.

- [ ] **Step 10: Queue page**

Create `web/src/features/imports/ImportQueuePage.tsx`:

```tsx
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportFailedEventDto, ImportQueuedEventDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useAccounts } from '@/features/accounts/queries'
import { formatDateTime, parseDateTime } from '@/lib/datetime'
import { useDiscardImportEvent, useIgnoreImportAccount, useImportQueue, useImportSources, useRetryImportEvent, useSkipQueuedEvent, useUnskipQueuedEvent } from './queries'

function groupByCard(rows: ImportQueuedEventDto[]): Map<string, ImportQueuedEventDto[]> {
  const out = new Map<string, ImportQueuedEventDto[]>()
  for (const row of rows) {
    const key = `${row.sourceId} ${row.externalAccountId}`
    out.set(key, [...(out.get(key) ?? []), row])
  }
  return out
}

export function ImportQueuePage() {
  const { t } = useTranslation()
  const { data: queue } = useImportQueue()
  const { data: sources = [] } = useImportSources()
  const { data: accounts = [] } = useAccounts()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const skip = useSkipQueuedEvent()
  const unskip = useUnskipQueuedEvent()
  const retry = useRetryImportEvent()
  const discard = useDiscardImportEvent()
  const ignore = useIgnoreImportAccount()

  const queued = queue?.queued ?? []
  const skipped = queue?.skipped ?? []
  const failed = queue?.failed ?? []
  const empty = queue !== undefined && queued.length === 0 && skipped.length === 0 && failed.length === 0

  const cardState = (row: ImportQueuedEventDto) =>
    sources.find((s) => s.id === row.sourceId)?.cards.find((c) => c.externalAccountId === row.externalAccountId)?.state ?? 'unmapped'
  const accountName = (id: string) => accounts.find((a) => a.id === id)?.name ?? ''
  const reasonText = (row: ImportQueuedEventDto) =>
    row.reason === 'no_rate'
      ? t('imports.queue.reason.no_rate', { currency: row.currency })
      : row.reason === 'account_deleted'
        ? t('imports.queue.reason.account_deleted')
        : t('imports.queue.reason.unmapped')

  const review = (row: ImportQueuedEventDto) =>
    openTransactionModal({
      importQueued: { linkId: row.linkId, type: row.type, accountId: row.accountId, amount: row.amount, payee: row.payee, date: row.postedAt },
    })

  const retryResultText = (status: string) =>
    status === 'created'
      ? t('imports.queue.retry_result.created')
      : status === 'queued'
        ? t('imports.queue.retry_result.queued')
        : status === 'skipped'
          ? t('imports.queue.retry_result.skipped')
          : status === 'duplicate'
            ? t('imports.queue.retry_result.duplicate')
            : t('imports.queue.retry_result.failed')

  const retryEvent = (ev: ImportFailedEventDto) =>
    retry.mutate(ev.eventId, {
      onSuccess: (result) => toast.success(retryResultText(result.status)),
    })

  const row = (r: ImportQueuedEventDto, action: ReactNode) => (
    <div key={r.linkId} className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3 text-sm">
      <button type="button" className="min-w-0 flex-1 text-left" title={t('imports.queue.import')} onClick={() => review(r)}>
        <div className="truncate font-medium">{r.payee}</div>
        <div className="text-xs text-muted-foreground">
          {formatDateTime(parseDateTime(r.postedAt))} · {reasonText(r)}
        </div>
      </button>
      <div className={`shrink-0 ${r.type === 'expense' ? 'text-expense' : 'text-income'}`}>
        {r.type === 'expense' ? '-' : '+'}{r.amount} {r.currency}
      </div>
      {action}
    </div>
  )

  return (
    <SettingsShell title={t('imports.queue.header')} backTo={RouterPage.HOME}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-6">
        {empty ? <p className="px-1 text-sm text-muted-foreground">{t('imports.queue.empty')}</p> : null}

        {queued.length > 0 ? (
          <section className="flex flex-col gap-2">
            <p className="px-1 text-xs uppercase text-muted-foreground">{t('imports.queue.queued')}</p>
            {[...groupByCard(queued).entries()].map(([key, rows]) => {
              const first = rows[0]
              const state = cardState(first)
              return (
                <div key={key} className="flex flex-col gap-2">
                  <div className="flex items-center justify-between gap-2 px-1 pt-2 text-sm">
                    <span className="font-medium">{first.externalAccountId}</span>
                    {state === 'unmapped' ? (
                      <span className="flex gap-3">
                        <Link to={RouterPage.SETTINGS_DATA} className="text-primary underline-offset-2 hover:underline">{t('imports.apple_wallet.cards.map')}</Link>
                        <button type="button" className="text-muted-foreground underline-offset-2 hover:underline" disabled={ignore.isPending}
                          onClick={() => ignore.mutate({ sourceId: first.sourceId, externalAccountId: first.externalAccountId })}>
                          {t('imports.apple_wallet.cards.ignore')}
                        </button>
                      </span>
                    ) : first.accountId ? (
                      <span className="text-xs text-muted-foreground">{accountName(first.accountId)}</span>
                    ) : null}
                  </div>
                  {rows.map((r) => row(r,
                    <Button type="button" size="sm" variant="ghost" disabled={skip.isPending} onClick={() => skip.mutate(r.linkId)}>
                      {t('imports.queue.skip')}
                    </Button>,
                  ))}
                </div>
              )
            })}
          </section>
        ) : null}

        {skipped.length > 0 ? (
          <section className="flex flex-col gap-2">
            <p className="px-1 text-xs uppercase text-muted-foreground">{t('imports.queue.skipped')}</p>
            {skipped.map((r) => row(r,
              <Button type="button" size="sm" variant="ghost" disabled={unskip.isPending} onClick={() => unskip.mutate(r.linkId)}>
                {t('imports.queue.unskip')}
              </Button>,
            ))}
          </section>
        ) : null}

        {failed.length > 0 ? (
          <section className="flex flex-col gap-2">
            <p className="px-1 text-xs uppercase text-muted-foreground">{t('imports.queue.failed')}</p>
            {failed.map((ev) => (
              <div key={ev.eventId} className="flex flex-col gap-2 rounded-lg bg-econumo-card px-4 py-3 text-sm">
                <div className="text-xs text-muted-foreground">{t('imports.queue.received', { date: formatDateTime(parseDateTime(ev.receivedAt)) })}</div>
                <div className="text-destructive">{ev.error}</div>
                <pre className="overflow-x-auto whitespace-pre-wrap break-all text-xs text-muted-foreground">{ev.payload}</pre>
                <div className="flex gap-2">
                  <Button type="button" size="sm" variant="secondary" disabled={retry.isPending} onClick={() => retryEvent(ev)}>{t('imports.queue.retry')}</Button>
                  <Button type="button" size="sm" variant="ghost" disabled={discard.isPending} onClick={() => discard.mutate(ev.eventId)}>{t('imports.queue.discard')}</Button>
                </div>
              </div>
            ))}
          </section>
        ) : null}
      </div>
    </SettingsShell>
  )
}
```

Notes for the implementer:
- Rows on an unmapped card still open the dialog: `accountId` is `''`, so `initialFormState` leaves the account empty and the dialog's account select is where the user picks one (the server accepts any account the caller may write to). The "Map to account" link is the faster path when many rows share a card.
- `reasonText` / `retryResultText` spell every key out as a literal on purpose — the i18n coverage guard (`internal/test/i18ntest`) scans source for literal `t('…')` keys, so a template-literal key would leave those catalogue entries "unused".
- The `Needs attention` test locates the section via `parentElement` of the heading, so the heading `<p>` must stay a direct child of the `<section>` (as written).
- `empty` is false while the first fetch is in flight so the page does not flash "Nothing to review." before the data lands.

`web/src/app/routes.tsx` — import `ImportQueuePage` from `@/features/imports/ImportQueuePage` and add after the `/settings/data` route:

```tsx
                { path: '/imports/queue', element: <ImportQueuePage /> },
```

Run: `(cd web && pnpm vitest run src/features/imports && pnpm lint && pnpm exec tsc -b)`
Expected: PASS, lint clean, type-check clean.

- [ ] **Step 11: i18n coverage guard + full web suite**

Run: `go test ./internal/test/i18ntest/ && (cd web && pnpm test)`
Expected: PASS except the pre-existing `exportTransactionList … resolves a Blob` failure. If the guard names an `imports.*` key as unused, it is one of the literal keys above that got refactored into a template — restore the literal.

- [ ] **Step 12: Commit**

```bash
git add web/src/features/imports web/src/features/transactions web/src/app/uiStore.ts web/src/app/routes.tsx web/src/app/layouts/ApplicationLayout.tsx web/src/test/fixtures.ts
git commit -m "feat(web): import queue page, review banner, imported-transaction surfaces"
```

---

### Task 11: Docs, Shortcut contract, regression plan, full gates

**Files:**
- Modify: `CLAUDE.md:186-191` (the `imports` stage paragraph), `CLAUDE.md:471-490` (rate-limit list), the "Web UI config" bullet (served keys), the "API conventions" examples
- Modify: `web/shortcuts/README.md` (exists since stage 1; reconcile, do not rewrite)
- Modify: `docs/regression-test-plan.md:155-157` (stale "no UI badge yet" item), new section after `## 5. Transactions`, one item in `## 12`
- Modify: `.env.example:80-85` (ingest rate limit next to the matcher block)

**Interfaces:**
- Consumes: everything Tasks 1–10 produced; this task adds no code.
- Produces: the Shortcut build contract (`web/shortcuts/README.md`) the maintainer follows to produce `web/public/shortcuts/econumo-wallet-v1.shortcut` and `econumo-setup-v1.shortcut` by hand.

- [ ] **Step 1: CLAUDE.md**

Replace the stage paragraph (lines 186-191) with:

```
no `repository.go` either. `imports` (the bank/phone transaction-import
subsystem, spec in `docs/superpowers/specs/2026-08-15-transaction-import-design.md`)
ships in stages: stage 1 (persistence + matcher core) and stage 2 (the Apple
Wallet push provider) are in; it has `repository.go`, `ports.go` (account /
currency / transaction-creation ports, wired in `internal/server/glue_imports.go`),
`repo/`, and `api/` with the 14 routes under `/api/v1/import/` (`create-source`,
`get-source-list`, `delete-source`, `link-account`, `ignore-account`,
`unlink-account`, `ingest-apple-wallet-event`, `get-queued-event-list`,
`import-queued-event`, `skip-queued-event`, `unskip-queued-event`, `retry-event`,
`discard-event`, `get-transaction-import-list`). The package name is `imports`
(not `import`, a Go keyword). No MCP surface yet.
```

In the rate-limit bullet (after the `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL_CHANGE` line) add:

```
  `ECONUMO_RATE_LIMIT_INGEST` — `import/ingest-apple-wallet-event` pushes per user per window (default `60`; every request counts).
```

In the "Web UI config" bullet, after the sentence listing `ECONUMO_VERSION`, add:

```
  `IMPORT_MATCHER` (`{matchDays, tipDays, tipTolerancePct, tokenMinLength}`, the
  effective `ECONUMO_IMPORT_*` values) is always merged so the SPA can explain the
  matcher's windows.
```

In "Authentication", the `ingest` scope sentence already names `/api/v1/import/ingest-*`; leave it. In "Notable behaviours" append:

```
- **Transaction import (Apple Wallet)**: one `import_sources` row per user per
  provider (`create-source` is idempotent on the pair); a Wallet card is keyed by its
  normalized name and starts `unmapped` — its events queue (`import_transaction_links`
  with no transaction) until the user maps it to an OWNED account (`link-account`
  replays the queue in one run) or ignores it. Ingest is scope-gated (`ingest` PATs),
  exact-duplicate-safe (`(source, external_transaction_id)`), and never fails the
  request on a bad transaction — parse errors land in `import_events.status = failed`
  for the queue page's "needs attention" list. Imported transactions read
  `isImported: 1`; `get-transaction-import-list` is their provenance.
```

- [ ] **Step 2: `.env.example`**

After the `ECONUMO_IMPORT_TOKEN_MIN_LENGTH` line add:

```
#ECONUMO_RATE_LIMIT_INGEST=60         # ingest pushes per user per rate-limit window (0 = off)
```

- [ ] **Step 3: Shortcut build guide — reconcile with the shipped contract**

`web/shortcuts/README.md` already exists (written with stage 1) and is the maintainer's build guide for the two signed `.shortcut` files. Do NOT rewrite it; make exactly these edits so it matches what Tasks 2, 7 and 9 shipped:

1. Every "Settings → Data" becomes "Settings → Import & export" (the menu row and page from Task 9; occurrences in the intro, the Setup alert text, the Wallet "not configured" notification, and "Test on a device" step 2).
2. In "Contract with the server", extend the request body with the two optional fields the parser accepts and describe the response:

```
{
  "account":    "<Card or Pass name>",
  "payee":      "<Merchant>",
  "amount":     "<Amount, number>",
  "currency":   "<ISO 4217 code>",
  "occurredAt": "<ISO 8601 with offset>",
  "type":       "expense",
  "eventId":    "<Transaction Identifier, optional>"
}
```

   followed by this paragraph replacing the existing "The server answers 200 …" one:

```
`eventId` is optional: when present it is the dedupe key (a re-run of the
automation for the same tap is `duplicate`); when absent the server derives
`sha256(account|occurredAt UTC|amount|payee)`. `type` is `expense` or
`income` (refunds); anything else is a parse error.

The server answers 200 `{"status": "created"|"queued"|"skipped"|"duplicate"|"failed", "eventId": "…"}`
whenever the event was stored, even if it could not be imported (`queued` /
`failed` rows are handled on the web queue page), so the shortcut has nothing
to do with the response. Non-200s: 401 (token revoked — run Setup again),
402 (read-only access), 429 (the per-user ingest rate limit,
`ECONUMO_RATE_LIMIT_INGEST`).
```

3. In the "Build: `Econumo Wallet`" request-body table add a row
   `| \`eventId\` | Text | \`Shortcut Input\` › **Transaction Identifier** (omit the row if the picker has no such property) |`.
4. Add to the intro a sentence pointing at the in-app fallback: "The same recipe, reduced to the five actions a user can type in, is shown in the SPA under *Configure manually* (`web/src/features/imports/AppleWalletSetup.tsx`); keep the URL, headers and field names in the two in sync."

Leave the rest of the file untouched (signing, extraction, device test).

- [ ] **Step 4: Regression test plan**

Replace lines 155-157 with:

```
- [ ] Transaction list rows carry `isImported` (0/1) in the API response; an
      imported row shows the import glyph (tooltip "Imported"), hand-entered
      rows do not; the preview dialog of an imported row lists "Imported from"
      (source · card · merchant amount currency · posted time).
```

Insert a new section after the CSV export item at the end of `## 5. Transactions`:

```
## 5a. Imports — Apple Wallet

- [ ] Settings → Data group has one row, "Import & export" 📱; the page holds the
      CSV import/export rows (dialogs open as before) and the Apple Wallet section.
- [ ] "Set up Apple Wallet" creates the source (idempotent: a second click or a
      second device does not create a second source); the section flips to
      "Connected" with the three steps; "Disconnect" (confirmation) removes the
      source, its cards and its queue; already-imported transactions stay.
- [ ] Step 1 links download `econumo-wallet-v1.shortcut` and
      `econumo-setup-v1.shortcut`.
- [ ] iOS only 📱: "Configure on this iPhone" mints an ingest PAT (visible under
      Profile → Tokens with scope `ingest`) and opens the Shortcuts app with the
      Setup shortcut prefilled; desktop shows the "open on your iPhone" note instead.
- [ ] "Configure manually" reveals the token once (copy button), the server URL,
      the request body and the five Shortcuts actions.
- [ ] Ingest with an `ingest`-scoped PAT: `POST /api/v1/import/ingest-apple-wallet-event`
      → `status: queued` for an unmapped card; the card appears in the list as
      "Unmapped · 1 queued", tap count and last-seen date update per event; a
      `full` PAT / session token is accepted too; an `ingest` PAT on any other route
      is 401.
- [ ] Same payload twice → `duplicate`, no second row; a body without `account`
      or with a bad currency → `status: failed`, row in "Needs attention" with the
      error text and the raw payload; Retry re-parses (toast with the outcome),
      Discard removes it.
- [ ] Map card → account (owned accounts only in the picker; shared accounts
      absent): the queue replays — toast "N imported, N matched, N skipped";
      imported transactions appear on the account with the glyph; a same-amount
      hand-entered transaction within ±3 days is adopted (no duplicate) and shows
      the provenance card.
- [ ] Currency mismatch (card USD → EUR account) is refused with a field error on
      the account; an ignored card offers "Map instead"; "Unmap" (confirmation)
      returns the card to unmapped and new taps queue again.
- [ ] Review banner 📱: with queued rows, every page except the queue shows
      "N imported transactions are waiting for review" + "Review"; the banner
      disappears when the queue empties.
- [ ] Queue page 📱: rows grouped by card, unmapped cards carry "Map to account"
      (→ Import & export page) and "Ignore"; tapping a row opens the add-transaction
      dialog prefilled (account, amount, merchant as description, posted date);
      saving posts `import-queued-event` — the row leaves the queue and the
      transaction is created with the glyph; Skip moves a row to "Skipped",
      Restore brings it back.
- [ ] Tip adopt: a tap for 40.00 at "Blue Bottle" followed within 5 days by a
      posted 48.00 "BLUE BOTTLE COFFEE" from another source adopts (amount
      corrected to 48.00) rather than creating a second transaction.
- [ ] Rate limit: the 61st ingest within the window from one user is 429 with the
      frozen envelope.
```

In `## 12` the existing "ingest token (create via API)" item becomes:

```
- [ ] Create a personal token with scope "full" — it works everywhere; an
      "ingest" token (created via "Configure on this iPhone" or the API) is
      rejected with 401 on every non-import route and accepted on
      `import/ingest-apple-wallet-event`.
```

- [ ] **Step 5: Gates**

```bash
make go-test
make web-lint
make web-test
(cd web && pnpm exec tsc -b)
```

Expected: all green except the known pre-existing `exportTransactionList … resolves a Blob` vitest failure. `make go-test` covers vet, gofmt, the OpenAPI-docs-fresh check (run `make swagger` and commit `docs/` if it complains — the 14 new handlers carry `@` blocks), i18ntest, apiparity goldens, mcpparity goldens, and the 80 % coverage gate.

Then the PostgreSQL tier, with a throwaway container:

```bash
docker run -d --name econumo-pg-test -e POSTGRES_USER=econumo -e POSTGRES_PASSWORD=econumo -e POSTGRES_DB=econumo_test -p 15432:5432 postgres:17-alpine
sleep 5
DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@localhost:15432/econumo_test?sslmode=disable' make test-repo-pgsql
DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@localhost:15432/econumo_test?sslmode=disable' go test -tags enginecompare ./internal/test/enginecompare/ ./internal/test/mcpparity/
docker rm -f econumo-pg-test
```

Expected: PASS. A pgsql-only failure is almost always a `?` placeholder in hand-written SQL (use `db.Rebind`) or a NUMERIC-vs-text decimal compare (normalize through `vo.Decimal`).

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md .env.example web/shortcuts/README.md docs/regression-test-plan.md docs/
git commit -m "docs: Apple Wallet import — CLAUDE.md, shortcut contract, regression plan"
```

---

## Self-review notes (kept for the executor)

- Spec Part 6's `type` field: `expense` default, `income` accepted — the SPA's sample body omits it on purpose (the common case), the README documents it.
- Spec Part 9 analytics keys: `IMPORT_SOURCE_CONNECT`, `IMPORT_ACCOUNT_LINK`, `IMPORT_ACCOUNT_IGNORE`, `IMPORT_QUEUE_IMPORT`, `IMPORT_QUEUE_SKIP` (Task 8), `IMPORT_SHORTCUT_DOWNLOAD`, `IMPORT_SHORTCUT_CONFIGURE` (Task 9) — all fired from the shared hook/component choke points, so `metrics-coverage.test.ts` passes without `NOT_WIRED` additions.
- Spec Part 9 also lists `IMPORT_QUEUE_MAP`; the queue page's "Map to account" is a link to the Import & export page, where the mapping fires `IMPORT_ACCOUNT_LINK` from the shared hook — one key covers the one choke point, so `IMPORT_QUEUE_MAP` is not added (an unfired key would fail `metrics-coverage.test.ts`).
- The `.shortcut` binaries are out of scope for every task: the SPA links them, the README specifies them, the maintainer builds and commits them separately.
