# Reports v1 — design

Status: draft for review (2026-10-22). Follow-up project, not in this spec:
"What will happen to my money?" (forecast + "can I / when can I afford X").

## Why

Econumo shows one month at a time (Budget) and plans forward (Plan), but has no way
to look back across months and years. The motivating question, in the owner's words:
how much did we spend on groceries (or any category) over time, how did it grow, and
how does this year compare with last year. Sometimes the long view across years,
sometimes just "this year vs last year".

Research (other trackers' report sets and feature boards — YNAB, Monarch, Copilot,
Actual, Lunch Money, Simplifi) points the same way: users expect spending by category
over time with period comparison, income vs expenses, and net worth. "Budget vs actual"
is the most requested report elsewhere, but Econumo already answers it in Budget and the
Plan grid (Plan · Actual per cell, multi-cell sums), so it is deliberately not a report
here.

## Goals

- Look back over any range, up to all history, and see how spending changed — as a
  chart first, details in a table below.
- Compare two periods like-for-like (Jan–Oct 2026 vs Jan–Oct 2025).
- Three reports, each titled as the question it answers:
  1. **What do I spend money on, and how is it changing?** (Spending)
  2. **Do I earn more than I spend?** (Cash flow)
  3. **Am I getting richer or poorer?** (Net worth)
- Two scopes: **a budget** (everything the budget counts, partners' member accounts
  included, grouped the way the budget is built) and **my accounts** (the user's own
  accounts, grouped by their own categories).
- Report **tabs**: several reports open at once, each with its own parameters, quick
  switching, renameable, stored on the device.
- Works well on a phone (the iOS home-screen PWA is in daily use) and on desktop.

## Non-goals (v1)

- Forecast and "can I afford X / when can I afford X" — separate project; the new-tab
  report list reserves a greyed "What will happen to my money?" entry for it.
- A separate "Budget vs actual" report (see Why). Spending carries a planned line instead.
- A custom report builder, user dashboards, saved reports on the server, syncing tabs
  across devices, export (CSV export already exists elsewhere), MCP tools for reports.
- Reports on the Home page or in the Inbox.

## Anti-goals (failure even if it "works")

- **Wrong numbers.** A report figure that disagrees with the Budget page or Plan grid for
  the same budget, rows and months. Users stop trusting both screens.
- **Bad UI.** Cluttered, table-first, hard to read on a phone, pies with fifteen slices.

## Decided

### Navigation and tabs

- New top-level route `/reports`, a sidebar/nav item beside Budget and Plan.
- The page is a set of **tabs**. Each tab holds one report (or the report list) plus all
  its parameters (scope, period, mode, grouping, step, selected row, comparison periods,
  expenses/income switch).
- **"+" opens a new tab showing the report list**: one card per report, titled as its
  question, each with a headline number, a tiny chart and the one thing worth noticing
  (e.g. "Groceries grew most: +$1,870"). The forecast card shows greyed "Coming later".
  Choosing a card turns that tab into the report. The list has a scope picker that the
  chosen report inherits.
- Tabs can be **renamed** (desktop: double-click the tab; phone: long-press the pill →
  Rename / Duplicate / Close), closed, and reordered by drag. Default tab name is
  generated from the report and its main parameter (e.g. "Groceries", "Net worth",
  "2026 vs 2025") until the user renames it.
- Desktop: a tab strip across the top. Phone: a horizontally scrolling row of pills.
- **Stored locally** (this device/browser, e.g. localStorage keyed by user id). On
  opening Reports, the **first tab** is shown (not the last-used). First ever visit:
  one tab showing the report list. Closing the last tab leaves one report-list tab.
- Local storage must be cleared/ignored for another user (same rule as other per-user
  local state), and tolerate a stored parameter that no longer resolves (a deleted
  budget, category or account): fall back to a sensible default, never crash.

### Report anatomy (all three)

1. Headline figure + one plain-language sentence ("Spent $34,900 this year, 14% more
   than last year; Groceries grew most").
2. The chart.
3. A detail table below with totals, monthly average and change.
4. Drill-down: tapping a bar, a legend item or a table row narrows the chart to it;
   from a narrowed view, tapping a period opens the matching transactions (reuse the
   existing transaction list UI, filtered).

### Shared parameters

- **Scope:** a budget (the user's accessible budgets) or "My accounts".
- **Period:** presets (this year, last 12 months, last 3 years, all time) and custom.
- **Step:** month / quarter / year (where the chart is over time).
- Amounts shown in the budget's currency (budget scope) or the user's base currency
  (my accounts).

### 1. Spending — "What do I spend money on, and how is it changing?"

- Switches: **Expenses / Income**; group by **Category / Payee / Tag**.
- Mode switch **Over time / Compare years**:
  - **Over time:** bars per step across the period, each bar stacked by the top few
    groups plus "Everything else". The current, unfinished year (or quarter) is drawn
    projected to the full period with a dashed outline so growth does not look like a
    drop; the table shows actual-to-date beside it. Headline: period total and change
    vs the previous period.
  - **Compare years:** two periods side by side as paired bars per month (earlier in
    grey, later in colour), for one selected group or total. Always like-for-like
    ranges (if the later year is incomplete, both are cut to the same months).
- Selecting one row (legend or table) shows only that row in the chart — the
  "groceries over the years" case.
- **Planned line:** in budget scope, when one budget row is selected and step is month,
  overlay that row's planned amount per month as a line (from the budget's plans).
- Table: group, earlier period, later period, change (money and %), monthly average;
  sorted by biggest change so "what changed" is first.
- Budget scope groups by the budget's own rows (categories, envelopes, tags, folders)
  exactly as the Budget page does; an envelope drills into its categories.

### 2. Cash flow — "Do I earn more than I spend?"

- Monthly (or per step) income bars above a zero line, spending below, dashed net-saved
  line. Headline: saved amount and share of income.
- Table per period: income, spent, saved.
- A **Flow** view swaps the chart for a Sankey: income → spending groups → savings, for
  the chosen period. Desktop only in v1; on a phone the Flow switch is hidden.

### 3. Net worth — "Am I getting richer or poorer?"

- Stacked area of account balances over time grouped by account folder; debts
  (negative balances) below zero; a line for the total.
- Headline: today's total and change over the period.
- Table: folder, balance at period start, now, change. Drill into a folder → its
  accounts.
- Budget scope = the budget's member accounts (partners' included); My accounts =
  the user's own. Foreign-currency balances converted at each month's rate.

### Numbers rule (the main constraint)

Budget-scope figures must equal what Budget and Plan show for the same budget, rows
and months: same treatment of transfers, savings members, deleted accounts, archived
rows, carried-over amounts excluded, same currency conversion. The way to guarantee it
is to compute budget-scope reports **on the server through the budget feature's existing
read path** (the plan builder already produces per-row actuals per month over a window:
`BuildBudgetPlan`, `SpendingByMonth`, `IncomeByMonth`, `TransfersByMonth`,
`AccountsNetByMonth`; conversion via `currency.Convertor.BulkConvert`), not by
re-deriving them in the browser from the IndexedDB transaction store. Tests must assert
report totals == Plan actuals for the same window.

"My accounts" scope uses the same rules applied to the user's own accounts and
categories.

**Decided (owner, 2026-10-22): new server read API for both scopes; the browser only
draws.** Report data comes from new `GET` endpoints (one per report) that aggregate by
month in SQL and reuse the budget feature's monthly queries and the currency conversion.
No report aggregation in the browser, and no split where one scope is computed
client-side. Responses go into the persisted query cache so a previously opened report
still renders offline (with its "as of" time). Transaction drill-down uses the existing
locally stored transaction list.

### Scope rules (designer's calls — check these)

- **Group by in budget scope:** the switch offers only the budget's own rows (tags and
  envelopes are already rows there). Payee and Tag grouping are My-accounts-only in v1,
  so every budget-scope figure has a Plan figure to be tested against.
- **My accounts** = accounts the user owns, hidden-folder accounts included, deleted
  accounts included for the months they had activity (so history doesn't shrink when an
  account is deleted). Accounts shared with me are not mine; they appear through a
  budget's scope. Transfers between my own accounts are ignored. A transfer to or from an
  account outside the set is not spending or income; Cash flow shows it as a separate
  "transferred out / in" figure.
- **Cash flow "saved"** = income − spending, which matches the sum of Plan's income rows
  minus its expense rows for the same months. In budget scope the Sankey splits "saved"
  into "moved to savings accounts" (Plan's Saved for savings members) and "stayed in
  everyday accounts"; transfers in/out of the budget's member set are a separate node.
- **Net worth balances** are converted at the same month-average rates the Plan uses for
  savings closing balances (revised during planning: no month-end rate exists in the
  rate store, and this keeps net worth consistent with Plan's savings figures); net worth
  is not part of the Plan parity tests. The server returns per-account monthly balances
  already converted; grouping by folder/owner is a client-side sum. In budget
  scope the grouping is by owner first: my accounts grouped by my folders, each partner's
  accounts listed under the partner's name, without exposing their folder names.

### History before a budget started (owner's decision)

Budget scope **shows months before the budget's start date**: "All time" begins at the
first transaction on any of the budget's member accounts, grouped into the budget's
current rows with the current membership (deleted members included). The parity tests
apply to months inside the budget's window; earlier months use the same rules but have
no Plan figure to compare. The Plan endpoint caps its window at 24 months
(`planMonthsMax`), so multi-year report ranges need their own read path rather than a
lifted Plan cap. Designer's call: months after an ended budget's end month are not shown,
matching every other budget reader. Planned lines only exist where plans exist.

## Build order

1. Tabs shell + report list + Spending in **My accounts** scope (the owner said
   personal reports are the easier start).
2. Budget scope for Spending, including the planned line and the parity tests.
3. Cash flow (bars, then Sankey on desktop).
4. Net worth.

Each step is shippable on its own.

## Constraints from the project

- API conventions: `GET` reads under `/api/v1/<module>/get-…`, envelope, apiparity
  scenario + goldens, enginecompare parity (SQLite and PostgreSQL byte-identical).
- Features never import features; cross-feature data goes through ports + glue.
- Every user-facing action fires an analytics event (`METRICS`): at least report
  opened (with report kind), tab created/renamed/closed, mode switched, drill-down.
- i18n: all strings in every `locales/<lang>.json` (11 languages).
- `docs/regression-test-plan.md` gets Reports items (with phone markers).
- Charts: `recharts` and `web/src/components/ui/chart.tsx` are already in the web app;
  follow the dataviz guidance (consistent colour per category across charts, readable in
  light and dark themes). No bold for hierarchy (house rule).
- Large histories: all-time ranges over years must stay fast; aggregation in SQL by
  month, not per-transaction in Go.

## Left to the builder

- Endpoint shapes and whether reports live in the `budget` feature, a new `report`
  feature with ports, or both (the numbers rule above is the constraint, not the
  package layout).
- How many "top groups" a stacked bar shows before "Everything else" (suggest 5).
- Exact projection method for the unfinished period (suggest: actual-to-date scaled by
  elapsed share, labelled as projected).
- The insight-sentence rules (biggest change, share of income, etc.) — keep them few
  and deterministic.
- Tab limit (suggest a soft cap, e.g. 12) and the exact local storage format.
- Whether the active tab is also reflected in the URL for browser back/forward.
