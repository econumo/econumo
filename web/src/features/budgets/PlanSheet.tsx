import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronLeft, ChevronRight, Plus } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
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
import {
  canConfigureBudget,
  canEditBudget,
  canUpdateLimits,
  useBudgetPlan,
  useCreateEnvelope,
  useFillPlannedCells,
  usePlanSetLimit,
  useUpdateEnvelope,
} from './queries'
import { LimitEditor } from './LimitEditor'
import { SetLimitDialog } from './SetLimitDialog'
import { EnvelopeDialog } from './EnvelopeDialog'
import {
  PLAN_MIN_MONTH_COL_PX,
  PLAN_NAME_COL_PX,
  addMonths,
  balanceRow,
  bucketPlanRows,
  clampFirstMonth,
  currentMonth,
  fillTargetCol,
  makePlanExchange,
  monthDate,
  planInitialFirstMonth,
  planTotals,
  planVisibleCount,
  visibleSectionRows,
} from './planMath'
import type { MonthExchange, PlanFolderSection, PlanMonthTotals, PlanRow, PlanRows } from './planMath'

export interface PlanSheetProps {
  /** the ALREADY-LOADED budget (meta for permissions/currency); plan data is fetched inside */
  budget: BudgetDto
  currencies: CurrencyDto[]
  userId: Id | undefined
}

const rowKey = (r: PlanRow): string => `${r.element.id}:${r.element.type}`

// A stable DOM id per gridcell, used by aria-activedescendant. rowKey already embeds
// a ':' (id:type), which is valid in an HTML id but not worth relying on downstream, so
// it's sanitized to a safe character set.
const cellDomId = (rk: string, col: number): string => `plan-cell-${rk.replace(/[^a-zA-Z0-9_-]/g, '_')}-${col}`

const SELECTED_RING = ' ring-2 ring-ring rounded-sm'
const selectedClass = (selected: boolean): string => (selected ? SELECTED_RING : '')

// Radix portals render popover/dialog/drawer/dropdown-menu content outside the grid's
// DOM subtree, but React re-dispatches both keyboard AND click events through the
// component tree (portals are still React children), so they still reach the cell's
// onClick/the grid's onKeyDown.
//
// The keydown guard: a keystroke typed inside an open editor/menu must not be hijacked
// by grid navigation (Enter closing the popover without committing, ArrowLeft not
// moving the caret, ArrowDown moving the grid selection instead of a menu highlight).
const KEYDOWN_ESCAPE_SELECTOR =
  'input, textarea, [data-slot="popover-content"], [data-slot="dialog-content"], [data-slot="drawer-content"], [data-slot="dropdown-menu-content"]'
// The click guard (F2): a cell click that bubbles up from a nested interactive control —
// the LimitEditor trigger, a still-open popover's input, an open dropdown menu's items —
// must not steal focus back onto the grid; those controls already manage their own
// focus, and the grid regains it naturally once they close.
const CLICK_ESCAPE_SELECTOR = `button, [role="button"], ${KEYDOWN_ESCAPE_SELECTOR}`

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

/** roving grid selection: col -1 = the row's name cell, 0..visible-1 = month cells.
 *  -1 is the leftmost reachable column: ArrowRight there goes to 0, ArrowLeft at 0
 *  goes to -1, and ArrowLeft AT -1 shifts the window back a month (selection stays
 *  at -1) — the name cell is always reachable by keyboard alone. */
export interface PlanSelection {
  rowKey: string
  col: number
}

/** Excel-style fill-right drag state: startCol is the source column (the value being
 *  copied), targetCol the column the pointer currently covers (>= startCol). */
interface FillDrag {
  rowKey: string
  elementId: Id
  amount: string
  startCol: number
  targetCol: number
  startX: number
  colWidth: number
}

/** A row is editable per-cell only for a non-uncategorized, non-archived parent row,
 *  with a fetched cell and update rights for that month — children never carry limits. */
function isEditableCell(el: PlanElementDto, month: string, monthIndex: number, meta: BudgetMetaDto, userId: Id | undefined): boolean {
  return el.id !== UNCATEGORIZED_ID && el.isArchived === 0 && monthIndex >= 0 && canUpdateLimits(meta, userId, month)
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
  monthFmt: Intl.DateTimeFormat
  commit: (elementId: Id, month: string, monthIndex: number, amount: string | null) => void
  openDialog: (target: PlanLimitTarget) => void
  canEdit: boolean
  selection: PlanSelection | null
  select: (rowKey: string, col: number, e?: { target: EventTarget | null }) => void
  fill: {
    active: { rowKey: string; startCol: number; targetCol: number } | null
    start: (rowKey: string, el: PlanElementDto, col: number, e: ReactPointerEvent<HTMLElement>) => void
    move: (e: ReactPointerEvent<HTMLElement>) => void
    end: () => void
    cancel: () => void
  }
}

const ChildRow = memo(function ChildRow({
  child,
  parentCurrency,
  ctx,
}: {
  child: PlanChildDto
  parentCurrency: CurrencyDto | undefined
  ctx: GridCtx
}) {
  const { t } = useTranslation()
  const displayName = elementDisplayName(child.id, child.name, t)
  const rk = `${child.id}:${child.type}`
  const nameSelected = ctx.selection?.rowKey === rk && ctx.selection.col === -1
  return (
    <div
      role="row"
      data-row-id={rk}
      className="plan-row grid items-center gap-1 py-1 pr-2 pl-9 text-xs text-muted-foreground"
      style={{ gridTemplateColumns: ctx.gridCols }}
    >
      <span
        role="gridcell"
        id={cellDomId(rk, -1)}
        aria-selected={nameSelected}
        className={`flex min-w-0 items-center gap-1.5 truncate${selectedClass(nameSelected)}`}
        title={displayName}
        onClick={(e) => ctx.select(rk, -1, e)}
      >
        <EntityIcon name={child.icon} className="text-base" />
        <span className="truncate">{displayName}</span>
      </span>
      {ctx.visibleMonths.map((m, i) => {
        const idx = ctx.monthIndex(m)
        const cell = idx >= 0 ? child.cells[idx] : undefined
        const actualText = renderActual(cell?.actual, m, ctx.cur, parentCurrency)
        const selected = ctx.selection?.rowKey === rk && ctx.selection.col === i
        return (
          <div
            key={m}
            role="gridcell"
            id={cellDomId(rk, i)}
            aria-selected={selected}
            aria-label={t('budgets.page.plan.cell.aria', { name: displayName, month: ctx.monthFmt.format(monthDate(m)), actual: actualText, planned: '—' })}
            data-month={m}
            data-col={i}
            data-testid={`plan-cell-${child.id}:${i}`}
            className={`flex items-end justify-end px-2 py-1 ${m === ctx.cur ? 'bg-accent/40' : ''}${selectedClass(selected)}`}
            onClick={(e) => ctx.select(rk, i, e)}
          >
            <span data-testid="cell-actual">{actualText}</span>
          </div>
        )
      })}
    </div>
  )
})

const ElementRow = memo(function ElementRow({ row, ctx }: { row: PlanRow; ctx: GridCtx }) {
  const { t } = useTranslation()
  const el = row.element
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[el.id])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const currency = ctx.currencies.find((c) => c.id === el.currencyId)
  const displayName = elementDisplayName(el.id, el.name, t)
  const isUncategorized = el.id === UNCATEGORIZED_ID
  const expandable = el.children.length > 0
  const Chevron = unfolded ? ChevronDown : ChevronRight
  const rk = `${el.id}:${el.type}`
  const nameSelected = ctx.selection?.rowKey === rk && ctx.selection.col === -1

  const name = (
    <>
      <EntityIcon name={el.icon} className="text-lg text-muted-foreground" />
      <span className="truncate text-sm" title={displayName}>
        {displayName}
      </span>
    </>
  )

  return (
    <div data-row-id={rk} className="border-b border-border/60">
      <div
        role="row"
        className="plan-row grid items-center gap-1 px-2 py-1.5"
        style={{ gridTemplateColumns: ctx.gridCols }}
      >
        <div
          role="gridcell"
          id={cellDomId(rk, -1)}
          aria-selected={nameSelected}
          className={`flex min-w-0 items-center gap-1${selectedClass(nameSelected)}`}
          onClick={(e) => ctx.select(rk, -1, e)}
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
        </div>
        {ctx.visibleMonths.map((m, i) => {
          const idx = ctx.monthIndex(m)
          const cell = idx >= 0 ? el.cells[idx] : undefined
          const editable = isEditableCell(el, m, idx, ctx.meta, ctx.userId)
          // overspend highlight: expense side only, current/future months, a set plan the actual has already cleared
          const overspend =
            !isIncomeType(el.type) && m >= ctx.cur && !!cell && cell.planned !== '' && cmp(cell.actual, cell.planned) > 0
          const plannedValue = cell && cell.planned !== '' ? cell.planned : '0'
          const plannedText = cell && cell.planned !== '' ? moneyFormat(cell.planned, currency, { showCurrency: false, useNativePrecision: false }) : '—'
          const actualText = renderActual(cell?.actual, m, ctx.cur, currency)
          const selected = ctx.selection?.rowKey === rk && ctx.selection.col === i
          const filled = ctx.fill.active?.rowKey === rk && i > ctx.fill.active.startCol && i <= ctx.fill.active.targetCol
          const showFillHandle = selected && editable && !!cell && cell.planned !== '' && !ctx.isCompact && ctx.visibleMonths.length > 1
          return (
            <div
              key={m}
              role="gridcell"
              id={cellDomId(rk, i)}
              aria-selected={selected}
              aria-label={t('budgets.page.plan.cell.aria', { name: displayName, month: ctx.monthFmt.format(monthDate(m)), actual: actualText, planned: plannedText })}
              data-month={m}
              data-col={i}
              data-testid={`plan-cell-${el.id}:${i}`}
              className={`relative flex flex-col items-end px-2 py-1 ${m === ctx.cur ? 'bg-accent/40' : ''}${selectedClass(selected)}${filled ? ' fill-covered bg-ring/15' : ''}`}
              onClick={(e) => ctx.select(rk, i, e)}
            >
              <span data-testid="cell-actual" className={`text-xs ${overspend ? 'text-destructive' : 'text-muted-foreground'}`}>
                {actualText}
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
              {showFillHandle ? (
                <span
                  data-testid="fill-handle"
                  role="button"
                  aria-label={t('budgets.page.plan.fill.handle_aria')}
                  className="absolute -right-0.5 -bottom-0.5 z-10 size-2 touch-none cursor-crosshair rounded-[1px] border border-background bg-ring"
                  onPointerDown={(e) => ctx.fill.start(rk, el, i, e)}
                  onPointerMove={(e) => ctx.fill.move(e)}
                  onPointerUp={() => ctx.fill.end()}
                  onPointerCancel={() => ctx.fill.cancel()}
                  onClick={(e) => e.stopPropagation()}
                />
              ) : null}
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
})

function HiddenRowsNotice({ count, onShow }: { count: number; onShow: () => void }) {
  const { t } = useTranslation()
  if (count <= 0) {
    return null
  }
  return (
    <span className="flex shrink-0 items-center gap-1 text-[11px] font-normal normal-case tracking-normal text-muted-foreground">
      {t('budgets.page.plan.density.hidden', { count })}
      <button type="button" className="underline-offset-2 hover:underline" onClick={onShow}>
        {t('budgets.page.plan.density.show')}
      </button>
    </span>
  )
}

function SectionHeader({
  label,
  foldKey,
  folded,
  onToggleFold,
  hiddenCount,
  onShow,
  action,
}: {
  label: string
  foldKey: string
  folded: boolean
  onToggleFold: (key: string) => void
  hiddenCount: number
  onShow: () => void
  action?: ReactNode
}) {
  const { t } = useTranslation()
  const Chevron = folded ? ChevronRight : ChevronDown
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-2 px-2 pt-2 pb-1">
      <button
        type="button"
        className="flex items-center gap-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground"
        aria-expanded={!folded}
        title={t(folded ? 'common.button.expand.label' : 'common.button.collapse.label')}
        onClick={() => onToggleFold(foldKey)}
      >
        <Chevron className="size-3.5 shrink-0" />
        {label}
      </button>
      <span className="flex items-center gap-1.5">
        {action}
        <HiddenRowsNotice count={hiddenCount} onShow={onShow} />
      </span>
    </div>
  )
}

// A folder with zero members (neutral, per folderSides) still renders — header only,
// no rows — same as the budget page: a folder only disappears if it doesn't exist, not
// because it currently has no side. section.rows is always the FULL unfiltered member
// list here (PlanSheet always calls bucketPlanRows(plan, false)), so an empty list means
// genuinely no members, not "everything hidden by the density toggle".
function FolderRows({
  section,
  ctx,
  hideEmpty,
  folded,
  revealed,
  onToggleFold,
  onReveal,
}: {
  section: PlanFolderSection
  ctx: GridCtx
  hideEmpty: boolean
  folded: boolean
  revealed: boolean
  onToggleFold: (key: string) => void
  onReveal: () => void
}) {
  const { t } = useTranslation()
  const visibleRows = visibleSectionRows(section.rows, folded, hideEmpty, revealed)
  const hiddenCount = !folded && hideEmpty && !revealed ? section.rows.filter((r) => r.hidden).length : 0
  const Chevron = folded ? ChevronRight : ChevronDown
  return (
    <div className="mb-1 rounded-md border p-1.5" data-testid={`plan-folder-${section.folder.id}`}>
      <div className="flex flex-wrap items-center justify-between gap-x-2 pb-1">
        <button
          type="button"
          className="flex min-w-0 items-center gap-1.5 truncate px-1.5 text-sm font-medium"
          aria-expanded={!folded}
          title={t(folded ? 'common.button.expand.label' : 'common.button.collapse.label')}
          onClick={() => onToggleFold(section.folder.id)}
        >
          <Chevron className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate">{section.folder.name}</span>
        </button>
        <HiddenRowsNotice count={hiddenCount} onShow={onReveal} />
      </div>
      {visibleRows.map((r) => (
        <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
      ))}
    </div>
  )
}

interface TotalsRowSpec {
  key: 'income' | 'expenses' | 'net'
  value: (t: PlanMonthTotals) => string
}

const TOTALS_ROWS: TotalsRowSpec[] = [
  { key: 'income', value: (t) => t.effectiveIncome },
  { key: 'expenses', value: (t) => t.effectiveExpense },
  { key: 'net', value: (t) => t.effectiveNet },
]

function PlanTotals({
  visibleMonths,
  monthIndex,
  cur,
  gridCols,
  totals,
  currency,
}: {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  cur: string
  gridCols: string
  totals: PlanMonthTotals[]
  currency: CurrencyDto | undefined
}) {
  const { t } = useTranslation()
  return (
    <div role="rowgroup" className="mt-2 flex flex-col border-t" data-testid="plan-totals">
      {TOTALS_ROWS.map((spec) => (
        <div key={spec.key} role="row" className="grid items-center gap-1 px-2 py-1" style={{ gridTemplateColumns: gridCols }}>
          <span className="truncate text-xs font-medium text-muted-foreground">{t(`budgets.page.plan.totals.${spec.key}`)}</span>
          {visibleMonths.map((m, i) => {
            const idx = monthIndex(m)
            const row = idx >= 0 ? totals[idx] : undefined
            return (
              <div key={m} data-col={i} className={`flex items-center justify-end px-2 py-1 ${m === cur ? 'bg-accent/40' : ''}`}>
                <span className="text-sm">
                  {row ? moneyFormat(spec.value(row), currency, { showCurrency: false, useNativePrecision: false }) : '—'}
                </span>
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}

function PlanBalanceRow({
  visibleMonths,
  monthIndex,
  cur,
  gridCols,
  balance,
  currency,
}: {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  cur: string
  gridCols: string
  balance: string[]
  currency: CurrencyDto | undefined
}) {
  const { t } = useTranslation()
  return (
    <div
      role="rowgroup"
      className="sticky bottom-0 z-10 border-t bg-background"
      data-testid="plan-balance-row"
    >
      <div role="row" className="grid items-center gap-1 px-2 py-1.5 font-semibold" style={{ gridTemplateColumns: gridCols }}>
        <span className="truncate text-xs">{t('budgets.page.plan.totals.balance')}</span>
        {visibleMonths.map((m, i) => {
          const idx = monthIndex(m)
          const value = idx >= 0 ? balance[idx] : undefined
          const negative = value !== undefined && cmp(value, '0') < 0
          return (
            <div
              key={m}
              data-col={i}
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

interface EnvelopeDialogState {
  open: boolean
  envelope: PlanElementDto | null
  folderId: Id | null
  side: 'expense' | 'income'
}

const CLOSED_ENVELOPE_DIALOG: EnvelopeDialogState = { open: false, envelope: null, folderId: null, side: 'expense' }

// Same flattening the renderer walks (folders -> loose -> uncategorized, income then
// expense, then archived), so Up/Down can never reach a row that isn't on screen.
interface FlatRow {
  rowKey: string
  el: PlanElementDto
  child?: PlanChildDto
}

function buildFlatRows(
  rows: PlanRows,
  unfoldedElements: Record<string, boolean>,
  hideEmpty: boolean,
  revealedSections: Set<string>,
  folded: (key: string) => boolean,
): FlatRow[] {
  const flatRows: FlatRow[] = []
  const pushRow = (r: PlanRow) => {
    flatRows.push({ rowKey: rowKey(r), el: r.element })
    if (r.element.children.length > 0 && unfoldedElements[r.element.id]) {
      for (const child of r.element.children) {
        flatRows.push({ rowKey: `${child.id}:${child.type}`, el: r.element, child })
      }
    }
  }
  const incomeFolded = folded('income')
  if (!incomeFolded) {
    for (const f of rows.income.folders) {
      visibleSectionRows(f.rows, folded(f.folder.id), hideEmpty, revealedSections.has(f.folder.id)).forEach(pushRow)
    }
    visibleSectionRows(rows.income.loose, incomeFolded, hideEmpty, revealedSections.has('income')).forEach(pushRow)
    if (rows.income.uncategorized) {
      pushRow(rows.income.uncategorized)
    }
  }
  const expenseFolded = folded('expense')
  if (!expenseFolded) {
    for (const f of rows.expense.folders) {
      visibleSectionRows(f.rows, folded(f.folder.id), hideEmpty, revealedSections.has(f.folder.id)).forEach(pushRow)
    }
    visibleSectionRows(rows.expense.loose, expenseFolded, hideEmpty, revealedSections.has('expense')).forEach(pushRow)
    if (rows.expense.uncategorized) {
      pushRow(rows.expense.uncategorized)
    }
  }
  if (rows.archived.length > 0 && !folded('archived')) {
    rows.archived.forEach(pushRow)
  }
  return flatRows
}

export function PlanSheet({ budget, currencies, userId }: PlanSheetProps) {
  const { t, i18n } = useTranslation()
  const isCompact = useIsCompact()
  const [planLimitTarget, setPlanLimitTarget] = useState<PlanLimitTarget | null>(null)
  const [revealedSections, setRevealedSections] = useState<Set<string>>(new Set())
  const revealSection = (key: string) => setRevealedSections((prev) => new Set(prev).add(key))
  const [envelopeDialog, setEnvelopeDialog] = useState<EnvelopeDialogState>(CLOSED_ENVELOPE_DIALOG)
  const closeEnvelopeDialog = () => setEnvelopeDialog(CLOSED_ENVELOPE_DIALOG)
  const createEnvelope = useCreateEnvelope()
  const updateEnvelope = useUpdateEnvelope()
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
  const planFolds = useBudgetPeriodStore((s) => s.planFolds)
  const togglePlanFold = useBudgetPeriodStore((s) => s.togglePlanFold)
  const folded = useCallback((key: string): boolean => !!planFolds[key], [planFolds])
  const unfoldedElements = useBudgetPeriodStore((s) => s.unfoldedElements)
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const [selection, setSelection] = useState<PlanSelection | null>(null)
  const [fillDrag, setFillDrag] = useState<FillDrag | null>(null)
  const [hoverCol, setHoverCol] = useState<number | null>(null)
  // One listener on the container instead of per-cell handlers. The rows survive
  // a hover because `ctx` deliberately excludes hoverCol and the row components
  // are memo'd — keep hover-derived values OUT of ctx or every pointer move
  // re-renders the whole grid.
  const handleHover = useCallback((e: ReactMouseEvent<HTMLDivElement>) => {
    const cell = (e.target as HTMLElement).closest('[data-col]')
    const raw = cell?.getAttribute('data-col')
    setHoverCol(raw === null || raw === undefined ? null : Number(raw))
  }, [])
  const firstMonth = clampFirstMonth(persisted ?? planInitialFirstMonth(null, startedAt, visible), startedAt)
  const atStart = firstMonth <= startedAt.slice(0, 7) + '-01'

  const { data: plan, isPending, isError, refetch, planKey } = useBudgetPlan(budget.meta.id, firstMonth, visible)
  const setLimit = usePlanSetLimit(planKey)
  const fillCells = useFillPlannedCells(planKey)

  // clicking a cell must land keyboard focus on the grid too, or the arrow keys that
  // follow a click are dead until the user tabs in manually (F2). A cell click also
  // bubbles from any interactive descendant it contains (the LimitEditor popover
  // trigger, the row-actions dropdown trigger, the expand chevron) — grabbing grid
  // focus THEN would yank focus straight back out of the thing that click just opened,
  // so that path is skipped; those controls manage their own focus already, and the
  // grid regains it naturally once they close.
  const select = useCallback((rk: string, col: number, e?: { target: EventTarget | null }) => {
    setSelection({ rowKey: rk, col })
    const target = e?.target as HTMLElement | null | undefined
    if (!target?.closest(CLICK_ESCAPE_SELECTOR)) {
      containerRef.current?.focus()
    }
  }, [])

  const commit = useCallback(
    (elementId: Id, month: string, monthIndex: number, amount: string | null) =>
      setLimit.mutate({ budgetId: budget.meta.id, elementId, period: month, amount, monthIndex }),
    [setLimit, budget.meta.id],
  )

  const visibleMonths = useMemo(() => Array.from({ length: visible }, (_, i) => addMonths(firstMonth, i)), [visible, firstMonth])
  const monthIndex = useCallback((m: string): number => (plan ? plan.months.indexOf(m) : -1), [plan])
  const cur = currentMonth()
  const monthFmt = useMemo(() => new Intl.DateTimeFormat(i18n.language, { month: 'short', year: '2-digit' }), [i18n.language])
  const gridCols = `${PLAN_NAME_COL_PX}px repeat(${visible}, minmax(${PLAN_MIN_MONTH_COL_PX}px, 1fr))`
  const canEdit = canEditBudget(budget.meta, userId)
  const configure = canConfigureBudget(budget.meta, userId)

  const fillStart = useCallback(
    (rk: string, el: PlanElementDto, col: number, e: ReactPointerEvent<HTMLElement>) => {
      const colWidth = (e.currentTarget.closest('[role="gridcell"]') as HTMLElement | null)?.getBoundingClientRect().width ?? 0
      e.currentTarget.setPointerCapture(e.pointerId)
      e.preventDefault()
      // no stopPropagation here: the native pointerdown must still reach Radix's
      // document-level DismissableLayer listener, so an open LimitEditor popover
      // on this cell dismisses through the fill drag, same as any other outside
      // click. Cell-selection clicks are guarded separately by the handle's own
      // onClick stopPropagation below.
      const month = visibleMonths[col]
      const idx = month !== undefined ? monthIndex(month) : -1
      const amount = idx >= 0 ? (el.cells[idx]?.planned ?? '') : ''
      setFillDrag({ rowKey: rk, elementId: el.id, amount, startCol: col, targetCol: col, startX: e.clientX, colWidth })
    },
    [visibleMonths, monthIndex],
  )

  const fillMove = useCallback(
    (e: ReactPointerEvent<HTMLElement>) => {
      if (!fillDrag) {
        return
      }
      setFillDrag({ ...fillDrag, targetCol: fillTargetCol(fillDrag.startCol, e.clientX - fillDrag.startX, fillDrag.colWidth, visible - 1) })
    },
    [fillDrag, visible],
  )

  const fillEnd = useCallback(() => {
    if (!fillDrag) {
      return
    }
    if (fillDrag.targetCol > fillDrag.startCol) {
      const targets: { period: string; monthIndex: number }[] = []
      for (let c = fillDrag.startCol + 1; c <= fillDrag.targetCol; c++) {
        const period = visibleMonths[c]
        const idx = period !== undefined ? monthIndex(period) : -1
        if (period !== undefined && idx >= 0) {
          targets.push({ period, monthIndex: idx })
        }
      }
      fillCells.mutate({ budgetId: budget.meta.id, elementId: fillDrag.elementId, amount: fillDrag.amount, targets })
    }
    setFillDrag(null)
  }, [fillDrag, visibleMonths, monthIndex, fillCells, budget.meta.id])

  const fillCancel = useCallback(() => setFillDrag(null), [])

  // always the unfiltered structure (per-row `hidden` flags, nothing dropped) — hideEmpty
  // is applied per SECTION at render time so each header's reveal is independent
  const rows = useMemo(() => (plan ? bucketPlanRows(plan, false) : null), [plan])

  // An uncategorized row is dropped when every VISIBLE column's actual is zero — it
  // can still have spend outside the current window, so it reappears once navigation
  // brings that month into view. Derived once here (not per-section, not in
  // buildFlatRows) so the render sections and the keyboard flat-row list can never
  // disagree on which rows are on screen.
  const shownRows = useMemo(() => {
    if (!rows) {
      return null
    }
    const visibleUncat = (r: PlanRow | null): PlanRow | null =>
      r && visibleMonths.some((m) => {
        const i = monthIndex(m)
        return i >= 0 && !isZero(r.element.cells[i]?.actual ?? '0')
      })
        ? r
        : null
    return {
      ...rows,
      income: { ...rows.income, uncategorized: visibleUncat(rows.income.uncategorized) },
      expense: { ...rows.expense, uncategorized: visibleUncat(rows.expense.uncategorized) },
    }
  }, [rows, visibleMonths, monthIndex])

  // the expensive per-render work the arrow keys used to re-trigger on every keystroke:
  // FX conversion, totals, running balance, and the flattened keyboard-nav row list all
  // depend on the plan/currencies/fold state, never on `selection` — memoizing them means
  // moving the cursor no longer recomputes any of this (F7)
  const ex: MonthExchange | null = useMemo(() => (plan ? makePlanExchange(plan, currencies) : null), [plan, currencies])
  const totals = useMemo(() => (plan && ex ? planTotals(plan, ex) : []), [plan, ex])
  const balance = useMemo(() => (plan && ex ? balanceRow(plan, totals, ex) : []), [plan, ex, totals])
  const flatRows = useMemo(
    () => (shownRows ? buildFlatRows(shownRows, unfoldedElements, hideEmpty, revealedSections, folded) : []),
    [shownRows, unfoldedElements, hideEmpty, revealedSections, folded],
  )

  const ctx: GridCtx | null = useMemo(() => {
    if (!plan) {
      return null
    }
    return {
      visibleMonths,
      monthIndex,
      cur,
      currencies,
      gridCols,
      meta: budget.meta,
      userId,
      isCompact,
      monthFmt,
      commit,
      openDialog: setPlanLimitTarget,
      canEdit,
      selection,
      select,
      fill: {
        active: fillDrag ? { rowKey: fillDrag.rowKey, startCol: fillDrag.startCol, targetCol: fillDrag.targetCol } : null,
        start: fillStart,
        move: fillMove,
        end: fillEnd,
        cancel: fillCancel,
      },
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    plan,
    visibleMonths,
    monthIndex,
    cur,
    currencies,
    gridCols,
    budget.meta,
    userId,
    isCompact,
    monthFmt,
    commit,
    canEdit,
    selection,
    select,
    fillDrag,
    fillStart,
    fillMove,
    fillEnd,
    fillCancel,
  ])

  if (!plan || !shownRows || !ctx || !ex) {
    if (isError) {
      return (
        <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center" data-testid="plan-error">
          <p className="max-w-md text-sm text-muted-foreground">{t('common.app.error')}</p>
          <Button type="button" onClick={() => void refetch()}>
            {t('budgets.page.budget.error.retry')}
          </Button>
        </div>
      )
    }
    return isPending ? (
      <div className="flex flex-1 items-center justify-center">
        <CoinLoader label={t('common.app.modal.loading.data_loading')} />
      </div>
    ) : null
  }

  const planCurrency = currencies.find((c) => c.id === plan.meta.currencyId)
  const dialogCell = planLimitTarget ? planLimitTarget.el.cells[planLimitTarget.monthIndex] : undefined

  // per-section hide-empty: a row belongs to exactly one bucket — a folder, or its
  // side's loose rows — so each bucket's count/reveal is independent of the others
  const sectionHiddenCount = (sectionRows: PlanRow[], revealed: boolean) =>
    hideEmpty && !revealed ? sectionRows.filter((r) => r.hidden).length : 0

  const incomeFolded = folded('income')
  const incomeRevealed = revealedSections.has('income')
  const incomeLoose = visibleSectionRows(shownRows.income.loose, incomeFolded, hideEmpty, incomeRevealed)
  const incomeHiddenCount = incomeFolded ? 0 : sectionHiddenCount(shownRows.income.loose, incomeRevealed)

  const expenseFolded = folded('expense')
  const expenseRevealed = revealedSections.has('expense')
  const expenseLoose = visibleSectionRows(shownRows.expense.loose, expenseFolded, hideEmpty, expenseRevealed)
  const expenseHiddenCount = expenseFolded ? 0 : sectionHiddenCount(shownRows.expense.loose, expenseRevealed)

  function handleEnter(entry: FlatRow, col: number) {
    if (col === -1) {
      if (!entry.child && entry.el.children.length > 0) {
        toggleElement(entry.el.id)
      }
      return
    }
    if (entry.child) {
      return
    }
    const month = visibleMonths[col]
    if (month === undefined) {
      return
    }
    const idx = monthIndex(month)
    if (!isEditableCell(entry.el, month, idx, budget.meta, userId)) {
      return
    }
    if (isCompact) {
      setPlanLimitTarget({ el: entry.el, month, monthIndex: idx })
      return
    }
    const trigger = containerRef.current?.querySelector<HTMLButtonElement>(
      `[data-testid="plan-cell-${entry.el.id}:${col}"] [aria-label^="limit "]`,
    )
    trigger?.focus()
    trigger?.click()
  }

  function handleKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (fillDrag) {
      if (e.key === 'Escape') {
        setFillDrag(null)
        return
      }
      // A fill drag is pointer-driven and modal-ish: any grid-navigation key arriving
      // mid-drag (e.g. ArrowRight before pointerup) must be a no-op, or the selection/
      // window shifts under the still-in-flight drag and desyncs the eventual commit's
      // target columns from what's visually covered.
      if (e.key === 'ArrowUp' || e.key === 'ArrowDown' || e.key === 'ArrowLeft' || e.key === 'ArrowRight' || e.key === 'Enter') {
        e.preventDefault()
      }
      return
    }
    // Radix portals render popover/dialog/drawer/dropdown-menu content outside the
    // grid's DOM subtree, but React re-dispatches the event through the component
    // tree, so it still reaches this handler. Without this guard, typing in the
    // LimitEditor popover, any dialog, or an open row-actions menu gets its
    // Arrow/Enter keys hijacked by grid navigation (Enter closing the popover
    // without committing, ArrowLeft not moving the caret, ArrowDown moving the grid
    // selection instead of the menu highlight).
    const target = e.target as HTMLElement
    if (target.closest(KEYDOWN_ESCAPE_SELECTOR)) {
      return
    }
    if (flatRows.length === 0) {
      return
    }
    const arrowKeys = ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight']
    if (!selection || !flatRows.some((r) => r.rowKey === selection.rowKey)) {
      if (arrowKeys.includes(e.key)) {
        e.preventDefault()
        setSelection({ rowKey: flatRows[0].rowKey, col: 0 })
      }
      return
    }
    const idx = flatRows.findIndex((r) => r.rowKey === selection.rowKey)
    switch (e.key) {
      case 'ArrowUp':
        e.preventDefault()
        if (idx > 0) {
          select(flatRows[idx - 1].rowKey, selection.col)
        }
        break
      case 'ArrowDown':
        e.preventDefault()
        if (idx < flatRows.length - 1) {
          select(flatRows[idx + 1].rowKey, selection.col)
        }
        break
      case 'ArrowLeft':
        e.preventDefault()
        if (selection.col === -1) {
          // -1 is the leftmost reachable column, so ArrowLeft here shifts the window
          // instead of going nowhere — keeps the name cell reachable while the window
          // can still be paged from it. Clamped at the budget start, same as the prev
          // nav button (F8).
          if (!atStart) {
            setPlanFirstMonth(addMonths(firstMonth, -1))
          }
          select(selection.rowKey, -1)
        } else if (selection.col === 0) {
          select(selection.rowKey, -1)
        } else {
          select(selection.rowKey, selection.col - 1)
        }
        break
      case 'ArrowRight':
        e.preventDefault()
        if (selection.col === -1) {
          select(selection.rowKey, 0)
        } else if (selection.col >= visible - 1) {
          setPlanFirstMonth(addMonths(firstMonth, 1))
          select(selection.rowKey, visible - 1)
        } else {
          select(selection.rowKey, selection.col + 1)
        }
        break
      case 'Enter':
        e.preventDefault()
        handleEnter(flatRows[idx], selection.col)
        break
      default:
        break
    }
  }

  return (
    // the hovered-column attribute lives here, not on the scroller: the month
    // header is the scroller's SIBLING, and the crosshair has to reach it too
    <div
      className="flex min-h-0 flex-1 flex-col"
      data-hover-col={hoverCol ?? undefined}
      onMouseOver={handleHover}
      onMouseLeave={() => setHoverCol(null)}
    >
      <div role="rowgroup">
        <div role="row" className="grid items-center bg-background" style={{ gridTemplateColumns: gridCols }}>
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
          {visibleMonths.map((m, i) => (
            <div
              key={m}
              role="columnheader"
              data-month={m}
              data-col={i}
              className={`px-2 py-1 text-right text-xs uppercase tracking-wide ${m === cur ? 'font-bold text-foreground' : 'text-muted-foreground'}`}
            >
              {monthFmt.format(monthDate(m))}
            </div>
          ))}
        </div>
      </div>

      <div
        ref={containerRef}
        role="grid"
        tabIndex={0}
        onKeyDown={handleKeyDown}
        aria-activedescendant={selection ? cellDomId(selection.rowKey, selection.col) : undefined}
        className="flex min-h-0 flex-1 flex-col overflow-y-auto"
        data-testid="plan-sheet"
      >
        <section
          role="rowgroup"
          data-testid="plan-section-income"
          className="plan-band-income flex flex-col px-1 py-1"
        >
          <SectionHeader
            label={t('budgets.page.plan.section.income')}
            foldKey="income"
            folded={incomeFolded}
            onToggleFold={togglePlanFold}
            hiddenCount={incomeHiddenCount}
            onShow={() => revealSection('income')}
            action={
              configure ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-6"
                  aria-label="create income envelope"
                  title={t('budgets.modal.create_envelope_form.header')}
                  onClick={() => setEnvelopeDialog({ open: true, envelope: null, folderId: null, side: 'income' })}
                >
                  <Plus className="size-4" />
                </Button>
              ) : null
            }
          />
          {!incomeFolded ? (
            <>
              {shownRows.income.folders.map((f) => (
                <FolderRows
                  key={f.folder.id}
                  section={f}
                  ctx={ctx}
                  hideEmpty={hideEmpty}
                  folded={folded(f.folder.id)}
                  revealed={revealedSections.has(f.folder.id)}
                  onToggleFold={togglePlanFold}
                  onReveal={() => revealSection(f.folder.id)}
                />
              ))}
              {incomeLoose.map((r) => (
                <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
              ))}
              {shownRows.income.uncategorized ? (
                <ElementRow key={rowKey(shownRows.income.uncategorized)} row={shownRows.income.uncategorized} ctx={ctx} />
              ) : null}
            </>
          ) : null}
        </section>

        <section
          role="rowgroup"
          data-testid="plan-section-expense"
          className="plan-band-expense mt-2 flex flex-col border-t-2 border-border px-1 py-1"
        >
          <SectionHeader
            label={t('budgets.page.plan.section.expenses')}
            foldKey="expense"
            folded={expenseFolded}
            onToggleFold={togglePlanFold}
            hiddenCount={expenseHiddenCount}
            onShow={() => revealSection('expense')}
          />
          {!expenseFolded ? (
            <>
              {shownRows.expense.folders.map((f) => (
                <FolderRows
                  key={f.folder.id}
                  section={f}
                  ctx={ctx}
                  hideEmpty={hideEmpty}
                  folded={folded(f.folder.id)}
                  revealed={revealedSections.has(f.folder.id)}
                  onToggleFold={togglePlanFold}
                  onReveal={() => revealSection(f.folder.id)}
                />
              ))}
              {expenseLoose.map((r) => (
                <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
              ))}
              {shownRows.expense.uncategorized ? (
                <ElementRow key={rowKey(shownRows.expense.uncategorized)} row={shownRows.expense.uncategorized} ctx={ctx} />
              ) : null}
            </>
          ) : null}
        </section>

        {shownRows.archived.length > 0 ? (
          <section role="rowgroup" data-testid="plan-section-archived" className="plan-band-archived flex flex-col gap-1 px-1 py-1">
            <SectionHeader
              label={t('budgets.page.plan.section.archived')}
              foldKey="archived"
              folded={folded('archived')}
              onToggleFold={togglePlanFold}
              hiddenCount={0}
              onShow={() => {}}
            />
            {!folded('archived')
              ? shownRows.archived.map((r) => <ElementRow key={rowKey(r)} row={r} ctx={ctx} />)
              : null}
          </section>
        ) : null}

        <PlanTotals
          visibleMonths={visibleMonths}
          monthIndex={monthIndex}
          cur={cur}
          gridCols={gridCols}
          totals={totals}
          currency={planCurrency}
        />

        <PlanBalanceRow
          visibleMonths={visibleMonths}
          monthIndex={monthIndex}
          cur={cur}
          gridCols={gridCols}
          balance={balance}
          currency={planCurrency}
        />
      </div>

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

      <EnvelopeDialog
        open={envelopeDialog.open}
        envelope={envelopeDialog.envelope}
        budgetCurrencyId={budget.meta.currencyId}
        side={envelopeDialog.side}
        onClose={closeEnvelopeDialog}
        onSubmit={(form) => {
          if (envelopeDialog.envelope) {
            updateEnvelope.mutate(
              {
                budgetId: budget.meta.id,
                id: envelopeDialog.envelope.id,
                name: form.name,
                icon: form.icon,
                currencyId: form.currencyId,
                isArchived: form.isArchived,
                categories: form.categories,
              },
              { onSuccess: closeEnvelopeDialog },
            )
          } else {
            createEnvelope.mutate(
              {
                budgetId: budget.meta.id,
                id: uuidv7(),
                name: form.name,
                icon: form.icon,
                currencyId: form.currencyId,
                folderId: envelopeDialog.folderId,
                categories: form.categories,
                ...(envelopeDialog.side === 'income' ? { side: 'income' as const } : {}),
              },
              { onSuccess: closeEnvelopeDialog },
            )
          }
        }}
      />

    </div>
  )
}
