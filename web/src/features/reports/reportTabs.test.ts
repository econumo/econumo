import { beforeEach, expect, it } from 'vitest'
import { MAX_REPORT_TABS, defaultParams, tabTitle, useReportTabs } from './reportTabs'
import type { ReportTab } from './reportTabs'

const store = () => useReportTabs.getState()

beforeEach(() => {
  localStorage.clear()
  useReportTabs.setState({ userId: null, tabs: [], activeId: '' })
})

it('first visit has one list tab, active', () => {
  store().ensureFor('u1')
  const { tabs, activeId, userId } = store()
  expect(userId).toBe('u1')
  expect(tabs).toHaveLength(1)
  expect(tabs[0].kind).toBe('list')
  expect(tabs[0].title).toBeNull()
  expect(tabs[0].params).toEqual(defaultParams(null))
  expect(activeId).toBe(tabs[0].id)
})

it('defaultParams: last 3 years by year, over time, expenses by category, bars', () => {
  expect(defaultParams('b1')).toEqual({
    budgetId: 'b1', period: 'last3y', step: 'year', mode: 'overTime', side: 'expense', groupBy: 'category',
    selectedId: null, compare: null, view: 'bars',
  })
})

it('a new first tab starts in the given scope', () => {
  store().ensureFor('u1', 'b1')
  expect(store().tabs[0].params.budgetId).toBe('b1')
})

it('openNew adds a list tab, activates it and inherits the active tab scope', () => {
  store().ensureFor('u1', 'b1')
  const id = store().openNew()
  const { tabs, activeId } = store()
  expect(tabs).toHaveLength(2)
  expect(tabs[1].id).toBe(id)
  expect(tabs[1].kind).toBe('list')
  expect(tabs[1].params.budgetId).toBe('b1')
  expect(activeId).toBe(id)
})

it('choose turns a tab into a report', () => {
  store().ensureFor('u1')
  const id = store().tabs[0].id
  store().choose(id, 'spending')
  expect(store().tabs[0].kind).toBe('spending')
})

it('rename sets a title; a blank title goes back to the generated one', () => {
  store().ensureFor('u1')
  const id = store().tabs[0].id
  store().rename(id, '  Groceries 3y ')
  expect(store().tabs[0].title).toBe('Groceries 3y')
  store().rename(id, '   ')
  expect(store().tabs[0].title).toBeNull()
})

it('closing the last tab leaves one list tab', () => {
  store().ensureFor('u1')
  const id = store().tabs[0].id
  store().choose(id, 'networth')
  store().close(id)
  const { tabs, activeId } = store()
  expect(tabs).toHaveLength(1)
  expect(tabs[0].kind).toBe('list')
  expect(tabs[0].id).not.toBe(id)
  expect(activeId).toBe(tabs[0].id)
})

it('closing the active tab activates its neighbour', () => {
  store().ensureFor('u1')
  const first = store().tabs[0].id
  const second = store().openNew()!
  const third = store().openNew()!
  store().activate(second)
  store().close(second)
  expect(store().tabs.map((t) => t.id)).toEqual([first, third])
  expect(store().activeId).toBe(third)
  store().close(third)
  expect(store().activeId).toBe(first)
})

it('duplicate copies the tab right after it and activates the copy', () => {
  store().ensureFor('u1')
  const id = store().tabs[0].id
  store().choose(id, 'spending')
  store().setParams(id, { selectedId: 'cat-food' })
  store().rename(id, 'Food')
  store().openNew()
  const copy = store().duplicate(id)
  const { tabs, activeId } = store()
  expect(tabs).toHaveLength(3)
  expect(tabs[1].id).toBe(copy)
  expect(tabs[1]).toMatchObject({ kind: 'spending', title: 'Food', params: { selectedId: 'cat-food' } })
  expect(activeId).toBe(copy)
})

it('move reorders tabs', () => {
  store().ensureFor('u1')
  const a = store().tabs[0].id
  const b = store().openNew()!
  const c = store().openNew()!
  store().move(c, 0)
  expect(store().tabs.map((t) => t.id)).toEqual([c, a, b])
  store().move(c, 5)
  expect(store().tabs.map((t) => t.id)).toEqual([a, b, c])
})

it('setParams merges a patch into one tab', () => {
  store().ensureFor('u1')
  const id = store().tabs[0].id
  store().setParams(id, { step: 'month', budgetId: 'b2' })
  expect(store().tabs[0].params).toMatchObject({ step: 'month', budgetId: 'b2', mode: 'overTime' })
})

it('a different user resets the tabs', () => {
  store().ensureFor('u1')
  store().openNew()
  store().ensureFor('u1')
  expect(store().tabs).toHaveLength(2)
  store().ensureFor('u2')
  expect(store().userId).toBe('u2')
  expect(store().tabs).toHaveLength(1)
  expect(store().tabs[0].kind).toBe('list')
})

it('repairs a tab whose budget is gone', () => {
  store().ensureFor('u1', 'b-gone')
  const id = store().tabs[0].id
  store().setParams(id, { selectedId: 'el-1' })
  store().repair(id, ['b1', 'b2'])
  expect(store().tabs[0].params.budgetId).toBeNull()
  expect(store().tabs[0].params.selectedId).toBeNull()
  store().setParams(id, { budgetId: 'b1' })
  store().repair(id, ['b1', 'b2'])
  expect(store().tabs[0].params.budgetId).toBe('b1')
})

it('tolerates a malformed stored state', async () => {
  localStorage.setItem('econumo.reportTabs', JSON.stringify({ state: { userId: 'u1', tabs: 'nope', activeId: 7 }, version: 0 }))
  await useReportTabs.persist.rehydrate()
  store().ensureFor('u1')
  expect(store().tabs).toHaveLength(1)
  expect(store().activeId).toBe(store().tabs[0].id)
})

it('fills params a stored tab is missing', async () => {
  localStorage.setItem('econumo.reportTabs', JSON.stringify({
    state: { userId: 'u1', tabs: [{ id: 't1', kind: 'spending', title: null, params: { budgetId: 'b1' } }], activeId: 't1' },
    version: 0,
  }))
  await useReportTabs.persist.rehydrate()
  store().ensureFor('u1')
  expect(store().tabs[0].params).toEqual({ ...defaultParams('b1') })
})

it(`caps tabs at ${MAX_REPORT_TABS}`, () => {
  store().ensureFor('u1')
  for (let i = 0; i < 20; i++) store().openNew()
  expect(store().tabs).toHaveLength(MAX_REPORT_TABS)
  expect(store().openNew()).toBeNull()
  expect(store().duplicate(store().tabs[0].id)).toBeNull()
  expect(store().tabs).toHaveLength(MAX_REPORT_TABS)
})

const names = {
  report: { list: 'New tab', spending: 'Spending', cashflow: 'Cash flow', networth: 'Net worth' },
  rows: { 'cat-food': 'Groceries' } as Record<string, string>,
  versus: (later: number, earlier: number) => `${later} vs ${earlier}`,
}
const tab = (over: Partial<ReportTab> = {}, params: Partial<ReportTab['params']> = {}): ReportTab => ({
  id: 't', kind: 'spending', title: null, ...over, params: { ...defaultParams(null), ...params },
})

it('tabTitle: user title wins, else generated from the report and its main parameter', () => {
  expect(tabTitle(tab({ title: 'Mine' }), names)).toBe('Mine')
  expect(tabTitle(tab({ kind: 'list' }), names)).toBe('New tab')
  expect(tabTitle(tab({ kind: 'networth' }), names)).toBe('Net worth')
  expect(tabTitle(tab(), names)).toBe('Spending')
  expect(tabTitle(tab({}, { selectedId: 'cat-food' }), names)).toBe('Groceries')
  expect(tabTitle(tab({}, { selectedId: 'gone' }), names)).toBe('Spending')
  expect(tabTitle(tab({}, { mode: 'compare', compare: [2025, 2026] }), names)).toBe('2026 vs 2025')
  expect(tabTitle(tab({}, { mode: 'compare', compare: [2025, 2026], selectedId: 'cat-food' }), names)).toBe('Groceries · 2026 vs 2025')
  expect(tabTitle(tab({}, { mode: 'overTime', compare: [2025, 2026] }), names)).toBe('Spending')
})
