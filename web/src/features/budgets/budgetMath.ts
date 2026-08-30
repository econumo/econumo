import type { BudgetBalanceDto, BudgetDto, BudgetElementDto, BudgetFolderDto } from '@/api/dto/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { compareNames } from '@/lib/collate'
import { abs, add, cmp, div } from '@/lib/decimal'
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

export interface WidgetMath {
  spent: string
  total: string
  /** ratio for the progress bar; float precision is fine for a CSS width */
  progress: number
  overspent: boolean
}

// nulls count as zero; negative exchange/holdings fold into spent, positive into total.
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
