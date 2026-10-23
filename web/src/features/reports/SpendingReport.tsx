import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, CartesianGrid, ComposedChart, Line, XAxis, YAxis } from 'recharts'
import { ChevronRight, X } from 'lucide-react'
import type { ReportRowDto } from '@/api/dto/report'
import type { Id } from '@/api/types'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import { CHART_OTHER, CHART_PALETTE } from '@/components/ui/chartPalette'
import type { ChartConfig } from '@/components/ui/chart'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useCurrencies } from '@/features/currencies/queries'
import { abs, add, isNegative, isZero, sub } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import { METRICS, trackEvent } from '@/lib/metrics'
import { changeRows, compareYears, rollUp, spendingInsight, topGroups } from './reportMath'
import type { Bucket, ChangeRow } from './reportMath'
import { OTHER_ID, seriesColors } from './reportColors'
import { PILL, ReportFilters, Segmented } from './ReportFilters'
import {
  bucketLabel,
  compareYearsOf,
  comparisonRanges,
  flowQueryArgs,
  lastMonthOf,
  rangeLabel,
  shortMonth,
} from './reportPeriod'
import type { MonthRange } from './reportPeriod'
import { useFlowReport } from './queries'
import { asRow, findRow, rowDisplayName } from './reportRows'
import { ReportTransactionsDialog } from './ReportTransactionsDialog'
import type { DrillRow } from './reportRows'
import { useReportTabs } from './reportTabs'
import type { ReportParams, ReportTab } from './reportTabs'

const TOP = 5
const TABLE_ROWS = 10
const PROJECTION = '__projected'
const PLANNED = '__planned'

const sum = (xs: string[]) => xs.reduce((acc, x) => add(acc, x), '0')

function sliceRows(rows: ReportRowDto[], months: string[], [from, to]: MonthRange): ReportRowDto[] {
  return rows.map((r) => ({ ...r, amounts: r.amounts.filter((_, i) => months[i] >= from && months[i] <= to) }))
}

function useMoney(currencyId: Id | undefined) {
  const { data: currencies } = useCurrencies()
  const currency = currencies?.find((c) => c.id === currencyId)
  // report figures are whole units: cents add nothing to "how much per year"
  return (amount: string) => moneyFormat(amount, { symbol: currency?.symbol ?? '', fractionDigits: 0 }, { showCurrency: !!currency })
}

function compactNumber(lang: string) {
  const fmt = new Intl.NumberFormat(lang, { notation: 'compact', maximumFractionDigits: 1 })
  return (v: number) => fmt.format(v)
}

function percent(pct: number | null): string | null {
  if (pct === null) return null
  const r = Math.round(pct)
  return `${r > 0 ? '+' : r < 0 ? '−' : ''}${Math.abs(r)}%`
}

function signedMoney(diff: string, money: (a: string) => string): string {
  if (isZero(diff)) return money('0')
  return `${isNegative(diff) ? '−' : '+'}${money(abs(diff))}`
}

// more spending reads as worse, more income as better; the sign carries the
// meaning, the colour only repeats it
function changeClass(diff: string, side: ReportParams['side']): string {
  if (isZero(diff)) return 'text-muted-foreground'
  const worse = side === 'expense' ? !isNegative(diff) : isNegative(diff)
  return worse ? 'text-orange-700 dark:text-orange-400' : 'text-emerald-700 dark:text-emerald-400'
}

function Change({ diff, pct, side, money, showMoney = true }: {
  diff: string
  pct: number | null
  side: ReportParams['side']
  money: (a: string) => string
  showMoney?: boolean
}) {
  const p = percent(pct)
  return (
    <span className={changeClass(diff, side)}>
      {showMoney ? signedMoney(diff, money) : null}
      {showMoney && p ? ' · ' : null}
      {p ?? (showMoney ? null : signedMoney(diff, money))}
    </span>
  )
}

interface Series {
  id: Id
  name: string
  amounts: string[]
}

function Legend({ series, colors, selectedId, onSelect }: {
  series: Series[]
  colors: Record<Id, string>
  selectedId: Id | null
  onSelect: (id: Id) => void
}) {
  const { t } = useTranslation()
  return (
    <ul aria-label={t('reports.spending.legend')} className="flex flex-wrap gap-x-3 gap-y-1 text-sm">
      {series.map((s) => (
        <li key={s.id}>
          {s.id === OTHER_ID ? (
            <span className="flex items-center gap-1.5 text-muted-foreground">
              <span className="size-2.5 shrink-0 rounded-sm" style={{ background: colors[s.id] }} />
              {s.name}
            </span>
          ) : (
            <button
              type="button"
              aria-pressed={selectedId === s.id}
              onClick={() => onSelect(s.id)}
              className="flex items-center gap-1.5 rounded px-0.5 text-muted-foreground hover:text-foreground"
            >
              <span className="size-2.5 shrink-0 rounded-sm" style={{ background: colors[s.id] }} />
              {s.name}
            </button>
          )}
        </li>
      ))}
    </ul>
  )
}

interface TooltipEntry {
  dataKey?: string | number
  value?: number | string | null
  payload?: Record<string, unknown>
}

function TooltipBox({ active, payload, series, money, extra }: {
  active?: boolean
  payload?: readonly TooltipEntry[]
  series: { id: Id; name: string; color: string }[]
  money: (a: string) => string
  extra?: (data: Record<string, unknown>) => { label: string; value: string }[]
}) {
  if (!active || !payload?.length) return null
  const data = payload[0].payload ?? {}
  const shown = series.filter((s) => data[s.id] !== null && data[s.id] !== undefined)
  return (
    <div className="grid min-w-36 gap-1 rounded-lg border bg-background px-2.5 py-1.5 text-xs shadow-xl">
      <span>{String(data.title ?? data.label ?? '')}</span>
      {shown.map((s) => (
        <span key={s.id} className="flex items-center gap-2">
          <span className="size-2.5 shrink-0 rounded-sm" style={{ background: s.color }} />
          <span className="flex-1 text-muted-foreground">{s.name}</span>
          <span className="tabular-nums">{money(String(data[s.id]))}</span>
        </span>
      ))}
      {(extra?.(data) ?? []).map((e) => (
        <span key={e.label} className="flex items-center gap-2">
          <span className="flex-1 text-muted-foreground">{e.label}</span>
          <span className="tabular-nums">{e.value}</span>
        </span>
      ))}
    </div>
  )
}

function ChangeTable({ rows, months, a, b, aLabel, bLabel, side, selectedId, colors, money, onSelect }: {
  rows: ReportRowDto[]
  months: string[]
  a: MonthRange | null
  b: MonthRange
  aLabel: string
  bLabel: string
  side: ReportParams['side']
  selectedId: Id | null
  colors: Record<Id, string>
  money: (a: string) => string
  onSelect: (id: Id) => void
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<Set<Id>>(new Set())
  const [showAll, setShowAll] = useState(false)
  const list = changeRows(rows, months, a ?? b, b)
  const byId = new Map(rows.map((r) => [r.id, r]))
  const visible = showAll ? list : list.slice(0, TABLE_ROWS)
  const comparing = a !== null

  const line = (r: ChangeRow, depth: number, expandable: boolean) => {
    const selected = selectedId === r.id
    const open = expanded.has(r.id)
    const name = rowDisplayName(r, t)
    return (
      <tr
        key={`${depth}-${r.id}`}
        aria-selected={selected}
        onClick={() => onSelect(r.id)}
        className={`cursor-pointer border-b border-border/60 ${selected ? 'bg-econumo-hover' : 'hover:bg-econumo-hover/60'}`}
      >
        <td className="py-1.5 pr-2">
          <span className={`flex min-w-0 items-center gap-1.5 ${depth ? 'pl-5' : ''}`}>
            {expandable ? (
              <button
                type="button"
                aria-expanded={open}
                aria-label={t(open ? 'reports.spending.collapse' : 'reports.spending.expand', { name })}
                className="-ml-1 grid size-5 shrink-0 place-items-center rounded text-muted-foreground hover:bg-accent"
                onClick={(e) => {
                  e.stopPropagation()
                  setExpanded((s) => {
                    const next = new Set(s)
                    if (next.has(r.id)) next.delete(r.id)
                    else next.add(r.id)
                    return next
                  })
                }}
              >
                <ChevronRight className={`size-3.5 transition-transform ${open ? 'rotate-90' : ''}`} />
              </button>
            ) : depth === 0 ? <span className="w-4 shrink-0" /> : null}
            <span className="size-2.5 shrink-0 rounded-sm" style={colors[r.id] ? { background: colors[r.id] } : undefined} />
            <button type="button" className="truncate text-left" aria-pressed={selected}>{name}</button>
          </span>
        </td>
        {comparing ? <td className="hidden py-1.5 pl-2 text-right tabular-nums text-muted-foreground sm:table-cell">{money(r.a)}</td> : null}
        <td className="py-1.5 pl-2 text-right tabular-nums">{money(r.b)}</td>
        {comparing ? (
          <td className="py-1.5 pl-2 text-right tabular-nums whitespace-nowrap">
            <span className="hidden sm:inline"><Change diff={r.diff} pct={r.pct} side={side} money={money} /></span>
            <span className="sm:hidden"><Change diff={r.diff} pct={r.pct} side={side} money={money} showMoney={false} /></span>
          </td>
        ) : null}
        <td className="hidden py-1.5 pl-2 text-right tabular-nums text-muted-foreground sm:table-cell">{money(r.monthlyAvg)}</td>
      </tr>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      <table className="w-full table-fixed text-sm">
        <thead>
          <tr className="border-b text-xs text-muted-foreground">
            <th className="py-1.5 pr-2 text-left font-normal">{t('reports.spending.col_name')}</th>
            {comparing ? <th className="hidden w-24 py-1.5 pl-2 text-right font-normal sm:table-cell">{aLabel}</th> : null}
            <th className="w-24 py-1.5 pl-2 text-right font-normal">{bLabel}</th>
            {comparing ? <th className="w-20 py-1.5 pl-2 text-right font-normal sm:w-40">{t('reports.spending.col_change')}</th> : null}
            <th className="hidden w-24 py-1.5 pl-2 text-right font-normal sm:table-cell">{t('reports.spending.col_avg')}</th>
          </tr>
        </thead>
        <tbody>
          {visible.flatMap((r) => {
            const source = byId.get(r.id)
            const children = source && source.children.length > 0 ? source.children.map(asRow) : []
            const out = [line(r, 0, children.length > 0)]
            if (children.length > 0 && expanded.has(r.id)) {
              for (const c of changeRows(children, months, a ?? b, b)) out.push(line(c, 1, false))
            }
            return out
          })}
        </tbody>
      </table>
      {list.length > TABLE_ROWS ? (
        <button type="button" className="self-start px-1 py-1 text-sm text-muted-foreground hover:text-foreground" onClick={() => setShowAll((v) => !v)}>
          {showAll ? t('reports.spending.show_less') : t('reports.spending.show_more', { count: list.length - TABLE_ROWS })}
        </button>
      ) : null}
    </div>
  )
}

interface Drill {
  row: DrillRow
  from: string
  to: string
}

export function SpendingReport({ tab }: { tab: ReportTab }) {
  const { t, i18n } = useTranslation()
  const p = tab.params
  const setParams = (patch: Partial<ReportParams>) => useReportTabs.getState().setParams(tab.id, patch)
  const today = new Date()
  const report = useFlowReport(flowQueryArgs(p, today))
  const money = useMoney(report.data?.currencyId)
  const compact = useMemo(() => compactNumber(i18n.language), [i18n.language])
  const [drill, setDrill] = useState<Drill | null>(null)

  const months = report.data?.months ?? []
  const rows = useMemo(() => report.data?.[p.side] ?? [], [report.data, p.side])
  const found = findRow(rows, p.selectedId)
  const selected = found?.row ?? null
  const rowName = (r: { id: Id; name: string; kind: string }) => rowDisplayName(r, t)

  const select = (id: Id) => {
    const next = p.selectedId === id ? null : id
    setParams({ selectedId: next })
    trackEvent(METRICS.REPORTS_SELECT_ROW, { selected: next !== null })
  }

  const openDrill = (from: string, to: string) => {
    if (!found) return
    setDrill({
      row: {
        id: found.row.id,
        kind: found.row.kind,
        name: rowName(found.row),
        side: p.side,
        parent: found.parent ? { id: found.parent.id, kind: found.parent.kind } : undefined,
      },
      from,
      to,
    })
    trackEvent(METRICS.REPORTS_DRILL_DOWN, { scope: p.budgetId ? 'budget' : 'mine', mode: p.mode })
  }

  const top = topGroups(rows, TOP)
  const totalAmounts = months.map((_, i) => sum(rows.map((r) => r.amounts[i] ?? '0')))
  const series: Series[] = selected
    ? [{ id: selected.id, name: rowName(selected), amounts: selected.amounts }]
    : [
        ...top.ids.map((id) => {
          const r = rows.find((x) => x.id === id)!
          return { id, name: rowName(r), amounts: r.amounts }
        }),
        ...(top.otherIds.length > 0
          ? [{
              id: OTHER_ID,
              name: t('reports.spending.other'),
              amounts: months.map((_, i) => sum(rows.filter((r) => top.otherIds.includes(r.id)).map((r) => r.amounts[i] ?? '0'))),
            }]
          : []),
      ]
  // colours are resolved over the top set even when one row is shown, so a
  // row keeps the colour it had in the stacked chart
  const colors = seriesColors(selected && !top.ids.includes(selected.id) ? [selected.id] : [...top.ids, OTHER_ID])

  const header = (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Segmented
          label={t('reports.spending.mode')}
          value={p.mode}
          options={[
            { value: 'overTime', label: t('reports.spending.mode_overTime') },
            { value: 'compare', label: t('reports.spending.mode_compare') },
          ]}
          onChange={(mode) => {
            setParams({ mode })
            trackEvent(METRICS.REPORTS_SPENDING_MODE, { mode })
          }}
        />
        <Segmented
          label={t('reports.spending.side')}
          value={p.side}
          options={[
            { value: 'expense', label: t('reports.spending.side_expense') },
            { value: 'income', label: t('reports.spending.side_income') },
          ]}
          onChange={(side) => {
            // a row belongs to one side
            setParams({ side, selectedId: null })
            trackEvent(METRICS.REPORTS_CHANGE_SIDE, { side })
          }}
        />
      </div>
      <ReportFilters tab={tab} showGroupBy showStep={p.mode === 'overTime'} showPeriod={p.mode === 'overTime'} />
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

  const selectedChip = selected ? (
    <button
      type="button"
      onClick={() => select(selected.id)}
      className="flex max-w-full items-center gap-1 self-start rounded-full border px-2.5 py-0.5 text-sm"
      aria-label={t('reports.spending.show_all')}
    >
      <span className="size-2.5 shrink-0 rounded-sm" style={{ background: colors[selected.id] }} />
      <span className="truncate">{rowName(selected)}</span>
      <X className="size-3.5 shrink-0 text-muted-foreground" />
    </button>
  ) : null

  const spentKey = p.side === 'expense' ? 'reports.spending.spent_in' : 'reports.spending.earned_in'
  const empty = rows.length === 0 || totalAmounts.every((a) => isZero(a))
  const dim = report.isPlaceholderData ? 'opacity-60 transition-opacity' : ''

  if (empty) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <p className="py-6 text-sm text-muted-foreground">{t('reports.insight.empty')}</p>
      </div>
    )
  }

  if (p.mode === 'compare') {
    const [earlier, later] = compareYearsOf(p, today)
    const amounts = selected ? selected.amounts : totalAmounts
    const cy = compareYears(months, amounts, earlier, later, today)
    const through = String(cy.throughMonth).padStart(2, '0')
    const aRange: MonthRange = [`${earlier}-01-01`, `${earlier}-${through}-01`]
    const bRange: MonthRange = [`${later}-01-01`, `${later}-${through}-01`]
    const diff = sub(cy.laterTotal, cy.earlierTotal)
    const pct = isZero(cy.earlierTotal) ? null : (Number(diff) * 100) / Math.abs(Number(cy.earlierTotal))
    const laterColor = selected ? colors[selected.id] : CHART_PALETTE[0]
    const data = cy.slots.map((s) => ({
      label: shortMonth(s.month, i18n.language),
      title: shortMonth(s.month, i18n.language),
      month: s.month,
      earlier: s.earlier === null ? null : Number(s.earlier),
      later: s.later === null ? null : Number(s.later),
    }))
    const years = Array.from({ length: 10 }, (_, i) => today.getFullYear() - i)
    const setYears = (pair: [number, number]) => {
      setParams({ compare: pair })
      trackEvent(METRICS.REPORTS_CHANGE_PERIOD, { period: 'compare' })
    }
    const period = cy.throughMonth === 12 ? null : rangeLabel(['2000-01-01', `2000-${through}-01`], i18n.language).replace(' 2000', '')
    const config: ChartConfig = {
      earlier: { label: String(earlier), color: CHART_OTHER },
      later: { label: String(later), color: laterColor },
    }
    const onBar = (year: number) => (_: unknown, index: number) => {
      const slot = cy.slots[index]
      if (!slot) return
      const m = `${year}-${String(slot.month).padStart(2, '0')}-01`
      openDrill(m, m)
    }
    return (
      <div className="flex flex-col gap-4">
        {header}
        <div className="flex flex-wrap items-center gap-2">
          <Select value={String(later)} onValueChange={(v) => setYears([earlier, Number(v)])}>
            <SelectTrigger aria-label={t('reports.spending.later_year')} className={PILL}><SelectValue /></SelectTrigger>
            <SelectContent>
              {years.filter((y) => y > earlier).map((y) => <SelectItem key={y} value={String(y)}>{y}</SelectItem>)}
            </SelectContent>
          </Select>
          <span className="text-sm text-muted-foreground">{t('reports.spending.versus')}</span>
          <Select value={String(earlier)} onValueChange={(v) => setYears([Number(v), later])}>
            <SelectTrigger aria-label={t('reports.spending.earlier_year')} className={PILL}><SelectValue /></SelectTrigger>
            <SelectContent>
              {years.filter((y) => y < later).map((y) => <SelectItem key={y} value={String(y)}>{y}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <section className={`flex flex-col gap-1 ${dim}`}>
          {selectedChip}
          <span className="text-sm text-muted-foreground">
            {t(spentKey, { period: period ? `${period} ${later}` : String(later) })}
          </span>
          <span className="flex flex-wrap items-baseline gap-x-2">
            <span className="text-2xl tabular-nums">{money(cy.laterTotal)}</span>
            <span className="text-sm">
              <Change diff={diff} pct={pct} side={p.side} money={money} />
              <span className="text-muted-foreground"> {t('reports.spending.vs', { period: period ? `${period} ${earlier}` : String(earlier) })}</span>
            </span>
          </span>
        </section>
        <figure className={`flex flex-col gap-2 ${dim}`}>
          <ChartContainer config={config} className="aspect-auto h-60 w-full sm:h-72" aria-label={t('reports.spending.chart')}>
            <ComposedChart data={data} margin={{ top: 8, right: 4, left: 0, bottom: 0 }} barGap={2} barCategoryGap="20%">
              <CartesianGrid vertical={false} />
              <XAxis dataKey="label" tickLine={false} axisLine={false} interval="preserveStartEnd" />
              <YAxis width={44} tickLine={false} axisLine={false} tickFormatter={compact} />
              <ChartTooltip
                cursor={{ fillOpacity: 0.4 }}
                content={(props) => (
                  <TooltipBox
                    {...(props as { active?: boolean; payload?: readonly TooltipEntry[] })}
                    money={money}
                    series={[
                      { id: 'earlier', name: String(earlier), color: CHART_OTHER },
                      { id: 'later', name: String(later), color: laterColor },
                    ]}
                  />
                )}
              />
              <Bar dataKey="earlier" fill={CHART_OTHER} radius={[3, 3, 0, 0]} onClick={selected ? onBar(earlier) : undefined} className={selected ? 'cursor-pointer' : ''} />
              <Bar dataKey="later" fill={laterColor} radius={[3, 3, 0, 0]} onClick={selected ? onBar(later) : undefined} className={selected ? 'cursor-pointer' : ''} />
            </ComposedChart>
          </ChartContainer>
          <ul aria-label={t('reports.spending.legend')} className="flex flex-wrap gap-x-3 text-sm text-muted-foreground">
            {[[earlier, CHART_OTHER], [later, laterColor]].map(([year, color]) => (
              <li key={year} className="flex items-center gap-1.5">
                <span className="size-2.5 shrink-0 rounded-sm" style={{ background: color }} />
                {year}
              </li>
            ))}
          </ul>
        </figure>
        <ChangeTable
          rows={rows}
          months={months}
          a={aRange}
          b={bRange}
          aLabel={rangeLabel(aRange, i18n.language)}
          bLabel={rangeLabel(bRange, i18n.language)}
          side={p.side}
          selectedId={selected?.id ?? null}
          colors={colors}
          money={money}
          onSelect={select}
        />
        <p className="text-xs text-muted-foreground">
          {cy.throughMonth < 12 ? `${t('reports.spending.compare_note', { period: period ?? '' })} ` : null}
          {t(selected ? 'reports.spending.hint_drill' : 'reports.spending.hint_select')}
        </p>
        {drill ? <ReportTransactionsDialog scope={{ budgetId: p.budgetId }} {...drill} onClose={() => setDrill(null)} /> : null}
      </div>
    )
  }

  const bucketsBy = (amounts: string[]) => rollUp(months, amounts, p.step, today)
  const totalBuckets = bucketsBy(totalAmounts)
  const seriesBuckets = series.map((s) => bucketsBy(s.amounts))
  const chartBuckets: Bucket[] = selected ? seriesBuckets[0] : totalBuckets
  const projected = chartBuckets.find((b) => b.projected !== b.total) ?? null
  const showPlanned = p.budgetId !== null && selected !== null && p.step === 'month' && found?.parent === null && selected.planned.length === months.length
  const data = chartBuckets.map((b, i) => {
    const label = bucketLabel(b.start, p.step, i18n.language)
    const point: Record<string, unknown> = {
      label: b.projected !== b.total ? `${label}*` : label,
      title: rangeLabel([b.start, lastMonthOf(b)], i18n.language),
      start: b.start,
      end: lastMonthOf(b),
      [PROJECTION]: b.projected !== b.total ? Number(b.projected) : null,
    }
    series.forEach((s, k) => {
      point[s.id] = Number(seriesBuckets[k][i]?.total ?? 0)
    })
    if (showPlanned) {
      const v = selected.planned[i]
      point[PLANNED] = v === '' || v === undefined ? null : Number(v)
    }
    return point
  })
  const config: ChartConfig = Object.fromEntries(series.map((s) => [s.id, { label: s.name, color: colors[s.id] }]))

  const ranges = comparisonRanges(totalBuckets, p.step, today)
  const lastBucket = totalBuckets[totalBuckets.length - 1]
  const bRange: MonthRange = ranges?.b ?? [months[0], months[months.length - 1]]
  const aRange: MonthRange | null = ranges?.a ?? null
  const inRange = (amounts: string[], [from, to]: MonthRange) => sum(amounts.filter((_, i) => months[i] >= from && months[i] <= to))
  const headlineAmounts = selected ? selected.amounts : totalAmounts
  const bTotal = inRange(headlineAmounts, bRange)
  const aTotal = aRange ? inRange(headlineAmounts, aRange) : null
  const diff = aTotal !== null ? sub(bTotal, aTotal) : null
  const pct = aTotal !== null && diff !== null && !isZero(aTotal) ? (Number(diff) * 100) / Math.abs(Number(aTotal)) : null
  const insight = selected ? null : spendingInsight(sliceRows(rows, months, bRange))
  const insightRow = insight ? rows.find((r) => r.name === insight.params.name) : undefined
  const insightName = insightRow ? rowName(insightRow) : (insight?.params.name ?? '')
  const paceBucket = selected ? seriesBuckets[0][seriesBuckets[0].length - 1] : lastBucket
  const onPace = paceBucket && paceBucket.projected !== paceBucket.total && bRange[0] <= paceBucket.start && bRange[1] >= lastMonthOf(paceBucket)
    ? paceBucket
    : null

  const onSegment = (id: Id) => (_: unknown, index: number) => {
    const bucket = chartBuckets[index]
    if (!bucket) return
    if (selected) {
      openDrill(bucket.start < months[0] ? months[0] : bucket.start, lastMonthOf(bucket))
    } else if (id !== OTHER_ID) {
      select(id)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {header}
      <section className={`flex flex-col gap-1 ${dim}`}>
        {selectedChip}
        <span className="text-sm text-muted-foreground">{t(spentKey, { period: rangeLabel(bRange, i18n.language) })}</span>
        <span className="flex flex-wrap items-baseline gap-x-2">
          <span className="text-2xl tabular-nums">{money(bTotal)}</span>
          {diff !== null && aRange ? (
            <span className="text-sm">
              <Change diff={diff} pct={pct} side={p.side} money={money} />
              <span className="text-muted-foreground"> {t('reports.spending.vs', { period: rangeLabel(aRange, i18n.language) })}</span>
            </span>
          ) : null}
        </span>
        {onPace ? (
          <span className="text-sm text-muted-foreground">
            {t('reports.spending.on_pace', { amount: money(onPace.projected), period: bucketLabel(onPace.start, p.step, i18n.language) })}
          </span>
        ) : null}
        {insight && insight.key !== 'reports.insight.empty' ? (
          <span className="text-sm text-muted-foreground">
            {t(insight.key, { ...insight.params, name: insightName, amount: money(insight.params.amount ?? '0') })}
          </span>
        ) : null}
      </section>
      <figure className={`flex flex-col gap-2 ${dim}`}>
        <ChartContainer config={config} className="aspect-auto h-60 w-full sm:h-72" aria-label={t('reports.spending.chart')}>
          <ComposedChart data={data} margin={{ top: 8, right: 4, left: 0, bottom: 0 }} stackOffset="sign" barCategoryGap="22%">
            <CartesianGrid vertical={false} />
            <XAxis dataKey="label" tickLine={false} axisLine={false} interval="preserveStartEnd" />
            <XAxis xAxisId="projection" dataKey="label" hide />
            <YAxis width={44} tickLine={false} axisLine={false} tickFormatter={compact} />
            <ChartTooltip
              cursor={{ fillOpacity: 0.4 }}
              content={(props) => (
                <TooltipBox
                  {...(props as { active?: boolean; payload?: readonly TooltipEntry[] })}
                  money={money}
                  series={series.map((s) => ({ id: s.id, name: s.name, color: colors[s.id] }))}
                  extra={(d) => [
                    ...(series.length > 1
                      ? [{ label: t('reports.spending.total'), value: money(String(series.reduce((acc, s) => acc + Number(d[s.id] ?? 0), 0))) }]
                      : []),
                    ...(d[PROJECTION] !== null && d[PROJECTION] !== undefined
                      ? [{ label: t('reports.spending.projected'), value: money(String(d[PROJECTION])) }]
                      : []),
                    ...(d[PLANNED] !== null && d[PLANNED] !== undefined
                      ? [{ label: t('reports.spending.planned'), value: money(String(d[PLANNED])) }]
                      : []),
                  ]}
                />
              )}
            />
            {projected ? (
              <Bar
                xAxisId="projection"
                dataKey={PROJECTION}
                fill="transparent"
                stroke="var(--muted-foreground)"
                strokeWidth={1.5}
                strokeDasharray="4 3"
                radius={[3, 3, 0, 0]}
                isAnimationActive={false}
                legendType="none"
              />
            ) : null}
            {series.map((s) => (
              <Bar
                key={s.id}
                dataKey={s.id}
                stackId="flow"
                fill={colors[s.id]}
                stroke="var(--background)"
                strokeWidth={1}
                className={s.id === OTHER_ID && !selected ? '' : 'cursor-pointer'}
                onClick={onSegment(s.id)}
                isAnimationActive={false}
              />
            ))}
            {showPlanned ? (
              <Line
                dataKey={PLANNED}
                type="linear"
                stroke="var(--foreground)"
                strokeWidth={2}
                strokeDasharray="6 4"
                dot={{ r: 3, fill: 'var(--foreground)', strokeWidth: 0 }}
                connectNulls={false}
                isAnimationActive={false}
              />
            ) : null}
          </ComposedChart>
        </ChartContainer>
        <div className="flex flex-col gap-1">
          <Legend series={series} colors={colors} selectedId={selected?.id ?? null} onSelect={select} />
          {showPlanned ? (
            <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <span className="h-0 w-4 border-t-2 border-dashed border-foreground" />
              {t('reports.spending.planned')}
            </span>
          ) : null}
        </div>
      </figure>
      <ChangeTable
        key={`${bRange[0]}|${aRange?.[0] ?? ''}`}
        rows={rows}
        months={months}
        a={aRange}
        b={bRange}
        aLabel={aRange ? rangeLabel(aRange, i18n.language) : ''}
        bLabel={rangeLabel(bRange, i18n.language)}
        side={p.side}
        selectedId={selected?.id ?? null}
        colors={colors}
        money={money}
        onSelect={select}
      />
      <p className="text-xs text-muted-foreground">
        {projected
          ? `${t('reports.spending.projected_note', { period: bucketLabel(projected.start, p.step, i18n.language) })} `
          : null}
        {t(selected ? 'reports.spending.hint_drill' : 'reports.spending.hint_select')}
      </p>
      {drill ? <ReportTransactionsDialog scope={{ budgetId: p.budgetId }} {...drill} onClose={() => setDrill(null)} /> : null}
    </div>
  )
}
