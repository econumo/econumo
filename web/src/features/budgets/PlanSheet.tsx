import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { CoinLoader } from '@/components/CoinLoader'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { BudgetDto, BudgetMetaDto, PlanChildDto, PlanElementDto } from '@/api/dto/budget'
import { isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { useIsCompact } from '@/hooks/useIsCompact'
import { elementDisplayName } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import { canUpdateLimits, useBudgetPlan, usePlanSetLimit } from './queries'
import { LimitEditor } from './LimitEditor'
import { SetLimitDialog } from './SetLimitDialog'
import {
  PLAN_MIN_MONTH_COL_PX,
  PLAN_NAME_COL_PX,
  addMonths,
  balanceRow,
  bucketPlanRows,
  clampFirstMonth,
  currentMonth,
  makePlanExchange,
  planInitialFirstMonth,
  planTotals,
  planVisibleCount,
} from './planMath'
import type { PlanFolderSection, PlanMonthTotals, PlanRow } from './planMath'

export interface PlanSheetProps {
  /** the ALREADY-LOADED budget (meta for permissions/currency); plan data is fetched inside */
  budget: BudgetDto
  currencies: CurrencyDto[]
  userId: Id | undefined
}

const rowKey = (r: PlanRow): string => `${r.element.id}:${r.element.type}`

// A future month with no activity yet reads as a dash, same as a missing
// cell; a real (possibly zero) actual in a past/current month still prints.
function renderActual(actual: string | undefined, month: string, cur: string, currency: CurrencyDto | undefined): string {
  if (actual === undefined) {
    return '—'
  }
  if (month > cur && isZero(actual)) {
    return '—'
  }
  return moneyFormat(actual, currency, { showCurrency: false, useNativePrecision: false })
}

interface PlanLimitTarget {
  el: PlanElementDto
  month: string
  monthIndex: number
}

interface GridCtx {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  cur: string
  currencies: CurrencyDto[]
  gridCols: string
  meta: BudgetMetaDto
  userId: Id | undefined
  isCompact: boolean
  commit: (elementId: Id, month: string, monthIndex: number, amount: string | null) => void
  openDialog: (target: PlanLimitTarget) => void
}

function ChildRow({ child, parentCurrency, ctx }: { child: PlanChildDto; parentCurrency: CurrencyDto | undefined; ctx: GridCtx }) {
  const { t } = useTranslation()
  const displayName = elementDisplayName(child.id, child.name, t)
  return (
    <div
      role="row"
      data-row-id={`${child.id}:${child.type}`}
      className="grid items-center gap-1 py-1 pr-2 pl-9 text-xs text-muted-foreground"
      style={{ gridTemplateColumns: ctx.gridCols }}
    >
      <span className="flex min-w-0 items-center gap-1.5 truncate" title={displayName}>
        <EntityIcon name={child.icon} className="text-base" />
        <span className="truncate">{displayName}</span>
      </span>
      {ctx.visibleMonths.map((m, i) => {
        const idx = ctx.monthIndex(m)
        const cell = idx >= 0 ? child.cells[idx] : undefined
        return (
          <div
            key={m}
            data-month={m}
            data-testid={`plan-cell-${child.id}:${i}`}
            className={`flex items-end justify-end px-2 py-1 ${m === ctx.cur ? 'bg-accent/40' : ''}`}
          >
            <span data-testid="cell-actual">{renderActual(cell?.actual, m, ctx.cur, parentCurrency)}</span>
          </div>
        )
      })}
    </div>
  )
}

function ElementRow({ row, ctx }: { row: PlanRow; ctx: GridCtx }) {
  const { t } = useTranslation()
  const el = row.element
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[el.id])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const currency = ctx.currencies.find((c) => c.id === el.currencyId)
  const displayName = elementDisplayName(el.id, el.name, t)
  const isUncategorized = el.id === UNCATEGORIZED_ID
  const expandable = el.children.length > 0
  const Chevron = unfolded ? ChevronDown : ChevronRight
  // children/uncategorized/archived rows never carry their own limit; canUpdateLimits below adds the role + start-date gate per cell
  const editableRow = !isUncategorized && el.isArchived === 0

  const name = (
    <>
      <EntityIcon name={el.icon} className="text-lg text-muted-foreground" />
      <span className="truncate text-sm" title={displayName}>
        {displayName}
      </span>
    </>
  )

  return (
    <div data-row-id={`${el.id}:${el.type}`}>
      <div
        role="row"
        className="grid items-center gap-1 rounded-md px-2 py-1.5 hover:bg-accent/50"
        style={{ gridTemplateColumns: ctx.gridCols }}
      >
        {expandable ? (
          <button
            type="button"
            className="flex min-w-0 items-center gap-1.5 text-left"
            aria-expanded={unfolded}
            title={t(unfolded ? 'common.button.collapse.label' : 'common.button.expand.label')}
            onClick={() => toggleElement(el.id)}
          >
            <Chevron className="size-3.5 shrink-0 text-muted-foreground" />
            {name}
          </button>
        ) : (
          <span className="flex min-w-0 items-center gap-1.5">
            {isUncategorized ? null : <span className="w-3.5 shrink-0" />}
            {name}
          </span>
        )}
        {ctx.visibleMonths.map((m, i) => {
          const idx = ctx.monthIndex(m)
          const cell = idx >= 0 ? el.cells[idx] : undefined
          const editable = editableRow && idx >= 0 && canUpdateLimits(ctx.meta, ctx.userId, m)
          // overspend highlight: expense side only, current/future months, a set plan the actual has already cleared
          const overspend =
            !isIncomeType(el.type) && m >= ctx.cur && !!cell && cell.planned !== '' && cmp(cell.actual, cell.planned) > 0
          const plannedValue = cell && cell.planned !== '' ? cell.planned : '0'
          const plannedText = cell && cell.planned !== '' ? moneyFormat(cell.planned, currency, { showCurrency: false, useNativePrecision: false }) : '—'
          return (
            <div
              key={m}
              data-month={m}
              data-testid={`plan-cell-${el.id}:${i}`}
              className={`flex flex-col items-end px-2 py-1 ${m === ctx.cur ? 'bg-accent/40' : ''}`}
            >
              <span data-testid="cell-actual" className={`text-xs ${overspend ? 'text-destructive' : 'text-muted-foreground'}`}>
                {renderActual(cell?.actual, m, ctx.cur, currency)}
              </span>
              <span data-testid="cell-planned" className="text-sm">
                {editable && !ctx.isCompact ? (
                  <LimitEditor
                    id={`${el.id}-${m}`}
                    name={displayName}
                    value={plannedValue}
                    currency={currency}
                    onCommit={(amount) => ctx.commit(el.id, m, idx, amount)}
                  />
                ) : editable ? (
                  <button
                    type="button"
                    className="w-full text-right underline-offset-2 hover:underline"
                    aria-label={`limit ${displayName}`}
                    onClick={() => ctx.openDialog({ el, month: m, monthIndex: idx })}
                  >
                    {moneyFormat(plannedValue, currency, { showCurrency: false, useNativePrecision: false })}
                  </button>
                ) : (
                  plannedText
                )}
              </span>
            </div>
          )
        })}
      </div>
      {expandable && unfolded ? (
        <div>
          {el.children.map((child) => (
            <ChildRow key={child.id} child={child} parentCurrency={currency} ctx={ctx} />
          ))}
        </div>
      ) : null}
    </div>
  )
}

function SectionHeader({ label }: { label: string }) {
  return <div className="px-2 pt-2 pb-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">{label}</div>
}

function FolderRows({ section, ctx }: { section: PlanFolderSection; ctx: GridCtx }) {
  if (section.rows.length === 0) {
    return null
  }
  return (
    <div className="mb-1 rounded-md border p-1.5" data-testid={`plan-folder-${section.folder.id}`}>
      <div className="truncate px-1.5 pb-1 text-sm font-medium" title={section.folder.name}>
        {section.folder.name}
      </div>
      {section.rows.map((r) => (
        <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
      ))}
    </div>
  )
}

interface TotalsRowSpec {
  key: 'income' | 'expenses' | 'net'
  actual: (t: PlanMonthTotals) => string
  planned: (t: PlanMonthTotals) => string
}

const TOTALS_ROWS: TotalsRowSpec[] = [
  { key: 'income', actual: (t) => t.incomeActual, planned: (t) => t.incomePlanned },
  { key: 'expenses', actual: (t) => t.expenseActual, planned: (t) => t.expensePlanned },
  { key: 'net', actual: (t) => t.netActual, planned: (t) => t.netPlanned },
]

function TotalsFooter({
  visibleMonths,
  monthIndex,
  cur,
  gridCols,
  totals,
  balance,
  currency,
}: {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  cur: string
  gridCols: string
  totals: PlanMonthTotals[]
  balance: string[]
  currency: CurrencyDto | undefined
}) {
  const { t } = useTranslation()
  return (
    <div className="sticky bottom-0 z-10 flex flex-col border-t bg-background" data-testid="plan-totals">
      {TOTALS_ROWS.map((spec) => (
        <div key={spec.key} role="row" className="grid items-center gap-1 px-2 py-1" style={{ gridTemplateColumns: gridCols }}>
          <span className="truncate text-xs font-medium text-muted-foreground">{t(`budgets.page.plan.totals.${spec.key}`)}</span>
          {visibleMonths.map((m) => {
            const idx = monthIndex(m)
            const row = idx >= 0 ? totals[idx] : undefined
            return (
              <div key={m} className={`flex flex-col items-end px-2 py-1 ${m === cur ? 'bg-accent/40' : ''}`}>
                <span className="text-xs text-muted-foreground">
                  {row ? moneyFormat(spec.actual(row), currency, { showCurrency: false, useNativePrecision: false }) : '—'}
                </span>
                <span className="text-sm">
                  {row ? moneyFormat(spec.planned(row), currency, { showCurrency: false, useNativePrecision: false }) : '—'}
                </span>
              </div>
            )
          })}
        </div>
      ))}
      <div role="row" className="grid items-center gap-1 border-t px-2 py-1.5 font-semibold" style={{ gridTemplateColumns: gridCols }}>
        <span className="truncate text-xs">{t('budgets.page.plan.totals.balance')}</span>
        {visibleMonths.map((m, i) => {
          const idx = monthIndex(m)
          const value = idx >= 0 ? balance[idx] : undefined
          const negative = value !== undefined && cmp(value, '0') < 0
          return (
            <div
              key={m}
              data-testid={`plan-balance-${i}`}
              className={`px-2 py-1 text-right text-sm ${m === cur ? 'bg-accent/40' : ''} ${negative ? 'text-destructive' : ''}`}
            >
              {value !== undefined ? moneyFormat(value, currency, { showCurrency: false, useNativePrecision: false }) : '—'}
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function PlanSheet({ budget, currencies, userId }: PlanSheetProps) {
  const { t, i18n } = useTranslation()
  const isCompact = useIsCompact()
  const [planLimitTarget, setPlanLimitTarget] = useState<PlanLimitTarget | null>(null)
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {
    const el = containerRef.current
    if (!el) {
      return
    }
    const ro = new ResizeObserver((entries) => setWidth(entries[0]?.contentRect.width ?? 0))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  // ResizeObserver never fires in jsdom, so width stays 0 there — the same
  // floor a real narrow viewport would collapse to (planVisibleCount<3 -> 1).
  const visible = width > 0 ? planVisibleCount(width) : 3

  const startedAt = budget.meta.startedAt
  const persisted = useBudgetPeriodStore((s) => s.planFirstMonth)
  const setPlanFirstMonth = useBudgetPeriodStore((s) => s.setPlanFirstMonth)
  const hideEmpty = useBudgetPeriodStore((s) => s.planHideEmpty)
  const firstMonth = clampFirstMonth(persisted ?? planInitialFirstMonth(null, startedAt, visible), startedAt)

  const { data: plan, isPending, planKey } = useBudgetPlan(budget.meta.id, firstMonth, visible)
  const setLimit = usePlanSetLimit(planKey)
  const commit = (elementId: Id, month: string, monthIndex: number, amount: string | null) =>
    setLimit.mutate({ budgetId: budget.meta.id, elementId, period: month, amount, monthIndex })

  const visibleMonths = Array.from({ length: visible }, (_, i) => addMonths(firstMonth, i))
  const monthIndex = (m: string): number => (plan ? plan.months.indexOf(m) : -1)

  const rows = useMemo(() => (plan ? bucketPlanRows(plan, hideEmpty) : null), [plan, hideEmpty])

  if (!plan || !rows) {
    return isPending ? (
      <div className="flex flex-1 items-center justify-center">
        <CoinLoader label={t('common.app.modal.loading.data_loading')} />
      </div>
    ) : null
  }

  const cur = currentMonth()
  const monthFmt = new Intl.DateTimeFormat(i18n.language, { month: 'short', year: '2-digit' })
  const atStart = firstMonth <= startedAt.slice(0, 7) + '-01'
  const gridCols = `${PLAN_NAME_COL_PX}px repeat(${visible}, minmax(${PLAN_MIN_MONTH_COL_PX}px, 1fr))`
  const ex = makePlanExchange(plan, currencies)
  const totals = planTotals(plan, ex)
  const balance = balanceRow(plan, totals, ex)
  const planCurrency = currencies.find((c) => c.id === plan.meta.currencyId)
  const ctx: GridCtx = {
    visibleMonths,
    monthIndex,
    cur,
    currencies,
    gridCols,
    meta: budget.meta,
    userId,
    isCompact,
    commit,
    openDialog: setPlanLimitTarget,
  }
  const dialogCell = planLimitTarget ? planLimitTarget.el.cells[planLimitTarget.monthIndex] : undefined

  return (
    <div ref={containerRef} className="flex min-h-0 flex-1 flex-col overflow-y-auto" data-testid="plan-sheet">
      <div className="sticky top-0 z-10 grid items-center bg-background" style={{ gridTemplateColumns: gridCols }}>
        <div className="flex items-center gap-1 px-2">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('budgets.page.plan.nav.prev')}
            disabled={atStart}
            onClick={() => setPlanFirstMonth(addMonths(firstMonth, -1))}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('budgets.page.plan.nav.next')}
            onClick={() => setPlanFirstMonth(addMonths(firstMonth, 1))}
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
        {visibleMonths.map((m) => (
          <div
            key={m}
            data-month={m}
            className={`px-2 py-1 text-right text-xs uppercase tracking-wide ${m === cur ? 'font-bold text-foreground' : 'text-muted-foreground'}`}
          >
            {monthFmt.format(new Date(m))}
          </div>
        ))}
      </div>

      <section data-testid="plan-section-income" className="flex flex-col gap-1 px-1 py-1">
        <SectionHeader label={t('budgets.page.plan.section.income')} />
        {rows.income.folders.map((f) => (
          <FolderRows key={f.folder.id} section={f} ctx={ctx} />
        ))}
        {rows.income.loose.map((r) => (
          <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
        ))}
        {rows.income.uncategorized ? <ElementRow key={rowKey(rows.income.uncategorized)} row={rows.income.uncategorized} ctx={ctx} /> : null}
      </section>

      <section data-testid="plan-section-expense" className="flex flex-col gap-1 px-1 py-1">
        {rows.expense.folders.map((f) => (
          <FolderRows key={f.folder.id} section={f} ctx={ctx} />
        ))}
        {rows.expense.loose.map((r) => (
          <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
        ))}
        {rows.expense.uncategorized ? <ElementRow key={rowKey(rows.expense.uncategorized)} row={rows.expense.uncategorized} ctx={ctx} /> : null}
      </section>

      {rows.archived.length > 0 ? (
        <section data-testid="plan-section-archived" className="flex flex-col gap-1 px-1 py-1">
          <SectionHeader label={t('budgets.page.plan.section.archived')} />
          {rows.archived.map((r) => (
            <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
          ))}
        </section>
      ) : null}

      <TotalsFooter
        visibleMonths={visibleMonths}
        monthIndex={monthIndex}
        cur={cur}
        gridCols={gridCols}
        totals={totals}
        balance={balance}
        currency={planCurrency}
      />

      <SetLimitDialog
        target={
          planLimitTarget
            ? {
                id: planLimitTarget.el.id,
                name: elementDisplayName(planLimitTarget.el.id, planLimitTarget.el.name, t),
                value: dialogCell && dialogCell.planned !== '' ? dialogCell.planned : '0',
              }
            : null
        }
        onClose={() => setPlanLimitTarget(null)}
        onCommit={(elementId, amount) => {
          if (planLimitTarget) {
            commit(elementId, planLimitTarget.month, planLimitTarget.monthIndex, amount)
          }
        }}
      />
    </div>
  )
}
