# Multi-tag mechanics: reporting labels

**Date:** 2026-08-02
**Status:** Design approved; reviewed. Ready for implementation planning.

## Delivery

**One PR, built in phases.** The feature ships as a single branch/PR rather than a
chain of smaller ones, but the work is sequenced into phases with a green suite at
each boundary, so the PR grows in reviewable increments:

1. **Label entity + persistence** — `labels` table, `tags.icon` column, migrations
   (both engines), sqlc, `internal/label/` package, CRUD routes, shared-account
   ownership.
2. **Labels on transactions** — `transactions_labels`, `labelIds` on transaction
   create/update/read, the `labelId` read filter.
3. **Budget surfacing** — per-label period aggregation, the budget-neutral block,
   drill-down.
4. **Recurring** — `recurring_transactions_labels`, posting copies labels.
5. **CSV** — export column after `tag`, import mapping key + separator +
   guardrails.
6. **Frontend** — create/edit dialog, transaction editor chips, budget block,
   import separator dialog + label multi-select, per-kind icon/color.
7. **Cross-cutting** — MCP surface, analytics keys, i18n catalogues, and the final
   golden regeneration (apiparity, mcpparity, budget, CSV) + `enginecompare`.

Phases 1–2 are the foundation everything else depends on; 3–5 are independent of
each other once 2 lands.

## Problem

Users want to attribute a single transaction to **multiple** things for
reporting — e.g. a cafe visit shared across two kids, or spending tracked per
pet — so they can later see "how much did I spend on each kid/pet." Today a
transaction carries **at most one tag**, and that tag is a budgeting construct:
it participates in envelope math, limits, and carryover (`tag_id` → a
`budgets_elements` row of `type=tag` → per-period limits).

Overloading the existing single tag can't express "€50 counts fully under kid-A
*and* fully under kid-B" — the amounts overlap on purpose, which is
incompatible with budget math (they'd double-count into envelope totals).

## Goal

Add a **reporting-only** classification — a **label** — that:
- can be attached **many-per-transaction**,
- is **budget-neutral**: never has limits, never affects envelope math / carryover,
- surfaces its per-period spend in the existing budget view as an overlay you can
  click through to the underlying transactions, so users get the "per kid/pet"
  answer without a net-new reports page.

Keep today's **tag** exactly as it is (single-per-transaction, budgeting).

## Terminology

Two backend concepts, one user-facing word:

| | **Tag** (budgeting) | **Label** (reporting) |
|---|---|---|
| Per transaction | at most **one** (unchanged) | **many** |
| Budget role | full: limits, envelope math, carryover | none — budget-neutral spend overlay |
| Shown in budget | as today | collapsible budget-neutral block near uncategorized (clickable → transactions) |
| Icon | seeded to `tag` (hashtag `#`) at create; **stored + rendered from the row** | seeded to `label` at create; **stored + rendered from the row** |
| Color | **hardcoded by kind**, applied to the icon | **hardcoded by kind**, applied to the icon |

In the UI both appear under the umbrella word **"Tags."** The words "tag" and
"label" are internal/backend. In v1 the icon value is **hardcoded only at
create** (seeded to the kind default) and thereafter **stored on the row and
rendered from the DB value** everywhere — never re-derived from kind at render
time. So every budgeting tag currently shows the same hashtag icon and every
label the same `label` icon, but the rendering path already reads the persisted
value, meaning user-defined icons later need only a create/edit change, no
rendering or schema change. Color, by contrast, is **not stored** — it is a
frontend constant keyed on kind, applied to whichever icon the row carries.
Material Symbols provides both ligatures directly: `tag` renders as a hashtag
`#`, `label` as the label shape.

## Non-goals

- **Proportional split across multiple budgeting tags** — explicitly dropped. It
  fights the "one envelope, clear math" model and confuses users.
- **User-picked colors and icons** — both are fixed constants per kind in v1
  (no icon-picker UI). The `icon` column is still stored per row so user-defined
  icons become an additive follow-up, not a schema change.
- **Converting a tag into a label (or vice versa) in place** — not supported. A
  user re-creates + re-assigns. (Two separate tables make this a clean
  non-issue; folding labels into `tags` later stays possible if the concepts
  converge — that migration direction is easy, unlike splitting an overloaded
  table apart.)

## Why two entities, not a `mode` column on `tags`

The transaction side is identical either way (both need a join table for the
many reporting attachments, since one `tag_id` column can't hold many). The
only real difference is whether a reporting row lives in `tags` (with a `mode`
column) or in a separate `labels` table. Two entities was chosen because:

- The frozen, well-tested **budget↔tag coupling stays byte-for-byte untouched** —
  labels never enter the budget-element code path. A `mode` column would force a
  `WHERE mode='budget'` filter through ~5 load-bearing sites (`seedTagElements`,
  `getElementSelfHeal`, member-accept seeding, `SetLimit`, the visibility
  builder), and the one you forget is a silent leak.
- "Labels can't corrupt budget math" becomes a **structural guarantee** (different
  table) instead of a discipline maintained in every query.
- Independent name namespaces; no ambiguous mode-flip state transition.
- The extra code is mechanical mirrors of the existing `internal/tag/` package —
  the cheap kind of code.

## Data model

### New `labels` table
Mirrors `tags`, plus an `icon` column:
```
id          TEXT PK
user_id     TEXT  FK users ON DELETE CASCADE
name        VARCHAR(64)   -- 3-64 runes, unique per owner
icon        TEXT          -- Material ligature; seeded to the kind default "label"
position    SMALLINT
is_archived BOOLEAN DEFAULT 0
created_at, updated_at
```
Per-owner name uniqueness, independent of `tags`.

### New `icon` column on `tags`
Additive, nullable. New tags are seeded to the kind default `"tag"` (hashtag);
the migration backfills existing rows to `"tag"` as well (matching the current
hardcoded budget-view fallback). Persisted per row so user-defined icons stay a
future additive change; in v1 it is always the kind default.

### New join table `transactions_labels`
```
transaction_id TEXT  FK transactions ON DELETE CASCADE
label_id       TEXT  FK labels       ON DELETE CASCADE
PRIMARY KEY (transaction_id, label_id)
INDEX (label_id)          -- reverse lookup: delete-label, filter-by-label
```
Many-to-many. `transactions.tag_id` stays a single nullable FK — unchanged.
The PK's leading `transaction_id` serves the budget aggregation (join from the
transactions it already walks); the separate `label_id` index serves the reverse
direction, mirroring the existing `IDX_…BAD26311 (tag_id)` on `transactions`.

### New join table `recurring_transactions_labels` (v1 includes recurring)
```
recurring_transaction_id TEXT FK recurring_transactions ON DELETE CASCADE
label_id                 TEXT FK labels                 ON DELETE CASCADE
PRIMARY KEY (recurring_transaction_id, label_id)
INDEX (label_id)
```
Posting a due template copies its labels onto the created transaction (see
Recurring below). `recurring_transactions.tag_id` stays single.

Migrations authored for **both** SQLite and PostgreSQL; sqlc queries added for
both engines.

## Backend feature package

New `internal/label/` package mirroring `internal/tag/`:
- `internal/model/label.go` (`model.Label`: id, userId, name, icon, position,
  isArchived, timestamps + invariant-preserving mutators) and
  `internal/model/label_dto.go` (`LabelResult`, request DTOs).
- Use cases: `create`, `update` (rename; icon is not user-editable in v1),
  `archive`/`unarchive`,
  `delete`, `read` (get-label-list, own + shared-account union like tags),
  `order`.
- `repository.go` interfaces, `repo/` engine adapters (sqlite + pgsql),
  `api/` HTTP edge, `mcp/` edge.
- Wired in `server.BuildAPI` alongside tags; archtest auto-detects the new
  feature package.

### Shared accounts

Labels keep **exactly the same shared-account behavior as tags** — a label always
**belongs to the account's owner**, never to whoever created it:
- `create-label` carries the same optional `accountId` pointer as `create-tag`.
  When present, the service resolves the shared account's **owner** and creates the
  label under that owner (subject to the same `AccountAccess` port check tags use),
  so a collaborator adding a label to a shared account creates it for the owner —
  not for themselves.
- `get-label-list` returns **own labels + labels of users who shared an account**
  with the caller (the `accounts_access`, `is_accepted` union, identical to
  `get-tag-list`), so shared labels resolve for rendering.
- In the budget view, a transaction on a shared account aggregates under the
  **owner's** labels, consistent with how the owner's budget already attributes
  shared-account spend.

## Wire contract

All JSON changes are additive; existing envelopes unchanged. **The one
non-additive change in the whole spec is the CSV export column order** (the
inserted `labels` column shifts `payee,amount,date` right by one) — called out
in the CSV section.

- **`TagResult`** gains `icon` (additive field; in v1 always the kind default).
- **`LabelResult`**: `{id, ownerUserId, name, icon, position, isArchived (0/1),
  createdAt, updatedAt}`.
- **Create/update requests carry no `icon` field in v1** — the server sets it to
  the kind default (`create-tag` → `"tag"`, `create-label` → `"label"`). The
  column and the response field exist so a picker can be added later without a
  contract change.
- **Routes** under `/api/v1/label/`: `get-label-list`, `create-label`,
  `update-label`, `archive-label`, `unarchive-label`, `delete-label`,
  `order-label-list` — mirroring the tag routes and conventions.
- **Transaction create/update** gain `labelIds: []string` (optional; empty =
  none). `get-transaction*` responses include `labelIds`.
- **Recurring create/update** gain `labelIds: []string`; recurring reads return
  them.
- **`get-budget`** response gains the labels block (see below).
- **`get-transaction-list` (budget module)** gains an optional `labelId` query
  param, backing the budget block's drill-down.
- **CSV export** adds a `labels` column (right after `tag`, `;`-joined); **CSV
  import** gains a `labels` mapping key, a `labelsSeparator` field, and an optional
  `labelIds` override (see CSV section).

Deletion: deleting a label cascades its `transactions_labels` /
`recurring_transactions_labels` rows (many-to-many, `ON DELETE CASCADE`). No
`SET NULL` needed. Archived labels are hidden from pickers but still resolve for
historical rendering.

## Budget-view surfacing

Labels are **not** budget elements — they never create `budgets_elements` rows
and never touch envelope totals, available-to-spend, or carryover.

- The budget read layer aggregates, **per label, the period spend** over the
  **same transaction set the budget builder already walks** (the budget's
  accounts, the period, expenses), using the **same currency conversion** applied
  to tag/category spend, so label totals are consistent with the rest of the
  view.
- **This must be a SEPARATE read-model method** (e.g. `CountSpendingByLabel`,
  grouping `label_id, currency_id`), *not* an extra column on the existing
  `CountSpending`. That query groups by `(category_id, tag_id, currency_id)` and
  the builder assumes **one bucket per row**; joining a many-to-many there would
  fan one transaction into N rows and double-count the element totals. Keeping the
  label aggregation on its own query is what makes the overlap safe.
- The builder emits a **separate "labels" block**: collapsible, positioned near
  the uncategorized bucket, each label rendered as a chip (icon + name + period
  spend), in `position` order. "Read-only" means **no limits and no budget math** —
  the chips are still interactive (see drill-down).
- **Visibility:** a label appears in the block only if it has spend in the
  period (labels have no limits, so spend is the only trigger). Archived labels
  with no period spend are hidden.
- **Drill-down:** clicking a label chip opens the existing budget-transactions
  dialog filtered to that label, mirroring how a budgeting tag element drills down
  today. This runs through the **budget module's own** transaction list
  (`GET /api/v1/budget/get-transaction-list`), not the transaction feature's
  filter, so the work is: `BudgetTransactionListRequest` gains `LabelId *string`;
  the dispatch switch in `internal/budget/txlist.go` gains a label branch; the read
  repo gains `BudgetTransactionsByLabel` mirroring the existing
  `BudgetTransactionsByTag` (joining `transactions_labels`, served by its
  `label_id` index); and the frontend `BudgetTransactionsParams` gains `labelId`.
  Without this the block would show a number the user cannot investigate, which
  defeats the "where did the money for kid A go" use case.
  (The transaction feature's own `TransactionFilter` is **not** changed in v1 —
  filtering the main transaction list by label is a separate follow-up.)
- **Whose labels (shared budgets):** the block aggregates under the **account
  owner's** labels, consistent with the shared-account rule above and with how the
  budget already attributes shared-account spend. So a budget spanning a
  collaborator's shared account can show that owner's labels alongside your own,
  and a collaborator's labelling work stays visible to you.
- **Overlap is intentional and safe**: a €50 expense labeled `kid-A` + `kid-B`
  shows €50 under each. The block is display-only, so it need not (and will not)
  sum to total spend, and it must not feed any envelope/available/carryover math.

## Transaction editor UX

- One **"Tags"** section. Budgeting tags and labels render as chips in a **single
  flat list**. Each chip renders the **icon stored on its row** (currently the kind
  default — hashtag for tags, `label` for labels) plus the **per-kind color**
  constant, and the name.
- Selection behaves per kind:
  - **Budgeting tags are radio-like** — selecting one deselects the previous
    (preserves the single `tag_id`).
  - **Labels toggle freely** — any number.
- Inline create picks the **kind** (radio) — no icon picker; the server seeds the
  icon from the kind, and the chip then renders that stored value.
- Recurring editor mirrors this (single tag + many labels).

## Recurring transactions (v1)

- Templates carry labels via `recurring_transactions_labels`.
- **Posting a due template** copies the template's labels onto the created
  transaction (into `transactions_labels`), alongside the existing single-tag
  copy. Posting stays idempotent on the client-supplied transaction id — a
  re-post does not duplicate label attachments.

## CSV import / export

Today the CSV format carries a single `tag` column (a tag **name**; empty when
nil), and import matches that name case-insensitively against the owner's tags,
**auto-creating** on miss. The budgeting tag and reporting labels must be
**separate columns**, because they are separate concepts and labels are
multi-valued.

### Export (`internal/transaction/export.go`)
- **Keep the existing `tag` column** — same meaning (the single budgeting tag
  name, empty when nil).
- **Insert a new `labels` column immediately after `tag`**, so the two
  classification columns sit together:
  ```
  transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date
  ```
  This shifts `payee,amount,date` right by one index. Econumo's own import matches
  by header *name*, so a round-trip is unaffected; a position-based external
  consumer of those three columns would need to re-map (documented).
- The `labels` cell is the transaction's label **names joined by `;`** (semicolon),
  in label `position` order; empty string when none. Each label name passes through
  the existing `sanitizeExportValue` (CR/LF collapse + formula-injection guard)
  **before** joining. (Edge case: a label name containing a literal `;` is not
  round-trip-safe — acceptable, and documented.)
- Transfer recipient rows write `labels` empty, exactly as they do for `tag`.

### Import (`internal/transaction/import.go`, `api/import.go`)
- **New mapping key `labels`** (a CSV column header name or null), parallel to
  `tag`. The mapped cell is **split on a user-chosen separator** (see below), each
  piece trimmed, blanks dropped, deduped case-insensitively, then each resolved by
  name against the owner's labels and **auto-created on miss** — mirroring the tag
  find-or-create path, but against the label repo. Resolved ids attach via
  `transactions_labels`.
- **Configurable separator.** The import posts a new `labelsSeparator` field
  (default `;`). The backend splits the mapped `labels` cell on exactly that
  string. This decouples import from the export default, so a CSV produced by
  another tool (comma-, pipe-, or newline-in-cell separated) still imports.
- Blank/absent `labels` cell → no labels (mirrors blank tag → nil).
- **Guardrails against a mis-mapped column.** Auto-create is convenient but a user
  who maps `description` to `labels` would otherwise mass-create junk — a bigger
  blast radius than tags, since every cell now splits into many values:
  - **Cap labels per row** at 10; a row exceeding it is a row-level error rather
    than a silent truncation.
  - **Preview the count**: before the import runs, the dialog states how many
    **new** labels it will create ("this import will create N new labels"), so a
    wrong mapping is obvious at the confirm step rather than after the fact.
  - Pieces that fail label name validation (outside 3–64 runes) are reported via
    the existing per-row error reporting (surfaced by `ImportResultDialog`), not
    silently dropped.
- **New optional override `labelIds`** (comma-joined id list) parallel to the
  single `tagId` override — applies the same labels to every imported row. Each id
  is belongs-to checked against the account owner, same rule as the `tagId`
  override (`OwnerID == accountOwnerID`); an id given but not found is the same
  top-level error style as `tagId`.
- Backend does all label resolution/creation; the frontend forwards a column name
  or the `labelIds`, exactly as it does for tags.

### Frontend (`web/src/features/transactions/importCsv.ts`, `ImportCsvDialog.tsx`)
- Add `labels` to the import field keys and `autoDetect` (new translated label
  `transactions.import_csv.fields.labels`); `buildImportPayload` sets
  `mapping.labels` in `csv_column` mode.
- **Separator picker.** When the `labels` column is mapped, show a small
  **"separator" button** next to it that opens a dialog to choose how that cell is
  split — presets (`;` default, `,`, `|`, tab/newline) plus a custom free-text
  value. The choice is sent as `labelsSeparator`. The button/dialog appear only in
  `csv_column` mode (no separator needed when applying fixed `labelIds`).
- In "existing" mode, add an optional **label multi-select** (owner-scoped, like
  the existing single-tag `tagId` picker) that sends `labelIds`.
- Export dialog is unchanged (download-only) — the new `labels` column comes from
  the backend automatically.

## Management UX (create/edit dialog)

Improve the **existing tag create/edit dialog** (rather than build a new one):
- Fields are just a **name input** and a **kind radio** (budgeting tag / label) —
  no icon picker.
- The dialog **shows the icon** the row will get — for a not-yet-saved row it
  previews the selected kind's default (hashtag / `label`) in the kind's color, and
  **swaps it live** as the radio changes. Saved rows in the list render the icon
  **from the stored DB value**, not re-derived from kind.
- Delete/archive/reorder mirror today's tag management.
- The settings surface lists **both kinds under "Tags,"** each chip rendering its
  stored icon + per-kind color. (Backend: two separate CRUD surfaces; the settings
  screen composes both lists.)

## Icons & color (both hardcoded per kind)

- **No icon picker in v1**, but the icon is **not a render-time constant** — it is
  **hardcoded only at create** (the server seeds `tags.icon`/`labels.icon` to the
  kind default: `tag` for budgeting tags, `label` for labels) and then **stored per
  row and rendered from the stored value** on every surface. Because the whole
  render path already reads the DB value, user-defined icons later are a pure
  create/edit change — no rendering or schema work.
- Budget view renders the stored icon (currently always the kind default) instead
  of the hardcoded `"tag"`; falls back to `"tag"` when null.
- Color is a **frontend constant per kind** (mirrors the `avatars.ts` /
  category-color approach), applied to the icon: one fixed color for budgeting
  tags, one for labels.

## MCP, analytics, i18n

- **MCP:** `internal/label/mcp/` mirrors the tag MCP surface (list/create/etc.);
  the transaction MCP tool gains `labelIds`. mcpparity goldens added.
- **Analytics:** new `METRICS` keys (frozen camelCase) for label create + label
  attach, fired at the mutation success points per the product-analytics rule;
  `metrics-coverage.test.ts` must pass.
- **i18n:** new catalogue keys for the labels UI + any coded errors, added to the
  `en`/`ru` catalogues (and the other shipped languages per the add-language
  guards); placeholder-set parity maintained. The budget labels-block title is a
  new catalogue key.

## Testing / contract impact

- Migrations (SQLite + PostgreSQL), sqlc query variants for both engines.
- `apiparity` scenarios + goldens for every new `label` route and the new
  transaction/recurring `labelIds` field; existing tag goldens change only by
  the additive `icon` field.
- `mcpparity` scenarios + goldens for the label MCP surface.
- Budget goldens updated for the new labels block.
- CSV export/import goldens updated for the inserted `labels` column (and the
  resulting index shift of `payee,amount,date`) and the `labels` mapping key;
  `importCsv.test.ts` gains label-mapping + custom-separator split cases.
- `enginecompare` (sqlite vs pgsql byte-identical) for all of the above.
- `i18ntest` parity guards; `archtest` picks up the new feature package
  automatically.
- Feature tests mirroring `internal/tag/` and the budget label-visibility /
  overlap behavior (analogous to `tag_limit_visibility_test.go` /
  `tag_child_drilldown_test.go`), plus label drill-down (the new `labelId`
  transaction filter) and the shared-budget case where a collaborator's
  owner-scoped labels appear in the block.
- Import guardrail tests: labels-per-row cap, the new-label count preview, and
  name-validation failures surfacing as row errors.

## Decisions log

- **Reporting surface:** reuse the budget view (no net-new reports page).
- **Model:** two backend entities (tags + labels), unified under "Tags" in UI.
- **Recurring labels:** included in v1.
- **Picker:** one flat list, kind distinguished by hardcoded per-kind icon + color.
- **Visuals:** no icon picker in v1. Icon is hardcoded **only at create** (seeded to
  the kind default — hashtag `tag` for budgeting tags, `label` for labels) then
  **stored per row and rendered from the DB value** everywhere; color is a per-kind
  frontend constant (not stored). User-defined icons are a future additive change.
- **Create/edit dialog:** improve the existing tag dialog — name input + kind radio,
  showing the kind's icon/color live; no icon picker.
- **CSV:** keep `tag` column; add a `labels` column **right after `tag`** (names
  joined by `;` on export); import gains a `labels` mapping key with a
  **user-chosen separator** (button → dialog, default `;`; name-match + auto-create
  like tags) and an optional `labelIds` override.
- **Drill-down:** label chips are clickable → transactions filtered by label, via a
  `labelId` param on the budget module's `get-transaction-list` (the transaction
  feature's own filter is untouched in v1).
- **Shared budgets:** the labels block aggregates under the **account owner's**
  labels, so a collaborator's labels can appear alongside your own.
- **Import guardrails:** cap 10 labels/row, preview the new-label count before
  running, surface name-validation failures as row errors.
- **Dropped:** proportional split across multiple budgeting tags.
