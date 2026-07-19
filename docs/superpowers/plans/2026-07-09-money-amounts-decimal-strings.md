# Money Amounts as Decimal Strings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Money amounts travel as decimal strings in both directions between the React SPA and the Go backend, with the frontend doing all amount arithmetic in exact decimal (big.js) instead of floats.

**Architecture:** Backend responses already carry amounts as strings — they must not change (zero apiparity golden diffs). The frontend drops its `Number()` coercion layer, stores amounts as strings in the TanStack Query cache, and routes all arithmetic through a new `web/src/lib/decimal.ts` (the only importer of big.js), which mirrors the backend's `vo.DecimalNumber` semantics: scale 8, Mul/Div truncate toward zero, Round half-away-from-zero. The backend keeps accepting JSON numbers via `vo.FlexString` (frozen leniency) but `FlexString` becomes a struct that remembers it decoded from a number, and the endpoint layer logs one WARN per such request.

**Tech Stack:** Go 1.25 (backend), React 19 + TypeScript + Vite + vitest (frontend, in `web/`), big.js (new dependency, `web/` only).

**Spec:** `docs/superpowers/specs/2026-07-08-money-amounts-decimal-strings-design.md`

## Global Constraints

- The wire contract is FROZEN: response bodies must not change. `make go-test` must pass with **zero apiparity golden diffs** — if a golden changes, the task is wrong.
- Backend must keep ACCEPTING JSON numbers on amount fields (no 400s for numeric amounts).
- Logs never carry amount values, emails, or bodies — field names / routes / ids only.
- big.js may be imported ONLY by `web/src/lib/decimal.ts`.
- Frontend amounts are plain `string` in DTOs, domain types, and the TanStack Query cache — never `Big` instances (cache stays JSON-serializable).
- Decimal semantics mirror `internal/shared/vo/decimal.go`: scale 8; Mul/Div TRUNCATE toward zero; Round is half-AWAY-from-zero; normalization trims trailing fraction zeros and leading integer zeros ("0.92000000" → "0.92").
- Verification commands: `make go-test` (backend), `cd web && pnpm test` (vitest), `cd web && pnpm lint` (oxlint), `cd web && pnpm build` (tsc typecheck + vite build).
- Comments: sparse, only non-obvious rationale (see CLAUDE.md). No references to the old Vue/PHP implementations.
- Commit after every task with the trailer lines used in this repo (Co-Authored-By: Claude Fable 5, Claude-Session — see repo git log for the format).

---

### Task 1: Frontend decimal core (`web/src/lib/decimal.ts`)

**Files:**
- Create: `web/src/lib/decimal.ts`
- Create: `web/src/lib/decimal.test.ts`
- Create: `web/src/lib/decimal_vectors.json` (copy of the backend's golden vectors)
- Modify: `web/package.json` (new dependency)

**Interfaces:**
- Consumes: nothing (leaf module).
- Produces (used by every later frontend task):
  - `normalize(v: string | number | null | undefined): string` — lenient; invalid/empty → `'0'`
  - `tryNormalize(raw: string): string | null` — strict; `null` on unparseable input
  - `add(a, b)`, `sub(a, b)`, `mul(a, b)`, `div(a, b)` → `string` (mul/div truncate at scale 8; div-by-zero returns `'0'`)
  - `cmp(a, b): -1 | 0 | 1`, `isZero(a): boolean`, `isNegative(a): boolean`, `abs(a): string`
  - `round(a, precision: number): string` (half-away-from-zero)
  - `toFixedString(a, digits: number): string` (half-away rounding, zero-padded — for formatting)
  - All value parameters accept `string | number | null | undefined` (the `Numeric` type) so display paths need no pre-casting.

- [ ] **Step 1: Add the big.js dependency**

```bash
cd web && pnpm add big.js && pnpm add -D @types/big.js
```

Check whether `web/node_modules/big.js` ships its own `*.d.ts`; if it does, `pnpm remove @types/big.js` (don't keep both).

- [ ] **Step 2: Copy the backend's golden decimal vectors**

```bash
cp internal/shared/vo/testdata/decimal_vectors.json web/src/lib/decimal_vectors.json
```

This is a frozen copy of frozen data (the file asserts the wire contract and does not drift). Shape: `{"arith": [{a, b, add, sub, mul, div}], "round": [{value, precision, result}]}` — 17 arith cases.

- [ ] **Step 3: Write the failing test**

Create `web/src/lib/decimal.test.ts`:

```ts
import { normalize, tryNormalize, add, sub, mul, div, cmp, isZero, isNegative, abs, round, toFixedString } from './decimal'
import vectors from './decimal_vectors.json'

describe('golden parity with vo.DecimalNumber', () => {
  it.each(vectors.arith)('arith $a ∘ $b', (c) => {
    expect(add(c.a, c.b)).toBe(c.add)
    expect(sub(c.a, c.b)).toBe(c.sub)
    expect(mul(c.a, c.b)).toBe(c.mul)
    expect(div(c.a, c.b)).toBe(c.div)
  })

  it.each(vectors.round)('round($value, $precision)', (c) => {
    expect(round(c.value, c.precision)).toBe(c.result)
  })
})

describe('normalize', () => {
  it('trims zeros like the backend', () => {
    expect(normalize('0.92000000')).toBe('0.92')
    expect(normalize('100.50')).toBe('100.5')
    expect(normalize('007')).toBe('7')
    expect(normalize('')).toBe('0')
    expect(normalize(null)).toBe('0')
    expect(normalize(undefined)).toBe('0')
    expect(normalize('not-a-number')).toBe('0')
  })

  it('truncates to scale 8 and expands scientific notation', () => {
    expect(normalize('1.123456789')).toBe('1.12345678')
    expect(normalize('1e-7')).toBe('0.0000001')
    expect(normalize(1e-7)).toBe('0.0000001')
  })

  it('preserves large amounts exactly (the reason this module exists)', () => {
    expect(normalize('12345678901234567.89')).toBe('12345678901234567.89')
    expect(add('12345678901234567.89', '0.01')).toBe('12345678901234567.9')
    expect(sub('12345678901234567.90', '0.01')).toBe('12345678901234567.89')
  })
})

describe('tryNormalize', () => {
  it('is strict: null for garbage, normalized string otherwise', () => {
    expect(tryNormalize('100.50')).toBe('100.5')
    expect(tryNormalize('  42 ')).toBe('42')
    expect(tryNormalize('')).toBeNull()
    expect(tryNormalize('5+5')).toBeNull()
    expect(tryNormalize('abc')).toBeNull()
  })
})

describe('comparisons and helpers', () => {
  it('cmp / isZero / isNegative / abs', () => {
    expect(cmp('1.5', '1.50')).toBe(0)
    expect(cmp('2', '10')).toBe(-1)
    expect(cmp('-1', '-2')).toBe(1)
    expect(isZero('0.00000000')).toBe(true)
    expect(isZero('0.00000001')).toBe(false)
    expect(isNegative('-3.5')).toBe(true)
    expect(isNegative('3.5')).toBe(false)
    expect(abs('-3.5')).toBe('3.5')
  })

  it('div by zero yields 0 (display-safe, unlike the backend panic)', () => {
    expect(div('5', '0')).toBe('0')
  })

  it('toFixedString pads and rounds half-away', () => {
    expect(toFixedString('10.5', 2)).toBe('10.50')
    expect(toFixedString('10.567', 2)).toBe('10.57')
    expect(toFixedString('10.565', 2)).toBe('10.57')
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd web && pnpm vitest run src/lib/decimal.test.ts`
Expected: FAIL — cannot resolve `./decimal`.

- [ ] **Step 5: Implement `web/src/lib/decimal.ts`**

```ts
import Big from 'big.js'

// Mirrors the backend's vo.DecimalNumber (internal/shared/vo/decimal.go):
// scale 8; Mul/Div TRUNCATE toward zero; Round is half-away-from-zero.
// All exports take and return decimal strings — Big instances never escape,
// and no other module may import big.js.
const SCALE = 8

// Scoped constructor so div() truncates toward zero at scale 8 without
// mutating the library-global Big config.
const BigD = Big()
BigD.DP = SCALE
BigD.RM = BigD.roundDown

export type Numeric = string | number | null | undefined

const big = (v: Numeric): Big => {
  if (v === null || v === undefined) return new BigD(0)
  const s = String(v).trim()
  if (s === '') return new BigD(0)
  try {
    return new BigD(s)
  } catch {
    return new BigD(0)
  }
}

// toFixed() with no argument always renders normal notation (never exponential).
const toPlain = (b: Big): string => {
  const s = b.toFixed()
  if (!s.includes('.')) return s
  return s.replace(/0+$/, '').replace(/\.$/, '')
}

const truncate8 = (b: Big): Big => b.round(SCALE, BigD.roundDown)

export function tryNormalize(raw: string): string | null {
  const s = raw.trim()
  if (s === '') return null
  try {
    return toPlain(truncate8(new BigD(s)))
  } catch {
    return null
  }
}

export function normalize(v: Numeric): string {
  return toPlain(truncate8(big(v)))
}

export const add = (a: Numeric, b: Numeric): string => toPlain(big(a).plus(big(b)))
export const sub = (a: Numeric, b: Numeric): string => toPlain(big(a).minus(big(b)))
export const mul = (a: Numeric, b: Numeric): string => toPlain(truncate8(big(a).times(big(b))))

// Division by zero yields '0' (display-safe) where the backend panics.
export const div = (a: Numeric, b: Numeric): string => {
  const den = big(b)
  if (den.eq(0)) return '0'
  return toPlain(big(a).div(den))
}

export const cmp = (a: Numeric, b: Numeric): -1 | 0 | 1 => big(a).cmp(big(b)) as -1 | 0 | 1
export const isZero = (a: Numeric): boolean => big(a).eq(0)
export const isNegative = (a: Numeric): boolean => big(a).lt(0)
export const abs = (a: Numeric): string => toPlain(big(a).abs())

export const round = (a: Numeric, precision: number): string =>
  toPlain(big(a).round(Math.max(0, precision), BigD.roundHalfUp))

export const toFixedString = (a: Numeric, digits: number): string =>
  big(a).round(digits, BigD.roundHalfUp).toFixed(digits)
```

If `tsconfig` complains about the JSON import, confirm `resolveJsonModule` is enabled in `web/tsconfig.app.json` (add it if missing).

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd web && pnpm vitest run src/lib/decimal.test.ts`
Expected: PASS (all golden vectors green).

- [ ] **Step 7: Commit**

```bash
git add web/package.json web/pnpm-lock.yaml web/src/lib/decimal.ts web/src/lib/decimal.test.ts web/src/lib/decimal_vectors.json
git commit -m "feat(react/money): decimal-string arithmetic core mirroring vo.DecimalNumber"
```

---

### Task 2: Rewrite `money.ts` on the decimal core

**Files:**
- Modify: `web/src/lib/money.ts`
- Modify: `web/src/lib/money.test.ts`

**Interfaces:**
- Consumes: `normalize`, `round`, `toFixedString`, `Numeric` from `@/lib/decimal` (Task 1).
- Produces: SAME exported names/signatures as today, so callers don't change:
  - `normalizeNumber(amount: number | string | null | undefined): string`
  - `formatNumber(amount: number | string | null | undefined, digits: number, useFixedPrecision: boolean): string`
  - `addThousandSeparators(formatted: string): string[]` (unchanged)
  - `moneyFormat(amount: number | string, currency?: CurrencyLike | null, opts?: MoneyFormatOptions): string`

- [ ] **Step 1: Add failing large-value tests**

Append to `web/src/lib/money.test.ts`:

```ts
describe('large amounts survive without float mangling', () => {
  it('normalizeNumber keeps every digit', () => {
    expect(normalizeNumber('12345678901234567.89')).toBe('12345678901234567.89')
  })

  it('moneyFormat formats large strings exactly', () => {
    expect(moneyFormat('12345678901234567.89', usd, { showCurrency: false })).toBe('12,345,678,901,234,567.89')
  })
})
```

- [ ] **Step 2: Run to verify the new tests fail**

Run: `cd web && pnpm vitest run src/lib/money.test.ts`
Expected: the two new tests FAIL (float parsing mangles the digits); existing tests PASS.

- [ ] **Step 3: Rewrite `web/src/lib/money.ts`**

Replace the whole file body (keep `CurrencyLike` and `MoneyFormatOptions` as-is):

```ts
import { normalize, round, toFixedString } from '@/lib/decimal'

export interface CurrencyLike {
  symbol: string
  fractionDigits: number
}

export function normalizeNumber(amount: number | string | null | undefined): string {
  return normalize(amount)
}

export function formatNumber(amount: number | string | null | undefined, digits: number, useFixedPrecision: boolean): string {
  const normalized = normalize(amount)
  if (digits === 0) {
    return round(normalized, 0)
  }
  if (useFixedPrecision) {
    return toFixedString(normalized, digits)
  }
  const actualDecimals = normalized.split('.')[1]?.length ?? 0
  const useDigits = Math.max(digits, Math.min(actualDecimals, 8))
  return toFixedString(normalized, useDigits)
}

export function addThousandSeparators(formatted: string): string[] {
  const parts = formatted.split('.')
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return parts
}

export interface MoneyFormatOptions {
  showCurrency?: boolean
  useNativePrecision?: boolean
  useThousandSeparator?: boolean
  /** round to at most this many decimals */
  maxPrecision?: number
}

export function moneyFormat(
  amount: number | string,
  currency?: CurrencyLike | null,
  opts: MoneyFormatOptions = {},
): string {
  const { showCurrency = true, useNativePrecision = true, useThousandSeparator = true, maxPrecision } = opts
  let normalizedAmount = normalize(amount)
  if (maxPrecision !== undefined) {
    normalizedAmount = normalize(round(normalizedAmount, maxPrecision))
  }
  const digits = useNativePrecision
    ? (currency?.fractionDigits ?? 8)
    : !normalizedAmount.includes('.')
      ? (currency?.fractionDigits ?? 0)
      : Math.max(currency?.fractionDigits ?? 0, Math.min((normalizedAmount.split('.')[1] || '').length, 8))
  const formattedNumber = formatNumber(normalizedAmount, digits, useNativePrecision)
  const parts = useThousandSeparator ? addThousandSeparators(formattedNumber) : formattedNumber.split('.')

  let result = parts[0]
  if (parts.length > 1) {
    result += '.' + parts[1]
  }
  if (showCurrency && currency) {
    result += ' ' + currency.symbol
  }
  return result
}
```

The regexp `\B(?=(\d{3})+(?!\d))` doesn't handle a leading minus specially and never did — behavior is unchanged.

- [ ] **Step 4: Run the whole money suite**

Run: `cd web && pnpm vitest run src/lib/money.test.ts`
Expected: ALL PASS. If any pre-existing assertion fails, the difference will be a rounding edge (old code used float `Math.round`/`toFixed`, which rounds negative halves toward +∞; the decimal core rounds half-away-from-zero). Update such an assertion to the decimal result ONLY if the case is a `x.5` negative half; anything else means the implementation is wrong — fix the implementation, not the test.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/money.ts web/src/lib/money.test.ts
git commit -m "refactor(react/money): money formatting on exact decimal strings"
```

---

### Task 3: Flip DTO amount types to string and delete the coercion layer

**Files:**
- Modify: `web/src/api/dto/account.ts`, `web/src/api/dto/transaction.ts`, `web/src/api/dto/budget.ts`, `web/src/api/dto/currency.ts`
- Modify: `web/src/api/account.ts`, `web/src/api/transaction.ts`, `web/src/api/budget.ts`, `web/src/api/currency.ts`
- Modify: `web/src/api/account.test.ts`, `web/src/api/transaction.test.ts`, `web/src/api/budget.test.ts`

**Interfaces:**
- Consumes: nothing new.
- Produces (every later task relies on these types):
  - `AccountDto.balance: string`
  - `CreateTransactionDto.amount: string`, `CreateTransactionDto.amountRecipient: string | null` (and thus `TransactionDto`)
  - `BudgetChildElementDto.spent/budgetSpent: string`; `BudgetElementDto.budgeted/available: string` (inherits spent/budgetSpent)
  - `BudgetBalanceDto.{startBalance,endBalance,income,expenses,exchanges,holdings}: string | null`
  - `BudgetRateDto.rate: string`; `BudgetTransactionDto.amount: string`
  - `CurrencyRateDto.rate: string`
  - `CreateAccountForm.balance: string`, `UpdateAccountForm.balance: string`
  - API functions now return wire strings verbatim (no coercion).

Note: after this task the web workspace does NOT fully typecheck until Tasks 4–8 land — that is expected. vitest (esbuild) still runs per-file, so each task remains testable. `pnpm build` gates Task 8.

- [ ] **Step 1: Update the API client tests to expect pass-through strings**

In `web/src/api/account.test.ts`: the fixtures already use wire strings (`balance: '100.5'`, `amount: '100.5'`). Change the assertions and form values:
- `expect(items[0].balance).toBe(100.5)` → `expect(items[0].balance).toBe('100.5')`
- `createAccount({ ..., balance: 100.5, ... })` → `balance: '100.5'`, and the body assertion `expect(body).toEqual({ ..., balance: '100.5', ... })`
- `expect(result.item.balance).toBe(100.5)` → `toBe('100.5')`; `expect(result.transaction?.amount).toBe(100.5)` → `toBe('100.5')`
- `createAccount({ ..., balance: 0, ... })` → `balance: '0'`
- Rename test descriptions that say "coerces … to numbers" to say "passes decimal strings through".
- Add one large-value case:

```ts
it('passes large balances through without precision loss', async () => {
  server.use(/* same handler pattern as the list test, with balance: '12345678901234567.89' */)
  const items = await accountApi.getAccountList()
  expect(items[0].balance).toBe('12345678901234567.89')
})
```

(Write the handler exactly like the existing `getAccountList` test in that file, substituting the balance value.)

In `web/src/api/transaction.test.ts` and `web/src/api/budget.test.ts`: same mechanical transformation — every amount/balance/rate assertion flips from number literal to the canonical string (`9.99` → `'9.99'`, `100.5` → `'100.5'`, `10` → `'10'`, `15000` → `'15000'`). Null-preservation assertions (`toBeNull()`) stay.

- [ ] **Step 2: Run the api tests to verify they fail**

Run: `cd web && pnpm vitest run src/api`
Expected: FAIL — the clients still coerce to numbers.

- [ ] **Step 3: Flip the DTO types**

- `web/src/api/dto/account.ts`: `balance: number` → `balance: string`, and update its comment to `/** decimal string (wire format, kept verbatim) */`.
- `web/src/api/dto/transaction.ts`: `amount: number` → `amount: string`; `amountRecipient: number | null` → `amountRecipient: string | null`.
- `web/src/api/dto/budget.ts`: `spent`, `budgetSpent`, `budgeted`, `available` → `string`; every `BudgetBalanceDto` field `number | null` → `string | null`; `BudgetRateDto.rate` → `string`; `BudgetTransactionDto.amount` → `string`. Update the `/** wire: decimal string, coerced */` comments to `/** decimal string (wire format, kept verbatim) */`.
- `web/src/api/dto/currency.ts`: `rate: number` → `rate: string`, same comment update.

- [ ] **Step 4: Delete the coercion layer in the clients**

- `web/src/api/account.ts`: delete `coerceAccount`, `coerceTransaction`, `coerceAccountItem` and their uses — return `response.data.data.items` / `response.data.data` directly. Change `CreateAccountForm.balance` and `UpdateAccountForm.balance` to `string`. Delete the "coerce to numbers" comment.
- `web/src/api/transaction.ts`: delete `coerceItem` and the `coerceAccount, coerceTransaction` import; return envelope data directly.
- `web/src/api/budget.ts`: delete `num`, `numOrNull`, `coerceBudget` and the Vue-coercion comment; `getBudget` returns `response.data.data.item` directly; `getBudgetTransactions` returns `response.data.data.items` directly.
- `web/src/api/currency.ts`: `getCurrencyRateList` returns `response.data.data.items` directly.

- [ ] **Step 5: Run the api tests to verify they pass**

Run: `cd web && pnpm vitest run src/api`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/api
git commit -m "feat(react/api)!: amounts and rates stay decimal strings end-to-end, no float coercion"
```

---

### Task 4: `exchange.ts` and `useExchange` on decimal strings

**Files:**
- Modify: `web/src/lib/exchange.ts`
- Modify: `web/src/lib/exchange.test.ts`
- Modify: `web/src/features/currencies/useExchange.ts`

**Interfaces:**
- Consumes: `normalize`, `mul`, `div`, `round`, `isZero` from `@/lib/decimal`; `CurrencyRateDto.rate: string` (Task 3).
- Produces: `exchange(fromCurrencyId: string, toCurrencyId: string, amount: string, rates: CurrencyRateDto[], currencies: CurrencyDto[]): string`; `useExchange()` returns `(fromCurrencyId: string, toCurrencyId: string, amount: string) => string`. **The exchange-callback shape `(from: string, to: string, amount: string) => string` is the `ExchangeFn` every later task uses.**

- [ ] **Step 1: Update the tests to string in/out**

Rewrite the assertions in `web/src/lib/exchange.test.ts` (fixtures: `rate: 1` → `rate: '1'`, `rate: 0.9` → `'0.9'`, `rate: 150` → `'150'`):

```ts
it('returns the amount unchanged for same currency', () => {
  expect(exchange('usd', 'usd', '42.5', rates, currencies)).toBe('42.5')
})

it('converts through the base currency both directions', () => {
  expect(exchange('eur', 'usd', '90', rates, currencies)).toBe('100')
  expect(exchange('usd', 'eur', '100', rates, currencies)).toBe('90')
  expect(exchange('eur', 'jpy', '90', rates, currencies)).toBe('15000')
})

it('rounds to the target currency fraction digits', () => {
  expect(exchange('usd', 'eur', '1', rates, currencies)).toBe('0.9')
  expect(exchange('eur', 'jpy', '1', rates, currencies)).toBe('167')
})

it('falls back to the raw amount when rates or currencies are missing', () => {
  expect(exchange('usd', 'eur', '10', [], currencies)).toBe('10')
  expect(exchange('usd', 'xxx', '10', rates, currencies)).toBe('10')
  expect(exchange('xxx', 'eur', '10', rates, currencies)).toBe('10')
})

it('keeps large amounts exact through conversion', () => {
  // 12345678901234567.89 USD -> EUR at 0.9
  expect(exchange('usd', 'eur', '12345678901234567.89', rates, currencies)).toBe('11111111011111111.1')
})
```

(`eur → jpy` at fractionDigits 0: `1 / 0.9 * 150 = 166.66…` rounds half-away to `167` — same result as before.)

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm vitest run src/lib/exchange.test.ts`
Expected: FAIL (current implementation returns numbers).

- [ ] **Step 3: Rewrite `web/src/lib/exchange.ts`**

```ts
import type { CurrencyDto, CurrencyRateDto } from '@/api/dto/currency'
import { div, isZero, mul, normalize, round } from '@/lib/decimal'

// Convert through the base currency, falling back to the unconverted amount
// whenever a rate or currency is missing (or a rate is zero).
export function exchange(
  fromCurrencyId: string,
  toCurrencyId: string,
  amount: string,
  rates: CurrencyRateDto[],
  currencies: CurrencyDto[],
): string {
  const parsedAmount = normalize(amount)
  if (fromCurrencyId === toCurrencyId) {
    return parsedAmount
  }
  if (!Array.isArray(rates) || rates.length === 0) {
    return parsedAmount
  }
  const toCurrency = currencies.find((c) => c.id === toCurrencyId)
  if (!toCurrency) {
    return parsedAmount
  }
  let result = parsedAmount
  const fromRate = rates.find((r) => r.currencyId === fromCurrencyId)
  if (fromRate === undefined || isZero(fromRate.rate)) {
    return parsedAmount
  }
  if (fromCurrencyId !== fromRate.baseCurrencyId) {
    result = div(result, fromRate.rate)
  }
  const toRate = rates.find((r) => r.currencyId === toCurrencyId)
  if (toRate === undefined) {
    return parsedAmount
  }
  if (toCurrencyId !== toRate.baseCurrencyId) {
    result = mul(result, toRate.rate)
  }
  return round(result, toCurrency.fractionDigits)
}
```

- [ ] **Step 4: Update `web/src/features/currencies/useExchange.ts`**

```ts
import { useCallback } from 'react'
import { exchange } from '@/lib/exchange'
import { useCurrencies, useCurrencyRates } from './queries'

export function useExchange() {
  const { data: rates } = useCurrencyRates()
  const { data: currencies } = useCurrencies()
  return useCallback(
    (fromCurrencyId: string, toCurrencyId: string, amount: string): string =>
      exchange(fromCurrencyId, toCurrencyId, amount, rates ?? [], currencies ?? []),
    [rates, currencies],
  )
}
```

- [ ] **Step 5: Run to verify pass**

Run: `cd web && pnpm vitest run src/lib/exchange.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/exchange.ts web/src/lib/exchange.test.ts web/src/features/currencies/useExchange.ts
git commit -m "refactor(react/currency): exact-decimal currency conversion, string in string out"
```

---

### Task 5: `budgetMath.ts` on decimal strings

**Files:**
- Modify: `web/src/features/budgets/budgetMath.ts`
- Modify: `web/src/features/budgets/budgetMath.test.ts`

**Interfaces:**
- Consumes: `add`, `sub`, `abs`, `cmp`, `div` from `@/lib/decimal`; string DTO fields (Task 3); string `exchange` (Task 4).
- Produces:
  - `BucketStats { budgeted: string; spent: string; available: string }`
  - `ExchangeFn = (fromCurrencyId: string, toCurrencyId: string, amount: string) => string`
  - `displaySpent(spent: string): string`; `displayAvailable(el: { available: string; budgeted: string }): string`
  - `WidgetMath { spent: string; total: string; progress: number; overspent: boolean }` — `progress` stays a plain number ratio in [0,1] (drives a CSS width; float precision is fine there).
  - `bucketStats`, `bucketElements`, `budgetTotals`, `periodRange`, `widgetMath` keep their names and arities.

- [ ] **Step 1: Update the tests**

In `web/src/features/budgets/budgetMath.test.ts`, flip every amount fixture/assertion from number to canonical string (`spent: 10` → `spent: '10'`, `expect(stats.budgeted).toBe(100)` → `toBe('100')`, etc. — mechanical, null stays null). `position`, `isArchived`, `fractionDigits`, and `progress` assertions stay numeric. Add:

```ts
it('totals large amounts exactly', () => {
  // two elements of 9007199254740993 each (beyond Number.MAX_SAFE_INTEGER)
  // must sum to 18014398509481986, which float math cannot represent
  /* build two elements with budgeted: '9007199254740993', same currency as the
     budget, exchangeFn = (_f, _t, a) => a, and assert
     stats.budgeted === '18014398509481986' */
})
```

(Fill the fixture using the same element-builder shape the existing tests in that file use.)

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm vitest run src/features/budgets/budgetMath.test.ts`
Expected: FAIL.

- [ ] **Step 3: Rewrite the math in `budgetMath.ts`**

Only the arithmetic changes; bucketing/sorting logic stays identical. The changed parts:

```ts
import { abs, add, cmp, div, sub } from '@/lib/decimal'

export interface BucketStats {
  budgeted: string
  spent: string
  available: string
}

type ExchangeFn = (fromCurrencyId: string, toCurrencyId: string, amount: string) => string

export function bucketStats(elements: BudgetElementDto[], budget: BudgetDto, exchangeFn: ExchangeFn): BucketStats {
  const base = budget.meta.currencyId
  let budgeted = '0'
  let spent = '0'
  let available = '0'
  for (const el of elements) {
    const from = el.currencyId ?? base
    budgeted = add(budgeted, exchangeFn(from, base, el.budgeted))
    spent = add(spent, el.budgetSpent)
    available = add(available, exchangeFn(from, base, add(el.available, el.budgeted)))
  }
  return { budgeted, spent, available }
}
```

`budgetTotals` reduce becomes:

```ts
    (acc, s) => ({ budgeted: add(acc.budgeted, s.budgeted), spent: add(acc.spent, s.spent), available: add(acc.available, s.available) }),
    { budgeted: '0', spent: '0', available: '0' },
```

Display helpers:

```ts
export const displaySpent = (spent: string): string => sub('0', spent)
export const displayAvailable = (el: { available: string; budgeted: string }): string => add(el.available, el.budgeted)
```

`widgetMath`:

```ts
export interface WidgetMath {
  spent: string
  total: string
  /** ratio for the progress bar; float precision is fine for a CSS width */
  progress: number
  overspent: boolean
}

export function widgetMath(balance: BudgetBalanceDto | undefined): WidgetMath {
  const n = (v: string | null | undefined) => v ?? '0'
  const expenses = n(balance?.expenses)
  const exchanges = n(balance?.exchanges)
  const holdings = n(balance?.holdings)
  const startBalance = n(balance?.startBalance)
  const income = n(balance?.income)

  let spent = abs(expenses)
  if (cmp(exchanges, '0') < 0) spent = add(spent, abs(exchanges))
  if (cmp(holdings, '0') < 0) spent = add(spent, abs(holdings))

  let total = abs(add(startBalance, income))
  if (cmp(exchanges, '0') > 0) total = add(total, exchanges)
  if (cmp(holdings, '0') > 0) total = add(total, holdings)

  const progress = cmp(total, '0') <= 0 ? 0 : Math.max(0, Math.min(Number(div(spent, total)), 1))
  return { spent, total, progress, overspent: cmp(spent, total) > 0 }
}
```

`makeBudgetExchange`, `bucketElements`, `periodRange` keep their bodies (only the `ExchangeFn` type changed).

- [ ] **Step 4: Run to verify pass**

Run: `cd web && pnpm vitest run src/features/budgets/budgetMath.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets/budgetMath.ts web/src/features/budgets/budgetMath.test.ts
git commit -m "refactor(react/budget): budget aggregates in exact decimal strings"
```

---

### Task 6: `accountsTree.ts` totals on decimal strings

**Files:**
- Modify: `web/src/features/accounts/accountsTree.ts`
- Modify: `web/src/features/accounts/accountsTree.test.ts`

**Interfaces:**
- Consumes: `add` from `@/lib/decimal`; `AccountDto.balance: string` (Task 3); the `ExchangeFn` shape from Task 4.
- Produces: `AccountsTreeItem.total: string`; `buildAccountsTree(accounts, folders, userCurrency, exchangeFn: (fromCurrencyId: string, toCurrencyId: string, amount: string) => string, defaultFolderName)` — same name/arity.

- [ ] **Step 1: Update the tests** — flip balance fixtures and `total` assertions to strings (`balance: 100` → `balance: '100'`, `expect(item.total).toBe(150)` → `toBe('150')`), and pass a string-typed exchange stub (`(_f, _t, a) => a` already fits once types flip).

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm vitest run src/features/accounts/accountsTree.test.ts`
Expected: FAIL.

- [ ] **Step 3: Update the implementation** — in `buildAccountsTree`:
- `total: number` → `total: string` in `AccountsTreeItem`
- `exchangeFn` param type → `(fromCurrencyId: string, toCurrencyId: string, amount: string) => string`
- inside `buildItem`:

```ts
    let nativeTotal = '0'
    let convertedTotal = '0'
    for (const account of folderAccounts) {
      if (sharedCurrency === null) {
        sharedCurrency = account.currency
      } else if (sharedCurrency.id !== account.currency.id) {
        mixed = true
      }
      nativeTotal = add(nativeTotal, account.balance)
      convertedTotal = add(convertedTotal, userCurrency ? exchangeFn(account.currency.id, userCurrency.id, account.balance) : account.balance)
    }
```

- [ ] **Step 4: Run to verify pass**

Run: `cd web && pnpm vitest run src/features/accounts/accountsTree.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/accounts/accountsTree.ts web/src/features/accounts/accountsTree.test.ts
git commit -m "refactor(react/accounts): folder totals in exact decimal strings"
```

---

### Task 7: Forms post normalized decimal strings

**Files:**
- Modify: `web/src/features/transactions/useTransactionForm.ts`
- Modify: `web/src/features/transactions/TransactionDialog.tsx`
- Modify: `web/src/features/accounts/AccountDialog.tsx`
- Modify: `web/src/features/budgets/limitAmount.ts`
- Modify: `web/src/features/budgets/limitAmount.test.ts`
- Modify: `web/src/features/budgets/LimitEditor.tsx`, `web/src/features/budgets/SetLimitDialog.tsx`
- Modify: `web/src/features/budgets/queries.ts`
- Modify: `web/src/features/transactions/useTransactionForm.test.ts` (if present; check with `ls web/src/features/transactions/*.test.*`)

**Interfaces:**
- Consumes: `normalize`, `tryNormalize`, `isZero` from `@/lib/decimal`; `evaluateFormula`, `sanitizeInput` from `@/lib/calculator` (unchanged); string DTOs (Task 3); string `ExchangeFn` (Task 4).
- Produces:
  - `evaluatedAmount(raw: string): string` in `useTransactionForm.ts` — replaces the exported `evaluatedNumber`. **Key rule: plain decimal input (`/^\d+(\.\d+)?$/` after `sanitizeInput`) bypasses the float calculator entirely** — that is what preserves large typed amounts. Formula input (`5+5`) still evaluates through the calculator (float-backed; formulas accept float limits).
  - `buildPayload(form: TransactionFormState): CreateTransactionDto` — amounts now strings.
  - `limitAmountFromInput(raw: string)` — same contract, decimal-exact.

- [ ] **Step 1: Write failing tests for the amount rules**

In `web/src/features/budgets/limitAmount.test.ts`, add:

```ts
it('preserves large plain amounts without float mangling', () => {
  expect(limitAmountFromInput('12345678901234567.89')).toEqual({ ok: true, amount: '12345678901234567.89' })
})

it('still clears on zero and rejects garbage', () => {
  expect(limitAmountFromInput('0.00')).toEqual({ ok: true, amount: null })
  expect(limitAmountFromInput('abc')).toEqual({ ok: false })
})
```

If `useTransactionForm` has a test file, add the mirror case for `buildPayload`:

```ts
it('posts large plain amounts verbatim', () => {
  const form = { ...baseForm, amount: '12345678901234567.89', type: 'expense' as const }
  expect(buildPayload(form).amount).toBe('12345678901234567.89')
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm vitest run src/features/budgets/limitAmount.test.ts`
Expected: the new large-value test FAILS (current code round-trips through `Number`).

- [ ] **Step 3: `useTransactionForm.ts`** — replace `evaluatedNumber` and `buildPayload`, update `seedAmount` and `recomputeRecipientAmount`:

```ts
import { normalize, tryNormalize } from '@/lib/decimal'

const seedAmount = (value: string | null | undefined, account: AccountDto | undefined): string => {
  if (value === null || value === undefined) {
    return ''
  }
  return moneyFormat(value, account?.currency, { showCurrency: false, useNativePrecision: false, useThousandSeparator: false })
}

// Plain decimal input skips the float-backed calculator so large amounts keep
// every digit; only actual formulas ("5+5") go through evaluation.
export const evaluatedAmount = (raw: string): string => {
  const sanitized = sanitizeInput(raw)
  if (/^\d+(\.\d+)?$/.test(sanitized)) {
    return normalize(sanitized)
  }
  return normalize(evaluateFormula(sanitized + '='))
}

export function buildPayload(form: TransactionFormState): CreateTransactionDto {
  const isTransfer = form.type === 'transfer'
  const amount = evaluatedAmount(form.amount)
  return {
    id: form.id,
    type: form.type,
    accountId: form.accountId as Id,
    accountRecipientId: isTransfer ? form.accountRecipientId : null,
    amount,
    amountRecipient: isTransfer ? (form.amountRecipient === '' ? amount : normalize(form.amountRecipient)) : null,
    categoryId: isTransfer ? null : form.categoryId,
    description: form.description || '',
    payeeId: isTransfer ? null : form.payeeId,
    tagId: isTransfer ? null : form.tagId,
    date: form.date,
  }
}
```

In `recomputeRecipientAmount` (and `swapAccounts`' use of it), the exchange callback type becomes `(fromId: string, toId: string, amount: string) => string` and the NaN guard becomes a strict-parse guard:

```ts
  const recomputeRecipientAmount = (
    amount: string,
    from: AccountDto | undefined,
    to: AccountDto | undefined,
    exchangeFn: (fromId: string, toId: string, amount: string) => string,
  ): string => {
    if (!to || !from || amount === '' || tryNormalize(amount) === null) {
      return amount
    }
    if (from.currency.id === to.currency.id) {
      return amount
    }
    return exchangeFn(from.currency.id, to.currency.id, normalize(amount))
  }
```

- [ ] **Step 4: `TransactionDialog.tsx`** — the `amountErrors` NaN check flips to the decimal strict parse (imports: `evaluatedAmount` instead of `evaluatedNumber`, plus `tryNormalize` from `@/lib/decimal`):

```ts
      const evaluated = evaluatedAmount(raw)
      if (tryNormalize(evaluated) === null) {
        return t('elements.validation.invalid_number')
      }
```

(The two `recomputeRecipientAmount`/`exchangeFn` call sites in this file need no code change — the types now line up via Task 4's `useExchange`.)

- [ ] **Step 5: `AccountDialog.tsx`** — the submit path stops converting to float:

```ts
    const balanceAmount = evaluatedAmount(balance)
```

and both `mutateAsync` calls pass `balance: balanceAmount` (import `evaluatedAmount` from `../transactions/useTransactionForm`). The validation block is unchanged (`evaluateFormula` string checks still work). Delete the now-unused `const numericBalance` line.

- [ ] **Step 6: `limitAmount.ts`** — decimal-exact:

```ts
import { evaluateFormula, sanitizeInput, validateFormula } from '@/lib/calculator'
import { tryNormalize } from '@/lib/decimal'

// Unified set-limit amount rule: empty / 0 / unparseable clears or rejects;
// otherwise the normalized decimal string. Plain decimals skip the float
// calculator so large limits keep every digit.
export function limitAmountFromInput(raw: string): { ok: true; amount: string | null } | { ok: false } {
  const trimmed = raw.trim()
  if (trimmed === '') {
    return { ok: true, amount: null }
  }
  const sanitized = sanitizeInput(trimmed)
  if (!validateFormula(sanitized)) {
    return { ok: false }
  }
  const evaluated = /^\d+(\.\d+)?$/.test(sanitized) ? tryNormalize(sanitized) : tryNormalize(evaluateFormula(sanitized + '='))
  if (evaluated === null) {
    return { ok: false }
  }
  if (evaluated === '0') {
    return { ok: true, amount: null }
  }
  return { ok: true, amount: evaluated }
}
```

- [ ] **Step 7: Seed/optimistic-update sites**
- `LimitEditor.tsx:41` and `SetLimitDialog.tsx:25`: `element.budgeted === 0` → `isZero(element.budgeted)` (import from `@/lib/decimal`); the `normalizeNumber(element.budgeted)` calls keep working (string input).
- `web/src/features/budgets/queries.ts:118` (setLimit optimistic update): `budgeted: form.amount === null ? 0 : Number(form.amount)` → `budgeted: form.amount === null ? '0' : form.amount`.

- [ ] **Step 8: Run the affected suites**

Run: `cd web && pnpm vitest run src/features/budgets src/features/transactions`
Expected: PASS (update any remaining number-literal assertions to strings — same mechanical rule as Tasks 5–6).

- [ ] **Step 9: Commit**

```bash
git add web/src/features
git commit -m "feat(react/forms): amount inputs post normalized decimal strings, exact for plain input"
```

---

### Task 8: Frontend flush — typecheck clean, whole suite green

**Files:**
- Modify: whatever `tsc` flags — known remainders: `web/src/features/transactions/ViewTransactionDialog.tsx` (`accountRow(account, amount: number | null)` → `string | null`), `web/src/features/transactions/useAccountTransactions.ts` (the `ViewTransaction` amount types), `web/src/features/transactions/queries.test.tsx`, `web/src/api/dto/*` stragglers, any component comparing amounts numerically.

**Interfaces:**
- Consumes: everything above. Produces: a compiling, fully green web workspace.

- [ ] **Step 1: Typecheck and enumerate the stragglers**

Run: `cd web && pnpm build`
Expected: a finite list of type errors — every one is an amount that is now a string.

- [ ] **Step 2: Fix each straggler with these rules (no exceptions):**
- **Display** (`moneyFormat(...)`, rendering): just widen the local type to `string` — `moneyFormat` accepts strings.
- **Comparison** (`x < 0`, `x === 0`, `a > b`): use `isNegative(x)`, `isZero(x)`, `cmp(a, b) > 0` from `@/lib/decimal`.
- **Arithmetic** (`a + b`, `-x`, `Math.abs(x)`): use `add`, `sub('0', x)`, `abs` from `@/lib/decimal`.
- **Sorting by amount**: `.sort((a, b) => cmp(a.amount, b.amount))`.
- **Test fixtures/assertions**: number literal → canonical decimal string.
- NEVER fix a type error by `Number(...)` — that reintroduces the bug this project removes.

- [ ] **Step 3: Full frontend verification**

Run: `cd web && pnpm build && pnpm test && pnpm lint`
Expected: build succeeds, all vitest suites pass, oxlint clean. Also grep to prove the coercion layer is gone and big.js stayed contained:

```bash
grep -rn "Number(" web/src/api && echo "FAIL: coercion left in api layer" || echo OK
grep -rln "big.js" web/src | grep -v "lib/decimal" && echo "FAIL: big.js leaked" || echo OK
```

- [ ] **Step 4: Commit**

```bash
git add web/src
git commit -m "refactor(react): amounts are decimal strings everywhere; typecheck-driven flush"
```

---

### Task 9: Backend — `vo.FlexString` remembers it decoded from a number

**Files:**
- Modify: `internal/shared/vo/flexstring.go`
- Modify: `internal/shared/vo/flexstring_test.go`
- Modify: `internal/model/account_dto.go` (lines ~50, ~89), `internal/model/transaction_dto.go` (lines ~39-40, ~80-81), `internal/model/budget_dto.go` (line ~415) — add `swaggertype:"string"` tags
- Possibly modify: any code doing `string(x)` / `vo.FlexString("…")` conversions (flushed by the compiler)

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `type FlexString struct` with methods `String() string`, `StrPtr() *string`, `FromNumber() bool`, `UnmarshalJSON`, `MarshalJSON`
  - `func NewFlexString(s string) FlexString` — for tests/fixtures that previously used a `vo.FlexString("…")` conversion
  - Task 10 depends on `FromNumber()`.

- [ ] **Step 1: Extend the tests**

Rewrite `internal/shared/vo/flexstring_test.go` to cover the existing decode contract PLUS the new flag (keep any existing cases that don't duplicate these):

```go
package vo

import (
	"encoding/json"
	"testing"
)

func TestFlexString_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		want       string
		fromNumber bool
	}{
		{"string", `"123.45"`, "123.45", false},
		{"number verbatim", `123.45`, "123.45", true},
		{"integer number", `100`, "100", true},
		{"scientific number verbatim", `1.5e3`, "1.5e3", true},
		{"null", `null`, "", false},
		{"empty string", `""`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s FlexString
			if err := json.Unmarshal([]byte(c.in), &s); err != nil {
				t.Fatalf("unmarshal %s: %v", c.in, err)
			}
			if s.String() != c.want {
				t.Errorf("String() = %q, want %q", s.String(), c.want)
			}
			if s.FromNumber() != c.fromNumber {
				t.Errorf("FromNumber() = %v, want %v", s.FromNumber(), c.fromNumber)
			}
		})
	}
}

func TestFlexString_UnmarshalJSON_ResetsPriorState(t *testing.T) {
	var s FlexString
	if err := json.Unmarshal([]byte(`100`), &s); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`"200"`), &s); err != nil {
		t.Fatal(err)
	}
	if s.FromNumber() {
		t.Error("FromNumber() should reset to false on a string decode")
	}
}

func TestFlexString_MarshalJSON(t *testing.T) {
	var s FlexString
	if err := json.Unmarshal([]byte(`123.45`), &s); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"123.45"` {
		t.Errorf("MarshalJSON = %s, want %q", b, `"123.45"`)
	}
}

func TestFlexString_StrPtr(t *testing.T) {
	if (*FlexString)(nil).StrPtr() != nil {
		t.Error("nil receiver should map to nil")
	}
	s := NewFlexString("9.99")
	if got := s.StrPtr(); got == nil || *got != "9.99" {
		t.Errorf("StrPtr() = %v, want 9.99", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/shared/vo/`
Expected: FAIL — `FromNumber`, `NewFlexString`, struct semantics don't exist yet.

- [ ] **Step 3: Rewrite `internal/shared/vo/flexstring.go`**

```go
package vo

import (
	"bytes"
	"encoding/json"
)

// FlexString is a decimal-string request field that decodes from JSON as either
// a string OR a number.
//
// The frozen wire contract treats money fields (amount, amountRecipient,
// balance, the budget limit) as normalized decimal strings, but it was set when
// scalars deserialized leniently, so numeric bodies were always accepted and
// third-party clients may still send them. FlexString keeps that leniency and
// records which form arrived (FromNumber), so the HTTP edge can log the
// deprecated numeric form without rejecting it.
//
// A JSON number is captured VERBATIM (its source bytes), not via float parsing,
// so no precision is lost — 123.45 stays "123.45". The captured value flows
// into NewDecimal downstream, which normalizes plain and scientific forms.
type FlexString struct {
	value      string
	fromNumber bool
}

// NewFlexString builds a FlexString holding s (for tests and fixtures).
func NewFlexString(s string) FlexString { return FlexString{value: s} }

// UnmarshalJSON accepts a JSON string, a JSON number, or null (-> ""). For a
// quoted string the quotes are stripped; any other scalar is captured verbatim
// and flagged as numeric.
func (s *FlexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*s = FlexString{}
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		s.value = str
		return nil
	}
	s.value = string(b)
	s.fromNumber = true
	return nil
}

// MarshalJSON renders the canonical form: always a JSON string.
func (s FlexString) MarshalJSON() ([]byte, error) { return json.Marshal(s.value) }

// String returns the underlying string.
func (s FlexString) String() string { return s.value }

// FromNumber reports whether the value decoded from a JSON number (the
// deprecated lenient form) rather than a string.
func (s FlexString) FromNumber() bool { return s.fromNumber }

// StrPtr dereferences a *FlexString to a *string, preserving nil.
func (s *FlexString) StrPtr() *string {
	if s == nil {
		return nil
	}
	v := s.value
	return &v
}
```

- [ ] **Step 4: Flush conversions and tag the model fields**

Run: `go build ./...`
Fix every error: `string(x)` → `x.String()`, `vo.FlexString("…")` → `vo.NewFlexString("…")` (find candidates with `grep -rn "FlexString(" internal/ | grep -v NewFlexString`).

Then add the swagger override so the OpenAPI spec keeps typing these fields as strings now that the Go type is a struct — on all 5 fields:

```go
	Balance    vo.FlexString `json:"balance" swaggertype:"string"`
```

(`internal/model/account_dto.go` ×2, `internal/model/transaction_dto.go` amount + amountRecipient ×2 each, `internal/model/budget_dto.go` set-limit `Amount *vo.FlexString` ×1.)

- [ ] **Step 5: Regenerate the OpenAPI docs and verify no drift**

Run: `make swagger && git diff --stat -- '*docs*' '*swagger*'`
Expected: no diff (the fields were already documented as strings). A diff here means the swaggertype tag is missing or misspelled on some field.

- [ ] **Step 6: Run the backend tests**

Run: `go test ./internal/shared/vo/ && make go-test`
Expected: PASS, zero golden diffs (decode behavior is byte-identical).

- [ ] **Step 7: Commit**

```bash
git add internal/shared/vo internal/model
git commit -m "feat(api): FlexString records the deprecated numeric form without changing the wire contract"
```

---

### Task 10: Backend — WARN once per request that sent numeric amounts

**Files:**
- Create: `internal/web/endpoint/numericamount.go`
- Create: `internal/web/endpoint/numericamount_test.go`
- Modify: `internal/web/endpoint/endpoint.go` (two call sites)

**Interfaces:**
- Consumes: `vo.FlexString.FromNumber()` (Task 9), `middleware.RequestIDFromCtx` (exists), `r.Pattern` (Go ≥1.23; module is at 1.25).
- Produces: `warnNumericAmounts(r *http.Request, req any)` — package-private; called by `Handle` and `HandlePublic` after a successful decode.

- [ ] **Step 1: Write the failing test**

Create `internal/web/endpoint/numericamount_test.go`:

```go
package endpoint

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
)

type numericAmountReq struct {
	Balance vo.FlexString  `json:"balance"`
	Amount  *vo.FlexString `json:"amount"`
	Name    string         `json:"name"`
}

func captureWarns(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestWarnNumericAmounts_NumberFields(t *testing.T) {
	buf := captureWarns(t)
	var req numericAmountReq
	if err := json.Unmarshal([]byte(`{"balance": 100.5, "amount": 9.99, "name": "x"}`), &req); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/account/create-account", nil)
	warnNumericAmounts(r, &req)

	out := buf.String()
	if !strings.Contains(out, "deprecated numeric amount") {
		t.Fatalf("expected the WARN line, got: %s", out)
	}
	if !strings.Contains(out, "fields=balance,amount") {
		t.Errorf("expected fields=balance,amount, got: %s", out)
	}
	if strings.Contains(out, "100.5") || strings.Contains(out, "9.99") {
		t.Errorf("amount VALUES must never be logged: %s", out)
	}
}

func TestWarnNumericAmounts_SilentForStrings(t *testing.T) {
	buf := captureWarns(t)
	var req numericAmountReq
	if err := json.Unmarshal([]byte(`{"balance": "100.5", "name": "x"}`), &req); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/account/create-account", nil)
	warnNumericAmounts(r, &req)
	if buf.Len() != 0 {
		t.Errorf("string amounts must not warn, got: %s", buf.String())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/web/endpoint/`
Expected: FAIL — `warnNumericAmounts` undefined.

- [ ] **Step 3: Implement `internal/web/endpoint/numericamount.go`**

```go
package endpoint

import (
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

var flexStringType = reflect.TypeOf(vo.FlexString{})

// warnNumericAmounts emits one WARN when a decoded request carried any money
// field as a JSON number — the deprecated lenient form the contract still
// accepts. Field names only, never values (amounts are user financial data).
func warnNumericAmounts(r *http.Request, req any) {
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	var fields []string
	for i := 0; i < t.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Pointer {
			if f.IsNil() {
				continue
			}
			f = f.Elem()
		}
		if f.Type() != flexStringType {
			continue
		}
		if !f.Interface().(vo.FlexString).FromNumber() {
			continue
		}
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" {
			name = t.Field(i).Name
		}
		fields = append(fields, name)
	}
	if len(fields) == 0 {
		return
	}
	slog.Warn("deprecated numeric amount",
		slog.String("route", r.Pattern),
		slog.String("request_id", middleware.RequestIDFromCtx(r.Context())),
		slog.String("fields", strings.Join(fields, ",")))
}
```

- [ ] **Step 4: Wire it into the combinators**

In `internal/web/endpoint/endpoint.go`, in BOTH `Handle` and `HandlePublic`, right after the successful `httpx.DecodeValidate` block:

```go
	warnNumericAmounts(r, &req)
```

(`HandleNoBody` has no body — nothing to add.)

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/web/endpoint/ && make go-test`
Expected: PASS, zero golden diffs (a log line is not part of the response contract). Note: apiparity scenarios that post numeric amounts will now print WARN lines in test output — that is expected and harmless.

- [ ] **Step 6: Commit**

```bash
git add internal/web/endpoint
git commit -m "feat(api): WARN when a request sends money amounts as JSON numbers"
```

---

### Task 11: Full verification

**Files:** none (verification only; fix regressions where they surface).

- [ ] **Step 1: Backend full smoke**

Run: `make go-test`
Expected: PASS. Then prove the goldens never moved:

```bash
git status --porcelain internal/test/apiparity/testdata/ ; git diff --stat internal/test/apiparity/testdata/
```

Expected: empty output. Any golden change is a contract regression — find and fix the cause; never regenerate.

- [ ] **Step 2: Frontend full suite**

Run: `cd web && pnpm build && pnpm test && pnpm lint`
Expected: all green.

- [ ] **Step 3: Full engine-comparison tier (needs Docker or `DATABASE_TEST_PGSQL_URL`)**

Run: `make test`
Expected: PASS. If the environment has no PostgreSQL available, state that explicitly in the completion report instead of skipping silently.

- [ ] **Step 4: End-to-end large-amount round trip (manual smoke)**

Run the server (`make go-run`) and the SPA (`make web-run`), create a transaction with amount `12345678901234567.89`, reload, and confirm the list and account balance render every digit. (Skill note: use the project's `verify` skill if executing interactively.)

- [ ] **Step 5: Final commit (if any fixups landed)**

```bash
git add -A && git commit -m "test: verify decimal-string amounts end to end"
```

---

## Self-Review Notes (already applied)

- **Spec coverage:** contract pinning (Tasks 3, 9, 11), frontend strings end-to-end (Tasks 1–8), backend WARN (Tasks 9–10), OpenAPI annotations (Task 9 Step 4–5), parity vectors (Task 1), zero-golden-diff gate (Tasks 9–11). The spec's "amounts stay strings in the TanStack Query cache" is enforced by construction: the api layer returns wire strings and nothing converts them.
- **Known accepted divergences** (document in commit messages if a reviewer asks): (1) formula input (`5+5`) still evaluates through the float calculator — formulas are a convenience, plain input is exact; (2) `decimal.div` returns `'0'` on division by zero where the backend panics — display-safe; (3) rounding of negative `x.5` display values is half-away-from-zero (decimal) vs `Math.round`'s toward-+∞ — display-only edge.
- **Type consistency:** the `ExchangeFn` shape `(fromCurrencyId: string, toCurrencyId: string, amount: string) => string` is identical in Tasks 4, 5, 6, 7. `BucketStats`/`WidgetMath` string fields match their consumers (Task 8 flush catches any missed component).
