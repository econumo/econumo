# AI Import Automatic Assignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace per-transaction AI suggestions with automatic assignment of category and
payee, and let rule suggestions carry a new payee for well-known merchants.

**Architecture:** Revision 2 of PR #501. The AI import job (`internal/imports/suggest_job.go`)
writes validated category/payee onto transactions itself (recorded in
`import_transaction_suggestions` as `filled`), creating a payee only for a `wellKnown` merchant.
Rule suggestions gain `payee_name`; the SPA creates that payee when the rule is saved. The
revision-1 Inbox section and its three routes are removed.

**Tech Stack:** Go, sqlc (SQLite + PostgreSQL), React 19 + TanStack Query, vitest.

**Spec:** `docs/superpowers/specs/2026-11-22-ai-import-suggestion-settings-design.md` (revision 2;
read it whole). Base: `docs/superpowers/specs/2026-11-11-ai-import-rule-suggestions-design.md`.

## Global Constraints

- Go: `export PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1`; sqlc at `~/go/bin/sqlc`.
- `.sql` ASCII-only, `;` on the query's last line; `sqlc generate` after query edits.
- Migration: edit the unreleased `20261122000000.sql` (both engines) to add
  `ALTER TABLE import_suggestions ADD COLUMN payee_name TEXT DEFAULT NULL;`. No new version.
- Options keep their names: `ai_suggest_rules`, `ai_suggest_fill` (= assign automatically); absent = on.
- Route paths: keep `update-ai-suggestions`; remove `get-transaction-suggestion-list`,
  `fill-transaction-suggestion`, `dismiss-transaction-suggestion`. Imports routes back to 33.
- AI data allowlist unchanged; reply schema gains `wellKnown` only.
- Goldens: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect, never hand-edit; delete
  orphaned goldens of removed routes. `make swagger` after DTO/route changes.
- Transaction writes go through `txns.UpdateTransaction` (revision bump); `revisionguard` must pass.
- i18n in all 11 catalogues (registers: de Sie, nl u, fr vous, pt European).
- Comments only for non-obvious why. Commit messages end with
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; stage by path.
- Local tests: touched packages and vitest files only; CI runs the rest (incl. AI evals).

## Review Focus

1. A transaction the user edited between the import and the job's write: never overwritten (lock + re-read).
2. Same merchant on several rows in one run, or a payee named "Whole Foods" vs "WHOLE FOODS" already present: exactly one payee.
3. An archived payee with the merchant's name: no new payee, nothing assigned for payee.
4. A shared account: assignments and new payees belong to the account owner, never the partner.
5. Rule suggestion with `payee_name` whose payee got created meanwhile (by assignment or by hand): reads as that payee, saving does not create a duplicate.

---

### Task 1: Remove the revision-1 transaction-suggestion surface

**Files:** `internal/imports/api/{routes.go,suggestion.go}` (3 handlers), `internal/imports/transaction_suggestion.go` (list/fill/dismiss service methods and helpers only used by them — keep anything Task 3 needs: `LockTransactionRow` repo method, `FilledTransactionSuggestionLinks`, `InsertTransactionSuggestions`), `internal/model/imports_dto.go` (their DTOs), `internal/imports/suggestion.go` (`ClearSuggestions` no longer needs to delete transaction suggestions: they are `filled` records now — keep deleting only `open` ones for leftovers), apiparity catalogue + goldens, `make swagger`; SPA: `web/src/features/imports/ImportedTransactionsSection*`, transaction-suggestion API/DTO/queries/refresh code, `useInbox` field and count, `InboxPage`, `ViewTransactionDialog` `suggested` prop, metrics keys `IMPORT_TRANSACTION_SUGGESTION_*`, i18n keys only used there (all 11 catalogues), related tests, `docs/regression-test-plan.md` items for the section.

- [ ] Remove; keep `get-suggestion-list`, accept/dismiss rule suggestion, `update-ai-suggestions` intact.
- [ ] `go build ./... && go test ./internal/imports/... ./internal/test/apiparity/ ./internal/test/i18ntest/`; web: vitest for inbox/imports/settings/transactions dialog + metrics/offline coverage, `pnpm lint`, `pnpm exec tsc -b --noEmit` — PASS.
- [ ] Commit `refactor: Drop the per-transaction suggestion Inbox section`.

### Task 2: The `wellKnown` flag in the suggest prompt

**Files:** `prompts/suggest.md`, `internal/imports/prompts.go` (reply schema), `internal/imports/suggest.go` (`suggestReplyRow.WellKnown bool \`json:"wellKnown"\``, carried onto `validSuggestion.WellKnown` and `MerchantName string` = trimmed `merchants[0]`), `internal/imports/aieval_test.go` (cases).

- [ ] Failing test `TestParseSuggestReplyWellKnown`: a reply with `"wellKnown":true,"merchants":["Whole Foods"]` yields `WellKnown` true and `MerchantName` "Whole Foods" on the valid row; missing field → false; empty merchants → `MerchantName` "".
- [ ] Prompt text: true only for a major chain/brand most people in the request's language would recognise; examples both ways (Starbucks, Uber, Whole Foods, Яндекс Такси true; a single local café, a person, "SQ *" processor with no merchant false).
- [ ] Eval cases (`aieval` tag) in en and ru for both outcomes, scored on `wellKnown`; compile with `go vet -tags aieval ./internal/imports/`. (Runs in CI's AI evals job; must stay ≥ 0.90.)
- [ ] Key-set guard tests for the prompt builders still pass unchanged.
- [ ] Commit `feat: Ask the AI whether an imported merchant is well known`.

### Task 3: Automatic assignment and `payee_name` in the job

**Files:** migration `20261122000000.sql` (both engines, add column), queries + sqlc for `import_suggestions` insert/select with `payee_name`, `model.ImportSuggestion.PayeeName *string`, repo, `internal/imports/ports.go` (new `PayeeCreator interface { CreatePayee(ctx, ownerID vo.Id, name string) (model.ImportNamed, error) }`, a Service field set in the constructor/options like the other ports), `internal/server` wiring (reuse `TransactionImportPayees.CreatePayee` or an equivalent adapter in a glue file), `internal/imports/suggest_job.go`, `suggest.go` (grouping/merge/`liveOpen` with `PayeeName`), `suggestion.go` (`pendingSuggestionRows` agreement on name; list result `PayeeName`), tests in `suggest_job_test.go` / `suggestion_test.go`.

Behaviour: spec sections "Payee resolution per row", "Writing assignments", "Rule suggestions", "Interaction with Suggested rules". Key points: assignment happens only when `SuggestFill`; lock + re-read each transaction; empty fields only; move `applied_category_id`/`applied_payee_id` for written fields (not `applied_rule_id`); record `filled`; one payee per normalised name per run; archived match = no payee; per-row failure = skip + WARN; post-commit opt-out does not undo assignments but still removes the run's rule suggestions; reuse step assigns from the open rule suggestion.

- [ ] Failing tests: `TestAssignWritesEmptyFieldsOnly`, `TestAssignMovesAppliedSnapshot` (a later apply-rule without include-edited replaces the value), `TestAssignCreatesPayeeOnlyWhenWellKnown`, `TestAssignReusesPayeeByNormalisedName`, `TestAssignArchivedPayeeMatchMeansNoPayee`, `TestAssignOnePayeePerMerchantPerRun`, `TestAssignOffWritesNothing`, `TestAssignSharedAccountGoesToOwner`, `TestAssignSkipsTransactionEditedMeanwhile`, `TestRuleSuggestionCarriesPayeeName` (rules on, assign off, well-known unknown-payee merchant → suggestion with `payee_name`, no payee created), `TestRuleSuggestionPayeeNameResolvesToExistingPayee`, `TestRuleSuggestionAgreesOnPayeeName`, `TestRuleSuggestionsStayListedForAssignedRows`.
- [ ] Implement; existing job/suggestion tests keep passing (update only where revision 1's `open` transaction suggestions were asserted).
- [ ] `go test ./internal/imports/... ./internal/server/... ./internal/test/apiparity/ ./internal/test/revisionguard/`, `go vet -tags enginecompare ./internal/imports/...`; regenerate goldens (`get-suggestion-list` gains `payeeName`), `make swagger` — PASS.
- [ ] Commit `feat: Assign category and payee automatically on import`.

### Task 4: SPA — labels and saving a rule with a new payee

**Files:** `locales/*.json` (switch label "Assign category and payee automatically"; rule row "(new)" text), `web/src/api/dto/imports.ts` (`payeeName: string` on the rule suggestion), `web/src/features/imports/SuggestionsSection.tsx` ("→ Coffee · Blue Bottle (new)"), the rule editor (`RulePromptDialog.tsx`, `suggestionRule.ts`) prefilled with the pending new payee; on Save create the payee first (`create-payee`, id minted once and kept with the dialog state so a retried save reuses it; if a payee with that normalised name exists in the cache, use it instead), then `create-rule`, then `accept-suggestion` as today; picking another payee drops the pending name. Analytics: the existing suggestion-saved event gains `newPayee: true` when it created one (add a key only if the event has no props). `docs/regression-test-plan.md` rule-suggestion items.

- [ ] Failing vitest: row shows "(new)"; Save posts create-payee then create-rule with that payee id; retried Save after a lost create-payee answer reuses the same id; existing same-name payee → no create-payee; dismiss creates nothing; AiDialog shows the new label.
- [ ] Implement; vitest touched files + metrics/offline coverage, lint, typecheck, `go test ./internal/test/i18ntest/` — PASS.
- [ ] Commit `feat: Save a suggested rule with a new payee`.

### Task 5: Docs

`CLAUDE.md` (route count 33; AI bullet: automatic assignment, `wellKnown`, `payee_name`), regression plan items per spec "Docs and tests" not yet added, spec status line. Commit `docs: AI import automatic assignment`.
