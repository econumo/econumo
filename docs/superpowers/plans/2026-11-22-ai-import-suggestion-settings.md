# AI Import Suggestion Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Two AI switches (Suggest rules, Suggest category and payee) and a new Inbox section
"Imported transactions" listing per-transaction category/payee suggestions with Fill in and
dismiss.

**Architecture:** The existing AI import job (`internal/imports/suggest_job.go`) also writes
rows of a new table `import_transaction_suggestions` from each row's own validated AI answer.
Three new import routes read, fill and dismiss them; one new user route stores the two
switches as user options. The SPA adds the switches to `AiDialog` and a Due-style section to
the Inbox.

**Tech Stack:** Go (stdlib HTTP, sqlc, SQLite + PostgreSQL), React 19 + TanStack Query +
vitest.

**Spec:** `docs/superpowers/specs/2026-11-22-ai-import-suggestion-settings-design.md`
(builds on `docs/superpowers/specs/2026-11-11-ai-import-rule-suggestions-design.md`). Read both.

## Global Constraints

- Go is at `/usr/local/go/bin` (not on PATH): `export PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1`.
- `.sql` query files ASCII-only; a `;` ends the query on the same line; run `sqlc generate`
  (config `internal/infra/storage/sqlc/sqlc.yaml`) after editing queries.
- New migration version: `20261122000000` on both engines (`internal/infra/storage/migrations/{sqlite,pgsql}`).
- User options: `ai_suggest_rules`, `ai_suggest_fill`, values `"1"`/`"0"`, absent or empty = on.
- Route paths exactly: `POST /api/v1/user/update-ai-suggestions` (body `{suggestRules: bool, suggestFill: bool}`, both required),
  `GET /api/v1/import/get-transaction-suggestion-list`, `POST /api/v1/import/fill-transaction-suggestion {id}`,
  `POST /api/v1/import/dismiss-transaction-suggestion {id}`.
- Statuses: `open`, `filled`. Dismiss deletes.
- No AI prompt, schema or data-allowlist change. Rule-suggestion tests must keep passing unchanged
  (except where a test asserts something this spec changes, which is none expected).
- Every new route: apiparity scenario + golden (`UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect diff);
  swag block on the handler; `make swagger` regenerates `internal/web/apidoc/docs`.
- Comments only for non-obvious why (CLAUDE.md "Comments").
- i18n: every key in all 11 `locales/*.json`; register of each catalogue (de Sie, nl u, fr vous, pt European).
- Frontend: METRICS keys `app`-prefixed camelCase, fired at success; write controls read `useOnline`.
- Local tests: only the touched packages (`go test ./internal/imports/... ./internal/user/... ./internal/server/... ./internal/test/apiparity/...`)
  and the touched vitest files; full suites run in CI.

## Review Focus

1. A transaction suggestion whose category was archived/deleted, or whose transaction changed type, after the job ran: the list must hide it (not crash, not offer it), and Fill in must fill only what is still valid.
2. Fill in racing a hand edit (user set the category between list and tap): never overwrite; `updated` 0 and the row disappears.
3. A shared account: a partner's tap creates a transaction suggestion only for the owner; a non-owner calling fill/dismiss with that id gets not-found.
4. Repeated Fill in (lost answer, Idempotency-Key replay or a second tap): no double write, client treats not-found as done.
5. Rules switch off with fill on: no rule suggestion inserted/joined, but transaction suggestions still created; both off: no AI call and no rate-limit slot taken.

---

### Task 1: Settings backend (user options, route, owner facts)

**Files:**
- Modify: `internal/model/user.go` (option constants, getters/setters next to `AIEnabled`/`SetAI`)
- Modify: `internal/model/user_dto.go` (request/result next to `UpdateAIRequest`)
- Modify: `internal/user/profile.go` (service method), `internal/user/api/ai.go` + `routes.go` (handler + route)
- Modify: `internal/web/middleware/auth.go` (read-only allowlist entry)
- Modify: `internal/imports/ports.go` (`OwnerInfo` gains `SuggestRules, SuggestFill bool`), `internal/server/glue_imports.go` (`ImportsOwnerFacts.OwnerFacts` fills them)
- Modify: `internal/test/apiparity/catalogue.go` (scenario `update-ai-suggestions`), golden
- Test: `internal/model/user_test.go`, `internal/user/profile_test.go` (or the file holding `UpdateAI` tests), `internal/server/glue_imports_test.go`

**Interfaces:**
- Produces: `model.OptionAISuggestRules = "ai_suggest_rules"`, `model.OptionAISuggestFill = "ai_suggest_fill"`;
  `(*User).AISuggestRules() bool`, `(*User).AISuggestFill() bool`, `(*User).SetAISuggestions(rules, fill bool, newID func-or-id like SetAI, now time.Time)`;
  `model.UpdateAISuggestionsRequest{SuggestRules *bool \`json:"suggestRules"\`; SuggestFill *bool \`json:"suggestFill"\`}` with `Validate()` requiring both;
  `model.UpdateAISuggestionsResult{User}`; `(*user.Service).UpdateAISuggestions(ctx, userID, req)`;
  `imports.OwnerInfo{Language, Timezone string; Readonly, AIOff, SuggestRules, SuggestFill bool}`.

- [ ] **Step 1: Failing tests**: `TestUserAISuggestionsDefaultOn` (no option rows → both true; `"0"` → false; `""` → true); `TestUpdateAISuggestions` (writes both options, returns user whose options list carries them; missing field → validation error naming the field); `TestImportsOwnerFacts` extended to assert `SuggestRules`/`SuggestFill` follow the options.
- [ ] **Step 2: Run** `go test ./internal/model/ ./internal/user/... ./internal/server/ -run 'AISuggest|OwnerFacts'` — FAIL (undefined).
- [ ] **Step 3: Implement** as above. `SetAISuggestions` upserts the two option rows exactly as `SetAI` does for `ai`. No suggestion cleanup on turning a switch off (hidden, not deleted). Add `"/api/v1/user/update-ai-suggestions": true` to the read-only allowlist.
- [ ] **Step 4: apiparity**: add scenario `{Label: "update-ai-suggestions", Method: "POST", Path: "/api/v1/user/update-ai-suggestions", Auth: "owner", Body: {"suggestRules": false, "suggestFill": true}}`; `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`; inspect the new golden; `make swagger`.
- [ ] **Step 5: Run** the Step 2 command + `go test ./internal/test/apiparity/ ./internal/web/...` — PASS.
- [ ] **Step 6: Commit** `feat: Store the AI import suggestion switches`.

### Task 2: Storage for transaction suggestions

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261122000000.sql`, `.../pgsql/20261122000000.sql`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/imports.sql`, regenerate `gen/`
- Modify: `internal/model/imports.go` (or wherever `ImportSuggestion` lives): new entity
- Create: `internal/imports/repo/transaction_suggestion.go`; modify `internal/imports/repo/{repo,sqlite,pgsql}.go` (querier + adapters)
- Modify: `internal/imports/repository.go` (interface)
- Test: `internal/imports/repo/repo_integration_test.go` (or a new `transaction_suggestion_test.go` in that package)

**Interfaces:**
- Produces:
  ```go
  type ImportTransactionSuggestion struct {
      LinkID, UserID vo.Id
      CategoryID, PayeeID *vo.Id
      Type TransactionType
      Status string // ImportTransactionSuggestionStatusOpen = "open", ...Filled = "filled"
      CreatedAt, UpdatedAt time.Time
  }
  ```
  Repository methods:
  `InsertTransactionSuggestions(ctx, []model.ImportTransactionSuggestion) error` (ON CONFLICT (link_id) DO NOTHING),
  `ListOpenTransactionSuggestionsByUser(ctx, userID) ([]model.ImportTransactionSuggestion, error)`,
  `LockTransactionSuggestion(ctx, linkID) (*model.ImportTransactionSuggestion, error)` (row lock via no-op UPDATE like `LockImportSuggestion`, then read; not found → `errs` not-found as `LockSuggestion` does),
  `MarkTransactionSuggestionFilled(ctx, linkID, now) error`,
  `DeleteTransactionSuggestion(ctx, linkID) error`,
  `DeleteOpenTransactionSuggestionsByUser(ctx, userID) error`,
  `DeleteTransactionSuggestionsByLinks(ctx, linkIDs []vo.Id) error` (for opt-out undo),
  `FilledTransactionSuggestionLinks(ctx, linkIDs []vo.Id) (map[vo.Id]bool, error)` (built IN list like `ExistingLinkIDs`).

- [ ] **Step 1: Migration** (same shape both engines; pgsql uses TIMESTAMP like the 20261115000000 pgsql file):
  ```sql
  CREATE TABLE import_transaction_suggestions (
      link_id TEXT NOT NULL PRIMARY KEY,
      user_id TEXT NOT NULL,
      category_id TEXT DEFAULT NULL,
      payee_id TEXT DEFAULT NULL,
      transaction_type TEXT NOT NULL,
      status TEXT NOT NULL,
      created_at DATETIME NOT NULL,
      updated_at DATETIME NOT NULL,
      CONSTRAINT FK_import_transaction_suggestions_link FOREIGN KEY (link_id) REFERENCES import_transaction_links (id) ON DELETE CASCADE,
      CONSTRAINT FK_import_transaction_suggestions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
  );
  CREATE INDEX IDX_import_transaction_suggestions_user_status ON import_transaction_suggestions (user_id, status);
  ```
- [ ] **Step 2: Failing repo test** `TestTransactionSuggestionRepo`: insert two (second insert of same link is ignored, keeps first values), list open by user returns only that user's open rows, mark filled removes it from the open list and `FilledTransactionSuggestionLinks` reports it, delete by user removes only open rows, deleting the ledger link cascades. Runs on sqlite via `dbtest`.
- [ ] **Step 3: Queries + sqlc generate + repo methods** following the engine-adapter pattern of `internal/imports/repo/suggestion.go`.
- [ ] **Step 4: Run** `go test ./internal/imports/repo/ ./internal/infra/storage/...` — PASS. If the schema-equality/upgrade or enginecompare guards list tables, add the table there.
- [ ] **Step 5: Commit** `feat: Store per-transaction AI suggestions`.

### Task 3: The job writes transaction suggestions and honours the switches

**Files:**
- Modify: `internal/imports/suggest_job.go`, `internal/imports/suggest.go` (only if a helper is needed)
- Test: `internal/imports/suggest_job_test.go` (existing recording-completer harness)

**Interfaces:**
- Consumes: Task 1 `OwnerInfo.SuggestRules/SuggestFill`; Task 2 repository methods.

Behaviour (spec "The AI job"):
- `suggestForOwner`: return early (no AI call, no `countAttempt`) when `!SuggestRules && !SuggestFill`, in the same place as `Readonly || AIOff`.
- Rules off: skip the reuse step (`openFor`/`covered`) and skip `mergeIntoOpen`/`addSuggestionLinks`/`InsertSuggestion` in the write transaction.
- Fill on: in the write transaction, after `stillLinked(validateReply(...))`, build one `ImportTransactionSuggestion` per valid row from that row's own `CategoryID`/`PayeeID` (status open, owner, the row's type) and `InsertTransactionSuggestions`. When rules are also on, rows reused by the pre-AI `covered` step get a transaction suggestion with that open rule suggestion's category/payee, limited to the fields the row lacks (`suggestRow.Missing`), written in the pre-join transaction; skip a row left with neither.
- The opt-out re-checks (`optedOut`) stay; also re-read the two switches where `optedOut` is called inside the write transaction (a switch turned off meanwhile stops that kind being written). Simplest: make `optedOut` return the `OwnerInfo` or add a sibling.
- `undoIfOptedOut` also deletes the transaction suggestions this run inserted (`DeleteTransactionSuggestionsByLinks`).

- [ ] **Step 1: Failing tests** (names and assertions):
  - `TestSuggestWritesTransactionSuggestionsPerRow`: three rows same merchant, AI answers Coffee, Coffee, Restaurants (same payee) → one rule suggestion with Coffee; three transaction suggestions, the third carrying Restaurants.
  - `TestSuggestRulesOffWritesNoRuleSuggestion`: SuggestRules=false, SuggestFill=true → zero `import_suggestions` rows, transaction suggestions present; an open rule suggestion matching the row is not reused (the AI is called).
  - `TestSuggestFillOffWritesNoTransactionSuggestion`: SuggestFill=false → rule suggestion as today, zero transaction suggestions.
  - `TestSuggestBothOffMakesNoCall`: both false → recording completer saw 0 calls, rate limiter saw 0 attempts.
  - `TestSuggestReuseCopiesOpenSuggestionToTransaction`: both on, open rule suggestion covers the row → no AI call, transaction suggestion with the open suggestion's values for the missing fields only.
  - `TestSuggestOptOutUndoRemovesTransactionSuggestions`: opt-out flips after commit → both kinds gone.
  - `TestSuggestSharedAccountTransactionSuggestionGoesToOwner`: partner's tap → transaction suggestion `user_id` = owner.
- [ ] **Step 2: Run** `go test ./internal/imports/ -run 'TestSuggest'` — new ones FAIL, existing PASS.
- [ ] **Step 3: Implement.** Update the test harness's fake `OwnerFacts` so existing tests default to both switches on.
- [ ] **Step 4: Run** `go test ./internal/imports/...` — PASS.
- [ ] **Step 5: Commit** `feat: Suggest a category and payee per imported transaction`.

### Task 4: List, fill and dismiss transaction suggestions (service + API)

**Files:**
- Create: `internal/imports/transaction_suggestion.go`, `internal/imports/transaction_suggestion_test.go`
- Modify: `internal/model/imports_dto.go` (or where `GetImportSuggestionListResult` lives)
- Create: handlers in `internal/imports/api/suggestion.go` (next to the rule ones), routes in `internal/imports/api/routes.go`
- Modify: `internal/imports/suggestion.go` `ClearSuggestions` also calls `DeleteOpenTransactionSuggestionsByUser`; `dropSuggestions` path likewise
- Modify: `internal/test/apiparity/catalogue.go` (three scenarios: list, fill, dismiss; plus an error scenario for fill of an unknown id), goldens; `make swagger`
- Modify: `internal/web/middleware/idempotency.go` callers need nothing: the service pins (`reqctx.PinIdempotency`) in fill.

**Interfaces:**
- Produces:
  `model.ImportTransactionSuggestionResult{Id, TransactionId, AccountId, Type, Amount, Currency, Date, Description, ExternalPayee string; CategoryId, PayeeId *string}` (JSON camelCase as named; `Amount` decimal string; `Date` `"2006-01-02 15:04:05"`),
  `model.GetImportTransactionSuggestionListResult{Items []ImportTransactionSuggestionResult}`,
  `model.FillImportTransactionSuggestionRequest{Id string}` / `Result{Updated int}`,
  `model.DismissImportTransactionSuggestionRequest{Id string}` → returns the list result.
  Service: `GetTransactionSuggestionList(ctx, userID)`, `FillTransactionSuggestion(ctx, userID, req)`, `DismissTransactionSuggestion(ctx, userID, req)`.

Behaviour (spec "Reading", "Fill in", "Dismiss"): reuse `suggestionTargetCached`, `categoryFits`, `ownedEntities`, `updateRequest` from `suggestion.go`/`ruleapply.go`. List: AI off → delete open ones, empty; readonly or `!SuggestFill` → empty without deleting; invalid fields count as unset; nothing left to fill → not listed, deleted best effort; order date DESC then id. Fill: in one `WithTx`, `LockTransactionSuggestion`, not found / not open / other user → `errs.NewNotFound("Import transaction suggestion not found")`; fill only empty, valid fields via `s.txns.UpdateTransaction(ctx, userID, updateRequest(live, next))`; do **not** touch the link's `applied_*` snapshot or `RuleID`; `MarkTransactionSuggestionFilled` (or delete when nothing filled, `Updated` 0); `reqctx.PinIdempotency(ctx)`. Dismiss: owned open → delete; then return the list (a failed list read answers an empty list, as `DismissSuggestion` does).

- [ ] **Step 1: Failing tests**:
  - `TestTransactionSuggestionListHidesInvalid`: archived category + valid payee → listed with payee only; archived both → not listed and row deleted; transaction type changed → not listed.
  - `TestTransactionSuggestionListRespectsSwitches`: SuggestFill off → empty, row kept; AI off → empty, row deleted; readonly → empty, row kept.
  - `TestFillTransactionSuggestionFillsEmptyOnly`: transaction already has a payee → only category written, payee unchanged, `Updated` 1, status filled, link `AppliedCategoryID` unchanged, account revision bumped.
  - `TestFillTransactionSuggestionNothingLeft`: both fields set by hand → `Updated` 0, row deleted, transaction unchanged.
  - `TestFillTransactionSuggestionNotOwner`: another user (incl. the partner on a shared account) → not found.
  - `TestFilledRowSkippedByApplyRule`: after Fill in, `ApplyRule` without include-edited leaves the filled field; `PreviewRule` counts it in `alreadyEdited`.
  - `TestDismissTransactionSuggestionDeletes`: deleted; second dismiss → not found.
- [ ] **Step 2: Run** `go test ./internal/imports/ -run 'TransactionSuggestion|FilledRow'` — FAIL.
- [ ] **Step 3: Implement** service, DTOs, handlers (swag blocks like the rule-suggestion handlers, `endpoint.Handle`/`HandleNoBody`), routes.
- [ ] **Step 4: apiparity** scenarios + `UPDATE_GOLDEN=1`, inspect; `make swagger`; `go test ./internal/test/apiparity/ ./internal/imports/...` — PASS. Run `go test ./internal/test/revisionguard/`.
- [ ] **Step 5: Commit** `feat: List, fill and dismiss transaction suggestions`.

### Task 5: Filled rows keep an agreeing rule suggestion

**Files:**
- Modify: `internal/imports/suggestion.go` (`pendingSuggestionRows`)
- Test: `internal/imports/suggestion_test.go`

Behaviour (spec "Interaction with Suggested rules"): a row whose transaction has no empty field the rule suggestion sets is still pending when its transaction suggestion is `filled` (`FilledTransactionSuggestionLinks`) and every field the rule suggestion sets equals the transaction's current value. Accept then walks and releases it (nothing to fill). Rule-suggestion list also returns empty (hidden, not deleted) when `!SuggestRules`.

- [ ] **Step 1: Failing tests**: `TestRuleSuggestionKeepsAgreeingFilledRow` (Fill in Coffee → rule suggestion Coffee still lists the row); `TestRuleSuggestionDropsDisagreeingFilledRow` (Fill in Restaurants vs rule Coffee → not listed); `TestRuleSuggestionDropsHandEditedRow` (filled by hand, no Fill in → not listed, as today); `TestRuleSuggestionListHiddenWhenRulesOff` (empty, row kept).
- [ ] **Step 2: Run** `go test ./internal/imports/ -run RuleSuggestion` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/imports/... ./internal/test/apiparity/` — PASS.
- [ ] **Step 5: Commit** `feat: Keep a rule suggestion for rows filled with its values`.

### Task 6: Settings UI

**Files:**
- Modify: `web/src/api/dto/user.ts` (`UserOptions.AI_SUGGEST_RULES = 'ai_suggest_rules'`, `AI_SUGGEST_FILL = 'ai_suggest_fill'`), `web/src/api/user.ts` (`updateAiSuggestions(body)`), `web/src/features/user/queries.ts` (`userAiSuggestRules(user)`, `userAiSuggestFill(user)` = option `!== '0'`; `useUpdateAiSuggestions()` mirroring `useUpdateAi`)
- Modify: `web/src/features/settings/AiDialog.tsx`, `web/src/features/settings/ProfilePage.tsx`
- Modify: `web/src/lib/metrics.ts` (`USER_AI_SUGGESTIONS_UPDATE: 'appUserAiSuggestionsUpdate'`, fired on success with `{rules, fill}`)
- Modify: `locales/*.json` (11): `user.page.settings.profile.ai.imports_heading` ("Imported transactions"), `.suggest_rules_label` ("Suggest rules"), `.suggest_fill_label` ("Suggest category and payee")
- Test: `web/src/features/settings/ProfilePage.test.tsx` (or a new `AiDialog.test.tsx`)

- [ ] **Step 1: Failing test** `AiDialog shows the import switches only while AI is on`; toggling "Suggest rules" calls `onSuggestionsChange({suggestRules: false, suggestFill: true})`; switches disabled offline.
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/settings` — FAIL.
- [ ] **Step 3: Implement.** AiDialog props gain `suggestRules`, `suggestFill`, `onSuggestionsChange(next: {suggestRules: boolean, suggestFill: boolean})`. Group sits below the main switch inside the same dialog, same card style.
- [ ] **Step 4: Run** the vitest file + `pnpm vitest run src/lib/metrics-coverage.test.ts src/lib/offline-coverage.test.ts` + `go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat: AI import suggestion switches in settings`.

### Task 7: Inbox section "Imported transactions"

**Files:**
- Modify: `web/src/api/dto/imports.ts` (`ImportTransactionSuggestionDto` matching Task 4's JSON), `web/src/api/imports.ts` (`getImportTransactionSuggestionList`, `fillImportTransactionSuggestion(id)`, `dismissImportTransactionSuggestion(id)`)
- Modify: `web/src/features/imports/queries.ts` (`useImportTransactionSuggestions`, `useFillImportTransactionSuggestion`, `useDismissImportTransactionSuggestion`; query key `importTransactionSuggestions` in the shared `queryKeys`), `web/src/features/imports/suggestionRefresh.ts` (refetch the new key too)
- Create: `web/src/features/imports/ImportedTransactionsSection.tsx` + `.test.tsx`
- Modify: `web/src/features/inbox/useInbox.ts` (field `transactionSuggestions`, counted; rule `suggestions` empty when `!userAiSuggestRules`, transaction ones empty when `!userAiSuggestFill`; 404 → empty; error → `importsError`; refetch), `web/src/features/inbox/InboxPage.tsx` (section after Suggested rules), `useInbox.test.tsx`
- Modify: `web/src/lib/metrics.ts` (`IMPORT_TRANSACTION_SUGGESTION_FILL: 'appImportTransactionSuggestionFill'`, `IMPORT_TRANSACTION_SUGGESTION_DISMISS: 'appImportTransactionSuggestionDismiss'`), offline coverage list if needed
- Modify: `locales/*.json` (11): `inbox.sections.imported_transactions` ("Imported transactions"), `imports.transaction_suggestions.suggested` ("Suggested"), `.fill` ("Fill in"), `.dismiss` ("Dismiss")

Behaviour: rows like `DueRecurringSection`: a `<button>` card wrapping `TransactionRow` with a transaction built from the DTO (account/category/payee resolved from the caches, suggested category and payee in place; when only one is suggested the other shows what the transaction has — use the external payee text as the description fallback), `titleNote` = "Suggested", no `pageAccount`. Tap → `ViewTransactionDialog` with the same transaction and a footer `[✕ icon button] [Fill in wide button]` (copy `ViewRecurringDialog`'s footer layout), both `disabled={!online || pending}`. Fill success → close sheet, `trackEvent`, invalidate the transactions query (`['transactions']`) and the list; dismiss success → close, `trackEvent`. A 404 on fill/dismiss counts as done (close + refetch). Errors → `toast.error(apiErrorMessage(err))`.

- [ ] **Step 1: Failing tests**: `ImportedTransactionsSection renders one row per suggestion with the suggested category and payee and a Suggested note`; `tapping opens the sheet; Fill in posts fill-transaction-suggestion and closes`; `✕ posts dismiss`; `buttons disabled offline`; `useInbox counts transaction suggestions and hides them when ai_suggest_fill is 0`; `useInbox hides rule suggestions when ai_suggest_rules is 0`.
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/imports/ImportedTransactionsSection.test.tsx src/features/inbox` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** those + metrics/offline coverage tests + `pnpm lint` + `pnpm exec tsc -b --noEmit` (or the repo's typecheck script) + `go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat: Imported transactions section in the Inbox`.

### Task 8: Docs

**Files:** `CLAUDE.md` (imports route count 33 → 36; AI rule-suggestions bullet mentions per-transaction suggestions and the two switches), `docs/regression-test-plan.md` (items listed in the spec's "Docs and tests"), `docs/superpowers/specs/2026-11-22-...` status → implemented.

- [ ] **Step 1:** Edit; check the route count with `grep -c 'mux.Handle(' internal/imports/api/routes.go` against the doc's statement.
- [ ] **Step 2: Commit** `docs: AI import suggestion settings`.
