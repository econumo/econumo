// SPIKE (throwaway): simplified net worth — one total line, own accounts + one row per partner,
// controls tucked into the corners. Strings are hard-coded English on purpose.
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Area, CartesianGrid, ComposedChart, ReferenceLine, XAxis, YAxis } from 'recharts'
import type { NetWorthAccountDto } from '@/api/dto/report'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import type { ChartConfig } from '@/components/ui/chart'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useBudgets } from '@/features/budgets/queries'
import { useUserData } from '@/features/user/queries'
import { add, cmp, isNegative, isZero, sub } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { netWorthGroupOf } from './reportMath'
import { netWorthQueryArgs } from './reportPeriod'
import { usePeriodLabels } from './usePeriodLabels'
import { useNetWorthReport } from './queries'
import { compactNumber, signedMoney, useMoney } from './reportFormat'
import { ReportUpdatedAt } from './ReportUpdatedAt'
import { useReportTabs } from './reportTabs'
import type { ReportParams, ReportTab } from './reportTabs'

const PRESETS = [
  { value: 'last12', label: '1y', words: 'in the last year' },
  { value: 'last3y', label: '3y', words: 'in the last 3 years' },
  { value: 'all', label: 'All', words: 'since the start' },
] as const
const MINE = 'mine'
const LINE = 'var(--primary)'
// calm, ordered by size: the biggest band gets the first colour
const BANDS = ['#a78bfa', '#7dd3fc', '#6ee7b7', '#fcd34d', '#f9a8d4', '#cbd5e1']

const changeClass = (v: string) => (isZero(v) ? 'text-muted-foreground' : isNegative(v) ? 'text-expense' : 'text-income')
const allZero = (a: NetWorthAccountDto) => a.balances.every((b) => isZero(b))
const sumAt = (rows: NetWorthAccountDto[], i: number) => rows.reduce((s, a) => add(s, a.balances[i] ?? '0'), '0')

interface Line {
  id: string
  name: string
  start: string
  now: string
  change: string
}
const lineOf = (id: string, name: string, start: string, now: string): Line => ({ id, name, start, now, change: sub(now, start) })
const byNow = (a: Line, b: Line) => cmp(b.now, a.now) || a.name.localeCompare(b.name)

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
  const { data: budgets } = useBudgets()
  const { data: folders } = useFolders()
  const [open, setOpen] = useState<Set<string>>(new Set())
  const preset = PRESETS.find((o) => o.value === p.period) ?? PRESETS[1]

  const scopePicker = (
    <Select
      value={p.budgetId ?? MINE}
      onValueChange={(v) => {
        const budgetId = v === MINE ? null : v
        setParams({ budgetId, selectedId: null })
        trackEvent(METRICS.REPORTS_CHANGE_SCOPE, { scope: budgetId ? 'budget' : 'mine' })
      }}
    >
      <SelectTrigger
        aria-label={t('reports.list.scope')}
        className="h-7 w-auto gap-1 border-0 bg-transparent px-1.5 text-sm text-muted-foreground shadow-none hover:text-foreground dark:bg-transparent"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent align="end">
        <SelectItem value={MINE}>{t('reports.list.scope_mine')}</SelectItem>
        {(budgets ?? []).map((b) => (
          <SelectItem key={b.id} value={b.id}>{b.name}</SelectItem>
        ))}
      </SelectContent>
    </Select>
  )

  const periodPicker = (
    <div role="radiogroup" aria-label={t('reports.filters.period')} className="flex gap-0.5 text-xs">
      {PRESETS.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={preset.value === o.value}
          onClick={() => {
            if (preset.value === o.value) return
            setParams({ period: o.value })
            trackEvent(METRICS.REPORTS_CHANGE_PERIOD, { period: o.value })
          }}
          className={`rounded-md px-2 py-0.5 ${preset.value === o.value ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:text-foreground'}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )

  const data = report.data
  const shown = data ? data.accounts.filter((a) => !allZero(a)) : []
  const me = user?.id ?? ''
  const budget = budgets?.find((b) => b.id === p.budgetId)
  const ownerName = (id: string) =>
    budget?.access.find((a) => a.user.id === id)?.user.name ??
    (accounts ?? []).find((a) => a.owner.id === id)?.owner.name ??
    t('reports.networth.other_owner')

  const n = data?.months.length ?? 0
  const total = data ? data.months.map((_, i) => sumAt(shown, i)) : []
  const totalLine = lineOf('total', t('reports.spending.total'), total[0] ?? '0', total[n - 1] ?? '0')
  const partnerIds = [...new Set(shown.filter((a) => a.ownerUserId !== me).map((a) => a.ownerUserId))]
  const partners = partnerIds
    .map((id) => {
      const rows = shown.filter((a) => a.ownerUserId === id)
      return lineOf(id, ownerName(id), sumAt(rows, 0), sumAt(rows, n - 1))
    })
    .sort(byNow)

  const num = 'py-2 pl-2 text-right tabular-nums'
  // My accounts: folders, each expandable to its accounts (as before).
  const groupOf = netWorthGroupOf({
    me,
    folderOf: Object.fromEntries((accounts ?? []).map((a) => [a.id, a.folderId])),
    folders: folders ?? [],
    owners: {},
    names: { noFolder: t('reports.networth.no_folder'), deleted: t('reports.networth.deleted'), otherOwner: t('reports.networth.other_owner') },
  })
  const folderGroups = p.budgetId
    ? []
    : [...new Set(shown.map((a) => groupOf(a).id))]
        .map((id) => {
          const rows = shown.filter((a) => groupOf(a).id === id)
          return {
            balances: Array.from({ length: n }, (_, i) => sumAt(rows, i)),
            line: lineOf(id, groupOf(rows[0]).name, sumAt(rows, 0), sumAt(rows, n - 1)),
            accounts: rows.map((a) => lineOf(a.id, a.name, a.balances[0], a.balances[n - 1])).sort(byNow),
          }
        })
        .sort((a, b) => byNow(a.line, b.line))
  const toggle = (id: string) =>
    setOpen((cur) => {
      const next = new Set(cur)
      if (next.has(id)) next.delete(id)
      else {
        next.add(id)
        trackEvent(METRICS.REPORTS_NETWORTH_EXPAND, { scope: 'mine' })
      }
      return next
    })
  const groupRows = (groups: { line: Line; accounts: Line[] }[]) => groups.flatMap((g) => {
    const isOpen = open.has(g.line.id)
    if (g.accounts.length === 0) {
      return [
        <tr key={g.line.id} className="border-b border-border/60 hover:bg-econumo-hover/60">
          <td className="py-2 pr-2 pl-[1.125rem]">
            <span className="flex min-w-0 items-center gap-1">
                <span className="truncate">{g.line.name}</span>
            </span>
          </td>
          <td className={`hidden ${num} text-muted-foreground sm:table-cell`}>{money(g.line.start)}</td>
          <td className={num}>{money(g.line.now)}</td>
          <td className={`${num} whitespace-nowrap ${changeClass(g.line.change)}`}>{signedMoney(g.line.change, money)}</td>
        </tr>,
      ]
    }
    return [
      <tr key={g.line.id} onClick={() => toggle(g.line.id)} className="cursor-pointer border-b border-border/60 hover:bg-econumo-hover/60">
        <td className="py-2 pr-2">
          <span className="flex min-w-0 items-center gap-1">
            <ChevronRight className={`size-3.5 shrink-0 text-muted-foreground transition-transform ${isOpen ? 'rotate-90' : ''}`} />
            <button type="button" aria-expanded={isOpen} className="truncate text-left" onClick={(e) => { e.stopPropagation(); toggle(g.line.id) }}>
              {g.line.name}
            </button>
          </span>
        </td>
        <td className={`hidden ${num} text-muted-foreground sm:table-cell`}>{money(g.line.start)}</td>
        <td className={num}>{money(g.line.now)}</td>
        <td className={`${num} whitespace-nowrap ${changeClass(g.line.change)}`}>{signedMoney(g.line.change, money)}</td>
      </tr>,
      ...(isOpen
        ? g.accounts.map((a) => (
            <tr key={`${g.line.id}-${a.id}`} className="border-b border-border/60 text-muted-foreground hover:bg-econumo-hover/60">
              <td className="truncate py-2 pr-2 pl-5">{a.name}</td>
              <td className={`hidden ${num} sm:table-cell`}>{money(a.start)}</td>
              <td className={num}>{money(a.now)}</td>
              <td className={`${num} whitespace-nowrap ${changeClass(a.change)}`}>{signedMoney(a.change, money)}</td>
            </tr>
          ))
        : []),
    ]
  })

  const meRows = shown.filter((a) => a.ownerUserId === me)
  const series = (rows: NetWorthAccountDto[]) => Array.from({ length: n }, (_, i) => sumAt(rows, i))
  const bands = p.budgetId
    ? [
        ...partners.map((l) => ({ id: l.id, name: l.name, balances: series(shown.filter((a) => a.ownerUserId === l.id)) })),
        ...(meRows.length > 0 ? [{ id: me, name: user?.name ?? '', balances: series(meRows) }] : []),
      ]
    : folderGroups.map((g) => ({ id: g.line.id, name: g.line.name, balances: g.balances }))
  const DEBT = '#fda4af'
  const assets = bands.filter((b) => !isNegative(b.balances[n - 1] ?? '0'))
  const colorOf = (id: string) => {
    const i = assets.findIndex((b) => b.id === id)
    return i < 0 ? DEBT : BANDS[i % BANDS.length]
  }
  const folderRows = groupRows(folderGroups)
  // Each person: their savings members by name, everything else summed as "Other";
  // partners first, the user last.
  const personGroup = (id: string, name: string) => {
    const rows = shown.filter((a) => a.ownerUserId === id)
    const saving = rows.filter((a) => a.isSavings === 1)
    const rest = rows.filter((a) => a.isSavings !== 1)
    const children = saving.map((a) => lineOf(a.id, a.name, a.balances[0], a.balances[n - 1])).sort(byNow)
    if (saving.length > 0 && rest.length > 0) children.push(lineOf(`${id}-other`, 'Everyday accounts', sumAt(rest, 0), sumAt(rest, n - 1)))
    return { line: lineOf(id, name, sumAt(rows, 0), sumAt(rows, n - 1)), accounts: saving.length > 0 ? children : [] }
  }
  const personRows = p.budgetId
    ? groupRows(
        [
          ...partners.map((l) => personGroup(l.id, l.name)),
          ...(meRows.length > 0 ? [personGroup(me, user?.name ?? '')] : []),
        ],
      )
    : []

  const chartData = data
    ? data.months.map((m, i) => ({
        label: labels.month(m),
        value: Number(total[i]),
        ...Object.fromEntries(bands.map((b, bi) => [`b${bi}`, Number(b.balances[i])])),
      }))
    : []
  const config: ChartConfig = {
    value: { label: t('reports.spending.total'), color: LINE },
    ...Object.fromEntries(bands.map((b, bi) => [`b${bi}`, { label: b.name, color: colorOf(b.id) }])),
  }
  const hasNegative = total.some((v) => isNegative(v))
  const dim = report.isPlaceholderData ? 'opacity-60 transition-opacity' : ''

  const row = (l: Line, strong = false) => (
    <tr key={l.id} className={`hover:bg-econumo-hover/60 ${strong ? '' : 'border-b border-border/60'}`}>
      <td className="truncate py-2 pr-2">{l.name}</td>
      <td className={`hidden ${num} text-muted-foreground sm:table-cell`}>{money(l.start)}</td>
      <td className={num}>{money(l.now)}</td>
      <td className={`${num} whitespace-nowrap ${changeClass(l.change)}`}>{signedMoney(l.change, money)}</td>
    </tr>
  )

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <div className={`flex min-w-0 flex-col ${dim}`}>
          <span className="text-sm text-muted-foreground">{t('reports.networth.today')}</span>
          <span className="text-2xl tabular-nums sm:text-3xl">{data ? money(totalLine.now) : '…'}</span>
          {data && n > 1 ? (
            <span className="text-sm">
              <span className={changeClass(totalLine.change)}>{signedMoney(totalLine.change, money)}</span>{' '}
              <span className="text-muted-foreground">{preset.words}</span>
            </span>
          ) : null}
        </div>
        {scopePicker}
      </div>
      <ReportUpdatedAt query={report} />

      {report.isPending ? (
        <div className="h-64 animate-pulse rounded-xl bg-econumo-hover" />
      ) : report.isError && !data ? (
        <p className="text-sm text-muted-foreground">{t('common.app.error')}</p>
      ) : shown.length === 0 ? (
        <p className="py-6 text-sm text-muted-foreground">{t('reports.networth.empty')}</p>
      ) : (
        <>
          <figure aria-label={t('reports.networth.chart')} className={`flex flex-col gap-1 ${dim}`}>
            <div className="flex justify-end">{periodPicker}</div>
            <ChartContainer config={config} className="aspect-auto h-56 w-full sm:h-64">
              <ComposedChart data={chartData} margin={{ top: 6, right: 4, left: 0, bottom: 0 }}>
                <CartesianGrid vertical={false} strokeOpacity={0.5} />
                <XAxis dataKey="label" tickLine={false} axisLine={false} interval="preserveStartEnd" minTickGap={40} />
                <YAxis width={40} tickLine={false} axisLine={false} tickFormatter={compact} />
                {hasNegative ? <ReferenceLine y={0} stroke="var(--muted-foreground)" strokeOpacity={0.6} /> : null}
                <ChartTooltip
                  cursor={{ stroke: 'var(--muted-foreground)', strokeDasharray: '3 3' }}
                  content={({ active, payload }) => {
                    if (!active || !payload?.length) return null
                    const row = payload[0].payload as Record<string, number | string>
                    return (
                      <div className="min-w-40 rounded-lg border bg-background px-2.5 py-1.5 text-xs shadow-sm">
                        <div className="mb-1 text-muted-foreground">{row.label}</div>
                        {bands.map((b, bi) => (
                          <div key={b.id} className="flex items-center justify-between gap-3 text-muted-foreground">
                            <span className="flex items-center gap-1.5">
                              <span className="size-2 rounded-full" style={{ background: colorOf(b.id) }} />
                              {b.name}
                            </span>
                            <span className="tabular-nums">{money(String(row[`b${bi}`]))}</span>
                          </div>
                        ))}
                        <div className="mt-1 flex items-center justify-between gap-3 border-t pt-1">
                          <span>{t('reports.spending.total')}</span>
                          <span className="tabular-nums">{money(String(row.value))}</span>
                        </div>
                      </div>
                    )
                  }}
                />
                <defs>
                  <linearGradient id="nw-fill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={LINE} stopOpacity={0.25} />
                    <stop offset="100%" stopColor={LINE} stopOpacity={0.02} />
                  </linearGradient>
                </defs>
                <Area dataKey="value" type="monotone" stroke={LINE} strokeWidth={2.5} fill="url(#nw-fill)" isAnimationActive={false} />
              </ComposedChart>
            </ChartContainer>
          </figure>

          <table className={`w-full table-fixed text-sm ${dim}`}>
            <thead>
              <tr className="text-xs text-muted-foreground">
                <th className="py-1 pr-2 text-left font-normal" />
                <th className="hidden w-28 py-1 pl-2 text-right font-normal sm:table-cell">{t('reports.networth.col_start')}</th>
                <th className="w-28 py-1 pl-2 text-right font-normal">{t('reports.networth.col_now')}</th>
                <th className="w-28 py-1 pl-2 text-right font-normal">{t('reports.spending.col_change')}</th>
              </tr>
            </thead>
            <tbody>
              {p.budgetId ? (
                personRows
              ) : (
                folderRows
              )}
            </tbody>
            <tfoot className="border-t">{row(totalLine, true)}</tfoot>
          </table>
        </>
      )}
    </div>
  )
}
