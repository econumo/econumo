# AI import suggestions: settings and per-transaction suggestions

Date: 2026-11-22. Status: draft, awaiting approval.

Builds on `2026-11-11-ai-import-rule-suggestions-design.md` (the AI rule suggestions job,
#473). Everything that spec says still holds unless this one changes it.

## Intent

Today the AI import job can only propose **rules** ("Suggested rules" in the Inbox). The owner
wants two things, both controlled from the AI settings:

1. A choice of what the AI suggests for imported transactions: rules, a category and payee
   for each transaction, both, or neither.
2. A new kind of suggestion: for one imported transaction, "fill in this category and payee",
   accepted with one tap, without creating a rule.

The AI still never writes anything by itself. Every change to a transaction goes through a
tap in the Inbox.

## Goals

- Two switches under "Use AI features": **Suggest rules** and **Suggest category and payee**.
- A new Inbox section, **Imported transactions**, listing imported transactions the AI has a
  suggestion for: one row per transaction, not grouped, newest first, across accounts,
  drawn like the Due section's rows (a full `TransactionRow`), with the suggested category
  and payee shown in place and a "Suggested" tag.
- Tapping a row opens the transaction preview sheet with two actions: **✕** (dismiss) and
  **Fill in**. Both act on that one transaction only.
- "Suggested rules" stays its own section, unchanged, above "Imported transactions".
- One AI pass feeds both sections: no extra AI calls.

## Non-goals

- No automatic writes. Auto-assigning and auto-creating rules were discussed and dropped in
  favour of suggestions only (decided by the owner).
- No change to which transactions qualify: still ongoing live imports only, the account
  owner only, never readonly (see the rule-suggestions spec, "Which transactions qualify").
- No change to the prompts or to the data sent to the AI. The data allowlist and its guard
  tests are untouched.
- No change to how "Suggested rules" behaves, except the one interaction below.
- No MCP surface. No offline queueing: Fill in and ✕ need the server, like Dismiss today.
- No grouping, bulk fill or "fill all from this merchant". Covering a merchant at once is
  what a rule is for.

## Anti-goals

- A Fill in that overwrites a category or payee the user set.
- The AI called more often than today, or called at all when both switches are off.
- "Suggested rules" regressing: its grouping, reuse, accept and dismiss behaviour was
  settled over several review rounds and must keep passing its tests.
- A transaction suggestion shown to anyone but the account owner, or naming another user's
  categories or payees.

## Decisions made with the owner

1. Suggestions only, no automatic writes.
2. Two independent on/off switches: Suggest rules, Suggest category and payee.
3. Separate Inbox sections: "Suggested rules" (unchanged) and "Imported transactions"
   (new, ungrouped, one row per transaction, newest first).
4. Rows look like the Due section's rows; tapping opens the preview sheet with ✕ and Fill in.
5. Fill in and ✕ act on one transaction only.
6. Approach B: a separate table for transaction suggestions, written by the same AI job.
7. Values accepted with Fill in are protected like a hand edit: a later rule apply skips them
   unless "include edited" is ticked.

## Approaches considered

- **A. Reuse the rule-suggestion storage** (a per-link state on `import_suggestion_links`,
  dismissing a rule hides instead of deletes). Less new code, but it rewrites the rule
  suggestions' read, dismiss and cleanup logic, and each row's values would be the
  merchant group's, not the AI's answer for that transaction.
- **B. A separate table, same AI job (chosen).** Independent lifecycles that match the two
  sections, per-row values straight from the AI reply, rule-suggestion code mostly untouched.
  Costs a table and three routes.
- **C. A second AI job.** Cleanest separation, but doubles AI calls per import. Rejected.

## Settings

### Storage

Two new user options, read like `ai` and `analytics` (absent or empty = on, so no backfill):

- `ai_suggest_rules`: `"1"` / `"0"`.
- `ai_suggest_fill`: `"1"` / `"0"`.

Defaults: both on, for existing and new users (*my call, stated to the owner*: both only
produce suggestions, and rules-on keeps today's behaviour).

### API

`POST /api/v1/user/update-ai-suggestions {suggestRules: bool, suggestFill: bool}`, both
required, returns the refreshed user like `update-ai`. On the read-only allowlist next to
`update-ai` (*my call*: turning a preference off must work for a lapsed trial too, and it
sends nothing anywhere). An apiparity scenario and golden.

### Effects

- `Use AI features` off: as today, nothing is suggested and all open suggestions are deleted,
  now including transaction suggestions (`ClearSuggestions` clears both).
- A switch off: its section is **hidden, not deleted**; turning it back on shows what is still
  pending. The job stops creating that kind while it is off.
- Both off: the job makes no AI call and writes nothing (checked with the other owner facts,
  before the rate-limit slot is taken).
- `OwnerFacts` gains the two switches. The job re-reads them where it re-reads the opt-out
  today (before each AI call and inside the write transaction).
- The better match text in the rule editor (`suggest-match-text`) still follows only the main
  switch.

### UI

`AiDialog` keeps the privacy promise, the description and the main switch. Below it, shown
only while the main switch is on, a small group headed "Imported transactions" with the two
switches. Each switch is disabled offline (`useOnline`). The SPA reads both options from the
user like `userAiOn`.

## The AI job

The job, its qualification rules, the reuse step, the two-step prompts, validation and the
rate cap stay as they are. What changes:

- **Rules off:** no `import_suggestions` row is inserted or joined, and the reuse step
  (attaching rows to an open rule suggestion without an AI call) is skipped. Rule
  suggestions left open from before stay in the table, hidden.
- **Fill on:** after validation (`validateReply`), every valid row also gets a transaction
  suggestion carrying **that row's own** validated category and payee (not the group's
  majority classification). Rows the reuse step attached to an open rule suggestion without
  an AI call get a transaction suggestion with that rule suggestion's category and payee
  (still valid per `liveOpen`), limited to the fields the row lacks.
- Validation is unchanged, including the requirement for a valid match text (*my call*: a row
  whose match text the AI could not place is a sign it did not identify the merchant, and
  keeping one validation path avoids touching the delicate code). A field the transaction
  already carries is dropped, as today; a row left with neither field gets no suggestion.
- Transaction suggestions are written in the same DB transaction as the rule suggestions, and
  `undoIfOptedOut` deletes them too.
- One transaction suggestion per ledger row: a second write for the same link is ignored.

## Storage

New table on both engines, migration version above every released one (currently above
`20261116000000`):

`import_transaction_suggestions`
- `link_id` TEXT PRIMARY KEY, FK to `import_transaction_links (id)` ON DELETE CASCADE.
- `user_id` TEXT NOT NULL, the account owner, FK to `users (id)` ON DELETE CASCADE.
- `category_id` TEXT NULL, `payee_id` TEXT NULL.
- `transaction_type` TEXT NOT NULL (`expense` / `income`).
- `status` TEXT NOT NULL: `open` or `filled`.
- `created_at`, `updated_at` DATETIME NOT NULL.
- Index on `(user_id, status)`.

Dismissed suggestions are deleted (nothing reads them; a link only ever qualifies once, at
its live import, so it is never suggested again). `filled` rows are kept for the
rule-suggestion interaction below and go when the link goes.

## Reading: `GET /api/v1/import/get-transaction-suggestion-list`

Returns the caller's `open` transaction suggestions, newest first by the transaction's date
(then id), computed at read time like the rule-suggestion list:

- the transaction is live, on an account the caller owns, still of the suggestion's type;
- the suggestion's category (if any) is a non-archived category of the caller that fits the
  type, its payee (if any) a non-archived payee of the caller; an invalid field counts as
  unset;
- at least one remaining field would fill an empty field of the transaction.

A suggestion failing these is not listed, and is deleted lazily (best effort, as the rule
list does). Hidden (not deleted) when the caller is readonly or `ai_suggest_fill` is off;
empty when AI is off.

Each item: `id` (the link id), `transactionId`, `accountId`, `type`, `amount`, `currency`,
`date`, `description`, `externalPayee`, `categoryId`, `payeeId`. Enough for the SPA to draw a
`TransactionRow` with the suggested fields in place without the local transaction store
(*builder's call* whether to prefer the stored transaction when present).

## Fill in: `POST /api/v1/import/fill-transaction-suggestion {id}`

In one DB transaction, under a lock on the suggestion row:

1. Re-read the suggestion; gone, not `open` or not the caller's = not found (the client treats
   a not-found as done, like accept and dismiss today).
2. Re-check the transaction as the read does. Fill only the **empty** fields: the category if
   the transaction has none and the category is valid and fits the type, the payee if the
   transaction has none and the payee is valid. Never overwrite.
3. Write through `txns.UpdateTransaction` (revision bump, `revisionguard` passes) and leave
   the ledger's `applied_*` snapshot alone, unlike `AcceptSuggestion` (decided by the owner):
   values accepted with Fill in are the user's choice, protected like a hand edit. The row
   then reads as edited (`edited()` in `ruleapply.go`), so a later `apply-rule` skips it unless
   "include edited" is ticked, and `preview-rule` counts it in `alreadyEdited`.
4. Set the status to `filled`. `reqctx.PinIdempotency`.

Returns `{updated: 0|1}`. If nothing was left to fill, the suggestion is deleted instead and
`updated` is 0.

## Dismiss: `POST /api/v1/import/dismiss-transaction-suggestion {id}`

Deletes the caller's open suggestion. Not found counts as done on the client. Returns the
refreshed list, like `dismiss-suggestion`.

## Interaction with Suggested rules

Today a rule suggestion lists only transactions still empty in a field it would set, so it
disappears once they are filled. A transaction filled **through Fill in** (its
transaction suggestion is `filled`) keeps counting as pending for its rule suggestion while
the transaction **agrees** with it: every field the rule suggestion sets equals the
transaction's current value (decided with the owner: accepting the AI's values means its
guess was right, so the rule is still worth offering). A transaction's own suggestion can
differ from its rule suggestion (the rule suggestion takes the merchant's majority answer, or
an open suggestion's values), so a fill that disagrees, or a later hand change that makes the
transaction disagree, counts against the rule suggestion (*my call*: accepting Restaurants
argues against a Coffee rule). No record of which fields the fill wrote is needed. A
transaction the user filled by hand without Fill in counts against the rule suggestion as
today.

Accepting the rule suggestion then has nothing to fill on such rows. It still walks and
releases them; the rule applies to later imports as usual.

Making a rule fills its transactions, which removes them from "Imported transactions" through
the read-time check (no extra write path). Dismissing a rule suggestion leaves its
transactions' suggestions in place.

## Inbox

- New section **Imported transactions** (`inbox.sections.imported_transactions`), right after
  "Suggested rules", fed through `useInbox()` and counted in the badge (one per row). Shown only
  while the server has AI configured, the user's AI is active and `ai_suggest_fill` is on.
- "Suggested rules" is shown only while `ai_suggest_rules` is on.
- Each row: a `TransactionRow` of the imported transaction with the suggested category and
  payee overlaid, the account shown (cross-account list, like Due), and a "Suggested" note.
- Tapping opens the transaction preview sheet (`ViewTransactionDialog`, as
  `ViewRecurringDialog` does) with the suggested values shown and the footer `[ ✕ ] [ Fill in ]`
  (small, wide), both disabled offline. Fill in closes the sheet on success.
- Refresh: the list refetches on the same schedule as the rule suggestions
  (`suggestionRefresh.ts`), and the transaction caches are invalidated after a fill so the
  revision sync downloads the changed account.
- A 404 from an older server without the route is treated as an empty list, like the rule
  list.

## Analytics

New `METRICS` keys fired at the success points: a transaction suggestion filled, a
transaction suggestion dismissed, and the AI suggestion settings changed (with which switch).

## Docs and tests

- `CLAUDE.md`: imports route count (33 → 36), the AI rule-suggestions bullet mentions the
  transaction suggestions and the two switches.
- `docs/regression-test-plan.md`: both switches (each section hides, both off = no new
  suggestions), an Apple Pay tap at a known merchant appears in Imported transactions, Fill in
  fills only empty fields, ✕ removes it, a filled row keeps its rule suggestion only when they
  agree, a filled row is skipped by a later apply unless "include edited", making a rule clears
  the matching rows, AI off clears everything.
- apiparity scenarios and goldens for the four new routes.
- Go tests with the recording completer: per-row values (not the group's), rules off writes no
  rule suggestion and skips reuse, fill off writes no transaction suggestion, both off makes no
  AI call, fill never overwrites, readonly and AI off, opt-out undo removes both kinds, the
  filled-row interaction with the rule list, owner-only on a shared account.
- Frontend tests for the settings dialog, the new section, the sheet actions, offline
  disabling; `metrics-coverage` and the offline coverage lists.
- i18n keys in all 11 catalogues.

## Left to the builder

Every *builder's call* above, plus: exact copy and keys, the "Suggested" note's styling, how
the preview sheet marks suggested values, whether `update-ai-suggestions` is one route or the
two options ride on `update-ai`, and the internal shape of the job changes as long as the
rule-suggestion behaviour and tests are preserved.
