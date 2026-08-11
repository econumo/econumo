# Budget Plan View — Design

Date: 2026-08-02
Status: Approved design (amended 2026-08-10 after codebase review), pending implementation plan

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
| Income rows | Per income category, groupable into envelopes (shared budgets need e.g. one "Salaries" envelope across members). All income rows live under a single fixed "Income" section — rendered like a folder but synthetic, not a `budget_folders` row |
| Folder sides | Content-derived: an empty folder is neutral (shows in both views); its first member's side makes it income-only or expense-only; cross-side moves are rejected. Income folders render inside the plan's Income section; `get-budget` filters income-sided folders out |
| Row structure | Mirrors the budget page exactly (folders, envelopes with children, uncategorized, archived) |
| Placement | A Budget \| Plan toggle on the existing budget page |
| Visible window | Responsive: as many month columns as fit the screen (3–12); on phones the sheet collapses to a single month column; left/right arrows shift the window; no horizon selector. Plan mode is desktop-first — mobile gets a usable degradation, not parity |
| Initial position | Current month in the second column (one past month first); clamped at the budget start month; persisted |
| Backend approach | Income categories become budget elements (new element type); planned income reuses `budgets_elements_limits` and `set-limit`; one new read endpoint `get-budget-plan` |
| Envelope side | Stored and **immutable**: set at creation (optional side field on `create-envelope`, absent = expense so existing clients are untouched); children must match it — an envelope never changes side |
| Uncategorized income | A second synthetic Uncategorized row on the income side (actual-only), so category-less income is visible and the totals/Balance math reconciles |
| Carryover | Not shown per cell; enters the totals via the running Balance row |
| Balance row seed | Real account balances at the window start (existing financial-summary machinery, excluded accounts respected) |

## Backend

### Income elements

- `budgets_elements.type` gains two new values — *income category* and *income
  envelope* — mirroring the existing envelope/category split (existing values
  envelope/category/tag stay untouched). No schema migration — the column and the
  `UNIQUE(budget_id, external_id)` constraint already accommodate them (no income
  category ever received a row: seeding and sync have always skipped them).
- *Considered and rejected*: reusing the existing `category` type value and
  deriving the side from the category's income flag. Income is excluded today by
  `IsIncome` checks over category metadata already in hand at every exclusion
  site (`buildFilters`, `seedCategoryElements`, `syncElements`,
  `envelopeChildren`), so a stored type is not what protects `get-budget` — those
  checks stay. Rejected anyway because (a) envelopes have no category to derive a
  side from, so income envelopes need a stored side regardless, and (b) the
  limits queries (`SummarizedLimits`, the per-period listing) group by
  `(external_id, type)`, so a self-describing type keeps them side-aware without
  category joins — one consistent representation beats a split one. The stored
  side cannot drift: a category's income/expense type is immutable
  (`UpdateCategory` changes only name and icon), and an envelope's side is
  immutable by rule (below).
- **Reconciliation is the primary implementation site — not seeding.**
  `syncElements` (`internal/budget/move.go`) is the single reconciler that runs
  as the last step of every element-mutating use case (move, envelope
  create/update/delete, and the `SetLimit` self-heal, which is just "sync, then
  retry once"). Its final pass **deletes every element row its walk did not mark
  seen**, and `budgets_elements_limits` cascades on element delete — so if the
  walk is not taught the new types, the first envelope edit or drag-reorder
  after seeding silently destroys every income element *and all planned income
  entered so far*. It must therefore:
  - walk income categories (ensuring income-category-typed rows) instead of
    skipping `IsIncome`;
  - ensure each envelope under its **stored side's type** instead of the
    unconditional expense-envelope ensure — a side-blind ensure creates a
    duplicate row for the same external id, and because inserts run before the
    delete-unseen pass, `UNIQUE(budget_id, external_id)` fails the whole
    transaction;
  - treat a type mismatch on an existing external id as an **in-place type
    update**, never delete-and-recreate (the delete would cascade the row's
    limits away and the insert trips the unique constraint).
  Teaching `syncElements` gives the `SetLimit` self-heal income support for
  free, and lazily backfills older budgets' element rows on their first
  element-mutating write.
- **Seeding**: `seedCategoryElements` (budget creation, re-run per member on
  `AcceptAccess`) also seeds the members' income categories with the income
  type, continuing the same fractional sort-key chain (`sortkey.Seed`/`Between`)
  after the expense categories.
- **Element rows are optional for display.** `get-budget` synthesizes its rows
  from category/tag metadata; `budgets_elements` rows contribute only options
  (currency, folder, sort key), which is why a category created after the budget
  is visible with no row. `get-budget-plan` must do the same for income rows —
  existing budgets then show their income categories immediately with **no
  backfill**, and a row is persisted only when first needed (seeding at
  creation, `syncElements` on any element-mutating write, or the `SetLimit`
  self-heal).
- **Planned income storage**: rows in the existing `budgets_elements_limits` table,
  keyed (element, month) — identical to expense limits.

### Envelopes with income categories

- An envelope's side is **stored and immutable**: `create-envelope` gains an
  optional side field (absent = expense, so existing clients, the SPA, and the
  apiparity goldens are untouched); the plan view's Income section creates
  income-sided envelopes. `UpdateEnvelope` cannot change the side. Immutability
  buys two structural guarantees: the reconciler never sees an envelope type
  transition, and a folder can only become mixed via a move — one enforcement
  point. A zero-child envelope keeps its stored side (children are not needed to
  derive it).
- `CreateEnvelope` / `UpdateEnvelope` validate children against the envelope's
  side — the **homogeneity rule**: an envelope's child categories all match its
  side, never mixed. Violations return a new coded error (registered in
  `errs.AllCodes` with `errors.*` catalogue entries in every language — the
  i18n guards enforce the pairing). This is **net-new validation**: today those
  endpoints accept arbitrary category ids with no existence, ownership, or side
  check at all — the write path needs the checks added, not adjusted.
- **Dirty data**: because writes were never validated, production envelopes may
  already hold income-category children — persisted in
  `budgets_envelopes_categories` but invisible (`envelopeChildren` filters
  `IsIncome` on read). All existing envelopes are expense-sided by definition,
  so a one-time cleanup (migration) deletes income-category child links from
  them. Those links were never user-visible, so removing them changes no
  observed behavior; without the cleanup the plan view would suddenly surface
  them, and the homogeneity rule would reject edits to envelopes the user never
  knowingly mixed.
- An income-sided envelope is an income-typed element and appears in the plan
  view's income section, never in `get-budget`.

### Folders with content-derived sides

Folders follow the same principle as envelopes — **a container has no side of its
own; its contents give it one** — but derived rather than stored:

- A folder's side is computed from its member elements: no members → **neutral**,
  any income member → **income folder**, any expense member → **expense folder**.
  Both reads already load all elements, so derivation is a free pass over data in
  hand — no schema change, and no stale state (removing the last income element
  reverts the folder to neutral automatically).
- **Enforcement**: exactly two writes assign an element a `folderId` —
  `MoveElement` (`POST /api/v1/budget/move-element`) and `create-envelope`
  (which takes an optional `folderId`) — and both reject a placement that would
  mix sides, with a coded error sibling to the envelope homogeneity one.
  Because envelope sides are immutable and `syncElements` updates types in
  place, no other path can mix a folder. The UI never offers cross-side
  targets; the error is the API backstop. Implementation notes: `MoveElement`
  matches the element by external id with no type discriminator and silently
  no-ops on unknown ids — the side check slots in where the element is
  resolved.
- **`get-budget` filters income-sided folders out**; neutral (empty) folders keep
  appearing exactly as today. So: create an empty folder → it shows in both views
  and moves like any folder; give it an income member → it disappears from the
  budget view and lives in the plan's Income section. The plan read returns all
  folders.

### Frozen contract: get-budget unchanged

`get-budget` never filters by element type today — income is excluded by the
`IsIncome` checks over category metadata (`buildFilters` feeds every builder;
`seedCategoryElements`, `syncElements`, and `envelopeChildren` cover the write
side), and those checks stay untouched. The new income-typed rows are inert in
the expense builders because every element-row lookup there is keyed
`<externalId>-<typeAlias>` — income aliases match nothing. `get-budget` gains
exactly one new behavior: income-sided folders (any income-typed member) drop
out of its folder list; neutral folders keep appearing as today.
The apiparity goldens for `get-budget` (and every other existing route) must come
out **byte-identical** — that diff is the regression proof. The stock fixtures
hold no income elements, so the suite also gains a scenario that creates income
elements and planned income and re-asserts `get-budget` output unchanged (the
goldens alone cannot prove what the fixtures never exercise).

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
the side, so a dedicated array would duplicate it. Income elements may carry a
`folderId` like any other element (see folder sides below); the SPA renders the
income rows — foldered or loose — under the synthetic Income section, one more
bucket in the existing `bucketElements` pattern. `structure.folders` in the plan
response includes **all** folders: income-sided, expense-sided, and neutral. One
builder pass on the server, one shape for clients.

`PlanElement`: id, type, name, icon, currencyId, isArchived, position, folderId?,
ownerUserId?, `cells: [{actual, planned}]` aligned with `months` (`planned` is `""`
when no limit row exists; amounts are decimal strings in the element's currency),
`children: [{ …, cells: [{actual}] }]` for envelope children (planned lives on the
parent, exactly like `budgeted` today). **Two** synthetic Uncategorized rows appear
with actual-only cells: the expense one (as on the budget page) and an
**income-side Uncategorized** for income transactions with no category — without
it, category-less income vanishes from the Income totals and the Balance row can
never reconcile with the opening balances. All existing wire conventions hold
(datetime layout, `isArchived` as 0/1, decimal strings, envelope with HTML
escaping off).

- `openingBalances` — per-currency account balances at the start of `months[0]`,
  computed by the budget read repo's `AccountsBalancesOnDate` (already takes an
  arbitrary as-of date and the excluded-accounts-respecting
  `includedAccountIDs`). Seeds the client-side Balance row.
- `currencyRates` — the existing per-period average-rate computation, once per
  month in the window. Future months are already handled: `AverageRates` snaps
  to the latest published rate's month, so every month in the window carries
  rates. The server computes **no totals**; the SPA does FX and totals
  client-side, as the budget page does.
- **Reconciliation note** — the Balance row is a projection, not an accounting
  identity. Its seed is real account balances, but the per-month nets sum budget
  rows: categorized income/expense plus the two Uncategorized rows. Not
  captured: exchange gains/losses on transfers (`amount != amount_recipient`)
  and transfers crossing the excluded-accounts boundary. That residual drift is
  accepted for v1 — it re-zeroes at every fetch-window start, since the seed is
  recomputed there — and the income-side Uncategorized row exists precisely to
  close the largest gap.

### Queries

Three grouped-by-month additions to `internal/budget/repo/read.go` (already
hand-built per-engine dynamic SQL, so the dialect split is at home there):

- spending by month: expense transactions summed per (month, category/tag, currency)
  over `[from, to)`;
- income by month: same for income transactions per (month, category, currency);
- limits by month: `budgets_elements_limits` per (external_id, type, period).

Both by-month transaction queries take the budget's `includedAccountIDs` and
resolve classifications over the **participant-owner** set (`CategoriesByOwners`
over the budget's user ids), mirroring `CountSpending` — a shared account's
transactions carry the account *owner's* classifications, so resolving over the
caller instead would misfile co-sharers' rows as uncategorized. The income query
keeps `category_id IS NULL` rows: they feed the income-side Uncategorized row.

Month bucketing is `strftime('%Y-%m-01', …)` on SQLite vs `date_trunc('month', …)`
on PostgreSQL; results must normalize to byte-identical wire output (enginecompare
covers it). **No carryover loop** — a plan cell is just actual + planned, so the
endpoint is O(1) queries regardless of window size (today's `get-budget` runs one
spending query *per month since the budget started*; the plan read must not
inherit that shape).

## Frontend

**Guiding principle: it's a spreadsheet.** The plan view should look and feel like
a familiar spreadsheet with dead-simple navigation — that is the main focus, and
every UI decision below serves it: a clean row/column grid with sticky headers,
arrow buttons and arrow keys to move through months, click/Enter on a cell to
edit, no modes or configuration beyond what fits on the sheet itself. When a
detail is in doubt during implementation, resolve it toward "what would a
spreadsheet do".

### Mode toggle

`BudgetPage` gets a **Budget | Plan** segmented toggle, persisted in
`useBudgetPeriodStore` alongside the selected period. Plan mode reuses the page
header (budget picker, currency) and replaces the month strip + budget table with
the plan sheet.

### The sheet

- **Responsive column count**: the grid measures its container (ResizeObserver);
  after the sticky row-name column it shows as many month columns as fit at a
  comfortable minimum column width, from 3 up to 12, reflowing live on resize.
  Below the 3-column fit threshold (phones) the sheet collapses to a **single
  month column** — never an awkward two — and the arrows become a month switcher.
  Plan mode is expected to live mainly on desktop; the mobile single-month view is
  a deliberate degradation that keeps the two-value cells, sections, and the
  totals block fully usable.
- **Navigation**: left/right arrow buttons shift the first visible month by one
  month (press-and-hold repeats). Navigation clamps at the budget's start month;
  there is no forward limit.
- **Initial position**: the current month sits in the **second column** — one past
  month for comparison, the rest of the width looking forward — regardless of how
  many columns fit; clamped at the budget start (a budget starting this month shows
  the current month first). In the single-column mobile view the initial month is
  simply the current month. The first visible month persists in the budget store
  and is restored on return.
- **Current month column** is visually highlighted.
- **Hard layout invariant: income on top, expenses under it — always.** Every row
  (folder, envelope, category, tag) renders in exactly one of the two areas,
  determined solely by its side; the two areas never interleave, whatever folders
  or envelopes exist and however they are ordered.
- Sections top to bottom: **Income** — a single fixed section header (styled like a
  folder, but synthetic: no `budget_folders` row, so it cannot be renamed, deleted,
  or receive expense elements) holding income-sided folders, income envelopes
  (expandable to children), and standalone income categories; then the **expense
  structure** exactly as the budget page orders it (folders as section headers,
  elements, uncategorized, archived); then the **Totals block**. Neutral (empty)
  folders render in the expense structure area, as on the budget page, until they
  gain a side.

### Cells

- Each cell: actual on top (muted, read-only; rendered as a dash when zero in a
  future month), planned below (editable where permitted). In the current and
  future months an expense cell whose actual exceeds its planned value gets a
  subtle overspend highlight — that cell's actual is what feeds the Balance
  projection, so the user can see where the plan is already broken.
- Editing reuses the existing machinery — `LimitEditor` popover with
  `CalculatorInput` on desktop, `SetLimitDialog` on compact screens — after a
  small but required refactor: today `LimitEditor`, `SetLimitDialog`,
  `useSetLimit`, and `canUpdateLimits` are all period-implicit (the month is
  injected from the store's global `selectedDate` inside the hook, and
  `useSetLimit`'s optimistic update patches the single-month `get-budget` cache
  key; nothing invalidates). Each gains an explicit period parameter; plan-view
  commits call `set-limit` with the cell's month, optimistically patch the plan
  query's cache, and invalidate the budget query for that month. The wire form
  already takes any first-of-month `period`, so the API needs nothing.
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
`budgetMath` FX helpers (note: the shared `exchange` falls back to converting
1:1 silently when a rate is missing — acceptable on the budget page's single
month, but a sheet summing a year of FX should at least be aware of it):

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
existing envelope create/edit dialogs for members with edit rights. Row menus in
the plan view offer a **"Move to folder…"** action — net-new UI over the
existing `move-element` endpoint (the budget page changes folder membership only
via drag today; there is no such menu to reuse) — whose target list is
side-filtered: an income row sees income and neutral folders, an expense row
sees expense and neutral folders. Drag re-ordering of income rows stays out of
scope for v1 (seed order = category order).

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
  homogeneity rejection (with the coded error); folder-side derivation
  (neutral/income/expense, reverting to neutral when emptied) and cross-side move
  rejection; income elements and income-sided folders absent from `get-budget`
  output; `months` bounds validation. **Reconciler regressions** (the
  delete-unseen pass): income elements *and their limits* survive envelope
  create/update/delete and `move-element`; the envelope walk ensures the stored
  side's type (no `UNIQUE(budget_id, external_id)` violation, no
  delete-and-recreate); the dirty-data cleanup removes income children from
  existing expense envelopes; the income-side Uncategorized row aggregates
  category-less income.
- **Frontend (vitest)**: window math (responsive count, initial anchoring,
  clamping), totals + Balance rolling math (actual for past months; per-cell
  `max(actual, planned)` for current/future, including the overspent-category
  case),
  a grid-cell edit component test; keyboard navigation (arrow movement, window
  shift at the visible edge, Enter-to-edit/Esc, read-only cells inert);
  metrics-coverage; `pnpm exec tsc -b` before claiming done.

## Out of scope (v1)

- Drag re-ordering of income rows and folders in the plan view (assignment happens
  via the "Move to folder…" row menu; full dnd arranging stays on the budget page
  and remains expense-side only).
- Per-cell carryover ("available") display.
- Server-side totals.
- Any change to the budget page's single-month view or its wire contract.
- Archive/unarchive actions in the budget/plan row menus (deferred; the
  `archive-category`/`archive-tag` + unarchive endpoints already exist, so this is
  a frontend-only follow-up — until then the classifications pages remain the
  place to archive).
