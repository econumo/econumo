import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, CartesianGrid, ComposedChart, Line, XAxis, YAxis } from 'recharts'
import { ChevronRight, List, X } from 'lucide-react'
import type { ReportRowDto } from '@/api/dto/report'
import type { Id } from '@/api/types'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import { CHART_OTHER, CHART_PALETTE } from '@/components/ui/chartPalette'
import type { ChartConfig } from '@/components/ui/chart'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { abs, add, div, isNegative, isZero, mul, sub } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { bucketChanges, changeRows, compareYears, rollUp, spendingInsight, topGroups } from './reportMath'
import type { Bucket, ChangeRow } from './reportMath'
import { OTHER_ID, seriesColors } from './reportColors'
import { PILL, ReportFilters, Segmented } from './ReportFilters'
import { compareYearsOf, flowQueryArgs, lastMonthOf, periodRange, previousRange } from './reportPeriod'
import { usePeriodLabels } from './usePeriodLabels'
import { compactNumber, percent, signedMoney, useMoney } from './reportFormat'
import { TooltipBox } from './ReportTooltip'
import type { TooltipEntry } from './ReportTooltip'
import type { MonthRange } from './reportPeriod'
import { useFlowReport } from './queries'
import { asRow, findRow, rowDisplayName } from './reportRows'
import { ReportTransactionsDialog } from './ReportTransactionsDialog'
import type { DrillRow } from './reportRows'
import type { Expected } from './ReportTransactionsDialog'
import { ReportUpdatedAt } from './ReportUpdatedAt'
import { useReportTabs } from './reportTabs'
import type { ReportParams, ReportTab } from './reportTabs'

const TOP = 5
const TABLE_ROWS = 10
const PROJECTION = '__projected'
const PLANNED = '__planned'

const sum = (xs: string[]) => xs.reduce((acc, x) => add(acc, x), '0')

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

function ChangeTable({ list, childrenOf, aLabel, bLabel, side, selectedId, colors, money, onSelect }: {
  list: ChangeRow[]
  /** an envelope's (or tag's) child rows, compared the same way; [] for none */
  childrenOf: (id: Id) => ChangeRow[]
  /** null = one period only, no comparison columns */
  aLabel: string | null
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
  const visible = showAll ? list : list.slice(0, TABLE_ROWS)
  const comparing = aLabel !== null

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
            const children = childrenOf(r.id)
            const out = [line(r, 0, children.length > 0)]
            if (children.length > 0 && expanded.has(r.id)) {
              for (const c of children) out.push(line(c, 1, false))
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
  expected?: Expected
}

function changeOf(later: string, earlier: string): { diff: string; pct: number | null } {
  const diff = sub(later, earlier)
  return { diff, pct: isZero(earlier) ? null : Number(div(mul(diff, 100), abs(earlier))) }
}

export function SpendingReport({ tab }: { tab: ReportTab }) {
  const { t, i18n } = useTranslation()
  const labels = usePeriodLabels()
  const p = tab.params
  const setParams = (patch: Partial<ReportParams>) => useReportTabs.getState().setParams(tab.id, patch)
  const today = new Date()
  const args = flowQueryArgs(p, today)
  const report = useFlowReport(args)
  const responseMonths = report.data?.months
  const months = useMemo(() => responseMonths ?? [], [responseMonths])
  // the headline's "vs" figure: the equal-length period before the window the
  // server returned, which an ended budget can cut short of the selected one
  const prevRange: MonthRange | null =
    p.mode === 'overTime' && periodRange(p, today) !== null && months.length > 0 && !report.isPlaceholderData
      ? previousRange([months[0], months[months.length - 1]])
      : null
  const prevReport = useFlowReport({ ...args, from: prevRange?.[0], to: prevRange?.[1], enabled: prevRange !== null })
  const money = useMoney(report.data?.currencyId)
  const compact = useMemo(() => compactNumber(i18n.language), [i18n.language])
  const [drill, setDrill] = useState<Drill | null>(null)

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
      expected: report.data
        ? { amount: sum(found.row.amounts.filter((_, i) => months[i] >= from && months[i] <= to)), currencyId: report.data.currencyId }
        : undefined,
    })
    trackEvent(METRICS.REPORTS_DRILL_DOWN, { scope: p.budgetId ? 'budget' : 'mine', mode: p.mode })
  }

  const top = useMemo(() => topGroups(rows, TOP), [rows])
  const totalAmounts = useMemo(() => months.map((_, i) => sum(rows.map((r) => r.amounts[i] ?? '0'))), [months, rows])
  const otherAmounts = useMemo(() => {
    if (top.otherIds.length === 0) return null
    const ids = new Set(top.otherIds)
    const others = rows.filter((r) => ids.has(r.id))
    return months.map((_, i) => sum(others.map((r) => r.amounts[i] ?? '0')))
  }, [months, rows, top])
  const series: Series[] = useMemo(
    () =>
      selected
        ? [{ id: selected.id, name: rowDisplayName(selected, t), amounts: selected.amounts }]
        : [
            ...top.ids.map((id) => {
              const r = rows.find((x) => x.id === id)!
              return { id, name: rowDisplayName(r, t), amounts: r.amounts }
            }),
            ...(otherAmounts ? [{ id: OTHER_ID, name: t('reports.spending.other'), amounts: otherAmounts }] : []),
          ],
    [selected, top, rows, otherAmounts, t],
  )
  // the bucket roll-ups walk every row and month; they read today's date only,
  // so a new day is the one time-based reason to recompute
  const day = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()
  const overTime = useMemo(() => {
    if (p.mode !== 'overTime') return null
    const now = new Date(day)
    return {
      totalBuckets: rollUp(months, totalAmounts, p.step, now),
      seriesBuckets: series.map((s) => rollUp(months, s.amounts, p.step, now)),
      table: bucketChanges(rows, months, p.step, now),
    }
  }, [p.mode, p.step, months, totalAmounts, series, rows, day])
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

  const selection = (range: MonthRange) =>
    selected ? (
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => select(selected.id)}
          className="flex max-w-full items-center gap-1 rounded-full border px-2.5 py-0.5 text-sm"
          aria-label={t('reports.spending.show_all')}
        >
          <span className="size-2.5 shrink-0 rounded-sm" style={{ background: colors[selected.id] }} />
          <span className="truncate">{rowName(selected)}</span>
          <X className="size-3.5 shrink-0 text-muted-foreground" />
        </button>
        <button
          type="button"
          onClick={() => openDrill(range[0], range[1])}
          className="flex items-center gap-1 rounded-full px-2 py-0.5 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <List className="size-3.5" />
          {t('reports.spending.show_transactions')}
        </button>
      </div>
    ) : null

  const childChanges = (compute: (children: ReportRowDto[]) => ChangeRow[]) => (id: Id) => {
    const r = rows.find((x) => x.id === id)
    return r && r.children.length > 0 ? compute(r.children.map(asRow)) : []
  }

  if (p.mode === 'compare') {
    const [earlier, later] = compareYearsOf(p, today)
    const amounts = selected ? selected.amounts : totalAmounts
    const cy = compareYears(months, amounts, earlier, later, today)
    const through = String(cy.throughMonth).padStart(2, '0')
    const aRange: MonthRange = [`${earlier}-01-01`, `${earlier}-${through}-01`]
    const bRange: MonthRange = [`${later}-01-01`, `${later}-${through}-01`]
    const { diff, pct } = changeOf(cy.laterTotal, cy.earlierTotal)
    const laterColor = selected ? colors[selected.id] : CHART_PALETTE[0]
    const data = cy.slots.map((s) => ({
      label: labels.monthShort(s.month),
      title: labels.monthShort(s.month),
      earlier: s.earlier === null ? null : Number(s.earlier),
      later: s.later === null ? null : Number(s.later),
    }))
    const years = [...new Set([...Array.from({ length: 10 }, (_, i) => today.getFullYear() - i), earlier, later])].sort((a, b) => b - a)
    const setYears = (pair: [number, number]) => {
      setParams({ compare: pair })
      trackEvent(METRICS.REPORTS_CHANGE_PERIOD, { period: 'compare' })
    }
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
    const compareRows = (list: ReportRowDto[]) => changeRows(list, months, aRange, bRange)
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
        <section aria-label={t('reports.spending.headline')} className={`flex flex-col gap-1 ${dim}`}>
          {selection(bRange)}
          <span className="text-sm text-muted-foreground">{t(spentKey, { period: labels.range(bRange) })}</span>
          <span className="flex flex-wrap items-baseline gap-x-2">
            <span className="text-2xl tabular-nums">{money(cy.laterTotal)}</span>
            <span className="text-sm">
              <Change diff={diff} pct={pct} side={p.side} money={money} />
              <span className="text-muted-foreground"> {t('reports.spending.vs', { period: labels.range(aRange) })}</span>
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
          list={compareRows(rows)}
          childrenOf={childChanges(compareRows)}
          aLabel={labels.range(aRange)}
          bLabel={labels.range(bRange)}
          side={p.side}
          selectedId={selected?.id ?? null}
          colors={colors}
          money={money}
          onSelect={select}
        />
        <p className="text-xs text-muted-foreground">
          {cy.throughMonth < 12 ? `${t('reports.spending.compare_note', { period: labels.monthsOfYear(1, cy.throughMonth) })} ` : null}
          {t(selected ? 'reports.spending.hint_drill' : 'reports.spending.hint_select')}
        </p>
        {drill ? <ReportTransactionsDialog scope={{ budgetId: p.budgetId }} {...drill} onClose={() => setDrill(null)} /> : null}
      </div>
    )
  }

  const { totalBuckets, seriesBuckets, table } = overTime!
  const chartBuckets: Bucket[] = selected ? seriesBuckets[0] : totalBuckets
  const projected = chartBuckets.find((b) => b.projected !== b.total) ?? null
  const showPlanned = p.budgetId !== null && selected !== null && p.step === 'month' && found?.parent === null && selected.planned.length === months.length
  const bucketName = (b: Bucket) => `${labels.bucket(b.start, p.step)}${b.projected !== b.total ? '*' : ''}`
  const data = chartBuckets.map((b, i) => {
    const point: Record<string, unknown> = {
      label: bucketName(b),
      title: labels.range([b.first, lastMonthOf(b)]),
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

  // headline: the whole selected period against the equal-length period before it
  const windowRange: MonthRange = [months[0], months[months.length - 1]]
  const headlineTotal = sum(selected ? selected.amounts : totalAmounts)
  // a previous window the server cut or moved is no like-for-like comparison
  const prevMonths = prevReport.data?.months ?? []
  const prevMatches = prevRange !== null && prevMonths[0] === prevRange[0] && prevMonths[prevMonths.length - 1] === prevRange[1]
  const prevRows = prevReport.data && !prevReport.isPlaceholderData && prevMatches ? prevReport.data[p.side] : null
  // a selected row (a child included) missing from the previous period counts as no data there
  const prevAmounts = prevRows ? (selected ? findRow(prevRows, selected.id)?.row.amounts : prevRows.flatMap((r) => r.amounts)) : undefined
  const prevTotal = prevAmounts && prevAmounts.some((a) => !isZero(a)) ? sum(prevAmounts) : null
  const headlineChange = prevTotal !== null ? changeOf(headlineTotal, prevTotal) : null
  const insight = selected ? null : spendingInsight(rows)
  const insightRow = insight?.params.id ? rows.find((r) => r.id === insight.params.id) : undefined
  const lastBucket = chartBuckets[chartBuckets.length - 1]
  const onPace = lastBucket && lastBucket.projected !== lastBucket.total ? lastBucket : null

  // table: the last bucket against the one before it
  const tableOf = (list: ReportRowDto[]) => bucketChanges(list, months, p.step, today)

  const onSegment = (id: Id) => (_: unknown, index: number) => {
    const bucket = chartBuckets[index]
    if (!bucket) return
    if (selected) {
      openDrill(bucket.first, lastMonthOf(bucket))
    } else if (id !== OTHER_ID) {
      select(id)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {header}
      <section aria-label={t('reports.spending.headline')} className={`flex flex-col gap-1 ${dim}`}>
        {selection(windowRange)}
        <span className="text-sm text-muted-foreground">{t(spentKey, { period: labels.range(windowRange) })}</span>
        <span className="flex flex-wrap items-baseline gap-x-2">
          <span className="text-2xl tabular-nums">{money(headlineTotal)}</span>
          {headlineChange && prevRange ? (
            <span className="text-sm">
              <Change diff={headlineChange.diff} pct={headlineChange.pct} side={p.side} money={money} />
              <span className="text-muted-foreground"> {t('reports.spending.vs', { period: labels.range(prevRange) })}</span>
            </span>
          ) : null}
        </span>
        {onPace ? (
          <span className="text-sm text-muted-foreground">
            {t('reports.spending.on_pace', { amount: money(onPace.projected), period: labels.bucket(onPace.start, p.step) })}
          </span>
        ) : null}
        {insight && insightRow ? (
          <span className="text-sm text-muted-foreground">
            {t(insight.key, { ...insight.params, name: rowName(insightRow), amount: money(insight.params.amount ?? '0') })}
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
                      ? [{ label: t('reports.spending.total'), value: money(sum(series.map((s) => String(d[s.id] ?? 0)))) }]
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
      {table ? (
        <ChangeTable
          key={`${table.later.start}|${table.earlier?.start ?? ''}`}
          list={table.rows}
          childrenOf={childChanges((list) => tableOf(list)?.rows ?? [])}
          aLabel={table.earlier ? labels.bucket(table.earlier.start, p.step) : null}
          bLabel={bucketName(table.later)}
          side={p.side}
          selectedId={selected?.id ?? null}
          colors={colors}
          money={money}
          onSelect={select}
        />
      ) : null}
      <p className="text-xs text-muted-foreground">
        {projected
          ? `${t('reports.spending.projected_note', { period: labels.bucket(projected.start, p.step) })} `
          : null}
        {t(selected ? 'reports.spending.hint_drill' : 'reports.spending.hint_select')}
      </p>
      {drill ? <ReportTransactionsDialog scope={{ budgetId: p.budgetId }} {...drill} onClose={() => setDrill(null)} /> : null}
    </div>
  )
}
