import { useTranslation } from 'react-i18next'
import type { GroupBy } from '@/api/dto/report'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useBudgets } from '@/features/budgets/queries'
import { addMonths, currentMonth } from '@/features/budgets/planMath'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { Step } from './reportMath'
import { periodFrom } from './reportPeriod'
import { useReportTabs } from './reportTabs'
import type { ReportParams, ReportTab } from './reportTabs'

const MINE = 'mine'
const PERIODS: ReportParams['period'][] = ['thisYear', 'last12', 'last3y', 'all', 'custom']
const STEPS: Step[] = ['month', 'quarter', 'year']
const GROUP_BYS: GroupBy[] = ['category', 'payee', 'tag']

export const PILL = 'h-8 w-auto rounded-full px-3 max-md:data-[size=default]:h-9'

export function Segmented<T extends string>({ label, value, options, onChange }: {
  label: string
  value: T
  options: { value: T; label: string }[]
  onChange: (value: T) => void
}) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex shrink-0 rounded-full border p-0.5">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          onClick={() => value !== o.value && onChange(o.value)}
          className={`rounded-full px-3 py-1 text-sm transition-colors ${value === o.value ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

const toMonthInput = (m: string | undefined) => (m ? m.slice(0, 7) : '')
const fromMonthInput = (v: string) => (/^\d{4}-\d{2}$/.test(v) ? `${v}-01` : null)

export function ReportFilters({ tab, showStep = true, showGroupBy = false, showPeriod = true }: {
  tab: ReportTab
  showStep?: boolean
  showGroupBy?: boolean
  showPeriod?: boolean
}) {
  const { t } = useTranslation()
  const { data: budgets } = useBudgets()
  const setParams = (patch: Partial<ReportParams>) => useReportTabs.getState().setParams(tab.id, patch)
  const p = tab.params
  const thisMonth = currentMonth()
  const known = p.budgetId === null || !budgets || budgets.some((b) => b.id === p.budgetId)
  const customTo = p.to ?? thisMonth

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={known && p.budgetId ? p.budgetId : MINE}
          onValueChange={(v) => {
            const budgetId = v === MINE ? null : v
            // a selected row belongs to the scope it was picked in
            setParams({ budgetId, selectedId: null })
            trackEvent(METRICS.REPORTS_CHANGE_SCOPE, { scope: budgetId ? 'budget' : 'mine' })
          }}
        >
          <SelectTrigger aria-label={t('reports.list.scope')} className={`${PILL} max-w-56`}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={MINE}>{t('reports.list.scope_mine')}</SelectItem>
            {(budgets ?? []).map((b) => (
              <SelectItem key={b.id} value={b.id}>{b.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        {showPeriod ? (
          <Select
            value={p.period}
            onValueChange={(v) => {
              const period = v as ReportParams['period']
              if (period === 'custom') {
                // the custom range opens on what was on screen
                setParams({ period, from: periodFrom(p) ?? addMonths(thisMonth, -35), to: undefined })
              } else {
                setParams({ period })
              }
              trackEvent(METRICS.REPORTS_CHANGE_PERIOD, { period })
            }}
          >
            <SelectTrigger aria-label={t('reports.filters.period')} className={PILL}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PERIODS.map((period) => (
                <SelectItem key={period} value={period}>{t(`reports.filters.period_${period}`)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
        {showGroupBy && p.budgetId === null ? (
          <Select
            value={p.groupBy}
            onValueChange={(v) => {
              setParams({ groupBy: v as GroupBy, selectedId: null })
              trackEvent(METRICS.REPORTS_CHANGE_GROUP_BY, { groupBy: v })
            }}
          >
            <SelectTrigger aria-label={t('reports.filters.group_by')} className={PILL}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {GROUP_BYS.map((g) => (
                <SelectItem key={g} value={g}>{t(`reports.filters.group_${g}`)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
        {showStep ? (
          <Segmented
            label={t('reports.filters.step')}
            value={p.step}
            options={STEPS.map((s) => ({ value: s, label: t(`reports.filters.step_${s}`) }))}
            onChange={(step) => {
              setParams({ step })
              trackEvent(METRICS.REPORTS_CHANGE_STEP, { step })
            }}
          />
        ) : null}
      </div>
      {showPeriod && p.period === 'custom' ? (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <label className="flex items-center gap-1.5">
            <span className="text-muted-foreground">{t('reports.filters.from')}</span>
            <input
              type="month"
              className="h-8 rounded-full border bg-transparent px-3"
              value={toMonthInput(p.from)}
              max={toMonthInput(customTo)}
              onChange={(e) => {
                const from = fromMonthInput(e.target.value)
                if (from && from <= customTo) setParams({ from })
              }}
            />
          </label>
          <label className="flex items-center gap-1.5">
            <span className="text-muted-foreground">{t('reports.filters.to')}</span>
            <input
              type="month"
              className="h-8 rounded-full border bg-transparent px-3"
              value={toMonthInput(customTo)}
              min={toMonthInput(p.from)}
              max={toMonthInput(thisMonth)}
              onChange={(e) => {
                const to = fromMonthInput(e.target.value)
                if (to && (!p.from || to >= p.from) && to <= thisMonth) setParams({ to: to === thisMonth ? undefined : to })
              }}
            />
          </label>
        </div>
      ) : null}
    </div>
  )
}
