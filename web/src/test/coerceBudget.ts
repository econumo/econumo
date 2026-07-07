import type { BudgetDto } from '@/api/dto/budget'

// Test helper: apply the API layer's decimal-string coercions to the wire fixture.
export function coerceBudgetFixture(wire: unknown): BudgetDto {
  const raw = JSON.parse(JSON.stringify(wire)) as BudgetDto
  const numOrNull = (v: unknown) => (v === null || v === undefined ? null : Number(v))
  raw.balances = raw.balances.map((b) => ({
    ...b,
    startBalance: numOrNull(b.startBalance),
    endBalance: numOrNull(b.endBalance),
    income: numOrNull(b.income),
    expenses: numOrNull(b.expenses),
    exchanges: numOrNull(b.exchanges),
    holdings: numOrNull(b.holdings),
  }))
  raw.currencyRates = raw.currencyRates.map((r) => ({ ...r, rate: Number(r.rate) }))
  raw.structure.elements = raw.structure.elements.map((el) => ({
    ...el,
    spent: Number(el.spent),
    budgetSpent: Number(el.budgetSpent),
    budgeted: Number(el.budgeted),
    available: Number(el.available),
    children: el.children.map((c) => ({ ...c, spent: Number(c.spent), budgetSpent: Number(c.budgetSpent) })),
  }))
  return raw
}
