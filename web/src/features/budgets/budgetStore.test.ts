import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useBudgetPeriodStore } from './budgetStore'
import * as metrics from '@/lib/metrics'

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

describe('the default period follows the budget', () => {
  const now = new Date(2026, 9, 30, 12)
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(now)
    useBudgetPeriodStore.setState({ selectedDate: '2026-10-01', periodChosen: false, adoptedPeriod: null })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('a fresh store on 30 October opens a day-28 budget on November, without a date-change event', () => {
    const track = vi.spyOn(metrics, 'trackEvent')
    useBudgetPeriodStore.getState().followCurrentPeriod(28)
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-11-01')
    expect(track).not.toHaveBeenCalled()
  })

  it('a calendar-month budget stays on October', () => {
    useBudgetPeriodStore.getState().followCurrentPeriod(1)
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-10-01')
  })

  it('a month the user picked is never overridden', () => {
    const track = vi.spyOn(metrics, 'trackEvent')
    useBudgetPeriodStore.getState().setPeriod('2026-10-01')
    expect(track).toHaveBeenCalledWith(metrics.METRICS.BUDGET_CHANGE_DATE)
    useBudgetPeriodStore.getState().followCurrentPeriod(28)
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-10-01')
  })

  it('a stored month other than the calendar one is kept', () => {
    useBudgetPeriodStore.setState({ selectedDate: '2026-08-01' })
    useBudgetPeriodStore.getState().followCurrentPeriod(28)
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01')
  })

  it('a default it moved follows the next budget opened', () => {
    useBudgetPeriodStore.getState().followCurrentPeriod(28)
    useBudgetPeriodStore.getState().followCurrentPeriod(1)
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-10-01')
  })

  it('the choice is not persisted', () => {
    useBudgetPeriodStore.getState().setPeriod('2026-09-01')
    const stored = JSON.parse(localStorage.getItem('budgetPeriod') ?? '{}') as { state: Record<string, unknown> }
    expect(stored.state.selectedDate).toBe('2026-09-01')
    expect(stored.state).not.toHaveProperty('periodChosen')
    expect(stored.state).not.toHaveProperty('adoptedPeriod')
  })
})
