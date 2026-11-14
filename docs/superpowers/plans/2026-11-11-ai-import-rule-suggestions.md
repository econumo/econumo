# AI Rule Suggestions for Imported Transactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** With an AI provider configured, ongoing Apple Wallet / SimpleFIN imports that no rule classified produce one rule suggestion per merchant in the account owner's Inbox. Saving it through the rule editor creates the rule and fills those transactions. The rule prompt also gets an AI-picked match text and asks "apply to existing?" only when something would change.

**Architecture:** Everything server-side lives in `internal/imports`. A pure core (`suggest.go`: prompt, reply parsing, validation, grouping) sits under a background job (`suggest_job.go`) started after the import's DB transaction commits, on the onboarding job pattern. Suggestions persist in two new tables and are filtered at read time, so edits, rules and deletions clear them without extra write paths. Owner facts (language, timezone, access level) come through a new port wired in `internal/server`. The SPA adds an Inbox section, opens `RulePromptDialog` in a new "suggestion" mode, and fetches the AI match text in the existing "edit" mode.

**Tech Stack:** Go 1.27 (stdlib, sqlc, SQLite + PostgreSQL), React 19 + TanStack Query + Zustand, vitest, oxlint.

**Spec:** `docs/superpowers/specs/2026-11-11-ai-import-rule-suggestions-design.md`

## Global Constraints

- Without AI (`completer == nil`) nothing changes: no job, no suggestions, `suggest-match-text` answers an empty `matchText`, and the SPA never calls it (`aiEnabled()` false).
- The AI never writes a transaction. Only `accept-suggestion` (after the user saved a rule) writes, and only into **empty** fields.
- Suggestions only name the account **owner's** non-archived categories and payees, a category of the transaction's type (`categoryFits`), and a match text that occurs in the row's text.
- Only the account owner sees, accepts or dismisses a suggestion. Rules stay per user (no change to #342).
- Ongoing only: placed by `IngestAppleWallet` or `Sync` (never `processEvent` from `RetryEvent`, never `convertQueued`, never `ImportQueuedEvent`), **and** `ImportedAt - ExternalPostedAt <= 7 days`.
- One open suggestion per (owner, merchant). Dismissal is not remembered.
- Limits (constants, no env vars): new limiter scope `RateScopeSuggest = "import-ai-suggest"`, 30 jobs per window, IP-exempt; 50 rows per AI request; 60 s job timeout; 500 payees max in the prompt; `suggest-match-text` 10 s timeout, counted under `RateScopePreviewRule`.
- Logs never carry the prompt, the reply, payee text or error text: `user_id` + `err_type` only.
- New migration version: `20261112000000` (above every released one), both engines, identical tables.
- `.sql` files ASCII-only. Comments sparse, the why only (CLAUDE.md "Comments").
- Every new route gets an `apiparity` scenario + golden. Regenerate with `UPDATE_GOLDEN=1` and inspect the diff.
- Every new i18n key in all 11 `locales/*.json`. Plural strings are pipe-joined (`pluralPick`).
- Every new user action fires a `METRICS` key at its success point. New write controls read `useOnline`.
- Go commands: `export PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1`.

## Review Focus

1. **A partner's tap on the owner's account** must produce a suggestion for the owner (owner's categories, payees, language), and the partner must never see it. Pinned in Task 5 (`TestSuggestSharedAccountGoesToOwner`) and Task 6 (`TestSuggestionListOwnerOnly`).
2. **A user who edits a suggested transaction by hand before tapping the suggestion** must see the row leave the suggestion (both fields filled), or stay with only the still-empty field filled on accept, never overwritten. Pinned in Task 6 (`TestAcceptFillsOnlyEmptyFields`, `TestSuggestionListDropsFilledTransactions`).
3. **A first SimpleFIN sync over 90 days** must not flood the Inbox: only rows posted within 7 days of import qualify. Pinned in Task 5 (`TestSuggestSkipsBackfill`).
4. **An AI reply that is malformed, invents ids, or proposes a match text not in the row** must produce nothing for that row and never fail the import. Pinned in Task 4 (`TestValidateReply*`) and Task 5 (`TestSuggestJobFailureLeavesImportIntact`).
5. **Repeat taps at a merchant with an open suggestion** must join it with no new AI call, even when the AI would now pick a different category. Pinned in Task 4 (`TestMergeIntoOpen*`) and Task 5 (`TestSuggestReusesOpenSuggestionWithoutAI`).

---

## File Structure

Server (`internal/imports`):
- `suggest.go` (new): pure core: the `suggestRow` input, prompt building, reply parsing, `validateReply`, `groupRows`, `mergeIntoOpen`, `matchTextIn`.
- `suggest_job.go` (new): `collectCandidates`, `startSuggestJob`, the job body, the runner.
- `suggestion.go` (new): use cases `GetSuggestionList`, `AcceptSuggestion`, `DismissSuggestion`, `SuggestMatchText`.
- `ports.go`: add `OwnerFacts`.
- `repository.go`: add the suggestion repo methods.
- `ingest.go`, `sync.go`: return the created links and start the job after commit.
- `ruleapply.go`: preview/apply skip rows the rule would not change.
- `repo/suggestion.go` (new): repository implementation.
- `api/suggestion.go` (new) + `api/routes.go`: four handlers.

Shared:
- `internal/model/imports.go` / `imports_dto.go`: `ImportSuggestion`, DTOs.
- `internal/model/transaction_view.go`: `ImportNamed.Archived`.
- `internal/infra/storage/migrations/{sqlite,pgsql}/20261112000000.sql`, `internal/infra/storage/sqlc/query/{sqlite,pgsql}/imports.sql`.
- `internal/server/glue_imports.go`, `glue_transaction.go`, `server.go`: `OwnerFacts` adapter, `Archived`, the limiter scope, wiring.

SPA (`web/src`):
- `api/dto/imports.ts`, `api/imports.ts` (or the existing imports client file), `app/queryKeys.ts`.
- `features/imports/queries.ts`: `useImportSuggestions`, `useAcceptImportSuggestion`, `useDismissImportSuggestion`, `useSuggestMatchText`.
- `features/imports/SuggestionsSection.tsx` (new), `features/inbox/useInbox.ts`, `features/inbox/InboxPage.tsx`.
- `app/uiStore.ts` (`RulePromptParams` becomes a union), `features/imports/RulePromptDialog.tsx`.
- `lib/metrics.ts`, `locales/*.json`.

Docs: `CLAUDE.md`, `docs/regression-test-plan.md`.

---

### Task 1: Preview and apply skip rows the rule would not change

The "apply to existing?" question must count only rows that would actually change. Today `PreviewRule` counts every matching row the rule `touches`, including rows already carrying its values (the transaction just edited, or a suggestion's rows just accepted).

**Files:**
- Modify: `internal/imports/ruleapply.go`
- Test: `internal/imports/ruleapply_test.go`
- Goldens: `internal/test/apiparity/testdata/golden/` (regenerate only if a preview/apply scenario's counts move)

**Interfaces:**
- Produces: `func changes(r *model.ImportRule, live *model.Transaction, income map[vo.Id]bool) bool` in `ruleapply.go`: true when `applyTargets(r, live, income)` differs from `live`'s classification (category, payee, tag, label set; reuse `sameID`/`sameIDs`).

- [ ] **Step 1: Write the failing tests** in `ruleapply_test.go`, using the existing ruleapply test harness:
  - `TestPreviewRuleSkipsRowsAlreadyClassified`: two imported rows match "COFFEE". Row A already carries the rule's category and payee (set by hand, so its applied snapshot differs), row B is empty. `PreviewRule` returns `Matched == 1`, `AlreadyEdited == 0`.
  - `TestApplyRuleLeavesUnchangedRowsUncounted`: the same setup. `ApplyRule` returns `Updated == 1`, `Skipped == 0`, and row A's transaction is not rewritten (its `transactions_revision` is unchanged).
- [ ] **Step 2: Run them.** `go test ./internal/imports/ -run 'TestPreviewRuleSkipsRowsAlreadyClassified|TestApplyRuleLeavesUnchangedRowsUncounted'`. Expected: FAIL (Matched 2, Skipped 1).
- [ ] **Step 3: Implement `changes`** and call it in `PreviewRule` and `ApplyRule` right after the `owned`/`touches` check: a row that `!changes(...)` is `continue`d before it is counted or checked for `edited`.
- [ ] **Step 4: Run the package tests and the parity smoke.** `go test ./internal/imports/... ./internal/test/apiparity/ ./internal/test/revisionguard/`. Expected: PASS. If a golden moved, regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and check that only `matched`/`skipped` counts changed.
- [ ] **Step 5: Commit.** `git commit -m "fix: Rule preview and apply count only rows the rule would change"`

---

### Task 2: The rule prompt skips an apply step with nothing to apply

**Files:**
- Modify: `web/src/features/imports/RulePromptDialog.tsx`
- Test: `web/src/features/imports/RulePromptDialog.test.tsx`
- Modify: `docs/regression-test-plan.md` (the rule-prompt item)

**Interfaces:**
- Consumes: Task 1's server semantics (preview `matched` excludes the edited transaction itself).

- [ ] **Step 1: Write the failing tests** (extend the existing msw setup):
  - `skips the run step when the preview matches nothing`: after "Create rule", the refreshed preview answers `{matched: 0, alreadyEdited: 0}` for the run scope and `{matched: 3}` for `all`. The dialog shows the `all_question` text with 3 and never the `apply_question` text.
  - `closes after saving when neither scope has anything to apply`: both previews answer `matched: 0`. After "Create rule" the dialog unmounts, `create-rule` was called once, `apply-rule` never.
- [ ] **Step 2: Run them.** `cd web && pnpm vitest run src/features/imports/RulePromptDialog.test.tsx`. Expected: FAIL.
- [ ] **Step 3: Implement.** In `save()`, after the refreshed preview: when `matched === 0` (or the run step does not apply because `link.runId` is empty), go to the `all` step (fetching `allPreview`), and close (`onDone()`) when that answers `matched === 0`. Also skip the `all` step after a run apply when `allPreview` answers 0. The `includeEdited` control stays per step as today.
- [ ] **Step 4: Run them.** The same command. Expected: PASS. Then `pnpm lint`.
- [ ] **Step 5: Update the regression plan** (the "create a rule from an edit" item): "the apply question appears only when the rule would change at least one other transaction".
- [ ] **Step 6: Commit.** `git commit -m "feat: Rule prompt asks to apply only when something would change"`

---

### Task 3: Suggestion storage

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261112000000.sql`, `internal/infra/storage/migrations/pgsql/20261112000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/imports.sql`, `.../pgsql/imports.sql`, then `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`)
- Modify: `internal/model/imports.go`, `internal/imports/repository.go`, `internal/imports/repo/sqlite.go`, `repo/pgsql.go`
- Create: `internal/imports/repo/suggestion.go`
- Test: `internal/imports/repo/repo_integration_test.go`

**Interfaces:**
- Produces in `internal/model/imports.go`:
  ```go
  const (
      ImportSuggestionStatusOpen     = "open"
      ImportSuggestionStatusAccepted = "accepted"
  )
  type ImportSuggestion struct {
      ID         vo.Id
      UserID     vo.Id   // the account owner
      MatchField string  // ImportRuleMatchField* constant: external_payee | description
      MatchText  string  // normalized (imports.NormalizeText)
      CategoryID *vo.Id
      PayeeID    *vo.Id
      Status     string
      LinkIDs    []vo.Id // filled by the read methods
      CreatedAt  time.Time
      UpdatedAt  time.Time
  }
  ```
- Produces on `imports.Repository`:
  ```go
  InsertSuggestion(ctx context.Context, s *model.ImportSuggestion) error          // row + its link rows
  AddSuggestionLinks(ctx context.Context, suggestionID vo.Id, linkIDs []vo.Id) error // ignores pairs already present
  GetSuggestion(ctx context.Context, id vo.Id) (*model.ImportSuggestion, error)    // errs.NotFound when missing
  ListOpenSuggestionsByUser(ctx context.Context, userID vo.Id) ([]model.ImportSuggestion, error) // oldest first
  SetSuggestionStatus(ctx context.Context, id vo.Id, status string, at time.Time) error
  DeleteSuggestion(ctx context.Context, id vo.Id) error                            // link rows cascade
  ```
- Schema (both engines): `import_suggestions(id TEXT PK, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, match_field TEXT NOT NULL, match_text TEXT NOT NULL, category_id TEXT NULL, payee_id TEXT NULL, status TEXT NOT NULL, created_at, updated_at)` with an index on `(user_id, status)`. `import_suggestion_links(suggestion_id TEXT NOT NULL REFERENCES import_suggestions(id) ON DELETE CASCADE, link_id TEXT NOT NULL REFERENCES import_transaction_links(id) ON DELETE CASCADE, PRIMARY KEY (suggestion_id, link_id))`. No FK on `category_id`/`payee_id` (the read filters stale ids, spec "Storage"). Datetime column types follow the existing import tables in each engine.

- [ ] **Step 1: Write the failing integration test** `TestSuggestionRepoRoundTrip` in `repo_integration_test.go`, on `dbtest.New(t)`: insert a suggestion with two links. `ListOpenSuggestionsByUser` returns it with both `LinkIDs`. `AddSuggestionLinks` with one old and one new id leaves three. `SetSuggestionStatus(accepted)` drops it from the open list. `DeleteSuggestion` removes it and its link rows. Deleting a ledger row (`import_transaction_links`) removes its link row.
- [ ] **Step 2: Run it.** `go test ./internal/imports/repo/ -run TestSuggestionRepoRoundTrip`. Expected: FAIL (compile).
- [ ] **Step 3: Write the migrations and queries, run `sqlc generate`, implement the repo** with the engine-adapter pattern used by `repo/rule.go` (canonical sqlite types in the `querier`, a pgsql shim).
- [ ] **Step 4: Run it on both engines.** `go test ./internal/imports/repo/` then `make test-repo-pgsql` (or `DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/imports/repo/` with `DATABASE_TEST_PGSQL_URL` set). Expected: PASS. `go vet ./...` clean.
- [ ] **Step 5: Commit.** `git commit -m "feat: Storage for import rule suggestions"`

---

### Task 4: The pure suggestion core

**Files:**
- Create: `internal/imports/suggest.go`
- Test: `internal/imports/suggest_test.go`

**Interfaces:**
- Produces (package-private, used by Tasks 5 and 7):
  ```go
  type suggestRow struct {
      Link    model.ImportTransactionLink
      Type    model.TransactionType
      Amount  string   // in the account's currency
      Currency string
      Account string   // account name
      Missing struct{ Category, Payee bool } // which fields the transaction still lacks
  }
  type suggestVocabulary struct {
      Categories []model.ImportNamed // owner's, non-archived
      Payees     []model.ImportNamed // owner's, non-archived, at most 500
      income     map[vo.Id]bool
  }
  type validSuggestion struct {
      Row        suggestRow
      MatchField string
      MatchText  string // normalized
      CategoryID *vo.Id
      PayeeID    *vo.Id
  }
  func buildSuggestPrompt(rows []suggestRow, v suggestVocabulary, language, timezone string) (system, user string)
  func parseSuggestReply(reply string) (map[string]suggestReplyRow, error) // key = row index as decimal string
  func validateReply(rows []suggestRow, reply map[string]suggestReplyRow, v suggestVocabulary) []validSuggestion
  func matchTextIn(link model.ImportTransactionLink, raw string) (field, normalized string, ok bool)
  func groupRows(valid []validSuggestion) []model.ImportSuggestion // new suggestions, LinkIDs filled, Status open
  func mergeIntoOpen(groups []model.ImportSuggestion, open []model.ImportSuggestion) (fresh []model.ImportSuggestion, joins map[vo.Id][]vo.Id)
  func openFor(row suggestRow, open []model.ImportSuggestion, income map[vo.Id]bool) (vo.Id, bool) // reuse before asking
  ```
- Prompt content (exact intent, wording is the builder's): the system prompt says the AI helps classify imported bank and card transactions. It must include `categoryId` / `payeeId` only when highly confident, use only ids from the lists, pick a category whose `type` matches the row's `type`, and set `matchText` to the shortest substring of `payee` (or of `description` when `payee` is empty) that identifies the merchant, copied exactly. Names may be in any language; use the timezone and language to read local merchant names. Reply with JSON only: `{"rows":{"<key>":{"matchText":"...","categoryId":"...","payeeId":"..."}}}`, leaving out fields and rows it is not confident about. The user message is JSON: `{"language","timezone","categories":[{id,name,type}],"payees":[{id,name}],"rows":[{key,payee,description,amount,currency,type,date,account}]}`. Rows send only fields from `suggestRow`, never ids of transactions.
- Rules for `matchTextIn`: normalize both with `NormalizeText`. Use `external_payee` when `link.ExternalPayee` is non-blank, else `description`. ok only when the normalized raw text is non-empty and occurs case-insensitively in the normalized field text. `normalized` keeps the AI's casing.
- Rules for `validateReply` (spec "Validation"): drop a category not in the vocabulary or failing `categoryFits` for the row's type, a payee not in the vocabulary, a field the row is not `Missing`, a row whose `matchText` fails `matchTextIn`, and a row left with neither field.
- Rules for `groupRows`: key = (MatchField, lower-cased MatchText). Within a key, the classification held by the most rows wins (a category+payee pair compared as one value). Ties go to the row with the newest `ExternalPostedAt`.
- Rules for `mergeIntoOpen` and `openFor`: a group or row joins an open suggestion when the fields are equal and one lower-cased match text contains the other (`openFor`: the open suggestion's match text occurs in the row's text, i.e. `matchRule` semantics with `contains`, and its category, when set, fits the row's type). A joined group keeps the open suggestion's classification.

- [ ] **Step 1: Write the failing tests** in `suggest_test.go` (table-driven where natural):
  - `TestMatchTextIn`: `"SQ *BLUE BOTTLE COFFEE #0423"` + `"blue bottle coffee"` → `external_payee`, ok. `"STARBUCKS"` + `"Peets"` → not ok. An empty payee falls back to the description. A blank raw text → not ok.
  - `TestValidateReplyDropsInventedIDs`: a category id not in the vocabulary → that field dropped, the payee kept.
  - `TestValidateReplyDropsWrongTypeCategory`: an income category on an expense row → dropped, so a row with only that is dropped entirely.
  - `TestValidateReplyDropsMatchTextNotInRow`.
  - `TestValidateReplyDropsFieldAlreadySet`: `Missing.Payee == false` → the payee is dropped.
  - `TestParseSuggestReplyRejectsGarbage`: non-JSON, and JSON without `rows`, → error.
  - `TestGroupRowsMajorityWins`: three rows "BLUE BOTTLE", two with Coffee and one with Dining → one suggestion with Coffee and three LinkIDs.
  - `TestGroupRowsTieGoesToNewest`.
  - `TestMergeIntoOpenJoinsContainedMatchText`: open "BLUE BOTTLE" (Coffee) + group "BLUE BOTTLE COFFEE" (Dining) → `joins[open.ID]` holds the group's links and `fresh` is empty.
  - `TestMergeIntoOpenKeepsDifferentField`: same text but `description` vs `external_payee` → fresh.
  - `TestOpenForChecksCategoryType`.
  - `TestBuildSuggestPromptOmitsOtherData`: the user JSON has exactly the listed keys, and no transaction id or email appears.
- [ ] **Step 2: Run them.** `go test ./internal/imports/ -run 'TestMatchTextIn|TestValidateReply|TestParseSuggestReply|TestGroupRows|TestMergeIntoOpen|TestOpenFor|TestBuildSuggestPrompt'`. Expected: FAIL (compile).
- [ ] **Step 3: Implement `suggest.go`** to the signatures and rules above.
- [ ] **Step 4: Run them.** The same command. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: Validation and grouping for AI import rule suggestions"`

---

### Task 5: The suggestion job

**Files:**
- Create: `internal/imports/suggest_job.go`
- Modify: `internal/imports/ingest.go`, `internal/imports/sync.go`, `internal/imports/service.go`, `internal/imports/ports.go`
- Modify: `internal/model/transaction_view.go` (`ImportNamed.Archived bool`), `internal/server/glue_transaction.go` (set `Archived` in `TransactionImportCategories.CategoriesByOwner` and `TransactionImportPayees.PayeesByOwner`), `internal/server/glue_imports.go` (`ImportsOwnerFacts`), `internal/server/server.go` (limiter scope 30 + IP-exempt, `importsSvc.SetSuggester(aiClient, NewImportsOwnerFacts(userRepo))` instead of `SetCompleter`)
- Test: `internal/imports/suggest_job_test.go`, `internal/server/glue_imports_test.go`

**Interfaces:**
- Consumes: Task 3 repo methods, Task 4 core.
- Produces in `ports.go`:
  ```go
  type OwnerFacts interface {
      // Language and Timezone are the stored values ("" when unknown); Readonly is the effective access level.
      OwnerFacts(ctx context.Context, userID vo.Id) (language, timezone string, readonly bool, err error)
  }
  const RateScopeSuggest = "import-ai-suggest"
  ```
- Produces on `Service`:
  ```go
  func (s *Service) SetSuggester(c Completer, facts OwnerFacts) // replaces SetCompleter
  func (s *Service) SetJobRunner(run func(func()))              // tests run jobs inline; default: go run()
  func (s *Service) startSuggestJob(links []model.ImportTransactionLink) // nil-safe, returns at once
  ```
- Changes `applyEvent` to return the created ledger row: `(status string, amountUpdated bool, created *model.ImportTransactionLink, err error)`, non-nil only for `ImportIngestStatusCreated`. `processEvent` returns it too. `IngestAppleWallet` calls `s.startSuggestJob` with it **after** `WithTx` returns nil. `RetryEvent` ignores it. `syncAccount` appends to a slice on the run, and `Sync` calls `startSuggestJob` once after `finishRun`. `convertQueued` and `ImportQueuedEvent` never call it.
- Job body (spec "The AI job"), with a fresh context and a 60 s timeout, panics recovered, a WARN `import suggest failed` with `user_id` + `err_type`:
  1. Keep links with `TransactionID != nil` and `ImportedAt.Sub(ExternalPostedAt) <= 7*24h`.
  2. Read each live transaction (`lister.GetByID`). Skip transfers, rows with both category and payee set, and deleted ones. The owner is `accounts.AccountOwner(live.AccountID)`.
  3. Per owner: skip when `OwnerFacts` says readonly. Load the open suggestions. A row `openFor` an open suggestion → `AddSuggestionLinks`, no AI.
  4. The rest (at most 50, newest posted first) → `limiter.Allow` + `Reserve` on `RateScopeSuggest` keyed by the owner (over the cap: return silently, INFO `import suggest skipped`), `buildSuggestPrompt`, `completer.Complete(ctx, owner, system, user)`, `parseSuggestReply`, `validateReply`, `groupRows`, `mergeIntoOpen` against the open list re-read right before the write, then in one `WithTx`: `InsertSuggestion` for fresh groups and `AddSuggestionLinks` for joins.
- `ImportsOwnerFacts` reads `GetLanguage`, `GetTimezone` and the user aggregate's `EffectiveAccessLevel(clk.Now())` through the user repository.

- [ ] **Step 1: Write the failing tests.** In `suggest_job_test.go` (fake completer, inline runner):
  - `TestSuggestCreatesSuggestionForLivePush`: a mapped card, an Apple Wallet push at a new merchant, the AI answers a valid row → one open suggestion with that link. `ImportIngestStatusCreated` is unchanged.
  - `TestSuggestSkipsBackfill`: a SimpleFIN sync with one row posted 30 days ago and one posted 2 days ago → the prompt holds only the recent row.
  - `TestSuggestNotForReplayOrRetry`: rows placed by `LinkAccount`'s replay, by `RetryEvent` and by `ImportQueuedEvent` → the completer is never called.
  - `TestSuggestSkipsRowsRulesClassified`: a rule set both fields → no call. A rule set only the category → the prompt asks for the payee only (validated by `Missing`).
  - `TestSuggestSharedAccountGoesToOwner`: a partner's card mapped onto the owner's account. The suggestion's `UserID` is the owner, the prompt carries the owner's categories and stored language, and `Complete` was called with the owner's id.
  - `TestSuggestReusesOpenSuggestionWithoutAI`: an open "BLUE BOTTLE" suggestion, then a new push "SQ *BLUE BOTTLE #12" → the link joins it and the completer is not called.
  - `TestSuggestSkipsReadonlyOwner`.
  - `TestSuggestRespectsCap`: a limiter that refuses → no call, no suggestion.
  - `TestSuggestJobFailureLeavesImportIntact`: the completer errors, then panics → the push still answers `created`, the transaction exists, and there are no suggestions.
  - `TestSuggestNoCompleterDoesNothing`.
  In `glue_imports_test.go`: `TestImportsOwnerFacts` (language, timezone, readonly from an expired `access_until`).
- [ ] **Step 2: Run them.** `go test ./internal/imports/ -run TestSuggest && go test ./internal/server/ -run TestImportsOwnerFacts`. Expected: FAIL.
- [ ] **Step 3: Implement** the port, the `Archived` field, `applyEvent`'s new return, the job, and the server wiring (limiter `appimports.RateScopeSuggest: 30` in the limits map and `true` in `limiterIPExempt`).
- [ ] **Step 4: Run.** `go test ./internal/imports/... ./internal/server/... ./internal/test/archtest/ ./internal/test/apiparity/ ./internal/test/revisionguard/`. Expected: PASS, and no golden change (no AI in the parity harness).
- [ ] **Step 5: Commit.** `git commit -m "feat: Background AI job proposes rules for new imported transactions"`

---

### Task 6: Suggestion list, accept and dismiss

**Files:**
- Create: `internal/imports/suggestion.go`, `internal/imports/api/suggestion.go`
- Modify: `internal/imports/api/routes.go`, `internal/model/imports_dto.go`
- Test: `internal/imports/suggestion_test.go`, `internal/imports/api/suggestion_test.go` (harness), `internal/test/apiparity/` scenarios + goldens, `internal/server/import_suggestions_test.go`
- Regenerate: `make swagger`

**Interfaces:**
- Produces DTOs in `imports_dto.go`:
  ```go
  type ImportSuggestionTransactionResult struct {
      TransactionId string `json:"transactionId"`
      AccountId     string `json:"accountId"`
      Amount        string `json:"amount"`
      Currency      string `json:"currency"`      // the account's currency code
      Date          string `json:"date"`          // datetime.Layout
      ExternalPayee string `json:"externalPayee"`
  }
  type ImportSuggestionResult struct {
      Id           string                              `json:"id"`
      MatchField   string                              `json:"matchField"`
      MatchText    string                              `json:"matchText"`
      CategoryId   string                              `json:"categoryId"` // "" when none
      PayeeId      string                              `json:"payeeId"`
      Transactions []ImportSuggestionTransactionResult `json:"transactions"` // newest first
  }
  type GetImportSuggestionListResult struct { Items []ImportSuggestionResult `json:"items"` }
  type AcceptImportSuggestionRequest struct { Id string `json:"id"`; RuleId string `json:"ruleId"` } // Validate(): both UUIDs
  type AcceptImportSuggestionResult struct { Updated int `json:"updated"` }
  type DismissImportSuggestionRequest struct { Id string `json:"id"` }
  ```
- Produces on `Service`:
  ```go
  func (s *Service) GetSuggestionList(ctx context.Context, userID vo.Id) (*model.GetImportSuggestionListResult, error)
  func (s *Service) AcceptSuggestion(ctx context.Context, userID vo.Id, req model.AcceptImportSuggestionRequest) (*model.AcceptImportSuggestionResult, error)
  func (s *Service) DismissSuggestion(ctx context.Context, userID vo.Id, req model.DismissImportSuggestionRequest) (*model.GetImportSuggestionListResult, error)
  ```
- Routes: `GET /api/v1/import/get-suggestion-list`, `POST /api/v1/import/accept-suggestion`, `POST /api/v1/import/dismiss-suggestion`. Always mounted.
- Read-time filter (spec "Reading suggestions"): for each open suggestion of the caller, drop a category id not among the caller's non-archived categories and a payee id not among their payees. Keep a transaction only when it is live, on an account the caller owns (`ownsAccount`), and still empty in at least one field the suggestion sets (a category only counts when it fits the type). Skip a suggestion with no field or no transaction left, and delete it (lazy cleanup).
- Accept: the suggestion must be the caller's and open, else a not-found that names nothing. The rule must be the caller's (`ownedRule`) with action classify. For each still-pending transaction the rule matches (`matchRule` on `linkEvent`), fill only the empty fields from the rule's category (when it fits) and payee, through `txns.UpdateTransaction` (keeping labels), `setApplied` + `UpdateLink`, one `WithTx` per transaction like `ApplyRule`. Then `SetSuggestionStatus(accepted)`. Answer `Updated`. Pin the idempotency key (`reqctx.PinIdempotency`) inside the status write's transaction.
- Dismiss: owner-only, then `DeleteSuggestion`. Answer the fresh list.

- [ ] **Step 1: Write the failing tests** in `suggestion_test.go`:
  - `TestSuggestionListOwnerOnly`: a partner calling the list sees nothing, and their accept and dismiss get not-found.
  - `TestSuggestionListDropsFilledTransactions`: one of two transactions edited by hand (both fields) → the list shows one. Both edited → the suggestion is gone and its row deleted.
  - `TestSuggestionListDropsArchivedCategory`: the category is archived and there is no payee → not listed.
  - `TestAcceptFillsOnlyEmptyFields`: a transaction whose payee was set by hand → after accept the category is filled and the hand-set payee kept. `Updated == 1`, status accepted, and the ledger `applied_*` updated.
  - `TestAcceptSkipsRowsRuleDoesNotMatch`: the user edited the match text in the editor → the non-matching transaction stays empty.
  - `TestAcceptRejectsForeignRule`.
  - `TestDismissForgets`: dismiss, then a new push from the same merchant → a fresh suggestion (job inline).
  - Add apiparity scenarios for the three routes (empty list, accept, dismiss, foreign id), then add `TestImportSuggestionsEndToEnd` in `internal/server/import_suggestions_test.go` (through `server.BuildAPI` with the `recordingCompleter` of `internal/server/onboarding_steps_test.go`): push → list → `create-rule` → accept → the transaction carries the category, and the list is empty.
- [ ] **Step 2: Run them.** `go test ./internal/imports/... ./internal/server/ -run 'Suggestion'`. Expected: FAIL.
- [ ] **Step 3: Implement** the use cases, the handlers with swag blocks (pattern of `api/rule.go`, `endpoint.Handle` / `endpoint.HandleNoBody`), and the routes.
- [ ] **Step 4: Regenerate and run.** `make swagger`, `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` (inspect: only the new scenarios added), then `make go-test`. Expected: PASS with coverage ≥ 80.
- [ ] **Step 5: Commit.** `git commit -m "feat: List, accept and dismiss import rule suggestions"`

---

### Task 7: AI match text for the rule prompt

**Files:**
- Modify: `internal/imports/suggestion.go`, `internal/imports/api/suggestion.go`, `internal/imports/api/routes.go`, `internal/model/imports_dto.go`
- Test: `internal/imports/suggestion_test.go`, apiparity scenario + golden

**Interfaces:**
- Consumes: `matchTextIn` (Task 4), the completer and `OwnerFacts` (Task 5).
- Produces:
  ```go
  type SuggestImportMatchTextRequest struct { LinkId string `json:"linkId"` } // Validate(): UUID
  type SuggestImportMatchTextResult struct { MatchField string `json:"matchField"`; MatchText string `json:"matchText"` }
  func (s *Service) SuggestMatchText(ctx context.Context, userID vo.Id, req model.SuggestImportMatchTextRequest) (*model.SuggestImportMatchTextResult, error)
  ```
  Route `POST /api/v1/import/suggest-match-text` (POST: the answer comes from a paid call, and it is not cached like a GET).
- Behaviour: the link must be linked to a live transaction on an account the caller owns, else not-found. With no completer, answer `{"matchField":"","matchText":""}` with no limiter count. Otherwise count `RateScopePreviewRule`, ask with a 10 s context-derived timeout (system prompt: return `{"matchText":"..."}`, the shortest substring identifying the merchant, copied exactly, in the caller's language/timezone), and validate with `matchTextIn`. Any failure answers empty (a WARN with `err_type`, never an error response).

- [ ] **Step 1: Write the failing tests**: `TestSuggestMatchTextValid` (fake completer returns "BLUE BOTTLE COFFEE" for "SQ *BLUE BOTTLE COFFEE #0423" → `external_payee` + `BLUE BOTTLE COFFEE`), `TestSuggestMatchTextRejectsInvented` (returns "STARBUCKS" → empty), `TestSuggestMatchTextNoAI` (empty, limiter untouched), `TestSuggestMatchTextForeignLink` (not-found), `TestSuggestMatchTextCompleterError` (empty, 200). Add the apiparity scenario (no AI in the harness → empty answer).
- [ ] **Step 2: Run them.** `go test ./internal/imports/ -run TestSuggestMatchText`. Expected: FAIL.
- [ ] **Step 3: Implement it.**
- [ ] **Step 4: Run.** `make swagger && UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && make go-test`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: AI-picked match text for import rules"`

---

### Task 8: Inbox section and saving a suggestion

**Files:**
- Modify: `web/src/api/dto/imports.ts`, the imports API client in `web/src/api/` (where `getImportQueue` lives), `web/src/app/queryKeys.ts`, `web/src/features/imports/queries.ts`, `web/src/features/inbox/useInbox.ts`, `web/src/features/inbox/InboxPage.tsx`, `web/src/app/uiStore.ts`, `web/src/features/imports/RulePromptDialog.tsx`, `web/src/lib/metrics.ts`, `locales/*.json` (all 11)
- Create: `web/src/features/imports/SuggestionsSection.tsx`
- Test: `web/src/features/imports/SuggestionsSection.test.tsx`, `RulePromptDialog.test.tsx`, `web/src/features/inbox/useInbox.test.tsx`

**Interfaces:**
- Consumes: the Task 6 routes.
- Produces:
  - DTOs `ImportSuggestionDto`, `ImportSuggestionTransactionDto` mirroring the Go JSON.
  - `queryKeys.importSuggestions`. `useImportSuggestions()` (`staleTime: TEN_MINUTES`), invalidated wherever `importQueue` is invalidated after an import and after any transaction write (an edit can clear a suggestion).
  - `useAcceptImportSuggestion()` → `{ updated }`. On success it invalidates suggestions, transactions, accounts, budget, budgetTransactions and fires `METRICS.IMPORT_SUGGESTION_ACCEPT` with `{ changedMatch, changedCategory, changedPayee }` (booleans).
  - `useDismissImportSuggestion()`: on success it writes the returned list into the cache and fires `METRICS.IMPORT_SUGGESTION_DISMISS`.
  - `RulePromptParams` becomes `{ kind: 'edit'; link: TransactionImportLinkDto; diff: RuleDiff } | { kind: 'suggestion'; suggestion: ImportSuggestionDto }`. Every existing `setRulePrompt` call passes `kind: 'edit'`.
  - `Inbox.suggestions: ImportSuggestionDto[]`, counted in `count`. `isLoaded` waits for it like the queue.
- Section: `inbox.sections.suggested_rules` ("Suggested rules"), right after "To review". Each row (a button) shows the match text, `→ {category} · {payee}` (whichever exists, names from `useCategories`/`usePayees`), and `{count} transactions` (pluralized) with the newest amount and date. Its trailing button is **Dismiss** (`imports.suggestions.dismiss`), disabled offline. A tap calls `setRulePrompt({ kind: 'suggestion', suggestion })`.
- Suggestion mode in `RulePromptDialog`: the summary line `imports.suggestions.prompt_summary` ("Suggested for {count} transactions"), `matchField`/`matchValue` initialised from the suggestion, `matchType: 'contains'`, the spec's `categoryId`/`payeeId` from the suggestion, and an editable category and payee (reuse the pickers the rule editor `ImportRuleDialog.tsx` uses). The live count preview uses scope `all`. **Save** = `create-rule`, then `accept-suggestion {id, ruleId}`, then the `all` preview: when `matched > 0`, the existing `all` step; else `onDone()` and a toast `imports.rules.prompt.applied_toast` with the accept's `updated`. **Cancel** leaves the suggestion. Offline the Save button is disabled as today.
- `METRICS`: `IMPORT_SUGGESTION_ACCEPT: 'appImportSuggestionAccept'`, `IMPORT_SUGGESTION_DISMISS: 'appImportSuggestionDismiss'`.

- [ ] **Step 1: Write the failing tests:**
  - `SuggestionsSection.test.tsx`: `renders one row per suggestion with names and count`; `dismiss removes the row and fires the metric`; `dismiss is disabled offline`; `tapping a row opens the rule prompt in suggestion mode`.
  - `RulePromptDialog.test.tsx`: `suggestion mode prefills match text, category and payee`; `saving creates the rule, accepts the suggestion and closes when nothing else matches`; `saving asks to apply to others when the all preview matches 2`; `the accept metric reports changed fields`.
  - `useInbox.test.tsx`: `counts suggestions in the badge`.
- [ ] **Step 2: Run them.** `cd web && pnpm vitest run src/features/imports src/features/inbox`. Expected: FAIL.
- [ ] **Step 3: Implement**, adding the i18n keys to all 11 catalogues (match the register in `locale-register`: de Sie, nl u, fr vous, pt European).
- [ ] **Step 4: Run.** `pnpm test` (includes `metrics-coverage`, `offline-coverage`) and `pnpm lint`, then `go test ./internal/test/i18ntest/`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: Suggested rules in the Inbox"`

---

### Task 9: AI match text in the edit flow

**Files:**
- Modify: the imports API client, `web/src/features/imports/queries.ts`, `web/src/features/imports/RulePromptDialog.tsx`, `web/src/lib/metrics.ts`
- Test: `web/src/features/imports/RulePromptDialog.test.tsx`

**Interfaces:**
- Consumes: the Task 7 route, `aiEnabled()` from `web/src/lib/config.ts`.
- Produces: `useSuggestMatchText(linkId: Id, enabled: boolean)` (a `useQuery` keyed `['importMatchText', linkId]`, `staleTime: Infinity`, `retry: false`, POST under the hood), and `METRICS.IMPORT_RULE_AI_MATCH_USED: 'appImportRuleAiMatchUsed'`.
- Behaviour in edit mode with no `existing` rule: enabled only when `aiEnabled()` and online. The field starts with `suggestMatchValue(link.externalPayee)`. When the query answers a non-empty `matchText` and the user has not typed (a `touched` flag set in the condition's `onChange`), set `matchField`/`matchValue` from it. When the rule is saved with the value still equal to the AI's, fire the metric.

- [ ] **Step 1: Write the failing tests**: `uses the AI match text when it arrives`; `keeps what the user typed when the AI answer arrives later`; `never calls suggest-match-text when AI is off`; `fires the AI-match metric when saved unchanged`.
- [ ] **Step 2: Run them.** `cd web && pnpm vitest run src/features/imports/RulePromptDialog.test.tsx`. Expected: FAIL.
- [ ] **Step 3: Implement it.**
- [ ] **Step 4: Run.** `pnpm test && pnpm lint`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat: Rule prompt uses the AI match text when AI is on"`

---

### Task 10: Docs and the full suite

**Files:**
- Modify: `CLAUDE.md`, `docs/regression-test-plan.md`

- [ ] **Step 1: Update `CLAUDE.md`.** The imports paragraph: 32 routes, add the four new names and a sentence on suggestions (owner-only, ongoing only, one open per merchant, dismiss not remembered). Replace the "The optional `Completer` … retained for future AI features" sentence. The `ECONUMO_AI_DSN` consumers bullet: import suggestions (the transaction details listed in the spec, the owner's category and payee names, language, timezone) and the match text. The `AI_ENABLED` note: the rule prompt uses it.
- [ ] **Step 2: Update the regression plan.** Add items: a suggestion appears for an Apple Pay tap at a new merchant (AI on); none without AI; none for the first SimpleFIN sync's older rows or the card-mapping replay; a partner's tap goes to the owner only; repeat taps join one suggestion; saving fills the transactions and asks only when others would change; Dismiss, then the next tap suggests again; the AI match text in the edit dialog. Mark the Inbox items 📱.
- [ ] **Step 3: Run the full suite.** `make test` (needs PostgreSQL; `make test` provisions it). Expected: PASS, including `enginecompare`, `test-repo-pgsql`, `upgradetest` (the new migration versions above every released one) and the web suite.
- [ ] **Step 4: Commit.** `git commit -m "docs: AI import rule suggestions"`
