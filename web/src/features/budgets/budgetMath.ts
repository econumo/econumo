import type { BudgetBalanceDto, BudgetDto, BudgetElementDto, BudgetFolderDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { exchange } from '@/lib/exchange'

export interface BucketStats {
  budgeted: number
  spent: number
  available: number
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
}

type ExchangeFn = (fromCurrencyId: string, toCurrencyId: string, amount: number) => number

export function makeBudgetExchange(budget: BudgetDto, currencies: CurrencyDto[]): ExchangeFn {
  // budget math uses the period-scoped rates embedded in the response
  const rates = budget.currencyRates.map((r) => ({ ...r, updatedAt: r.periodStart }))
  return (from, to, amount) => exchange(from, to, amount, rates, currencies)
}

// Port of the Vue folder-bucket stats: budgeted/available exchanged into the
// budget currency; spent uses budgetSpent (already budget-currency, no exchange).
export function bucketStats(elements: BudgetElementDto[], budget: BudgetDto, exchangeFn: ExchangeFn): BucketStats {
  const base = budget.meta.currencyId
  const stats: BucketStats = { budgeted: 0, spent: 0, available: 0 }
  for (const el of elements) {
    const from = el.currencyId ?? base
    stats.budgeted += exchangeFn(from, base, el.budgeted)
    stats.spent += el.budgetSpent
    stats.available += exchangeFn(from, base, el.available + el.budgeted)
  }
  return stats
}

export function bucketElements(budget: BudgetDto, exchangeFn: ExchangeFn): BudgetBuckets {
  const folders = [...budget.structure.folders].sort((a, b) => a.position - b.position)
  const elements = budget.structure.elements
  const active = elements.filter((el) => el.isArchived === 0)
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

  const archived = elements.filter((el) => el.isArchived === 1).sort((a, b) => a.name.localeCompare(b.name))

  return {
    withFolder,
    withoutFolder: { folder: null, elements: folderless, stats: bucketStats(folderless, budget, exchangeFn) },
    archive: { folder: null, elements: archived, stats: bucketStats(archived, budget, exchangeFn) },
  }
}

export function budgetTotals(buckets: BudgetBuckets): BucketStats {
  const all = [...buckets.withFolder.map((b) => b.stats), buckets.withoutFolder.stats, buckets.archive.stats]
  return all.reduce(
    (acc, s) => ({ budgeted: acc.budgeted + s.budgeted, spent: acc.spent + s.spent, available: acc.available + s.available }),
    { budgeted: 0, spent: 0, available: 0 },
  )
}

export const displaySpent = (spent: number): number => -spent
export const displayAvailable = (el: { available: number; budgeted: number }): number => el.available + el.budgeted

export interface PeriodItem {
  value: string
  label: string
  isActive: boolean
  outsideBudget: boolean
}

export const MONTHS_AROUND = 23

export function periodRange(
  selectedDate: string,
  startedAt: string | null,
  monthsBefore = MONTHS_AROUND,
  monthsAfter = MONTHS_AROUND,
): PeriodItem[] {
  const [y, m] = selectedDate.split('-').map(Number)
  const currentYear = new Date().getFullYear()
  const startMonth = startedAt ? startedAt.slice(0, 7) : null
  const items: PeriodItem[] = []
  for (let offset = -monthsBefore; offset <= monthsAfter; offset++) {
    const d = new Date(y, m - 1 + offset, 1)
    const value = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
    const monthName = new Intl.DateTimeFormat('en', { month: 'long' }).format(d)
    const shortName = new Intl.DateTimeFormat('en', { month: 'short' }).format(d)
    items.push({
      value,
      label: d.getFullYear() === currentYear ? monthName : `${shortName} ${d.getFullYear()}`,
      isActive: offset === 0,
      outsideBudget: startMonth !== null && value.slice(0, 7) < startMonth,
    })
  }
  return items
}

export interface WidgetMath {
  spent: number
  total: number
  progress: number
  overspent: boolean
}

// Port of BudgetExpenseWidget: nulls count as zero; negative exchange/holdings
// fold into spent, positive into total.
export function widgetMath(balance: BudgetBalanceDto | undefined): WidgetMath {
  const n = (v: number | null | undefined) => Number(v ?? 0)
  const expenses = n(balance?.expenses)
  const exchanges = n(balance?.exchanges)
  const holdings = n(balance?.holdings)
  const startBalance = n(balance?.startBalance)
  const income = n(balance?.income)

  let spent = Math.abs(expenses)
  if (exchanges < 0) spent += Math.abs(exchanges)
  if (holdings < 0) spent += Math.abs(holdings)

  let total = Math.abs(startBalance + income)
  if (exchanges > 0) total += exchanges
  if (holdings > 0) total += holdings

  const progress = total <= 0 ? 0 : Math.max(0, Math.min(spent / total, 1))
  return { spent, total, progress, overspent: spent > total }
}
