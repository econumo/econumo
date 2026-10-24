import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useCurrencies } from '@/features/currencies/queries'
import { abs, add, div, isNegative, isZero, mul, sub } from '@/lib/decimal'
import { useUiStore } from '@/app/uiStore'
import { isDevEnabled, isFeedbackEnabled } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import { moneyFormat } from '@/lib/money'
import { cashFlow, netWorthInsight, netWorthSeries, spendingInsight } from './reportMath'
import { useFlowReport, useNetWorthReport, useReportBudgets } from './queries'
import { REPORT_QUESTION } from './reportTabs'
import type { ReportKind } from './reportTabs'

const MINE = 'mine'

function useMoney(currencyId: Id | undefined) {
  const { data: currencies } = useCurrencies()
  const currency: CurrencyDto | undefined = currencies?.find((c) => c.id === currencyId)
  // headline figures are whole units: cents add nothing to "how much this year"
  return (amount: string) =>
    moneyFormat(amount, { symbol: currency?.symbol ?? '', fractionDigits: 0 }, { showCurrency: !!currency })
}

function Sparkline({ values, className }: { values: string[]; className: string }) {
  if (values.length < 2) return <div className="h-8" />
  const nums = values.map(Number)
  // the trend is the point, so the line spans its own range; zero joins only when crossed
  const crosses = Math.min(...nums) < 0 && Math.max(...nums) > 0
  const min = Math.min(...nums)
  const max = Math.max(...nums)
  const span = max - min
  const y = (v: number) => (span === 0 ? 16 : 30 - ((v - min) / span) * 28)
  const points = nums.map((v, i) => `${(i / (nums.length - 1)) * 100},${y(v)}`).join(' ')
  const zero = y(0)
  return (
    <svg viewBox="0 0 100 32" preserveAspectRatio="none" className={`h-8 w-full ${className}`} aria-hidden="true">
      {crosses ? <line x1="0" x2="100" y1={zero} y2={zero} className="stroke-border" strokeWidth="1" vectorEffect="non-scaling-stroke" /> : null}
      <polyline points={points} fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

function Card({ question, onClick, disabled, children }: {
  question: string
  onClick?: () => void
  disabled?: boolean
  children: ReactNode
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="flex flex-col gap-1.5 rounded-xl bg-econumo-card p-4 text-left transition-colors hover:bg-econumo-hover disabled:pointer-events-none disabled:opacity-50"
    >
      <span className="text-[15px] leading-snug">{question}</span>
      {children}
    </button>
  )
}

interface FigureProps {
  isPending: boolean
  isError: boolean
  empty: boolean
  amount?: string
  caption?: string
  series?: string[]
  seriesClass?: string
  note?: ReactNode
}

function Figure({ isPending, isError, empty, amount, caption, series, seriesClass = '', note }: FigureProps) {
  const { t } = useTranslation()
  if (isPending) {
    return <div className="mt-1 h-[4.5rem] animate-pulse rounded-md bg-econumo-hover" />
  }
  if (isError) {
    return <span className="text-sm text-muted-foreground">{t('common.app.error')}</span>
  }
  if (empty) {
    return <span className="text-sm text-muted-foreground">{t('reports.insight.empty')}</span>
  }
  return (
    <>
      <span className="flex items-baseline gap-2">
        <span className="text-2xl tabular-nums">{amount}</span>
        <span className="text-sm text-muted-foreground">{caption}</span>
      </span>
      {series ? <Sparkline values={series} className={seriesClass} /> : null}
      {note ? <span className="text-sm text-muted-foreground">{note}</span> : null}
    </>
  )
}

const sum = (xs: string[]) => xs.reduce((acc, x) => add(acc, x), '0')

function SpendingCard({ budgetId, year, onChoose }: { budgetId: Id | null; year: number; onChoose: () => void }) {
  const { t } = useTranslation()
  const report = useFlowReport({ budgetId, from: `${year}-01-01` })
  const money = useMoney(report.data?.currencyId)
  const rows = report.data?.expense ?? []
  const monthly = (report.data?.months ?? []).map((_, i) => sum(rows.map((r) => r.amounts[i] ?? '0')))
  const total = sum(monthly)
  const insight = spendingInsight(rows)
  return (
    <Card question={t(REPORT_QUESTION.spending)} onClick={onChoose}>
      <Figure
        isPending={report.isPending}
        isError={report.isError && !report.data}
        empty={insight.key === 'reports.insight.empty'}
        amount={money(total)}
        caption={t('reports.list.spent_caption', { year })}
        series={monthly}
        seriesClass="text-orange-500 dark:text-orange-400"
        note={t(insight.key, { ...insight.params, amount: money(insight.params.amount ?? '0') })}
      />
    </Card>
  )
}

function CashFlowCard({ budgetId, year, onChoose }: { budgetId: Id | null; year: number; onChoose: () => void }) {
  const { t } = useTranslation()
  const report = useFlowReport({ budgetId, from: `${year}-01-01` })
  const money = useMoney(report.data?.currencyId)
  const months = report.data ? cashFlow(report.data) : []
  const income = sum(months.map((m) => m.income))
  const saved = sum(months.map((m) => m.saved))
  const empty = isZero(income) && isZero(sum(months.map((m) => m.spent)))
  const overspent = isNegative(saved)
  const rate = isZero(income) ? null : Math.round(Number(div(mul(saved, 100), income)))
  return (
    <Card question={t(REPORT_QUESTION.cashflow)} onClick={onChoose}>
      <Figure
        isPending={report.isPending}
        isError={report.isError && !report.data}
        empty={empty}
        amount={money(abs(saved))}
        caption={t(overspent ? 'reports.list.overspent_caption' : 'reports.list.saved_caption', { year })}
        series={months.map((m) => m.saved)}
        seriesClass="text-sky-600 dark:text-sky-400"
        note={!overspent && rate !== null ? t('reports.list.saved_rate', { rate }) : null}
      />
    </Card>
  )
}

const NETWORTH_NOTE: Record<string, string> = {
  'reports.insight.networth_up': 'reports.list.networth_up',
  'reports.insight.networth_down': 'reports.list.networth_down',
  'reports.insight.networth_flat': 'reports.list.networth_flat',
}

function NetWorthCard({ budgetId, year, onChoose }: { budgetId: Id | null; year: number; onChoose: () => void }) {
  const { t } = useTranslation()
  // balances are month-end: last December's closing balance is where this year started
  const report = useNetWorthReport({ budgetId, from: `${year - 1}-12-01` })
  const money = useMoney(report.data?.currencyId)
  const total = report.data ? netWorthSeries(report.data, (a) => ({ id: a.id, name: a.name })).total : []
  const insight = netWorthInsight(total)
  const noteKey = NETWORTH_NOTE[insight.key]
  const change = total.length >= 2 ? sub(total[total.length - 1], total[0]) : '0'
  return (
    <Card question={t(REPORT_QUESTION.networth)} onClick={onChoose}>
      <Figure
        isPending={report.isPending}
        isError={report.isError && !report.data}
        empty={(report.data?.accounts.length ?? 0) === 0}
        amount={money(total[total.length - 1] ?? '0')}
        caption={t('reports.list.networth_caption')}
        series={total}
        seriesClass="text-emerald-600 dark:text-emerald-400"
        note={noteKey ? (
          <span className={isZero(change) ? '' : isNegative(change) ? 'text-orange-700 dark:text-orange-400' : 'text-emerald-700 dark:text-emerald-400'}>
            {t(noteKey, { amount: money(abs(change)) })}
          </span>
        ) : null}
      />
    </Card>
  )
}

export function ReportList({ budgetId, onScope, onChoose }: {
  budgetId: Id | null
  onScope: (budgetId: Id | null) => void
  onChoose: (kind: Exclude<ReportKind, 'list'>) => void
}) {
  const { t } = useTranslation()
  const dev = isDevEnabled()
  const openFeedback = useUiStore((s) => s.openFeedback)
  const { data: budgets } = useReportBudgets()
  const year = new Date().getFullYear()
  // until the list lands a stored budget is trusted; the page repairs one that is gone
  const known = budgetId === null || !budgets || budgets.some((b) => b.id === budgetId)
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm text-muted-foreground">{t('reports.list.choose')}</span>
        {(budgets ?? []).length > 0 ? (
        <Select value={known && budgetId ? budgetId : MINE} onValueChange={(v) => onScope(v === MINE ? null : v)}>
          <SelectTrigger aria-label={t('reports.list.scope')} className="w-auto max-w-64 min-w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="end">
            <SelectItem value={MINE}>{t('reports.list.scope_mine')}</SelectItem>
            {(budgets ?? []).map((b) => (
              <SelectItem key={b.id} value={b.id}>{b.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        ) : null}
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        {/* Spending, Cash flow and the forecast are unfinished: ECONUMO_DEV only */}
        {dev ? <SpendingCard budgetId={known ? budgetId : null} year={year} onChoose={() => onChoose('spending')} /> : null}
        {dev ? <CashFlowCard budgetId={known ? budgetId : null} year={year} onChoose={() => onChoose('cashflow')} /> : null}
        <NetWorthCard budgetId={known ? budgetId : null} year={year} onChoose={() => onChoose('networth')} />
        {dev ? (
          <Card question={t('reports.list.forecast_q')} disabled>
            <span className="text-sm text-muted-foreground">{t('reports.list.coming_later')}</span>
          </Card>
        ) : null}
        {isFeedbackEnabled() ? (
          <Card
            question={t('reports.list.suggest_q')}
            onClick={() => {
              trackEvent(METRICS.REPORTS_SUGGEST)
              openFeedback('feature_request')
            }}
          >
            <span className="text-sm text-muted-foreground">{t('reports.list.suggest_note')}</span>
          </Card>
        ) : null}
      </div>
    </div>
  )
}
