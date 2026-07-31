# Money amounts as decimal strings on the wire

**Date:** 2026-07-08
**Status:** Approved
**Scope:** Money amounts only — account balance, transaction amount/amountRecipient,
budget limit/spent/balances. Exchange rates keep their existing string wire format
and join the frontend's decimal math internally, but are not part of the contract
change.

## Problem

During the React migration we found the frontend sends money amounts to the backend
inconsistently — sometimes as JSON numbers, sometimes as strings. The backend papers
over this with `vo.FlexString` (9 request fields across account, transaction, budget).
Worse, the frontend coerces every response amount to a JS `number`
(`Number(raw.balance)` in `web/src/api/account.ts`, `num`/`numOrNull` in
`web/src/api/budget.ts`) and does float arithmetic on it, so large amounts lose
precision the moment a response is parsed — `Number("12345678901234567.89")` is
already wrong before any math runs.

## Goal

Money amounts are **decimal strings in both directions**, and the frontend never
converts them through a float. The backend already responds with strings everywhere
(zero float fields in `internal/model`), so responses are untouched — the apiparity
goldens must stay byte-identical, which is a free regression check.

## Design

### 1. The contract

- Frontend → backend: the React app always posts amounts as normalized decimal
  strings.
- Backend → frontend: already all strings; now pinned as contract. No response
  change, no golden change.
- Backend keeps accepting JSON numbers on amount fields (`vo.FlexString` stays
  lenient — it is part of the frozen wire contract and third-party clients may rely
  on it) but logs a deprecation WARN when one arrives (section 3).
- OpenAPI annotations that describe amount request fields as numbers are corrected
  to `string` where wrong.

### 2. Frontend — strings end-to-end

**New `web/src/lib/decimal.ts`** — the only file that imports the new `big.js`
dependency (~3 KB, zero transitive deps; add to `web/package.json`). Exposes a small
string-in/string-out API (`add`, `sub`, `mul`, `div`, `cmp`, `isZero`, `isNegative`,
`abs`, `round`, `normalize`, …) configured to mirror the backend's `vo.Decimal`:
scale 8, half-up rounding, same normalization (no trailing zeros, `"0"` for
empty/invalid-per-contract cases). Big instances never escape this module.

**Kill the coercion layer.** `Number(raw.balance)` in `api/account.ts`,
`num`/`numOrNull` in `api/budget.ts`, `Number(r.rate)` in `api/currency.ts` are
removed. Amount fields in `api/dto/*.ts` and the domain types change `number` →
`string`. Amounts stay plain strings in the TanStack Query cache — JSON-serializable,
no Big instances stored.

**Math call sites** rewrite their arithmetic on `lib/decimal.ts`:
`web/src/lib/money.ts` (its float-based `normalizeNumber`/`formatNumber` become
string-based; formatting must not round-trip through `Number`),
`web/src/lib/exchange.ts`, `web/src/features/budgets/budgetMath.ts`,
`web/src/features/transactions/useTransactionForm.ts`,
`web/src/features/accounts/accountsTree.ts`, and any other site the `number` type
change flushes out via `tsc`.

**Forms** validate user input through the decimal helper — parse failure feeds the
existing per-field validation error path — and post the normalized string.

### 3. Backend — keep leniency, add a WARN

`vo.FlexString` becomes a small struct (`value string`, `fromNumber bool`) keeping
`String()`/`StrPtr()` and the exact decoding behavior (string, number, or null →
`""`); it gains `FromNumber() bool`. After decode, `endpoint.Handle` (and
`HandlePublic`) walks the request struct with a tiny reflection helper in
`internal/web/endpoint` and, if any FlexString came from a JSON number, emits one
WARN with a static message `"deprecated numeric amount"` and fields `route`,
`fields` (the JSON field names), plus the request-scoped attrs (`request_id`).
Never the amount value — logging rules forbid bodies/PII. No 400s, no contract
break; requests proceed normally.

### 4. Testing

- `lib/decimal.ts` unit tests, including large-value round-trips
  (`"12345678901234567.89"` survives untouched) and parity vectors borrowed from
  `internal/shared/vo`'s decimal test cases so JS and Go arithmetic agree
  digit-for-digit on the shared vectors.
- API client tests flip from asserting `Number` coercion to asserting pass-through
  strings on reads and string amounts in request bodies.
- Backend: FlexString `fromNumber` unit tests + one endpoint test asserting the WARN
  line fires for a numeric amount and stays silent for a string one.
- `make go-test` must pass with **zero apiparity golden diffs** (responses are
  contractually unchanged); `make web-test` and `make web-lint` green.

## Decisions taken (with alternatives considered)

- **Strings end-to-end** over boundary-only conversion (precision would still be
  lost at parse time) and over a strings-plus-lossy-display-math hybrid.
- **big.js** over decimal.js (more surface than needed) and over a hand-rolled
  BigInt scale-8 port of `vo.Decimal` (more code to write and test; big.js +
  shared parity vectors gets equivalent confidence cheaper).
- **Accept both + WARN** over keeping silent leniency (no visibility into whether
  numeric senders still exist) and over strict rejection (breaks the frozen wire
  contract).
