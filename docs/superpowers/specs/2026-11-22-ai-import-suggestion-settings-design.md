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
   **Assign category and payee automatically** and **Suggest rules**.
2. Automatic assignment writes straight onto the transaction (no Inbox step), empty fields only.
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
  per-row category and payee; it writes them through the transaction update path, records them
  in `import_transaction_suggestions` (status `filled`), and keeps rule suggestions alive
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
- AiDialog labels: "Suggest rules" and "Assign category and payee automatically" (new key
  text in all 11 catalogues). Shown only while "Use AI features" is on.
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
payee, *my call*: the user archived it on purpose).

### Writing assignments (switch "assign" on)

Inside the job's post-AI write transaction, for each valid row (same validation as today:
match text placeable, category of the right type), under the owner's job lock:

1. Lock the transaction row (`LockTransactionRow`), re-read it, skip it if it is gone, changed
   type, or no longer empty in the fields to fill.
2. Create the new payee if the row needs one (through a new imports port
   `PayeeCreator.CreatePayee(ctx, ownerID, name) (model.ImportNamed, error)`, wired in
   `internal/server` to the payee service like `TransactionImportPayees.CreatePayee`). One
   payee per normalised name per run (rows of the same merchant share it).
3. Write only empty fields through `txns.UpdateTransaction` (revision bump, sort keys untouched)
   and move the ledger's `applied_category_id` / `applied_payee_id` snapshot for the fields
   written (like a rule's write: a later apply-rule may replace them; `applied_rule_id` stays
   unset).
4. Record `import_transaction_suggestions` (link, owner, the category and payee written, type,
   status `filled`).

Rows the pre-AI reuse step attached to an open rule suggestion get that suggestion's
category/payee assigned the same way (no AI call), limited to the fields the row lacks.

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

- `import_transaction_suggestions` (migration `20261122000000`, unreleased): unchanged schema;
  only `filled` rows are written now. `open` stays a valid status for rows written by earlier
  builds of this branch (none in production).
- `import_suggestions` gains `payee_name TEXT DEFAULT NULL`, in the same unreleased migration
  `20261122000000` on both engines (*my call*: nothing outside this branch ran it).

## Removed from revision 1

- Routes `get-transaction-suggestion-list`, `fill-transaction-suggestion`,
  `dismiss-transaction-suggestion`, their handlers, DTOs, apiparity scenarios and goldens.
- The Inbox section "Imported transactions", its queries, refresh hooks, metrics keys, the
  preview sheet's "Suggested" marks, i18n keys and tests.
- Fill in's "counts as a hand edit" rule.

The list's read-time cleanup is not needed: nothing is open any more.

## Interaction with Suggested rules

`pendingSuggestionRows` keeps a row whose transaction has no empty field the rule suggestion
sets when its link has a `filled` record and every field the suggestion sets equals the
transaction's current value (revision 1's agreement check, unchanged). A suggestion's
`payee_name` counts as "sets the payee"; it agrees when the transaction's payee has that name
(normalised). Accepting such a suggestion has nothing to fill on those rows and releases them.

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
