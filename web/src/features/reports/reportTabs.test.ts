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

it('defaultParams: the last 3 years', () => {
  expect(defaultParams('b1')).toEqual({ budgetId: 'b1', period: 'last3y' })
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
  store().choose(id, 'networth')
  expect(store().tabs[0].kind).toBe('networth')
})

it('rename sets a title; a blank title goes back to the generated one', () => {
  store().ensureFor('u1')
  const id = store().tabs[0].id
  store().rename(id, '  Family 3y ')
  expect(store().tabs[0].title).toBe('Family 3y')
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
  store().choose(id, 'networth')
  store().setParams(id, { period: 'all' })
  store().rename(id, 'Everything')
  store().openNew()
  const copy = store().duplicate(id)
  const { tabs, activeId } = store()
  expect(tabs).toHaveLength(3)
  expect(tabs[1].id).toBe(copy)
  expect(tabs[1]).toMatchObject({ kind: 'networth', title: 'Everything', params: { period: 'all' } })
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
  store().setParams(id, { budgetId: 'b2' })
  expect(store().tabs[0].params).toEqual({ budgetId: 'b2', period: 'last3y' })
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
  store().repair(id, ['b1', 'b2'])
  expect(store().tabs[0].params.budgetId).toBeNull()
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

const storedTabs = async (tabs: unknown[]) => {
  localStorage.setItem('econumo.reportTabs', JSON.stringify({ state: { userId: 'u1', tabs, activeId: 't1' }, version: 0 }))
  await useReportTabs.persist.rehydrate()
  store().ensureFor('u1')
  return store().tabs
}

it('fills params a stored tab is missing and replaces unknown ones', async () => {
  const tabs = await storedTabs([{ id: 't1', kind: 'networth', title: null, params: { budgetId: 'b1', period: 'forever' } }])
  expect(tabs[0].params).toEqual(defaultParams('b1'))
})

it('keeps valid stored values', async () => {
  const tabs = await storedTabs([{ id: 't1', kind: 'networth', title: null, params: { budgetId: null, period: 'all' } }])
  expect(tabs[0].params).toEqual({ budgetId: null, period: 'all' })
})

// a beta build offered these; the tab goes, the others stay
it('drops a stored tab of a report that is not offered', async () => {
  const tabs = await storedTabs([
    { id: 't1', kind: 'spending', title: null, params: {} },
    { id: 't2', kind: 'networth', title: null, params: {} },
  ])
  expect(tabs.map((t) => t.id)).toEqual(['t2'])
  expect(store().activeId).toBe('t2')
})

it(`caps tabs at ${MAX_REPORT_TABS}`, () => {
  store().ensureFor('u1')
  for (let i = 0; i < 20; i++) store().openNew()
  expect(store().tabs).toHaveLength(MAX_REPORT_TABS)
  expect(store().openNew()).toBeNull()
  expect(store().duplicate(store().tabs[0].id)).toBeNull()
  expect(store().tabs).toHaveLength(MAX_REPORT_TABS)
})

const tab = (over: Partial<ReportTab> = {}): ReportTab => ({ id: 't', kind: 'networth', title: null, params: defaultParams(null), ...over })

it('tabTitle: the user title wins, else the report name', () => {
  const names = { list: 'New tab', networth: 'Net worth' }
  expect(tabTitle(tab({ title: 'Mine' }), names)).toBe('Mine')
  expect(tabTitle(tab({ kind: 'list' }), names)).toBe('New tab')
  expect(tabTitle(tab(), names)).toBe('Net worth')
})
