# Budget Plan View — Design

Date: 2026-08-02
Status: Approved design, pending implementation plan

## Overview

A **Plan** mode on the budget page: a multi-month sheet where every row is a budget
element (incomes and expenses) and every column is a month. Each cell shows two
values — the **actual** amount (read-only: money received/spent that month) and the
**planned** amount (editable: the same value the budget page calls "budgeted").
A totals block at the bottom (Income / Expenses / Net / running Balance) turns it
into a one-page planner for up to a year ahead.

Planned expense in the plan view **is** the budget limit for that month — one value,
two views. The genuinely new concept is **planned income**, which this design adds by
making income categories first-class budget elements.

## Decisions made during brainstorming

| Topic | Decision |
|---|---|
| Income rows | Per income category, groupable into envelopes (shared budgets need e.g. one "Salaries" envelope across members). All income rows live under a single fixed "Income" section — rendered like a folder but synthetic, not a `budget_folders` row; user-created folders stay expense-only |
| Row structure | Mirrors the budget page exactly (folders, envelopes with children, uncategorized, archived) |
| Placement | A Budget \| Plan toggle on the existing budget page |
| Visible window | Responsive: as many month columns as fit the screen (min 3, max 12); left/right arrows shift the first month; no horizon selector |
| Initial position | Current month in the second column (one past month first); clamped at the budget start month; persisted |
| Backend approach | Income categories become budget elements (new element type); planned income reuses `budgets_elements_limits` and `set-limit`; one new read endpoint `get-budget-plan` |
| Carryover | Not shown per cell; enters the totals via the running Balance row |
| Balance row seed | Real account balances at the window start (existing financial-summary machinery, excluded accounts respected) |

## Backend

### Income elements

- `budget_elements.type` gains two new values — *income category* and *income
  envelope* — mirroring the existing envelope/category split (existing values
  envelope/category/tag stay untouched). No schema migration — the column and the
  `UNIQUE(budget_id, external_id)` constraint already accommodate them.
- *Considered and rejected*: reusing the existing `category` type value and
  deriving the side from the category's income flag. Rejected because (a) every
  `get-budget` exclusion site (structure, limits/carryover queries, seeding) would
  need category data joined in instead of a self-describing `type` predicate on
  the elements table — fragile protection for a byte-frozen contract; and (b)
  envelopes have no category to derive a side from, so income envelopes need a
  stored side regardless — one consistent representation beats a split one. The
  stored side cannot drift: a category's income/expense type is immutable
  (`UpdateCategory` changes only name and icon).
- **Seeding**: `seedCategoryElements` (budget creation) also seeds the members'
  income categories with the income type, positioned after their category order.
  The `SetLimit` self-heal path learns to create income-typed elements for income
  categories created after the budget (it reads the category's income flag to pick
  the type).
- **Planned income storage**: rows in the existing `budgets_elements_limits` table,
  keyed (element, month) — identical to expense limits.

### Envelopes with income categories

- `CreateEnvelope` / `UpdateEnvelope` accept income categories, with a
  **homogeneity rule**: an envelope's child categories are all-expense or
  all-income, never mixed. Violations return a new coded error
  (registered in `errs.AllCodes` with `errors.*` catalogue entries in every
  language — the i18n guards enforce the pairing).
- An envelope containing income categories is an income-typed element and appears
  in the plan view's income section, never in `get-budget`.

### Frozen contract: get-budget unchanged

Every existing builder (`buildStructure`, `buildFilters`, `buildElementsSpending`,
seed listing for the single-month read) explicitly excludes income-typed elements.
The apiparity goldens for `get-budget` (and every other existing route) must come
out **byte-identical** — that diff is the regression proof.

### Writes: no new endpoints

Editable plan cells call the existing `POST /api/v1/budget/set-limit` with the
cell's month as `period` — for expense elements and income categories alike.
Permissions are unchanged: role owner/admin/user and `period >= budget.startedAt`.
`Amount: null` clears a planned value, as today.

### New read: get-budget-plan

`GET /api/v1/budget/get-budget-plan?id=<budgetId>&from=<Y-m-d first of month>&months=<1..24>`

Readable by any accepted budget member (same visibility rule as `get-budget`).
`from` is snapped to first-of-month; `months` outside 1–24 is a validation error.

Response sketch (final field names to follow the existing DTO conventions in
`internal/model/budget_dto.go`):

```jsonc
{
  "meta": { /* same shape as get-budget meta */ },
  "months": ["2026-06-01", "..."],                  // ordered, length = months
  "openingBalances": [ {"currencyId": "...", "amount": "..."} ],
  "currencyRates": [ { "period": "2026-06-01", "rates": [ /* AverageCurrencyRateResult */ ] } ],
  "structure": { "folders": [ /* as get-budget */ ], "elements": [ /* PlanElement: ALL rows — income and expense */ ] }
}
```

There is **no separate incomes section**: income elements (income envelopes with
children, standalone income categories) appear in `structure.elements` alongside
the expense rows, distinguished solely by their `type` value — it fully encodes
the side, so a dedicated array would duplicate it. Income elements carry no
`folderId` (the SPA renders them under the synthetic Income section, one more
bucket in the existing `bucketElements` pattern). One builder pass on the server,
one shape for clients, and the wire doesn't change if income rows ever gain
folders or reordering.

`PlanElement`: id, type, name, icon, currencyId, isArchived, position, folderId?,
ownerUserId?, `cells: [{actual, planned}]` aligned with `months` (`planned` is `""`
when no limit row exists; amounts are decimal strings in the element's currency),
`children: [{ …, cells: [{actual}] }]` for envelope children (planned lives on the
parent, exactly like `budgeted` today). The synthetic Uncategorized element appears
with actual-only cells. All existing wire conventions hold (datetime layout,
`isArchived` as 0/1, decimal strings, envelope with HTML escaping off).

- `openingBalances` — per-currency account balances at the start of `months[0]`,
  computed by the existing financial-summary machinery with excluded accounts
  respected. Seeds the client-side Balance row.
- `currencyRates` — the existing per-period average-rate computation, once per
  month in the window. The server computes **no totals**; the SPA does FX and
  totals client-side, as the budget page does.

### Queries

Three grouped-by-month additions to `internal/budget/repo/read.go` (already
hand-built per-engine dynamic SQL, so the dialect split is at home there):

- spending by month: expense transactions summed per (month, category/tag, currency)
  over `[from, to)`;
- income by month: same for income transactions per (month, category, currency);
- limits by month: `budgets_elements_limits` per (external_id, type, period).

Month bucketing is `strftime('%Y-%m-01', …)` on SQLite vs `date_trunc('month', …)`
on PostgreSQL; results must normalize to byte-identical wire output (enginecompare
covers it). **No carryover loop** — a plan cell is just actual + planned, so the
endpoint is O(1) queries regardless of window size.

## Frontend

### Mode toggle

`BudgetPage` gets a **Budget | Plan** segmented toggle, persisted in
`useBudgetPeriodStore` alongside the selected period. Plan mode reuses the page
header (budget picker, currency) and replaces the month strip + budget table with
the plan sheet.

### The sheet

- **Responsive column count**: the grid measures its container (ResizeObserver);
  after the sticky row-name column it shows as many month columns as fit at a
  comfortable minimum column width — never fewer than 3 (narrow phones), up to 12
  on wide screens, reflowing live on resize.
- **Navigation**: left/right arrow buttons shift the first visible month by one
  month (press-and-hold repeats). Navigation clamps at the budget's start month;
  there is no forward limit.
- **Initial position**: the current month sits in the **second column** — one past
  month for comparison, the rest of the width looking forward — regardless of how
  many columns fit; clamped at the budget start (a budget starting this month shows
  the current month first). The first visible month persists in the budget store
  and is restored on return.
- **Current month column** is visually highlighted.
- Sections top to bottom: **Income** — a single fixed section header (styled like a
  folder, but synthetic: no `budget_folders` row, so it cannot be renamed, deleted,
  or receive expense elements) holding income envelopes (expandable to children)
  and standalone income categories; then the **expense structure** exactly as the
  budget page orders it (folders as section headers, elements, uncategorized,
  archived); then the **Totals block**.

### Cells

- Each cell: actual on top (muted, read-only; rendered as a dash when zero in a
  future month), planned below (editable where permitted). In the current and
  future months an expense cell whose actual exceeds its planned value gets a
  subtle overspend highlight — that cell's actual is what feeds the Balance
  projection, so the user can see where the plan is already broken.
- Editing reuses the existing machinery: `LimitEditor` popover with
  `CalculatorInput` on desktop, `SetLimitDialog` on compact screens — committing
  `set-limit` with the cell's month, optimistic cell patch, invalidating both the
  budget and plan queries.
- Editability per cell: `canUpdateLimits` semantics (role owner/admin/user and
  month ≥ budget start). Past months remain editable, months before the budget
  start are read-only. Envelope children and Uncategorized are actual-only.

### Row density controls

Large category sets (e.g. ~30 income categories spanning two countries) must not
drown the sheet. Three complementary mechanisms, all client-side:

- **Combine**: envelopes — the primary tool; children stay collapsed until
  expanded.
- **Collapse**: the Income section header and expense folders fold/unfold in the
  plan sheet, fold state persisted (extending the budget store's existing
  fold-state pattern).
- **Hide**: a **"hide empty rows"** toggle hides any row (income and expense
  alike) with no actual and no planned value in every fetched month. Dormant
  categories disappear automatically and return the moment they get activity or a
  plan. While on, each section header shows an "N hidden — show" affordance so a
  dormant category can be revealed to start planning it. Persisted per user,
  default off; toggling fires a `METRICS` event.

Archived categories keep today's treatment (tucked into the Archived section).

### Keyboard interaction

The sheet follows the ARIA grid pattern with a roving cell selection:

- **Arrow keys** move the selected cell: up/down walk the rows (skipping section
  headers and the totals block), left/right walk the month columns. Moving left
  from the first visible column (or right from the last) shifts the sheet window
  by one month — the same operation as the on-screen arrow buttons — with the
  selection staying on its row.
- **Enter** on a selected editable cell opens the planned-amount editor
  (`CalculatorInput` popover) prefilled with the current value; **Enter** commits
  and returns focus to the cell, **Esc** cancels. On read-only cells (envelope
  children, Uncategorized, months before the budget start, insufficient role)
  Enter does nothing; on an envelope parent's row-name cell it toggles expansion.
- Cells expose screen-reader labels ("<row name>, <month>: actual …, planned …")
  and `aria-selected`; selection is arrow-key driven rather than Tab-driven, which
  also sidesteps Safari's Tab-skips-buttons behavior.

### Totals block (client-computed, budget currency)

Per visible month, converted with that month's average rates via the existing
`budgetMath` FX helpers:

- **Income** — actual | planned
- **Expenses** — actual | planned
- **Net** — actual | planned (income − expenses)
- **Balance** — one running row: `balance(m) = balance(m−1) + net(m)`, seeded from
  the FX-converted `openingBalances` at the fetch-window start. Fully elapsed
  months contribute their **actual** net. The current and future months contribute
  an **effective** net computed per cell as `max(actual, planned)`: an overspent
  category counts at its actual spend (overspending never vanishes from the
  projection), an underspent one at its plan (the rest is still expected to be
  spent). The same rule applies on the income side (income already received above
  plan counts at its actual). The max is taken **per element cell**, not on the
  totals — overspend in one category must not be offset by another category's
  not-yet-spent budget. Unspent money rolls forward automatically — this is where
  carryover surfaces, telescoped into one number.

### Data fetching

`useBudgetPlan(budgetId, from, months)` (TanStack Query): fetches the visible
window plus a 2-month buffer on each side, `placeholderData` keeps the previous
grid rendered while the next window loads, so arrow navigation never flashes a
spinner. The Balance seed corresponds to the fetch start, so buffered months keep
the running math consistent.

### Envelope management for incomes

Income rows exist only in the plan view, so the income section header exposes the
existing envelope create/edit dialogs for members with edit rights. Drag
re-ordering of income rows is out of scope for v1 (seed order = category order;
`MoveElementList` already exists server-side if we add UI later).

The envelope dialog's category picker is **side-constrained** — the UI enforcement
of the homogeneity rule (the server's coded rejection remains the backstop):

- **Budget view** (expense structure only): the dialog offers expense categories
  only — exactly today's behavior, unchanged.
- **Plan view**: the offered categories follow the envelope's side — editing or
  creating from the Income section offers income categories only; editing or
  creating from the expense structure offers expense categories only. A single
  dialog never mixes the two sides.

### Analytics

New `METRICS` keys (frozen camelCase, PostHog name derived): one for opening Plan
mode, one for arrow navigation. Planned-value edits are already covered by the
shared `useSetLimit` choke point. The metrics-coverage test forces the wiring.

### i18n

New UI strings (mode toggle, section titles, totals labels, envelope
homogeneity error) go into **every** catalogue in `locales/`; the parity guards
(`internal/test/i18ntest`) enforce key and placeholder parity, and the
`errs.AllCodes` ↔ `errors.*` guard covers the new error code.

## Testing

- **Repo queries**: sqlite unit tests for the three grouped-by-month queries and
  opening balances; rerun under `make test-repo-pgsql`; byte-parity via the
  `enginecompare` suite.
- **apiparity**: scenario + golden for `get-budget-plan`; all existing goldens
  (notably `get-budget`) byte-identical — the frozen-contract regression proof.
- **MCP**: a `get-budget-plan` tool in `internal/budget/mcp` mirroring the REST
  read, with an `mcpparity` golden.
- **Behavior tests**: `SetLimit` self-heals an income-typed element; envelope
  homogeneity rejection (with the coded error); income elements excluded from every
  `get-budget` builder; `months` bounds validation.
- **Frontend (vitest)**: window math (responsive count, initial anchoring,
  clamping), totals + Balance rolling math (actual for past months; per-cell
  `max(actual, planned)` for current/future, including the overspent-category
  case),
  a grid-cell edit component test; keyboard navigation (arrow movement, window
  shift at the visible edge, Enter-to-edit/Esc, read-only cells inert);
  metrics-coverage; `pnpm exec tsc -b` before claiming done.

## Out of scope (v1)

- Drag re-ordering of income rows; user-created folders for income rows (the single
  synthetic "Income" section is the only income-side grouping above envelopes).
- Per-cell carryover ("available") display.
- Server-side totals.
- Any change to the budget page's single-month view or its wire contract.
- Archive/unarchive actions in the budget/plan row menus (deferred; the
  `archive-category`/`archive-tag` + unarchive endpoints already exist, so this is
  a frontend-only follow-up — until then the classifications pages remain the
  place to archive).
