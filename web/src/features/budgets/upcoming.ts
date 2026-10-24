import type {
  BudgetChildElementDto,
  BudgetDto,
  BudgetPlanDto,
  PlanMonthTransfersDto,
  PlanOpeningBalanceDto,
  PlanSavingsFlowDto,
  PlanTransferDto,
} from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { add, isZero, sub } from '@/lib/decimal'

// The server reports recurring payments not yet posted apart from the real
// money (spent/actual stay real). Every screen shows ONE combined figure, so
// the queries fold the two together here, once, and nothing downstream needs to
// know upcoming money exists. A field an older server does not send counts as
// zero, and a zero leaves the wire string untouched.

const plus = (value: string, upcoming: string | undefined): string =>
  upcoming === undefined || isZero(upcoming) ? value : add(value, upcoming)

const minus = (value: string, upcoming: string | undefined): string =>
  upcoming === undefined || isZero(upcoming) ? value : sub(value, upcoming)

// select runs per observer; caching per data object keeps the folded value
// referentially stable for as long as the cache entry is unchanged
const budgetFolds = new WeakMap<BudgetDto, BudgetDto>()
const planFolds = new WeakMap<BudgetPlanDto, BudgetPlanDto>()

const foldChild = (ch: BudgetChildElementDto): BudgetChildElementDto => ({
  ...ch,
  spent: plus(ch.spent, ch.upcoming),
  budgetSpent: plus(ch.budgetSpent, ch.budgetUpcoming),
})

export function foldBudgetUpcoming(b: BudgetDto): BudgetDto {
  const cached = budgetFolds.get(b)
  if (cached) {
    return cached
  }
  const { labels, savings } = b.structure
  const folded: BudgetDto = {
    ...b,
    structure: {
      ...b.structure,
      elements: b.structure.elements.map((el) => ({
        ...el,
        spent: plus(el.spent, el.upcoming),
        budgetSpent: plus(el.budgetSpent, el.budgetUpcoming),
        available: minus(minus(el.available, el.upcomingBefore), el.upcoming),
        children: el.children.map(foldChild),
      })),
      ...(labels
        ? {
            labels: labels.map((l) => ({
              ...l,
              spent: plus(l.spent, l.upcoming),
              ...(l.children ? { children: l.children.map(foldChild) } : {}),
            })),
          }
        : {}),
      // closingBalance is left alone: the server already counts upcoming money in it
      ...(savings
        ? { savings: savings.map((s) => ({ ...s, spent: plus(s.spent, s.upcoming), available: minus(s.available, s.upcoming) })) }
        : {}),
    },
  }
  budgetFolds.set(b, folded)
  return folded
}

// opening balances follow the server's rule for them: budget currency first,
// then discovery order (an upcoming-only currency is discovered last)
function mergeOpening(base: PlanOpeningBalanceDto[], extra: PlanOpeningBalanceDto[] | undefined, budgetCurrencyId: Id): PlanOpeningBalanceDto[] {
  if (!extra || extra.every((e) => isZero(e.amount))) {
    return base
  }
  const byCurrency = new Map(extra.map((e) => [e.currencyId, e.amount]))
  const merged = base.map((b) => ({ ...b, amount: plus(b.amount, byCurrency.get(b.currencyId)) }))
  const known = new Set(base.map((b) => b.currencyId))
  const added = extra.filter((e) => !known.has(e.currencyId) && !isZero(e.amount))
  const budgetFirst = added.filter((e) => e.currencyId === budgetCurrencyId)
  return [...budgetFirst, ...merged, ...added.filter((e) => e.currencyId !== budgetCurrencyId)]
}

// the server's order for per-currency rows: budget currency first, then by id
const currencyOrder = (budgetCurrencyId: Id) => (a: Id, b: Id): number => {
  if (a === b) {
    return 0
  }
  if (a === budgetCurrencyId) {
    return -1
  }
  if (b === budgetCurrencyId) {
    return 1
  }
  return a < b ? -1 : 1
}

function mergeTransfers(base: PlanMonthTransfersDto[], extra: PlanMonthTransfersDto[] | undefined, budgetCurrencyId: Id): PlanMonthTransfersDto[] {
  if (!extra || extra.every((m) => m.items.length === 0)) {
    return base
  }
  const order = currencyOrder(budgetCurrencyId)
  const byPeriod = new Map(extra.map((m) => [m.period, m.items]))
  const mergeItems = (items: PlanTransferDto[], upcoming: PlanTransferDto[]): PlanTransferDto[] => {
    if (upcoming.length === 0) {
      return items
    }
    const byCurrency = new Map(upcoming.map((t) => [t.currencyId, t]))
    const merged = items.map((t) => {
      const u = byCurrency.get(t.currencyId)
      return u ? { ...t, in: plus(t.in, u.in), out: plus(t.out, u.out) } : t
    })
    const known = new Set(items.map((t) => t.currencyId))
    return [...merged, ...upcoming.filter((t) => !known.has(t.currencyId))].sort((a, b) => order(a.currencyId, b.currencyId))
  }
  const knownPeriods = new Set(base.map((m) => m.period))
  return [
    ...base.map((m) => ({ ...m, items: mergeItems(m.items, byPeriod.get(m.period) ?? []) })),
    ...extra.filter((m) => !knownPeriods.has(m.period)),
  ]
}

function mergeFlows(base: PlanSavingsFlowDto[] | undefined, extra: PlanSavingsFlowDto[] | undefined, budgetCurrencyId: Id): PlanSavingsFlowDto[] | undefined {
  if (!extra || extra.length === 0) {
    return base
  }
  const order = currencyOrder(budgetCurrencyId)
  const key = (f: PlanSavingsFlowDto) => `${f.month}|${f.currencyId}`
  const byKey = new Map(extra.map((f) => [key(f), f.amount]))
  const rows = base ?? []
  const merged = rows.map((f) => ({ ...f, amount: plus(f.amount, byKey.get(key(f))) }))
  const known = new Set(rows.map(key))
  return [...merged, ...extra.filter((f) => !known.has(key(f)))].sort((a, b) =>
    a.month === b.month ? order(a.currencyId, b.currencyId) : a.month < b.month ? -1 : 1,
  )
}

export function foldPlanUpcoming(p: BudgetPlanDto): BudgetPlanDto {
  const cached = planFolds.get(p)
  if (cached) {
    return cached
  }
  const budgetCurrencyId = p.meta.currencyId
  const { savings } = p.structure
  const savingsFlows = mergeFlows(p.savingsFlows, p.upcomingSavingsFlows, budgetCurrencyId)
  const savingsIncomeExpense = mergeFlows(p.savingsIncomeExpense, p.upcomingSavingsIncomeExpense, budgetCurrencyId)
  const folded: BudgetPlanDto = {
    ...p,
    openingBalances: mergeOpening(p.openingBalances, p.upcomingOpeningBalances, budgetCurrencyId),
    ...(p.savingsOpeningBalances
      ? { savingsOpeningBalances: mergeOpening(p.savingsOpeningBalances, p.upcomingSavingsOpeningBalances, budgetCurrencyId) }
      : {}),
    transfers: mergeTransfers(p.transfers, p.upcomingTransfers, budgetCurrencyId),
    ...(savingsFlows ? { savingsFlows } : {}),
    ...(savingsIncomeExpense ? { savingsIncomeExpense } : {}),
    structure: {
      ...p.structure,
      elements: p.structure.elements.map((el) => ({
        ...el,
        cells: el.cells.map((c) => ({ ...c, actual: plus(c.actual, c.upcoming) })),
        children: el.children.map((ch) => ({
          ...ch,
          cells: ch.cells.map((c) => ({ ...c, actual: plus(c.actual, c.upcoming) })),
        })),
      })),
      // a plan closing balance is booked only: it takes every upcoming change of
      // its row from the window start through its month
      ...(savings
        ? {
            savings: savings.map((row) => {
              let due = '0'
              return {
                ...row,
                cells: row.cells.map((c) => {
                  due = plus(due, c.upcoming)
                  return {
                    ...c,
                    actual: plus(c.actual, c.upcoming),
                    ...(c.closingBalance !== undefined ? { closingBalance: plus(c.closingBalance, due) } : {}),
                  }
                }),
              }
            }),
          }
        : {}),
    },
  }
  planFolds.set(p, folded)
  return folded
}
