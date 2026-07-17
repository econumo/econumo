# Recurring Transactions — Design

**Date:** 2026-07-14
**Status:** Approved

## Summary

Recurring transactions let a user schedule repeating payments (rent, salary,
subscriptions, savings transfers) as **templates** that are **posted manually**
— nothing is ever created automatically. Each template owns a single
**next-payment date** and appears as one **virtual row** in the account
transaction list at that date; posting it creates a real transaction and
advances the schedule by one interval.

## Decisions (validated with the user)

| Question | Decision |
|---|---|
| Future rows in the transaction list | Virtual projection from a separate template entity; nothing exists in `transactions` until posted; no balance/budget effect until posting |
| Schedules supported | Preset intervals: `weekly`, `biweekly`, `monthly`, `quarterly`, `yearly` |
| Posting semantics | Advances next-payment by one interval **from the scheduled date** (not from today); a **skip** action advances without creating a transaction |
| Occurrences shown | Exactly **one** virtual row per template (its next-payment date); an overdue template's row sits at its past date until posted or skipped |
| Creation entry points | Both: "make recurring" from an existing transaction (prefills a template; source transaction untouched) AND from scratch on the recurring page |
| Transaction types | All three: expense, income, transfer |
| Cross-currency transfers | Template stores **only the source amount** (no `amountRecipient` field); the posting dialog prefills the recipient amount from the current rate, editable before confirming |
| Shared accounts | Templates follow account access like transactions: anyone with account access sees them; writes follow the owner/admin/user matrix, guests read-only |
| Lifecycle | Delete only — no pause, no end date |
| Navigation | No sidebar entry; "Recurring transactions" lives in Settings → Finance section |
| Row tap behaviour | Always preview-first via a shared view modal (matching existing transaction behaviour); Post appears only in the account-list context |
| Posting date prefill | The template's existing next-payment date (not today) |

## Architecture

**Chosen approach:** new vertical feature package `internal/recurring` with
frontend-side merging of virtual rows (over backend merge into
`get-transaction-list`, and over schedule columns on the `transactions` table).

Rationale:
- Matches the codebase's feature-slice + glue architecture exactly.
- Every frozen wire contract stays byte-identical — `get-transaction-list` is
  untouched; a virtual row is not a transaction and never appears there.
- Posting gets atomicity-in-practice via the shared idempotency guard.

## Data model

### Entity: `RecurringTransaction` (`internal/model/recurring.go`)

Mirrors `Transaction` minus timestamps-of-fact and minus `AmountRecipient`,
plus scheduling:

| Field | Notes |
|---|---|
| `ID` | UUIDv7, client-suppliable on create (idempotency convention) |
| `UserID` | creator |
| `Type` | expense / income / transfer (same encoding as transactions) |
| `AccountID` | source account |
| `AccountRecipID *` | transfer recipient account (nil otherwise) |
| `Amount` | source amount only — no recipient amount is stored |
| `CategoryID *`, `PayeeID *`, `TagID *` | optional, as on transactions |
| `Description` | |
| `Schedule` | enum: `weekly` \| `biweekly` \| `monthly` \| `quarterly` \| `yearly` |
| `NextPaymentAt` | datetime, frozen wire format `2006-01-02 15:04:05` |
| `ScheduledDay` | 1–31, captured from the initial next-payment date; used by month-based schedules to clamp without drifting (monthly on the 31st → Feb 28 → **back to Mar 31**). Ignored by weekly/biweekly. |
| `CreatedAt`, `UpdatedAt` | |

### Advance logic

Pure function `nextOccurrence(current, schedule, scheduledDay)`:
- `weekly` / `biweekly`: add 7 / 14 days.
- `monthly` / `quarterly` / `yearly`: add 1 / 3 / 12 months, then clamp the day
  to `min(scheduledDay, daysInMonth(target))`.
- Always advances from the **scheduled** date — posting late never drifts the
  schedule.

Table-driven tests: February (leap and non-leap), 31st/30th clamping and
recovery, quarterly and yearly rollover.

### Table: `recurring_transactions`

Migrations for both engines (`internal/infra/storage/migrations/{sqlite,pgsql}`),
sqlc queries per engine, standard engine-adapter repo (querier interface in
canonical sqlite types + pgsql conversion shim).

Column types and FK semantics copy the `transactions` table exactly: `TEXT`
ids, amounts stored as transactions store them, nullable
`category_id`/`payee_id`/`tag_id` with the same ON DELETE behaviour, account
deletion cascades to its templates. Additional columns: `schedule TEXT`,
`next_payment_at`, `scheduled_day`. Index on `account_id`.

Reminder: sqlc `.sql` comments must be ASCII-only (em dashes mangle the v1.30
sqlite codegen).

## Backend feature package `internal/recurring`

One file per verb, package-level `Service`:

- `read.go` — `GetRecurringTransactionList`: all templates on accounts the
  caller can access. Consumed by both the settings page and the SPA's
  virtual-row merge.
- `create.go` — from-scratch creation; "make recurring" needs no special
  endpoint (the client prefills the create payload from an existing
  transaction).
- `update.go` — edit any field including schedule and next-payment date
  (`ScheduledDay` re-derives when the next-payment date changes).
- `delete.go` — hard delete.
- `post.go` — input: template id, a **client-generated transaction id**, and
  the (possibly tweaked) transaction fields from the dialog. Creates the real
  transaction through a `TransactionCreator` port, then advances
  `NextPaymentAt` by one interval from the scheduled date. Replay-safe via the
  shared idempotency guard keyed on the client-supplied transaction id — a
  retried post cannot double-create. Returns the created transaction plus the
  template's new `nextPaymentAt`.
- `skip.go` — advances the date only; no transaction is created.

`repository.go` — repository interface in `model` types.
`ports.go` — consumer-side interfaces: `TransactionCreator` and account-access
checking. `internal/server` wires glue adapters
(`glue_recurring_transactioncreate.go`, `glue_recurring_accountaccess.go`)
over the transaction/account services — features never import features.

Access rules: reads for anyone with account access; writes (create, update,
delete, post, skip) require owner/admin/user on the account (guests denied),
matching the transaction write matrix. Transfers check access on the source
account the same way transaction transfers do.

## API

Module `recurring`, registered in `internal/recurring/api/routes.go`, thin
handlers via the `endpoint.Handle` combinators with swag annotations:

```
GET  /api/v1/recurring/get-recurring-transaction-list
POST /api/v1/recurring/create-recurring-transaction
POST /api/v1/recurring/update-recurring-transaction
POST /api/v1/recurring/delete-recurring-transaction
POST /api/v1/recurring/post-recurring-transaction
POST /api/v1/recurring/skip-recurring-transaction
```

DTOs in `internal/model/recurring_dto.go`: `RecurringTransactionDto` mirrors
`TransactionDto` field names and frozen encodings (datetime format, alias
strings for `type`), plus `"schedule"` (alias string) and `"nextPaymentAt"`;
no `amountRecipient`. Standard response envelope everywhere.

`get-transaction-list` and all other existing contracts stay byte-identical.

## Frontend (`web/src/features/recurring/`)

- **Navigation**: a "Recurring transactions" item in the Settings page's
  **Finance** section → route `/settings/recurring`
  (`RecurringTransactionsPage`), registered in `routes.tsx` under
  `ApplicationLayout`. No sidebar entry.
- **`RecurringTransactionsPage`**: list of all templates across accessible
  accounts — description/payee/category, amount + account, schedule label,
  next payment date; overdue highlighted. A create button opens the template
  dialog. Tapping a row opens the view modal.
- **`ViewRecurringDialog`** (shared view modal, mirrors `ViewTransactionDialog`):
  template details + schedule + next payment date.
  - From the **account transaction list** (virtual row tap): **Post** is the
    primary action, plus skip / edit / delete.
  - From the **settings page**: skip / edit / delete (no Post).
- **Template dialog** (create/edit): reuses the transaction form machinery
  (`useTransactionForm`, `EntitySelect`, pickers) plus schedule select and
  next-payment date. No recipient-amount field for transfers.
- **Posting flow**: Post opens the existing `TransactionDialog` prefilled from
  the template with `spentAt` = the template's **next-payment date**; for
  cross-currency transfers the recipient amount prefills from the current
  rate; everything editable. Confirm calls `post-recurring-transaction` with a
  client-generated transaction id; on success, transaction-list and recurring
  queries invalidate.
- **Virtual rows**: `useAccountTransactions` (or a thin wrapper) merges one
  virtual row per template targeting the viewed account (source account only
  for transfers), positioned by `nextPaymentAt`, rendered by `TransactionRow`
  in a distinct "scheduled" style (muted + repeat icon), excluded from any
  running-balance math. Tap → `ViewRecurringDialog` (account-list variant).
- **"Make recurring"**: an action on an existing transaction (in
  `ViewTransactionDialog` / row menu) opening the template dialog prefilled
  from that transaction; the source transaction is untouched.
- **Plumbing**: `web/src/api/recurring.ts` typed client + DTOs, query keys in
  `queryKeys.ts`, `features/recurring/queries.ts`, i18n strings in all
  locales.

## Edge cases

- **Overdue**: a past `nextPaymentAt` keeps its single virtual row at that
  past date until posted or skipped; advancing is always from the scheduled
  date.
- **Month-end**: `ScheduledDay` clamping (above); quarterly/yearly reuse the
  same month-add + clamp.
- **Deletions**: account delete cascades to templates; category/payee/tag
  deletion follows the same FK semantics as `transactions`.
- **Permissions**: UI hides write actions from guests; the backend enforces
  regardless (error envelope on denied writes).
- **Balances/budgets**: virtual rows never affect account balances, projected
  balances, or budgets — only posted (real) transactions do.

## Testing

- Table-driven unit tests for `nextOccurrence`.
- Use-case tests per verb, including post idempotency replay and
  access-denied paths for each write.
- Repo integration tests on sqlite and pgsql (`make test-repo-pgsql`).
- apiparity scenarios + goldens for all six routes (guard-enforced), which
  also enrolls them in the `enginecompare` byte-identical suite.
- Frontend vitest: virtual-row merge logic, template dialog,
  `ViewRecurringDialog` action variants, posting flow.

## Out of scope (v1)

- Automatic posting (by design — manual only).
- Pause/resume, end dates, occurrence counts.
- Custom recurrence rules beyond the five presets.
- Projecting more than one upcoming occurrence per template.
- Fixed recipient-amount transfers (destination-currency obligations).
