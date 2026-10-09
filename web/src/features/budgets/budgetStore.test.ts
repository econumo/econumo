import { beforeEach, expect, it } from 'vitest'
import { useBudgetPeriodStore } from './budgetStore'

beforeEach(() => {
  localStorage.clear()
})

it('splits the fold state both views shared before into one per view', async () => {
  localStorage.setItem(
    'budgetPeriod',
    JSON.stringify({ state: { unfoldedElements: { e1: true }, planFolds: { income: true } }, version: 0 }),
  )
  await useBudgetPeriodStore.persist.rehydrate()
  const s = useBudgetPeriodStore.getState()
  expect(s.unfoldedElements).toEqual({ e1: true })
  expect(s.planUnfoldedElements).toEqual({ e1: true })
  expect(s.planFolds).toEqual({ income: true })
  expect(s.budgetFolds).toEqual({ income: true })

  s.toggleBudgetFold('income')
  s.togglePlanElement('e1')
  const next = useBudgetPeriodStore.getState()
  expect(next.budgetFolds).toEqual({})
  expect(next.planFolds).toEqual({ income: true })
  expect(next.planUnfoldedElements).toEqual({})
  expect(next.unfoldedElements).toEqual({ e1: true })
})

it('a budget switch resets both views’ element folds', () => {
  useBudgetPeriodStore.setState({ foldBudgetId: 'b1', unfoldedElements: { e1: true }, planUnfoldedElements: { e2: true } })
  useBudgetPeriodStore.getState().resetFoldsFor('b2')
  expect(useBudgetPeriodStore.getState().unfoldedElements).toEqual({})
  expect(useBudgetPeriodStore.getState().planUnfoldedElements).toEqual({})
})
