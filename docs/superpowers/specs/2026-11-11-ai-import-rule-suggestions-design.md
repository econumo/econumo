# AI rule suggestions for imported transactions

Date: 2026-11-11. Status: draft, awaiting approval.

## Intent

An Apple Pay tap (Apple Wallet import) or a SimpleFIN row usually arrives with no category
and no payee. Today the user opens the transaction, fills both in, and the rule prompt
(`web/src/features/imports/RulePromptDialog.tsx`) turns that edit into an import rule, so
the next tap from that merchant is classified automatically. The first tap from every
new merchant still costs a manual edit.

When the server has an AI provider (`ECONUMO_AI_DSN`), the AI should do that first step:
look at a new imported transaction no rule classified, and when it is confident, propose
a **rule** (match text, category, payee) in the Inbox. The account owner taps it, checks
it in the rule editor, saves, and the rule fills that transaction and every later one from
the merchant. The AI never writes to a transaction by itself.

The AI also improves the match text the rule prompt suggests today. The heuristic
`suggestMatchValue` (`web/src/lib/importMatch.ts`) often leaves noise in, while the AI can
pick out the merchant part ("SQ *BLUE BOTTLE COFFEE #0423 SAN FRANCISCO CA" becomes
"BLUE BOTTLE COFFEE").

## Goals

- With AI configured, ongoing imports produce rule suggestions in the account owner's Inbox
  for transactions no rule classified.
- A suggestion only names categories and payees the account owner already has, only when
  the AI is confident, and only a category that fits the transaction's type.
- Saving a suggestion creates an ordinary import rule through the existing rule editor and
  applies it.
- The rule prompt gets a better match text when AI is configured, in both the suggestion
  flow and the existing edit flow.
- The rule prompt's "apply to existing" question is asked only when there is more than one
  transaction to apply to (a change to today's flow too).

## Non-goals

- Without AI nothing changes: no suggestions, no new UI, the heuristic match text.
- No suggestions for the initial import (see "Ongoing only").
- No new payees or categories. The AI picks only from existing ones.
- No change to whose rules apply to shared accounts (still the account owner's, #342), and
  no shared or per-account rules.
- No MCP surface.
- No tag or label suggestions.

## Anti-goals (a failure even if it works)

- The AI writing a category or payee onto a transaction without the user saving a rule.
- An Inbox flooded with hundreds of suggestions after connecting a bank.
- A rule saved from a suggestion that matches nothing, because the AI made up a match text.
- An import (an Apple Pay push, a sync) slowed down or failed by the AI.
- A suggestion naming another user's categories or payees, or shown to someone other than
  the account owner.

## Decisions made with the owner

1. **Suggestions, not writes.** Nothing changes on a transaction until the user saves the
   rule.
2. **Existing payees and categories only.**
3. **Ongoing transactions only.** The initial import is skipped.
4. **Inbox interaction.** Tapping a suggestion opens the rule editor prefilled with the
   AI's match text, category and payee. Saving creates the rule (never silently, always
   through the editor) and applies it. The row's only other control is **Dismiss**.
5. **The apply question.** After saving, the editor asks whether to apply the rule to
   existing transactions only when more than one would be changed. With one it applies
   without asking. The existing edit flow changes the same way.
6. **The AI's match text in the existing edit flow too**, when AI is configured.
7. **Only the account owner sees a suggestion.** Rules stay per user. A tap a partner imports
   into the owner's account produces a suggestion for the owner, never for the partner (the
   owner's rules are the ones that classify that account).

## How it works

### Which transactions qualify

A ledger row (`import_transaction_links`) qualifies when all of these hold:

- the import **created** its transaction (`MatchCreate` in `place`). Adopted transactions
  (the matcher matched an existing one), amount corrections and pending→posted updates do not
  qualify;
- after the account owner's rules ran, the transaction still has no category or no payee,
  and the row is not a transfer;
- it is **ongoing** (below);
- the account owner's access level is not `readonly` (*builder's call*: a lapsed trial costs
  AI calls for suggestions they cannot save, because saving a rule is a POST and gets a 402).

**Ongoing only.** *Builder's call, not confirmed by the owner:* a row is ongoing when both hold:
- it was placed by a live import: an Apple Wallet push (`IngestAppleWallet`) or a SimpleFIN sync
  (`Sync`). Replays never qualify: the queue replayed by `link-account` (initial mapping, remap,
  ignore→import), `retry-event`, and `import-queued-event` (there the user filled the category and
  payee in themselves and no rules run);
- its posted date is no more than 7 days before its `imported_at`, so a SimpleFIN backfill of
  history (or a first sync) suggests nothing older than a week. The 7 days allow for posting lag.

The mapping's `created_at` is not used: it never moves on a remap, so a replayed backlog would
count as ongoing.

### The AI job

- It runs after the import's DB transaction commits, never inside it: one background job per
  Apple Wallet push, and one per SimpleFIN run (each account of the run, batched). No other path
  starts one. It uses the
  onboarding job pattern (`internal/onboarding/service.go` `runJob`): goroutine, fresh context,
  timeout (*builder's call*: 60 s), panics recovered, an injectable runner so tests run it
  inline, a WARN with `user_id` + `err_type` only on failure, never the prompt or reply.
  Jobs are not drained on shutdown, so a restart can lose one. That's accepted.
- The job runs per **account owner**: a run whose rows land in accounts of different owners
  makes one request per owner, under that owner's id, with their categories, payees, language
  and timezone.
- It counts against a per-user cap in the shared auth limiter (*builder's call*: a new scope,
  30 jobs per rate-limit window, a constant, no new env var). Over the cap, no suggestions.
- **Reuse before asking.** Before calling the AI, a row whose text an open suggestion of the
  same owner already matches (the suggestion's match text, rule matching semantics, `contains`
  on the same field) is attached to that suggestion without an AI call. Dismissed suggestions play no part.
- At most 50 rows per request (*builder's call*). A larger batch keeps the first 50 by posted
  date, newest first, and drops the rest. Ongoing-only keeps batches small.

**What it sends**, per row: an opaque row key, the external payee, the external description,
the amount, the currency, income or expense, the posted date, the account name. For the owner:
their non-archived categories (id, name, type), their non-archived payees (id, name;
*builder's call*: at most 500, sorted by name, since there is no usage data to rank them by;
the order keeps the capped set stable from one request to the next), language and timezone. Never email
addresses, other users' data, or balances.

**What it asks for**, per row it is confident about:
`{"rows":{"<key>":{"matchText":"...","categoryId":"...","payeeId":"..."}}}`, leaving out a field
(or a whole row) it is not confident about. The system prompt says to include a field only when
it is highly confident, to pick only from the listed ids, to choose a category of the row's type,
and to pick as `matchText` the shortest substring of the payee (or the description when the
payee is empty) that identifies the merchant, copied exactly.

*Builder's call:* confidence is expressed by leaving fields out rather than by a numeric score.
A score from an LLM is not calibrated, while the prompt plus validation does the same filtering
more simply.

**Validation.** The server keeps only what passes:
- `categoryId` is one of the owner's non-archived categories and fits the row's type
  (`categoryFits`); `payeeId` is one of the owner's non-archived payees;
- `matchText`, after `NormalizeText`, occurs case-insensitively in the normalized payee (field
  `external_payee`), or in the description when the payee is empty (field `description`).
  Otherwise the row is dropped, because a rule needs a working match text;
- a row with neither a valid category nor a valid payee is dropped;
- reuse (attaching to an open suggestion without asking) also checks that the suggestion is of
  the row's type and fills a field the row lacks, else the row goes to the AI. A suggestion's
  category or payee the owner has since archived or deleted counts as unset, and a suggestion
  left with neither takes no rows (the list hides it, so they would be lost);
- a field the transaction already carries is dropped (a rule may have set one of the two).

**One open suggestion per merchant (decided by the owner).** The owner never gets a new
suggestion similar to one still open (not accepted, not dismissed). The remaining rows are
grouped by (owner, match field, normalized match text). A group whose match text equals that of
an open suggestion of the same owner, or that an open suggestion's match text matches, or whose
match text matches the open suggestion's (one contains the other), joins that suggestion and
keeps the open suggestion's category and payee, even when the AI proposed different ones this
time, provided the open suggestion is of the group's type; this is decided per row: a row joins
only when the open suggestion fills a field that row lacks, and the group's other rows become a
new suggestion under the AI's classification. A join locks the suggestion's row first;
one deleted meanwhile (the list read's cleanup) sends its rows to the AI, or, after the call, into
a new suggestion. One owner's jobs run one at a time in the process, so two imports landing
together cannot each open a suggestion for the same merchant. Within one reply, rows sharing a match text
but given different categories or payees take the classification of the most rows (ties: the
newest row).

### Storage

A new table pair, migrated on both engines (version above every released migration):

- `import_suggestions`: `id`, `user_id` (the account owner), `match_field`, `match_text`,
  `category_id` (nullable), `payee_id` (nullable), `transaction_type` (`expense` / `income`, the
  group's type; a payee-only suggestion has no category to tell it), `status` (`open` /
  `dismissed` / `accepted`), `created_at`, `updated_at` (moved when rows join).
- `import_suggestion_links`: `suggestion_id`, `link_id` (the ledger row), primary key on both.

*Builder's call:* deleting a category or payee leaves the suggestion. The read below drops its
invalid fields, and a suggestion left with neither is not listed.

### Reading suggestions

`GET /api/v1/import/get-suggestion-list` returns the caller's open suggestions, each with
`id`, `matchField`, `matchText`, `categoryId`, `payeeId`, and its still-pending transactions
(transaction id, account id, amount, currency, date, external payee). The list is computed at
read time: a transaction is still pending when it is live, still on an account the caller owns,
and still empty in at least one field the suggestion sets. A suggestion with no pending
transaction left, or with no valid field left, is not listed. So a manual edit, a rule applied,
a deletion or an unmapped account clears it with no extra write path. *Builder's call:* such
suggestions are deleted lazily on read, or left for a periodic purge.

### Inbox

A new Inbox section, **"Suggested rules"** (`inbox.sections.suggested_rules`), fed through
`useInbox()` and counted in the badge. It sits right after "To review" (*builder's call*). Each
row shows the match text, "→ Category · Payee" (whichever are set), and "N transactions" with
the latest amount and date. The row has one button, **Dismiss**. Tapping the row opens the rule
editor. Writes are disabled offline (`useOnline`), as for every other write.

The job writes after the import has answered, and an Apple Wallet push never reaches the device,
so the list is stale after 60 s and refetches on focus; after a sync that imported rows, with AI
on, it is refetched again at 15 s and 60 s. A failed list read shows the Inbox's import error
(Retry refetches it), except a 404 from an older server without the route.

### Saving a suggestion

Tapping opens the rule editor (the existing `RulePromptDialog`, given a suggestion instead of a
link + diff, *builder's call*) prefilled with `contains` on the suggestion's match field and
match text, the category and the payee. The user may change anything, as in today's prompt.
**Save**:

1. creates the rule (`create-rule`);
2. accepts the suggestion: `POST /api/v1/import/accept-suggestion {id, ruleId}` fills the
   rule's category and payee into the **empty** fields of the suggestion's pending transactions
   and deletes it (no `accepted` row is kept). This is its own apply path, not `apply-rule`: `apply-rule` skips any
   row the user touched (`edited()`), which would skip a row whose payee was filled by hand and
   whose category is still empty. A field the user set is never overwritten. A transaction the
   saved rule no longer matches (the user edited the match text), or whose type the rule's
   category does not fit, keeps that field empty. The ledger's `applied_*` snapshot moves only for
   the fields accept wrote, so a field the user set by hand still reads as edited and a later
   apply-rule (without "include edited") leaves it alone;
3. previews the rule over all the owner's imports (`preview-rule`, scope `all`, run after the
   accept so the suggestion's own rows no longer count as matched). If it would change at least
   one more transaction, it asks "Apply to N imported transactions?" (the existing apply step,
   with its "include edited" control). Otherwise the dialog closes.

*Builder's call on reading decision 5:* "more than one" counts the transactions the rule would
change. The suggestion's own transactions are always applied, since saving the suggestion is
the user's acceptance of them; the question covers only the others.

Cancel leaves the suggestion open.

### Dismiss

`POST /api/v1/import/dismiss-suggestion {id}` takes it out of the Inbox. **Dismissal is not
remembered (decided by the owner):** a later tap from the same merchant goes to the AI as usual
and may bring a fresh suggestion. *Builder's call:* a dismissed suggestion is deleted with its
link rows (or kept as `dismissed` if that is simpler; nothing reads it).

### Better match text in the edit flow

With AI configured, when the rule prompt opens after an edit of an imported transaction,
`POST /api/v1/import/suggest-match-text {linkId}` (owner of the link's account only, rate-limited
like `preview-rule`, *builder's call*) asks the AI for the merchant substring of that row's
payee or description, validated as above, with a 10 s timeout. It answers `{matchField,
matchText}` or an empty `matchText` on any failure. The dialog shows the heuristic value at
once and replaces it with the AI's when it arrives, unless the user has already typed in the
field, or until Save has started. The SPA calls it only when the served `AI_ENABLED` is true,
never for a link a rule classified (that prompt updates the rule and keeps its match), and only
once the rule list has loaded.

### The apply question in today's flow

In `RulePromptDialog`, the run-scoped step ("apply to N in this import") and the import-wide
step ("apply to N across all imports") each appear only when their count, not counting the
transaction the user just edited, is at least one. Otherwise the step is skipped: the rule is
saved, the edited transaction already has its values, and the dialog closes (or moves on to the
next step that has something to apply).

### Analytics

New `METRICS` keys fired at the success points: a suggestion saved as a rule (with whether the
user changed the match text, category or payee), a suggestion dismissed, and the AI match text
used in the edit flow (fired when the rule is saved with the AI's text unchanged).

### Docs and tests

- `CLAUDE.md`: the imports section (new routes, the route count), the `ECONUMO_AI_DSN` consumers.
- `docs/regression-test-plan.md`: suggestion appears for an Apple Pay tap at a new merchant; not
  for the initial replay; not for a partner; save, dismiss; the apply-question change; the AI
  match text in the edit flow; nothing without AI.
- Every new route gets an `apiparity` scenario and golden. `revisionguard` must pass
  (accept-suggestion writes transactions through the existing update path).
- Tests with a recording completer (`internal/server/onboarding_steps_test.go` pattern) for:
  validation (invented ids, wrong-type category, match text not in the text, another user's
  payee), grouping and reuse, no duplicate of an open suggestion, a fresh suggestion after a dismiss, ongoing-only, owner-only on a shared account,
  readonly skip, AI off.
- i18n keys in all 11 catalogues.

### AI turned off later

*Builder's call:* open suggestions stay listed and can still be saved or dismissed (they need no
AI). No new ones appear, and the edit flow goes back to the heuristic match text.

## Left to the builder

Every item marked *builder's call* above, plus: the exact prompts, the job timeout, the Inbox
row layout within the existing section style, and whether `accept-suggestion` and the rule
creation become one request.
