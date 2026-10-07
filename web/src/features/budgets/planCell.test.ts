import { describe, expect, it } from 'vitest'
import { BudgetElementType } from '@/api/dto/budget'
import { planCellView } from './planCell'

const cat = BudgetElementType.CATEGORY
const sel = '2026-10-01'

describe('planCellView', () => {
  it('shows actual and plan up to the selected month', () => {
    expect(planCellView({ type: cat, cell: { actual: '612', planned: '600' }, month: '2026-09-01', selected: sel })).toEqual({ actual: '612', plan: '600', over: true })
    expect(planCellView({ type: cat, cell: { actual: '341', planned: '600' }, month: sel, selected: sel })).toEqual({ actual: '341', plan: '600', over: false })
  })
  it('shows only the plan after the selected month', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '55' }, month: '2026-11-01', selected: sel })).toEqual({ actual: null, plan: '55', over: false })
  })
  it('leaves an unplanned month blank, never 0', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '' }, month: '2026-11-01', selected: sel }).plan).toBeNull()
  })
  it('never marks income or savings as over', () => {
    expect(planCellView({ type: BudgetElementType.INCOME_CATEGORY, cell: { actual: '9', planned: '1' }, month: sel, selected: sel }).over).toBe(false)
    expect(planCellView({ type: BudgetElementType.SAVINGS, cell: { actual: '9', planned: '1' }, month: sel, selected: sel }).over).toBe(false)
  })
  it('reads a missing cell as nothing at all', () => {
    expect(planCellView({ type: cat, cell: undefined, month: sel, selected: sel })).toEqual({ actual: null, plan: null, over: false })
  })
})
