import { createContext, Fragment, memo, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DndContext, MeasuringStrategy, PointerSensor, pointerWithin, rectIntersection, useDroppable, useSensor, useSensors } from '@dnd-kit/core'
import type { CollisionDetection, DragEndEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
// aliased: a bare `CSS` import would shadow the global CSS object, whose
// CSS.escape the selection scroll-into-view effect below depends on
import { CSS as DndCSS } from '@dnd-kit/utilities'
import type { SortableHandleProps } from '@/components/SortableList'
import { afterIdFromDrop } from '@/lib/ordering'
import { ChevronDown, ChevronLeft, ChevronRight, GripVertical, MoreVertical } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { EntityIcon } from '@/components/EntityIcon'
import { CoinLoader } from '@/components/CoinLoader'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { BudgetDto, BudgetFolderDto, BudgetMetaDto, BudgetPlanDto, PlanChildDto, PlanElementDto } from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { useIsCompact } from '@/hooks/useIsCompact'
import { CategoryDialog } from '@/features/classifications/CategoryDialog'
import { TagDialog } from '@/features/classifications/TagDialog'
import type { TagDialogItem } from '@/features/classifications/TagDialog'
import { useUpdateCategory } from '@/features/classifications/queries'
import { elementDisplayName } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import {
  canDeleteEnvelope,
  canEditBudget,
  canUpdateLimits,
  useBudgetPlan,
  useChangeElementCurrency,
  useCreateBudgetFolder,
  useDeleteEnvelope,
  useFillPlannedCells,
  useMoveBudgetFolder,
  useMoveElement,
  usePlanSetLimit,
  useUpdateEnvelope,
} from './queries'
import { arrangementItem, moveElementInArrangement, placeElements } from './elementMove'
import type { ElementContainer } from './elementMove'
import { EnvelopeDialog } from './EnvelopeDialog'
import { LimitEditor } from './LimitEditor'
import { PlanCreateFolderDialog } from './PlanCreateFolderDialog'
import { SetLimitDialog } from './SetLimitDialog'
import {
  PLAN_ACTIONS_COL_PX,
  PLAN_CURRENCY_COL_PX,
  PLAN_MIN_MONTH_COL_PX,
  PLAN_NAME_COL_PX,
  addMonths,
  balanceRow,
  bucketPlanRows,
  clampFirstMonth,
  currentMonth,
  fillTargetCol,
  folderSides,
  makePlanExchange,
  monthDate,
  planInitialFirstMonth,
  planTotals,
  planVisibleCount,
  visibleSectionRows,
} from './planMath'
import type { FolderSide, MonthExchange, PlanFolderSection, PlanMonthTotals, PlanRow, PlanRows } from './planMath'

export interface PlanSheetProps {
  /** the ALREADY-LOADED budget (meta for permissions/currency); plan data is fetched inside */
  budget: BudgetDto
  currencies: CurrencyDto[]
  userId: Id | undefined
  editMode: boolean
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
  editMode: boolean
  onChangeCurrency: (el: PlanElementDto) => void
  onMoveToFolder: (el: PlanElementDto) => void
  onEditEnvelope: (el: PlanElementDto) => void
  onDeleteEnvelope: (el: PlanElementDto) => void
  canDeleteEnvelopes: boolean
}

// The budget view's wire response strips income envelopes and income-sided folders
// (internal/budget/builder_structure_build.go), so the plan sheet is the only surface
// where an income envelope is reachable — Edit/Delete must live here or an existing
// one could never be renamed, archived, re-scoped, or removed through any UI.
const isEnvelopeType = (type: BudgetElementType): boolean =>
  type === BudgetElementType.ENVELOPE || type === BudgetElementType.INCOME_ENVELOPE

function RowMenu({ el, ctx }: { el: PlanElementDto; ctx: GridCtx }) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" className="w-8 shrink-0" aria-label={`element actions ${el.name}`}>
          <MoreVertical className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => ctx.onChangeCurrency(el)}>
          {t('budgets.page.budget.structure.element.action.change_currency')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => ctx.onMoveToFolder(el)}>
          {t('budgets.page.plan.menu.move_to_folder')}
        </DropdownMenuItem>
        {isEnvelopeType(el.type) ? (
          <>
            <DropdownMenuItem onSelect={() => ctx.onEditEnvelope(el)}>{t('common.button.edit.label')}</DropdownMenuItem>
            {ctx.canDeleteEnvelopes ? (
              <DropdownMenuItem variant="destructive" onSelect={() => ctx.onDeleteEnvelope(el)}>
                {t('common.button.delete.label')}
              </DropdownMenuItem>
            ) : null}
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// Side-filtered folder picker: an income element may only land in an income or
// neutral folder. The server enforces this too (CodeBudgetFolderSideMixed); the
// filter keeps the user from ever seeing that error.
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

// Rows nest inside their folder section, and the dragged row travels under the
// pointer (its own rect always wins a pointer test) — so ignore the active row,
// prefer whatever OTHER row the pointer is inside, and fall back to sections
// (folder headers, or the loose-area container droppable for an empty band).
const preferRowCollisions: CollisionDetection = (args) => {
  const collisions = pointerWithin(args)
  const candidates = (collisions.length > 0 ? collisions : rectIntersection(args)).filter((c) => c.id !== args.active.id)
  const row = candidates.find((c) => !String(c.id).startsWith('pfolder:') && !String(c.id).startsWith('bfolder:'))
  return row ? [row] : candidates
}

// The grip is the activation handle; the whole row travels with the transform.
// items-start is required because an unfolded element renders its children inside
// this same wrapper — centering would drag the grip down to the middle of the whole
// expanded block. So the grip gets its own box matching the root row's height
// (py-1.5, mirroring ElementRow) and centers inside that, rather than carrying a
// hand-tuned top margin that silently drifts whenever row padding changes.
function PlanSortableRow({ id, name, children }: { id: string; name: string; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  return (
    <div
      ref={setNodeRef}
      data-plan-sortable={id}
      style={{ transform: DndCSS.Transform.toString(transform), transition }}
      className={isDragging ? 'opacity-60' : undefined}
    >
      <div className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-1">
        <button
          type="button"
          aria-label={`move ${name}`}
          className="row-start-1 flex h-full cursor-grab touch-none items-center text-muted-foreground"
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-4" />
        </button>
        <div className="row-start-1 min-w-0">{children}</div>
      </div>
    </div>
  )
}

// The folder is a sortable item itself; its grip lives in the header rendered by
// FolderRows, so the handle props travel via context rather than another prop hop.
const PlanFolderHandleContext = createContext<SortableHandleProps | null>(null)

function PlanFolderGrip({ name }: { name: string }) {
  const handle = useContext(PlanFolderHandleContext)
  if (!handle) {
    return null
  }
  return (
    <button
      type="button"
      aria-label={`move folder ${name}`}
      className="cursor-grab touch-none text-muted-foreground"
      {...handle.attributes}
      {...(handle.listeners ?? {})}
    >
      <GripVertical className="size-4" />
    </button>
  )
}

function PlanSortableFolder({ section, children }: { section: PlanFolderSection; children: ReactNode }) {
  const sortable = useSortable({ id: `pfolder:${section.folder.id}` })
  return (
    <div
      ref={sortable.setNodeRef}
      style={{ transform: DndCSS.Transform.toString(sortable.transform), transition: sortable.transition }}
      className={sortable.isDragging ? 'opacity-60' : undefined}
    >
      <PlanFolderHandleContext.Provider value={{ attributes: sortable.attributes, listeners: sortable.listeners }}>
        {children}
      </PlanFolderHandleContext.Provider>
    </div>
  )
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
      className="plan-row grid items-stretch gap-1 py-1 pr-2 pl-9 text-xs text-muted-foreground"
      style={{ gridTemplateColumns: ctx.gridCols }}
    >
      <span
        role="gridcell"
        id={cellDomId(rk, -1)}
        aria-selected={nameSelected}
        className={`flex h-full min-w-0 items-center gap-1.5 truncate${selectedClass(nameSelected)}`}
        title={displayName}
        onClick={(e) => ctx.select(rk, -1, e)}
      >
        <EntityIcon name={child.icon} className="text-base" />
        <span className="min-w-0 flex-1 truncate">{displayName}</span>
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
            className={`flex items-center justify-end px-2 py-1 ${selectedClass(selected)}`}
            onClick={(e) => ctx.select(rk, i, e)}
          >
            <span data-testid="cell-actual">{actualText}</span>
          </div>
        )
      })}
      <span />
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
        className="plan-row grid items-stretch gap-1 px-2 py-1.5"
        style={{ gridTemplateColumns: ctx.gridCols }}
      >
        <div
          role="gridcell"
          id={cellDomId(rk, -1)}
          aria-selected={nameSelected}
          className={`flex h-full min-w-0 items-center gap-1${selectedClass(nameSelected)}`}
          onClick={(e) => ctx.select(rk, -1, e)}
        >
          {expandable ? (
            <button
              type="button"
              className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
              aria-expanded={unfolded}
              title={t(unfolded ? 'common.button.collapse.label' : 'common.button.expand.label')}
              onClick={() => toggleElement(el.id)}
            >
              <Chevron className="size-3.5 shrink-0 text-muted-foreground" />
              {name}
            </button>
          ) : (
            <span className="flex min-w-0 flex-1 items-center gap-1.5">
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
          // an unset cell still shows (and edits as) 0, so it is draggable too —
          // gating on a set limit would hide the handle on every 0.00 cell
          const showFillHandle = selected && editable && !!cell && !ctx.isCompact && ctx.visibleMonths.length > 1
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
              className={`relative flex flex-col items-end justify-center px-2 py-1${editable ? ' cursor-pointer' : ''} ${selectedClass(selected)}${filled ? ' fill-covered bg-ring/15' : ''}`}
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
        {/* trailing track: currency, then the actions menu in edit mode — the budget
            table's geometry. Uncategorized has neither but still occupies the track. */}
        <div className="flex items-center justify-end gap-1">
          <span className="w-6 text-center text-xs text-muted-foreground">
            {isUncategorized ? null : currency?.symbol}
          </span>
          {ctx.editMode && !isUncategorized ? <RowMenu el={el} ctx={ctx} /> : null}
        </div>
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

// One sortable list per bucket (a folder's members, or a band's loose rows).
// Outside edit mode this is a plain map, so the read-only sheet keeps its exact
// DOM. The wrapper always sits OUTSIDE the row's own [data-row-id] element, so
// selection, keyboard navigation and the fill handle are untouched by it.
function PlanRowList({ rows, ctx }: { rows: PlanRow[]; ctx: GridCtx }) {
  const { t } = useTranslation()
  if (!ctx.editMode) {
    return (
      <>
        {rows.map((r) => (
          <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
        ))}
      </>
    )
  }
  return (
    <SortableContext items={rows.filter(isDraggableRow).map((r) => r.element.id)} strategy={verticalListSortingStrategy}>
      {rows.map((r) =>
        isDraggableRow(r) ? (
          <PlanSortableRow key={rowKey(r)} id={r.element.id} name={elementDisplayName(r.element.id, r.element.name, t)}>
            <ElementRow row={r} ctx={ctx} />
          </PlanSortableRow>
        ) : (
          <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
        ),
      )}
    </SortableContext>
  )
}

// A band's loose rows have no bordered wrapper the way a folder does (FolderRows
// supplies one), so an empty loose list leaves no droppable surface at all —
// a row could never leave a folder unless it happened to land exactly on another
// loose row. This container droppable gives that empty space a drop target,
// mirroring BudgetPage's per-bucket `bfolder:<key>` droppable so the existing
// `bfolder:null` branch in moveElementInArrangement (elementMove.ts) becomes reachable.
function LooseRowsContainer({ rows, ctx }: { rows: PlanRow[]; ctx: GridCtx }) {
  const { setNodeRef } = useDroppable({ id: 'bfolder:null' })
  return (
    <div ref={setNodeRef} data-testid="plan-loose-drop" className="min-h-2">
      <PlanRowList rows={rows} ctx={ctx} />
    </div>
  )
}

// Uncategorized is a synthetic bucket with no stored position, and an archived row
// is out of the ordering entirely — neither can be dropped anywhere meaningful.
const isDraggableRow = (r: PlanRow): boolean => r.element.id !== UNCATEGORIZED_ID && r.element.isArchived === 0

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
}: {
  label: string
  foldKey: string
  folded: boolean
  onToggleFold: (key: string) => void
  hiddenCount: number
  onShow: () => void
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
  collapsed,
  revealed,
  onToggleFold,
  onReveal,
}: {
  section: PlanFolderSection
  ctx: GridCtx
  hideEmpty: boolean
  folded: boolean
  /** a folder drag is in flight: hide every folder's rows so the headers reorder as
   *  compact blocks. Distinct from `folded`, which is the user's own fold state and
   *  still drives the chevron and aria-expanded. */
  collapsed: boolean
  revealed: boolean
  onToggleFold: (key: string) => void
  onReveal: () => void
}) {
  const { t } = useTranslation()
  const visibleRows = collapsed ? [] : visibleSectionRows(section.rows, folded, hideEmpty, revealed)
  const hiddenCount = !folded && hideEmpty && !revealed ? section.rows.filter((r) => r.hidden).length : 0
  const Chevron = folded ? ChevronRight : ChevronDown
  return (
    <div className="mb-1 rounded-md border p-1.5" data-testid={`plan-folder-${section.folder.id}`}>
      <div className="flex flex-wrap items-center justify-between gap-x-2 pb-1">
        <span className="flex min-w-0 items-center gap-1">
          {ctx.editMode ? <PlanFolderGrip name={section.folder.name} /> : null}
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
        </span>
        <HiddenRowsNotice count={hiddenCount} onShow={onReveal} />
      </div>
      <PlanRowList rows={visibleRows} ctx={ctx} />
    </div>
  )
}

interface TotalsRowSpec {
  key: 'income' | 'expenses' | 'uncategorized' | 'net'
  /** the totals.* key holds the label for every row but uncategorized, which reuses
   *  the shared common.uncategorized string the element rows already render */
  labelKey: string
  value: (t: PlanMonthTotals) => string
}

const TOTALS_ROWS: TotalsRowSpec[] = [
  { key: 'income', labelKey: 'budgets.page.plan.totals.income', value: (t) => t.effectiveIncome },
  { key: 'expenses', labelKey: 'budgets.page.plan.totals.expenses', value: (t) => t.effectiveExpense },
  { key: 'uncategorized', labelKey: 'common.uncategorized', value: (t) => t.uncategorizedActual },
  { key: 'net', labelKey: 'budgets.page.plan.totals.net', value: (t) => t.effectiveNet },
]

function PlanTotals({
  visibleMonths,
  monthIndex,
  gridCols,
  totals,
  currency,
}: {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  gridCols: string
  totals: PlanMonthTotals[]
  currency: CurrencyDto | undefined
}) {
  const { t } = useTranslation()
  return (
    <div role="rowgroup" className="mt-2 flex flex-col border-t" data-testid="plan-totals">
      {TOTALS_ROWS.map((spec) => (
        <Fragment key={spec.key}>
          <div role="row" className="grid items-center gap-1 px-2 py-1" style={{ gridTemplateColumns: gridCols }}>
            <span className="truncate text-xs font-medium text-muted-foreground">{t(spec.labelKey)}</span>
            {visibleMonths.map((m, i) => {
              const idx = monthIndex(m)
              const row = idx >= 0 ? totals[idx] : undefined
              return (
                <div key={m} data-col={i} className={`flex items-center justify-end px-2 py-1 `}>
                  <span className="text-sm">
                    {row ? moneyFormat(spec.value(row), currency, { showCurrency: false, useNativePrecision: false }) : '—'}
                  </span>
                </div>
              )
            })}
            <span />
          </div>
        </Fragment>
      ))}
    </div>
  )
}

function PlanBalanceRow({
  visibleMonths,
  monthIndex,
  gridCols,
  balance,
  currency,
}: {
  visibleMonths: string[]
  monthIndex: (m: string) => number
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
              className={`px-2 py-1 text-right text-sm  ${negative ? 'text-destructive' : ''}`}
            >
              {value !== undefined ? moneyFormat(value, currency, { showCurrency: false, useNativePrecision: false }) : '—'}
            </div>
          )
        })}
        <span />
      </div>
    </div>
  )
}


// Same flattening the renderer walks (folders -> loose, income then expense, then
// archived), so Up/Down can never reach a row that isn't on screen. The uncategorized
// expense figure is excluded: it renders as a totals line, not a selectable row.
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
  }
  if (rows.archived.length > 0 && !folded('archived')) {
    rows.archived.forEach(pushRow)
  }
  // the uncategorized expense figure is a totals line now, not a selectable element
  // row, so it stays out of the keyboard order entirely
  return flatRows
}

// Each band gets its own DndContext, and that is what enforces the two hard
// constraints: an element's side comes from its type and a folder's from its
// members, so neither may cross the divider. A drag started in one band simply
// has no droppable in the other — the invalid drop cannot be expressed, rather
// than being rejected after the fact (the server would answer
// CodeBudgetFolderSideMixed for elements, and order-folders persists position
// only, so a cross-band folder move would silently snap back on reload).
function PlanBand({
  editMode,
  sensors,
  folderIds,
  onDragStart,
  onDragEnd,
  onDragCancel,
  children,
}: {
  editMode: boolean
  sensors: ReturnType<typeof useSensors>
  folderIds: string[]
  onDragStart: (event: DragStartEvent) => void
  onDragEnd: (event: DragEndEvent) => void
  onDragCancel: () => void
  children: ReactNode
}) {
  if (!editMode) {
    return <>{children}</>
  }
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={preferRowCollisions}
      measuring={{ droppable: { strategy: MeasuringStrategy.Always } }}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragCancel={onDragCancel}
    >
      <SortableContext items={folderIds.map((id) => `pfolder:${id}`)} strategy={verticalListSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}

export function PlanSheet({ budget, currencies, userId, editMode }: PlanSheetProps) {
  const { t, i18n } = useTranslation()
  const isCompact = useIsCompact()
  const [planLimitTarget, setPlanLimitTarget] = useState<PlanLimitTarget | null>(null)
  const [dragArrangement, setDragArrangement] = useState<ElementContainer[] | null>(null)
  const [draggingFolder, setDraggingFolder] = useState(false)
  const [moveFolderTarget, setMoveFolderTarget] = useState<PlanElementDto | null>(null)
  const [currencyTarget, setCurrencyTarget] = useState<PlanElementDto | null>(null)
  const [createFolderOpen, setCreateFolderOpen] = useState(false)
  const [envelopeTarget, setEnvelopeTarget] = useState<PlanElementDto | null>(null)
  const [deleteEnvelopeTarget, setDeleteEnvelopeTarget] = useState<PlanElementDto | null>(null)
  const [categoryTarget, setCategoryTarget] = useState<Pick<CategoryDto, 'id' | 'name' | 'type' | 'icon'> | null>(null)
  const [tagTarget, setTagTarget] = useState<TagDialogItem | null>(null)
  // A modal opened from the keyboard (Enter on the name cell) has no trigger for
  // Radix to hand focus back to, so on close focus would fall to <body> and the
  // arrow keys go dead. Remember that the grid opened it and reclaim focus once it
  // closes; mouse-opened dialogs (row menu) leave focus alone as before.
  const editorFromGrid = useRef(false)
  const moveElement = useMoveElement()
  const orderFolders = useMoveBudgetFolder()
  const changeCurrency = useChangeElementCurrency()
  const createFolder = useCreateBudgetFolder()
  const updateEnvelope = useUpdateEnvelope()
  const deleteEnvelope = useDeleteEnvelope()
  const updateCategory = useUpdateCategory()
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  const [revealedSections, setRevealedSections] = useState<Set<string>>(new Set())
  const revealSection = (key: string) => setRevealedSections((prev) => new Set(prev).add(key))
  const containerRef = useRef<HTMLDivElement | null>(null)
  const observerRef = useRef<ResizeObserver | null>(null)
  const [width, setWidth] = useState(0)

  // A CALLBACK ref, not an effect: the component early-returns a loader while the plan
  // is fetching, so the grid node does not exist on first commit. A mount effect would
  // run against a null ref, never re-run, and leave the sheet stuck at the fallback
  // column count until something else forced a resize.
  //
  // clientWidth, not contentRect/getBoundingClientRect: this element scrolls
  // vertically, and only clientWidth excludes the scrollbar gutter — the wider box
  // overstates the space by ~15px, enough to push the last month past the edge.
  const attachContainer = useCallback((el: HTMLDivElement | null) => {
    containerRef.current = el
    observerRef.current?.disconnect()
    observerRef.current = null
    if (!el) {
      return
    }
    setWidth(el.clientWidth)
    const ro = new ResizeObserver(() => setWidth(el.clientWidth))
    ro.observe(el)
    observerRef.current = ro
  }, [])
  useEffect(() => () => observerRef.current?.disconnect(), [])
  const editorOpen = envelopeTarget !== null || categoryTarget !== null || tagTarget !== null
  useEffect(() => {
    if (!editorOpen && editorFromGrid.current) {
      editorFromGrid.current = false
      containerRef.current?.focus()
    }
  }, [editorOpen])
  // ResizeObserver never fires in jsdom, so width stays 0 there — the same
  // floor a real narrow viewport would collapse to (planVisibleCount<3 -> 1).
  const visible = width > 0 ? planVisibleCount(width, editMode) : 3

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

  // Keyboard navigation moves the selection without moving the scroller, so the
  // cursor walks off screen. Scroll the minimum needed to bring it back, measuring
  // against the sticky balance row that floats over the scroller's bottom edge.
  useEffect(() => {
    const scroller = containerRef.current
    if (!selection || !scroller) {
      return
    }
    const cell = scroller.querySelector<HTMLElement>(`#${CSS.escape(cellDomId(selection.rowKey, selection.col))}`)
    if (!cell) {
      return
    }
    const view = scroller.getBoundingClientRect()
    const box = cell.getBoundingClientRect()
    const footer = scroller.querySelector<HTMLElement>('[data-testid="plan-balance-row"]')
    const bottomEdge = footer ? footer.getBoundingClientRect().top : view.bottom
    if (box.top < view.top) {
      scroller.scrollTop -= view.top - box.top
    } else if (box.bottom > bottomEdge) {
      scroller.scrollTop += box.bottom - bottomEdge
    }
  }, [selection])
  const firstMonth = clampFirstMonth(persisted ?? planInitialFirstMonth(null, startedAt, visible), startedAt)
  const atStart = firstMonth <= startedAt.slice(0, 7) + '-01'

  const { data: plan, isPending, isError, refetch, planKey } = useBudgetPlan(budget.meta.id, firstMonth, visible)
  const setLimit = usePlanSetLimit(planKey)
  const fillCells = useFillPlannedCells(planKey)

  // The optimistic drop order is released only when genuinely fresh plan data arrives:
  // a refetch yields a new object, so keying on identity hands over in one frame with
  // no window where the stale server order is rendered.
  const arrangedFrom = useRef<BudgetPlanDto | null | undefined>(undefined)
  useEffect(() => {
    if (arrangedFrom.current === undefined) {
      return
    }
    if (plan !== arrangedFrom.current) {
      arrangedFrom.current = undefined
      setDragArrangement(null)
    }
  }, [plan])

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
  // A trailing track closes every row with the element's currency and, in edit mode,
  // its actions menu — the budget table's geometry. Every grid consumer (rows, month
  // header, totals, balance) shares this string, so they gain the column together and
  // stay aligned.
  const tailPx = PLAN_CURRENCY_COL_PX + (editMode ? PLAN_ACTIONS_COL_PX : 0)
  const gridCols = `${PLAN_NAME_COL_PX}px repeat(${visible}, minmax(${PLAN_MIN_MONTH_COL_PX}px, 1fr)) ${tailPx}px`
  const canEdit = canEditBudget(budget.meta, userId)
  const canDeleteEnvelopes = canDeleteEnvelope(budget.meta, userId)

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
      // an unset cell reads as 0 everywhere else in the grid, so dragging it
      // copies an explicit 0 rather than an empty limit
      const planned = idx >= 0 ? (el.cells[idx]?.planned ?? '') : ''
      const amount = planned === '' ? '0' : planned
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
  const rows = useMemo(() => {
    if (!plan) {
      return null
    }
    if (!dragArrangement) {
      return bucketPlanRows(plan, false)
    }
    const structure = { ...plan.structure, elements: placeElements(plan.structure.elements, dragArrangement) }
    return bucketPlanRows({ ...plan, structure }, false)
  }, [plan, dragArrangement])

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
  const folderSideMap = useMemo(() => (plan ? folderSides(plan) : new Map<Id, FolderSide>()), [plan])

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
      editMode,
      onChangeCurrency: setCurrencyTarget,
      onMoveToFolder: setMoveFolderTarget,
      onEditEnvelope: setEnvelopeTarget,
      onDeleteEnvelope: setDeleteEnvelopeTarget,
      canDeleteEnvelopes,
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
    editMode,
    canDeleteEnvelopes,
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

  // The band's element buckets as the arrangement elementMove.ts operates on:
  // one container per folder plus the loose rows. Uncategorized and archived
  // rows are excluded — they carry no position the server would honour.
  //
  // Mirrors the RENDER-side filtering exactly (FolderRows' visibleSectionRows call
  // per folder, and the incomeLoose/expenseLoose above for the loose bucket): only
  // rows actually on screen can be under the pointer during a drag, so afterId must
  // be read from that same filtered set — anchoring to a hideEmpty-hidden or
  // folded-away row would silently place the moved element after something the
  // user never saw.
  function bandArrangement(side: 'income' | 'expense'): ElementContainer[] {
    const band = shownRows![side]
    const loose = side === 'income' ? incomeLoose : expenseLoose
    return [
      ...band.folders.map((f) => ({
        folderId: f.folder.id as Id | null,
        ids: visibleSectionRows(f.rows, folded(f.folder.id), hideEmpty, revealedSections.has(f.folder.id))
          .filter(isDraggableRow)
          .map((r) => r.element.id),
      })),
      { folderId: null as Id | null, ids: loose.filter(isDraggableRow).map((r) => r.element.id) },
    ]
  }

  function handleBandDragStart(event: DragStartEvent) {
    setDraggingFolder(String(event.active.id).startsWith('pfolder:'))
  }

  function handleBandDragEnd(side: 'income' | 'expense', event: DragEndEvent) {
    setDraggingFolder(false)
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    const activeId = String(active.id)
    const overId = String(over.id)

    if (activeId.startsWith('pfolder:')) {
      // order-folders takes one global sequence, so the anchor is read from the
      // full position-sorted folder list, not just this band's slice.
      const draggedId = activeId.slice('pfolder:'.length)
      const targetId = overId.startsWith('pfolder:') ? overId.slice('pfolder:'.length) : null
      const folderIds = [...plan!.structure.folders].sort((a, b) => a.position - b.position).map((f) => f.id)
      const from = folderIds.indexOf(draggedId)
      const to = targetId ? folderIds.indexOf(targetId) : -1
      if (from === -1 || to === -1 || from === to) {
        return
      }
      const reordered = arrayMove(folderIds, from, to)
      orderFolders.mutate({ budgetId: budget.meta.id, id: draggedId, afterId: afterIdFromDrop(reordered, draggedId) })
      return
    }

    // a row dropped on a folder header lands in that folder, appended
    const target = overId.startsWith('pfolder:') ? `bfolder:${overId.slice('pfolder:'.length)}` : overId
    const base = bandArrangement(side)
    const moved = moveElementInArrangement(base, activeId, target)
    const item = arrangementItem(moved, activeId)
    const before = arrangementItem(base, activeId)
    if (!item || (before && before.folderId === item.folderId && before.position === item.position)) {
      return
    }
    // Hold the dropped order locally or the row snaps back to its server position.
    // The mutation's own callbacks are too early to clear it: invalidate() only kicks
    // off a refetch, so clearing on settle renders the STALE list for a frame and the
    // row visibly bounces. An effect on the plan data drops it once the new data is in.
    arrangedFrom.current = plan
    setDragArrangement(moved)
    moveElement.mutate(
      { budgetId: budget.meta.id, item },
      {
        onError: () => {
          arrangedFrom.current = undefined
          setDragArrangement(null)
        },
      },
    )
  }

  // Enter on the highlighted name cell opens the element's own edit dialog (the one
  // the row menu / settings pages use), gated by the right the backend enforces on
  // the matching update endpoint: budget role for an envelope (owner|admin|user),
  // row ownership for a category or tag — update-category/update-tag answer anyone
  // but the owner with NotFound, so a shared row must not offer the dialog at all.
  function openElementEditor(entry: FlatRow) {
    const target = entry.child ?? entry.el
    if (target.id === UNCATEGORIZED_ID) {
      return
    }
    // no right to edit: say why instead of silently ignoring the keystroke; a fixed
    // toast id so hammering Enter does not stack copies
    if (isEnvelopeType(target.type)) {
      if (canEdit) {
        editorFromGrid.current = true
        setEnvelopeTarget(entry.el)
      } else {
        toast.error(t('budgets.page.plan.edit.no_access_envelope'), { id: 'plan-edit-no-access' })
      }
      return
    }
    if (!userId || target.ownerUserId !== userId) {
      toast.error(
        target.type === BudgetElementType.TAG ? t('budgets.page.plan.edit.no_access_tag') : t('budgets.page.plan.edit.no_access_category'),
        { id: 'plan-edit-no-access' },
      )
      return
    }
    editorFromGrid.current = true
    if (target.type === BudgetElementType.TAG) {
      setTagTarget({ id: target.id, name: target.name, kind: 'tag', icon: target.icon })
      return
    }
    setCategoryTarget({ id: target.id, name: target.name, icon: target.icon, type: isIncomeType(target.type) ? 'income' : 'expense' })
  }

  function handleEnter(entry: FlatRow, col: number) {
    if (col === -1) {
      openElementEditor(entry)
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
      case ' ':
        // Space on the name cell folds/unfolds the element's children (Enter is the
        // edit shortcut); preventDefault so the scroller does not page down.
        if (selection.col === -1) {
          e.preventDefault()
          const entry = flatRows[idx]
          if (!entry.child && entry.el.children.length > 0) {
            toggleElement(entry.el.id)
          }
        }
        break
      default:
        break
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {editMode ? (
        <div className="flex items-center gap-2 px-2 pb-1">
          <Button type="button" variant="secondary" size="sm" onClick={() => setCreateFolderOpen(true)}>
            {t('budgets.page.budget.structure.action.create_folder')}
          </Button>
        </div>
      ) : null}
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
          <span />
        </div>
      </div>

      <div
        ref={attachContainer}
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
          />
          {!incomeFolded ? (
            <PlanBand
              editMode={editMode}
              sensors={sensors}
              folderIds={shownRows.income.folders.map((f) => f.folder.id)}
              onDragStart={handleBandDragStart}
              onDragEnd={(e) => handleBandDragEnd('income', e)}
              onDragCancel={() => setDraggingFolder(false)}
            >
              {shownRows.income.folders.map((f) => {
                const section = (
                  <FolderRows
                    section={f}
                    ctx={ctx}
                    hideEmpty={hideEmpty}
                    folded={folded(f.folder.id)}
                    collapsed={draggingFolder}
                    revealed={revealedSections.has(f.folder.id)}
                    onToggleFold={togglePlanFold}
                    onReveal={() => revealSection(f.folder.id)}
                  />
                )
                return editMode ? (
                  <PlanSortableFolder key={f.folder.id} section={f}>
                    {section}
                  </PlanSortableFolder>
                ) : (
                  <Fragment key={f.folder.id}>{section}</Fragment>
                )
              })}
              {editMode ? <LooseRowsContainer rows={incomeLoose} ctx={ctx} /> : <PlanRowList rows={incomeLoose} ctx={ctx} />}
              {shownRows.income.uncategorized ? (
                <ElementRow key={rowKey(shownRows.income.uncategorized)} row={shownRows.income.uncategorized} ctx={ctx} />
              ) : null}
            </PlanBand>
          ) : null}
        </section>

        <section
          role="rowgroup"
          data-testid="plan-section-expense"
          className="plan-band-expense mt-6 flex flex-col px-1 py-1"
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
            <PlanBand
              editMode={editMode}
              sensors={sensors}
              folderIds={shownRows.expense.folders.map((f) => f.folder.id)}
              onDragStart={handleBandDragStart}
              onDragEnd={(e) => handleBandDragEnd('expense', e)}
              onDragCancel={() => setDraggingFolder(false)}
            >
              {shownRows.expense.folders.map((f) => {
                const section = (
                  <FolderRows
                    section={f}
                    ctx={ctx}
                    hideEmpty={hideEmpty}
                    folded={folded(f.folder.id)}
                    collapsed={draggingFolder}
                    revealed={revealedSections.has(f.folder.id)}
                    onToggleFold={togglePlanFold}
                    onReveal={() => revealSection(f.folder.id)}
                  />
                )
                return editMode ? (
                  <PlanSortableFolder key={f.folder.id} section={f}>
                    {section}
                  </PlanSortableFolder>
                ) : (
                  <Fragment key={f.folder.id}>{section}</Fragment>
                )
              })}
              {editMode ? <LooseRowsContainer rows={expenseLoose} ctx={ctx} /> : <PlanRowList rows={expenseLoose} ctx={ctx} />}
            </PlanBand>
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
          gridCols={gridCols}
          totals={totals}
          currency={planCurrency}
        />

        <PlanBalanceRow
          visibleMonths={visibleMonths}
          monthIndex={monthIndex}
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

      <MoveToFolderDialog
        target={moveFolderTarget}
        folders={plan.structure.folders}
        folderSideMap={folderSideMap}
        onClose={() => setMoveFolderTarget(null)}
        onPick={(folderId) => {
          if (moveFolderTarget) {
            moveElement.mutate({
              budgetId: budget.meta.id,
              item: { id: moveFolderTarget.id, folderId, position: 0, afterId: null },
            })
          }
          setMoveFolderTarget(null)
        }}
      />

      {currencyTarget ? (
        <CurrencyPickerDialog
          open
          title={t('budgets.modal.change_element_currency_form.header')}
          value={currencyTarget.currencyId ?? budget.meta.currencyId}
          onClose={() => setCurrencyTarget(null)}
          onPick={(currencyId) => {
            changeCurrency.mutate(
              { budgetId: budget.meta.id, elementId: currencyTarget.id, currencyId },
              { onSuccess: () => setCurrencyTarget(null) },
            )
          }}
        />
      ) : null}

      <PlanCreateFolderDialog
        open={createFolderOpen}
        elements={plan.structure.elements}
        onClose={() => setCreateFolderOpen(false)}
        onSubmit={({ name, memberIds }) => {
          const id = uuidv7()
          createFolder.mutate(
            { budgetId: budget.meta.id, id, name },
            {
              onSuccess: () => {
                for (const memberId of memberIds) {
                  moveElement.mutate({ budgetId: budget.meta.id, item: { id: memberId, folderId: id, position: 0, afterId: null } })
                }
                setCreateFolderOpen(false)
              },
            },
          )
        }}
      />

      <EnvelopeDialog
        open={envelopeTarget !== null}
        envelope={envelopeTarget}
        budgetCurrencyId={budget.meta.currencyId}
        side={envelopeTarget && isIncomeType(envelopeTarget.type) ? 'income' : 'expense'}
        onClose={() => setEnvelopeTarget(null)}
        onSubmit={(form) => {
          if (envelopeTarget) {
            updateEnvelope.mutate(
              {
                budgetId: budget.meta.id,
                id: envelopeTarget.id,
                name: form.name,
                icon: form.icon,
                currencyId: form.currencyId,
                isArchived: form.isArchived,
                categories: form.categories,
              },
              { onSuccess: () => setEnvelopeTarget(null) },
            )
          }
        }}
      />

      <CategoryDialog
        open={categoryTarget !== null}
        category={categoryTarget}
        onClose={() => setCategoryTarget(null)}
        onSubmit={(form) => {
          if (categoryTarget) {
            updateCategory.mutate(
              { id: categoryTarget.id, name: form.name, icon: form.icon },
              { onSuccess: () => setCategoryTarget(null) },
            )
          }
        }}
      />

      <TagDialog open={tagTarget !== null} item={tagTarget} onClose={() => setTagTarget(null)} />

      <ConfirmDialog
        open={deleteEnvelopeTarget !== null}
        onClose={() => setDeleteEnvelopeTarget(null)}
        onConfirm={() => {
          if (deleteEnvelopeTarget) {
            deleteEnvelope.mutate({ budgetId: budget.meta.id, id: deleteEnvelopeTarget.id }, { onSettled: () => setDeleteEnvelopeTarget(null) })
          }
        }}
        title={t('budgets.modal.delete_envelope.header')}
        question={t('budgets.modal.delete_envelope.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
