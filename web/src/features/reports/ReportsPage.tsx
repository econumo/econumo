import { useEffect, useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import type { FlowReportDto } from '@/api/dto/report'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { ChevronLeft } from 'lucide-react'
import { UserOptions } from '@/api/dto/user'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { useBudgets } from '@/features/budgets/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useUserData, userOption } from '@/features/user/queries'
import { useIsCompact } from '@/hooks/useIsCompact'
import { METRICS, trackEvent } from '@/lib/metrics'
import { ReportList } from './ReportList'
import { MAX_REPORT_TABS, REPORT_QUESTION, tabTitle, useReportTabs } from './reportTabs'
import type { ReportKind, ReportTab } from './reportTabs'
import { TabBar } from './TabBar'
import { flowReportQuery } from './queries'
import { flowQueryArgs } from './reportPeriod'
import { SpendingReport } from './SpendingReport'
import { CashFlowReport } from './CashFlowReport'
import { NetWorthReport } from './NetWorthReport'
import { spendingRowNames } from './reportRows'

function ReportBody({ tab }: { tab: ReportTab }) {
  const { t } = useTranslation()
  const setParams = useReportTabs((s) => s.setParams)
  const choose = useReportTabs((s) => s.choose)
  if (tab.kind === 'list') {
    return (
      <ReportList
        budgetId={tab.params.budgetId}
        onScope={(budgetId) => setParams(tab.id, { budgetId, selectedId: null })}
        onChoose={(kind) => {
          choose(tab.id, kind)
          trackEvent(METRICS.REPORTS_OPEN_REPORT, { report: kind, scope: tab.params.budgetId ? 'budget' : 'mine' })
        }}
      />
    )
  }
  if (tab.kind === 'spending') {
    return (
      <div className="flex flex-col gap-3">
        <h2 className="text-lg font-normal">{t(REPORT_QUESTION.spending)}</h2>
        <SpendingReport tab={tab} />
      </div>
    )
  }
  if (tab.kind === 'cashflow') {
    return (
      <div className="flex flex-col gap-3">
        <h2 className="text-lg font-normal">{t(REPORT_QUESTION.cashflow)}</h2>
        <CashFlowReport tab={tab} />
      </div>
    )
  }
  return (
    <NetWorthReport tab={tab} />
  )
}

export function ReportsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const { data: user } = useUserData()
  const { data: budgets } = useBudgets()
  const { data: categories } = useCategories()
  const { data: tags } = useTags()
  const { data: payees } = usePayees()
  const store = useReportTabs()
  const userId = user?.id
  const ready = !!userId && store.userId === userId && store.tabs.length > 0

  useEffect(() => {
    trackEvent(METRICS.REPORTS_OPEN)
  }, [])

  useEffect(() => {
    if (!user) return
    const { ensureFor, activate } = useReportTabs.getState()
    ensureFor(user.id, userOption(user, UserOptions.BUDGET))
    activate(useReportTabs.getState().tabs[0].id)
    // the page opens on the first tab once per visit, not on every user-data refetch
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [userId])

  const tabIds = store.tabs.map((tab) => tab.id).join('|')
  useEffect(() => {
    if (!ready || !budgets) return
    const ids = budgets.map((b) => b.id)
    const { tabs, repair } = useReportTabs.getState()
    for (const tab of tabs) repair(tab.id, ids)
  }, [ready, budgets, tabIds])

  // titles name a selected row; budget rows (envelopes) are only known from the
  // report itself, so each spending tab's cached report is read, never fetched
  const spendingTabs = store.tabs.filter((tab) => tab.kind === 'spending' && tab.params.selectedId)
  const cached = useQueries({
    queries: spendingTabs.map((tab) => ({ ...flowReportQuery(flowQueryArgs(tab.params)), enabled: false })),
  })
  const reportRows = cached.map((q, i) => {
    const data = q.data as FlowReportDto | undefined
    return spendingRowNames(data?.[spendingTabs[i].params.side], t)
  })
  const reportRowsKey = JSON.stringify(reportRows)

  const names = useMemo(() => ({
    report: {
      list: t('reports.tabs.new'),
      spending: t('reports.kind.spending'),
      cashflow: t('reports.kind.cashflow'),
      networth: t('reports.kind.networth'),
    } satisfies Record<ReportKind, string>,
    rows: {
      ...Object.fromEntries([...(categories ?? []), ...(tags ?? []), ...(payees ?? [])].map((r) => [r.id, r.name])),
      ...Object.assign({}, ...(JSON.parse(reportRowsKey) as Record<string, string>[])),
    },
    versus: (later: number, earlier: number) => t('reports.tabs.versus', { later, earlier }),
  }), [t, categories, tags, payees, reportRowsKey])

  const active = store.tabs.find((tab) => tab.id === store.activeId) ?? store.tabs[0]

  return (
    <div className="flex min-h-full flex-col gap-3 p-2.5 sm:p-4">
      <header className="flex items-center gap-2">
        {isCompact ? (
          <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(RouterPage.HOME)}>
            <ChevronLeft className="size-5" />
          </Button>
        ) : null}
        <h1 className="min-w-0 truncate text-lg sm:text-xl">{t('reports.page.title')}</h1>
      </header>
      {ready && active ? (
        <>
          <TabBar
            tabs={store.tabs.map((tab) => ({ id: tab.id, title: tabTitle(tab, names) }))}
            activeId={active.id}
            canAdd={store.tabs.length < MAX_REPORT_TABS}
            onActivate={store.activate}
            onNew={() => {
              if (store.openNew()) trackEvent(METRICS.REPORTS_TAB_NEW)
            }}
            onClose={(id) => {
              store.close(id)
              trackEvent(METRICS.REPORTS_TAB_CLOSE)
            }}
            onRename={(id, title) => {
              store.rename(id, title)
              trackEvent(METRICS.REPORTS_TAB_RENAME)
            }}
            onDuplicate={(id) => {
              if (store.duplicate(id)) trackEvent(METRICS.REPORTS_TAB_DUPLICATE)
            }}
            onMove={(id, toIndex) => {
              store.move(id, toIndex)
              trackEvent(METRICS.REPORTS_TAB_MOVE)
            }}
          />
          <div role="tabpanel" aria-label={tabTitle(active, names)} className="w-full max-w-4xl">
            <ReportBody tab={active} />
          </div>
        </>
      ) : null}
    </div>
  )
}
