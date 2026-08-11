import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronLeft, ChevronRight, MoreVertical, Plus } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { EntityIcon } from '@/components/EntityIcon'
import { CoinLoader } from '@/components/CoinLoader'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { BudgetDto, BudgetFolderDto, BudgetMetaDto, PlanChildDto, PlanElementDto } from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
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
  useMoveElement,
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
  folderSides,
  makePlanExchange,
  planInitialFirstMonth,
  planTotals,
  planVisibleCount,
} from './planMath'
import type { FolderSide, PlanFolderSection, PlanMonthTotals, PlanRow } from './planMath'

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
  canEdit: boolean
  onEditEnvelope: (el: PlanElementDto) => void
  onMoveToFolder: (el: PlanElementDto) => void
}

// Every non-uncategorized, non-child parent row gets this menu; envelope
// rows additionally get Edit. "Move to folder…" opens a second, side-filtered
// picker (MoveToFolderDialog) rather than a nested submenu — Radix menu
// submenus lose focus/click races under jsdom's synthetic pointer events.
function RowMenu({ el, ctx }: { el: PlanElementDto; ctx: GridCtx }) {
  const { t } = useTranslation()
  const isEnvelope = el.type === BudgetElementType.ENVELOPE || el.type === BudgetElementType.INCOME_ENVELOPE
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" className="size-5 shrink-0" aria-label={`plan row actions ${el.name}`}>
          <MoreVertical className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {isEnvelope ? (
          <DropdownMenuItem onSelect={() => ctx.onEditEnvelope(el)}>{t('common.button.edit.label')}</DropdownMenuItem>
        ) : null}
        <DropdownMenuItem onSelect={() => ctx.onMoveToFolder(el)}>{t('budgets.page.plan.menu.move_to_folder')}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// The side-filtered folder picker opened by RowMenu's "Move to folder…":
// same-side + neutral folders (per the shared folderSides derivation), plus
// "No folder". A plain list dialog, not a DropdownMenuSub (see RowMenu).
function MoveToFolderDialog({
  target,
  folders,
  folderSideMap,
  onClose,
  onPick,
}: {
  target: PlanElementDto | null
  folders: BudgetFolderDto[]
  folderSideMap: Map<Id, FolderSide>
  onClose: () => void
  onPick: (folderId: Id | null) => void
}) {
  const { t } = useTranslation()
  if (!target) {
    return null
  }
  const side: 'income' | 'expense' = isIncomeType(target.type) ? 'income' : 'expense'
  const targets = folders.filter((f) => {
    const s = folderSideMap.get(f.id) ?? 'neutral'
    return s === side || s === 'neutral'
  })
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={t('budgets.page.plan.menu.move_to_folder')}>
      <ul className="flex max-h-72 flex-col overflow-y-auto scrollbar-slim">
        {targets.map((f) => (
          <li key={f.id}>
            <button
              type="button"
              className="w-full truncate rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover"
              onClick={() => onPick(f.id)}
            >
              {f.name}
            </button>
          </li>
        ))}
        <li>
          <button type="button" className="w-full rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover" onClick={() => onPick(null)}>
            {t('budgets.page.plan.menu.no_folder')}
          </button>
        </li>
      </ul>
    </ResponsiveDialog>
  )
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
        <div className="flex min-w-0 items-center justify-between gap-1">
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
          {!isUncategorized && ctx.canEdit ? <RowMenu el={el} ctx={ctx} /> : null}
        </div>
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
  if (section.rows.length === 0) {
    return null
  }
  const showEmpty = hideEmpty && !revealed
  const visibleRows = folded ? [] : showEmpty ? section.rows.filter((r) => !r.hidden) : section.rows
  const hiddenCount = !folded && showEmpty ? section.rows.filter((r) => r.hidden).length : 0
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

interface EnvelopeDialogState {
  open: boolean
  envelope: PlanElementDto | null
  folderId: Id | null
  side: 'expense' | 'income'
}

const CLOSED_ENVELOPE_DIALOG: EnvelopeDialogState = { open: false, envelope: null, folderId: null, side: 'expense' }

export function PlanSheet({ budget, currencies, userId }: PlanSheetProps) {
  const { t, i18n } = useTranslation()
  const isCompact = useIsCompact()
  const [planLimitTarget, setPlanLimitTarget] = useState<PlanLimitTarget | null>(null)
  const [revealedSections, setRevealedSections] = useState<Set<string>>(new Set())
  const revealSection = (key: string) => setRevealedSections((prev) => new Set(prev).add(key))
  const [envelopeDialog, setEnvelopeDialog] = useState<EnvelopeDialogState>(CLOSED_ENVELOPE_DIALOG)
  const closeEnvelopeDialog = () => setEnvelopeDialog(CLOSED_ENVELOPE_DIALOG)
  const [moveFolderTarget, setMoveFolderTarget] = useState<PlanElementDto | null>(null)
  const createEnvelope = useCreateEnvelope()
  const updateEnvelope = useUpdateEnvelope()
  const moveElement = useMoveElement()
  const pickFolder = (folderId: Id | null) => {
    if (moveFolderTarget) {
      // position only drives the local drag preview elsewhere; plan-view moves ignore it
      // and rely on the invalidated refetch (useMoveElement extends useInvalidateBudget)
      moveElement.mutate({ budgetId: budget.meta.id, item: { id: moveFolderTarget.id, folderId, position: 0, afterId: null } })
    }
    setMoveFolderTarget(null)
  }
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
  const togglePlanHideEmpty = useBudgetPeriodStore((s) => s.togglePlanHideEmpty)
  const planFolds = useBudgetPeriodStore((s) => s.planFolds)
  const togglePlanFold = useBudgetPeriodStore((s) => s.togglePlanFold)
  const folded = (key: string): boolean => !!planFolds[key]
  const firstMonth = clampFirstMonth(persisted ?? planInitialFirstMonth(null, startedAt, visible), startedAt)

  const { data: plan, isPending, planKey } = useBudgetPlan(budget.meta.id, firstMonth, visible)
  const setLimit = usePlanSetLimit(planKey)
  const commit = (elementId: Id, month: string, monthIndex: number, amount: string | null) =>
    setLimit.mutate({ budgetId: budget.meta.id, elementId, period: month, amount, monthIndex })

  const visibleMonths = Array.from({ length: visible }, (_, i) => addMonths(firstMonth, i))
  const monthIndex = (m: string): number => (plan ? plan.months.indexOf(m) : -1)

  // always the unfiltered structure (per-row `hidden` flags, nothing dropped) — hideEmpty
  // is applied per SECTION at render time so each header's reveal is independent
  const rows = useMemo(() => (plan ? bucketPlanRows(plan, false) : null), [plan])
  const folderSideMap = useMemo(() => (plan ? folderSides(plan) : new Map<Id, FolderSide>()), [plan])
  const canEdit = canEditBudget(budget.meta, userId)
  const configure = canConfigureBudget(budget.meta, userId)

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
    canEdit,
    onEditEnvelope: (el) =>
      setEnvelopeDialog({ open: true, envelope: el, folderId: el.folderId, side: isIncomeType(el.type) ? 'income' : 'expense' }),
    onMoveToFolder: setMoveFolderTarget,
  }
  const dialogCell = planLimitTarget ? planLimitTarget.el.cells[planLimitTarget.monthIndex] : undefined

  // per-section hide-empty: a row belongs to exactly one bucket — a folder, or its
  // side's loose rows — so each bucket's count/reveal is independent of the others
  const sectionVisibleRows = (sectionRows: PlanRow[], revealed: boolean) =>
    hideEmpty && !revealed ? sectionRows.filter((r) => !r.hidden) : sectionRows
  const sectionHiddenCount = (sectionRows: PlanRow[], revealed: boolean) =>
    hideEmpty && !revealed ? sectionRows.filter((r) => r.hidden).length : 0

  const incomeFolded = folded('income')
  const incomeRevealed = revealedSections.has('income')
  const incomeLoose = incomeFolded ? [] : sectionVisibleRows(rows.income.loose, incomeRevealed)
  const incomeHiddenCount = incomeFolded ? 0 : sectionHiddenCount(rows.income.loose, incomeRevealed)

  const expenseRevealed = revealedSections.has('expense')
  const expenseLoose = sectionVisibleRows(rows.expense.loose, expenseRevealed)
  const expenseHiddenCount = sectionHiddenCount(rows.expense.loose, expenseRevealed)

  return (
    <div ref={containerRef} className="flex min-h-0 flex-1 flex-col overflow-y-auto" data-testid="plan-sheet">
      <div className="flex items-center gap-2 border-b px-2 py-1.5">
        <Switch
          id="plan-hide-empty"
          size="sm"
          checked={hideEmpty}
          onCheckedChange={() => togglePlanHideEmpty()}
          aria-label={t('budgets.page.plan.density.hide_empty')}
        />
        <Label htmlFor="plan-hide-empty" className="text-xs font-normal text-muted-foreground">
          {t('budgets.page.plan.density.hide_empty')}
        </Label>
      </div>
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
            {rows.income.folders.map((f) => (
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
            {rows.income.uncategorized ? <ElementRow key={rowKey(rows.income.uncategorized)} row={rows.income.uncategorized} ctx={ctx} /> : null}
          </>
        ) : null}
      </section>

      <section data-testid="plan-section-expense" className="flex flex-col gap-1 px-1 py-1">
        {rows.expense.folders.map((f) => (
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
        {rows.expense.uncategorized ? <ElementRow key={rowKey(rows.expense.uncategorized)} row={rows.expense.uncategorized} ctx={ctx} /> : null}
        {expenseHiddenCount > 0 ? (
          <div className="px-2 pb-1">
            <HiddenRowsNotice count={expenseHiddenCount} onShow={() => revealSection('expense')} />
          </div>
        ) : null}
      </section>

      {rows.archived.length > 0 ? (
        <section data-testid="plan-section-archived" className="flex flex-col gap-1 px-1 py-1">
          <SectionHeader
            label={t('budgets.page.plan.section.archived')}
            foldKey="archived"
            folded={folded('archived')}
            onToggleFold={togglePlanFold}
            hiddenCount={0}
            onShow={() => {}}
          />
          {!folded('archived')
            ? rows.archived.map((r) => <ElementRow key={rowKey(r)} row={r} ctx={ctx} />)
            : null}
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

      <MoveToFolderDialog
        target={moveFolderTarget}
        folders={plan.structure.folders}
        folderSideMap={folderSideMap}
        onClose={() => setMoveFolderTarget(null)}
        onPick={pickFolder}
      />
    </div>
  )
}
