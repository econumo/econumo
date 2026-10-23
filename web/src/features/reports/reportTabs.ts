import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { v7 as uuidv7 } from 'uuid'
import type { GroupBy } from '@/api/dto/report'
import type { Id } from '@/api/types'
import type { Step } from './reportMath'

export type ReportKind = 'list' | 'spending' | 'cashflow' | 'networth'

export interface ReportParams {
  budgetId: Id | null
  period: 'thisYear' | 'last12' | 'last3y' | 'all' | 'custom'
  from?: string
  to?: string
  step: Step
  mode: 'overTime' | 'compare'
  side: 'expense' | 'income'
  groupBy: GroupBy
  selectedId: Id | null
  compare: [number, number] | null
  view: 'bars' | 'flow'
}

export interface ReportTab {
  id: string
  kind: ReportKind
  /** null = generated from the report and its main parameter */
  title: string | null
  params: ReportParams
}

export const MAX_REPORT_TABS = 12

export const REPORT_QUESTION: Record<Exclude<ReportKind, 'list'>, string> = {
  spending: 'reports.list.spending_q',
  cashflow: 'reports.list.cashflow_q',
  networth: 'reports.list.networth_q',
}

const KINDS: ReportKind[] = ['list', 'spending', 'cashflow', 'networth']

export function defaultParams(budgetId: Id | null): ReportParams {
  return {
    budgetId,
    period: 'last3y',
    step: 'year',
    mode: 'overTime',
    side: 'expense',
    groupBy: 'category',
    selectedId: null,
    compare: null,
    view: 'bars',
  }
}

function listTab(budgetId: Id | null): ReportTab {
  return { id: uuidv7(), kind: 'list', title: null, params: defaultParams(budgetId) }
}

// A stored tab comes from an older build or a hand-edited localStorage: anything
// unreadable is dropped, missing params fall back to the defaults.
function sanitize(raw: unknown): ReportTab | null {
  if (!raw || typeof raw !== 'object') return null
  const t = raw as Partial<ReportTab>
  if (typeof t.id !== 'string' || !KINDS.includes(t.kind as ReportKind)) return null
  return {
    id: t.id,
    kind: t.kind as ReportKind,
    title: typeof t.title === 'string' && t.title.trim() ? t.title : null,
    params: sanitizeParams(t.params),
  }
}

const PERIODS: ReportParams['period'][] = ['thisYear', 'last12', 'last3y', 'all', 'custom']
const STEPS: Step[] = ['month', 'quarter', 'year']
const MODES: ReportParams['mode'][] = ['overTime', 'compare']
const SIDES: ReportParams['side'][] = ['expense', 'income']
const GROUP_BYS: GroupBy[] = ['category', 'payee', 'tag']
const VIEWS: ReportParams['view'][] = ['bars', 'flow']
const MONTH = /^\d{4}-(0[1-9]|1[0-2])-01$/

function oneOf<T>(allowed: readonly T[], value: unknown, fallback: T): T {
  return allowed.includes(value as T) ? (value as T) : fallback
}

function sanitizeParams(raw: unknown): ReportParams {
  const p = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const d = defaultParams(typeof p.budgetId === 'string' ? p.budgetId : null)
  const from = typeof p.from === 'string' && MONTH.test(p.from) ? p.from : undefined
  const to = typeof p.to === 'string' && MONTH.test(p.to) ? p.to : undefined
  const period = oneOf(PERIODS, p.period, d.period)
  const compare = Array.isArray(p.compare) && p.compare.length === 2 && p.compare.every((y) => Number.isInteger(y))
    ? [p.compare[0] as number, p.compare[1] as number] as [number, number]
    : null
  const out: ReportParams = {
    ...d,
    // a custom range without a readable start would silently become "all time"
    period: period === 'custom' && !from ? d.period : period,
    step: oneOf(STEPS, p.step, d.step),
    mode: oneOf(MODES, p.mode, d.mode),
    side: oneOf(SIDES, p.side, d.side),
    groupBy: oneOf(GROUP_BYS, p.groupBy, d.groupBy),
    view: oneOf(VIEWS, p.view, d.view),
    selectedId: typeof p.selectedId === 'string' ? p.selectedId : null,
    compare,
  }
  if (from) out.from = from
  if (to) out.to = to
  return out
}

interface ReportTabsState {
  userId: Id | null
  tabs: ReportTab[]
  activeId: string
  ensureFor: (userId: Id, budgetId?: Id | null) => void
  /** the id of the new tab, null at the tab cap */
  openNew: () => string | null
  choose: (id: string, kind: ReportKind) => void
  rename: (id: string, title: string) => void
  close: (id: string) => void
  duplicate: (id: string) => string | null
  move: (id: string, toIndex: number) => void
  activate: (id: string) => void
  setParams: (id: string, patch: Partial<ReportParams>) => void
  repair: (id: string, budgetIds: Id[]) => void
}

export const useReportTabs = create<ReportTabsState>()(
  persist(
    (set, get) => {
      const patchTab = (id: string, fn: (tab: ReportTab) => ReportTab) =>
        set((s) => ({ tabs: s.tabs.map((t) => (t.id === id ? fn(t) : t)) }))

      return {
        userId: null,
        tabs: [],
        activeId: '',
        ensureFor: (userId, budgetId = null) => {
          const s = get()
          const tabs = s.userId === userId && Array.isArray(s.tabs) ? s.tabs.map(sanitize).filter((t) => t !== null) : []
          if (tabs.length === 0) {
            const tab = listTab(budgetId)
            set({ userId, tabs: [tab], activeId: tab.id })
            return
          }
          const activeId = tabs.some((t) => t.id === s.activeId) ? s.activeId : tabs[0].id
          set({ userId, tabs, activeId })
        },
        openNew: () => {
          const s = get()
          if (s.tabs.length >= MAX_REPORT_TABS) return null
          const active = s.tabs.find((t) => t.id === s.activeId)
          const tab = listTab(active?.params.budgetId ?? null)
          set({ tabs: [...s.tabs, tab], activeId: tab.id })
          return tab.id
        },
        choose: (id, kind) => patchTab(id, (t) => ({ ...t, kind })),
        rename: (id, title) => patchTab(id, (t) => ({ ...t, title: title.trim() || null })),
        close: (id) => {
          const s = get()
          const index = s.tabs.findIndex((t) => t.id === id)
          if (index === -1) return
          const tabs = s.tabs.filter((t) => t.id !== id)
          if (tabs.length === 0) {
            const tab = listTab(s.tabs[index].params.budgetId)
            set({ tabs: [tab], activeId: tab.id })
            return
          }
          const activeId = s.activeId === id ? tabs[Math.min(index, tabs.length - 1)].id : s.activeId
          set({ tabs, activeId })
        },
        duplicate: (id) => {
          const s = get()
          const index = s.tabs.findIndex((t) => t.id === id)
          if (index === -1 || s.tabs.length >= MAX_REPORT_TABS) return null
          const source = s.tabs[index]
          const copy: ReportTab = { ...source, id: uuidv7(), params: { ...source.params } }
          const tabs = [...s.tabs]
          tabs.splice(index + 1, 0, copy)
          set({ tabs, activeId: copy.id })
          return copy.id
        },
        move: (id, toIndex) => {
          const tabs = [...get().tabs]
          const from = tabs.findIndex((t) => t.id === id)
          if (from === -1) return
          const [tab] = tabs.splice(from, 1)
          tabs.splice(Math.max(0, Math.min(toIndex, tabs.length)), 0, tab)
          set({ tabs })
        },
        activate: (id) => {
          if (get().tabs.some((t) => t.id === id)) set({ activeId: id })
        },
        setParams: (id, patch) => patchTab(id, (t) => ({ ...t, params: { ...t.params, ...patch } })),
        repair: (id, budgetIds) => {
          const tab = get().tabs.find((t) => t.id === id)
          if (!tab || tab.params.budgetId === null || budgetIds.includes(tab.params.budgetId)) return
          // a selected row belongs to the scope it was picked in
          patchTab(id, (t) => ({ ...t, params: { ...t.params, budgetId: null, selectedId: null } }))
        },
      }
    },
    {
      name: 'econumo.reportTabs',
      version: 0,
      partialize: (s) => ({ userId: s.userId, tabs: s.tabs, activeId: s.activeId }),
    },
  ),
)

export interface TabTitleNames {
  report: Record<ReportKind, string>
  rows: Record<Id, string>
  versus: (later: number, earlier: number) => string
}

export function tabTitle(tab: ReportTab, names: TabTitleNames): string {
  if (tab.title) return tab.title
  if (tab.kind === 'list') return names.report.list
  const row = tab.params.selectedId ? names.rows[tab.params.selectedId] : undefined
  const compare = tab.params.mode === 'compare' && tab.params.compare ? tab.params.compare : null
  const years = compare ? names.versus(compare[1], compare[0]) : null
  if (row && years) return `${row} · ${years}`
  return row ?? years ?? names.report[tab.kind]
}
