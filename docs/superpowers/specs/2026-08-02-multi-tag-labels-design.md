# Multi-tag mechanics: reporting labels

**Date:** 2026-08-02
**Status:** Design approved, pending spec review

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
- is **display-only**: never has limits, never affects envelope math / carryover,
- surfaces its per-period spend in the existing budget view as a read-only
  overlay, so users get the "per kid/pet" answer without a net-new reports page.

Keep today's **tag** exactly as it is (single-per-transaction, budgeting).

## Terminology

Two backend concepts, one user-facing word:

| | **Tag** (budgeting) | **Label** (reporting) |
|---|---|---|
| Per transaction | at most **one** (unchanged) | **many** |
| Budget role | full: limits, envelope math, carryover | none — read-only spend overlay |
| Shown in budget | as today | collapsible read-only block near uncategorized |
| Icon | user-picked (new) | user-picked |
| Color | **hardcoded by kind** (not user-picked) | **hardcoded by kind** |

In the UI both appear under the umbrella word **"Tags."** The words "tag" and
"label" are internal/backend. Color encodes *kind* (budget vs report); the
user-picked **icon** encodes *identity*.

## Non-goals

- **Proportional split across multiple budgeting tags** — explicitly dropped. It
  fights the "one envelope, clear math" model and confuses users.
- **User-picked colors** — color is a fixed constant per kind. Only the icon is
  user-selectable.
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
icon        TEXT          -- Material ligature name (e.g. "child_care", "pets")
position    SMALLINT
is_archived BOOLEAN DEFAULT 0
created_at, updated_at
```
Per-owner name uniqueness, independent of `tags`.

### New `icon` column on `tags`
Budgeting tags get an icon picker too. Additive, nullable; existing rows render
the current fallback `"tag"` when null. Color for tags stays a fixed constant.

### New join table `transactions_labels`
```
transaction_id TEXT  FK transactions ON DELETE CASCADE
label_id       TEXT  FK labels       ON DELETE CASCADE
PRIMARY KEY (transaction_id, label_id)
```
Many-to-many. `transactions.tag_id` stays a single nullable FK — unchanged.

### New join table `recurring_transactions_labels` (v1 includes recurring)
```
recurring_transaction_id TEXT FK recurring_transactions ON DELETE CASCADE
label_id                 TEXT FK labels                 ON DELETE CASCADE
PRIMARY KEY (recurring_transaction_id, label_id)
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
- Use cases: `create`, `update` (rename + icon), `archive`/`unarchive`,
  `delete`, `read` (get-label-list, own + shared-account union like tags),
  `order`.
- `repository.go` interfaces, `repo/` engine adapters (sqlite + pgsql),
  `api/` HTTP edge, `mcp/` edge.
- Wired in `server.BuildAPI` alongside tags; archtest auto-detects the new
  feature package.

Shared-account behavior mirrors tags: a user sees own labels + labels of users
who shared an account (via `accounts_access`, `is_accepted`).

## Wire contract

All additive; existing envelopes unchanged.

- **`TagResult`** gains `icon` (additive field).
- **`LabelResult`**: `{id, ownerUserId, name, icon, position, isArchived (0/1),
  createdAt, updatedAt}`.
- **Routes** under `/api/v1/label/`: `get-label-list`, `create-label`,
  `update-label`, `archive-label`, `unarchive-label`, `delete-label`,
  `order-label-list` — mirroring the tag routes and conventions.
- **Transaction create/update** gain `labelIds: []string` (optional; empty =
  none). `get-transaction*` responses include `labelIds`.
- **Recurring create/update** gain `labelIds: []string`; recurring reads return
  them.
- **`get-budget`** response gains a read-only labels block (see below).

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
- The builder emits a **separate read-only "labels" block**: collapsible,
  positioned near the uncategorized bucket, each label rendered as a read-only
  chip (icon + name + period spend), in `position` order.
- **Visibility:** a label appears in the block only if it has spend in the
  period (labels have no limits, so spend is the only trigger). Archived labels
  with no period spend are hidden.
- **Overlap is intentional and safe**: a €50 expense labeled `kid-A` + `kid-B`
  shows €50 under each. The block is display-only, so it need not (and will not)
  sum to total spend, and it must not feed any envelope/available/carryover math.

## Transaction editor UX

- One **"Tags"** section. Budgeting tags and labels render as chips in a **single
  flat list**, visually distinguished by a **hardcoded color per kind** (tags one
  color, labels another). Each chip shows its user-picked icon + name.
- Selection behaves per kind:
  - **Budgeting tags are radio-like** — selecting one deselects the previous
    (preserves the single `tag_id`).
  - **Labels toggle freely** — any number.
- Inline create supports choosing kind + icon.
- Recurring editor mirrors this (single tag + many labels).

## Recurring transactions (v1)

- Templates carry labels via `recurring_transactions_labels`.
- **Posting a due template** copies the template's labels onto the created
  transaction (into `transactions_labels`), alongside the existing single-tag
  copy. Posting stays idempotent on the client-supplied transaction id — a
  re-post does not duplicate label attachments.

## Management UX

The existing tags settings surface lists **both kinds under "Tags,"** each chip
tinted by kind and showing its icon. Create/edit dialog picks kind (budget tag /
label) + icon; archive/reorder mirror today's tag management. (Backend: two
separate CRUD surfaces; the settings screen composes both lists.)

## Icons (color hardcoded)

- Reuse the category icon-picker pattern; store **only the ligature name** (no
  color) on `tags.icon` / `labels.icon`.
- Budget view renders the chosen icon instead of the current hardcoded `"tag"`
  (fallback `"tag"` when null).
- Color is a **frontend constant per kind** (mirrors the `avatars.ts` /
  category-color approach): one fixed color for budgeting tags, one for labels.

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
- Budget goldens updated for the new read-only labels block.
- `enginecompare` (sqlite vs pgsql byte-identical) for all of the above.
- `i18ntest` parity guards; `archtest` picks up the new feature package
  automatically.
- Feature tests mirroring `internal/tag/` and the budget label-visibility /
  overlap behavior (analogous to `tag_limit_visibility_test.go` /
  `tag_child_drilldown_test.go`).

## Decisions log

- **Reporting surface:** reuse the budget view (no net-new reports page).
- **Model:** two backend entities (tags + labels), unified under "Tags" in UI.
- **Recurring labels:** included in v1.
- **Picker:** one flat list, kind distinguished by hardcoded color.
- **Visuals:** user-picked icon; color hardcoded per kind (not per tag).
- **Dropped:** proportional split across multiple budgeting tags.
