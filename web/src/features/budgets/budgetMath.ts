import type { BudgetDto, BudgetElementDto, BudgetFolderDto } from '@/api/dto/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { compareNames } from '@/lib/collate'
import { add, cmp, div, isZero } from '@/lib/decimal'
import { exchange } from '@/lib/exchange'

export interface BucketStats {
  budgeted: string
  spent: string
  available: string
}

export interface FolderBucket {
  folder: BudgetFolderDto | null
  elements: BudgetElementDto[]
  stats: BucketStats
}

export interface BudgetBuckets {
  withFolder: FolderBucket[]
  withoutFolder: FolderBucket
  archive: FolderBucket
  // categoryless spending: its own read-only section, never a drag container
  uncategorized: FolderBucket
}

type ExchangeFn = (fromCurrencyId: string, toCurrencyId: string, amount: string) => string

export function makeBudgetExchange(budget: BudgetDto, currencies: CurrencyDto[]): ExchangeFn {
  // budget math uses the period-scoped rates embedded in the response
  const rates = budget.currencyRates.map((r) => ({ ...r, updatedAt: r.periodStart }))
  return (from, to, amount) => exchange(from, to, amount, rates, currencies)
}

// Folder-bucket stats: budgeted/available exchanged into the budget currency;
// spent uses budgetSpent (already budget-currency, no exchange).
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

export function bucketElements(budget: BudgetDto, exchangeFn: ExchangeFn, lang = 'en'): BudgetBuckets {
  const folders = [...budget.structure.folders].sort((a, b) => a.position - b.position)
  const elements = budget.structure.elements
  // The uncategorized element is presentation-only (no persisted row to move or
  // budget), so it is pulled out before bucketing and never joins a folder. It
  // only earns a place when the selected month has uncategorized spend.
  const uncategorizedElements = elements.filter(
    (el) => el.id === UNCATEGORIZED_ID && el.isArchived === 0 && cmp(el.spent, '0') !== 0,
  )
  const active = elements.filter((el) => el.isArchived === 0 && el.id !== UNCATEGORIZED_ID)
  const byPosition = (a: BudgetElementDto, b: BudgetElementDto) => a.position - b.position

  // Vue quirk: zero folders -> ALL active elements land in the no-folder bucket
  const withFolder: FolderBucket[] =
    folders.length === 0
      ? []
      : folders.map((folder) => {
          const folderElements = active.filter((el) => el.folderId === folder.id).sort(byPosition)
          return { folder, elements: folderElements, stats: bucketStats(folderElements, budget, exchangeFn) }
        })

  const folderless =
    folders.length === 0 ? [...active].sort(byPosition) : active.filter((el) => el.folderId === null).sort(byPosition)

  // archive is read-only history: a row earns its place only if one of its
  // displayed numbers (budget, spent, available) is nonzero
  const archived = elements
    .filter((el) => el.isArchived === 1 && el.id !== UNCATEGORIZED_ID)
    .filter((el) => cmp(el.budgeted, '0') !== 0 || cmp(el.spent, '0') !== 0 || cmp(displayAvailable(el), '0') !== 0)
    .sort((a, b) => compareNames(a.name, b.name, lang))

  return {
    withFolder,
    withoutFolder: { folder: null, elements: folderless, stats: bucketStats(folderless, budget, exchangeFn) },
    archive: { folder: null, elements: archived, stats: bucketStats(archived, budget, exchangeFn) },
    uncategorized: {
      folder: null,
      elements: uncategorizedElements,
      stats: bucketStats(uncategorizedElements, budget, exchangeFn),
    },
  }
}

export function budgetTotals(buckets: BudgetBuckets): BucketStats {
  const all = [...buckets.withFolder.map((b) => b.stats), buckets.withoutFolder.stats, buckets.archive.stats]
  const totals = all.reduce(
    (acc, s) => ({ budgeted: add(acc.budgeted, s.budgeted), spent: add(acc.spent, s.spent), available: add(acc.available, s.available) }),
    { budgeted: '0', spent: '0', available: '0' },
  )
  // Categoryless spending is real money out, so it still counts toward the
  // spent total — but it can never be budgeted, so it adds nothing to the
  // budgeted/available totals.
  return { ...totals, spent: add(totals.spent, buckets.uncategorized.stats.spent) }
}

/** The Total row with the savings rows added in, in the budget currency: saving is
 *  money leaving the everyday accounts like spending, so planned joins budgeted,
 *  saved joins spent and remaining joins available — the figures the block shows. */
export function totalsWithSavings(totals: BucketStats, budget: BudgetDto, exchangeFn: ExchangeFn): BucketStats {
  const base = budget.meta.currencyId
  return (budget.structure.savings ?? []).reduce(
    (acc, row) => ({
      budgeted: add(acc.budgeted, exchangeFn(row.currencyId, base, row.budgeted)),
      spent: add(acc.spent, exchangeFn(row.currencyId, base, row.spent)),
      available: add(acc.available, exchangeFn(row.currencyId, base, row.available)),
    }),
    totals,
  )
}

export interface SavingsTotals {
  /** what the month saves: actual for a past month; from the current month on,
   *  each live row's larger of planned and saved (the Plan view's Savings line) */
  savings: string
  /** end-of-month balance; null when the server sends no balance */
  balance: string | null
}

/** The phone Total card's savings lines, in the budget currency (the phone rows
 *  leave the Balance column out). null when the budget has no savings rows.
 *  `projected`: the month is the caller's current one or later. */
export function savingsTotals(budget: BudgetDto, exchangeFn: ExchangeFn, projected: boolean): SavingsTotals | null {
  const rows = budget.structure.savings ?? []
  if (rows.length === 0) {
    return null
  }
  const base = budget.meta.currencyId
  const savings = rows.reduce((acc, row) => {
    const amount = projected && row.isArchived === 0 && cmp(row.budgeted, row.spent) > 0 ? row.budgeted : row.spent
    return add(acc, exchangeFn(row.currencyId, base, amount))
  }, '0')
  const balance = rows.some((row) => row.closingBalance === undefined)
    ? null
    : rows.reduce((acc, row) => add(acc, exchangeFn(row.currencyId, base, row.closingBalance ?? '0')), '0')
  return { savings, balance }
}

export const displayAvailable = (el: { available: string; budgeted: string }): string => add(el.available, el.budgeted)

// The wire name for the Uncategorized element is the English literal
// "Uncategorized" (see internal/model.UncategorizedName); the SPA renders the
// translated label instead, everywhere this element's (or its tag-child
// copy's) name would show.
export const elementDisplayName = (id: string, name: string, t: (key: string) => string): string =>
  id === UNCATEGORIZED_ID ? t('common.uncategorized') : name

export interface PeriodItem {
  value: string
  label: string
  isActive: boolean
  /** before the budget's start month: browsable but read-only, rendered dimmed */
  outsideBudget: boolean
  /** past the end month of an ended budget: not offered at all */
  afterEnd: boolean
}

export const MONTHS_AROUND = 23

// The one month-label rule for every budget surface (period strip, plan sheet):
// this year's months by full name only, any other year as "Mon YYYY".
export function periodLabeler(lang: string, now: Date = new Date()): (d: Date) => string {
  const currentYear = now.getFullYear()
  const longMonth = new Intl.DateTimeFormat(lang, { month: 'long' })
  const shortMonth = new Intl.DateTimeFormat(lang, { month: 'short' })
  return (d) => (d.getFullYear() === currentYear ? longMonth.format(d) : `${shortMonth.format(d)} ${d.getFullYear()}`)
}

export function periodRange(
  selectedDate: string,
  startedAt: string | null,
  monthsBefore = MONTHS_AROUND,
  monthsAfter = MONTHS_AROUND,
  lang = 'en',
  endedAt: string | null = null,
): PeriodItem[] {
  const [y, m] = selectedDate.split('-').map(Number)
  const startMonth = startedAt ? startedAt.slice(0, 7) : null
  const endMonth = endedAt ? endedAt.slice(0, 7) : null
  const label = periodLabeler(lang)
  const items: PeriodItem[] = []
  for (let offset = -monthsBefore; offset <= monthsAfter; offset++) {
    const d = new Date(y, m - 1 + offset, 1)
    const value = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
    items.push({
      value,
      label: label(d),
      isActive: offset === 0,
      outsideBudget: startMonth !== null && value.slice(0, 7) < startMonth,
      afterEnd: endMonth !== null && endMonth !== '' && value.slice(0, 7) > endMonth,
    })
  }
  return items
}

export type RowState = 'none' | 'ok' | 'covered' | 'over'

/** The one colour rule for an expense row. `available` is the displayed Available
 *  (`displayAvailable`); a future month has no spending yet, so it has no state. */
export function rowState(row: { budgeted: string; spent: string; available: string }, future = false): RowState {
  if (future || (isZero(row.budgeted) && isZero(row.spent))) {
    return 'none'
  }
  if (cmp(row.available, '0') < 0) {
    return 'over'
  }
  return cmp(row.spent, row.budgeted) > 0 ? 'covered' : 'ok'
}

/** Spending against what this month can draw on: its budget plus what earlier
 *  months left (an earlier overspend does not shrink the bar's scale). */
export function rowProgress(row: { budgeted: string; spent: string; carry?: string }, future = false): number | null {
  const carry = row.carry !== undefined && cmp(row.carry, '0') > 0 ? row.carry : '0'
  const pool = add(row.budgeted, carry)
  if (future || cmp(pool, '0') <= 0) {
    return null
  }
  return Math.max(0, Math.min(Number(div(row.spent, pool)), 1))
}

/** The row turns red only when this month spent more than its budget and what
 *  earlier months left does not cover it. `available` is the displayed Available. */
export function overBudget(row: { budgeted: string; spent: string; available: string }, future = false): boolean {
  return !future && cmp(row.spent, row.budgeted) > 0 && cmp(row.available, '0') < 0
}

// the wire `available` already nets this month's spending against what earlier
// months left, so adding the spending back leaves the carry-over alone
export const carryOver = (el: { available: string; spent: string }): string => add(el.available, el.spent)
