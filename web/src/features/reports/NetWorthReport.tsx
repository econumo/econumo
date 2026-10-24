import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { EntityIcon } from '@/components/EntityIcon'
import { UserAvatar } from '@/components/UserAvatar'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Area, CartesianGrid, ComposedChart, ReferenceLine, XAxis, YAxis } from 'recharts'
import type { NetWorthAccountDto } from '@/api/dto/report'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import type { ChartConfig } from '@/components/ui/chart'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'
import { add, cmp, isNegative, isZero, sub } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { netWorthGroupOf } from './reportMath'
import { netWorthQueryArgs } from './reportPeriod'
import { usePeriodLabels } from './usePeriodLabels'
import { useNetWorthReport, useReportBudgets } from './queries'
import { compactNumber, signedMoney, useMoney } from './reportFormat'
import { ReportUpdatedAt } from './ReportUpdatedAt'
import { LINE, NAME_COL } from '@/features/budgets/monthLayout'
import { useReportTabs } from './reportTabs'
import type { ReportParams, ReportTab } from './reportTabs'

const PRESETS = [
  { value: 'last12', key: 'reports.networth.period_1y' },
  { value: 'last3y', key: 'reports.networth.period_3y' },
  { value: 'all', key: 'reports.networth.period_all' },
] as const
const MINE = 'mine'
const COL = 'w-24 shrink-0 justify-end text-right tabular-nums whitespace-nowrap sm:w-28 flex'
const STROKE = 'var(--primary)'
// calm, ordered by size: the biggest band gets the first colour
const BANDS = ['#a78bfa', '#7dd3fc', '#6ee7b7', '#fcd34d', '#f9a8d4', '#cbd5e1']

const changeClass = (v: string) => (isZero(v) ? 'text-muted-foreground' : isNegative(v) ? 'text-expense' : 'text-income')
const allZero = (a: NetWorthAccountDto) => a.balances.every((b) => isZero(b))
const sumAt = (rows: NetWorthAccountDto[], i: number) => rows.reduce((s, a) => add(s, a.balances[i] ?? '0'), '0')

interface Line {
  icon?: ReactNode
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
  const { data: budgets } = useReportBudgets()
  const { data: folders } = useFolders()
  const [open, setOpen] = useState<Set<string>>(new Set())
  const preset = PRESETS.find((o) => o.value === p.period) ?? PRESETS[1]

  const scopePicker = (budgets ?? []).length === 0 ? null : (
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
          {t(o.key)}
        </button>
      ))}
    </div>
  )

  const data = report.data
  const shown = data ? data.accounts.filter((a) => !allZero(a)) : []
  const me = user?.id ?? ''
  const budget = budgets?.find((b) => b.id === p.budgetId)
  const ownerAvatar = (id: string) =>
    id === me
      ? user?.avatar
      : (budget?.access.find((a) => a.user.id === id)?.user.avatar ?? (accounts ?? []).find((a) => a.owner.id === id)?.owner.avatar)
  const accountLine = (a: NetWorthAccountDto): Line => ({
    ...lineOf(a.id, a.name, a.balances[0], a.balances[n - 1]),
    icon: <EntityIcon name={a.icon} className="text-base text-muted-foreground" />,
  })
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
            accounts: rows.map(accountLine).sort(byNow),
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
  // Lines styled like the Budget/Plan rows: rounded hover, no rules between rows.
  const figures = (l: Line, muted = false) => (
    <>
      <span role="cell" className={`hidden ${COL} text-muted-foreground sm:flex`}>{money(l.start)}</span>
      <span role="cell" className={`${COL} ${muted ? '' : 'text-foreground'}`}>{money(l.now)}</span>
      <span role="cell" className={`${COL} ${changeClass(l.change)}`}>{signedMoney(l.change, money)}</span>
    </>
  )
  const groupRows = (groups: { line: Line; accounts: Line[] }[]) => groups.flatMap((g) => {
    const isOpen = open.has(g.line.id)
    if (g.accounts.length === 0) {
      return [
        <div key={g.line.id} role="row" className={`${LINE} min-h-10 rounded-md py-1.5 pl-6 hover:bg-accent/50`}>
          <span role="cell" className={NAME_COL}>{g.line.icon}<span className="truncate">{g.line.name}</span></span>
          {figures(g.line)}
        </div>,
      ]
    }
    return [
      <div
        key={g.line.id}
        role="row"
        onClick={() => toggle(g.line.id)}
        className={`${LINE} min-h-10 cursor-pointer rounded-md py-1.5 hover:bg-accent/50`}
      >
        <span role="cell" className={NAME_COL}>
          <button
            type="button"
            aria-expanded={isOpen}
            aria-label={t(isOpen ? 'reports.networth.collapse' : 'reports.networth.expand', { name: g.line.name })}
            onClick={(e) => {
              e.stopPropagation()
              toggle(g.line.id)
            }}
            className="grid size-4 shrink-0 place-items-center text-muted-foreground"
          >
            <ChevronRight className={`size-4 transition-transform ${isOpen ? 'rotate-90' : ''}`} />
          </button>
          {g.line.icon}
          <span className="truncate">{g.line.name}</span>
        </span>
        {figures(g.line)}
      </div>,
      ...(isOpen
        ? g.accounts.map((acc) => (
            <div key={`${g.line.id}-${acc.id}`} role="row" className={`${LINE} min-h-8 rounded-md py-1 pl-12 text-sm text-muted-foreground hover:bg-accent/50`}>
              <span role="cell" className={NAME_COL}>{acc.icon}<span className="truncate">{acc.name}</span></span>
              {figures(acc, true)}
            </div>
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
    const children = saving.map(accountLine).sort(byNow)
    if (saving.length > 0 && rest.length > 0) children.push({ ...lineOf(`${id}-other`, t('reports.networth.everyday'), sumAt(rest, 0), sumAt(rest, n - 1)), icon: <EntityIcon name="account_balance_wallet" className="text-base text-muted-foreground" /> })
    const avatar = ownerAvatar(id)
    return {
      line: { ...lineOf(id, name, sumAt(rows, 0), sumAt(rows, n - 1)), icon: avatar ? <UserAvatar avatar={avatar} size="xs" /> : undefined },
      accounts: saving.length > 0 ? children : [],
    }
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
    value: { label: t('reports.spending.total'), color: STROKE },
    ...Object.fromEntries(bands.map((b, bi) => [`b${bi}`, { label: b.name, color: colorOf(b.id) }])),
  }
  const hasNegative = total.some((v) => isNegative(v))
  const dim = report.isPlaceholderData ? 'opacity-60 transition-opacity' : ''


  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <section aria-label={t('reports.spending.headline')} className={`flex min-w-0 flex-col ${dim}`}>
          <span className="text-sm text-muted-foreground">{t('reports.networth.today')}</span>
          <span className="text-2xl tabular-nums sm:text-3xl">{data ? money(totalLine.now) : '…'}</span>
          {data && n > 1 ? (
            <span className="text-sm">
              <span className={changeClass(totalLine.change)}>{signedMoney(totalLine.change, money)}</span>{' '}
              <span className="text-muted-foreground">{t(`${preset.key}_words`)}</span>
            </span>
          ) : null}
        </section>
        {scopePicker}
      </div>
      <ReportUpdatedAt query={report} />

      {report.isPending ? (
        <div className="h-64 animate-pulse rounded-xl bg-econumo-hover" />
      ) : report.isError && !data ? (
        <p className="text-sm text-muted-foreground">{t('common.app.error')}</p>
      ) : shown.length === 0 ? (
        // the period stays switchable: an empty 1y window may hide balances All would show
        <div className="flex flex-col gap-1">
          <div className="flex justify-end">{periodPicker}</div>
          <p className="py-6 text-sm text-muted-foreground">{t('reports.networth.empty')}</p>
        </div>
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
                    <stop offset="0%" stopColor={STROKE} stopOpacity={0.25} />
                    <stop offset="100%" stopColor={STROKE} stopOpacity={0.02} />
                  </linearGradient>
                </defs>
                <Area dataKey="value" type="monotone" stroke={STROKE} strokeWidth={2.5} fill="url(#nw-fill)" isAnimationActive={false} />
              </ComposedChart>
            </ChartContainer>
          </figure>

          <div role="table" aria-label={t('reports.list.networth_q')} className={`-mx-2 flex flex-col ${dim}`}>
            <div role="row" className={`${LINE} pb-1 text-xs text-muted-foreground`}>
              <span role="columnheader" className={NAME_COL} />
              <span role="columnheader" className={`hidden ${COL} sm:flex`}>{t('reports.networth.col_start')}</span>
              <span role="columnheader" className={COL}>{t('reports.networth.col_now')}</span>
              <span role="columnheader" className={COL}>{t('reports.spending.col_change')}</span>
            </div>
            {p.budgetId ? personRows : folderRows}
            <div className="mt-1 border-t pt-1">
              <div role="row" className={`${LINE} min-h-10 rounded-md py-1.5 pl-6 text-[15px] hover:bg-accent/50`}>
                <span role="cell" className={NAME_COL}>{totalLine.name}</span>
                {figures(totalLine)}
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
