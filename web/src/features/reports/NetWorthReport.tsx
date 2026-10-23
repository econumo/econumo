import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Area, CartesianGrid, ComposedChart, Line, ReferenceLine, XAxis, YAxis } from 'recharts'
import type { NetWorthAccountDto } from '@/api/dto/report'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import type { ChartConfig } from '@/components/ui/chart'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useBudgets } from '@/features/budgets/queries'
import { useUserData } from '@/features/user/queries'
import { cmp, isNegative, isZero, sub } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { netWorthGroupOf, netWorthInsight, netWorthSeries } from './reportMath'
import { ReportFilters, Segmented } from './ReportFilters'
import { netWorthQueryArgs } from './reportPeriod'
import { usePeriodLabels } from './usePeriodLabels'
import { useNetWorthReport } from './queries'
import { compactNumber, signedMoney, useMoney } from './reportFormat'
import { seriesColors } from './reportColors'
import { TooltipBox } from './ReportTooltip'
import type { TooltipEntry } from './ReportTooltip'
import { ReportUpdatedAt } from './ReportUpdatedAt'
import { useReportTabs } from './reportTabs'
import type { ReportParams, ReportTab } from './reportTabs'

const PRESETS = [
  { value: 'last12', key: 'reports.networth.period_1y' },
  { value: 'last3y', key: 'reports.networth.period_3y' },
  { value: 'all', key: 'reports.networth.period_all' },
] as const
const TOTAL_LINE = 'var(--foreground)'

const changeClass = (v: string) => (isZero(v) ? 'text-muted-foreground' : isNegative(v) ? 'text-expense' : 'text-income')
const allZero = (a: NetWorthAccountDto) => a.balances.every((b) => isZero(b))

interface TableLine {
  id: string
  name: string
  start: string
  now: string
  change: string
}

const lineOf = (id: string, name: string, balances: string[]): TableLine => {
  const start = balances[0] ?? '0'
  const now = balances[balances.length - 1] ?? '0'
  return { id, name, start, now, change: sub(now, start) }
}

const byNow = (a: TableLine, b: TableLine) => cmp(b.now, a.now) || a.name.localeCompare(b.name)

export function NetWorthReport({ tab }: { tab: ReportTab }) {
  const { t, i18n } = useTranslation()
  const labels = usePeriodLabels()
  const p = tab.params
  const setParams = (patch: Partial<ReportParams>) => useReportTabs.getState().setParams(tab.id, patch)
  const report = useNetWorthReport(netWorthQueryArgs(p))
  const money = useMoney(report.data?.currencyId)
  const compact = useMemo(() => compactNumber(i18n.language), [i18n.language])
  const { data: user } = useUserData()
  const { data: accounts } = useAccounts()
  const { data: folders } = useFolders()
  const { data: budgets } = useBudgets()
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const scope = p.budgetId ? 'budget' : 'mine'

  const header = (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-start gap-2">
        <ReportFilters tab={tab} showStep={false} showPeriod={false} />
        <Segmented
          label={t('reports.filters.period')}
          value={p.period}
          options={PRESETS.map((o) => ({ value: o.value, label: t(o.key) }))}
          onChange={(period) => {
            setParams({ period })
            trackEvent(METRICS.REPORTS_CHANGE_PERIOD, { period })
          }}
        />
      </div>
      <ReportUpdatedAt query={report} />
    </div>
  )

  if (report.isPending || (report.isError && !report.data)) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        {report.isPending ? (
          <div className="h-72 animate-pulse rounded-xl bg-econumo-hover" />
        ) : (
          <p className="text-sm text-muted-foreground">{t('common.app.error')}</p>
        )}
      </div>
    )
  }

  const data = report.data
  const shown = { ...data, accounts: data.accounts.filter((a) => !allZero(a)) }
  if (data.months.length === 0 || shown.accounts.length === 0) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <p className="py-6 text-sm text-muted-foreground">{t('reports.networth.empty')}</p>
      </div>
    )
  }

  const budget = budgets?.find((b) => b.id === p.budgetId)
  const groupOf = netWorthGroupOf({
    me: user?.id ?? '',
    folderOf: Object.fromEntries((accounts ?? []).map((a) => [a.id, a.folderId])),
    folders: folders ?? [],
    owners: {
      ...Object.fromEntries((accounts ?? []).map((a) => [a.owner.id, a.owner.name])),
      ...Object.fromEntries((budget?.access ?? []).map((a) => [a.user.id, a.user.name])),
    },
    names: {
      noFolder: t('reports.networth.no_folder'),
      deleted: t('reports.networth.deleted'),
      otherOwner: t('reports.networth.other_owner'),
    },
  })
  const series = netWorthSeries(shown, groupOf)
  const groupLines = series.groups.map((g) => lineOf(g.id, g.name, g.balances)).sort(byNow)
  const order = groupLines.map((g) => g.id)
  const groups = [...series.groups].sort((a, b) => order.indexOf(a.id) - order.indexOf(b.id))
  const colors = seriesColors(order)
  const accountLines = (groupId: string) =>
    shown.accounts.filter((a) => groupOf(a).id === groupId).map((a) => lineOf(a.id, a.name, a.balances)).sort(byNow)
  const totalLine = lineOf('total', t('reports.spending.total'), series.total)
  const insight = netWorthInsight(series.total)

  const key = (i: number) => `g${i}`
  const chartData = data.months.map((m, mi) => ({
    label: labels.month(m),
    title: labels.month(m),
    total: Number(series.total[mi]),
    ...Object.fromEntries(groups.map((g, gi) => [key(gi), Number(g.balances[mi])])),
  }))
  const config: ChartConfig = Object.fromEntries([
    ...groups.map((g, gi) => [key(gi), { label: g.name, color: colors[g.id] }]),
    ['total', { label: totalLine.name, color: TOTAL_LINE }],
  ])
  const dim = report.isPlaceholderData ? 'opacity-60 transition-opacity' : ''

  const toggle = (id: string) => {
    const opening = !expanded.has(id)
    setExpanded((s) => {
      const next = new Set(s)
      if (opening) next.add(id)
      else next.delete(id)
      return next
    })
    if (opening) trackEvent(METRICS.REPORTS_NETWORTH_EXPAND, { scope })
  }

  const num = 'py-1.5 pl-2 text-right tabular-nums'
  const cellsOf = (l: TableLine) => (
    <>
      <td className={`hidden ${num} text-muted-foreground sm:table-cell`}>{money(l.start)}</td>
      <td className={num}>{money(l.now)}</td>
      <td className={`${num} whitespace-nowrap ${changeClass(l.change)}`}>{signedMoney(l.change, money)}</td>
    </>
  )

  return (
    <div className="flex flex-col gap-4">
      {header}
      <section aria-label={t('reports.spending.headline')} className={`flex flex-col gap-1 ${dim}`}>
        <span className="text-sm text-muted-foreground">{t('reports.networth.today')}</span>
        <span className="text-xl tabular-nums sm:text-2xl">{money(totalLine.now)}</span>
        {insight.key !== 'reports.insight.empty' && (
          <span className="flex flex-wrap gap-x-1.5 text-sm">
            <span className={changeClass(totalLine.change)}>{t(insight.key, { amount: money(insight.params.amount ?? '0') })}</span>
            <span className="text-muted-foreground">{t('reports.networth.since', { month: labels.month(data.months[0]) })}</span>
          </span>
        )}
      </section>
      <figure aria-label={t('reports.networth.chart')} className={`flex flex-col gap-2 ${dim}`}>
        <ChartContainer config={config} className="aspect-auto h-60 w-full sm:h-72">
          <ComposedChart data={chartData} margin={{ top: 8, right: 4, left: 0, bottom: 0 }} stackOffset="sign">
            <CartesianGrid vertical={false} />
            <XAxis dataKey="label" tickLine={false} axisLine={false} interval="preserveStartEnd" minTickGap={24} />
            <YAxis width={44} tickLine={false} axisLine={false} tickFormatter={compact} />
            <ReferenceLine y={0} stroke="var(--muted-foreground)" />
            <ChartTooltip
              content={(props) => (
                <TooltipBox
                  {...(props as { active?: boolean; payload?: readonly TooltipEntry[] })}
                  money={money}
                  series={[
                    ...groups.map((g, gi) => ({ id: key(gi), name: g.name, color: colors[g.id] })),
                    { id: 'total', name: totalLine.name, color: TOTAL_LINE },
                  ]}
                />
              )}
            />
            {groups.map((g, gi) => (
              <Area
                key={g.id}
                dataKey={key(gi)}
                stackId="nw"
                type="linear"
                stroke={colors[g.id]}
                fill={colors[g.id]}
                fillOpacity={0.55}
                isAnimationActive={false}
              />
            ))}
            <Line dataKey="total" type="linear" stroke={TOTAL_LINE} strokeWidth={2} dot={false} isAnimationActive={false} />
          </ComposedChart>
        </ChartContainer>
        <ul aria-label={t('reports.spending.legend')} className="flex flex-wrap gap-x-3 text-sm text-muted-foreground">
          {groups.map((g) => (
            <li key={g.id} className="flex items-center gap-1.5">
              <span className="size-2.5 shrink-0 rounded-sm" style={{ background: colors[g.id] }} />
              {g.name}
            </li>
          ))}
          <li className="flex items-center gap-1.5">
            <span className="h-0 w-4 border-t-2 border-foreground" />
            {totalLine.name}
          </li>
        </ul>
      </figure>
      <table className={`w-full table-fixed text-sm ${dim}`}>
        <thead>
          <tr className="border-b text-xs text-muted-foreground">
            <th className="py-1.5 pr-2 text-left font-normal">{t(p.budgetId ? 'reports.networth.col_group' : 'reports.networth.col_folder')}</th>
            <th className="hidden w-28 py-1.5 pl-2 text-right font-normal sm:table-cell">{t('reports.networth.col_start')}</th>
            <th className="w-24 py-1.5 pl-2 text-right font-normal sm:w-28">{t('reports.networth.col_now')}</th>
            <th className="w-24 py-1.5 pl-2 text-right font-normal sm:w-28">{t('reports.spending.col_change')}</th>
          </tr>
        </thead>
        <tbody>
          {groupLines.flatMap((g) => {
            const open = expanded.has(g.id)
            const rows = [
              <tr key={g.id} onClick={() => toggle(g.id)} className="cursor-pointer border-b border-border/60 hover:bg-econumo-hover/60">
                <td className="py-1.5 pr-2">
                  <span className="flex min-w-0 items-center gap-1.5">
                    <button
                      type="button"
                      aria-expanded={open}
                      aria-label={t(open ? 'reports.networth.collapse' : 'reports.networth.expand', { name: g.name })}
                      className="-ml-1 grid size-5 shrink-0 place-items-center rounded text-muted-foreground hover:bg-accent"
                      onClick={(e) => {
                        e.stopPropagation()
                        toggle(g.id)
                      }}
                    >
                      <ChevronRight className={`size-3.5 transition-transform ${open ? 'rotate-90' : ''}`} />
                    </button>
                    <span className="size-2.5 shrink-0 rounded-sm" style={{ background: colors[g.id] }} />
                    <span className="truncate">{g.name}</span>
                  </span>
                </td>
                {cellsOf(g)}
              </tr>,
            ]
            if (open) {
              for (const a of accountLines(g.id)) {
                rows.push(
                  <tr key={`${g.id}-${a.id}`} className="border-b border-border/60 text-muted-foreground">
                    <td className="py-1.5 pr-2"><span className="block truncate pl-10">{a.name}</span></td>
                    {cellsOf(a)}
                  </tr>,
                )
              }
            }
            return rows
          })}
        </tbody>
        <tfoot>
          <tr>
            <td className="py-1.5 pr-2"><span className="block truncate pl-10">{totalLine.name}</span></td>
            {cellsOf(totalLine)}
          </tr>
        </tfoot>
      </table>
      <p className="text-xs text-muted-foreground">{t(p.budgetId ? 'reports.networth.note_budget' : 'reports.networth.note_mine')}</p>
    </div>
  )
}
