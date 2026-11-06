# Custom budget period start day (#279)

Status: design, 2026-10-28. Issue: https://github.com/econumo/econumo/issues/279 (milestone
v1.9, promised to users before the end of 2026).

## Why

People who are paid on the same day every month plan from payday to payday. A user paid on
the 28th budgets 28 September – 27 October. Today Econumo cuts every budget at the 1st, so
the salary and the fixed payments made on the 28th land in one month while the spending they
fund lands in the next. One pay cycle is split across two budget months, and "what is left
until the next payday" cannot be read anywhere. Moving transaction dates to the 1st would
tidy the budget but falsify the account history, so users must not have to do that.

## Goals

1. **A budget can start its months on any day from 1 to 28.** The setting belongs to the
   budget (`period_start_day`, default 1), so everyone sharing a budget sees the same
   periods. It is chosen when the budget is created and can be changed in the budget's
   settings later.
2. **Every budget figure follows the shifted period**: spent / actual, plans, Available,
   carry-over, balances, savings saved and closing balances, the Plan grid's columns, the
   transaction lists opened from a cell, upcoming money (future-dated rows and recurring
   payments still due), plan hints, "current month" rules, and the MCP budget tools.
3. **Transaction dates are never touched.** Account histories show the real dates.
4. **Calendar months stay the default.** A budget with start day 1 shows exactly the same
   figures as today.

## Non-goals

- Reports (spending, cash flow, net worth) stay on calendar months.
- No "last day of the month" or "last working day" option; 1–28 is enough (user decision).
- No per-user start day.
- Recurring transaction schedules, the Inbox, account pages and CSV import/export are not
  affected.

## Anti-goals (a failure even if it works)

- Rewriting stored data: plans, comments, start/end months, or transaction dates.
- Any change of figures for a budget left on day 1.
- Two participants of a shared budget seeing different periods.

## Decided

### Period naming and keys (user decision)

A period keeps being identified by a month, stored and sent as `YYYY-MM-01` exactly as now
(`budgets.started_at`, `budgets.ended_at`, `budgets_elements_limits.period`, comment
periods, request `periodStart`, Plan columns, MCP `month`; the one exception is the
`get-budget` `filters` block, below). Only the date range that key covers
changes. Every period is named after the month it starts in: key `M` covers
`[M-d, (M+1)-d)` for every `d`.

- `d = 1`: key `M` covers `[M-01, (M+1)-01)` (today's behaviour).
- Example `d = 5`: "October" = 5 Oct – 4 Nov.
- Example `d = 15`: "September" = 15 Sep – 14 Oct.
- Example `d = 28`: "October" = 28 Oct – 27 Nov; "September" = 28 Sep – 27 Oct.

Here `X-d` means day `d` of month `X` at 00:00. Because `d <= 28`, every month has that day
and no clamping is ever needed.

**Changed on 2026-11-04.** The first version of this design named a period after the
month it ends in from day 16 on (`16 <= d <= 28`: key `M` covered `[(M-1)-d, M-d)`, so
"November" was 28 Oct – 27 Nov at `d = 28`). After the issue reporters' feedback on #279
the rule became start-month naming for every day. Nothing had been released, so no data
moved.

The naming rule must live in exactly one helper per stack (below), so it can be changed
later. Changing it after release is a code change plus a migration: for budgets on the
affected days, move `started_at`, `ended_at`, limit and comment periods by one month so each
plan stays on the dates it was made for.

The inverse, the period a date belongs to: subtract `d - 1` days and take the first of that
month. Example `d = 28`, 30 Oct: 30 Oct − 27 days = 3 Oct → October ✓; 5 Oct − 27 days =
8 Sep → September ✓.

Bounds are day boundaries on the stored `spent_at` wall clock, exactly like today's month
bounds.

**Dates in requests name a key** (builder-check call): every request date that picks a
period — `get-budget` `date`, `create-budget`/`clone-budget` `startDate`, `update-budget`
`endDate`, the MCP `month`/`start_date`/`end_date` — keeps snapping to the first of its
month, and that month is the key. The day part is ignored, as today. ("The period
containing this date" was rejected: for `d = 5` the SPA's `2026-10-01` for "October" would
then resolve to September.) The MCP descriptions say the value names the budget month and
the day is ignored.

**`filters.periodStart` / `filters.periodEnd` carry the real window** (builder-check call):
in the `get-budget` response (REST and MCP) these two fields become the shifted date range
the figures were computed over (e.g. `2026-10-28 00:00:00` / `2026-11-28 00:00:00` for
"October" at `d = 28`); for a day-1 budget they are unchanged. They are the one place a
client or an MCP agent can read the real dates, and the SPA can show the range from them
instead of computing it. `currencyRates[].periodStart/periodEnd` keep reporting the rate
period: the calendar month for a day-1 budget, the shifted window itself once it holds rates
(see Currency conversion below).

### "Today" and the current period

The caller's current period is the period containing the caller's local today (`X-Timezone`,
as now). On 30 October a budget with `d = 28` opens on "October" (28 Oct – 27 Nov); on
5 October it opens on "September" (28 Sep – 27 Oct). Everything that today
asks for "the current month" asks for the current period instead: the default month when
opening a budget, the default start of a new budget, the Plan window that must include the
current month, plan hints and the still-due list (current period onward), the membership
removal window (`budget.account_not_removable`: no transactions between the start of the
budget's first period and the start of the current period), the period an overdue recurring
payment is counted in, and the MCP defaults.

### Setting the day (user decision)

- `create-budget` takes an optional `periodStartDay` (absent or 0 → 1).
- `update-budget` takes an optional `periodStartDay` (absent → untouched).
- `clone-budget` copies the source's day.
- Allowed values 1–28; anything else is a field error on `periodStartDay` with a new
  catalogue code (e.g. `budget.invalid_period_start_day`, translated in all 11 languages).
- The archived-budget rule is the same as for the rest of `update-budget`; changing the
  day needs the owner or an admin (below).
- **Changing the day on an existing budget is allowed, without a warning.** Plans and
  comments stay on their month keys and are simply read against the new date ranges. The
  update dialog just lets the user pick another day (user decision 2026-11-04; an earlier
  draft warned before saving).
- **Keys never move: `started_at` is the first period's key** (review fix, 2026-11-04, after
  two rejected attempts to move or derive the start). A day change writes only the day:
  the first key (`started_at`, `Budget.FirstPeriod()` = its first of month), the end key,
  plans and comments stay put, so changing the day back restores the budget exactly and an
  end month can never end up before the start. `create-budget` stores the current period's
  key by default and an explicit `startDate`'s key; `clone-budget` works in keys throughout
  (its limit and comment cutoffs are the copy's first key), from the source's first key to
  the later of the caller's current period and that first key.
- **A budget that has not started yet.** Today can fall before the first key's window: a day
  change on a brand-new budget (key October, day 1 -> 28 on 5 October puts today in
  September's period) or an explicit future start. This is treated exactly like a budget
  that has not started: the server's default period for `get-budget` / `get-budget-plan`
  (no date) and the create/clone responses open on the later of the current period and the
  first key, the SPA's default-month adoption clamps up to the first key the same way
  (`budgetCurrentPeriod`), so the page opens on the first month and never greys it out, and
  the Complete/Duplicate dialogs never offer an earlier month. Plans and comments on the
  first key work as always. Days before the first key's window (1–27 October in that
  example) belong to no period, like any days before a budget's start.
- On day 1 a legacy `started_at` with a time past the first of its month keeps its old
  carry-over walk (month steps from `started_at` itself, AddDate clamping a 31st start) and meta value, so day-1 budgets stay
  byte-identical; on other days the walk runs over plain period windows from the first key.
- **Changing the day is owner/admin only** (review fix, 2026-11-04): it reshapes every
  period for every participant, its first one included, so like sharing it needs the owner
  or an accepted admin (the SPA's `canConfigureBudget`); a `user`-role participant gets the
  usual access-denied error for a different day, while resending the stored day is no
  change.
- The budget's meta on the wire (`MetaResult`, every place it appears) gains
  `periodStartDay` (int, additive).
- The MCP `create_budget` / `update_budget` tools gain `period_start_day`; `get_budget`
  shows it through the meta; tool descriptions mention that a budget month may start on a
  day other than the 1st.

### Header (user direction, to be iterated later)

The header and month switcher keep showing the month name ("October"). The shifted date
range (e.g. "28 Oct – 27 Nov") must be visible somewhere for a budget whose day is not 1;
its final spot will be designed with the user afterwards. For this change the builder puts
it in a simple, unobtrusive place and keeps it easy to move. A day-1 budget shows no range.
Placement settled with the user (2026-11-05): the SELECTED month of the Budget view's period
strip carries its range as a small muted second line under the name, styled as the Plan
grid's month header (which shows every column's range); the other months show their names
only, and nothing sits at the end of the strip.

### Analytics

The create event carries the chosen day; changing the day on an existing budget fires a new
`METRICS` key (e.g. `BUDGET_PERIOD_START_DAY_CHANGE`) at the update's success point, with
the new day as event data.

## How (suggested; builder may refine)

### One definition of a period, per stack

- **Go**: helpers on the model, e.g. `model.PeriodWindow(key, d) (start, end time.Time)` and
  `model.PeriodOf(t, d) time.Time` (or methods on `*model.Budget`), in
  `internal/model/budget.go` next to `FirstOfMonth`. Exhaustive unit tests: every `d` 1–28,
  31/30/28/29-day months, leap years, December→January, and `PeriodOf(PeriodWindow(k).start)
  == k` / `PeriodOf(end - 1s) == k` round trips.
- **TS**: the same pair in a new `web/src/features/budgets/period.ts`, with the same tests,
  replacing `currentMonth()` and friends in `planMath.ts` wherever they mean a budget
  period.

### Server call sites (about 65)

Today period *keys* and date *windows* are the same `time.Time` (`periodStart` and
`periodStart.AddDate(0, 1, 0)`). Each site must be split into "key" and "window":

- `read.go`, `builder.go`, `txlist.go`, `builder_plan.go`, `builder_savings.go`,
  `builder_structure.go` (carry-over loop from `StartedAt`), `accounts.go` (removal
  window), `comments.go`, `lifecycle.go`, `clone.go`, `create.go` (`localMonth`),
  `usecase.go`, `projected_read.go` (`firstOfMonth(o.FiledAt)` → period of the occurrence),
  `dated_read.go` / `upcoming_merge.go` (window ends), `plan.go`.
- `buildFilters` and everything handed to the repo should carry window bounds; keys stay for
  limits/comments lookups and the response.
- Currency conversion: pass the shifted window as `PeriodStart`/`PeriodEnd` to
  `BulkConvert` (average rates over the window). The rate provider snaps a requested range
  to the calendar month of the latest rate before its end, and that snap decides the rates
  used for conversion, not only the reported `currencyRates` period: a day-5 "October"
  (5 Oct – 4 Nov) would be converted at November's average once a November rate exists.
  So budgets whose day is not 1 convert through a windowed rate provider
  (`RateProvider.Windowed()`, injected with `budget.Service.SetShiftedPeriodRates`): a
  window holding rates averages exactly those, and `SnappedRatePeriod` reports that window
  (review fix, 2026-11-04). Day-1 budgets (legacy mid-month windows included), imports and
  every other caller keep the default provider and its month snap, so they convert exactly
  as before; a shifted window holding no rate still falls back to the latest rate's month.

### SQL month grouping

The Plan queries in `internal/budget/repo/read.go` group by calendar month in SQL
(`planMonthExpr`). Replace that with grouping by **day** (`substr(col, 1, 10)` on SQLite,
`to_char(col, 'YYYY-MM-DD')` on PostgreSQL — the same bytes on both engines, and no SQLite
date functions, which the file avoids on purpose) and bucket the days into period keys in
Go with `PeriodOf`. The limits query keeps grouping by its `period` column (it already
holds keys). Bounds passed to these queries are window bounds.

### Storage

A migration on both engines adds `budgets.period_start_day INTEGER NOT NULL DEFAULT 1`;
update the sqlc queries (both engines) and regenerate; check `data:import-sqlite` copies
the new column.

### SPA

- `BudgetDialog` / `BudgetUpdateDialog`: a "Month starts on" day picker (1–28, default 1),
  using the existing picker components; the update dialog shows no warning when the value
  changes.
- Every place that derives a budget month from a date or a date range from a month goes
  through `period.ts` with the budget's `periodStartDay`: current month, plan window,
  `planHint.ts`, `UpcomingPaymentsPanel`, phone month view, Plan grid, any client-side
  "dated after today in this period" logic. Requests keep sending the month key
  (`periodStart`); the server resolves the window.
- The date-range display described above.
- New strings in all 11 catalogues (respect each catalogue's register).

### Tests and docs

- Go: model period tests; budget service/repo tests with `d = 28` and `d = 5` (spent split
  at the boundary, carry-over, savings saved/closing balance, upcoming and overdue
  recurring, removal window, end month); both engines (`make test-repo-pgsql`).
- `apiparity` scenarios for create/update/clone with `periodStartDay` and a `get-budget` /
  `get-budget-plan` on a shifted budget; regenerate goldens and inspect (every budget meta
  gains `periodStartDay: 1`, nothing else should change for existing scenarios).
  `mcpparity` likewise.
- Web: `period.ts` tests, dialog tests (picker, unchanged day not sent), a shifted-budget page test.
- `docs/regression-test-plan.md`: add items for the setting, a shifted budget's figures and
  lists, changing the day with existing plans, and a shared budget seen by both users.
- `CLAUDE.md`: a "Notable behaviours" bullet describing the period rule.

## Left to the builder

- Exact Go/TS helper names and where the key/window split is cleanest.
- The picker's form.
- Error code name.

## Delivery

One feature branch `feature/budget-period-start-day` off `main`, draft PR, Codex review
before reporting done.
