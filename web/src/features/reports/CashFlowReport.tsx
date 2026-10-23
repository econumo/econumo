import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, CartesianGrid, ComposedChart, Line, ReferenceLine, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import type { ChartConfig } from '@/components/ui/chart'
import { currentMonth } from '@/features/budgets/planMath'
import { useIsMobile } from '@/hooks/useIsMobile'
import { add, isNegative, isZero, sub } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { cashFlow, cashFlowInsight, rollUp } from './reportMath'
import type { Bucket } from './reportMath'
import { ReportFilters, Segmented } from './ReportFilters'
import { flowQueryArgs, lastMonthOf } from './reportPeriod'
import type { MonthRange } from './reportPeriod'
import { usePeriodLabels } from './usePeriodLabels'
import { useFlowReport } from './queries'
import { compactNumber, signedMoney, useMoney } from './reportFormat'
import { TooltipBox } from './ReportTooltip'
import type { TooltipEntry } from './ReportTooltip'
import { SankeyFlow } from './SankeyFlow'
import { useReportTabs } from './reportTabs'
import type { ReportParams, ReportTab } from './reportTabs'

const TABLE_ROWS = 12
const INCOME = 'var(--income)'
const SPENDING = 'var(--chart-2)'
const SAVED_LINE = 'var(--foreground)'

const sum = (xs: string[]) => xs.reduce((acc, x) => add(acc, x), '0')
const savedClass = (v: string) => (isZero(v) ? 'text-muted-foreground' : isNegative(v) ? 'text-expense' : 'text-income')

interface CashFlowBucket {
  bucket: Bucket
  income: string
  spent: string
  saved: string
  transfersIn: string
  transfersOut: string
  running: boolean
}

export function CashFlowReport({ tab }: { tab: ReportTab }) {
  const { t, i18n } = useTranslation()
  const labels = usePeriodLabels()
  const isPhone = useIsMobile()
  const p = tab.params
  const setParams = (patch: Partial<ReportParams>) => useReportTabs.getState().setParams(tab.id, patch)
  const today = new Date()
  // Cash flow has no Compare mode: a tab carrying one still reads its period
  const report = useFlowReport(flowQueryArgs({ ...p, mode: 'overTime' }, today))
  const money = useMoney(report.data?.currencyId)
  const compact = useMemo(() => compactNumber(i18n.language), [i18n.language])
  const [showAll, setShowAll] = useState(false)
  const flowView = !isPhone && p.view === 'flow'

  const header = (
    <div className="flex flex-col gap-2">
      {isPhone ? null : (
        <div>
          <Segmented
            label={t('reports.cashflow.view')}
            value={p.view}
            options={[
              { value: 'bars', label: t('reports.cashflow.view_bars') },
              { value: 'flow', label: t('reports.cashflow.view_flow') },
            ]}
            onChange={(view) => {
              setParams({ view })
              trackEvent(METRICS.REPORTS_CASHFLOW_VIEW, { view })
            }}
          />
        </div>
      )}
      <ReportFilters tab={tab} showStep={!flowView} showGroupBy={flowView} />
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
  const months = data.months
  const flow = cashFlow(data)
  const income = sum(flow.map((m) => m.income))
  const spent = sum(flow.map((m) => m.spent))
  if (months.length === 0 || (isZero(income) && isZero(spent))) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <p className="py-6 text-sm text-muted-foreground">{t('reports.insight.empty')}</p>
      </div>
    )
  }

  const dim = report.isPlaceholderData ? 'opacity-60 transition-opacity' : ''
  const cur = currentMonth(today)
  const roll = (pick: (m: (typeof flow)[number]) => string) => rollUp(months, flow.map(pick), p.step, today)
  const incomeB = roll((m) => m.income)
  const spentB = roll((m) => m.spent)
  const inB = roll((m) => m.transfersIn)
  const outB = roll((m) => m.transfersOut)
  const buckets: CashFlowBucket[] = incomeB.map((b, i) => ({
    bucket: b,
    income: b.total,
    spent: spentB[i].total,
    saved: sub(b.total, spentB[i].total),
    transfersIn: inB[i].total,
    transfersOut: outB[i].total,
    running: b.start <= cur && lastMonthOf(b) >= cur,
  }))
  const running = buckets.find((b) => b.running) ?? null
  const bucketName = (b: CashFlowBucket) => `${labels.bucket(b.bucket.start, p.step)}${b.running ? '*' : ''}`
  const showTransfers = buckets.some((b) => !isZero(b.transfersIn) || !isZero(b.transfersOut))

  const windowRange: MonthRange = [months[0], months[months.length - 1]]
  const insight = cashFlowInsight(flow)
  const overspent = insight.key === 'reports.insight.cashflow_overspent'

  const chartData = buckets.map((b) => ({
    label: bucketName(b),
    title: labels.range([b.bucket.start, lastMonthOf(b.bucket)]),
    income: Number(b.income),
    spentBar: -Number(b.spent),
    savedLine: Number(b.saved),
    spent: b.spent,
    saved: b.saved,
  }))
  const config: ChartConfig = {
    income: { label: t('reports.cashflow.income'), color: INCOME },
    spentBar: { label: t('reports.cashflow.spending'), color: SPENDING },
  }
  const newestFirst = [...buckets].reverse()
  const visible = showAll ? newestFirst : newestFirst.slice(0, TABLE_ROWS)
  const num = 'py-1.5 pl-2 text-right tabular-nums'
  const transferCol = `hidden ${num} text-muted-foreground sm:table-cell`

  return (
    <div className="flex flex-col gap-4">
      {header}
      <section aria-label={t('reports.spending.headline')} className={`flex flex-col gap-1 ${dim}`}>
        <span className="text-sm text-muted-foreground">{labels.range(windowRange)}</span>
        <span className={`text-xl tabular-nums sm:text-2xl ${overspent ? 'text-expense' : ''}`}>
          {t(insight.key, { ...insight.params, amount: money(insight.params.amount ?? '0') })}
        </span>
        <span className="flex flex-wrap gap-x-3 text-sm text-muted-foreground">
          <span>{t('reports.cashflow.income')} <span className="tabular-nums text-foreground">{money(income)}</span></span>
          <span>{t('reports.cashflow.spent')} <span className="tabular-nums text-foreground">{money(spent)}</span></span>
        </span>
      </section>
      {flowView ? (
        <div className={dim}>
          <SankeyFlow report={data} budget={p.budgetId !== null} money={money} />
        </div>
      ) : (
        <figure aria-label={t('reports.cashflow.chart')} className={`flex flex-col gap-2 ${dim}`}>
          <ChartContainer config={config} className="aspect-auto h-60 w-full sm:h-72">
            <ComposedChart data={chartData} margin={{ top: 8, right: 4, left: 0, bottom: 0 }} stackOffset="sign" barCategoryGap="22%">
              <CartesianGrid vertical={false} />
              <XAxis dataKey="label" tickLine={false} axisLine={false} interval="preserveStartEnd" />
              <YAxis width={44} tickLine={false} axisLine={false} tickFormatter={compact} />
              <ReferenceLine y={0} stroke="var(--muted-foreground)" />
              <ChartTooltip
                cursor={{ fillOpacity: 0.4 }}
                content={(props) => (
                  <TooltipBox
                    {...(props as { active?: boolean; payload?: readonly TooltipEntry[] })}
                    money={money}
                    series={[
                      { id: 'income', name: t('reports.cashflow.income'), color: INCOME },
                      { id: 'spent', name: t('reports.cashflow.spent'), color: SPENDING },
                    ]}
                    extra={(d) => [{ label: t('reports.cashflow.saved'), value: signedMoney(String(d.saved ?? '0'), money) }]}
                  />
                )}
              />
              <Bar dataKey="income" stackId="cf" fill={INCOME} radius={[3, 3, 0, 0]} isAnimationActive={false} />
              <Bar dataKey="spentBar" stackId="cf" fill={SPENDING} radius={[3, 3, 0, 0]} isAnimationActive={false} />
              <Line
                dataKey="savedLine"
                type="linear"
                stroke={SAVED_LINE}
                strokeWidth={2}
                strokeDasharray="6 4"
                dot={{ r: 3, fill: SAVED_LINE, strokeWidth: 0 }}
                isAnimationActive={false}
              />
            </ComposedChart>
          </ChartContainer>
          <ul aria-label={t('reports.spending.legend')} className="flex flex-wrap gap-x-3 text-sm text-muted-foreground">
            <li className="flex items-center gap-1.5">
              <span className="size-2.5 shrink-0 rounded-sm" style={{ background: INCOME }} />
              {t('reports.cashflow.income')}
            </li>
            <li className="flex items-center gap-1.5">
              <span className="size-2.5 shrink-0 rounded-sm" style={{ background: SPENDING }} />
              {t('reports.cashflow.spending')}
            </li>
            <li className="flex items-center gap-1.5">
              <span className="h-0 w-4 border-t-2 border-dashed border-foreground" />
              {t('reports.cashflow.net_saved')}
            </li>
          </ul>
        </figure>
      )}
      <div className={`flex flex-col gap-1 ${dim}`}>
        <table className="w-full table-fixed text-sm">
          <thead>
            <tr className="border-b text-xs text-muted-foreground">
              <th className="py-1.5 pr-2 text-left font-normal">{t('reports.cashflow.col_period')}</th>
              <th className="w-24 py-1.5 pl-2 text-right font-normal sm:w-28">{t('reports.cashflow.income')}</th>
              <th className="w-24 py-1.5 pl-2 text-right font-normal sm:w-28">{t('reports.cashflow.spent')}</th>
              <th className="w-24 py-1.5 pl-2 text-right font-normal sm:w-28">{t('reports.cashflow.saved')}</th>
              {showTransfers ? (
                <>
                  <th className="hidden w-28 py-1.5 pl-2 text-right font-normal sm:table-cell">{t('reports.cashflow.transfers_in')}</th>
                  <th className="hidden w-28 py-1.5 pl-2 text-right font-normal sm:table-cell">{t('reports.cashflow.transfers_out')}</th>
                </>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {visible.map((b) => (
              <tr key={b.bucket.start} className="border-b border-border/60">
                <td className="truncate py-1.5 pr-2">{bucketName(b)}</td>
                <td className={num}>{money(b.income)}</td>
                <td className={num}>{money(b.spent)}</td>
                <td className={`${num} ${savedClass(b.saved)}`}>{signedMoney(b.saved, money)}</td>
                {showTransfers ? (
                  <>
                    <td className={transferCol}>{money(b.transfersIn)}</td>
                    <td className={transferCol}>{money(b.transfersOut)}</td>
                  </>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
        {buckets.length > TABLE_ROWS ? (
          <button type="button" className="self-start px-1 py-1 text-sm text-muted-foreground hover:text-foreground" onClick={() => setShowAll((v) => !v)}>
            {showAll ? t('reports.spending.show_less') : t('reports.spending.show_more', { count: buckets.length - TABLE_ROWS })}
          </button>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {running ? `${t('reports.cashflow.running_note', { period: labels.bucket(running.bucket.start, p.step) })} ` : null}
        {flowView ? t('reports.cashflow.flow_note') : null}
      </p>
    </div>
  )
}
