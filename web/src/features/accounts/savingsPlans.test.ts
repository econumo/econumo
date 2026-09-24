import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { BudgetElementType, type BudgetDto, type BudgetPlanDto } from '@/api/dto/budget'
import { countBudgetsPlanningSavings } from './savingsPlans'

function savingsRow(id: string, budgeted: string) {
  return {
    id, type: BudgetElementType.SAVINGS, name: id, icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1',
    isArchived: 0 as const, position: 0, budgeted, spent: '0', available: budgeted,
  }
}

function budgetDetail(id: string, savings: ReturnType<typeof savingsRow>[] | undefined): BudgetDto {
  return {
    meta: { id, ownerUserId: 'u1', name: id, startedAt: '2026-01-01 00:00:00', endedAt: '', currencyId: 'cur-usd', isArchived: 0, access: [] },
    filters: { periodStart: '2026-09-01 00:00:00', periodEnd: '2026-10-01 00:00:00' },
    balances: [],
    currencyRates: [],
    structure: { folders: [], elements: [], ...(savings ? { savings } : {}) },
  }
}

function planWindow(id: string, savings: { id: string; planned: string[] }[]): BudgetPlanDto {
  return {
    meta: { id, ownerUserId: 'u1', name: id, startedAt: '2026-01-01 00:00:00', endedAt: '', currencyId: 'cur-usd', isArchived: 0, access: [] },
    months: ['2026-09-01', '2026-10-01'],
    openingBalances: [],
    currencyRates: [],
    transfers: [],
    structure: {
      folders: [],
      elements: [],
      savings: savings.map((s) => ({
        id: s.id, type: BudgetElementType.SAVINGS, name: s.id, icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1',
        isArchived: 0 as const, position: 0, cells: s.planned.map((planned) => ({ actual: '0', planned })),
      })),
    },
  }
}

function seeded(): QueryClient {
  const qc = new QueryClient()
  qc.setQueryData([...queryKeys.budget, 'b1', '2026-09-01'], budgetDetail('b1', [savingsRow('A', '100'), savingsRow('B', '0')]))
  // a second cached period of the same budget must not count b1 twice
  qc.setQueryData([...queryKeys.budget, 'b1', '2026-08-01'], budgetDetail('b1', [savingsRow('A', '20')]))
  qc.setQueryData([...queryKeys.budgetPlan, 'b2', '2026-07-01', 6], planWindow('b2', [
    { id: 'A', planned: ['', '50'] },
    { id: 'B', planned: ['', ''] },
  ]))
  // no default budget selected: the detail query caches null
  qc.setQueryData([...queryKeys.budget, 'none', '2026-09-01'], null)
  // a budget from an older server without the savings field
  qc.setQueryData([...queryKeys.budget, 'b3', '2026-09-01'], budgetDetail('b3', undefined))
  return qc
}

it('counts distinct budgets that plan savings for the account across detail and plan caches', () => {
  expect(countBudgetsPlanningSavings(seeded(), 'A')).toBe(2)
})

it('ignores a zero monthly budgeted amount and empty plan cells', () => {
  expect(countBudgetsPlanningSavings(seeded(), 'B')).toBe(0)
})

it('returns 0 for an account no cached budget knows', () => {
  expect(countBudgetsPlanningSavings(seeded(), 'unknown')).toBe(0)
})

it('counts a budget once when both its detail and its plan window plan the account', () => {
  const qc = seeded()
  qc.setQueryData([...queryKeys.budgetPlan, 'b1', '2026-07-01', 6], planWindow('b1', [{ id: 'A', planned: ['10', ''] }]))
  expect(countBudgetsPlanningSavings(qc, 'A')).toBe(2)
})
