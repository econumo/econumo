# Transaction Import — Stage 3 (SimpleFIN) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the first *pull* provider: a user connects a SimpleFIN bridge with a setup token, keeps the resulting access URL encrypted under a passphrase-derived key the server never sees, and presses **Sync** to have the server fetch bank transactions through the bridge, run them through the existing matcher/creation pipeline, record a run with per-account error handling, and show run history.

**Architecture:** `internal/imports` gains a pull-side `Provider` seam (`provider.go`) and a registry on the `Service`; `internal/imports/simplefin` is the one implementation (stateless HTTP proxy: claim URL → access URL, `GET {accessUrl}/accounts`). The `sync-source` use case receives the *plaintext* access URL per request from the SPA, never persists or logs it, opens an `import_runs` row, and processes every external account in its own DB transaction so one bad account leaves a `partial` run instead of a failed one. The SPA adds `web/src/lib/importCrypto.ts` (Web Crypto: PBKDF2 → wrapping key → AES-GCM data key kept non-extractable in IndexedDB), a Settings → SimpleFIN page (connect / unlock / accounts / Sync / history), a "Sync all" row on Settings → Import & export, and a run detail page.

**Tech Stack:** Go 1.2x stdlib (`net/http`, `encoding/json`, `log/slog`), sqlc (sqlite + pgsql query files, `go generate ./internal/infra/storage/sqlc/`), `modernc.org/sqlite`, `jackc/pgx/v5`; React 19 + Vite + TypeScript, TanStack Query, Zustand, react-i18next, vitest (+ `fake-indexeddb` devDependency for the crypto round-trip test), oxlint; `docs/regression-test-plan.md` for manual QA.

**Spec:** `docs/superpowers/specs/2026-08-15-transaction-import-design.md` — Part 1 threat model ("at-rest zero-knowledge, in-flight trusted"), Part 2 schema (`import_credential_keys`, `import_runs`, `import_sources.credential_ciphertext`/`last_synced_at`), Part 3 provider seam + SimpleFIN protocol, Part 5 pull flow + field mapping + failure handling + logging, Part 8 endpoint table (stage-3 subset), Part 9 Settings → Data Sync button + run summary, credential crypto, analytics, Testing list, staging item 3, resolved questions. Stages 1 (PR #230) and 2 (PR #231) are the base: `model.Import*`, `imports.Repository`, `imports.Service` + push pipeline (`applyEvent`/`place`), `imports.Match`, `internal/imports/api`, the SPA `features/imports` slice.

## Global Constraints

- Features never import features (`internal/test/archtest`). `internal/imports/simplefin` may import its parent `internal/imports` and the kernel (`model`, `shared/*`), nothing else; `internal/imports` never imports `simplefin` — `internal/server` wires it.
- Engine-adapter pattern: every new query exists in BOTH `internal/infra/storage/sqlc/query/sqlite/imports.sql` and `.../pgsql/imports.sql`; the `querier` interface is in sqlite-generated types; `sqlite.go` passes through, `pgsql.go` converts whole structs. pgsql files put `;` on its own line; sqlite files end statements inline (a `;` on its own line silently truncates generated sqlite SQL). Migrations are paired files `internal/infra/storage/migrations/{sqlite,pgsql}/20260907000000.sql`.
- Wire contract is frozen: only `GET` (reads) and `POST` (writes); paths `/api/v1/import/{action}-{subject}`; envelope `{"success","message","data"}`; datetimes `"2006-01-02 15:04:05"` (`datetime.Layout`), `""` for an unset optional datetime; ids are UUIDv7 strings; `errors` object always present on handled errors; JSON arrays are never `null` (empty slice → `[]`).
- Every new error code: a `Code…` const in `internal/shared/errs/codes.go`, an entry in `AllCodes`, and `errors.import.<key>` in ALL 11 catalogues (`de,en,es,fr,it,nl,pl,pt,ru,uk,zh`). Every new UI key: the `imports.*` namespace in all 11 catalogues (`internal/test/i18ntest` enforces parity; `{var}` placeholder sets must match per key). Translations are real translations, not English copied.
- **Access URL secrecy (spec Part 1/5):** the SimpleFIN access URL (which embeds HTTP Basic credentials) is received per request, used, and discarded. It is NEVER written to any table, NEVER passed to `reqctx.AddLogAttr`, NEVER included in an error message or a run's `errors` list, and NEVER returned by any endpoint other than `claim-setup-token` (which returns it to the caller who owns it). A regression test asserts this.
- Threat-model copy rule: the UI describes the scheme as encrypted at rest under a passphrase the server never learns; it NEVER says "end-to-end encrypted". The unlock prompt states that a forgotten passphrase is recovered by reconnecting with a fresh setup token, and offers "Forget this device" and "Reconnect" inline. No passphrase hint is stored anywhere.
- Logs: static messages, UUIDs/counts/statuses only — never a payee, description, account name, access URL, setup token, or ciphertext.
- Rate limits: `ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN` (default `5`) and `ECONUMO_RATE_LIMIT_SYNC` (default `10`), per user per `ECONUMO_RATE_LIMIT_WINDOW`, every request counts; 429 uses the frozen envelope. Both registered in `config.Load`'s strict table, `.env.example`, `CLAUDE.md`, and the apiparity harness config.
- Read-only users (402 on POST) get no allowlist entry for any new route.
- Analytics: `useSyncImportSource` fires `trackEvent(METRICS.IMPORT_SYNC, { trigger: 'manual', imported, matched })` on success; connecting fires the existing `IMPORT_SOURCE_CONNECT` (`{ provider: 'simplefin' }`) through `useCreateImportSource`. `metrics-coverage.test.ts` must pass without adding to `NOT_WIRED`.
- Comments: only *why*/non-obvious business rules; no references to PHP/Symfony; keep swag `// @…` blocks complete on every handler and run `make swagger` whenever a handler annotation changes (the `go-lint` docs-fresh check fails otherwise).
- `docs/regression-test-plan.md` MUST be updated in the same PR for every user-observable change.
- Run Go from the repo root with `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin`; frontend commands run in `web/` via `pnpm` (`pnpm --dir web exec tsc -b` for the type check that includes tests). Never `cd` out of the worktree; use repo-root-relative paths.
- Branch: work on the current worktree branch `feature/transaction-import-simplefin`; the PR targets `feature/transaction-import`.
- Commit trailer on every commit: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

---

## File map

| Path | Responsibility |
|---|---|
| `internal/infra/storage/migrations/{sqlite,pgsql}/20260907000000.sql` (create) | reshape `import_credential_keys` (spec columns), extend `import_runs` (`queued_count`, `amounts_updated_count`, `trigger`, `errors`) |
| `internal/infra/storage/migrations/migrations_import_test.go` (modify) | column probes for the new shape |
| `internal/infra/storage/sqlc/query/{sqlite,pgsql}/imports.sql` (modify) | run queries (new columns), `UpdateImportSource`, credential-key upsert/get/delete, run lists, links-by-run |
| `internal/model/imports.go` (modify) | `ImportRun` new fields, `ImportRunError`, `ImportCredentialKey`, `ExternalAccount`, `ExternalTransaction`, `ImportRunTriggerManual` |
| `internal/model/imports_dto.go` (modify) | stage-3 DTOs + `Validate()`; `CreateImportSourceRequest` accepts `simplefin` + `credentialCiphertext`; `ImportSourceResult` gains `lastSyncedAt`/`credentialCiphertext`; `ImportRunResult` extended |
| `internal/imports/repository.go`, `internal/imports/repo/{repo,sqlite,pgsql}.go` (modify) | new repository methods |
| `internal/test/fixture/imports.go` (modify) | `ImportRun` + `ImportCredentialKey` builders, `ImportSource.CredentialCiphertext` |
| `internal/shared/errs/codes.go`, `locales/*.json` (modify) | 4 new codes + catalogue entries |
| `internal/imports/provider.go` (create) | `Credential`, `FetchRequest`, `FetchResult`, `Provider`, `SetupTokenClaimer`, sentinel errors, `Service.RegisterProvider` |
| `internal/imports/simplefin/client.go` (create) | the SimpleFIN HTTP client (claim, list, fetch, error list, pending filter) |
| `internal/imports/simplefin/client_test.go` (create) | `httptest`-backed protocol tests |
| `internal/imports/simplefinevent.go` (create) | `ParseSimpleFINEvent` (stored envelope → `model.IngestEvent`) |
| `internal/imports/ports.go` (modify) | `TransactionCreator` → `TransactionWriter` (+`UpdateTransaction`), new rate scopes |
| `internal/imports/ingest.go` (modify) | `parse` gains `ctx` + the `simplefin` case; `place` gains amount correction |
| `internal/imports/credential.go` (create) | `ClaimSetupToken`, `GetCredentialKey`, `SetCredentialKey`, `ListExternalAccounts` |
| `internal/imports/sync.go` (create) | `Sync` (the pull run) |
| `internal/imports/run.go` (create) | `GetRunList`, `GetRun`, `runResult` mapper |
| `internal/imports/source.go`, `service.go` (modify) | reconnect overwrites the ciphertext; `sourceResult` carries `lastSyncedAt`/`credentialCiphertext` |
| `internal/imports/accountlink.go` (modify) | optional `externalName` on link/ignore |
| `internal/imports/{credential,sync,run}_test.go`, `ingest_test.go` (modify/create) | service tests incl. the access-URL regression |
| `internal/imports/api/{credential,sync,run}.go`, `routes.go`, `source.go` (create/modify) | HTTP edge for the 7 new routes |
| `internal/server/server.go`, `internal/server/glue_imports.go` (modify) | `Seams.ImportProviders`, `TransactionWriter` adapter, provider registration |
| `internal/config/config.go`, `config_test.go`, `.env.example` (modify) | `RateLimitClaimSetupToken`, `RateLimitSync` |
| `internal/test/apiparity/{harness.go,fixture.go,catalogue_import.go}` + goldens, `internal/test/apiparity/stubprovider.go` (create) | scenario coverage with a deterministic stub provider |
| `web/src/lib/importCrypto.ts`, `importCrypto.test.ts` (create), `web/src/test/setup.ts`, `web/package.json` (modify) | credential crypto + test env |
| `web/src/api/dto/imports.ts`, `web/src/api/imports.ts`, `web/src/features/imports/queries.ts`, `web/src/app/queryKeys.ts`, `web/src/lib/metrics.ts` (modify) | API client, hooks, keys, `IMPORT_SYNC` |
| `web/src/features/imports/{SimpleFINPage,SimpleFINConnect,SimpleFINUnlock,ImportRunSummary,ImportRunListPage,ImportRunPage}.tsx` (create), `ImportCards.tsx`, `ImportsDataPage.tsx`, `web/src/app/{routes.tsx,router-pages.ts}`, `web/src/features/settings/SettingsPage.tsx` (modify) | SPA surface |
| `web/src/test/fixtures.ts` (modify) | msw handlers for the new routes |
| `CLAUDE.md`, `docs/regression-test-plan.md` (modify) | docs |

---

### Task 1: Migration — reshape `import_credential_keys`, extend `import_runs`

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260907000000.sql`, `internal/infra/storage/migrations/pgsql/20260907000000.sql`
- Modify: `internal/infra/storage/migrations/migrations_import_test.go`
- Modify: `internal/infra/storage/sqlc/query/sqlite/imports.sql`, `internal/infra/storage/sqlc/query/pgsql/imports.sql` (run queries only)
- Modify: `internal/model/imports.go`, `internal/imports/repo/repo.go`, `internal/test/fixture/imports.go`
- Test: `internal/infra/storage/migrations/migrations_import_test.go`, `internal/imports/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: stage-1 `import_runs`/`import_credential_keys` DDL (`20260901000000.sql`), `runRow`/`insertRunParams`/`updateRunParams` aliases in `internal/imports/repo/repo.go`.
- Produces:
  ```go
  // internal/model/imports.go
  const ImportRunTriggerManual = "manual"
  type ImportRunError struct {
      ExternalAccountId string `json:"externalAccountId"` // "" for a run-level (bridge) error
      Message           string `json:"message"`
  }
  // ImportRun gains: QueuedCount int; AmountsUpdatedCount int; Trigger string; Errors []ImportRunError
  ```
  `import_runs.errors` is the JSON encoding of `Errors` (`"[]"` when empty); the repo marshals/unmarshals.

`import_credential_keys` shipped in stage 1 with a placeholder shape (`key_ciphertext`) and has never been written to; the spec (Part 2) wants `wrapped_data_key`, `kdf`, `created_at`, `updated_at`. Dropping and recreating is safe because the table is empty on every instance.

- [ ] **Step 1: Write the failing migration test**

In `internal/infra/storage/migrations/migrations_import_test.go`, change the two probes inside `TestMigration20260901_ImportTablesAndTokenScope` (the `import_credential_keys` and `import_runs` lines) to the new shape and add the sqlite/pgsql-agnostic default probe:

```go
"SELECT user_id, wrapped_data_key, kdf, created_at, updated_at FROM import_credential_keys WHERE 1 = 0",
"SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at FROM import_runs WHERE 1 = 0",
```

Add a new test in the same file:

```go
func TestMigration20260907_ImportRunDefaults(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	user := f.User(fixture.User{})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Provider: model.ImportProviderSimpleFIN, Name: "Bank"})
	_, err := db.Raw.ExecContext(ctx, db.Rebind(
		"INSERT INTO import_runs (id, user_id, source_id, provider, params, status, started_at) VALUES (?, ?, ?, ?, ?, ?, ?)"),
		"0f000000-0000-0000-0000-000000000001", user, src, model.ImportProviderSimpleFIN, "{}", model.ImportRunStatusRunning, time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	var queued, updated int64
	var trigger, errs string
	if err := db.Raw.QueryRowContext(ctx, db.Rebind("SELECT queued_count, amounts_updated_count, trigger, errors FROM import_runs WHERE id = ?"),
		"0f000000-0000-0000-0000-000000000001").Scan(&queued, &updated, &trigger, &errs); err != nil {
		t.Fatalf("select run: %v", err)
	}
	if queued != 0 || updated != 0 || trigger != "manual" || errs != "[]" {
		t.Fatalf("defaults = (%d, %d, %q, %q), want (0, 0, \"manual\", \"[]\")", queued, updated, trigger, errs)
	}
}
```

(Match the file's existing imports/helpers — it already uses `dbtest`, `fixture`, `model`; add `time` if missing. If `fixture.User`/`f.User` are named differently in `internal/test/fixture`, use the builder that file actually exposes — check `internal/test/fixture/user.go`.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin && go test ./internal/infra/storage/migrations/ -run 'TestMigration20260901|TestMigration20260907' -v`
Expected: FAIL — `no such column: wrapped_data_key` / `queued_count`.

- [ ] **Step 3: Write the sqlite migration**

`internal/infra/storage/migrations/sqlite/20260907000000.sql`:

```sql
-- Stage 3 (SimpleFIN): the credential-key table gets its real shape (one
-- wrapped data key + KDF params per user; the stage-1 placeholder column was
-- never written), and runs learn the pull-side counters and per-account errors.
DROP TABLE import_credential_keys;
CREATE TABLE import_credential_keys
(
    user_id          TEXT NOT NULL
    , wrapped_data_key TEXT NOT NULL
    , kdf              TEXT NOT NULL
    , created_at       DATETIME NOT NULL
    , updated_at       DATETIME NOT NULL
    , PRIMARY KEY (user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

ALTER TABLE import_runs ADD COLUMN queued_count INTEGER DEFAULT 0 NOT NULL;
ALTER TABLE import_runs ADD COLUMN amounts_updated_count INTEGER DEFAULT 0 NOT NULL;
ALTER TABLE import_runs ADD COLUMN trigger TEXT DEFAULT 'manual' NOT NULL;
ALTER TABLE import_runs ADD COLUMN errors TEXT DEFAULT '[]' NOT NULL;
CREATE INDEX IDX_import_runs_user_started ON import_runs (user_id, started_at);
```

`internal/infra/storage/migrations/pgsql/20260907000000.sql`:

```sql
DROP TABLE import_credential_keys
;
CREATE TABLE import_credential_keys
(
    user_id          UUID NOT NULL
    , wrapped_data_key TEXT NOT NULL
    , kdf              TEXT NOT NULL
    , created_at       TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at       TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
)
;
ALTER TABLE import_runs ADD COLUMN queued_count BIGINT DEFAULT 0 NOT NULL
;
ALTER TABLE import_runs ADD COLUMN amounts_updated_count BIGINT DEFAULT 0 NOT NULL
;
ALTER TABLE import_runs ADD COLUMN trigger TEXT DEFAULT 'manual' NOT NULL
;
ALTER TABLE import_runs ADD COLUMN errors TEXT DEFAULT '[]' NOT NULL
;
CREATE INDEX IDX_import_runs_user_started ON import_runs (user_id, started_at)
;
```

Check how the existing pgsql migration files terminate statements (`sed -n 1,40p internal/infra/storage/migrations/pgsql/20260901000000.sql`) and match that style exactly.

- [ ] **Step 4: Update the run queries in both dialects**

sqlite `imports.sql` — replace the three run queries:

```sql
-- name: InsertImportRun :exec
INSERT INTO import_runs (id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetImportRunByID :one
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at
FROM import_runs
WHERE id = ?;

-- name: UpdateImportRun :exec
UPDATE import_runs
SET status = ?, imported_count = ?, matched_count = ?, skipped_count = ?, failed_count = ?, queued_count = ?, amounts_updated_count = ?, errors = ?, finished_at = ?
WHERE id = ?;
```

pgsql `imports.sql` — same three with `$1…$16` / `$1…$10` placeholders and the `;` on its own line, matching the file's existing style.

Regenerate: `go generate ./internal/infra/storage/sqlc/` (check `internal/infra/storage/sqlc/sqlc.yaml` for the exact `//go:generate` — `sqlc generate` pinned via go.mod; run from the repo root).

- [ ] **Step 5: Extend the model and the repo mappers**

`internal/model/imports.go` — next to the run status consts:

```go
const ImportRunTriggerManual = "manual"

// ImportRunError is one entry of a run's error list: a bridge-level message
// (ExternalAccountId "") or a per-account failure that left the rest of the
// run intact.
type ImportRunError struct {
	ExternalAccountId string `json:"externalAccountId"`
	Message           string `json:"message"`
}
```

Add to `ImportRun`: `QueuedCount int`, `AmountsUpdatedCount int`, `Trigger string`, `Errors []ImportRunError`.

`internal/imports/repo/repo.go` — `InsertRun`/`UpdateRun` marshal `run.Errors` (nil → `[]`), `runFromRow` unmarshals:

```go
func encodeRunErrors(errs []model.ImportRunError) string {
	if len(errs) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(errs)
	return string(b)
}

func decodeRunErrors(raw string) ([]model.ImportRunError, error) {
	out := []model.ImportRunError{}
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
```

Pass `QueuedCount: int64(run.QueuedCount), AmountsUpdatedCount: int64(run.AmountsUpdatedCount), Trigger: run.Trigger, Errors: encodeRunErrors(run.Errors)` in `InsertRun`; in `UpdateRun` the same minus `Trigger`; in `runFromRow` fill `QueuedCount`, `AmountsUpdatedCount`, `Trigger`, `Errors` (via `decodeRunErrors`). `InsertRun` must default `Trigger` to `model.ImportRunTriggerManual` when empty so stage-2 call sites (`convertQueued`) keep working unchanged.

- [ ] **Step 6: Fixture builder**

`internal/test/fixture/imports.go` — add:

```go
type ImportRun struct {
	ID, UserID, SourceID, Provider, Params, Status string
	ImportedCount, MatchedCount, SkippedCount, FailedCount, QueuedCount, AmountsUpdatedCount int
	Errors     string // JSON, default "[]"
	StartedAt  time.Time
	FinishedAt *time.Time
}

func (b *Builder) ImportRun(r ImportRun) string {
	id := b.orNewID(r.ID)
	if r.Params == "" {
		r.Params = "{}"
	}
	if r.Status == "" {
		r.Status = model.ImportRunStatusCompleted
	}
	if r.Errors == "" {
		r.Errors = "[]"
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = b.now()
	}
	b.insert(`INSERT INTO import_runs (id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, r.UserID, r.SourceID, r.Provider, r.Params, r.Status, r.ImportedCount, r.MatchedCount, r.SkippedCount, r.FailedCount,
		r.QueuedCount, r.AmountsUpdatedCount, model.ImportRunTriggerManual, r.Errors, r.StartedAt, r.FinishedAt)
	return id
}
```

(Match the builder's real helper names — `orNewID`, `now`, `insert` — and the way other builders pass nullable values; if `insert` cannot take a `*time.Time` directly, use the file's existing nullable helper.)

- [ ] **Step 7: Repo round-trip test**

`internal/imports/repo/repo_integration_test.go` already has `TestRepo_RunRoundTrip` (uses `setup(t)` → `(*importsrepo.Repo, *dbtest.DB)`, `newSource`, `userA`, `fixedTime`). Add a sibling test right after it:

```go
func TestRepo_RunRoundTripStage3Columns(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000031")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	run := &model.ImportRun{
		ID: vo.NewId(), UserID: src.UserID, SourceID: src.ID, Provider: src.Provider, Params: `{"startDate":"2026-08-01","endDate":"2026-08-20"}`,
		Status: model.ImportRunStatusRunning, StartedAt: fixedTime,
	}
	if err := repo.InsertRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	// an unset Trigger persists as the manual default and errors read back as an empty, non-nil slice
	if got.Trigger != model.ImportRunTriggerManual || got.Errors == nil || len(got.Errors) != 0 {
		t.Fatalf("fresh run: trigger=%q errors=%#v", got.Trigger, got.Errors)
	}
	run.Status = model.ImportRunStatusPartial
	run.QueuedCount, run.AmountsUpdatedCount = 2, 1
	run.Errors = []model.ImportRunError{{ExternalAccountId: "acc-1", Message: "boom"}}
	finished := fixedTime.Add(time.Minute)
	run.FinishedAt = &finished
	if err := repo.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.ImportRunStatusPartial || got.QueuedCount != 2 || got.AmountsUpdatedCount != 1 ||
		len(got.Errors) != 1 || got.Errors[0].ExternalAccountId != "acc-1" || got.Errors[0].Message != "boom" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}
```

(`ImportRun.FinishedAt` is already `*time.Time` in the stage-1 model; check `internal/model/imports.go` and match the existing `TestRepo_RunRoundTrip` if it names the field differently.)

- [ ] **Step 8: Run the tests**

Run: `go test ./internal/infra/storage/migrations/ ./internal/imports/... ./internal/test/... 2>&1 | tail -20`
Expected: PASS (goldens are unaffected — `ImportRunResult` is extended in Task 3).

- [ ] **Step 9: Commit**

```bash
git add internal/infra/storage internal/model/imports.go internal/imports/repo internal/test/fixture/imports.go
git commit -m "feat(imports): migration 20260907 — credential-key shape, run counters/errors"
```

---

### Task 2: Repository — credential keys, `UpdateSource`, run lists, links-by-run

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/imports.sql`, `internal/infra/storage/sqlc/query/pgsql/imports.sql`
- Modify: `internal/model/imports.go` (`ImportCredentialKey`)
- Modify: `internal/imports/repository.go`, `internal/imports/repo/repo.go`, `internal/imports/repo/sqlite.go`, `internal/imports/repo/pgsql.go`
- Modify: `internal/test/fixture/imports.go` (`ImportSource.CredentialCiphertext`, `ImportCredentialKey` builder)
- Test: `internal/imports/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: Task 1's `ImportRun` fields and the `runFromRow` mapper.
- Produces (added to `imports.Repository`):
  ```go
  UpdateSource(ctx context.Context, s *model.ImportSource) error            // name, credential_ciphertext, status, last_synced_at, updated_at
  UpsertCredentialKey(ctx context.Context, k *model.ImportCredentialKey) error
  GetCredentialKey(ctx context.Context, userID vo.Id) (*model.ImportCredentialKey, error) // (nil, nil) when none
  DeleteCredentialKey(ctx context.Context, userID vo.Id) error
  ListRunsByUser(ctx context.Context, userID vo.Id, sourceID *vo.Id, limit int) ([]model.ImportRun, error) // newest first
  ListLinksByRun(ctx context.Context, runID vo.Id) ([]model.ImportTransactionLink, error)             // by imported_at, id
  ```
  ```go
  // internal/model/imports.go
  type ImportCredentialKey struct {
      UserID         vo.Id
      WrappedDataKey string
      KDF            string // JSON, opaque to the server
      CreatedAt      time.Time
      UpdatedAt      time.Time
  }
  ```

- [ ] **Step 1: Write the failing repo tests**

Append to `internal/imports/repo/repo_integration_test.go`:

```go
func TestRepo_CredentialKeyUpsert(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	uid := vo.MustParseId(userA)
	got, err := repo.GetCredentialKey(ctx, uid)
	if err != nil || got != nil {
		t.Fatalf("no key yet: got %+v, err %v", got, err)
	}
	k := &model.ImportCredentialKey{UserID: uid, WrappedDataKey: "v1:iv:ct", KDF: `{"alg":"PBKDF2-SHA256"}`, CreatedAt: fixedTime, UpdatedAt: fixedTime}
	if err := repo.UpsertCredentialKey(ctx, k); err != nil {
		t.Fatal(err)
	}
	k.WrappedDataKey, k.UpdatedAt = "v1:iv2:ct2", fixedTime.Add(time.Hour)
	if err := repo.UpsertCredentialKey(ctx, k); err != nil {
		t.Fatalf("second upsert must update in place: %v", err)
	}
	got, err = repo.GetCredentialKey(ctx, uid)
	if err != nil || got == nil || got.WrappedDataKey != "v1:iv2:ct2" || !got.CreatedAt.Equal(fixedTime) || !got.UpdatedAt.Equal(fixedTime.Add(time.Hour)) {
		t.Fatalf("after upsert: %+v, err %v", got, err)
	}
	if err := repo.DeleteCredentialKey(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetCredentialKey(ctx, uid); got != nil {
		t.Fatal("key must be gone after delete")
	}
	if err := repo.DeleteCredentialKey(ctx, uid); err != nil {
		t.Fatalf("deleting a missing key is a no-op: %v", err)
	}
}

func TestRepo_UpdateSource(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	src := newSource("0c000000-0000-0000-0000-000000000032")
	if err := repo.InsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	ct := "v1:a:b"
	synced := fixedTime.Add(time.Hour)
	src.Name, src.CredentialCiphertext, src.LastSyncedAt, src.UpdatedAt = "Renamed", &ct, &synced, synced
	if err := repo.UpdateSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetSource(ctx, src.ID)
	if err != nil || got.Name != "Renamed" || got.CredentialCiphertext == nil || *got.CredentialCiphertext != ct || got.LastSyncedAt == nil || !got.LastSyncedAt.Equal(synced) {
		t.Fatalf("update lost data: %+v, err %v", got, err)
	}
}

func TestRepo_ListRunsByUserAndLinksByRun(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	srcA := f.ImportSource(fixture.ImportSource{UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank"})
	srcB := f.ImportSource(fixture.ImportSource{UserID: userA, Provider: model.ImportProviderAppleWallet, Name: "iPhone"})
	old := f.ImportRun(fixture.ImportRun{UserID: userA, SourceID: srcA, Provider: model.ImportProviderSimpleFIN, StartedAt: fixedTime})
	newer := f.ImportRun(fixture.ImportRun{UserID: userA, SourceID: srcA, Provider: model.ImportProviderSimpleFIN, StartedAt: fixedTime.Add(time.Hour)})
	other := f.ImportRun(fixture.ImportRun{UserID: userA, SourceID: srcB, Provider: model.ImportProviderAppleWallet, StartedAt: fixedTime.Add(2 * time.Hour)})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcA, RunID: newer, ExternalAccountID: "acc-1", ExternalTransactionID: "t-1", Status: "queued"})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: srcA, RunID: old, ExternalAccountID: "acc-1", ExternalTransactionID: "t-0", Status: "queued"})

	all, err := repo.ListRunsByUser(ctx, vo.MustParseId(userA), nil, 50)
	if err != nil || len(all) != 3 || all[0].ID.String() != other || all[1].ID.String() != newer || all[2].ID.String() != old {
		t.Fatalf("all runs newest first: %v (%d) err %v", all, len(all), err)
	}
	sid := vo.MustParseId(srcA)
	bySource, err := repo.ListRunsByUser(ctx, vo.MustParseId(userA), &sid, 1)
	if err != nil || len(bySource) != 1 || bySource[0].ID.String() != newer {
		t.Fatalf("by source, limit 1: %v err %v", bySource, err)
	}
	links, err := repo.ListLinksByRun(ctx, vo.MustParseId(newer))
	if err != nil || len(links) != 1 || links[0].ExternalTransactionID != "t-1" {
		t.Fatalf("links by run: %v err %v", links, err)
	}
}
```

Check `fixture.ImportTransactionLink` for a `RunID string` field; if the stage-1 builder lacks it, add `RunID string` (`""` → NULL) exactly like its existing `TransactionID` handling.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/imports/repo/ -run 'TestRepo_CredentialKey|TestRepo_UpdateSource|TestRepo_ListRuns' 2>&1 | head`
Expected: compile error — the methods don't exist.

- [ ] **Step 3: Add the queries (both dialects)**

sqlite `imports.sql` (append):

```sql
-- name: UpdateImportSource :exec
UPDATE import_sources SET name = ?, credential_ciphertext = ?, status = ?, last_synced_at = ?, updated_at = ? WHERE id = ?;

-- name: UpsertImportCredentialKey :exec
INSERT INTO import_credential_keys (user_id, wrapped_data_key, kdf, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET wrapped_data_key = excluded.wrapped_data_key, kdf = excluded.kdf, updated_at = excluded.updated_at;

-- name: GetImportCredentialKey :one
SELECT user_id, wrapped_data_key, kdf, created_at, updated_at
FROM import_credential_keys
WHERE user_id = ?;

-- name: DeleteImportCredentialKey :exec
DELETE FROM import_credential_keys WHERE user_id = ?;

-- name: ListImportRunsByUser :many
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at
FROM import_runs
WHERE user_id = ?
ORDER BY started_at DESC, id DESC
LIMIT ?;

-- name: ListImportRunsBySource :many
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at
FROM import_runs
WHERE user_id = ? AND source_id = ?
ORDER BY started_at DESC, id DESC
LIMIT ?;

-- name: ListImportTransactionLinksByRun :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE run_id = ?
ORDER BY imported_at, id;
```

pgsql: identical with `$N` placeholders and each `;` on its own line. `ListImportTransactionLinksByRun`'s parameter will be generated as `*string` (nullable column) — mirror the existing `ListImportTransactionLinksByTransaction` handling.

Run `go generate ./internal/infra/storage/sqlc/`.

- [ ] **Step 4: Extend the querier + both adapters**

`repo.go` — add aliases:

```go
updateSourceParams        = sqlitegen.UpdateImportSourceParams
credentialKeyRow          = sqlitegen.ImportCredentialKey
upsertCredentialKeyParams = sqlitegen.UpsertImportCredentialKeyParams
runsByUserParams          = sqlitegen.ListImportRunsByUserParams
runsBySourceParams        = sqlitegen.ListImportRunsBySourceParams
```

querier interface additions:

```go
UpdateImportSource(ctx context.Context, db backend.DBTX, p updateSourceParams) error
UpsertImportCredentialKey(ctx context.Context, db backend.DBTX, p upsertCredentialKeyParams) error
GetImportCredentialKey(ctx context.Context, db backend.DBTX, userID string) (credentialKeyRow, error)
DeleteImportCredentialKey(ctx context.Context, db backend.DBTX, userID string) error
ListImportRunsByUser(ctx context.Context, db backend.DBTX, p runsByUserParams) ([]runRow, error)
ListImportRunsBySource(ctx context.Context, db backend.DBTX, p runsBySourceParams) ([]runRow, error)
ListImportTransactionLinksByRun(ctx context.Context, db backend.DBTX, runID *string) ([]linkRow, error)
```

`sqlite.go`: passthrough to `sqlitegen.New(db).<Method>(ctx, …)` like the existing methods. `pgsql.go`: whole-struct conversion, e.g.

```go
func (pgsqlQuerier) ListImportRunsByUser(ctx context.Context, db backend.DBTX, p runsByUserParams) ([]runRow, error) {
	rows, err := pgsqlgen.New(db).ListImportRunsByUser(ctx, pgsqlgen.ListImportRunsByUserParams(p))
	if err != nil {
		return nil, err
	}
	out := make([]runRow, len(rows))
	for i, row := range rows {
		out[i] = runRow(row)
	}
	return out, nil
}
```

Repo methods:

```go
func (r *Repo) UpdateSource(ctx context.Context, s *model.ImportSource) error {
	return r.q.UpdateImportSource(ctx, r.db(ctx), updateSourceParams{
		Name: s.Name, CredentialCiphertext: s.CredentialCiphertext, Status: s.Status,
		LastSyncedAt: s.LastSyncedAt, UpdatedAt: s.UpdatedAt, ID: s.ID.String(),
	})
}

func (r *Repo) UpsertCredentialKey(ctx context.Context, k *model.ImportCredentialKey) error {
	return r.q.UpsertImportCredentialKey(ctx, r.db(ctx), upsertCredentialKeyParams{
		UserID: k.UserID.String(), WrappedDataKey: k.WrappedDataKey, Kdf: k.KDF, CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt,
	})
}

func (r *Repo) GetCredentialKey(ctx context.Context, userID vo.Id) (*model.ImportCredentialKey, error) {
	row, err := r.q.GetImportCredentialKey(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &model.ImportCredentialKey{UserID: userID, WrappedDataKey: row.WrappedDataKey, KDF: row.Kdf, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *Repo) DeleteCredentialKey(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteImportCredentialKey(ctx, r.db(ctx), userID.String())
}

func (r *Repo) ListRunsByUser(ctx context.Context, userID vo.Id, sourceID *vo.Id, limit int) ([]model.ImportRun, error) {
	var rows []runRow
	var err error
	if sourceID == nil {
		rows, err = r.q.ListImportRunsByUser(ctx, r.db(ctx), runsByUserParams{UserID: userID.String(), Limit: int64(limit)})
	} else {
		rows, err = r.q.ListImportRunsBySource(ctx, r.db(ctx), runsBySourceParams{UserID: userID.String(), SourceID: sourceID.String(), Limit: int64(limit)})
	}
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportRun, 0, len(rows))
	for _, row := range rows {
		run, err := runFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, nil
}

func (r *Repo) ListLinksByRun(ctx context.Context, runID vo.Id) ([]model.ImportTransactionLink, error) {
	id := runID.String()
	rows, err := r.q.ListImportTransactionLinksByRun(ctx, r.db(ctx), &id)
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
```

(sqlc names the generated field for column `kdf` `Kdf`; the `LIMIT` parameter is `Limit int64` — check the generated `gen/sqlite/imports.sql.go` and use the exact names. If the pgsql generator types `created_at` as `time.Time` differently from sqlite, the existing source/run adapters already show the conversion shape to copy.)

- [ ] **Step 5: Fixture — `ImportCredentialKey` builder and `ImportSource.CredentialCiphertext`**

`internal/test/fixture/imports.go`: add `CredentialCiphertext string` to `ImportSource` (empty → `NULL`, else bound) and:

```go
type ImportCredentialKey struct {
	UserID, WrappedDataKey, KDF string
}

func (b *Builder) ImportCredentialKey(k ImportCredentialKey) {
	b.t.Helper()
	if k.KDF == "" {
		k.KDF = `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`
	}
	now := b.now()
	b.insert(`INSERT INTO import_credential_keys (user_id, wrapped_data_key, kdf, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		k.UserID, k.WrappedDataKey, k.KDF, now, now)
}
```

For the nullable ciphertext follow the file's `ImportTransactionLink` builder, which already turns `""` into `NULL` via a helper — reuse that helper rather than adding a second one.

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/imports/... ./internal/test/fixture/ 2>&1 | tail -5`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/storage/sqlc internal/model/imports.go internal/imports/repository.go internal/imports/repo internal/test/fixture/imports.go
git commit -m "feat(imports): credential-key, run-list and links-by-run repository methods"
```

---

### Task 3: Wire DTOs, error codes, and the `errors.import.*` catalogue entries

**Files:**
- Modify: `internal/model/imports_dto.go`
- Modify: `internal/shared/errs/codes.go`
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (`errors.import.*`)
- Test: `internal/model/imports_dto_test.go` (create), `internal/test/i18ntest` (existing guards)

**Interfaces:**
- Produces (all in `package model`, file `imports_dto.go`):
  ```go
  type ClaimSetupTokenRequest struct{ SetupToken string `json:"setupToken"` }   // Validate: non-blank
  type ClaimSetupTokenResult struct{ AccessUrl string `json:"accessUrl"` }

  // CreateImportSourceRequest gains CredentialCiphertext string `json:"credentialCiphertext"`;
  // Validate: provider ∈ {apple-wallet, simplefin}; simplefin requires a non-blank ciphertext.

  type GetImportCredentialKeyResult struct {
      WrappedDataKey string `json:"wrappedDataKey"` // "" when the user has no key
      Kdf            string `json:"kdf"`
      UpdatedAt      string `json:"updatedAt"`
  }
  type SetImportCredentialKeyRequest struct {
      WrappedDataKey string `json:"wrappedDataKey"`
      Kdf            string `json:"kdf"`
  } // Validate: both non-blank, each ≤ 4096 chars (common.too_long)

  type ListExternalAccountsRequest struct {
      SourceId  string `json:"sourceId"`
      AccessUrl string `json:"accessUrl"`
  } // Validate: both non-blank
  type ExternalAccountResult struct {
      ExternalAccountId string `json:"externalAccountId"`
      ExternalName      string `json:"externalName"`
      ExternalCurrency  string `json:"externalCurrency"`
      Balance           string `json:"balance"`
      OrgName           string `json:"orgName"`
      State             string `json:"state"`     // mapped | ignored | unmapped
      AccountId         string `json:"accountId"` // "" unless mapped
  }
  type ListExternalAccountsResult struct{ Items []ExternalAccountResult `json:"items"` }

  type SyncImportSourceRequest struct {
      SourceId  string `json:"sourceId"`
      AccessUrl string `json:"accessUrl"`
      StartDate string `json:"startDate"` // 2006-01-02, required
      EndDate   string `json:"endDate"`   // 2006-01-02, optional (today)
  } // Validate: sourceId/accessUrl/startDate non-blank; dates parse as 2006-01-02 (common.invalid_datetime)
  type SyncImportSourceResult struct {
      Run      ImportRunResult         `json:"run"`
      Accounts []ExternalAccountResult `json:"accounts"`
  }

  type ImportRunResult struct {
      Id                  string           `json:"id"`
      SourceId            string           `json:"sourceId"`
      Provider            string           `json:"provider"`
      Status              string           `json:"status"`
      Trigger             string           `json:"trigger"`
      ImportedCount       int              `json:"importedCount"`
      MatchedCount        int              `json:"matchedCount"`
      AmountsUpdatedCount int              `json:"amountsUpdatedCount"`
      QueuedCount         int              `json:"queuedCount"`
      SkippedCount        int              `json:"skippedCount"`
      FailedCount         int              `json:"failedCount"`
      Errors              []ImportRunError `json:"errors"` // never null
      StartedAt           string           `json:"startedAt"`
      FinishedAt          string           `json:"finishedAt"` // "" while running
  }
  type GetImportRunListResult struct{ Items []ImportRunResult `json:"items"` }
  type ImportRunLinkResult struct {
      Id                    string `json:"id"`
      ExternalAccountId     string `json:"externalAccountId"`
      ExternalTransactionId string `json:"externalTransactionId"`
      TransactionId         string `json:"transactionId"` // "" for queued/skipped and for tombstones
      Status                string `json:"status"`
      ExternalPayee         string `json:"externalPayee"`
      ExternalAmount        string `json:"externalAmount"`
      ExternalCurrency      string `json:"externalCurrency"`
      ExternalPostedAt      string `json:"externalPostedAt"`
  }
  type GetImportRunResult struct {
      Item  ImportRunResult       `json:"item"`
      Links []ImportRunLinkResult `json:"links"` // never null
  }

  // ImportSourceResult gains LastSyncedAt string `json:"lastSyncedAt"` ("" unset)
  // and CredentialCiphertext string `json:"credentialCiphertext"` ("" for push providers).
  // LinkImportAccountRequest and ImportAccountActionRequest gain optional ExternalName string `json:"externalName"`.
  ```
  Error codes (`internal/shared/errs/codes.go`):
  ```go
  CodeImportSetupTokenRejected  = "import.setup_token_rejected"
  CodeImportProviderUnavailable = "import.provider_unavailable"
  CodeImportSyncRangeInvalid    = "import.sync_range_invalid"
  CodeImportAccessUrlInvalid    = "import.access_url_invalid"
  ```

- [ ] **Step 1: Write the failing DTO tests**

Create `internal/model/imports_dto_test.go`:

```go
package model_test

import (
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	out := map[string]string{}
	for _, f := range verr.Fields {
		out[f.Key] = f.Code
	}
	return out
}

func TestCreateImportSourceRequest_Validate(t *testing.T) {
	if err := (model.CreateImportSourceRequest{Provider: "apple-wallet", Name: "iPhone"}).Validate(); err != nil {
		t.Fatalf("apple-wallet needs no ciphertext: %v", err)
	}
	if err := (model.CreateImportSourceRequest{Provider: "simplefin", Name: "Bank", CredentialCiphertext: "v1:a:b"}).Validate(); err != nil {
		t.Fatalf("simplefin with ciphertext: %v", err)
	}
	codes := fieldCodes(t, model.CreateImportSourceRequest{Provider: "simplefin", Name: "Bank"}.Validate())
	if codes["credentialCiphertext"] != errs.CodeIsBlank {
		t.Fatalf("simplefin without ciphertext: %v", codes)
	}
	codes = fieldCodes(t, model.CreateImportSourceRequest{Provider: "csv", Name: "x"}.Validate())
	if codes["provider"] != errs.CodeImportProviderUnsupported {
		t.Fatalf("csv: %v", codes)
	}
}

func TestSyncImportSourceRequest_Validate(t *testing.T) {
	ok := model.SyncImportSourceRequest{SourceId: "s", AccessUrl: "https://u:p@bridge.example/x", StartDate: "2026-08-01"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("endDate is optional: %v", err)
	}
	codes := fieldCodes(t, model.SyncImportSourceRequest{SourceId: "s", AccessUrl: "u", StartDate: "08/01/2026", EndDate: "nope"}.Validate())
	if codes["startDate"] != errs.CodeInvalidDatetime || codes["endDate"] != errs.CodeInvalidDatetime {
		t.Fatalf("bad dates: %v", codes)
	}
	codes = fieldCodes(t, model.SyncImportSourceRequest{}.Validate())
	for _, k := range []string{"sourceId", "accessUrl", "startDate"} {
		if codes[k] != errs.CodeIsBlank {
			t.Fatalf("%s should be blank-checked: %v", k, codes)
		}
	}
}

func TestSetImportCredentialKeyRequest_Validate(t *testing.T) {
	long := make([]byte, 4097)
	for i := range long {
		long[i] = 'a'
	}
	codes := fieldCodes(t, model.SetImportCredentialKeyRequest{WrappedDataKey: string(long), Kdf: ""}.Validate())
	if codes["wrappedDataKey"] != errs.CodeTooLong || codes["kdf"] != errs.CodeIsBlank {
		t.Fatalf("codes: %v", codes)
	}
}

func TestClaimAndListRequests_Validate(t *testing.T) {
	if codes := fieldCodes(t, model.ClaimSetupTokenRequest{}.Validate()); codes["setupToken"] != errs.CodeIsBlank {
		t.Fatal(codes)
	}
	if codes := fieldCodes(t, model.ListExternalAccountsRequest{SourceId: "s"}.Validate()); codes["accessUrl"] != errs.CodeIsBlank {
		t.Fatal(codes)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/model/ -run 'ImportSource|SyncImport|CredentialKey|ClaimAnd' 2>&1 | head`
Expected: compile errors (undefined types).

- [ ] **Step 3: Implement the DTOs**

In `internal/model/imports_dto.go`:

```go
const importCredentialFieldMax = 4096

type CreateImportSourceRequest struct {
	Provider             string `json:"provider"`
	Name                 string `json:"name"`
	CredentialCiphertext string `json:"credentialCiphertext"`
}

func (r CreateImportSourceRequest) Validate() error {
	if err := requireNonBlank("provider", r.Provider, "name", r.Name); err != nil {
		return err
	}
	switch r.Provider {
	case ImportProviderAppleWallet:
		return nil
	case ImportProviderSimpleFIN:
		// a pull source is unusable without the encrypted access URL
		return requireNonBlank("credentialCiphertext", r.CredentialCiphertext)
	default:
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "provider", Message: "This import provider is not supported.", Code: errs.CodeImportProviderUnsupported})
	}
}

type ClaimSetupTokenRequest struct {
	SetupToken string `json:"setupToken"`
}

func (r ClaimSetupTokenRequest) Validate() error { return requireNonBlank("setupToken", r.SetupToken) }

type ClaimSetupTokenResult struct {
	AccessUrl string `json:"accessUrl"`
}

type GetImportCredentialKeyResult struct {
	WrappedDataKey string `json:"wrappedDataKey"`
	Kdf            string `json:"kdf"`
	UpdatedAt      string `json:"updatedAt"`
}

type SetImportCredentialKeyRequest struct {
	WrappedDataKey string `json:"wrappedDataKey"`
	Kdf            string `json:"kdf"`
}

func (r SetImportCredentialKeyRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{{"wrappedDataKey", r.WrappedDataKey}, {"kdf", r.Kdf}} {
		switch {
		case strings.TrimSpace(f.val) == "":
			fields = append(fields, blankField(f.key))
		case len(f.val) > importCredentialFieldMax:
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is too long.", Code: errs.CodeTooLong})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type ListExternalAccountsRequest struct {
	SourceId  string `json:"sourceId"`
	AccessUrl string `json:"accessUrl"`
}

func (r ListExternalAccountsRequest) Validate() error {
	return requireNonBlank("sourceId", r.SourceId, "accessUrl", r.AccessUrl)
}

type ExternalAccountResult struct {
	ExternalAccountId string `json:"externalAccountId"`
	ExternalName      string `json:"externalName"`
	ExternalCurrency  string `json:"externalCurrency"`
	Balance           string `json:"balance"`
	OrgName           string `json:"orgName"`
	State             string `json:"state"`
	AccountId         string `json:"accountId"`
}

type ListExternalAccountsResult struct {
	Items []ExternalAccountResult `json:"items"`
}

const ImportDateLayout = "2006-01-02"

type SyncImportSourceRequest struct {
	SourceId  string `json:"sourceId"`
	AccessUrl string `json:"accessUrl"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

func (r SyncImportSourceRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{{"sourceId", r.SourceId}, {"accessUrl", r.AccessUrl}, {"startDate", r.StartDate}} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, blankField(f.key))
		}
	}
	for _, f := range []struct{ key, val string }{{"startDate", r.StartDate}, {"endDate", r.EndDate}} {
		if strings.TrimSpace(f.val) == "" {
			continue
		}
		if _, err := time.Parse(ImportDateLayout, f.val); err != nil {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is not a valid datetime.", Code: errs.CodeInvalidDatetime})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type SyncImportSourceResult struct {
	Run      ImportRunResult         `json:"run"`
	Accounts []ExternalAccountResult `json:"accounts"`
}

type ImportRunResult struct {
	Id                  string           `json:"id"`
	SourceId            string           `json:"sourceId"`
	Provider            string           `json:"provider"`
	Status              string           `json:"status"`
	Trigger             string           `json:"trigger"`
	ImportedCount       int              `json:"importedCount"`
	MatchedCount        int              `json:"matchedCount"`
	AmountsUpdatedCount int              `json:"amountsUpdatedCount"`
	QueuedCount         int              `json:"queuedCount"`
	SkippedCount        int              `json:"skippedCount"`
	FailedCount         int              `json:"failedCount"`
	Errors              []ImportRunError `json:"errors"`
	StartedAt           string           `json:"startedAt"`
	FinishedAt          string           `json:"finishedAt"`
}

type GetImportRunListResult struct {
	Items []ImportRunResult `json:"items"`
}

type ImportRunLinkResult struct {
	Id                    string `json:"id"`
	ExternalAccountId     string `json:"externalAccountId"`
	ExternalTransactionId string `json:"externalTransactionId"`
	TransactionId         string `json:"transactionId"`
	Status                string `json:"status"`
	ExternalPayee         string `json:"externalPayee"`
	ExternalAmount        string `json:"externalAmount"`
	ExternalCurrency      string `json:"externalCurrency"`
	ExternalPostedAt      string `json:"externalPostedAt"`
}

type GetImportRunResult struct {
	Item  ImportRunResult       `json:"item"`
	Links []ImportRunLinkResult `json:"links"`
}
```

Add `LastSyncedAt string `json:"lastSyncedAt"`` and `CredentialCiphertext string `json:"credentialCiphertext"`` to `ImportSourceResult` (after `CreatedAt`); add `ExternalName string `json:"externalName"`` to `LinkImportAccountRequest` and `ImportAccountActionRequest` (not validated — optional). Add `"time"` to the imports. Any existing call site that constructs `ImportRunResult` positionally must be updated (there should be none; stage 2 uses keyed fields in `convertQueued` — grep `ImportRunResult{`).

- [ ] **Step 4: Error codes**

`internal/shared/errs/codes.go` — append after `CodeImportEventNotFailed` in the const block and in `AllCodes`:

```go
CodeImportSetupTokenRejected  = "import.setup_token_rejected"
CodeImportProviderUnavailable = "import.provider_unavailable"
CodeImportSyncRangeInvalid    = "import.sync_range_invalid"
CodeImportAccessUrlInvalid    = "import.access_url_invalid"
```

- [ ] **Step 5: Catalogue entries in all 11 languages**

`locales/en.json` → `errors.import` gains:

```json
"setup_token_rejected": "SimpleFIN rejected this setup token. It may have been claimed already — generate a new one in your SimpleFIN bridge.",
"provider_unavailable": "The bank bridge could not be reached. Try again in a few minutes.",
"sync_range_invalid": "The date range is invalid: the start must not be after the end, and the range must not exceed 400 days.",
"access_url_invalid": "The stored bank credential is not a valid access URL. Reconnect SimpleFIN with a new setup token."
```

Leave `errors.import.source_not_found` alone — it is only raised by the Apple Wallet ingest path; every stage-3 use case resolves sources through `ownedSource`, whose plain not-found error carries no code. Write real translations of the four strings in `de, es, fr, it, nl, pl, pt, ru, uk, zh` (the product name "SimpleFIN" stays as is). Keep every JSON file's existing key order; insert the new keys after `event_not_failed`.

- [ ] **Step 6: Run tests + guards**

Run: `go test ./internal/model/ ./internal/shared/... ./internal/test/i18ntest/ ./internal/imports/... 2>&1 | tail -8`
Expected: PASS (the `AllCodes` ↔ catalogue two-way guard and language parity pass).

- [ ] **Step 7: Commit**

```bash
git add internal/model/imports_dto.go internal/model/imports_dto_test.go internal/shared/errs/codes.go locales
git commit -m "feat(imports): stage-3 wire DTOs and error codes"
```

---

### Task 4: Provider seam + the SimpleFIN client

**Files:**
- Create: `internal/imports/provider.go`
- Create: `internal/imports/simplefin/client.go`, `internal/imports/simplefin/client_test.go`
- Modify: `internal/model/imports.go` (`ExternalAccount`, `ExternalTransaction`)
- Modify: `internal/imports/service.go` (`providers` map + `RegisterProvider`)

**Interfaces:**
- Produces:
  ```go
  // internal/model/imports.go
  type ExternalAccount struct {
      ID       string
      Name     string
      Currency string // ISO code as reported; may be a non-ISO string for crypto/custom, passed through
      Balance  string // decimal text as reported
      OrgName  string
  }
  type ExternalTransaction struct {
      ExternalAccountID string
      ID                string
      Amount            string // signed decimal text as reported ("-12.50")
      Posted            int64  // unix seconds
      Payee             string
      Description       string
      Raw               json.RawMessage // the provider's JSON for this transaction, stored verbatim
  }

  // internal/imports/provider.go
  type Credential struct{ AccessURL string }
  type FetchRequest struct{ StartDate, EndDate time.Time }
  type FetchResult struct {
      Accounts     []model.ExternalAccount
      Transactions []model.ExternalTransaction
      Warnings     []string // provider-level messages (SimpleFIN "errors"), surfaced on the run, never fatal
  }
  // Provider is the pull-side seam. SimpleFIN returns accounts and their
  // transactions in one document, so one call serves both.
  type Provider interface {
      ListAccounts(ctx context.Context, cred Credential) ([]model.ExternalAccount, error)
      FetchTransactions(ctx context.Context, cred Credential, req FetchRequest) (*FetchResult, error)
  }
  type SetupTokenClaimer interface {
      ClaimSetupToken(ctx context.Context, setupToken string) (accessURL string, err error)
  }
  var (
      ErrSetupTokenRejected  = errors.New("setup token rejected")   // 403 from the claim URL
      ErrProviderUnavailable = errors.New("provider unavailable")   // network, non-2xx, bad JSON
      ErrCredentialInvalid   = errors.New("credential invalid")     // access URL unparsable / not https / no userinfo
  )
  func (s *Service) RegisterProvider(name string, p Provider)  // also records p as a SetupTokenClaimer when it implements it

  // internal/imports/simplefin
  type Options struct {
      AllowHTTP bool          // tests only: accept http:// claim/access URLs
      Client    *http.Client  // nil → 60s timeout, no redirects
  }
  func New(opts Options) *Client   // implements imports.Provider + imports.SetupTokenClaimer
  ```

The SimpleFIN protocol (v1): a setup token is base64 of the claim URL; `POST <claimURL>` with an empty body returns the access URL as the body (`https://user:pass@bridge/simplefin`); `GET <accessURL>/accounts?start-date=<unix>&end-date=<unix>` returns `{"errors":[…],"accounts":[{"org":{"name",…},"id","name","currency","balance","available-balance","balance-date","transactions":[{"id","posted","amount","description","payee","memo","transacted_at","pending"}]}]}`. `balances-only=1` returns accounts without transactions. `errors` are plain strings in v1; tolerate objects `{message}` too. Transactions with `pending: true` or `posted == 0` are dropped before they reach the service (they change id/amount when they settle).

- [ ] **Step 1: Write the failing client tests**

`internal/imports/simplefin/client_test.go`:

```go
package simplefin_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/imports/simplefin"
)

const accountsDoc = `{"errors":["Connection to Big Bank may need attention"],"accounts":[{"org":{"domain":"bigbank.example","name":"Big Bank"},"id":"ACT-1","name":"Checking","currency":"USD","balance":"1000.10","available-balance":"990.00","balance-date":1756000000,"transactions":[
{"id":"TRN-1","posted":1755900000,"amount":"-12.50","description":"COFFEE SHOP","payee":"Blue Bottle","memo":"","transacted_at":1755899000},
{"id":"TRN-2","posted":1755910000,"amount":"2500.00","description":"PAYROLL","payee":"","memo":""},
{"id":"TRN-3","posted":0,"amount":"-4.00","description":"PENDING THING","pending":true}
]}]}`

func newBridge(t *testing.T) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/simplefin/claim/abc":
			w.Write([]byte("http://user:s3cret@" + r.Host + "/simplefin"))
		case r.Method == http.MethodPost && r.URL.Path == "/simplefin/claim/used":
			w.WriteHeader(http.StatusForbidden)
		case r.Method == http.MethodGet && r.URL.Path == "/simplefin/accounts":
			u, p, ok := r.BasicAuth()
			if !ok || u != "user" || p != "s3cret" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(accountsDoc))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func setupToken(srv *httptest.Server, path string) string {
	return base64.StdEncoding.EncodeToString([]byte(srv.URL + path))
}

func TestClaim_ReturnsAccessURL(t *testing.T) {
	srv, seen := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	got, err := c.ClaimSetupToken(context.Background(), setupToken(srv, "/simplefin/claim/abc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "http://user:s3cret@") || !strings.HasSuffix(got, "/simplefin") {
		t.Fatalf("access url = %q", got)
	}
	if (*seen)[0].ContentLength != 0 {
		t.Fatal("claim must POST an empty body")
	}
}

func TestClaim_ToleratesUnpaddedToken(t *testing.T) {
	srv, _ := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	tok := strings.TrimRight(setupToken(srv, "/simplefin/claim/abc"), "=")
	if _, err := c.ClaimSetupToken(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
}

func TestClaim_Rejected(t *testing.T) {
	srv, _ := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	_, err := c.ClaimSetupToken(context.Background(), setupToken(srv, "/simplefin/claim/used"))
	if !errors.Is(err, imports.ErrSetupTokenRejected) {
		t.Fatalf("err = %v, want ErrSetupTokenRejected", err)
	}
}

func TestClaim_BadTokenIsRejected(t *testing.T) {
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	for _, tok := range []string{"not base64!!", base64.StdEncoding.EncodeToString([]byte("ftp://x/y")), base64.StdEncoding.EncodeToString([]byte("http://insecure.example/claim"))} {
		if tok == base64.StdEncoding.EncodeToString([]byte("http://insecure.example/claim")) {
			c = simplefin.New(simplefin.Options{}) // https enforced by default
		}
		if _, err := c.ClaimSetupToken(context.Background(), tok); !errors.Is(err, imports.ErrSetupTokenRejected) {
			t.Fatalf("%q: err = %v", tok, err)
		}
	}
}

func TestFetch_ParsesAccountsAndDropsPending(t *testing.T) {
	srv, seen := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	cred := imports.Credential{AccessURL: "http://user:s3cret@" + strings.TrimPrefix(srv.URL, "http://") + "/simplefin"}
	res, err := c.FetchTransactions(context.Background(), cred, imports.FetchRequest{
		StartDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	q := (*seen)[0].URL.Query()
	if q.Get("start-date") != "1753999200" || q.Get("end-date") != "1756598400" {
		t.Fatalf("date params = %v", q)
	}
	if (*seen)[0].URL.User != nil {
		t.Fatal("credentials must travel in the Authorization header, not the URL")
	}
	if len(res.Accounts) != 1 || res.Accounts[0].ID != "ACT-1" || res.Accounts[0].Name != "Checking" || res.Accounts[0].Currency != "USD" || res.Accounts[0].Balance != "1000.10" || res.Accounts[0].OrgName != "Big Bank" {
		t.Fatalf("accounts = %+v", res.Accounts)
	}
	if len(res.Transactions) != 2 {
		t.Fatalf("pending/unposted rows must be dropped: %+v", res.Transactions)
	}
	tx := res.Transactions[0]
	if tx.ExternalAccountID != "ACT-1" || tx.ID != "TRN-1" || tx.Amount != "-12.50" || tx.Posted != 1755900000 || tx.Payee != "Blue Bottle" || tx.Description != "COFFEE SHOP" {
		t.Fatalf("tx = %+v", tx)
	}
	if !strings.Contains(string(tx.Raw), `"id":"TRN-1"`) {
		t.Fatalf("raw must be the provider's JSON: %s", tx.Raw)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "Connection to Big Bank may need attention" {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func TestListAccounts_UsesBalancesOnly(t *testing.T) {
	srv, seen := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	cred := imports.Credential{AccessURL: "http://user:s3cret@" + strings.TrimPrefix(srv.URL, "http://") + "/simplefin"}
	accts, err := c.ListAccounts(context.Background(), cred)
	if err != nil || len(accts) != 1 {
		t.Fatalf("accounts = %+v err %v", accts, err)
	}
	if (*seen)[0].URL.Query().Get("balances-only") != "1" {
		t.Fatalf("query = %v", (*seen)[0].URL.RawQuery)
	}
}

func TestFetch_BadCredentialAndUnavailable(t *testing.T) {
	srv, _ := newBridge(t)
	c := simplefin.New(simplefin.Options{AllowHTTP: true})
	host := strings.TrimPrefix(srv.URL, "http://")
	_, err := c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://" + host + "/simplefin"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrCredentialInvalid) {
		t.Fatalf("no userinfo: %v", err)
	}
	_, err = c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://user:wrong@" + host + "/simplefin"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrProviderUnavailable) {
		t.Fatalf("403 from accounts: %v", err)
	}
	srv.Close()
	_, err = c.FetchTransactions(context.Background(), imports.Credential{AccessURL: "http://user:s3cret@" + host + "/simplefin"}, imports.FetchRequest{})
	if !errors.Is(err, imports.ErrProviderUnavailable) {
		t.Fatalf("connection refused: %v", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error text leaks the credential: %v", err)
	}
}
```

(`1753999200` = 2026-08-01T00:00:00Z, `1756598400` = 2026-08-31T00:00:00Z — verify with `date -u -d @1753999200`; if the constants are off, fix the test constants, not the implementation.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/imports/simplefin/ 2>&1 | head`
Expected: package does not exist / undefined.

- [ ] **Step 3: Model types + provider seam**

`internal/model/imports.go` — add (`encoding/json` import):

```go
type ExternalAccount struct {
	ID       string
	Name     string
	Currency string
	Balance  string
	OrgName  string
}

// ExternalTransaction is one provider row before parsing; Raw is stored
// verbatim as the event payload so a retry re-parses exactly what arrived.
type ExternalTransaction struct {
	ExternalAccountID string
	ID                string
	Amount            string
	Posted            int64
	Payee             string
	Description       string
	Raw               json.RawMessage
}
```

`internal/imports/provider.go`:

```go
package imports

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
)

// Credential is a pull provider's secret for one call. It arrives from the
// client per request (decrypted in the browser) and is never persisted or
// logged by the server.
type Credential struct {
	AccessURL string
}

type FetchRequest struct {
	StartDate, EndDate time.Time
}

// FetchResult carries both halves of one provider round trip: SimpleFIN
// returns accounts and their transactions in a single document, so one call
// serves account discovery and the sync.
type FetchResult struct {
	Accounts     []model.ExternalAccount
	Transactions []model.ExternalTransaction
	Warnings     []string
}

type Provider interface {
	ListAccounts(ctx context.Context, cred Credential) ([]model.ExternalAccount, error)
	FetchTransactions(ctx context.Context, cred Credential, req FetchRequest) (*FetchResult, error)
}

// SetupTokenClaimer is the optional one-time exchange a provider offers
// (SimpleFIN: setup token -> access URL). The result goes back to the caller
// and is forgotten.
type SetupTokenClaimer interface {
	ClaimSetupToken(ctx context.Context, setupToken string) (string, error)
}

var (
	ErrSetupTokenRejected  = errors.New("setup token rejected")
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrCredentialInvalid   = errors.New("credential invalid")
)

func (s *Service) RegisterProvider(name string, p Provider) {
	if s.providers == nil {
		s.providers = map[string]Provider{}
	}
	s.providers[name] = p
}

func (s *Service) provider(name string) (Provider, bool) {
	p, ok := s.providers[name]
	return p, ok
}

func (s *Service) claimer(name string) (SetupTokenClaimer, bool) {
	c, ok := s.providers[name].(SetupTokenClaimer)
	return c, ok
}
```

`service.go`: add `providers map[string]Provider` to `Service` (not a `NewService` parameter — registration happens after construction, so the existing test harnesses keep compiling).

- [ ] **Step 4: The client**

`internal/imports/simplefin/client.go`:

```go
// Package simplefin is the SimpleFIN bridge client (protocol v1): a setup
// token is exchanged once for an access URL, and every later call is a GET
// against that URL with its embedded HTTP Basic credentials.
package simplefin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
)

const maxBody = 16 << 20

type Options struct {
	AllowHTTP bool
	Client    *http.Client
}

type Client struct {
	http      *http.Client
	allowHTTP bool
}

var (
	_ imports.Provider          = (*Client)(nil)
	_ imports.SetupTokenClaimer = (*Client)(nil)
)

func New(opts Options) *Client {
	c := opts.Client
	if c == nil {
		c = &http.Client{
			Timeout: 60 * time.Second,
			// A redirect could send the Basic credentials to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &Client{http: c, allowHTTP: opts.AllowHTTP}
}

func (c *Client) ClaimSetupToken(ctx context.Context, setupToken string) (string, error) {
	claimURL, err := c.decodeSetupToken(setupToken)
	if err != nil {
		return "", imports.ErrSetupTokenRejected
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claimURL, http.NoBody)
	if err != nil {
		return "", imports.ErrSetupTokenRejected
	}
	req.Header.Set("Content-Length", "0")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", imports.ErrProviderUnavailable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", imports.ErrProviderUnavailable
	}
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return "", imports.ErrSetupTokenRejected
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return "", imports.ErrProviderUnavailable
	}
	accessURL := strings.TrimSpace(string(body))
	if _, _, err := c.splitAccessURL(accessURL); err != nil {
		return "", imports.ErrProviderUnavailable
	}
	return accessURL, nil
}

func (c *Client) ListAccounts(ctx context.Context, cred imports.Credential) ([]model.ExternalAccount, error) {
	res, err := c.fetch(ctx, cred, url.Values{"balances-only": {"1"}})
	if err != nil {
		return nil, err
	}
	return res.Accounts, nil
}

func (c *Client) FetchTransactions(ctx context.Context, cred imports.Credential, req imports.FetchRequest) (*imports.FetchResult, error) {
	q := url.Values{}
	if !req.StartDate.IsZero() {
		q.Set("start-date", strconv.FormatInt(req.StartDate.Unix(), 10))
	}
	if !req.EndDate.IsZero() {
		q.Set("end-date", strconv.FormatInt(req.EndDate.Unix(), 10))
	}
	return c.fetch(ctx, cred, q)
}

func (c *Client) decodeSetupToken(tok string) (string, error) {
	tok = strings.TrimSpace(tok)
	raw, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		// tokens are often pasted without padding
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(tok, "="))
		if err != nil {
			return "", err
		}
	}
	u, err := url.Parse(strings.TrimSpace(string(raw)))
	if err != nil || u.Host == "" || !c.schemeOK(u.Scheme) {
		return "", errors.New("claim url invalid")
	}
	return u.String(), nil
}

func (c *Client) schemeOK(scheme string) bool {
	return scheme == "https" || (c.allowHTTP && scheme == "http")
}

// splitAccessURL separates the Basic credentials from the base URL so they
// travel in the Authorization header rather than the request line.
func (c *Client) splitAccessURL(raw string) (base *url.URL, user *url.Userinfo, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !c.schemeOK(u.Scheme) || u.User == nil {
		return nil, nil, imports.ErrCredentialInvalid
	}
	if _, ok := u.User.Password(); !ok {
		return nil, nil, imports.ErrCredentialInvalid
	}
	user = u.User
	u.User = nil
	return u, user, nil
}

type document struct {
	Errors   []json.RawMessage `json:"errors"`
	Accounts []struct {
		Org struct {
			Name string `json:"name"`
		} `json:"org"`
		ID           string            `json:"id"`
		Name         string            `json:"name"`
		Currency     string            `json:"currency"`
		Balance      string            `json:"balance"`
		Transactions []json.RawMessage `json:"transactions"`
	} `json:"accounts"`
}

type transaction struct {
	ID          string `json:"id"`
	Posted      int64  `json:"posted"`
	Amount      string `json:"amount"`
	Description string `json:"description"`
	Payee       string `json:"payee"`
	Pending     bool   `json:"pending"`
}

func (c *Client) fetch(ctx context.Context, cred imports.Credential, q url.Values) (*imports.FetchResult, error) {
	base, user, err := c.splitAccessURL(cred.AccessURL)
	if err != nil {
		return nil, err
	}
	endpoint := *base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/accounts"
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, imports.ErrCredentialInvalid
	}
	pass, _ := user.Password()
	req.SetBasicAuth(user.Username(), pass)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// never wrap err: url.Error carries the full URL, credentials included
		return nil, imports.ErrProviderUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", imports.ErrProviderUnavailable, resp.StatusCode)
	}
	var doc document
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&doc); err != nil {
		return nil, imports.ErrProviderUnavailable
	}
	out := &imports.FetchResult{Accounts: []model.ExternalAccount{}, Transactions: []model.ExternalTransaction{}, Warnings: warnings(doc.Errors)}
	for _, a := range doc.Accounts {
		out.Accounts = append(out.Accounts, model.ExternalAccount{ID: a.ID, Name: a.Name, Currency: a.Currency, Balance: a.Balance, OrgName: a.Org.Name})
		for _, raw := range a.Transactions {
			var t transaction
			if err := json.Unmarshal(raw, &t); err != nil {
				continue // a malformed row is a bridge bug; the rest of the account still imports
			}
			if t.Pending || t.Posted == 0 || t.ID == "" {
				continue // pending rows change id and amount when they settle
			}
			out.Transactions = append(out.Transactions, model.ExternalTransaction{
				ExternalAccountID: a.ID, ID: t.ID, Amount: t.Amount, Posted: t.Posted, Payee: t.Payee, Description: t.Description, Raw: raw,
			})
		}
	}
	return out, nil
}

// warnings accepts the v1 list of strings and, defensively, objects with a
// "message" key.
func warnings(raw []json.RawMessage) []string {
	out := []string{}
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			out = append(out, s)
			continue
		}
		var o struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(r, &o) == nil && o.Message != "" {
			out = append(out, o.Message)
		}
	}
	return out
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/imports/... 2>&1 | tail -5 && go vet ./internal/imports/... && go test ./internal/test/archtest/`
Expected: PASS; archtest passes (simplefin imports only `imports` + `model`).

- [ ] **Step 6: Commit**

```bash
git add internal/imports/provider.go internal/imports/simplefin internal/imports/service.go internal/model/imports.go
git commit -m "feat(imports): pull-provider seam and SimpleFIN client"
```

---

### Task 5: SimpleFIN event parsing + the `TransactionWriter` port + amount correction in `place`

**Files:**
- Create: `internal/imports/simplefinevent.go`, `internal/imports/simplefinevent_test.go`
- Modify: `internal/imports/ports.go` (`TransactionCreator` → `TransactionWriter`; `RateScopeClaimSetupToken`, `RateScopeSync`)
- Modify: `internal/imports/ingest.go` (`parse(ctx, …)`, `place(…, correctAmount bool)`), `internal/imports/service.go`, `internal/imports/accountlink.go` (`convertQueued` passes `false`)
- Modify: `internal/imports/ingest_test.go`, `internal/imports/api/harness_test.go` (fakes gain `UpdateTransaction`)
- Modify: `internal/server/glue_imports.go` (adapter gains `UpdateTransaction` → `UpdateTransactionPreservingLabels`)

**Interfaces:**
- Consumes: `model.ExternalTransaction`, `model.IngestEvent`, `imports.Match`/`MatchResult{Decision, CorrectAmount, …}` (stage 1 matcher; check `internal/imports/matcher.go` for the exact field names — the tip-adopt decision constant and the "adopt but fix the amount" flag).
- Produces:
  ```go
  // internal/imports/ports.go
  type TransactionWriter interface {
      CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
      UpdateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error)
  }
  const RateScopeClaimSetupToken = "import-claim"
  const RateScopeSync = "import-sync"

  // internal/imports/simplefinevent.go
  type simpleFINEnvelope struct {   // the stored import_events.payload for pull rows
      ExternalAccountID string          `json:"externalAccountId"`
      Transaction       json.RawMessage `json:"transaction"`
  }
  func EncodeSimpleFINEvent(tx model.ExternalTransaction) []byte
  func ParseSimpleFINEvent(payload []byte, loc *time.Location) (model.IngestEvent, error)

  // internal/imports/ingest.go
  func (s *Service) parse(ctx context.Context, src *model.ImportSource, ev *model.ImportEvent) (model.IngestEvent, error)
  func (s *Service) place(ctx context.Context, src *model.ImportSource, ev model.IngestEvent, r resolution, correctAmount bool) (txID vo.Id, adopted bool, amountUpdated bool, err error)
  ```

Field mapping (spec Part 5): `amount` is signed decimal text — negative → expense, positive → income, stored as the absolute value; `posted` (unix seconds) becomes the wall-clock time in `loc`; payee = `payee` when non-blank else `description`; description kept separately; currency comes from the account, so the parser leaves `Currency` empty and `applyEvent` fills it from the account link's `ExternalCurrency` — see Step 5.

- [ ] **Step 1: Write the failing parser tests**

`internal/imports/simplefinevent_test.go`:

```go
package imports_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
)

func TestParseSimpleFINEvent(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	raw := json.RawMessage(`{"id":"TRN-1","posted":1755900000,"amount":"-12.50","description":"COFFEE SHOP","payee":"Blue Bottle"}`)
	payload := imports.EncodeSimpleFINEvent(model.ExternalTransaction{ExternalAccountID: "ACT-1", ID: "TRN-1", Amount: "-12.50", Posted: 1755900000, Payee: "Blue Bottle", Description: "COFFEE SHOP", Raw: raw})
	ev, err := imports.ParseSimpleFINEvent(payload, berlin)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalAccountID != "ACT-1" || ev.ExternalTransactionID != "TRN-1" || ev.Type != model.TransactionTypeExpense || ev.Amount != "12.5" || ev.Payee != "Blue Bottle" || ev.Description != "COFFEE SHOP" {
		t.Fatalf("ev = %+v", ev)
	}
	if got := ev.PostedAt.Format("2006-01-02 15:04:05"); got != "2025-08-23 00:40:00" { // 1755900000 = 2025-08-22T22:40:00Z
		t.Fatalf("posted = %s", got)
	}
}

func TestParseSimpleFINEvent_IncomeAndPayeeFallback(t *testing.T) {
	payload := imports.EncodeSimpleFINEvent(model.ExternalTransaction{ExternalAccountID: "ACT-1", ID: "TRN-2", Amount: "2500.00", Posted: 1755910000, Description: "PAYROLL",
		Raw: json.RawMessage(`{"id":"TRN-2","posted":1755910000,"amount":"2500.00","description":"PAYROLL","payee":""}`)})
	ev, err := imports.ParseSimpleFINEvent(payload, time.UTC)
	if err != nil || ev.Type != model.TransactionTypeIncome || ev.Amount != "2500" || ev.Payee != "PAYROLL" {
		t.Fatalf("ev = %+v err %v", ev, err)
	}
}

func TestParseSimpleFINEvent_Rejects(t *testing.T) {
	for name, raw := range map[string]string{
		"zero amount":   `{"id":"T","posted":1755910000,"amount":"0"}`,
		"bad amount":    `{"id":"T","posted":1755910000,"amount":"12,50 EUR"}`,
		"no id":         `{"posted":1755910000,"amount":"1"}`,
		"no posted":     `{"id":"T","amount":"1"}`,
		"not json":      `nope`,
	} {
		payload := []byte(`{"externalAccountId":"ACT-1","transaction":` + func() string {
			if raw == "nope" {
				return `"nope"`
			}
			return raw
		}() + `}`)
		if _, err := imports.ParseSimpleFINEvent(payload, time.UTC); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := imports.ParseSimpleFINEvent([]byte(`{"transaction":{"id":"T","posted":1,"amount":"1"}}`), time.UTC); err == nil {
		t.Error("missing externalAccountId must fail")
	}
}
```

Verify the Berlin expectation with `TZ=Europe/Berlin date -d @1755900000`; fix the test constant if the arithmetic is off.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/imports/ -run ParseSimpleFIN 2>&1 | head`
Expected: undefined `EncodeSimpleFINEvent`/`ParseSimpleFINEvent`.

- [ ] **Step 3: Implement the parser**

`internal/imports/simplefinevent.go`:

```go
package imports

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// simpleFINEnvelope is the stored payload of a pull event: the provider's
// row verbatim plus the account it belongs to, which the row itself lacks.
type simpleFINEnvelope struct {
	ExternalAccountID string          `json:"externalAccountId"`
	Transaction       json.RawMessage `json:"transaction"`
}

type simpleFINTransaction struct {
	ID          string `json:"id"`
	Posted      int64  `json:"posted"`
	Amount      string `json:"amount"`
	Description string `json:"description"`
	Payee       string `json:"payee"`
}

var signedDecimalRe = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

func EncodeSimpleFINEvent(tx model.ExternalTransaction) []byte {
	b, _ := json.Marshal(simpleFINEnvelope{ExternalAccountID: tx.ExternalAccountID, Transaction: tx.Raw})
	return b
}

func ParseSimpleFINEvent(payload []byte, loc *time.Location) (model.IngestEvent, error) {
	var env simpleFINEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return model.IngestEvent{}, errors.New("invalid JSON")
	}
	account := normalizeExternalAccountID(env.ExternalAccountID)
	if account == "" {
		return model.IngestEvent{}, errors.New("externalAccountId is required")
	}
	var tx simpleFINTransaction
	if err := json.Unmarshal(env.Transaction, &tx); err != nil {
		return model.IngestEvent{}, errors.New("invalid transaction")
	}
	id := strings.TrimSpace(tx.ID)
	if id == "" {
		return model.IngestEvent{}, errors.New("id is required")
	}
	if tx.Posted <= 0 {
		return model.IngestEvent{}, errors.New("posted is required")
	}
	amount := strings.TrimSpace(tx.Amount)
	if !signedDecimalRe.MatchString(amount) {
		return model.IngestEvent{}, errors.New("amount must be a signed decimal")
	}
	d := vo.NewDecimal(amount)
	if d.IsZero() {
		return model.IngestEvent{}, errors.New("amount must not be zero")
	}
	typ := model.TransactionTypeIncome
	if d.IsNegative() {
		typ = model.TransactionTypeExpense
	}
	payee := strings.TrimSpace(tx.Payee)
	description := strings.TrimSpace(tx.Description)
	if payee == "" {
		payee = description
	}
	if runes := []rune(payee); len(runes) > 255 {
		payee = string(runes[:255])
	}
	return model.IngestEvent{
		ExternalAccountID: account, ExternalTransactionID: id, Type: typ, Amount: d.Abs().String(),
		PostedAt: time.Unix(tx.Posted, 0).In(loc), Payee: payee, Description: description,
	}, nil
}
```

(`vo.DecimalNumber.String()` renders `12.5` for `-12.50` after `Abs()` — confirm against `internal/shared/vo/decimal.go`; if it preserves trailing zeros, change the test expectation to whatever `String()` produces for the stored form — the stage-2 `normalizeAmount` uses the same `String()`, so both paths agree.)

- [ ] **Step 4: Rename the port and add the rate scopes**

`ports.go`:

```go
// TransactionWriter is the transaction feature's create/update use cases, so
// an import goes through exactly the checks a hand-entered transaction does
// (deleted account, write access, idempotency on the request id). Update is
// used only to adopt a corrected amount on a tip-matched transaction.
type TransactionWriter interface {
	CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
	UpdateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error)
}

const (
	RateScopeIngest          = "ingest"
	RateScopeClaimSetupToken = "import-claim"
	RateScopeSync            = "import-sync"
)
```

Rename the `Service.txns` field type and the `NewService` parameter type to `TransactionWriter`. Update the fakes: in `internal/imports/ingest_test.go` and `internal/imports/api/harness_test.go` the fake transaction service gains

```go
func (f *fakeTxns) UpdateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	f.updated = append(f.updated, req)
	return &model.UpdateTransactionResult{}, nil
}
```

(use the fake's real type name; add an `updated []model.UpdateTransactionRequest` field). `internal/server/glue_imports.go`: the adapter over `transactionSvc` gains

```go
func (a importsTransactionAdapter) UpdateTransaction(ctx context.Context, userID vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	return a.svc.UpdateTransactionPreservingLabels(ctx, userID, req)
}
```

(match the adapter's real receiver/type names in that file). Check `model.UpdateTransactionRequest`'s field set in `internal/model/transaction_dto.go` — the fields used below (`Id`, `Type`, `Amount`, `AccountId`, `Date`, `Description`, plus whatever is mandatory) must be filled from the candidate transaction.

- [ ] **Step 5: `parse` takes ctx; `place` corrects amounts**

`ingest.go`:

```go
func (s *Service) parse(ctx context.Context, src *model.ImportSource, ev *model.ImportEvent) (model.IngestEvent, error) {
	switch src.Provider {
	case model.ImportProviderAppleWallet:
		return ParseAppleWalletEvent([]byte(ev.Payload), ev.ReceivedAt)
	case model.ImportProviderSimpleFIN:
		return ParseSimpleFINEvent([]byte(ev.Payload), reqctx.Location(ctx))
	default:
		return model.IngestEvent{}, errors.New("unsupported provider " + src.Provider)
	}
}
```

Update `processEvent` to pass `ctx`. In `applyEvent`, right after the link lookup and before `resolve`: when `ev.Currency == ""` and the account link carries an `ExternalCurrency`, set `ev.Currency = *link.ExternalCurrency` (SimpleFIN rows carry no currency — the account does; `Sync` also pre-fills it from the fetched account, see Task 6, so this is the retry path's fallback). If neither is known, `resolve` treats it like a missing rate → queued with `ImportQueueReasonNoRate`.

`place` gains `correctAmount bool` and a third return `amountUpdated bool`. Keep a `byID map[vo.Id]*model.Transaction` of the listed candidates; when the match result adopts with a corrected amount (`res.Decision` is the tip-adopt decision AND `res.CorrectAmount` — use the stage-1 names from `matcher.go`) and `correctAmount` is true, build the update from the candidate:

```go
if m := Match(matchEv, src.ID, candidates, s.cfg); m.Kind != MatchCreate {
	if correctAmount && m.Kind == MatchTipAdopt && m.CorrectAmount {
		cand := byID[m.TransactionID] // *model.Transaction from the lister
		req := model.UpdateTransactionRequest{
			Id: cand.ID.String(), Type: cand.Type.Alias(), AccountId: cand.AccountID.String(),
			Amount: vo.NewFlexString(r.amount), Date: cand.SpentAt.Format(datetime.Layout),
			Description: optionalString(cand.Description),
			CategoryId: idString2(cand.CategoryID), PayeeId: idString2(cand.PayeeID), TagId: idString2(cand.TagID),
			AccountRecipientId: idString2(cand.AccountRecipID),
		}
		if cand.AmountRecipient != nil {
			ar := vo.NewFlexString(*cand.AmountRecipient)
			req.AmountRecipient = &ar
		}
		if _, err := s.txns.UpdateTransaction(ctx, src.UserID, req); err != nil {
			return vo.Id{}, false, false, err
		}
		return m.TransactionID, true, true, nil
	}
	return m.TransactionID, true, false, nil
}
```

where `idString2` is a new helper in `service.go`, `func idString2(p *vo.Id) *string` (nil → nil, else the string) — the existing `idString` returns `""` for nil, which `UpdateTransaction` would try to parse. The update goes through `UpdateTransactionPreservingLabels`, so labels are untouched; every other field is copied from the candidate so the only change is the amount. Delete the stale comment above `Match` ("m.CorrectAmount … is intentionally not applied here") — it is now wrong.

Both existing callers (`applyEvent` for push and `convertQueued`'s replay) pass `false` — push events deliberately never rewrite a hand-entered amount. `applyEvent` gets a `correctAmount bool` parameter threaded through from its callers (`IngestAppleWallet`/`RetryEvent`/`convertQueued` → `false`; Task 6's sync → `true`) and returns the `amountUpdated` flag alongside the status so the run can count it: change its signature to `applyEvent(ctx, src, eventID vo.Id, ev model.IngestEvent, runID *vo.Id, correctAmount bool) (status string, amountUpdated bool, err error)`, and set `RunID: runID` on every link it inserts (nil for push).

- [ ] **Step 6: Test the correction path**

Append to `internal/imports/ingest_test.go` a test using the file's harness (`setup(t)`, its fake lister/creator): seed a candidate transaction whose amount differs from the event within the tip tolerance so `Match` returns tip-adopt, call the internal `applyEvent` via an exported test hook — the package already has `export_test.go`-style access if tests are `package imports`; if they are `package imports_test`, add `internal/imports/export_test.go` with `func (s *Service) ApplyEventForTest(ctx, src, eventID, ev, runID, correctAmount) (string, bool, error) { return s.applyEvent(...) }`. Assert: with `correctAmount=false` the fake's `updated` stays empty; with `true` it holds one request whose `Amount` equals the event amount and `Id` equals the candidate id, and `amountUpdated` is `true`. Model the seeding on the existing `TestIngest_AdoptsHandEnteredTransaction` test in that file (it seeds the hand-entered candidate the same way).

- [ ] **Step 7: Run everything touched**

Run: `go build ./... && go test ./internal/imports/... ./internal/server/... 2>&1 | tail -8`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/imports internal/server/glue_imports.go
git commit -m "feat(imports): SimpleFIN event parser, TransactionWriter port, tip amount correction"
```

---

### Task 6: Service use cases — claim, credential key, external accounts, sync, run history

**Files:**
- Create: `internal/imports/credential.go`, `internal/imports/sync.go`, `internal/imports/run.go`
- Create: `internal/imports/sync_test.go`, `internal/imports/credential_test.go`, `internal/imports/run_test.go`
- Modify: `internal/imports/source.go` (`CreateSource` stores/overwrites the ciphertext), `internal/imports/service.go` (`sourceResult` new fields; `ownedRun`), `internal/imports/accountlink.go` (`ExternalName` on link/ignore)
- Modify: `internal/imports/ingest_test.go` (harness gains `providers` registration helper)

**Interfaces:**
- Consumes: Task 2 repo methods (`UpdateSource`, `UpsertCredentialKey`, `GetCredentialKey`, `DeleteCredentialKey`, `ListRunsByUser`, `ListLinksByRun`), Task 3 DTOs, Task 4 `Provider`/`SetupTokenClaimer`/`RegisterProvider`/sentinels, Task 5 `EncodeSimpleFINEvent`, `applyEvent(ctx, src, eventID, ev, runID, correctAmount) (status, amountUpdated, err)`, `RateScopeClaimSetupToken`, `RateScopeSync`.
- Produces:
  ```go
  func (s *Service) ClaimSetupToken(ctx, userID vo.Id, req model.ClaimSetupTokenRequest) (*model.ClaimSetupTokenResult, error)
  func (s *Service) GetCredentialKey(ctx, userID vo.Id) (*model.GetImportCredentialKeyResult, error)   // NotFound "Credential key not found" when none
  func (s *Service) SetCredentialKey(ctx, userID vo.Id, req model.SetImportCredentialKeyRequest) (*model.GetImportCredentialKeyResult, error)
  func (s *Service) ListExternalAccounts(ctx, userID vo.Id, req model.ListExternalAccountsRequest) (*model.ListExternalAccountsResult, error)
  func (s *Service) Sync(ctx, userID vo.Id, req model.SyncImportSourceRequest) (*model.SyncImportSourceResult, error)
  func (s *Service) GetRunList(ctx, userID vo.Id, sourceID string) (*model.GetImportRunListResult, error)  // sourceID "" = all
  func (s *Service) GetRun(ctx, userID vo.Id, rawID string) (*model.GetImportRunResult, error)
  const RunListLimit = 50
  ```

Rules this task enforces (spec Part 5 + the "no trust in the server at rest" model):
- The access URL is a per-request input. It is never written to any column, never passed to `reqctx.AddLogAttr`, never formatted into an error. The one thing the server stores is the client's opaque `credentialCiphertext` — and only via `create-source`.
- `Sync` runs the fetch OUTSIDE any DB transaction (a bridge call may take seconds; SQLite holds one writer). The run row is inserted first (status `running`), then each external account is applied in its own `WithTx`, so a failure in one account leaves the others imported and the run `partial`.
- Only the OWNER of a source can sync/list/claim; sync on a push provider is `CodeImportProviderUnsupported`.
- Counts: `created` → `ImportedCount`, adopted → `MatchedCount` (+ `AmountsUpdatedCount` when the tip amount was corrected), queued → `QueuedCount`, skipped → `SkippedCount`, parse failure → `FailedCount`, `duplicate` → not counted.
- Status: `failed` when the fetch itself failed or returned no accounts AND there are errors; `partial` when the run finished with at least one error; else `completed`. `last_synced_at` is written on `completed`/`partial` only.

- [ ] **Step 1: Write the failing tests**

`internal/imports/sync_test.go` (`package imports_test`, reusing the `setup(t)` harness, `userA`, `acct1`, `source` constants from `ingest_test.go`; the harness needs one addition, Step 2):

```go
package imports_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

const bankSource = "0c000000-0000-0000-0000-00000000000b"

type fakeProvider struct {
	accounts []model.ExternalAccount
	txs      []model.ExternalTransaction
	warnings []string
	err      error
	seen     []imports.Credential
	claimed  string
}

func (p *fakeProvider) ListAccounts(_ context.Context, cred imports.Credential) ([]model.ExternalAccount, error) {
	p.seen = append(p.seen, cred)
	return p.accounts, p.err
}

func (p *fakeProvider) FetchTransactions(_ context.Context, cred imports.Credential, _ imports.FetchRequest) (*imports.FetchResult, error) {
	p.seen = append(p.seen, cred)
	if p.err != nil {
		return nil, p.err
	}
	return &imports.FetchResult{Accounts: p.accounts, Transactions: p.txs, Warnings: p.warnings}, nil
}

func (p *fakeProvider) ClaimSetupToken(_ context.Context, tok string) (string, error) {
	p.claimed = tok
	if tok == "used" {
		return "", imports.ErrSetupTokenRejected
	}
	return "https://u:p@bridge.example/simplefin", nil
}

func extTx(account, id, amount string, posted int64, payee string) model.ExternalTransaction {
	raw, _ := json.Marshal(map[string]any{"id": id, "posted": posted, "amount": amount, "payee": payee, "description": payee})
	return model.ExternalTransaction{ExternalAccountID: account, ID: id, Amount: amount, Posted: posted, Payee: payee, Description: payee, Raw: raw}
}

func bankHarness(t *testing.T) (*harness, *fakeProvider) {
	t.Helper()
	h := setup(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	p := &fakeProvider{accounts: []model.ExternalAccount{{ID: "ACT-1", Name: "Checking", Currency: "USD", Balance: "10", OrgName: "Big Bank"}}}
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	return h, p
}

func syncReq(over ...func(*model.SyncImportSourceRequest)) model.SyncImportSourceRequest {
	r := model.SyncImportSourceRequest{SourceId: bankSource, AccessUrl: "https://u:p@bridge.example/simplefin", StartDate: "2026-08-01", EndDate: "2026-08-31"}
	for _, f := range over {
		f(&r)
	}
	return r
}

func TestSync_MappedAccountImportsAndCounts(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{
		extTx("ACT-1", "T1", "-12.50", 1755900000, "Coffee"),
		extTx("ACT-1", "T2", "2500", 1755910000, "Payroll"),
		extTx("ACT-1", "T3", "abc", 1755920000, "Broken"),
	}
	ctx := reqctx.WithLogAttrs(context.Background())
	res, err := h.svc.Sync(ctx, vo.MustParseId(userA), syncReq())
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Status != model.ImportRunStatusCompleted || res.Run.ImportedCount != 2 || res.Run.FailedCount != 1 || res.Run.Trigger != model.ImportRunTriggerManual {
		t.Fatalf("run = %+v", res.Run)
	}
	if len(res.Accounts) != 1 || res.Accounts[0].State != model.ImportCardStateMapped || res.Accounts[0].AccountId != acct1 || res.Accounts[0].OrgName != "Big Bank" {
		t.Fatalf("accounts = %+v", res.Accounts)
	}
	if len(h.txns.created) != 2 || h.txns.created[0].Type != "expense" || h.txns.created[0].Amount.String() != "12.5" || h.txns.created[1].Type != "income" {
		t.Fatalf("created = %+v", h.txns.created)
	}
	// second sync: the same rows are duplicates and nothing is counted
	res, err = h.svc.Sync(ctx, vo.MustParseId(userA), syncReq())
	// The broken row's event was stored too (status failed), so on the second
	// pass every row — including it — is a payload duplicate and counts nothing.
	if err != nil || res.Run.ImportedCount != 0 || res.Run.FailedCount != 0 || res.Run.Status != model.ImportRunStatusCompleted {
		t.Fatalf("second run = %+v err %v", res.Run, err)
	}
	src, _ := h.repo.GetSource(ctx, vo.MustParseId(bankSource))
	if src.LastSyncedAt == nil {
		t.Fatal("last_synced_at must be set after a completed run")
	}
	for _, a := range reqctx.LogAttrs(ctx) {
		if s, ok := a.Value.Any().(string); ok && (s == syncReq().AccessUrl || containsSecret(s)) {
			t.Fatalf("access url leaked into log attrs: %v", a)
		}
	}
}

func containsSecret(s string) bool { return len(s) > 0 && (s == "https://u:p@bridge.example/simplefin" || s == "u:p") }

func TestSync_UnmappedAccountQueues(t *testing.T) {
	h, p := bankHarness(t)
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-1", 1755900000, "X")}
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if err != nil || res.Run.QueuedCount != 1 || res.Run.ImportedCount != 0 || res.Accounts[0].State != model.ImportCardStateUnmapped {
		t.Fatalf("res = %+v err %v", res, err)
	}
}

func TestSync_ProviderWarningsMakeRunPartial(t *testing.T) {
	h, p := bankHarness(t)
	p.warnings = []string{"Connection to Big Bank may need attention"}
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if err != nil || res.Run.Status != model.ImportRunStatusPartial || len(res.Run.Errors) != 1 || res.Run.Errors[0].ExternalAccountId != "" || res.Run.Errors[0].Message != p.warnings[0] {
		t.Fatalf("res = %+v err %v", res.Run, err)
	}
}

func TestSync_ProviderDownIsCoded(t *testing.T) {
	h, p := bankHarness(t)
	p.err = imports.ErrProviderUnavailable
	_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	verr, ok := errs.AsValidation(err)
	if !ok || verr.MsgCode != errs.CodeImportProviderUnavailable {
		t.Fatalf("err = %v", err)
	}
	runs, _ := h.repo.ListRunsByUser(context.Background(), vo.MustParseId(userA), nil, 10)
	if len(runs) != 1 || runs[0].Status != model.ImportRunStatusFailed || runs[0].FinishedAt == nil {
		t.Fatalf("a failed fetch must leave a failed run: %+v", runs)
	}
}

func TestSync_BadCredentialIsCoded(t *testing.T) {
	h, p := bankHarness(t)
	p.err = imports.ErrCredentialInvalid
	_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportAccessUrlInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestSync_RangeRules(t *testing.T) {
	h, _ := bankHarness(t)
	for _, r := range []model.SyncImportSourceRequest{
		syncReq(func(r *model.SyncImportSourceRequest) { r.StartDate, r.EndDate = "2026-08-31", "2026-08-01" }),
		syncReq(func(r *model.SyncImportSourceRequest) { r.StartDate, r.EndDate = "2025-01-01", "2026-08-01" }),
	} {
		_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), r)
		if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportSyncRangeInvalid {
			t.Fatalf("%+v: err = %v", r, err)
		}
	}
	// end date defaults to today (clock), start may equal end
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq(func(r *model.SyncImportSourceRequest) { r.EndDate = "" }))
	if err != nil || res.Run.Status != model.ImportRunStatusCompleted {
		t.Fatalf("res = %+v err %v", res, err)
	}
}

func TestSync_PushProviderAndForeignSourceRejected(t *testing.T) {
	h, _ := bankHarness(t)
	_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq(func(r *model.SyncImportSourceRequest) { r.SourceId = source }))
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportProviderUnsupported {
		t.Fatalf("push provider: %v", err)
	}
	_, err = h.svc.Sync(context.Background(), vo.MustParseId(userB), syncReq())
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("foreign source: %v", err)
	}
}

func TestSync_PerAccountFailureIsIsolated(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.accounts = append(p.accounts, model.ExternalAccount{ID: "ACT-2", Name: "Savings", Currency: "USD"})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-2", AccountID: acct1})
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-1", 1755900000, "ok"), extTx("ACT-2", "T2", "-2", 1755900000, "boom")}
	h.txns.failOn = "boom" // the fake creator errors on this payee (add the field: see Step 2)
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if err != nil || res.Run.Status != model.ImportRunStatusPartial || res.Run.ImportedCount != 1 || len(res.Run.Errors) != 1 || res.Run.Errors[0].ExternalAccountId != "ACT-2" {
		t.Fatalf("res = %+v err %v", res.Run, err)
	}
	if res.Run.Errors[0].Message != "Import failed for this account" {
		t.Fatalf("message must be static, got %q", res.Run.Errors[0].Message)
	}
}

func TestSync_RateLimited(t *testing.T) {
	h, _ := bankHarness(t)
	h.withLimiter()
	h.lim.deny = errors.New("limited")
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err == nil {
		t.Fatal("expected the limiter's error")
	}
}

func TestSync_TipAdoptCorrectsAmount(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	// hand-entered 10.00 on 2025-08-22, bank posts 11.50 (a 15% tip) two days later
	h.txns.seed(t, acct1, "expense", "10", time.Date(2025, 8, 22, 12, 0, 0, 0, time.UTC), "Coffee")
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-11.50", time.Date(2025, 8, 24, 12, 0, 0, 0, time.UTC).Unix(), "Coffee")}
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq(func(r *model.SyncImportSourceRequest) { r.StartDate, r.EndDate = "2025-08-01", "2025-08-31" }))
	if err != nil || res.Run.MatchedCount != 1 || res.Run.AmountsUpdatedCount != 1 || res.Run.ImportedCount != 0 {
		t.Fatalf("res = %+v err %v", res.Run, err)
	}
	if len(h.txns.updated) != 1 || h.txns.updated[0].Amount.String() != "11.5" {
		t.Fatalf("updated = %+v", h.txns.updated)
	}
}
```

`internal/imports/credential_test.go`:

```go
package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestClaimSetupToken(t *testing.T) {
	h, p := bankHarness(t)
	res, err := h.svc.ClaimSetupToken(context.Background(), vo.MustParseId(userA), model.ClaimSetupTokenRequest{SetupToken: "abc"})
	if err != nil || res.AccessUrl != "https://u:p@bridge.example/simplefin" || p.claimed != "abc" {
		t.Fatalf("res = %+v err %v", res, err)
	}
	_, err = h.svc.ClaimSetupToken(context.Background(), vo.MustParseId(userA), model.ClaimSetupTokenRequest{SetupToken: "used"})
	var denied *errs.AccessDeniedError
	if !errorsAs(err, &denied) || denied.Code != errs.CodeImportSetupTokenRejected {
		t.Fatalf("err = %v", err)
	}
}

func TestClaimSetupToken_NoClaimer(t *testing.T) {
	h := setup(t) // no provider registered
	_, err := h.svc.ClaimSetupToken(context.Background(), vo.MustParseId(userA), model.ClaimSetupTokenRequest{SetupToken: "abc"})
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportProviderUnsupported {
		t.Fatalf("err = %v", err)
	}
}

func TestCredentialKey_RoundTrip(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	if _, err := h.svc.GetCredentialKey(ctx, vo.MustParseId(userA)); err == nil {
		t.Fatal("expected not found before set")
	}
	res, err := h.svc.SetCredentialKey(ctx, vo.MustParseId(userA), model.SetImportCredentialKeyRequest{WrappedDataKey: "v1:iv:ct", Kdf: `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`})
	if err != nil || res.WrappedDataKey != "v1:iv:ct" || res.UpdatedAt == "" {
		t.Fatalf("set = %+v err %v", res, err)
	}
	res, err = h.svc.SetCredentialKey(ctx, vo.MustParseId(userA), model.SetImportCredentialKeyRequest{WrappedDataKey: "v1:iv2:ct2", Kdf: res.Kdf})
	if err != nil || res.WrappedDataKey != "v1:iv2:ct2" {
		t.Fatalf("overwrite = %+v err %v", res, err)
	}
	got, err := h.svc.GetCredentialKey(ctx, vo.MustParseId(userA))
	if err != nil || got.WrappedDataKey != "v1:iv2:ct2" {
		t.Fatalf("get = %+v err %v", got, err)
	}
	if _, err := h.svc.GetCredentialKey(ctx, vo.MustParseId(userB)); err == nil {
		t.Fatal("keys are per user")
	}
}

func TestListExternalAccounts_MergesMappingState(t *testing.T) {
	h, p := bankHarness(t)
	p.accounts = append(p.accounts, model.ExternalAccount{ID: "ACT-2", Name: "Savings", Currency: "USD", Balance: "5"})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-2", AccountID: acct1})
	res, err := h.svc.ListExternalAccounts(context.Background(), vo.MustParseId(userA), model.ListExternalAccountsRequest{SourceId: bankSource, AccessUrl: "https://u:p@bridge.example/simplefin"})
	if err != nil || len(res.Items) != 2 {
		t.Fatalf("res = %+v err %v", res, err)
	}
	if res.Items[0].ExternalAccountId != "ACT-1" || res.Items[0].State != model.ImportCardStateUnmapped || res.Items[0].Balance != "10" {
		t.Fatalf("item0 = %+v", res.Items[0])
	}
	if res.Items[1].State != model.ImportCardStateMapped || res.Items[1].AccountId != acct1 {
		t.Fatalf("item1 = %+v", res.Items[1])
	}
	if len(p.seen) != 1 || p.seen[0].AccessURL != "https://u:p@bridge.example/simplefin" {
		t.Fatalf("provider saw %+v", p.seen)
	}
	_, err = h.svc.ListExternalAccounts(context.Background(), vo.MustParseId(userA), model.ListExternalAccountsRequest{SourceId: source, AccessUrl: "x"})
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportProviderUnsupported {
		t.Fatalf("push provider: %v", err)
	}
}

func TestCreateSource_SimpleFINStoresAndReconnectOverwritesCiphertext(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	res, err := h.svc.CreateSource(ctx, vo.MustParseId(userA), model.CreateImportSourceRequest{Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:a:b"})
	if err != nil || res.Item.CredentialCiphertext != "v1:a:b" || res.Item.LastSyncedAt != "" {
		t.Fatalf("res = %+v err %v", res, err)
	}
	again, err := h.svc.CreateSource(ctx, vo.MustParseId(userA), model.CreateImportSourceRequest{Provider: model.ImportProviderSimpleFIN, Name: "Bank 2", CredentialCiphertext: "v1:c:d"})
	if err != nil || again.Item.Id != res.Item.Id || again.Item.CredentialCiphertext != "v1:c:d" || again.Item.Name != "Bank 2" {
		t.Fatalf("reconnect = %+v err %v", again, err)
	}
	// Apple Wallet keeps its ciphertext empty and its result carries ""
	wallet, err := h.svc.CreateSource(ctx, vo.MustParseId(userA), model.CreateImportSourceRequest{Provider: model.ImportProviderAppleWallet, Name: "iPhone"})
	if err != nil || wallet.Item.CredentialCiphertext != "" {
		t.Fatalf("wallet = %+v err %v", wallet, err)
	}
}

func errorsAs(err error, target any) bool { return err != nil && errors.As(err, target) }
```

(Add `"errors"` to that file's imports.)

`internal/imports/run_test.go`:

```go
package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestGetRunListAndGetRun(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-1", 1755900000, "Coffee")}
	ctx := context.Background()
	res, err := h.svc.Sync(ctx, vo.MustParseId(userA), syncReq())
	if err != nil {
		t.Fatal(err)
	}
	list, err := h.svc.GetRunList(ctx, vo.MustParseId(userA), "")
	if err != nil || len(list.Items) != 1 || list.Items[0].Id != res.Run.Id || list.Items[0].SourceId != bankSource {
		t.Fatalf("list = %+v err %v", list, err)
	}
	filtered, err := h.svc.GetRunList(ctx, vo.MustParseId(userA), source)
	if err != nil || len(filtered.Items) != 0 {
		t.Fatalf("filtered = %+v err %v", filtered, err)
	}
	if _, err := h.svc.GetRunList(ctx, vo.MustParseId(userA), "not-a-uuid"); err == nil {
		t.Fatal("unknown source id must be not found")
	}
	one, err := h.svc.GetRun(ctx, vo.MustParseId(userA), res.Run.Id)
	if err != nil || one.Item.Id != res.Run.Id || len(one.Links) != 1 || one.Links[0].ExternalPayee != "Coffee" || one.Links[0].Status != model.ImportLinkStatusLinked || one.Links[0].TransactionId == "" {
		t.Fatalf("run = %+v err %v", one, err)
	}
	if _, err := h.svc.GetRun(ctx, vo.MustParseId(userB), res.Run.Id); err == nil {
		t.Fatal("foreign run must be not found")
	}
	if _, err := h.svc.GetRun(ctx, vo.MustParseId(userA), "nope"); err == nil {
		t.Fatal("bad id must be not found")
	}
	_ = errs.AsNotFound
}
```

- [ ] **Step 2: Extend the test harness**

`internal/imports/ingest_test.go`: add to `fakeTxns` the fields `updated []model.UpdateTransactionRequest` (Task 5), `failOn string` (when non-empty, `CreateTransaction` returns `errors.New("create failed")` for a request whose `Description` equals it), and a `seed(t, accountID, typeAlias, amount string, at time.Time, description string)` helper that inserts a transaction through the fixture builder (`h.f.Transaction(...)` — check `internal/test/fixture/entities.go` for the `Transaction` builder's field names; the existing `TestIngest_AdoptsHandEnteredTransaction` seeds one the same way, copy its call). Add `deny error` to the fake `limiter`: `Allow` returns `l.deny` when set. `harness` keeps `repo` exported enough for `sync_test.go` (it is the same package, so field access is fine). Add `userB` and `source` constants if `sync_test.go` cannot see them (they exist in `ingest_test.go` already: `userB`, `source`).

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/imports/ -run 'Sync|Claim|CredentialKey|ListExternalAccounts|CreateSource_SimpleFIN|GetRunList' 2>&1 | head`
Expected: compile errors (undefined `Sync`, `ClaimSetupToken`, …).

- [ ] **Step 4: `credential.go`**

```go
package imports

import (
	"context"
	"errors"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ClaimSetupToken exchanges a one-time SimpleFIN setup token for the access
// URL and hands it straight back: the browser encrypts it, the server never
// keeps it.
func (s *Service) ClaimSetupToken(ctx context.Context, userID vo.Id, req model.ClaimSetupTokenRequest) (*model.ClaimSetupTokenResult, error) {
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeClaimSetupToken, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopeClaimSetupToken, userID.String())
	}
	c, ok := s.claimer(model.ImportProviderSimpleFIN)
	if !ok {
		return nil, &errs.ValidationError{Msg: "Provider is not supported", MsgCode: errs.CodeImportProviderUnsupported}
	}
	accessURL, err := c.ClaimSetupToken(ctx, strings.TrimSpace(req.SetupToken))
	if err != nil {
		return nil, mapProviderErr(err)
	}
	return &model.ClaimSetupTokenResult{AccessUrl: accessURL}, nil
}

// mapProviderErr turns the provider sentinels into coded edge errors. Any
// other error is passed through unchanged (500 at the edge) — it never
// carries the credential because the client never formats it (Task 4).
func mapProviderErr(err error) error {
	switch {
	case errors.Is(err, ErrSetupTokenRejected):
		return &errs.AccessDeniedError{Msg: "Setup token rejected", Code: errs.CodeImportSetupTokenRejected}
	case errors.Is(err, ErrCredentialInvalid):
		return &errs.ValidationError{Msg: "Access URL is invalid", MsgCode: errs.CodeImportAccessUrlInvalid}
	case errors.Is(err, ErrProviderUnavailable):
		return &errs.ValidationError{Msg: "Provider unavailable", MsgCode: errs.CodeImportProviderUnavailable}
	}
	return err
}

func (s *Service) GetCredentialKey(ctx context.Context, userID vo.Id) (*model.GetImportCredentialKeyResult, error) {
	k, err := s.repo.GetCredentialKey(ctx, userID)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, errs.NewNotFound("Credential key not found")
	}
	return credentialKeyResult(k), nil
}

func (s *Service) SetCredentialKey(ctx context.Context, userID vo.Id, req model.SetImportCredentialKeyRequest) (*model.GetImportCredentialKeyResult, error) {
	now := s.clk.Now().UTC()
	k := &model.ImportCredentialKey{UserID: userID, WrappedDataKey: req.WrappedDataKey, KDF: req.Kdf, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.UpsertCredentialKey(ctx, k); err != nil {
		return nil, err
	}
	return credentialKeyResult(k), nil
}

func credentialKeyResult(k *model.ImportCredentialKey) *model.GetImportCredentialKeyResult {
	return &model.GetImportCredentialKeyResult{WrappedDataKey: k.WrappedDataKey, Kdf: k.KDF, UpdatedAt: k.UpdatedAt.Format(datetime.Layout)}
}

// pullSource is ownedSource plus the pull-provider checks every credential
// call shares.
func (s *Service) pullSource(ctx context.Context, userID vo.Id, rawID string) (*model.ImportSource, Provider, error) {
	src, err := s.ownedSource(ctx, userID, rawID)
	if err != nil {
		return nil, nil, err
	}
	p, ok := s.provider(src.Provider)
	if model.ImportProviderIsPush(src.Provider) || !ok {
		return nil, nil, &errs.ValidationError{Msg: "Provider is not supported", MsgCode: errs.CodeImportProviderUnsupported}
	}
	return src, p, nil
}

func (s *Service) ListExternalAccounts(ctx context.Context, userID vo.Id, req model.ListExternalAccountsRequest) (*model.ListExternalAccountsResult, error) {
	src, p, err := s.pullSource(ctx, userID, req.SourceId)
	if err != nil {
		return nil, err
	}
	accounts, err := p.ListAccounts(ctx, Credential{AccessURL: req.AccessUrl})
	if err != nil {
		return nil, mapProviderErr(err)
	}
	items, err := s.externalAccountResults(ctx, src, accounts)
	if err != nil {
		return nil, err
	}
	return &model.ListExternalAccountsResult{Items: items}, nil
}

// externalAccountResults joins what the bridge reports with the user's
// mapping state for each account.
func (s *Service) externalAccountResults(ctx context.Context, src *model.ImportSource, accounts []model.ExternalAccount) ([]model.ExternalAccountResult, error) {
	links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ExternalAccountResult, 0, len(accounts))
	for _, a := range accounts {
		item := model.ExternalAccountResult{
			ExternalAccountId: a.ID, ExternalName: a.Name, ExternalCurrency: a.Currency, Balance: a.Balance, OrgName: a.OrgName,
			State: model.ImportCardStateUnmapped,
		}
		if al := findAccountLink(links, a.ID); al != nil {
			item.State = model.ImportCardStateMapped
			if al.Mode == model.ImportAccountLinkModeIgnore {
				item.State = model.ImportCardStateIgnored
			}
			item.AccountId = idString(al.AccountID)
		}
		out = append(out, item)
	}
	return out, nil
}
```

- [ ] **Step 5: `sync.go`**

```go
package imports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const maxSyncRangeDays = 400

const accountFailedMessage = "Import failed for this account"

// Sync is the pull-side import: fetch the range from the provider, then run
// every account through the same pipeline a push event uses. The fetch runs
// outside any DB transaction (it may take seconds); each account is its own
// transaction so one bad account cannot roll back the others.
func (s *Service) Sync(ctx context.Context, userID vo.Id, req model.SyncImportSourceRequest) (*model.SyncImportSourceResult, error) {
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeSync, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopeSync, userID.String())
	}
	src, p, err := s.pullSource(ctx, userID, req.SourceId)
	if err != nil {
		return nil, err
	}
	start, end, err := s.syncRange(ctx, req)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "source_id", src.ID.String())
	params, _ := json.Marshal(map[string]string{"startDate": req.StartDate, "endDate": end.Format(model.ImportDateLayout)})
	run := &model.ImportRun{
		ID: vo.NewId(), UserID: userID, SourceID: src.ID, Provider: src.Provider, Params: string(params),
		Status: model.ImportRunStatusRunning, Trigger: model.ImportRunTriggerManual, StartedAt: s.clk.Now().UTC(), Errors: []model.ImportRunError{},
	}
	if err := s.repo.InsertRun(ctx, run); err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "run_id", run.ID.String())
	fetched, ferr := p.FetchTransactions(ctx, Credential{AccessURL: req.AccessUrl}, FetchRequest{StartDate: start, EndDate: end.AddDate(0, 0, 1)})
	if ferr != nil {
		run.Status = model.ImportRunStatusFailed
		if err := s.finishRun(ctx, run); err != nil {
			return nil, err
		}
		return nil, mapProviderErr(ferr)
	}
	for _, w := range fetched.Warnings {
		run.Errors = append(run.Errors, model.ImportRunError{Message: w})
	}
	byAccount := map[string][]model.ExternalTransaction{}
	for _, t := range fetched.Transactions {
		byAccount[t.ExternalAccountID] = append(byAccount[t.ExternalAccountID], t)
	}
	for _, a := range fetched.Accounts {
		if err := s.syncAccount(ctx, src, run, a, byAccount[a.ID]); err != nil {
			run.Errors = append(run.Errors, model.ImportRunError{ExternalAccountId: a.ID, Message: accountFailedMessage})
			reqctx.AddLogAttr(ctx, "account_error", a.ID)
		}
	}
	switch {
	case len(fetched.Accounts) == 0 && len(run.Errors) > 0:
		run.Status = model.ImportRunStatusFailed
	case len(run.Errors) > 0:
		run.Status = model.ImportRunStatusPartial
	default:
		run.Status = model.ImportRunStatusCompleted
	}
	if err := s.finishRun(ctx, run); err != nil {
		return nil, err
	}
	if run.Status != model.ImportRunStatusFailed {
		synced := s.clk.Now().UTC()
		src.LastSyncedAt, src.UpdatedAt = &synced, synced
		if err := s.repo.UpdateSource(ctx, src); err != nil {
			return nil, err
		}
	}
	accounts, err := s.externalAccountResults(ctx, src, fetched.Accounts)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "run_status", run.Status)
	return &model.SyncImportSourceResult{Run: runResult(run), Accounts: accounts}, nil
}

// syncAccount stores and applies one account's rows in a single transaction.
// A duplicate payload (same event already stored) is skipped silently, so a
// re-run of an overlapping range is a no-op for rows already seen.
func (s *Service) syncAccount(ctx context.Context, src *model.ImportSource, run *model.ImportRun, account model.ExternalAccount, rows []model.ExternalTransaction) error {
	snapshot := *run
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, row := range rows {
			payload := EncodeSimpleFINEvent(row)
			ev := &model.ImportEvent{
				ID: vo.NewId(), SourceID: src.ID, RunID: &run.ID, Payload: string(payload), PayloadHash: HashPayload(payload),
				Status: model.ImportEventStatusProcessed, ReceivedAt: s.clk.Now().UTC(),
			}
			inserted, err := s.repo.InsertEvent(ctx, ev)
			if err != nil {
				return err
			}
			if !inserted {
				continue
			}
			parsed, perr := s.parse(ctx, src, ev)
			if perr != nil {
				msg := perr.Error()
				if err := s.repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusFailed, &msg); err != nil {
					return err
				}
				run.FailedCount++
				continue
			}
			if parsed.Currency == "" {
				parsed.Currency = account.Currency
			}
			status, amountUpdated, err := s.applyEvent(ctx, src, ev.ID, parsed, &run.ID, true)
			if err != nil {
				return err
			}
			switch status {
			case model.ImportIngestStatusCreated:
				run.ImportedCount++
			case model.ImportIngestStatusMatched:
				run.MatchedCount++
				if amountUpdated {
					run.AmountsUpdatedCount++
				}
			case model.ImportIngestStatusQueued:
				run.QueuedCount++
			case model.ImportIngestStatusSkipped:
				run.SkippedCount++
			}
		}
		return nil
	})
	if err != nil {
		*run = snapshot // the transaction rolled back; so do its counters
	}
	return err
}

func (s *Service) finishRun(ctx context.Context, run *model.ImportRun) error {
	finished := s.clk.Now().UTC()
	run.FinishedAt = &finished
	return s.repo.UpdateRun(ctx, run)
}

// syncRange applies the range rules: end defaults to the caller's today,
// start must not follow end, and the span is capped so one click cannot
// pull a decade of history.
func (s *Service) syncRange(ctx context.Context, req model.SyncImportSourceRequest) (start, end time.Time, err error) {
	loc := reqctx.Location(ctx)
	start, _ = time.ParseInLocation(model.ImportDateLayout, req.StartDate, loc) // validated by the DTO
	if req.EndDate == "" {
		now := s.clk.Now().In(loc)
		end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	} else {
		end, _ = time.ParseInLocation(model.ImportDateLayout, req.EndDate, loc)
	}
	if start.After(end) || end.Sub(start) > maxSyncRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, &errs.ValidationError{Msg: "Invalid date range", MsgCode: errs.CodeImportSyncRangeInvalid}
	}
	return start, end, nil
}
```

`applyEvent` must return `model.ImportIngestStatusMatched` for an adopted transaction — check stage 2: it currently returns `ImportIngestStatusCreated` for both (`txID, _, err := s.place(...)`). Change it to return `ImportIngestStatusMatched` when `adopted` (the constant exists in `internal/model/imports.go`; grep `ImportIngestStatusMatched` — if absent, add `ImportIngestStatusMatched = "matched"` next to the other `ImportIngestStatus*` constants). The push endpoint's `IngestEventResult.Status` then reads `matched` for an adopted tap — this IS an observable change on `ingest-apple-wallet-event`; the apiparity golden for the adopt scenario (if one exists — grep `testdata/golden` for `"created"` under `ingest`) will change; regenerate and inspect in Task 9.

- [ ] **Step 6: `run.go`**

```go
package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const RunListLimit = 50

func (s *Service) GetRunList(ctx context.Context, userID vo.Id, sourceID string) (*model.GetImportRunListResult, error) {
	var filter *vo.Id
	if sourceID != "" {
		src, err := s.ownedSource(ctx, userID, sourceID)
		if err != nil {
			return nil, err
		}
		filter = &src.ID
	}
	runs, err := s.repo.ListRunsByUser(ctx, userID, filter, RunListLimit)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportRunListResult{Items: make([]model.ImportRunResult, 0, len(runs))}
	for i := range runs {
		out.Items = append(out.Items, runResult(&runs[i]))
	}
	return out, nil
}

func (s *Service) GetRun(ctx context.Context, userID vo.Id, rawID string) (*model.GetImportRunResult, error) {
	id, err := vo.ParseId(rawID)
	if err != nil {
		return nil, errs.NewNotFound("Import run not found")
	}
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if run.UserID != userID {
		return nil, errs.NewNotFound("Import run not found")
	}
	links, err := s.repo.ListLinksByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportRunResult{Item: runResult(run), Links: make([]model.ImportRunLinkResult, 0, len(links))}
	for _, l := range links {
		out.Links = append(out.Links, model.ImportRunLinkResult{
			Id: l.ID.String(), ExternalAccountId: l.ExternalAccountID, ExternalTransactionId: l.ExternalTransactionID,
			TransactionId: idString(l.TransactionID), Status: l.Status, ExternalPayee: l.ExternalPayee,
			ExternalAmount: l.ExternalAmount, ExternalCurrency: derefString(l.ExternalCurrency),
			ExternalPostedAt: l.ExternalPostedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}

func runResult(r *model.ImportRun) model.ImportRunResult {
	out := model.ImportRunResult{
		Id: r.ID.String(), SourceId: r.SourceID.String(), Provider: r.Provider, Status: r.Status, Trigger: r.Trigger,
		ImportedCount: r.ImportedCount, MatchedCount: r.MatchedCount, AmountsUpdatedCount: r.AmountsUpdatedCount,
		QueuedCount: r.QueuedCount, SkippedCount: r.SkippedCount, FailedCount: r.FailedCount,
		Errors: r.Errors, StartedAt: r.StartedAt.Format(datetime.Layout),
	}
	if out.Errors == nil {
		out.Errors = []model.ImportRunError{}
	}
	if r.FinishedAt != nil {
		out.FinishedAt = r.FinishedAt.Format(datetime.Layout)
	}
	return out
}
```

Replace the hand-built `ImportRunResult` at the end of `convertQueued` (`accountlink.go`) with `r := runResult(run); return &r, nil` so both paths render runs identically (this adds `sourceId`/`provider`/`trigger`/… to the `link-account` response — a golden change to inspect in Task 9).

- [ ] **Step 7: `source.go` — store and overwrite the ciphertext; `sourceResult` — new fields; `accountlink.go` — `ExternalName`**

In `CreateSource`, after the get-or-create branch and before `sourceResult`:

```go
		if req.CredentialCiphertext != "" && !model.ImportProviderIsPush(req.Provider) {
			ct := req.CredentialCiphertext
			src.CredentialCiphertext, src.Name, src.UpdatedAt = &ct, req.Name, s.clk.Now().UTC()
			if err := s.repo.UpdateSource(ctx, src); err != nil {
				return err
			}
		}
```

(On the create branch this runs an extra UPDATE — acceptable; or set the field on the struct before `InsertSource` and skip the update when the source was just created. Either is fine; keep it simple.) The name follows the ciphertext on reconnect so a renamed bridge shows the new name.

In `sourceResult`, add to the returned struct: `LastSyncedAt: optionalTime(src.LastSyncedAt), CredentialCiphertext: derefString(src.CredentialCiphertext)` with a new helper in `service.go`:

```go
func optionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(datetime.Layout)
}
```

In `LinkAccount` (and `IgnoreAccount`): when `req.ExternalName` is non-blank, use it (trimmed) as the account link's `ExternalName` instead of the external id — SimpleFIN ids are opaque (`ACT-…`) and the bridge's display name is what the user recognizes. Where the existing code sets `ExternalName: req.ExternalAccountId` (or `ext`), replace with `externalName(req.ExternalName, ext)`:

```go
func externalName(explicit, fallback string) string {
	if n := strings.TrimSpace(explicit); n != "" {
		return n
	}
	return fallback
}
```

- [ ] **Step 8: Run the tests**

Run: `go test ./internal/imports/... 2>&1 | tail -15`
Expected: PASS. If `TestSync_TipAdoptCorrectsAmount` fails on the seeded transaction date vs. `MatchDays`/`TipDays`, adjust the dates so the bank row lands within `TipDays` (default 5) AFTER the hand-entered one and the amount within `TipTolerancePct` (default 20); do not loosen the config.

- [ ] **Step 9: Commit**

```bash
git add internal/imports
git commit -m "feat(imports): claim, credential key, external accounts, sync and run history use cases"
```

---

### Task 7: Rate-limit configuration for claim and sync

**Files:**
- Modify: `internal/config/config.go` (struct fields + the int-limit table around line 265), `internal/config/config_test.go`
- Modify: `internal/server/server.go` (the `ratelimit.New` limits map, ~line 171)
- Modify: `internal/test/apiparity/harness.go` (cfg literal, ~line 96)
- Modify: `.env.example`, `CLAUDE.md` (the `ECONUMO_RATE_LIMIT_*` bullet)

**Interfaces:**
- Consumes: `RateScopeClaimSetupToken`, `RateScopeSync` (Task 5).
- Produces: `config.Config.RateLimitClaimSetupToken int` (`ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN`, default `5`), `config.Config.RateLimitSync int` (`ECONUMO_RATE_LIMIT_SYNC`, default `10`).

- [ ] **Step 1: Write the failing test**

In `internal/config/config_test.go`, extend the defaults test (the one asserting `c.RateLimitIngest != 60` at ~line 134) and the overrides test (~line 147):

```go
	if c.RateLimitClaimSetupToken != 5 || c.RateLimitSync != 10 {
		t.Fatalf("claim/sync = %d/%d, want 5/10", c.RateLimitClaimSetupToken, c.RateLimitSync)
	}
```

and in the overrides test add `t.Setenv("ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN", "0")`, `t.Setenv("ECONUMO_RATE_LIMIT_SYNC", "2")` with the assertion `c.RateLimitClaimSetupToken != 0 || c.RateLimitSync != 2`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ 2>&1 | head`
Expected: compile error, undefined field.

- [ ] **Step 3: Implement**

`internal/config/config.go` — next to `RateLimitIngest`:

```go
	RateLimitClaimSetupToken    int           // ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN: SimpleFIN setup-token claims per user (every request counts)
	RateLimitSync               int           // ECONUMO_RATE_LIMIT_SYNC: pull syncs per user (every request counts)
```

and in the int-limit table after the ingest row:

```go
		{&c.RateLimitClaimSetupToken, "ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN", 5},
		{&c.RateLimitSync, "ECONUMO_RATE_LIMIT_SYNC", 10},
```

`internal/server/server.go` limits map, after `appimports.RateScopeIngest`:

```go
			appimports.RateScopeClaimSetupToken: cfg.RateLimitClaimSetupToken,
			appimports.RateScopeSync:            cfg.RateLimitSync,
```

`internal/test/apiparity/harness.go` cfg literal, after `RateLimitIngest: 60,`:

```go
		RateLimitClaimSetupToken: 5,
		RateLimitSync:            10,
```

`.env.example`, after the `ECONUMO_RATE_LIMIT_INGEST` line:

```
#ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN=5  # SimpleFIN setup-token claims per user per rate-limit window (0 = off)
#ECONUMO_RATE_LIMIT_SYNC=10              # bank syncs per user per rate-limit window (0 = off)
```

`CLAUDE.md`, in the `ECONUMO_RATE_LIMIT_*` bullet after the `ECONUMO_RATE_LIMIT_INGEST` sentence:

```
  `ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN` — `import/claim-setup-token` calls per user per window (default `5`; every request counts).
  `ECONUMO_RATE_LIMIT_SYNC` — `import/sync-source` calls per user per window (default `10`; every request counts — a sync is a real bridge round trip).
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/config/ ./internal/server/ 2>&1 | tail -5`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config internal/server/server.go internal/test/apiparity/harness.go .env.example CLAUDE.md
git commit -m "feat(config): rate limits for SimpleFIN claim and sync"
```

---

### Task 8: HTTP handlers, routes, OpenAPI, composition-root wiring

**Files:**
- Create: `internal/imports/api/credential.go`, `internal/imports/api/sync.go`, `internal/imports/api/run.go`
- Create: `internal/imports/api/credential_test.go`, `internal/imports/api/sync_test.go`
- Modify: `internal/imports/api/routes.go`, `internal/imports/api/harness_test.go`
- Modify: `internal/server/server.go` (`Seams.ImportProviders`; provider registration)
- Modify: `internal/server/glue_imports.go` (`UpdateTransaction` adapter already added in Task 5; nothing else)
- Regenerate: `internal/web/apidoc/*` via `make swagger`

**Interfaces:**
- Consumes: Task 6 service methods; Task 3 DTOs (each with `Validate()`).
- Produces routes (all auth, all under `/api/v1/import/`):
  - `POST claim-setup-token` → `model.ClaimSetupTokenRequest` → `model.ClaimSetupTokenResult`
  - `GET get-credential-key` → `model.GetImportCredentialKeyResult`
  - `POST set-credential-key` → `model.SetImportCredentialKeyRequest` → `model.GetImportCredentialKeyResult`
  - `POST list-external-accounts` → `model.ListExternalAccountsRequest` → `model.ListExternalAccountsResult`
  - `POST sync-source` → `model.SyncImportSourceRequest` → `model.SyncImportSourceResult`
  - `GET get-run-list?sourceId=` → `model.GetImportRunListResult` (hand-written query-param handler)
  - `GET get-run?id=` → `model.GetImportRunResult` (hand-written)
  - `server.Seams.ImportProviders map[string]appimports.Provider` — nil map = register the real SimpleFIN client under `model.ImportProviderSimpleFIN`; a non-nil map registers exactly its entries (tests inject a stub).

`list-external-accounts` and `sync-source` are POSTs even though the first is a read: the access URL travels in the body, never in a query string (query strings land in access logs and browser history).

- [ ] **Step 1: Write the failing tests**

`internal/imports/api/sync_test.go` (`package api_test`, using `newHarness(t)`; the harness gains a `provider` field, Step 2):

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	appimports "github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

const bankSource = "0c000000-0000-0000-0000-00000000000b"

type stubProvider struct {
	accounts []model.ExternalAccount
	txs      []model.ExternalTransaction
	err      error
}

func (p *stubProvider) ListAccounts(context.Context, appimports.Credential) ([]model.ExternalAccount, error) {
	return p.accounts, p.err
}
func (p *stubProvider) FetchTransactions(context.Context, appimports.Credential, appimports.FetchRequest) (*appimports.FetchResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &appimports.FetchResult{Accounts: p.accounts, Transactions: p.txs}, nil
}
func (p *stubProvider) ClaimSetupToken(_ context.Context, tok string) (string, error) {
	if tok == "used" {
		return "", appimports.ErrSetupTokenRejected
	}
	return "https://u:p@bridge.example/simplefin", nil
}

func call(t *testing.T, h *harness, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, &buf)
	req.Header.Set("Authorization", "Bearer "+userA)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("body %s: %v", raw, err)
	}
	return res.StatusCode, env
}

func TestSyncSource_EndToEnd(t *testing.T) {
	h := newHarness(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	h.provider.accounts = []model.ExternalAccount{{ID: "ACT-1", Name: "Checking", Currency: "USD", Balance: "1"}}
	raw, _ := json.Marshal(map[string]any{"id": "T1", "posted": 1755900000, "amount": "-4.20", "payee": "Cafe"})
	h.provider.txs = []model.ExternalTransaction{{ExternalAccountID: "ACT-1", ID: "T1", Amount: "-4.20", Posted: 1755900000, Payee: "Cafe", Raw: raw}}

	status, env := call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@bridge.example/simplefin", "startDate": "2025-08-01", "endDate": "2025-08-31"})
	if status != 200 {
		t.Fatalf("status %d: %v", status, env)
	}
	data := env["data"].(map[string]any)
	run := data["run"].(map[string]any)
	if run["status"] != "completed" || run["importedCount"] != float64(1) || run["trigger"] != "manual" {
		t.Fatalf("run = %v", run)
	}
	if h.txns.created != 1 {
		t.Fatalf("created %d", h.txns.created)
	}
	accounts := data["accounts"].([]any)
	if len(accounts) != 1 || accounts[0].(map[string]any)["state"] != "mapped" {
		t.Fatalf("accounts = %v", accounts)
	}

	status, env = call(t, h, "GET", "/api/v1/import/get-run-list?sourceId="+bankSource, nil)
	if status != 200 || len(env["data"].(map[string]any)["items"].([]any)) != 1 {
		t.Fatalf("run list %d: %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-run?id="+run["id"].(string), nil)
	if status != 200 || len(env["data"].(map[string]any)["links"].([]any)) != 1 {
		t.Fatalf("get-run %d: %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-run?id=nope", nil)
	if status != 400 {
		t.Fatalf("bad id: %d %v", status, env)
	}
}

func TestSyncSource_ValidationAndProviderErrors(t *testing.T) {
	h := newHarness(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	status, env := call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "", "startDate": "2025-08-01"})
	if status != 400 || env["errors"].(map[string]any)["accessUrl"] == nil {
		t.Fatalf("blank accessUrl: %d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@b/x", "startDate": "08/01/2025"})
	if status != 400 || env["errors"].(map[string]any)["startDate"] == nil {
		t.Fatalf("bad startDate: %d %v", status, env)
	}
	h.provider.err = appimports.ErrProviderUnavailable
	status, env = call(t, h, "POST", "/api/v1/import/sync-source", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@b/x", "startDate": "2025-08-01", "endDate": "2025-08-31"})
	if status != 400 || env["message"] != "Provider unavailable" {
		t.Fatalf("unavailable: %d %v", status, env)
	}
	// the run row is on record even though the fetch failed
	_, env = call(t, h, "GET", "/api/v1/import/get-run-list", nil)
	items := env["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["status"] != "failed" {
		t.Fatalf("items = %v", items)
	}
}
```

`internal/imports/api/credential_test.go`:

```go
package api_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestClaimSetupToken_Handler(t *testing.T) {
	h := newHarness(t)
	status, env := call(t, h, "POST", "/api/v1/import/claim-setup-token", map[string]any{"setupToken": "abc"})
	if status != 200 || env["data"].(map[string]any)["accessUrl"] != "https://u:p@bridge.example/simplefin" {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/claim-setup-token", map[string]any{"setupToken": "used"})
	if status != 403 || env["message"] != "Setup token rejected" {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/claim-setup-token", map[string]any{"setupToken": ""})
	if status != 400 || env["errors"].(map[string]any)["setupToken"] == nil {
		t.Fatalf("%d %v", status, env)
	}
}

func TestCredentialKey_Handlers(t *testing.T) {
	h := newHarness(t)
	status, _ := call(t, h, "GET", "/api/v1/import/get-credential-key", nil)
	if status != 400 {
		t.Fatalf("missing key: %d", status)
	}
	status, env := call(t, h, "POST", "/api/v1/import/set-credential-key", map[string]any{"wrappedDataKey": "v1:iv:ct", "kdf": `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`})
	if status != 200 || env["data"].(map[string]any)["wrappedDataKey"] != "v1:iv:ct" {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/set-credential-key", map[string]any{"wrappedDataKey": "", "kdf": "{}"})
	if status != 400 || env["errors"].(map[string]any)["wrappedDataKey"] == nil {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "GET", "/api/v1/import/get-credential-key", nil)
	if status != 200 || env["data"].(map[string]any)["wrappedDataKey"] != "v1:iv:ct" {
		t.Fatalf("%d %v", status, env)
	}
}

func TestListExternalAccounts_Handler(t *testing.T) {
	h := newHarness(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	h.provider.accounts = []model.ExternalAccount{{ID: "ACT-1", Name: "Checking", Currency: "USD", Balance: "1", OrgName: "Big Bank"}}
	status, env := call(t, h, "POST", "/api/v1/import/list-external-accounts", map[string]any{"sourceId": bankSource, "accessUrl": "https://u:p@b/x"})
	if status != 200 {
		t.Fatalf("%d %v", status, env)
	}
	items := env["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["orgName"] != "Big Bank" || items[0].(map[string]any)["state"] != "unmapped" {
		t.Fatalf("items = %v", items)
	}
	status, _ = call(t, h, "POST", "/api/v1/import/list-external-accounts", map[string]any{"sourceId": source, "accessUrl": "https://u:p@b/x"})
	if status != 400 {
		t.Fatalf("push provider: %d", status)
	}
}

func TestCreateSource_SimpleFINHandler(t *testing.T) {
	h := newHarness(t)
	status, env := call(t, h, "POST", "/api/v1/import/create-source", map[string]any{"provider": "simplefin", "name": "Bank", "credentialCiphertext": "v1:a:b"})
	if status != 200 || env["data"].(map[string]any)["item"].(map[string]any)["credentialCiphertext"] != "v1:a:b" {
		t.Fatalf("%d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/create-source", map[string]any{"provider": "simplefin", "name": "Bank"})
	if status != 400 || env["errors"].(map[string]any)["credentialCiphertext"] == nil {
		t.Fatalf("simplefin without ciphertext must fail: %d %v", status, env)
	}
}
```

- [ ] **Step 2: Extend the api harness**

In `internal/imports/api/harness_test.go`: add `provider *stubProvider` to `harness`; in `newHarness` after `svc := ...`:

```go
	provider := &stubProvider{}
	svc.RegisterProvider(model.ImportProviderSimpleFIN, provider)
```

and return it in the struct literal. If `call` collides with an existing helper name in the package (grep `func call(` / `func post(` in `ingest_test.go`), reuse the existing one and delete the duplicate above.

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/imports/api/ 2>&1 | head`
Expected: 404s / compile errors (handlers undefined).

- [ ] **Step 4: Handlers**

`internal/imports/api/credential.go`:

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// ClaimSetupToken handles POST /api/v1/import/claim-setup-token (auth).
//
// @Summary     Claim a SimpleFIN setup token
// @Description Exchanges a one-time SimpleFIN setup token for the bridge access URL. The URL is returned to the client and never stored server-side; the client encrypts it with its credential key and sends the ciphertext to create-source. 403 when the bridge rejects the token (already claimed or expired).
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ClaimSetupTokenRequest true "Setup token"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ClaimSetupTokenResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/claim-setup-token [post]
func (h *Handlers) ClaimSetupToken(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ClaimSetupToken)
}

// GetCredentialKey handles GET /api/v1/import/get-credential-key (auth).
//
// @Summary     Get the wrapped import credential key
// @Description Returns the caller's passphrase-wrapped data key and KDF parameters. The server cannot unwrap it. 400 (coded not-found) when no key has been set.
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportCredentialKeyResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-credential-key [get]
func (h *Handlers) GetCredentialKey(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetCredentialKey)
}

// SetCredentialKey handles POST /api/v1/import/set-credential-key (auth).
//
// @Summary     Set the wrapped import credential key
// @Description Stores (or replaces) the caller's passphrase-wrapped data key and KDF parameters. Replacing the key does not touch stored credential ciphertexts; the client re-encrypts and re-submits them via create-source.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.SetImportCredentialKeyRequest true "Wrapped key"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportCredentialKeyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/set-credential-key [post]
func (h *Handlers) SetCredentialKey(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SetCredentialKey)
}

// ListExternalAccounts handles POST /api/v1/import/list-external-accounts (auth).
//
// @Summary     List the accounts a pull source exposes
// @Description Asks the provider (with the client-decrypted access URL in the body) for its accounts and joins each with the caller's mapping state. POST because the access URL must not travel in a query string.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ListExternalAccountsRequest true "Source + access URL"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ListExternalAccountsResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/list-external-accounts [post]
func (h *Handlers) ListExternalAccounts(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ListExternalAccounts)
}
```

`internal/imports/api/sync.go`:

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/endpoint"
)

// SyncSource handles POST /api/v1/import/sync-source (auth).
//
// @Summary     Pull transactions from a bank source
// @Description Fetches the date range from the provider using the client-decrypted access URL and runs every returned row through the import pipeline (create / match / queue / skip). Returns the run summary and the provider's accounts with their mapping state. endDate defaults to today; the span is capped at 400 days.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.SyncImportSourceRequest true "Sync request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SyncImportSourceResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/sync-source [post]
func (h *Handlers) SyncSource(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.Sync)
}
```

`internal/imports/api/run.go` (query-param reads are hand-written, same shape as `GetTransactionImportList`):

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetRunList handles GET /api/v1/import/get-run-list (auth).
// Optional query param: sourceId.
//
// @Summary     List import runs
// @Description The caller's most recent 50 import runs, newest first, optionally filtered to one source.
// @Tags        Import
// @Produce     json
// @Param       sourceId query    string false "Source id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportRunListResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-run-list [get]
func (h *Handlers) GetRunList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := h.svc.GetRunList(r.Context(), userID, r.URL.Query().Get("sourceId"))
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// GetRun handles GET /api/v1/import/get-run (auth).
// Required query param: id.
//
// @Summary     Get one import run
// @Description The run summary plus every ledger row it wrote. A row whose transaction was deleted since keeps its external fields with an empty transactionId.
// @Tags        Import
// @Produce     json
// @Param       id query    string true "Run id"
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportRunResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-run [get]
func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := h.svc.GetRun(r.Context(), userID, r.URL.Query().Get("id"))
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
```

`internal/imports/api/routes.go` — append after `get-transaction-import-list`:

```go
		mux.Handle("POST /api/v1/import/claim-setup-token", auth(h.ClaimSetupToken))
		mux.Handle("GET /api/v1/import/get-credential-key", auth(h.GetCredentialKey))
		mux.Handle("POST /api/v1/import/set-credential-key", auth(h.SetCredentialKey))
		// POST, not GET: the access URL rides in the body, never a query string.
		mux.Handle("POST /api/v1/import/list-external-accounts", auth(h.ListExternalAccounts))
		mux.Handle("POST /api/v1/import/sync-source", auth(h.SyncSource))
		mux.Handle("GET /api/v1/import/get-run-list", auth(h.GetRunList))
		mux.Handle("GET /api/v1/import/get-run", auth(h.GetRun))
```

- [ ] **Step 5: Composition root**

`internal/server/server.go`: add to `Seams`:

```go
	// ImportProviders overrides the pull-import providers keyed by
	// model.ImportProvider* name. nil registers the real SimpleFIN client;
	// tests inject a stub so no scenario reaches the network.
	ImportProviders map[string]appimports.Provider
```

and after `importsSvc := appimports.NewService(...)`:

```go
	if seams.ImportProviders == nil {
		importsSvc.RegisterProvider(model.ImportProviderSimpleFIN, simplefin.New(simplefin.Options{}))
	}
	for name, p := range seams.ImportProviders {
		importsSvc.RegisterProvider(name, p)
	}
```

with the import `"github.com/econumo/econumo/internal/imports/simplefin"`. Add an archtest expectation if `internal/test/archtest` lists allowed imports per package explicitly (grep `simplefin` there; if the test auto-detects `internal/imports/simplefin` as part of the `imports` feature via its top-level dir, nothing to add — verify by running it).

- [ ] **Step 6: OpenAPI docs + tests**

Run: `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin && make swagger && go test ./internal/imports/... ./internal/server/... ./internal/test/archtest/... 2>&1 | tail -8`
Expected: PASS; `git status` shows the regenerated `internal/web/apidoc/` files.

- [ ] **Step 7: Commit**

```bash
git add internal/imports/api internal/server internal/web/apidoc
git commit -m "feat(imports): SimpleFIN claim/credential/sync/run endpoints"
```

---

### Task 9: API parity scenarios and goldens

**Files:**
- Create: `internal/test/apiparity/stubprovider.go`
- Modify: `internal/test/apiparity/harness.go` (inject `Seams.ImportProviders`), `internal/test/apiparity/fixture.go` (a seeded SimpleFIN source), `internal/test/apiparity/catalogue_import.go`, `internal/test/apiparity/catalogue_ratelimit.go`
- Regenerate: `internal/test/apiparity/testdata/golden/**` (+ the `mcpparity` goldens if `tools/list` output changed — it should not; no MCP surface in this stage)

**Interfaces:**
- Consumes: Task 8 routes and `Seams.ImportProviders`; the fixture ids `ImportSourcePhone`, `OwnerAccount`, `OwnerID`, `Txn1`.
- Produces: fixture constant `ImportSourceBank = "0c000000-0000-0000-0000-000000000002"` (owner's SimpleFIN source, ciphertext `"v1:c2VlZA==:c2VlZA=="`), scenarios `import_simplefin_credentials`, `import_simplefin_sync`, and two calls appended to `auth_rate_limit`.

The guard test requires every registered route to appear in at least one scenario (`guard_test.go`), so all seven new routes need a call.

- [ ] **Step 1: Stub provider**

`internal/test/apiparity/stubprovider.go`:

```go
package apiparity

import (
	"context"
	"encoding/json"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
)

// stubProvider is the pull provider the parity harness wires in place of the
// real SimpleFIN client: deterministic, offline, and keyed on the access URL
// so one scenario can exercise both the healthy and the failing bridge.
//
//   - access URL "https://u:p@bridge.example/simplefin"  -> two accounts, three rows
//   - access URL "https://u:p@bridge.example/down"       -> ErrProviderUnavailable
//   - anything else                                       -> ErrCredentialInvalid
//   - setup token "used"                                  -> ErrSetupTokenRejected
type stubProvider struct{}

const (
	stubAccessURL = "https://u:p@bridge.example/simplefin"
	stubDownURL   = "https://u:p@bridge.example/down"
)

func stubAccounts() []model.ExternalAccount {
	return []model.ExternalAccount{
		{ID: "ACT-CHK", Name: "Checking", Currency: "USD", Balance: "1250.10", OrgName: "Example Bank"},
		{ID: "ACT-SAV", Name: "Savings", Currency: "USD", Balance: "9000", OrgName: "Example Bank"},
	}
}

func stubRow(account, id, amount string, posted int64, payee string) model.ExternalTransaction {
	raw, _ := json.Marshal(map[string]any{"id": id, "posted": posted, "amount": amount, "payee": payee, "description": payee, "pending": false})
	return model.ExternalTransaction{ExternalAccountID: account, ID: id, Amount: amount, Posted: posted, Payee: payee, Description: payee, Raw: raw}
}

func (stubProvider) ListAccounts(_ context.Context, cred imports.Credential) ([]model.ExternalAccount, error) {
	switch cred.AccessURL {
	case stubAccessURL:
		return stubAccounts(), nil
	case stubDownURL:
		return nil, imports.ErrProviderUnavailable
	}
	return nil, imports.ErrCredentialInvalid
}

func (stubProvider) FetchTransactions(_ context.Context, cred imports.Credential, _ imports.FetchRequest) (*imports.FetchResult, error) {
	switch cred.AccessURL {
	case stubAccessURL:
		// ClockTime is time.Now() (harness.go), so posted dates are relative to
		// it: inside the scenario's [ClockTime-30d, ClockTime] range.
		return &imports.FetchResult{
			Accounts: stubAccounts(),
			Transactions: []model.ExternalTransaction{
				stubRow("ACT-CHK", "chk-1", "-12.50", ClockTime.AddDate(0, 0, -3).Unix(), "Blue Bottle"),
				stubRow("ACT-CHK", "chk-2", "2500", ClockTime.AddDate(0, 0, -2).Unix(), "Payroll"),
				stubRow("ACT-SAV", "sav-1", "-100", ClockTime.AddDate(0, 0, -3).Unix(), "Transfer"),
			},
			Warnings: []string{"Connection to Example Bank may need attention"},
		}, nil
	case stubDownURL:
		return nil, imports.ErrProviderUnavailable
	}
	return nil, imports.ErrCredentialInvalid
}

func (stubProvider) ClaimSetupToken(_ context.Context, token string) (string, error) {
	if token == "used" {
		return "", imports.ErrSetupTokenRejected
	}
	return stubAccessURL, nil
}
```

`stubRow`'s fourth parameter is `int64` unix seconds; `ClockTime` is the package-level `time.Now().UTC().Truncate(time.Second)` in `harness.go`, so the rows always land a few days before "now" and the goldens' normalizer redacts the resulting datetimes (`externalPostedAt`, `startedAt`, …) like every other datetime.

- [ ] **Step 2: Harness + fixture**

`harness.go`: add `ImportProviders: map[string]appimports.Provider{model.ImportProviderSimpleFIN: stubProvider{}}` to the `server.Seams{...}` literal (import `appimports "github.com/econumo/econumo/internal/imports"` and `model`).

`fixture.go`: add the constant `ImportSourceBank = "0c000000-0000-0000-0000-000000000002"` beside `ImportSourcePhone` and, after the phone source seed:

```go
	f.ImportSource(fixture.ImportSource{ID: ImportSourceBank, UserID: OwnerID, Provider: model.ImportProviderSimpleFIN, Name: "Example Bank", CredentialCiphertext: "v1:c2VlZA==:c2VlZA=="})
	f.ImportAccountLink(fixture.ImportAccountLink{SourceID: ImportSourceBank, ExternalAccountID: "ACT-CHK", ExternalName: "Checking", ExternalCurrency: "USD", AccountID: OwnerAccount})
```

(Check the `fixture.ImportAccountLink` field names in `internal/test/fixture/entities.go`; the Wallet seed a few lines above uses the same builder.) Seeding a source changes the `get-source-list` goldens (a second item with `provider: simplefin`, `lastSyncedAt: ""`, `credentialCiphertext`) — expected.

- [ ] **Step 3: Scenarios**

In `catalogue_import.go`, change the existing call:

```go
			{Label: "err:create-source-unknown-provider", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "csv", "name": "Bank"}},
```

and append two scenarios:

```go
	register(Scenario{Name: "import_simplefin_credentials", Calls: func() []Call {
		return []Call{
			{Label: "err:get-credential-key-none", Method: "GET", Path: "/api/v1/import/get-credential-key", Auth: "owner"},
			{Label: "set-credential-key", Method: "POST", Path: "/api/v1/import/set-credential-key", Auth: "owner",
				Body: map[string]any{"wrappedDataKey": "v1:aXY=:Y3Q=", "kdf": `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`}},
			{Label: "get-credential-key", Method: "GET", Path: "/api/v1/import/get-credential-key", Auth: "owner"},
			{Label: "err:set-credential-key-blank", Method: "POST", Path: "/api/v1/import/set-credential-key", Auth: "owner",
				Body: map[string]any{"wrappedDataKey": "", "kdf": ""}},
			{Label: "claim-setup-token", Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
				Body: map[string]any{"setupToken": "aHR0cHM6Ly9icmlkZ2UuZXhhbXBsZS9zaW1wbGVmaW4vY2xhaW0vYWJj"}},
			{Label: "err:claim-setup-token-used", Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
				Body: map[string]any{"setupToken": "used"}},
			{Label: "err:claim-setup-token-blank", Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
				Body: map[string]any{"setupToken": ""}},
			// Guest connects a bank: the ciphertext is opaque to the server.
			{Label: "create-source-simplefin", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "My Bank", "credentialCiphertext": "v1:aXY=:Y3Q="}},
			{Label: "err:create-source-simplefin-no-ciphertext", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "My Bank"}},
			// Reconnect: same (user, provider) -> same source, new ciphertext + name.
			{Label: "create-source-simplefin-reconnect", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "My Bank (new)", "credentialCiphertext": "v1:aXYy:Y3Qy"}},
			{Label: "get-source-list-guest-bank", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "guest"},
		}
	}})

	register(Scenario{Name: "import_simplefin_sync", Calls: func() []Call {
		// ClockTime is "now"; the window is the last 30 days, which is where the
		// stub provider dates its rows. Request bodies are not part of the golden.
		day := func(offset int) string { return ClockTime.AddDate(0, 0, offset).Format("2006-01-02") }
		start, end := day(-30), day(0)
		return []Call{
			{Label: "list-external-accounts", Method: "POST", Path: "/api/v1/import/list-external-accounts", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL}},
			{Label: "err:list-external-accounts-bad-url", Method: "POST", Path: "/api/v1/import/list-external-accounts", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": "https://nope.example/"}},
			{Label: "err:list-external-accounts-push-source", Method: "POST", Path: "/api/v1/import/list-external-accounts", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "accessUrl": stubAccessURL}},
			// ACT-CHK is mapped -> 2 created; ACT-SAV unmapped -> 1 queued; one bridge warning -> partial.
			{Label: "sync-source", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start, "endDate": end}},
			// Same range again: every row is a duplicate, nothing is counted.
			{Label: "sync-source-again", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start, "endDate": end}},
			{Label: "err:sync-source-range", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": day(1), "endDate": start}},
			{Label: "err:sync-source-down", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubDownURL, "startDate": start, "endDate": end}},
			{Label: "err:sync-source-foreign", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "guest",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start, "endDate": end}},
			{Label: "err:readonly-sync-source", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "readonly",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start}},
			{Label: "get-run-list", Method: "GET", Path: "/api/v1/import/get-run-list", Auth: "owner"},
			{Label: "get-run-list-by-source", Method: "GET", Path: "/api/v1/import/get-run-list?sourceId=" + ImportSourceBank, Auth: "owner"},
			{Label: "get-run-list-guest-empty", Method: "GET", Path: "/api/v1/import/get-run-list", Auth: "guest"},
			// The sync's run id is server-minted and Call.CaptureIDInto only reads
			// data.item.id, so get-run reads the SEEDED run (fixture.go) instead.
			{Label: "get-run", Method: "GET", Path: "/api/v1/import/get-run?id=" + ImportRunSeeded, Auth: "owner"},
			{Label: "err:get-run-foreign", Method: "GET", Path: "/api/v1/import/get-run?id=" + ImportRunSeeded, Auth: "guest"},
			{Label: "err:get-run-unknown", Method: "GET", Path: "/api/v1/import/get-run?id=00000000-0000-0000-0000-000000000000", Auth: "owner"},
			{Label: "get-source-list-after-sync", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "owner"},
			{Label: "get-queued-event-list-after-sync", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
		}
	}})
```

`get-run` reads a seeded run because the catalogue cannot thread `data.run.id` from `sync-source` into a later path (`Call.Path` is a static string and `CaptureIDInto` captures only `data.item.id`). In `fixture.go` add the constant `ImportRunSeeded = "0c000000-0000-0000-0000-000000000003"` next to `ImportSourceBank` and, right after the bank source seed, one completed run with two rows — one imported (pointing at `Txn1`) and one tombstone (`Status: linked`, no `TransactionID` — the transaction it pointed at was deleted, so `transactionId` is `""` on the wire):

```go
	f.ImportRun(fixture.ImportRun{ID: ImportRunSeeded, UserID: OwnerID, SourceID: ImportSourceBank, Provider: model.ImportProviderSimpleFIN,
		Status: model.ImportRunStatusCompleted, ImportedCount: 2, StartedAt: ClockTime, FinishedAt: &ClockTime})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: ImportSourceBank, RunID: ImportRunSeeded, ExternalAccountID: "ACT-CHK", ExternalTransactionID: "seed-1",
		TransactionID: Txn1, Status: model.ImportLinkStatusLinked, ExternalPayee: "Seeded Shop", ExternalAmount: "12.50000000", ExternalCurrency: "USD", ExternalPostedAt: ClockTime})
	f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: ImportSourceBank, RunID: ImportRunSeeded, ExternalAccountID: "ACT-CHK", ExternalTransactionID: "seed-0",
		Status: model.ImportLinkStatusLinked, ExternalPayee: "Deleted Later", ExternalAmount: "3.00000000", ExternalCurrency: "USD", ExternalPostedAt: ClockTime})
```

(`fixture.ImportRun` is the Task 1 builder; `ImportTransactionLink` gained `RunID` in Task 1.) The seeded run also appears in the `get-run-list` goldens alongside the runs the scenario creates; ordering is `started_at DESC, id DESC` and every id is fixed or minted deterministically per run, so the goldens are stable across engines.

`catalogue_ratelimit.go` — append to `auth_rate_limit` before `return calls` (5 claims allowed, the 6th is 429; every request counts):

```go
		for i := 1; i <= 5; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("claim-setup-token-%d", i), Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
				Body: map[string]any{"setupToken": "aGVsbG8="}})
		}
		calls = append(calls, Call{Label: "err:claim-setup-token-limited", Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
			Body: map[string]any{"setupToken": "aGVsbG8="}})
```

Update the file's header comment total (22 → 28 calls; still under the global 60/min backstop).

- [ ] **Step 4: Regenerate goldens and inspect**

Run:
```bash
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ 2>&1 | tail -5
git status --short internal/test/apiparity/testdata | head -40
git diff --stat internal/test/apiparity/testdata
```

Inspect every CHANGED (not just new) golden and confirm each diff is one of the intended behavior changes:
1. `get-source-list*` goldens: the new seeded bank source, and `lastSyncedAt`/`credentialCiphertext` fields on every source (empty for the wallet).
2. `link-account` golden: the run object now has the full `runResult` shape (`sourceId`, `provider`, `trigger`, `amountsUpdatedCount`, `queuedCount`, `errors`, `startedAt`, `finishedAt`).
3. Any ingest golden whose `status` flips `created` → `matched` (only if an adopt scenario exists).
4. `err:create-source-unknown-provider`: identical envelope (provider name changed from `simplefin` to `csv`, same message).

Anything else in the diff is a bug — fix the code, not the golden.

- [ ] **Step 5: Full smoke tier**

Run: `make go-test 2>&1 | tail -15`
Expected: PASS incl. the coverage gate, the i18n guards (the new `errors.import.*` keys were added in Task 3), archtest, apiparity guards (route count grew by 7, scenario count grew).

- [ ] **Step 6: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(apiparity): SimpleFIN credential and sync scenarios"
```

---

### Task 10: `importCrypto.ts` — passphrase-wrapped data key in IndexedDB

**Files:**
- Create: `web/src/lib/importCrypto.ts`, `web/src/lib/importCrypto.test.ts`
- Modify: `web/package.json` (devDependency `fake-indexeddb`), `web/src/test/setup.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks (pure client-side module; the wire shape it produces is the one Task 3's `SetImportCredentialKeyRequest` stores verbatim).
- Produces (`web/src/lib/importCrypto.ts`):
  ```ts
  export interface WrappedKey { wrappedDataKey: string; kdf: string }
  export const KDF_ITERATIONS = 600_000
  export class WrongPassphraseError extends Error {}
  export function createKey(passphrase: string): Promise<WrappedKey>          // random data key, wraps it, stores unwrapped in IndexedDB
  export function unlockKey(passphrase: string, wrapped: WrappedKey): Promise<void>  // throws WrongPassphraseError
  export function loadStoredKey(): Promise<CryptoKey | null>
  export function forgetKey(): Promise<void>
  export function encryptCredential(plaintext: string): Promise<string>       // throws KeyLockedError when no key stored
  export function decryptCredential(ciphertext: string): Promise<string>
  export function changePassphrase(oldPassphrase: string, wrapped: WrappedKey, newPassphrase: string): Promise<WrappedKey>
  export class KeyLockedError extends Error {}
  ```
- Formats (frozen — the server stores them opaque, a future client must read them back):
  - `kdf` = `JSON.stringify({ alg: 'PBKDF2-SHA256', salt: <base64 16 bytes>, iterations: 600000 })`
  - `wrappedDataKey` = `v1:<base64 iv 12 bytes>:<base64 AES-GCM ciphertext of the raw 32-byte data key>`
  - credential ciphertext = `v1:<base64 iv>:<base64 AES-GCM ciphertext of UTF-8 plaintext>`
  - IndexedDB database `econumo-import` v1, object store `keys`, record key `dataKey`, value = the **non-extractable** `CryptoKey` (structured-clone stores CryptoKey objects natively).

- [ ] **Step 1: Test environment**

`web/package.json` devDependencies: add `"fake-indexeddb": "^6.0.0"`, then `pnpm --dir web install`.

`web/src/test/setup.ts` — append before the `await import('@/app/i18n')` line:

```ts
// jsdom ships neither IndexedDB nor a full Web Crypto; the import credential
// key lives in both. Node's webcrypto is spec-compliant for what we use.
import 'fake-indexeddb/auto'
if (typeof globalThis.crypto?.subtle === 'undefined') {
  const { webcrypto } = await import('node:crypto')
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, writable: true, configurable: true })
}
```

(`fake-indexeddb/auto` installs `indexedDB`, `IDBKeyRange`, etc. on `globalThis`. Static imports hoist, so the `import` may sit at the top of the file with the others — put it there if oxlint's `import/first` complains.)

- [ ] **Step 2: Write the failing tests**

`web/src/lib/importCrypto.test.ts`:

```ts
import { IDBFactory } from 'fake-indexeddb'
import {
  KDF_ITERATIONS, KeyLockedError, WrongPassphraseError,
  changePassphrase, createKey, decryptCredential, encryptCredential, forgetKey, loadStoredKey, unlockKey,
} from './importCrypto'

beforeEach(() => {
  // a fresh IndexedDB per test; the module opens the db lazily on every call
  globalThis.indexedDB = new IDBFactory()
})

it('createKey stores a non-extractable key and returns a wrapped key with the documented kdf', async () => {
  const wrapped = await createKey('correct horse')
  expect(wrapped.wrappedDataKey).toMatch(/^v1:[A-Za-z0-9+/=]+:[A-Za-z0-9+/=]+$/)
  const kdf = JSON.parse(wrapped.kdf)
  expect(kdf.alg).toBe('PBKDF2-SHA256')
  expect(kdf.iterations).toBe(KDF_ITERATIONS)
  expect(atob(kdf.salt)).toHaveLength(16)
  const key = await loadStoredKey()
  expect(key?.extractable).toBe(false)
  expect(key?.usages.sort()).toEqual(['decrypt', 'encrypt'])
})

it('round-trips a credential and uses a fresh iv per call', async () => {
  await createKey('pw')
  const a = await encryptCredential('https://u:p@bridge.example/simplefin')
  const b = await encryptCredential('https://u:p@bridge.example/simplefin')
  expect(a).not.toBe(b)
  expect(await decryptCredential(a)).toBe('https://u:p@bridge.example/simplefin')
  expect(await decryptCredential(b)).toBe('https://u:p@bridge.example/simplefin')
})

it('unlockKey restores the same data key on a new device; the wrong passphrase is rejected', async () => {
  const wrapped = await createKey('pw')
  const ct = await encryptCredential('secret')
  await forgetKey()
  expect(await loadStoredKey()).toBeNull()
  await expect(encryptCredential('x')).rejects.toBeInstanceOf(KeyLockedError)
  await expect(unlockKey('nope', wrapped)).rejects.toBeInstanceOf(WrongPassphraseError)
  await unlockKey('pw', wrapped)
  expect(await decryptCredential(ct)).toBe('secret')
})

it('changePassphrase re-wraps the SAME data key, so old ciphertexts still decrypt', async () => {
  const wrapped = await createKey('old')
  const ct = await encryptCredential('secret')
  const rewrapped = await changePassphrase('old', wrapped, 'new')
  expect(rewrapped.wrappedDataKey).not.toBe(wrapped.wrappedDataKey)
  await forgetKey()
  await expect(unlockKey('old', rewrapped)).rejects.toBeInstanceOf(WrongPassphraseError)
  await unlockKey('new', rewrapped)
  expect(await decryptCredential(ct)).toBe('secret')
})

it('rejects a malformed ciphertext without touching the key', async () => {
  await createKey('pw')
  await expect(decryptCredential('garbage')).rejects.toThrow(/ciphertext/)
  await expect(decryptCredential('v2:a:b')).rejects.toThrow(/ciphertext/)
})
```

- [ ] **Step 3: Run to verify failure**

Run: `pnpm --dir web exec vitest run src/lib/importCrypto.test.ts 2>&1 | tail -15`
Expected: module not found.

- [ ] **Step 4: Implement**

`web/src/lib/importCrypto.ts`:

```ts
// Credential encryption for pull-import sources (SimpleFIN access URLs).
//
// The server stores only ciphertext and a passphrase-WRAPPED data key; the
// passphrase never leaves the device and the unwrapped data key is kept
// non-extractable in IndexedDB, so a script on the page can use it but not
// read its bytes. The server still sees the plaintext access URL for the
// duration of a sync request (it does the bank fetch) — "at rest
// zero-knowledge, in flight trusted".
//
// Formats are frozen: another client must be able to unlock what this one
// wrote.
//   kdf:            {"alg":"PBKDF2-SHA256","salt":<b64 16B>,"iterations":600000}
//   wrappedDataKey: v1:<b64 iv 12B>:<b64 AES-GCM(raw 32B data key)>
//   credential:     v1:<b64 iv 12B>:<b64 AES-GCM(utf8 plaintext)>

export interface WrappedKey {
  wrappedDataKey: string
  kdf: string
}

export const KDF_ITERATIONS = 600_000
const KDF_ALG = 'PBKDF2-SHA256'
const DB_NAME = 'econumo-import'
const STORE = 'keys'
const RECORD = 'dataKey'
const VERSION_TAG = 'v1'

export class WrongPassphraseError extends Error {
  constructor() {
    super('Wrong passphrase')
    this.name = 'WrongPassphraseError'
  }
}

export class KeyLockedError extends Error {
  constructor() {
    super('Credential key is locked')
    this.name = 'KeyLockedError'
  }
}

interface KdfParams {
  alg: string
  salt: string
  iterations: number
}

const b64 = (bytes: ArrayBuffer | Uint8Array): string => btoa(String.fromCharCode(...new Uint8Array(bytes)))
const unb64 = (s: string): Uint8Array => Uint8Array.from(atob(s), (c) => c.charCodeAt(0))

function encode(iv: Uint8Array, ct: ArrayBuffer): string {
  return `${VERSION_TAG}:${b64(iv)}:${b64(ct)}`
}

function decode(value: string, what: string): { iv: Uint8Array; ct: Uint8Array } {
  const parts = value.split(':')
  if (parts.length !== 3 || parts[0] !== VERSION_TAG) {
    throw new Error(`Malformed ${what}`)
  }
  try {
    return { iv: unb64(parts[1]), ct: unb64(parts[2]) }
  } catch {
    throw new Error(`Malformed ${what}`)
  }
}

async function wrappingKey(passphrase: string, params: KdfParams): Promise<CryptoKey> {
  if (params.alg !== KDF_ALG) {
    throw new Error(`Unsupported kdf ${params.alg}`)
  }
  const material = await crypto.subtle.importKey('raw', new TextEncoder().encode(passphrase), 'PBKDF2', false, ['deriveKey'])
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', hash: 'SHA-256', salt: unb64(params.salt), iterations: params.iterations },
    material,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt'],
  )
}

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1)
    req.onupgradeneeded = () => req.result.createObjectStore(STORE)
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

function tx<T>(mode: IDBTransactionMode, run: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return openDb().then((db) => new Promise<T>((resolve, reject) => {
    const t = db.transaction(STORE, mode)
    const req = run(t.objectStore(STORE))
    t.oncomplete = () => { db.close(); resolve(req.result) }
    t.onerror = () => { db.close(); reject(t.error) }
    t.onabort = () => { db.close(); reject(t.error) }
  }))
}

export async function loadStoredKey(): Promise<CryptoKey | null> {
  const value = await tx<CryptoKey | undefined>('readonly', (s) => s.get(RECORD) as IDBRequest<CryptoKey | undefined>)
  return value ?? null
}

async function storeKey(key: CryptoKey): Promise<void> {
  await tx('readwrite', (s) => s.put(key, RECORD))
}

export async function forgetKey(): Promise<void> {
  await tx('readwrite', (s) => s.delete(RECORD))
}

async function wrap(rawDataKey: ArrayBuffer, passphrase: string, params: KdfParams): Promise<WrappedKey> {
  const wk = await wrappingKey(passphrase, params)
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, wk, rawDataKey)
  return { wrappedDataKey: encode(iv, ct), kdf: JSON.stringify(params) }
}

async function unwrapRaw(passphrase: string, wrapped: WrappedKey): Promise<ArrayBuffer> {
  const params = JSON.parse(wrapped.kdf) as KdfParams
  const wk = await wrappingKey(passphrase, params)
  const { iv, ct } = decode(wrapped.wrappedDataKey, 'wrapped key')
  try {
    return await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, wk, ct)
  } catch {
    // AES-GCM authentication failure is the only way a wrong passphrase shows up
    throw new WrongPassphraseError()
  }
}

function importDataKey(raw: ArrayBuffer): Promise<CryptoKey> {
  return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
}

function freshParams(): KdfParams {
  return { alg: KDF_ALG, salt: b64(crypto.getRandomValues(new Uint8Array(16))), iterations: KDF_ITERATIONS }
}

export async function createKey(passphrase: string): Promise<WrappedKey> {
  const raw = crypto.getRandomValues(new Uint8Array(32)).buffer
  const wrapped = await wrap(raw, passphrase, freshParams())
  await storeKey(await importDataKey(raw))
  return wrapped
}

export async function unlockKey(passphrase: string, wrapped: WrappedKey): Promise<void> {
  const raw = await unwrapRaw(passphrase, wrapped)
  await storeKey(await importDataKey(raw))
}

// The data key itself never changes (existing ciphertexts stay valid); only
// the passphrase that wraps it does. Requires the old passphrase because the
// stored key is non-extractable — the raw bytes only exist unwrapped here.
export async function changePassphrase(oldPassphrase: string, wrapped: WrappedKey, newPassphrase: string): Promise<WrappedKey> {
  const raw = await unwrapRaw(oldPassphrase, wrapped)
  const rewrapped = await wrap(raw, newPassphrase, freshParams())
  await storeKey(await importDataKey(raw))
  return rewrapped
}

async function requireKey(): Promise<CryptoKey> {
  const key = await loadStoredKey()
  if (!key) {
    throw new KeyLockedError()
  }
  return key
}

export async function encryptCredential(plaintext: string): Promise<string> {
  const key = await requireKey()
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(plaintext))
  return encode(iv, ct)
}

export async function decryptCredential(ciphertext: string): Promise<string> {
  const { iv, ct } = decode(ciphertext, 'ciphertext')
  const key = await requireKey()
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ct)
  return new TextDecoder().decode(pt)
}
```

If `tsc` complains about `Uint8Array` vs `BufferSource` in the `subtle.*` calls (TS 5.7+ `Uint8Array<ArrayBufferLike>`), pass `iv as BufferSource` / `ct as BufferSource` — the runtime accepts both.

- [ ] **Step 5: Run the tests + type check**

Run: `pnpm --dir web exec vitest run src/lib/importCrypto.test.ts 2>&1 | tail -8 && pnpm --dir web exec tsc -b && pnpm --dir web lint 2>&1 | tail -3`
Expected: 5 passed; clean.

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/pnpm-lock.yaml web/src/test/setup.ts web/src/lib/importCrypto.ts web/src/lib/importCrypto.test.ts
git commit -m "feat(web): passphrase-wrapped credential key for pull imports"
```

---

### Task 11: SPA API client, DTOs, query hooks, metrics

**Files:**
- Modify: `web/src/api/dto/imports.ts`, `web/src/api/imports.ts`, `web/src/features/imports/queries.ts`, `web/src/app/queryKeys.ts`, `web/src/lib/metrics.ts`, `web/src/test/fixtures.ts`
- Modify: `web/src/features/imports/queries.test.tsx`

**Interfaces:**
- Consumes: Task 3 wire DTOs (field names verbatim), Task 8 routes.
- Produces:
  - DTOs: `ImportProvider = 'apple-wallet' | 'simplefin'`, `ImportSourceDto` + `lastSyncedAt: string`, `credentialCiphertext: string`; `ImportRunDto` extended (`sourceId, provider, trigger, amountsUpdatedCount, queuedCount, errors: ImportRunErrorDto[], startedAt, finishedAt`); new `ImportRunErrorDto`, `ImportRunLinkDto`, `ImportRunDetailDto { item: ImportRunDto; links: ImportRunLinkDto[] }`, `ExternalAccountDto`, `ImportCredentialKeyDto { wrappedDataKey; kdf; updatedAt }`, `SyncImportSourceResultDto { run: ImportRunDto; accounts: ExternalAccountDto[] }`.
  - API: `claimSetupToken(setupToken)`, `createImportSource(provider, name, credentialCiphertext?)`, `getImportCredentialKey()` (returns `null` on the coded not-found 400), `setImportCredentialKey(key)`, `listExternalAccounts(sourceId, accessUrl)`, `syncImportSource(sourceId, accessUrl, startDate, endDate?)`, `getImportRunList(sourceId?)`, `getImportRun(id)`.
  - Hooks: `useImportCredentialKey()`, `useSetImportCredentialKey()`, `useClaimSetupToken()`, `useExternalAccounts(sourceId, accessUrl | null)`, `useSyncImportSource()`, `useImportRuns(sourceId?)`, `useImportRun(id)`; `useCreateImportSource` accepts `{ provider: ImportProvider; name: string; credentialCiphertext?: string }`.
  - `queryKeys.importRuns(sourceId: string)` (`''` = all), `queryKeys.importRun(id)`, `queryKeys.importCredentialKey`, `queryKeys.importExternalAccounts(sourceId)`.
  - `METRICS.IMPORT_SYNC = 'appImportSync'`.

- [ ] **Step 1: DTOs**

`web/src/api/dto/imports.ts` — replace `ImportProvider`, extend `ImportSourceDto` and `ImportRunDto`, add the new types:

```ts
export type ImportProvider = 'apple-wallet' | 'simplefin'
export type ImportRunStatus = 'running' | 'completed' | 'partial' | 'failed'

export interface ImportSourceDto {
  id: Id
  provider: ImportProvider
  name: string
  status: string
  createdAt: string
  /** "YYYY-MM-DD HH:mm:ss" of the last non-failed sync, '' before the first */
  lastSyncedAt: string
  /** opaque to the server; '' for push providers */
  credentialCiphertext: string
  cards: ImportCardDto[]
}

export interface ImportRunErrorDto {
  /** '' for a run-level (bridge) error */
  externalAccountId: string
  message: string
}

export interface ImportRunDto {
  id: Id
  sourceId: Id
  provider: ImportProvider
  status: ImportRunStatus
  trigger: string
  importedCount: number
  matchedCount: number
  amountsUpdatedCount: number
  queuedCount: number
  skippedCount: number
  failedCount: number
  errors: ImportRunErrorDto[]
  startedAt: string
  /** '' while running */
  finishedAt: string
}

export interface ImportRunLinkDto {
  id: Id
  externalAccountId: string
  externalTransactionId: string
  /** '' for queued/skipped rows and for tombstones (transaction deleted since) */
  transactionId: Id | ''
  status: string
  externalPayee: string
  externalAmount: string
  externalCurrency: string
  externalPostedAt: string
}

export interface ImportRunDetailDto {
  item: ImportRunDto
  links: ImportRunLinkDto[]
}

export interface ExternalAccountDto {
  externalAccountId: string
  externalName: string
  externalCurrency: string
  balance: string
  orgName: string
  state: ImportCardState
  accountId: Id | ''
}

export interface ImportCredentialKeyDto {
  wrappedDataKey: string
  kdf: string
  updatedAt: string
}

export interface SyncImportSourceResultDto {
  run: ImportRunDto
  accounts: ExternalAccountDto[]
}
```

Keep the existing `UpdateImportAccountDto`, queue and provenance types as they are. Search for other places that build an `ImportSourceDto`/`ImportRunDto` literal (`grep -rn "provider: 'apple-wallet'" web/src --include=*.test.tsx`, `web/src/test/fixtures.ts`) and add the two new source fields (`lastSyncedAt: ''`, `credentialCiphertext: ''`) and the run fields where the literal is typed — untyped `unknown` fixtures need no change.

- [ ] **Step 2: API client**

`web/src/api/imports.ts` — change `createImportSource` and append:

```ts
export async function createImportSource(provider: ImportProvider, name: string, credentialCiphertext?: string): Promise<ImportSourceDto> {
  const body: Record<string, string> = { provider, name }
  if (credentialCiphertext) {
    body.credentialCiphertext = credentialCiphertext
  }
  const response = await api.post<Envelope<{ item: ImportSourceDto }>>(apiUrl('/api/v1/import/create-source'), body)
  return response.data.data.item
}

export async function claimSetupToken(setupToken: string): Promise<string> {
  const response = await api.post<Envelope<{ accessUrl: string }>>(apiUrl('/api/v1/import/claim-setup-token'), { setupToken })
  return response.data.data.accessUrl
}

// A user without a key gets the coded not-found envelope (400); that is the
// ordinary "connect for the first time" state, not an error.
export async function getImportCredentialKey(): Promise<ImportCredentialKeyDto | null> {
  try {
    const response = await api.get<Envelope<ImportCredentialKeyDto>>(apiUrl('/api/v1/import/get-credential-key'))
    return response.data.data
  } catch (err) {
    if (axios.isAxiosError(err) && err.response?.status === 400) {
      return null
    }
    throw err
  }
}

export async function setImportCredentialKey(key: { wrappedDataKey: string; kdf: string }): Promise<ImportCredentialKeyDto> {
  const response = await api.post<Envelope<ImportCredentialKeyDto>>(apiUrl('/api/v1/import/set-credential-key'), key)
  return response.data.data
}

export async function listExternalAccounts(sourceId: Id, accessUrl: string): Promise<ExternalAccountDto[]> {
  const response = await api.post<Envelope<{ items: ExternalAccountDto[] }>>(apiUrl('/api/v1/import/list-external-accounts'), { sourceId, accessUrl })
  return response.data.data.items
}

export async function syncImportSource(sourceId: Id, accessUrl: string, startDate: string, endDate?: string): Promise<SyncImportSourceResultDto> {
  const body: Record<string, string> = { sourceId, accessUrl, startDate }
  if (endDate) {
    body.endDate = endDate
  }
  const response = await api.post<Envelope<SyncImportSourceResultDto>>(apiUrl('/api/v1/import/sync-source'), body)
  return response.data.data
}

export async function getImportRunList(sourceId?: Id): Promise<ImportRunDto[]> {
  const response = await api.get<Envelope<{ items: ImportRunDto[] }>>(
    apiUrl('/api/v1/import/get-run-list'),
    { params: sourceId ? { sourceId } : {} },
  )
  return response.data.data.items
}

export async function getImportRun(id: Id): Promise<ImportRunDetailDto> {
  const response = await api.get<Envelope<ImportRunDetailDto>>(apiUrl('/api/v1/import/get-run'), { params: { id } })
  return response.data.data
}
```

Add `import axios from 'axios'` and the new DTO type imports at the top. Check how `web/src/api/client.ts` exposes error status (if there is an `isApiError`/`apiStatus` helper used elsewhere in `web/src/api/*.ts`, use it instead of `axios.isAxiosError`).

- [ ] **Step 3: Query keys + metrics**

`web/src/app/queryKeys.ts` — after `transactionImports`:

```ts
  importRuns: (sourceId: string) => ['importRuns', sourceId] as const,
  importRun: (id: string) => ['importRun', id] as const,
  importCredentialKey: ['importCredentialKey'] as const,
  importExternalAccounts: (sourceId: string) => ['importExternalAccounts', sourceId] as const,
```

`web/src/lib/metrics.ts` — after `IMPORT_SHORTCUT_CHECK`:

```ts
  IMPORT_SYNC: 'appImportSync',
```

- [ ] **Step 4: Hooks**

`web/src/features/imports/queries.ts` — change `useCreateImportSource`'s `mutationFn` type and call:

```ts
    mutationFn: ({ provider, name, credentialCiphertext }: { provider: ImportProvider; name: string; credentialCiphertext?: string }) =>
      importsApi.createImportSource(provider, name, credentialCiphertext),
```

and append:

```ts
export function useImportCredentialKey() {
  return useQuery({ queryKey: queryKeys.importCredentialKey, queryFn: importsApi.getImportCredentialKey, staleTime: TEN_MINUTES })
}

export function useSetImportCredentialKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: importsApi.setImportCredentialKey,
    onSuccess: (key) => queryClient.setQueryData(queryKeys.importCredentialKey, key),
  })
}

export function useClaimSetupToken() {
  return useMutation({ mutationFn: importsApi.claimSetupToken })
}

// The access URL is a per-request secret: it is the query's input, never
// part of the key (keys are visible in devtools and persisted caches).
export function useExternalAccounts(sourceId: Id, accessUrl: string | null) {
  return useQuery({
    queryKey: queryKeys.importExternalAccounts(sourceId),
    queryFn: () => importsApi.listExternalAccounts(sourceId, accessUrl ?? ''),
    enabled: accessUrl !== null,
    staleTime: TEN_MINUTES,
    gcTime: 0,
  })
}

export function useSyncImportSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ sourceId, accessUrl, startDate, endDate }: { sourceId: Id; accessUrl: string; startDate: string; endDate?: string }) =>
      importsApi.syncImportSource(sourceId, accessUrl, startDate, endDate),
    onSuccess: (result, vars) => {
      queryClient.setQueryData(queryKeys.importExternalAccounts(vars.sourceId), result.accounts)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
      void queryClient.invalidateQueries({ queryKey: ['importRuns'] })
      if (result.run.importedCount + result.run.matchedCount + result.run.amountsUpdatedCount > 0) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
        void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
        void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
      }
      trackEvent(METRICS.IMPORT_SYNC, { trigger: result.run.trigger, imported: result.run.importedCount, matched: result.run.matchedCount })
    },
    // a failed sync still wrote a run row
    onError: () => void queryClient.invalidateQueries({ queryKey: ['importRuns'] }),
  })
}

export function useImportRuns(sourceId = '') {
  return useQuery({ queryKey: queryKeys.importRuns(sourceId), queryFn: () => importsApi.getImportRunList(sourceId || undefined), staleTime: TEN_MINUTES })
}

export function useImportRun(id: Id) {
  return useQuery({ queryKey: queryKeys.importRun(id), queryFn: () => importsApi.getImportRun(id), staleTime: TEN_MINUTES })
}
```

Add `ImportProvider` to the type import line.

- [ ] **Step 5: msw fixtures**

`web/src/test/fixtures.ts` — in `coreHandlers` defaults add `importRuns: [] as unknown[]`, `importCredentialKey: null as unknown`, and in the handler list:

```ts
    http.get('*/api/v1/import/get-run-list', () => envelope({ items: data.importRuns })),
    http.get('*/api/v1/import/get-credential-key', () =>
      data.importCredentialKey
        ? envelope(data.importCredentialKey)
        : HttpResponse.json({ success: false, message: 'Credential key not found', code: 400, errors: {} }, { status: 400 })),
```

- [ ] **Step 6: Hook tests**

Append to `web/src/features/imports/queries.test.tsx` (follow the file's existing `renderHook` + `QueryClientProvider` wrapper and `vi.mock('@/lib/metrics')` pattern — read the top of the file first):

```ts
it('useSyncImportSource refreshes ledger caches only when the run wrote something, and fires IMPORT_SYNC', async () => {
  const run = {
    id: 'r1', sourceId: 's2', provider: 'simplefin', status: 'completed', trigger: 'manual',
    importedCount: 2, matchedCount: 1, amountsUpdatedCount: 0, queuedCount: 0, skippedCount: 0, failedCount: 0,
    errors: [], startedAt: '2026-09-07 10:00:00', finishedAt: '2026-09-07 10:00:02',
  }
  let body: unknown
  server.use(http.post('*/api/v1/import/sync-source', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { run, accounts: [] } })
  }))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.transactions, { stale: true })
  const { result } = renderHook(() => useSyncImportSource(), { wrapper })
  await act(() => result.current.mutateAsync({ sourceId: 's2', accessUrl: 'https://u:p@b/x', startDate: '2026-08-01' }))
  expect(body).toEqual({ sourceId: 's2', accessUrl: 'https://u:p@b/x', startDate: '2026-08-01' })
  expect(queryClient.getQueryState(queryKeys.transactions)?.isInvalidated).toBe(true)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.IMPORT_SYNC, { trigger: 'manual', imported: 2, matched: 1 })
})

it('getImportCredentialKey maps the coded not-found 400 to null', async () => {
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useImportCredentialKey(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data).toBeNull()
})
```

(`makeWrapper` is whatever helper the file already uses to build a `QueryClient` + provider; if it has a different name, use that. `coreHandlers` is registered in the file's `beforeEach`; the credential-key default is `null` so the second test needs no override.)

- [ ] **Step 7: Run**

Run: `pnpm --dir web exec vitest run src/features/imports src/lib/metrics-coverage.test.ts 2>&1 | tail -8 && pnpm --dir web exec tsc -b && pnpm --dir web lint 2>&1 | tail -3`
Expected: PASS — `metrics-coverage` passes because `IMPORT_SYNC` is referenced in `queries.ts`.

- [ ] **Step 8: Commit**

```bash
git add web/src/api web/src/features/imports/queries.ts web/src/features/imports/queries.test.tsx web/src/app/queryKeys.ts web/src/lib/metrics.ts web/src/test/fixtures.ts
git commit -m "feat(web): SimpleFIN API client, hooks and IMPORT_SYNC metric"
```

---

### Task 12: Settings → SimpleFIN page (connect, unlock, accounts, Sync)

**Files:**
- Create: `web/src/features/imports/useImportKey.ts`, `web/src/features/imports/SimpleFINPage.tsx`, `web/src/features/imports/SimpleFINConnect.tsx`, `web/src/features/imports/SimpleFINUnlock.tsx`, `web/src/features/imports/ImportRunSummary.tsx`, `web/src/features/imports/SimpleFINPage.test.tsx`
- Modify: `web/src/features/imports/ImportCards.tsx`, `web/src/features/imports/queries.ts` (link/ignore accept `externalName`), `web/src/api/imports.ts` (same), `web/src/app/queryKeys.ts` (`importLocalKey`), `web/src/app/router-pages.ts`, `web/src/app/routes.tsx`, `web/src/features/settings/SettingsPage.tsx`, all 11 `locales/<lang>.json`

**Interfaces:**
- Consumes: Task 10 `importCrypto` API; Task 11 DTOs, hooks (`useImportCredentialKey`, `useSetImportCredentialKey`, `useClaimSetupToken`, `useExternalAccounts`, `useSyncImportSource`, `useImportRuns`, `useCreateImportSource`, `useImportSources`).
- Produces:
  - `useImportKey(): { state: ImportKeyState; refresh: () => void }` with `ImportKeyState = { status: 'loading' } | { status: 'none' } | { status: 'locked'; wrapped: ImportCredentialKeyDto } | { status: 'unlocked'; wrapped: ImportCredentialKeyDto }`.
  - `ImportRunSummary({ run }: { run: ImportRunDto })` — one card summarising a run (status badge, counts, errors); Task 13 wraps it in a link to the run page.
  - `ImportCards({ source, cards?, variant? })` — `variant: 'card' | 'account'` (default `'card'`) switches copy to `imports.simplefin.accounts.*` and hides the tap counter; `cards` overrides `source.cards`.
  - `RouterPage.SETTINGS_SIMPLEFIN = '/settings/simplefin'`.
  - Locale namespace `imports.simplefin.*` (keys listed in Step 6).

- [ ] **Step 1: `externalName` on link/ignore (Task 3 made it optional on the wire)**

`web/src/api/imports.ts` — `linkImportAccount(sourceId, externalAccountId, accountId, externalName?)` and `ignoreImportAccount(sourceId, externalAccountId, externalName?)` add `externalName` to the body only when given. `web/src/features/imports/queries.ts` — the two `mutationFn` signatures gain `externalName?: string` and pass it through. A pull source's account rows come from the bridge, not from a prior event, so the server has no name for them until the link/ignore call carries one.

- [ ] **Step 2: `useImportKey`**

`web/src/app/queryKeys.ts` — add `importLocalKey: ['importLocalKey'] as const,`.

`web/src/features/imports/useImportKey.ts`:

```ts
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { ImportCredentialKeyDto } from '@/api/dto/imports'
import { queryKeys } from '@/app/queryKeys'
import { loadStoredKey } from '@/lib/importCrypto'
import { useImportCredentialKey } from './queries'

export type ImportKeyState =
  | { status: 'loading' }
  | { status: 'none' }
  | { status: 'locked'; wrapped: ImportCredentialKeyDto }
  | { status: 'unlocked'; wrapped: ImportCredentialKeyDto }

// Server half: the passphrase-wrapped key (shared by every device). Local
// half: whether THIS device holds the unwrapped key in IndexedDB. Both are
// queries so a successful unlock/forget only has to invalidate the local one.
export function useImportKey(): { state: ImportKeyState; refresh: () => void } {
  const queryClient = useQueryClient()
  const remote = useImportCredentialKey()
  const local = useQuery({ queryKey: queryKeys.importLocalKey, queryFn: loadStoredKey, staleTime: Infinity, gcTime: 0 })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: queryKeys.importLocalKey })
  if (remote.isPending || local.isPending) {
    return { state: { status: 'loading' }, refresh }
  }
  if (!remote.data) {
    return { state: { status: 'none' }, refresh }
  }
  return { state: { status: local.data ? 'unlocked' : 'locked', wrapped: remote.data }, refresh }
}
```

- [ ] **Step 3: `ImportCards` variant**

`web/src/features/imports/ImportCards.tsx` — change the signature and the copy lookups:

```ts
export function ImportCards({ source, cards = source.cards, variant = 'card' }: { source: ImportSourceDto; cards?: ImportCardDto[]; variant?: 'card' | 'account' }) {
  const { t, i18n } = useTranslation()
  const ns = variant === 'account' ? 'imports.simplefin.accounts' : 'imports.apple_wallet.cards'
```

then replace every `t('imports.apple_wallet.cards.` in the component body with `` t(`${ns}. `` (same suffixes; keys below exist under both namespaces with identical `{var}` sets), iterate over `cards` instead of `source.cards`, render the taps line only when `variant === 'card'`, and pass `externalName: card.externalName` in the `link.mutate` and `ignore.mutate` variables. The `i18ntest` frontend key-coverage guard resolves `t()` calls with a template literal by prefix — check `internal/test/i18ntest` for how dynamic keys are matched; if it requires literal keys, write a small `const keys = variant === 'account' ? ACCOUNT_KEYS : CARD_KEYS` object with each full key spelled out as a literal, and index that instead.

- [ ] **Step 4: Connect + Unlock + run summary components**

`web/src/features/imports/SimpleFINConnect.tsx`:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { PasswordInput } from '@/components/PasswordInput'
import { Button } from '@/components/ui/button'
import { InfoBox } from '@/components/InfoBox'
import { apiErrorMessage } from '@/lib/apiError'
import { WrongPassphraseError, createKey, encryptCredential, unlockKey } from '@/lib/importCrypto'
import type { ImportKeyState } from './useImportKey'
import { useClaimSetupToken, useCreateImportSource, useSetImportCredentialKey } from './queries'

const SOURCE_NAME = 'SimpleFIN'

// The whole connect flow, in order: (1) make sure this device holds the data
// key — create one (first device), unlock the existing one, or, when the
// passphrase is lost, replace it; (2) claim the setup token (the server
// returns the access URL and forgets it); (3) encrypt the URL here and store
// only the ciphertext. The access URL is held in component state and handed
// to the parent — never persisted.
export function SimpleFINConnect({ keyState, onConnected, reconnect = false }: {
  keyState: Exclude<ImportKeyState, { status: 'loading' }>
  onConnected: (accessUrl: string) => void
  reconnect?: boolean
}) {
  const { t } = useTranslation()
  const claim = useClaimSetupToken()
  const setKey = useSetImportCredentialKey()
  const create = useCreateImportSource()
  const [setupToken, setSetupToken] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [confirm, setConfirm] = useState('')
  const [resetKey, setResetKey] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const needsNewKey = keyState.status === 'none' || (keyState.status === 'locked' && resetKey)
  const needsPassphrase = keyState.status !== 'unlocked' || needsNewKey

  const submit = async () => {
    setError(null)
    if (!setupToken.trim()) {
      setError(t('imports.simplefin.connect.errors.token_required'))
      return
    }
    if (needsPassphrase && passphrase.length < 8) {
      setError(t('imports.simplefin.connect.errors.passphrase_short'))
      return
    }
    if (needsNewKey && passphrase !== confirm) {
      setError(t('imports.simplefin.connect.errors.passphrase_mismatch'))
      return
    }
    setBusy(true)
    try {
      if (needsNewKey) {
        await setKey.mutateAsync(await createKey(passphrase))
      } else if (keyState.status === 'locked') {
        await unlockKey(passphrase, keyState.wrapped)
      }
      const accessUrl = await claim.mutateAsync(setupToken.trim())
      const credentialCiphertext = await encryptCredential(accessUrl)
      await create.mutateAsync({ provider: 'simplefin', name: SOURCE_NAME, credentialCiphertext })
      onConnected(accessUrl)
    } catch (err) {
      setError(err instanceof WrongPassphraseError ? t('imports.simplefin.unlock.wrong_passphrase') : apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <InfoBox>{t(reconnect ? 'imports.simplefin.connect.reconnect_intro' : 'imports.simplefin.connect.intro')}</InfoBox>
      <label className="text-xs uppercase text-muted-foreground" htmlFor="simplefin-token">{t('imports.simplefin.connect.token_label')}</label>
      <textarea
        id="simplefin-token" rows={3} value={setupToken} onChange={(e) => setSetupToken(e.target.value)}
        className="rounded-lg border bg-background px-3 py-2 text-sm"
        placeholder={t('imports.simplefin.connect.token_placeholder')}
      />
      <p className="text-xs text-muted-foreground">{t('imports.simplefin.connect.token_help')}</p>
      {keyState.status === 'locked' && !resetKey ? (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={resetKey} onCheckedChange={(v) => setResetKey(v === true)} />
          {t('imports.simplefin.connect.forgot_passphrase')}
        </label>
      ) : null}
      {needsPassphrase ? (
        <>
          <label className="text-xs uppercase text-muted-foreground" htmlFor="simplefin-passphrase">
            {t(needsNewKey ? 'imports.simplefin.connect.passphrase_new' : 'imports.simplefin.connect.passphrase_existing')}
          </label>
          <PasswordInput id="simplefin-passphrase" value={passphrase} onChange={(e) => setPassphrase(e.target.value)} autoComplete="new-password" />
          {needsNewKey ? (
            <>
              <PasswordInput
                aria-label={t('imports.simplefin.connect.passphrase_confirm')} placeholder={t('imports.simplefin.connect.passphrase_confirm')}
                value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password"
              />
              <p className="text-xs text-muted-foreground">{t('imports.simplefin.connect.passphrase_help')}</p>
            </>
          ) : null}
        </>
      ) : null}
      {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
      <Button type="button" className="w-full sm:w-auto" disabled={busy} onClick={() => void submit()}>
        {t(reconnect ? 'imports.simplefin.connect.submit_reconnect' : 'imports.simplefin.connect.submit')}
      </Button>
    </div>
  )
}
```

`web/src/features/imports/SimpleFINUnlock.tsx`:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ImportCredentialKeyDto } from '@/api/dto/imports'
import { PasswordInput } from '@/components/PasswordInput'
import { Button } from '@/components/ui/button'
import { InfoBox } from '@/components/InfoBox'
import { WrongPassphraseError, forgetKey, unlockKey } from '@/lib/importCrypto'

export function SimpleFINUnlock({ wrapped, onUnlocked, onReconnect }: {
  wrapped: ImportCredentialKeyDto
  onUnlocked: () => void
  onReconnect: () => void
}) {
  const { t } = useTranslation()
  const [passphrase, setPassphrase] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    setBusy(true)
    setError(null)
    try {
      await unlockKey(passphrase, wrapped)
      onUnlocked()
    } catch (err) {
      setError(err instanceof WrongPassphraseError ? t('imports.simplefin.unlock.wrong_passphrase') : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <InfoBox>{t('imports.simplefin.unlock.intro')}</InfoBox>
      <PasswordInput
        aria-label={t('imports.simplefin.unlock.passphrase')} placeholder={t('imports.simplefin.unlock.passphrase')}
        value={passphrase} onChange={(e) => setPassphrase(e.target.value)} autoComplete="current-password"
        onKeyDown={(e) => { if (e.key === 'Enter') void submit() }}
      />
      {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
      <div className="flex flex-wrap gap-2">
        <Button type="button" disabled={busy || !passphrase} onClick={() => void submit()}>{t('imports.simplefin.unlock.submit')}</Button>
        <Button type="button" variant="secondary" onClick={onReconnect}>{t('imports.simplefin.unlock.reconnect')}</Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('imports.simplefin.unlock.forgot_help')}</p>
    </div>
  )
}

// "Forget this device" lives on the page (it is also offered while unlocked).
export async function forgetThisDevice(): Promise<void> {
  await forgetKey()
}
```

`web/src/features/imports/ImportRunSummary.tsx`:

```tsx
import { useTranslation } from 'react-i18next'
import type { ImportRunDto } from '@/api/dto/imports'
import { formatDateTime, parseDateTime } from '@/lib/datetime'
import { cn } from '@/lib/utils'

const STATUS_CLASS: Record<ImportRunDto['status'], string> = {
  running: 'text-muted-foreground',
  completed: 'text-econumo-green',
  partial: 'text-econumo-yellow',
  failed: 'text-destructive',
}

export function ImportRunSummary({ run, accountName }: { run: ImportRunDto; accountName?: (externalAccountId: string) => string }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-1 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className={cn('font-medium', STATUS_CLASS[run.status])}>{t(`imports.runs.status.${run.status}`)}</span>
        <span className="text-xs text-muted-foreground">{formatDateTime(parseDateTime(run.startedAt))}</span>
      </div>
      <div className="text-xs text-muted-foreground">
        {t('imports.runs.counts', {
          imported: run.importedCount, matched: run.matchedCount, updated: run.amountsUpdatedCount,
          queued: run.queuedCount, skipped: run.skippedCount, failed: run.failedCount,
        })}
      </div>
      {run.errors.map((e, i) => (
        <div key={i} className="text-xs text-destructive">
          {e.externalAccountId ? `${accountName?.(e.externalAccountId) || e.externalAccountId}: ` : ''}{e.message}
        </div>
      ))}
    </div>
  )
}
```

(Check the Tailwind palette names in `web/src/index.css` / existing components — use whatever `econumo-*` green/yellow tokens exist there; `text-destructive` is shadcn's.)

- [ ] **Step 5: The page**

`web/src/features/imports/SimpleFINPage.tsx`:

```tsx
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportCardDto, ImportSourceDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { InfoBox } from '@/components/InfoBox'
import { useAccounts } from '@/features/accounts/queries'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDateTime, parseDateTime } from '@/lib/datetime'
import { decryptCredential } from '@/lib/importCrypto'
import { ImportCards } from './ImportCards'
import { ImportRunSummary } from './ImportRunSummary'
import { SimpleFINConnect } from './SimpleFINConnect'
import { SimpleFINUnlock, forgetThisDevice } from './SimpleFINUnlock'
import { useImportKey } from './useImportKey'
import { useExternalAccounts, useImportRuns, useImportSources, useSyncImportSource } from './queries'

// Default sync window: overlap the previous sync by a few days so pending
// rows that posted late are still picked up; a first sync looks back a month.
const OVERLAP_DAYS = 3
const FIRST_SYNC_DAYS = 30

function isoDay(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function defaultStartDate(source: ImportSourceDto): string {
  const from = source.lastSyncedAt ? parseDateTime(source.lastSyncedAt) : new Date()
  from.setDate(from.getDate() - (source.lastSyncedAt ? OVERLAP_DAYS : FIRST_SYNC_DAYS))
  return isoDay(from)
}

export function SimpleFINPage() {
  const { t } = useTranslation()
  const { data: sources = [], isPending: sourcesPending } = useImportSources()
  const { data: accounts = [] } = useAccounts()
  const { state: keyState, refresh: refreshKey } = useImportKey()
  const source = sources.find((s) => s.provider === 'simplefin') ?? null
  const [accessUrl, setAccessUrl] = useState<string | null>(null)
  const [reconnecting, setReconnecting] = useState(false)
  const [startDate, setStartDate] = useState('')
  const sync = useSyncImportSource()
  const { data: runs = [] } = useImportRuns(source?.id ?? '')
  const external = useExternalAccounts(source?.id ?? '', accessUrl)

  // Decrypt the stored credential once the device key is available. The
  // plaintext stays in React state for this page's lifetime only.
  useEffect(() => {
    if (!source?.credentialCiphertext || keyState.status !== 'unlocked' || accessUrl !== null) {
      return
    }
    let cancelled = false
    decryptCredential(source.credentialCiphertext)
      .then((url) => { if (!cancelled) setAccessUrl(url) })
      .catch(() => { if (!cancelled) setReconnecting(true) })  // ciphertext from another key: reconnect
    return () => { cancelled = true }
  }, [source?.credentialCiphertext, keyState.status, accessUrl])

  useEffect(() => {
    if (source && !startDate) {
      setStartDate(defaultStartDate(source))
    }
  }, [source, startDate])

  // Bridge accounts merged over the server's mapping rows: an account the
  // bridge reports that no row exists for yet shows up unmapped.
  const cards: ImportCardDto[] = useMemo(() => {
    if (!source) {
      return []
    }
    const byId = new Map(source.cards.map((c) => [c.externalAccountId, c]))
    const merged = (external.data ?? []).map((a) => byId.get(a.externalAccountId) ?? {
      externalAccountId: a.externalAccountId, externalName: a.externalName, externalCurrency: a.externalCurrency,
      state: a.state, accountId: a.accountId, queuedCount: 0, tapCount: 0, lastSeenAt: '',
    })
    const seen = new Set(merged.map((c) => c.externalAccountId))
    return [...merged, ...source.cards.filter((c) => !seen.has(c.externalAccountId))]
  }, [source, external.data])

  const accountName = (externalAccountId: string) => cards.find((c) => c.externalAccountId === externalAccountId)?.externalName ?? ''
  const lastRun = runs[0]

  const runSync = () => {
    if (!source || !accessUrl) {
      return
    }
    sync.mutate({ sourceId: source.id, accessUrl, startDate }, {
      onSuccess: (result) => {
        setStartDate(defaultStartDate({ ...source, lastSyncedAt: result.run.finishedAt || source.lastSyncedAt }))
        if (result.run.status === 'completed') {
          toast.success(t('imports.simplefin.sync.done_toast', { imported: result.run.importedCount, matched: result.run.matchedCount }))
        }
      },
      onError: (err) => toast.error(apiErrorMessage(err)),
    })
  }

  const forget = async () => {
    await forgetThisDevice()
    setAccessUrl(null)
    refreshKey()
  }

  const body = () => {
    if (sourcesPending || keyState.status === 'loading') {
      return <p className="text-sm text-muted-foreground">{t('common.loading')}</p>
    }
    if (!source || reconnecting) {
      return (
        <SimpleFINConnect
          keyState={keyState} reconnect={reconnecting}
          onConnected={(url) => { setAccessUrl(url); setReconnecting(false); refreshKey() }}
        />
      )
    }
    if (keyState.status === 'locked') {
      return <SimpleFINUnlock wrapped={keyState.wrapped} onUnlocked={refreshKey} onReconnect={() => setReconnecting(true)} />
    }
    if (keyState.status === 'none') {
      // a source exists but its key was deleted server-side (never happens through the UI); reconnect
      return <SimpleFINConnect keyState={keyState} reconnect onConnected={(url) => { setAccessUrl(url); refreshKey() }} />
    }
    return (
      <>
        <div className="flex flex-col gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
          <div className="flex items-center justify-between gap-2">
            <span className="font-medium">{source.name}</span>
            <span className="text-xs text-muted-foreground">
              {source.lastSyncedAt ? t('imports.simplefin.sync.last_synced', { date: formatDateTime(parseDateTime(source.lastSyncedAt)) }) : t('imports.simplefin.sync.never')}
            </span>
          </div>
          <label className="flex items-center gap-2 text-xs text-muted-foreground" htmlFor="simplefin-start">
            {t('imports.simplefin.sync.from')}
            <input id="simplefin-start" type="date" value={startDate} onChange={(e) => setStartDate(e.target.value)} className="rounded border bg-background px-2 py-1 text-sm text-foreground" />
          </label>
          <Button type="button" className="w-full sm:w-auto" disabled={sync.isPending || !accessUrl} onClick={runSync}>
            {sync.isPending ? t('imports.simplefin.sync.running') : t('imports.simplefin.sync.button')}
          </Button>
        </div>
        {lastRun ? <ImportRunSummary run={lastRun} accountName={accountName} /> : null}
        {external.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(external.error)}</p> : null}
        <ImportCards source={source} cards={cards} variant="account" />
        <div className="flex flex-wrap gap-2 pt-2">
          <Button type="button" variant="secondary" size="sm" onClick={() => setReconnecting(true)}>{t('imports.simplefin.unlock.reconnect')}</Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => void forget()}>{t('imports.simplefin.forget_device')}</Button>
        </div>
      </>
    )
  }

  return (
    <SettingsShell title={t('imports.simplefin.title')} backTo={RouterPage.SETTINGS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-3">
        <InfoBox>{t('imports.simplefin.intro')}</InfoBox>
        {body()}
      </div>
    </SettingsShell>
  )
}
```

`accounts` from `useAccounts` is unused above except to keep the accounts cache warm for `ImportCards` — drop the import if oxlint flags it (ImportCards fetches its own).

Routing/menu:
- `web/src/app/router-pages.ts` after `SETTINGS_APPLE_WALLET`: `SETTINGS_SIMPLEFIN: '/settings/simplefin',`
- `web/src/app/routes.tsx`: import `SimpleFINPage` next to `AppleWalletPage`; route `{ path: '/settings/simplefin', element: <SimpleFINPage /> },` after the apple-wallet route.
- `web/src/features/settings/SettingsPage.tsx` — in the Data `MenuGroup`, after the Apple Wallet row: `<MenuRow label={t('imports.simplefin.menu_item')} to={RouterPage.SETTINGS_SIMPLEFIN} />`.

- [ ] **Step 6: Locale keys (all 11 catalogues; translate the values, do not copy English)**

`locales/en.json` under `imports`, new object `simplefin` (place after `apple_wallet`) and, under `imports`, a new `runs` object whose `status`/`counts` keys `ImportRunSummary` reads (Task 13 adds the rest of `imports.runs.*`):

```json
"simplefin": {
  "menu_item": "SimpleFIN",
  "title": "SimpleFIN",
  "intro": "Connect a bank through SimpleFIN Bridge. Your bridge credentials are encrypted on this device with a passphrase only you know; the server stores them encrypted and only sees them for the duration of a sync.",
  "forget_device": "Forget this device",
  "connect": {
    "intro": "Paste the setup token from your SimpleFIN Bridge account and choose a passphrase. The passphrase is never sent to the server — if you forget it, you will need to reconnect with a new setup token.",
    "reconnect_intro": "Paste a new setup token from SimpleFIN Bridge. The previous connection will be replaced; your account mappings are kept.",
    "token_label": "Setup token",
    "token_placeholder": "aHR0cHM6Ly9…",
    "token_help": "SimpleFIN Bridge → New connection → copy the setup token. A token can be used once.",
    "forgot_passphrase": "I forgot my passphrase — set a new one",
    "passphrase_new": "Passphrase",
    "passphrase_existing": "Your passphrase",
    "passphrase_confirm": "Repeat passphrase",
    "passphrase_help": "At least 8 characters. Not recoverable: nobody, including us, can restore it.",
    "submit": "Connect",
    "submit_reconnect": "Reconnect",
    "errors": {
      "token_required": "Paste the setup token first.",
      "passphrase_short": "The passphrase must be at least 8 characters.",
      "passphrase_mismatch": "The passphrases do not match."
    }
  },
  "unlock": {
    "intro": "Enter your passphrase to use SimpleFIN on this device.",
    "passphrase": "Passphrase",
    "submit": "Unlock",
    "reconnect": "Reconnect",
    "wrong_passphrase": "Wrong passphrase.",
    "forgot_help": "Forgot it? Reconnect with a new setup token and a new passphrase."
  },
  "accounts": {
    "header": "Accounts",
    "empty": "No accounts reported by the bridge yet.",
    "state": { "mapped": "Mapped", "ignored": "Ignored", "unmapped": "Unmapped · {count} queued" },
    "taps": "{count} transaction | {count} transactions",
    "last_seen": "Last seen {date}",
    "map": "Map to account",
    "map_instead": "Map instead",
    "ignore": "Ignore",
    "unlink": "Unmap",
    "unlink_modal": { "title": "Unmap {card}?", "question": "New transactions from this account will wait for review until you map it again." },
    "map_modal": { "header": "Map {card}", "account": "Account", "submit": "Map" },
    "mapped_toast": "{imported} imported, {matched} matched, {skipped} skipped"
  },
  "sync": {
    "button": "Sync now",
    "running": "Syncing…",
    "from": "From",
    "last_synced": "Last synced {date}",
    "never": "Never synced",
    "done_toast": "{imported} imported, {matched} matched"
  }
},
"runs": {
  "status": { "running": "Running", "completed": "Completed", "partial": "Completed with errors", "failed": "Failed" },
  "counts": "{imported} imported · {matched} matched · {updated} amounts updated · {queued} queued · {skipped} skipped · {failed} failed"
}
```

`accounts.taps` must exist for key parity even though the account variant never renders it; every key under `simplefin.accounts` mirrors `apple_wallet.cards` one-to-one so the `${ns}.` lookup in Step 3 always resolves. Run `go test ./internal/test/i18ntest/` after editing: it enforces key parity across the 11 catalogues and `{var}` parity per key.

- [ ] **Step 7: Page test**

`web/src/features/imports/SimpleFINPage.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { IDBFactory } from 'fake-indexeddb'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { createKey, encryptCredential } from '@/lib/importCrypto'
import { SimpleFINPage } from './SimpleFINPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const ACCESS_URL = 'https://u:p@bridge.example/simplefin'
const source = (over: Record<string, unknown> = {}) => ({
  id: 's2', provider: 'simplefin', name: 'SimpleFIN', status: 'active', createdAt: '2026-09-01 00:00:00',
  lastSyncedAt: '', credentialCiphertext: '', cards: [], ...over,
})
const run = (over: Record<string, unknown> = {}) => ({
  id: 'r1', sourceId: 's2', provider: 'simplefin', status: 'completed', trigger: 'manual',
  importedCount: 2, matchedCount: 1, amountsUpdatedCount: 0, queuedCount: 0, skippedCount: 0, failedCount: 0,
  errors: [], startedAt: '2026-09-07 10:00:00', finishedAt: '2026-09-07 10:00:02', ...over,
})

function renderPage(data: Record<string, unknown>) {
  server.use(...coreHandlers(data))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/simplefin', element: <SimpleFINPage /> }], { initialEntries: ['/settings/simplefin'] })
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  globalThis.indexedDB = new IDBFactory()
})

it('first connect: creates a key, claims the token, and stores only ciphertext', async () => {
  const posted: Record<string, unknown> = {}
  server.use(
    http.post('*/api/v1/import/set-credential-key', async ({ request }) => {
      posted.key = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { ...(posted.key as object), updatedAt: '2026-09-07 10:00:00' } })
    }),
    http.post('*/api/v1/import/claim-setup-token', async ({ request }) => {
      posted.claim = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { accessUrl: ACCESS_URL } })
    }),
    http.post('*/api/v1/import/create-source', async ({ request }) => {
      posted.source = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: source({ credentialCiphertext: (posted.source as { credentialCiphertext: string }).credentialCiphertext }) } })
    }),
    http.post('*/api/v1/import/list-external-accounts', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })),
  )
  renderPage({ importSources: [] })
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Setup token'), 'tok-1')
  await user.type(screen.getByLabelText('Passphrase'), 'correct horse')
  await user.type(screen.getByPlaceholderText('Repeat passphrase'), 'correct horse')
  await user.click(screen.getByRole('button', { name: 'Connect' }))
  await waitFor(() => expect(posted.source).toBeDefined(), { timeout: 15_000 })
  expect(posted.claim).toEqual({ setupToken: 'tok-1' })
  expect((posted.key as { kdf: string }).kdf).toContain('PBKDF2-SHA256')
  const src = posted.source as Record<string, string>
  expect(src.provider).toBe('simplefin')
  expect(src.credentialCiphertext).toMatch(/^v1:/)
  expect(JSON.stringify(posted)).not.toContain(ACCESS_URL)
  expect(await screen.findByRole('button', { name: 'Sync now' })).toBeInTheDocument()
}, 20_000)

it('locked device: wrong passphrase is rejected, right one unlocks and lists bridge accounts', async () => {
  const wrapped = await createKey('pw')
  const ciphertext = await encryptCredential(ACCESS_URL)
  globalThis.indexedDB = new IDBFactory()  // "another device": server has the key, this one does not
  let listBody: unknown
  server.use(http.post('*/api/v1/import/list-external-accounts', async ({ request }) => {
    listBody = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { items: [
      { externalAccountId: 'ACT-CHK', externalName: 'Checking', externalCurrency: 'USD', balance: '100', orgName: 'Bank', state: 'unmapped', accountId: '' },
    ] } })
  }))
  renderPage({ importSources: [source({ credentialCiphertext: ciphertext })], importCredentialKey: { ...wrapped, updatedAt: '2026-09-07 10:00:00' } })
  const user = userEvent.setup()
  await user.type(await screen.findByPlaceholderText('Passphrase'), 'nope')
  await user.click(screen.getByRole('button', { name: 'Unlock' }))
  expect(await screen.findByRole('alert', {}, { timeout: 15_000 })).toHaveTextContent('Wrong passphrase.')
  await user.clear(screen.getByPlaceholderText('Passphrase'))
  await user.type(screen.getByPlaceholderText('Passphrase'), 'pw')
  await user.click(screen.getByRole('button', { name: 'Unlock' }))
  expect(await screen.findByText('Checking', {}, { timeout: 15_000 })).toBeInTheDocument()
  expect(listBody).toEqual({ sourceId: 's2', accessUrl: ACCESS_URL })
}, 40_000)

it('unlocked device: Sync now posts the decrypted access URL and shows the run summary', async () => {
  const wrapped = await createKey('pw')
  const ciphertext = await encryptCredential(ACCESS_URL)
  let syncBody: unknown
  server.use(
    http.post('*/api/v1/import/list-external-accounts', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })),
    http.post('*/api/v1/import/sync-source', async ({ request }) => {
      syncBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { run: run({ status: 'partial', failedCount: 1, errors: [{ externalAccountId: 'ACT-SAV', message: 'unsupported currency' }] }), accounts: [] } })
    }),
  )
  renderPage({
    importSources: [source({ credentialCiphertext: ciphertext, lastSyncedAt: '2026-09-05 08:00:00' })],
    importCredentialKey: { ...wrapped, updatedAt: '2026-09-07 10:00:00' },
    importRuns: [],
  })
  const user = userEvent.setup()
  expect(await screen.findByText('Last synced 2026-09-05 08:00')).toBeInTheDocument()  // adapt to formatDateTime's actual output
  await user.click(await screen.findByRole('button', { name: 'Sync now' }))
  await waitFor(() => expect(syncBody).toEqual({ sourceId: 's2', accessUrl: ACCESS_URL, startDate: '2026-09-02' }))
  // the run list is invalidated by the mutation; return the new run from the list so the summary shows it
})
```

For the third test, register `http.get('*/api/v1/import/get-run-list', …)` returning `[run({ status: 'partial', … })]` AFTER the sync fires (or unconditionally — the summary shows the latest run either way) and assert `await screen.findByText('Completed with errors')` and `screen.getByText(/ACT-SAV: unsupported currency/)`. Match the `Last synced` string to what `formatDateTime` actually renders (check `web/src/lib/datetime.ts`); the point of the assertion is the day and the "Last synced" label.

- [ ] **Step 8: Run**

Run: `pnpm --dir web exec vitest run src/features/imports 2>&1 | tail -12 && pnpm --dir web exec tsc -b && pnpm --dir web lint 2>&1 | tail -3 && cd <repo root> && go test ./internal/test/i18ntest/`
Expected: all green (PBKDF2 at 600k iterations takes ~1–3 s per derivation in Node, hence the raised timeouts).

- [ ] **Step 9: Commit**

```bash
git add web/src locales
git commit -m "feat(web): Settings → SimpleFIN page with connect, unlock, accounts and Sync"
```

---

### Task 13: "Sync all" on the Data page, run list and run detail pages

**Files:**
- Create: `web/src/features/imports/ImportRunListPage.tsx`, `web/src/features/imports/ImportRunPage.tsx`, `web/src/features/imports/ImportRunPage.test.tsx`
- Modify: `web/src/features/imports/ImportsDataPage.tsx`, `web/src/features/imports/ImportsDataPage.test.tsx`, `web/src/features/imports/SimpleFINPage.tsx` (history link), `web/src/features/imports/ImportRunSummary.tsx` (optional link wrapper), `web/src/app/router-pages.ts`, `web/src/app/routes.tsx`, all 11 `locales/<lang>.json`

**Interfaces:**
- Consumes: Task 11 hooks `useImportRuns`, `useImportRun`, `useSyncImportSource`, `useImportSources`; Task 12 `useImportKey`, `ImportRunSummary`, `decryptCredential`; DTOs `ImportRunDto`, `ImportRunLinkDto`.
- Produces: `RouterPage.IMPORT_RUNS = '/imports/runs'`, `RouterPage.IMPORT_RUN = '/imports/runs/:id'` (build with `RouterPage.IMPORT_RUN.replace(':id', id)`); locale keys `imports.runs.*` (Step 4); `imports.data_page.sync_all`, `imports.data_page.history`.

- [ ] **Step 1: Routes**

`web/src/app/router-pages.ts` after `IMPORT_QUEUE`:

```ts
  IMPORT_RUNS: '/imports/runs',
  IMPORT_RUN: '/imports/runs/:id',
```

`web/src/app/routes.tsx` — import `ImportRunListPage`, `ImportRunPage`; routes after the queue route:

```tsx
                { path: '/imports/runs', element: <ImportRunListPage /> },
                { path: '/imports/runs/:id', element: <ImportRunPage /> },
```

- [ ] **Step 2: Run list + run detail pages**

`web/src/features/imports/ImportRunSummary.tsx` — add an optional `to?: string` prop; when set, wrap the card in `<Link to={to} className="block rounded-lg hover:bg-econumo-hover">` (`Link` from `react-router`), otherwise render as before.

`web/src/features/imports/ImportRunListPage.tsx`:

```tsx
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { ImportRunSummary } from './ImportRunSummary'
import { useImportRuns } from './queries'

export function ImportRunListPage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const sourceId = params.get('sourceId') ?? ''
  const runs = useImportRuns(sourceId)
  return (
    <SettingsShell title={t('imports.runs.header')} backTo={RouterPage.SETTINGS_DATA}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {runs.isPending ? <p className="text-sm text-muted-foreground">{t('common.loading')}</p> : null}
        {runs.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(runs.error)}</p> : null}
        {runs.data?.length === 0 ? <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.runs.empty')}</p> : null}
        {runs.data?.map((run) => <ImportRunSummary key={run.id} run={run} to={RouterPage.IMPORT_RUN.replace(':id', run.id)} />)}
      </div>
    </SettingsShell>
  )
}
```

`web/src/features/imports/ImportRunPage.tsx` (read-only in this stage — see "Deferred" at the end of the plan):

```tsx
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import type { ImportRunLinkDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { dayKey, formatDayHeading } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { ImportRunSummary } from './ImportRunSummary'
import { useImportRun } from './queries'

// A linked row whose transaction is gone is a tombstone: the import happened,
// the user deleted the result afterwards. It stays listed so the history is
// honest, and so a future "allow re-import" action has something to act on.
function rowStatus(link: ImportRunLinkDto): 'imported' | 'deleted' | 'queued' | 'skipped' {
  if (link.status === 'linked') {
    return link.transactionId ? 'imported' : 'deleted'
  }
  return link.status === 'queued' ? 'queued' : 'skipped'
}

export function ImportRunPage() {
  const { t, i18n } = useTranslation()
  const { id = '' } = useParams()
  const detail = useImportRun(id)
  return (
    <SettingsShell title={t('imports.runs.detail_header')} backTo={RouterPage.IMPORT_RUNS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {detail.isPending ? <p className="text-sm text-muted-foreground">{t('common.loading')}</p> : null}
        {detail.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(detail.error)}</p> : null}
        {detail.data ? (
          <>
            <ImportRunSummary run={detail.data.item} />
            <p className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t('imports.runs.rows_header')}</p>
            {detail.data.links.length === 0 ? (
              <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.runs.rows_empty')}</p>
            ) : detail.data.links.map((link) => {
              const status = rowStatus(link)
              return (
                <div key={link.id} className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
                  <div className="min-w-0">
                    <div className={status === 'deleted' ? 'truncate line-through text-muted-foreground' : 'truncate'}>{link.externalPayee || link.externalTransactionId}</div>
                    <div className="text-xs text-muted-foreground">
                      {link.externalAccountId} · {formatDayHeading(dayKey(link.externalPostedAt), i18n.language)} · {t(`imports.runs.row_status.${status}`)}
                    </div>
                  </div>
                  <div className="shrink-0 tabular-nums">{moneyFormat(link.externalAmount, null, { showCurrency: false, useNativePrecision: false })} {link.externalCurrency}</div>
                </div>
              )
            })}
          </>
        ) : null}
      </div>
    </SettingsShell>
  )
}
```

`web/src/features/imports/SimpleFINPage.tsx` — under the run summary (Task 12's `{lastRun ? … : null}`), add the history link:

```tsx
        {source ? (
          <Link to={`${RouterPage.IMPORT_RUNS}?sourceId=${source.id}`} className="px-1 text-sm text-primary hover:underline">{t('imports.runs.history_link')}</Link>
        ) : null}
```

and pass `to={RouterPage.IMPORT_RUN.replace(':id', lastRun.id)}` to that `ImportRunSummary`.

- [ ] **Step 3: "Sync all" on the Data page**

`web/src/features/imports/ImportsDataPage.tsx` — add, after the CSV rows:

```tsx
        {pullSources.length > 0 ? (
          <ActionRow label={syncing ? t('imports.simplefin.sync.running') : t('imports.data_page.sync_all')} onClick={() => void syncAll()} />
        ) : null}
        <LinkRow label={t('imports.data_page.history')} to={RouterPage.IMPORT_RUNS} />
```

with the supporting code inside the component:

```tsx
  const navigate = useNavigate()
  const { data: sources = [] } = useImportSources()
  const { state: keyState } = useImportKey()
  const sync = useSyncImportSource()
  const [syncing, setSyncing] = useState(false)
  const pullSources = sources.filter((s) => s.provider === 'simplefin')

  // One source at a time: each sync holds the server's single SQLite writer
  // for a moment, and the summary toast wants the totals.
  const syncAll = async () => {
    if (syncing) {
      return
    }
    if (keyState.status !== 'unlocked') {
      navigate(RouterPage.SETTINGS_SIMPLEFIN)
      return
    }
    setSyncing(true)
    let imported = 0
    let matched = 0
    let failed = 0
    try {
      for (const source of pullSources) {
        try {
          const accessUrl = await decryptCredential(source.credentialCiphertext)
          const result = await sync.mutateAsync({ sourceId: source.id, accessUrl, startDate: syncStartDate(source) })
          imported += result.run.importedCount
          matched += result.run.matchedCount
          if (result.run.status !== 'completed') {
            failed += 1
          }
        } catch (err) {
          failed += 1
          toast.error(`${source.name}: ${apiErrorMessage(err)}`)
        }
      }
      toast[failed ? 'error' : 'success'](t('imports.data_page.sync_all_done', { imported, matched, failed }))
    } finally {
      setSyncing(false)
    }
  }
```

`LinkRow` is `ActionRow`'s sibling rendering a `<Link to>` with the same classes. Move Task 12's `defaultStartDate` out of `SimpleFINPage.tsx` into `web/src/features/imports/syncWindow.ts` as `export function syncStartDate(source: ImportSourceDto): string` (same body, same constants) and import it from both pages. `apiErrorMessage(err)` on a non-Axios error (e.g. `KeyLockedError`) — check `web/src/lib/apiError.ts` falls back to a generic message for unknown errors; if it returns `''`, use `err instanceof Error ? err.message : String(err)` first.

- [ ] **Step 4: Locale keys (all 11 catalogues)**

`locales/en.json` — extend `imports.runs` (Task 12 created `status` and `counts`) and `imports.data_page`:

```json
"data_page": {
  "menu_item": "Import & export",
  "header": "Import & export",
  "sync_all": "Sync bank connections",
  "history": "Import history",
  "sync_all_done": "{imported} imported, {matched} matched, {failed} failed"
},
"runs": {
  "header": "Import history",
  "detail_header": "Import run",
  "history_link": "Import history",
  "empty": "No imports yet.",
  "rows_header": "Transactions",
  "rows_empty": "This run touched no transactions.",
  "status": { "running": "Running", "completed": "Completed", "partial": "Completed with errors", "failed": "Failed" },
  "counts": "{imported} imported · {matched} matched · {updated} amounts updated · {queued} queued · {skipped} skipped · {failed} failed",
  "row_status": { "imported": "Imported", "deleted": "Deleted since", "queued": "Waiting for review", "skipped": "Skipped" }
}
```

- [ ] **Step 5: Tests**

`web/src/features/imports/ImportRunPage.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportRunListPage } from './ImportRunListPage'
import { ImportRunPage } from './ImportRunPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const run = {
  id: 'r1', sourceId: 's2', provider: 'simplefin', status: 'partial', trigger: 'manual',
  importedCount: 1, matchedCount: 0, amountsUpdatedCount: 0, queuedCount: 1, skippedCount: 0, failedCount: 1,
  errors: [{ externalAccountId: 'ACT-SAV', message: 'unsupported currency' }], startedAt: '2026-09-07 10:00:00', finishedAt: '2026-09-07 10:00:02',
}
const links = [
  { id: 'l1', externalAccountId: 'ACT-CHK', externalTransactionId: 'chk-1', transactionId: 't1', status: 'linked', externalPayee: 'Blue Bottle', externalAmount: '-12.5', externalCurrency: 'USD', externalPostedAt: '2026-09-06 09:00:00' },
  { id: 'l2', externalAccountId: 'ACT-CHK', externalTransactionId: 'chk-2', transactionId: '', status: 'linked', externalPayee: 'Payroll', externalAmount: '2500', externalCurrency: 'USD', externalPostedAt: '2026-09-05 09:00:00' },
  { id: 'l3', externalAccountId: 'ACT-SAV', externalTransactionId: 'sav-1', transactionId: '', status: 'queued', externalPayee: 'Transfer', externalAmount: '-100', externalCurrency: 'USD', externalPostedAt: '2026-09-04 09:00:00' },
]

function renderAt(path: string) {
  server.use(
    ...coreHandlers({ importRuns: [run] }),
    http.get('*/api/v1/import/get-run', ({ request }) =>
      new URL(request.url).searchParams.get('id') === 'r1'
        ? HttpResponse.json({ success: true, message: '', data: { item: run, links } })
        : HttpResponse.json({ success: false, message: 'Run not found', code: 400, errors: {} }, { status: 400 })),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/imports/runs', element: <ImportRunListPage /> }, { path: '/imports/runs/:id', element: <ImportRunPage /> }],
    { initialEntries: [path] },
  )
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('lists runs with status, counts and per-account errors, linking to the run', async () => {
  renderAt('/imports/runs')
  expect(await screen.findByText('Completed with errors')).toBeInTheDocument()
  expect(screen.getByText(/1 imported · 0 matched/)).toBeInTheDocument()
  expect(screen.getByText(/ACT-SAV: unsupported currency/)).toBeInTheDocument()
  expect(screen.getByRole('link')).toHaveAttribute('href', '/imports/runs/r1')
})

it('run detail shows imported, tombstone and queued rows read-only', async () => {
  renderAt('/imports/runs/r1')
  expect(await screen.findByText('Blue Bottle')).toBeInTheDocument()
  expect(screen.getByText(/ACT-CHK · .* · Imported$/)).toBeInTheDocument()
  expect(screen.getByText('Payroll')).toHaveClass('line-through')
  expect(screen.getByText(/Deleted since$/)).toBeInTheDocument()
  expect(screen.getByText(/Waiting for review$/)).toBeInTheDocument()
  expect(screen.getByText('-12.5 USD')).toBeInTheDocument()
  expect(screen.queryByRole('button')).toBeNull()
})

it('an unknown run id shows the server error', async () => {
  renderAt('/imports/runs/nope')
  expect(await screen.findByRole('alert')).toHaveTextContent('Run not found')
})
```

`web/src/features/imports/ImportsDataPage.test.tsx` — add:

```tsx
it('Sync all is hidden without a pull source and routes to SimpleFIN settings while the device is locked', async () => {
  renderPage()
  expect(await screen.findByText('Import & export')).toBeInTheDocument()
  expect(screen.queryByText('Sync bank connections')).toBeNull()
})
```

and a second test that registers `coreHandlers({ importSources: [simplefinSource], importCredentialKey: wrapped })` (with no local key → locked), clicks "Sync bank connections", and asserts the router landed on a `/settings/simplefin` stub route (`renderPage` needs to accept the extra route + handler data; extend it the way `ImportQueuePage.test.tsx`'s `renderPage` does). The unlocked "actually syncs" path is covered by the SimpleFINPage test in Task 12; do not duplicate the PBKDF2 setup here.

- [ ] **Step 6: Run**

Run: `pnpm --dir web exec vitest run src/features/imports 2>&1 | tail -12 && pnpm --dir web exec tsc -b && pnpm --dir web lint 2>&1 | tail -3 && go test ./internal/test/i18ntest/`
Expected: green.

- [ ] **Step 7: Commit**

```bash
git add web/src locales
git commit -m "feat(web): Sync all, import run history and run detail pages"
```

---

### Task 14: Docs, regression plan, final gates

**Files:**
- Modify: `docs/regression-test-plan.md`, `CLAUDE.md`, `.env.example` (verify Task 7 landed), `docs/superpowers/specs/2026-08-15-transaction-import-design.md` (status note only)

- [ ] **Step 1: Regression test plan**

`docs/regression-test-plan.md` — insert a new section after §5a "Imports — Apple Wallet" (before §6):

```markdown
### 5b. Imports — SimpleFIN 📱

Preconditions: a SimpleFIN Bridge account with at least one linked bank and a fresh setup token.

- [ ] Settings → SimpleFIN on a device with no key: the connect form asks for a setup token and a passphrase (+ repeat); a passphrase under 8 characters or a mismatch is rejected inline before any request. 📱
- [ ] Connect: after "Connect" the page shows the bridge's accounts as Unmapped rows, the source row reads "Never synced", and the network log shows `create-source` carrying `credentialCiphertext` starting `v1:` — the access URL appears in no request other than `list-external-accounts`/`sync-source` bodies.
- [ ] A used/invalid setup token shows the server's error inline; nothing is created (Settings → SimpleFIN still shows the connect form after reload).
- [ ] Second device (or same browser after "Forget this device"): the page shows the unlock prompt; a wrong passphrase reads "Wrong passphrase."; the right one lists the accounts. 📱
- [ ] "Forget this device" returns the page to the unlock prompt; "Reconnect" from the unlock prompt with "I forgot my passphrase" ticked accepts a new setup token + new passphrase and replaces the connection (account mappings kept).
- [ ] Map a bridge account to an owned account: "Sync now" imports its transactions; the toast reads "{n} imported, {m} matched"; the imported transactions carry the Imported badge and their provenance sheet names the SimpleFIN source.
- [ ] Sync with the "From" date after today or more than the server's window is rejected inline (`import.sync_range_invalid`); "From" defaults to 3 days before the last sync (30 days back before the first).
- [ ] Sync twice with the same window: the second run reports 0 imported, 0 matched (exact duplicates are skipped) and the last-synced timestamp advances.
- [ ] A pending transaction whose amount changed at the bridge between syncs updates the imported transaction's amount ("amounts updated" count > 0) rather than creating a second one.
- [ ] A hand-entered transaction matching an incoming bridge row within the tip tolerance is adopted (matched count), not duplicated.
- [ ] Unmapped bridge account with transactions: sync queues them (queue page shows them under the account name with "Card not mapped" reason); mapping the account replays the queue.
- [ ] Per-account failure (e.g. a currency without a rate): the run shows "Completed with errors", the failing account's error is listed under the run summary, other accounts' rows still import.
- [ ] Bridge unreachable / credentials revoked: "Sync now" shows the server's error inline, the run list shows a Failed run, and the failed state stays on the page until the next successful sync.
- [ ] Settings → Data: "Sync bank connections" is absent without a SimpleFIN source; with one and a locked device it navigates to Settings → SimpleFIN; unlocked it syncs every pull source and toasts the totals. 📱
- [ ] Settings → Data → Import history lists runs newest first with status, counts and errors; a run opens its detail; a transaction deleted after import shows struck-through with "Deleted since"; queued rows read "Waiting for review". Rows have no actions in this version. 📱
- [ ] Rate limits: the 6th `claim-setup-token` within 15 minutes and the 11th `sync-source` return 429 with the standard envelope.
- [ ] Ingest-scoped PATs get 401 on every SimpleFIN endpoint; a read-only (trial-ended) user gets 402 on `claim-setup-token`, `set-credential-key`, `sync-source`.
- [ ] Apple Wallet regression: §5a still passes unchanged (the `cards` list, queue, and provenance UI share code with the SimpleFIN account list).
```

Renumber nothing else (sections are lettered).

- [ ] **Step 2: CLAUDE.md**

Update the `imports` paragraph in "Feature packages (vertical slices)": stage 3 (SimpleFIN pull) is in; the package now has `internal/imports/simplefin` (the bridge client + `Provider`), a `Provider` registry on the service, and 21 routes under `/api/v1/import/` — append `claim-setup-token`, `get-credential-key`, `set-credential-key`, `list-external-accounts`, `sync-source`, `get-run-list`, `get-run` to the route list. Add one bullet to "Notable behaviours" after the Apple Wallet one:

```markdown
- **Transaction import (SimpleFIN pull)**: the client claims the one-shot setup token
  through `claim-setup-token` (the server returns the access URL and keeps nothing), encrypts
  it in the browser with a passphrase-wrapped data key (`web/src/lib/importCrypto.ts`; the
  wrapped key lives in `import_credential_keys`, one row per user; the unwrapped key stays
  non-extractable in IndexedDB), and stores only the ciphertext on the source. Every
  `list-external-accounts`/`sync-source` call carries the plaintext access URL in the body —
  "at rest zero-knowledge, in flight trusted" — and it is never persisted, logged, or
  formatted into an error. A sync is one `import_runs` row; per-account failures leave the
  run `partial`, a bridge failure leaves it `failed`; `last_synced_at` moves only on
  `completed`/`partial`. Sync is manual (a button); sync-on-open is a follow-up.
```

Add `ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN` (default `5`; every attempt counts) and `ECONUMO_RATE_LIMIT_SYNC` (default `10`; every attempt counts) to the rate-limit bullet in "Configuration" (Task 7 wrote these lines; verify they read as above). Confirm `.env.example` carries both.

- [ ] **Step 3: Spec status note**

`docs/superpowers/specs/2026-08-15-transaction-import-design.md` — in the staging section, mark stage 3 as implemented by this plan with the two explicit deferrals: run-row actions (delete transaction / allow re-import / delete both) and sync-on-open.

- [ ] **Step 4: Final gates**

```bash
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
make go-test
pnpm --dir web lint
pnpm --dir web test
pnpm --dir web exec tsc -b
```

Expected: all green except the pre-existing `web/src/api/transaction.test.ts` Blob failure (not ours; leave it). `make go-test` includes apiparity + mcpparity goldens, i18n guards, archtest, and the coverage gate.

- [ ] **Step 5: Commit**

```bash
git add docs/regression-test-plan.md CLAUDE.md .env.example docs/superpowers/specs/2026-08-15-transaction-import-design.md
git commit -m "docs: SimpleFIN import — regression plan, CLAUDE.md, spec status"
```

---

## Deferred (not in this stage — spec items with no task above)

- **Run-row actions** (spec Part 9, run detail): "delete transaction", "allow re-import", "delete both". They need three endpoints not in Task 8; the run detail page is read-only here and tombstones are displayed, not acted on.
- **Sync-on-open** (spec Part 9): automatic sync when the app opens with an unlocked key. Requires the sync `trigger` value `auto`, a throttle, and a background-error surface; `trigger` is already on the wire so the analytics property lands unchanged.
- **Passphrase change UI**: `changePassphrase` exists in `importCrypto.ts` (Task 10) and `set-credential-key` accepts a re-wrapped key, but no page exposes it yet.
