import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import { budgetTransactionsParams } from './budgetTransactionsParams'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'

const base = { name: 'x', icon: 'x', currencyId: null }
const at = (target: BudgetTransactionsTarget) => budgetTransactionsParams('b1', target, '2026-07-01')
const period = { budgetId: 'b1', periodStart: '2026-07-01' }

it('narrows a category listed under a tag to the intersection', () => {
  expect(at({ ...base, id: 'c1', type: BudgetElementType.CATEGORY, parent: { id: 't1', type: BudgetElementType.TAG } })).toEqual({
    ...period,
    categoryId: 'c1',
    tagId: 't1',
  })
})

it('selects Uncategorized by flag', () => {
  expect(at({ ...base, id: UNCATEGORIZED_ID, type: BudgetElementType.CATEGORY })).toEqual({ ...period, uncategorized: true })
})

it('selects a savings row by its account', () => {
  expect(at({ ...base, id: 'acc1', type: BudgetElementType.SAVINGS })).toEqual({ ...period, accountId: 'acc1' })
})

it('selects an income envelope as income', () => {
  expect(at({ ...base, id: 'e1', type: BudgetElementType.INCOME_ENVELOPE })).toEqual({ ...period, income: true, envelopeId: 'e1' })
})
