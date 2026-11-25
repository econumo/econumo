# AI import settings: automatic assignment and rule suggestions with new payees

Date: 2026-11-22, revised 2026-11-23. Status: approved (revision 2), implemented in PR #501.

Builds on `2026-11-11-ai-import-rule-suggestions-design.md` (the AI rule-suggestion job, #473).
Everything that spec says still holds unless this one changes it.

**Revision 2 (2026-11-23).** The first version added per-transaction *suggestions* (an Inbox
section "Imported transactions" with Fill in). After seeing it running, the owner found it too
much: the category and payee are now **assigned automatically**, and rule suggestions may
name a **new payee** for a well-known merchant. The Inbox section, its three routes and the
preview sheet are removed; the table stays as the record of what the AI assigned.

## Intent

An Apple Pay tap or a SimpleFIN row usually arrives with no category and no payee. With AI
configured, the owner wants Econumo to fill both in by itself for new imports, and to keep
proposing rules so a merchant is covered for good. A merchant the user has never had as a payee
should still get one when it is a well-known brand.

## Decisions made with the owner

1. Two switches under "Use AI features", both **on** by default:
   **Auto-categorize** (assign category and payee) and **Suggest rules**, under a heading
   "Imports" (shortened 2026-11-24 at the owner's request).
2. Automatic assignment writes straight onto the transaction (no Inbox step), empty fields only,
   and only when the transaction still has **no category** after the owner's import rules ran
   (2026-11-24, decided by the owner): a transaction a rule or the user categorised is left
   alone even if its payee is empty. Rules always run first, at import time.
3. A missing payee is **created** only when the merchant is a major, well-known brand
   (Starbucks, Uber, Whole Foods); an unknown local shop gets a category and no payee.
4. Suggested rules may carry a new payee (same well-known-only limit, *my proposal, accepted*);
   it is created only when the user saves the rule.
5. AI-assigned values count like a rule's write: a rule applied later may replace them
   (*my proposal, accepted*).
6. Rule suggestions keep counting an auto-assigned transaction while its values agree with them.

## Goals

- With "Assign automatically" on, every new live imported transaction no rule classified gets
  the AI's category (an existing category of the right type) and payee (existing, or new when
  well-known), in the background after the import answers.
- Suggested rules still appear for those merchants even though their transactions are filled.
- A rule suggestion can propose a payee that does not exist yet; saving creates it.
- No extra AI calls: one AI pass per import run serves both.

## Non-goals

- No Inbox item, badge or review list for automatic assignments, and no "AI" marker on
  transactions.
- No change to which rows qualify: ongoing live imports only, account owner only, never
  readonly (rule-suggestions spec, "Which transactions qualify").
- No new categories are ever created. No tags, labels or descriptions.
- No MCP surface.

## Anti-goals

- Overwriting a category or payee the user (or a rule) set.
- A new payee for an unknown shop, or two payees for one merchant.
- An import slowed or failed by the AI (the job still runs after the import commits).
- Any data outside the AI allowlist reaching the model.

## Approaches considered

- **A. Write assignments inside the existing job (chosen).** The job already validates a
  per-row category and payee; it writes them through the transaction update path, marks the
  link `assigned_by = 'ai'`, and keeps rule suggestions alive
  through the existing agreement check. Small change on top of revision 1.
- **B. A second job just for assignment.** Doubles AI calls. Rejected.
- **C. Assign through auto-created rules.** Rejected earlier by the owner (suggestions for rules,
  no automatic rules).

## Settings

- User options `ai_suggest_rules` (unchanged) and `ai_suggest_fill`, now meaning **assign
  automatically**. Values `"1"`/`"0"`, absent or empty = **on** (both). The option name stays
  `ai_suggest_fill` (unreleased, but already wired through route, goldens and SPA; *my call*).
- `POST /api/v1/user/update-ai-suggestions {suggestRules, suggestFill}` unchanged, on the
  read-only allowlist.
- AiDialog: heading "Imports", switches "Suggest rules" and "Auto-categorize" (all 11
  catalogues). Shown only while "Use AI features" is on.
- Both off: the job makes no AI call. "Use AI features" off: nothing runs; open rule
  suggestions are deleted as today.

## The AI job

### Prompt: the well-known flag

Step 1's reply rows gain `wellKnown` (boolean): true only for a major chain or brand most
people in the request's language would recognise. The system prompt (`prompts/suggest.md`)
says so, with examples on both sides (Starbucks / Uber / Whole Foods / Яндекс Такси true; a
single local café, a person, a payment processor naming no merchant false). Reply schema in
`internal/imports/prompts.go` gains the field. No new input data: the data allowlist and the
prompt builders' key-set guard tests are unchanged.

### Payee resolution per row

Unchanged: exact match against existing payees, then step 2 (candidates) picks an existing one.
New: a row that ends with **no** payee, has a merchant name (`merchants[0]`, trimmed) and
`wellKnown: true` gets a **new-payee name** = `merchants[0]`. Before creating, the name is
compared (normalised as in `similarityText`) with every payee the owner has, archived ones
included; an equal name reuses that payee (unarchived ones only; an archived match means no
payee, *my call*: the user archived it on purpose). A row that had step-2 candidates gets a new-payee
name only when step 2 answered and picked none of them (a null pick or the row left out); when
step 2 failed, replied unusably or picked something that is not one of the row's candidates, the
owner may well have the merchant's payee already, so the row gets no payee rather than a
near-duplicate.

### Writing assignments (switch "assign" on)

Inside the job's post-AI write transaction, for each valid row (same validation as today:
match text placeable, category of the right type), under the owner's job lock:

1. Lock the transaction row (`LockTransactionRow`), re-read it, skip it if it is gone, changed
   type, or **has a category** (whatever its payee). A row whose transaction has a category is
   never assigned, so a rule-set category with an empty payee stays as the rule left it; such a
   row can still feed a rule suggestion as before. Once the run's rows are locked, the owner's
   categories and payees are read again: a value archived while the job waited is not written,
   and a new-payee name an archived payee now carries creates nothing.
2. Create the new payee if the row needs one (through a new imports port
   `PayeeCreator.CreatePayee(ctx, ownerID, name) (model.ImportNamed, error)`, wired in
   `internal/server` to the payee service like `TransactionImportPayees.CreatePayee`). One
   payee per normalised name per run (rows of the same merchant share it). A create the payee
   feature refuses because that exact name exists by now (created meanwhile) reuses that payee
   when it is active; when it is archived, the row still gets its category, without a payee.
3. Write only empty fields through `txns.UpdateTransaction` (revision bump, sort keys untouched)
   and move the ledger's `applied_category_id` / `applied_payee_id` snapshot for the fields
   written (like a rule's write: a later apply-rule may replace them; `applied_rule_id` stays
   unset).
4. Set the link's `assigned_by` to `'ai'` (same write as the applied snapshot).

Rows the pre-AI reuse step attached to an open rule suggestion get that suggestion's
category/payee assigned the same way (no AI call), limited to the fields the row lacks. With
"assign" on, the reuse step takes a row only when the suggestion fills EVERY field the row lacks
(its category, and its payee when empty); a row it would leave half done (a payee-only
suggestion, or a category-only one for a row with no payee) goes to the AI instead and may still
join that suggestion afterwards, like any other row. A row the reuse wrote only in part (a value
archived while it waited for the row lock) goes to the AI too; when the reuse wrote its category,
the AI may still fill its empty payee in the same run, as long as the row still carries that
category. With "assign" off, a suggestion filling any missing field takes the row, as before.

A failure on one row (a payee name the payee service rejects, a write error) skips that row
and logs a WARN with ids and `err_type` only; the rest of the run proceeds. The opt-out
re-checks stay; an opt-out (AI off) found after commit does **not** undo assignments already
written (*my call*: they are ordinary data by then, like a rule's write), but the undo still
removes the run's rule suggestions.

### Rule suggestions (switch "rules" on)

Unchanged, except a group whose rows have no payee but a well-known merchant carries
`payee_name` (the new-payee name) instead of `payee_id`. Grouping and the one-open-suggestion
rule treat a `payee_name` like a payee value (two groups with different names do not merge). A
reused open suggestion with a `payee_name` whose payee now exists (created by assignment or by
hand) reads as that payee.

When "assign" is also on, the assignment usually creates the payee in the same run; the rule
suggestion then simply carries that `payee_id`.

## Storage

All in the unreleased migration `20261122000000` (both engines; *my call*: nothing outside this
branch ran it):

- `import_transaction_links` gains `assigned_by TEXT DEFAULT NULL` (decided by the owner,
  2026-11-24): who wrote the transaction's current classification through the import system.
  `'rule'`: an import rule at import time, apply-rule, or accepting a rule suggestion.
  `'ai'`: automatic assignment. NULL: the import's own data, an adopted transaction, a queued
  event the user filled in, a skip, or anything else. Every path that writes the applied
  snapshot sets it in the same write (`setApplied` and its siblings). Backfill: `'rule'` where
  `applied_rule_id IS NOT NULL AND transaction_id IS NOT NULL` (skip rows have no transaction).
  Internal for now (no API field); it is what a future "AI"/"by rule" marker would read.
  A **hand edit** that changes the transaction's category or payee clears it to NULL, whether it
  was `'ai'` or `'rule'` (decided by the owner, 2026-11-24: the user classified it now). Hand
  edits are the user-facing writes: `update-transaction` (incl. queued offline writes) and the
  MCP update/bulk-update tools. Editing only amount, date, description or tags leaves it.
  Writes made by the importer itself (rules, apply-rule, accept, AI) set it as listed above and
  never count as hand edits. The transaction feature signals the edit through a port wired in
  `internal/server` (features never import features).
- `import_suggestions` gains `payee_name TEXT DEFAULT NULL`.
- Revision 1's `import_transaction_suggestions` table is dropped from the migration (it ended
  up recording only which links the AI filled, which `assigned_by` now says).

## Removed from revision 1

- Routes `get-transaction-suggestion-list`, `fill-transaction-suggestion`,
  `dismiss-transaction-suggestion`, their handlers, DTOs, apiparity scenarios and goldens.
- The Inbox section "Imported transactions", its queries, refresh hooks, metrics keys, the
  preview sheet's "Suggested" marks, i18n keys and tests.
- Fill in's "counts as a hand edit" rule.

The list's read-time cleanup is not needed: nothing is open any more.

## Interaction with Suggested rules

`pendingSuggestionRows` keeps a row whose transaction has no empty field the rule suggestion
sets when its link is `assigned_by = 'ai'` and every field the suggestion sets equals the
transaction's current value (revision 1's agreement check, unchanged). A suggestion's
`payee_name` counts as "sets the payee"; it agrees when the transaction's payee has that name
(normalised). Accepting such a suggestion has nothing to fill on those rows and releases them.
A row that still has an empty field the suggestion sets but carries a value that disagrees
with it is never listed, whoever classified it (decided 2026-11-24: once a hand edit clears
`assigned_by`, such a row would otherwise reappear, and accepting would write the
suggestion's other field onto it).

## Suggested rules: saving a new payee

`GET get-suggestion-list` items gain `payeeName` (string, empty when none). The Inbox row shows
"→ Coffee · Blue Bottle (new)". Tapping opens the rule editor prefilled with the payee field
showing the new name as a pending new payee. On **Save**, the SPA first creates the payee
(`create-payee`, client-minted id, so a retried save reuses it; a name that now exists reuses the
existing payee), then `create-rule` with that payee, then `accept-suggestion` as today. Changing
the payee in the editor drops the pending name. Offline: disabled as today.

## Analytics

Removed: the two transaction-suggestion keys. Kept: `appUserAiSuggestionsUpdate`. New: a rule
saved from a suggestion that carried a new payee fires the existing save event with
`newPayee: true` (no new key needed if the event accepts props; else a new key).

## Docs and tests

- `CLAUDE.md`: imports route count back to 33; the AI bullet describes automatic assignment,
  the well-known flag and `payee_name`.
- `docs/regression-test-plan.md`: replace the Imported-transactions items with: a tap at a known
  merchant is categorised and gets a payee within a minute; an unknown shop gets a category
  only; a value already set is never changed; switch off = no assignment; a rule suggestion with
  "(new)" creates the payee on save and not on dismiss; rule suggestions still listed for
  auto-filled taps.
- Go tests with the recording completer: assignment writes only empty fields and moves the
  applied snapshot; new payee only when `wellKnown`; name reuse (case/punctuation), archived
  match = no payee; one payee per merchant per run; switch off writes nothing; both off no
  call; shared account goes to the owner; rule suggestion `payee_name` round trip, merge rules,
  agreement on name; a concurrent hand edit is not overwritten.
- AI evals (`aieval` tag): cases for `wellKnown` in English and Russian, at or above 0.90.
- apiparity: `get-suggestion-list` golden gains `payeeName`; removed routes' goldens deleted
  (the guard requires no orphans).
- Frontend tests: AiDialog labels; rule row "(new)"; save creates the payee then the rule;
  retried save reuses it.

## Left to the builder

Exact prompt wording and eval cases, the rule editor's presentation of a pending new payee,
and whether the analytics flag is a prop or a key.
