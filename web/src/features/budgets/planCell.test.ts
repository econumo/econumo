import { describe, expect, it } from 'vitest'
import { BudgetElementType } from '@/api/dto/budget'
import { planCellView, shownActual } from './planCell'

const cat = BudgetElementType.CATEGORY
const sel = '2026-10-01'

describe('planCellView', () => {
  it('shows actual and plan up to the selected month', () => {
    expect(planCellView({ type: cat, cell: { actual: '612', planned: '600' }, month: '2026-09-01', selected: sel })).toEqual({ actual: '612', dash: false, plan: '600', over: true })
    expect(planCellView({ type: cat, cell: { actual: '341', planned: '600' }, month: sel, selected: sel })).toEqual({ actual: '341', dash: false, plan: '600', over: false })
  })
  it('shows only the plan after the selected month', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '55' }, month: '2026-11-01', selected: sel })).toEqual({ actual: null, dash: false, plan: '55', over: false })
  })
  it('leaves an unplanned month blank, never 0', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '' }, month: '2026-11-01', selected: sel }).plan).toBeNull()
  })
  it('marks a planned month with nothing spent yet with a dash: "— · 55"', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '55' }, month: sel, selected: sel })).toEqual({ actual: null, dash: true, plan: '55', over: false })
    expect(planCellView({ type: cat, cell: { actual: '0.00', planned: '55' }, month: '2026-09-01', selected: sel })).toEqual({ actual: null, dash: true, plan: '55', over: false })
  })
  it('leaves a month with neither an actual nor a plan blank', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '' }, month: sel, selected: sel })).toEqual({ actual: null, dash: false, plan: null, over: false })
  })
  it('shows an actual with no plan on its own, red when it is overspent', () => {
    expect(planCellView({ type: cat, cell: { actual: '20', planned: '' }, month: sel, selected: sel })).toEqual({ actual: '20', dash: false, plan: null, over: true })
  })
  it('never marks income or savings as over', () => {
    expect(planCellView({ type: BudgetElementType.INCOME_CATEGORY, cell: { actual: '9', planned: '1' }, month: sel, selected: sel }).over).toBe(false)
    expect(planCellView({ type: BudgetElementType.SAVINGS, cell: { actual: '9', planned: '1' }, month: sel, selected: sel }).over).toBe(false)
  })
  it('reads a missing cell as nothing at all', () => {
    expect(planCellView({ type: cat, cell: undefined, month: sel, selected: sel })).toEqual({ actual: null, dash: false, plan: null, over: false })
  })
})

describe('shownActual', () => {
  it('keeps a non-zero actual, dashes a zero one next to a plan, drops it otherwise', () => {
    expect(shownActual('12', false)).toEqual({ actual: '12', dash: false })
    expect(shownActual('0', true)).toEqual({ actual: null, dash: true })
    expect(shownActual('0', false)).toEqual({ actual: null, dash: false })
    expect(shownActual(null, true)).toEqual({ actual: null, dash: false })
  })
})
