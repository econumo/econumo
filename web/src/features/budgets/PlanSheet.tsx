import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ClipboardEvent, CSSProperties, KeyboardEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DndContext, DragOverlay, MeasuringStrategy, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import type { CollisionDetection, DragEndEvent, DragOverEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { besidePointer, centerRowOnPointer } from '@/lib/dnd'
import { afterIdFromDrop } from '@/lib/ordering'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { CoinLoader } from '@/components/CoinLoader'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import type {
  BudgetDto,
  BudgetFolderSide,
  BudgetPlanDto,
  PlanCellDto,
  PlanElementDto,
} from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, isPlannedType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { useIsCompact } from '@/hooks/useIsCompact'
import { elementDisplayName, periodLabeler } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import { BudgetTransactionsDialog, TRANSFERS_TARGET_ID } from './BudgetTransactionsDialog'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import {
  canConfigureBudget,
  commentCellKey,
  planFetchWindow,
  useBudgetComments,
  useBudgetPlan,
  useFillPlannedCells,
  useMoveBudgetFolder,
  useMoveElement,
  useMoveIntoEnvelope,
  usePlanSetLimit,
} from './queries'
import {
  arrangementItem,
  dropIndicatorFor,
  envelopeCollisions,
  envelopeOfDrop,
  moveElementInArrangement,
  placeElements,
  placeFromEnvelope,
  preferRowCollisions,
} from './elementMove'
import type { DropIndicator, ElementContainer } from './elementMove'
import { DragFolder, DragGhost, DragRow, FolderGrip } from './MonthDrag'
import { CommentsPanel } from './CommentsPanel'
import { ElementSheet } from './ElementSheet'
import { planCellFigures } from './phoneMonth'
import { isEnvelopeType } from './elementEdit'
import { limitAmountFromInput } from './limitAmount'
import { METRICS, trackEvent } from '@/lib/metrics'
import { SetLimitDialog } from './SetLimitDialog'
import { useBudgetLineMenus } from './useBudgetLineMenus'
import {
  addMonths,
  balanceRow,
  bucketPlanRows,
  everydayBalanceRow,
  fillTargetCol,
  makePlanExchange,
  monthDate,
  monthDiff,
  planHasSavingsData,
  projectSavingsClosings,
  planMonthExchange,
  planGroupSums,
  planTotals,
  planVisibleCount,
  PLAN_NAME_COL_MAX_PX,
  PLAN_NAME_COL_MIN_PX,
  PLAN_NAME_COL_PX,
  clampPlanNameWidth,
  planWindow,
  savingsAsPlanElement,
  savingsBalanceRow,
} from './planMath'
import type { MonthExchange, PlanFolderSection, PlanRow, PlanRows } from './planMath'
import type { LineControls, MenuAction } from './monthLayout'
import { LineControlsContext, LineLayoutContext, PLAN_FIGURE_COL, PLAN_LINE, PLAN_CROSSHAIR, PLAN_NAME_COL, PLAN_SECTION_RULE, PLAN_SELECTED_TINT, ROW_INDENT, RowLevelContext } from './monthLayout'
import { FigureCells, FolderLine, MonthSectionHeader } from './monthLines'
import { ElementRow, SumCell, cellDomId, commentsReadOnly, isEditableCell, sourceAmount } from './PlanRows'
import { PlanBalanceRow, PlanTotals } from './PlanTotalsLines'
import type { GridCtx, PlanCellEdit, PlanLimitTarget, PlanSelection } from './PlanRows'
import type { CellMove } from './PlanCellInput'

export interface PlanSheetProps {
  /** the ALREADY-LOADED budget (meta for permissions/currency); plan data is fetched inside */
  budget: BudgetDto
  currencies: CurrencyDto[]
  userId: Id | undefined
  editMode: boolean
  /** opens Budget settings (the savings section's menu chooses accounts there) */
  onOpenSettings: () => void
  /** the Budget / Plan words: the grid's month row is this view's month selector,
   *  so they sit at its start, where the Budget view's strip has them */
  viewSwitch?: ReactNode
}

const rowKey = (r: PlanRow): string => `${r.element.id}:${r.element.type}`

const selectionDomId = (sel: PlanSelection): string => cellDomId(sel.rowKey, sel.col)

type GroupSums = ReturnType<typeof planGroupSums>

const monthColClass = (col: number, selectedCol: number): string => `${PLAN_FIGURE_COL}${col === selectedCol ? ` ${PLAN_SELECTED_TINT}` : ''}`

// Radix portals render popover/dialog/drawer/dropdown-menu content outside the grid's
// DOM subtree, but React re-dispatches both keyboard AND click events through the
// component tree (portals are still React children), so they still reach the cell's
// onClick/the grid's onKeyDown.
//
// The keydown guard: a keystroke typed inside an open editor/menu must not be hijacked
// by grid navigation (Enter running a grid action instead of the dialog's, ArrowLeft not
// moving the caret, ArrowDown moving the grid selection instead of a menu highlight).
const KEYDOWN_ESCAPE_SELECTOR =
  'input, textarea, [data-slot="popover-content"], [data-slot="dialog-content"], [data-slot="drawer-content"], [data-slot="dropdown-menu-content"]'
// The click guard (F2): a cell click that bubbles up from a nested interactive control —
// the actual's transactions link, the in-cell editor's input, an open dropdown menu's items —
// must not steal focus back onto the grid; those controls already manage their own
// focus, and the grid regains it naturally once they close.
const CLICK_ESCAPE_SELECTOR = `button, [role="button"], ${KEYDOWN_ESCAPE_SELECTOR}`
// A focused control inside the grid (a row's ⋮, a fold chevron, an actual's link) keeps
// its own activation keys: the grid acting on Enter too would open the cell editor or
// the edit dialog behind its menu, and its preventDefault would cancel the click.
const CONTROL_SELECTOR = 'button, [role="button"], [aria-haspopup]'
const GRID_KEYS_ON_CONTROL = ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Escape']

/** Excel-style fill-right state: startCol is the source column (the value being
 *  copied), targetCol the column currently covered (>= startCol). A pointer fill is
 *  driven by the handle drag (startX/colWidth translate pointer travel into columns)
 *  and commits on pointerup; a keyboard fill is grown/shrunk with Shift+Arrow and
 *  commits when Shift is released. */
interface FillDrag {
  source: 'pointer' | 'keyboard'
  rowKey: string
  elementId: Id
  amount: string
  startCol: number
  targetCol: number
  startX: number
  colWidth: number
}

// A row in a list: under its grip when drag is on (the wrapper sits OUTSIDE the
// row's own [data-row-id] element, so selection, keyboard navigation and the fill
// handle are untouched by it), bare otherwise.
function PlanRowList({ rows, ctx, indicator }: { rows: PlanRow[]; ctx: GridCtx; indicator: DropIndicator | null }) {
  return (
    <>
      {rows.map((r) =>
        ctx.drag && isDraggableRow(r) ? (
          <DragRow key={rowKey(r)} id={r.element.id} indicator={indicator?.kind === 'row' && indicator.id === r.element.id ? indicator.edge : undefined}>
            <ElementRow row={r} ctx={ctx} />
          </DragRow>
        ) : (
          <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
        ),
      )}
    </>
  )
}

const draggableIds = (rows: PlanRow[]): string[] => rows.filter(isDraggableRow).map((r) => r.element.id)

// Uncategorized is a synthetic bucket with no stored position, and an archived row
// is out of the ordering entirely — neither can be dropped anywhere meaningful.
const isDraggableRow = (r: PlanRow): boolean => r.element.id !== UNCATEGORIZED_ID && r.element.isArchived === 0

// A folder-like group of rows: a real folder, or the No folder group that names a
// section's folder-less rows once the section has folders. Its line folds the rows
// (`foldKey` is the group's fold key, shared with the Budget view) and is a row of
// the grid the keyboard selection can land on.
function FolderGroup({
  foldKey,
  name,
  ctx,
  sums,
  folded,
  onToggleFold,
  handle,
  menu,
  empty,
  children,
}: {
  foldKey: string
  name: string
  ctx: GridCtx
  /** one per visible month; null for a group with no rows */
  sums: ReactNode[] | null
  folded: boolean
  onToggleFold: (key: string) => void
  handle?: ReactNode
  menu?: MenuAction[]
  /** unfolded and holding no rows: the empty-folder hint shows instead of them */
  empty: boolean
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <div data-testid={`plan-folder-${foldKey}`}>
      {/* a click anywhere on the line folds it; the keyboard cursor never stops here */}
      <div role="row" data-plan-line="" className="border-b border-border/60">
        <FolderLine
          name={name}
          folded={folded}
          onToggle={() => onToggleFold(foldKey)}
          sums={sums}
          handle={handle}
          menu={menu}
          actionsColumn={false}
          nameCell={{ role: 'rowheader' }}
          sumKey={foldKey}
        />
      </div>
      {empty ? (
        // stacked over blank month cells, so the selected month's tint runs through it
        <div className="grid">
          <div className={`${PLAN_LINE} min-h-7 [grid-area:1/1]`}>
            <span className={PLAN_NAME_COL} />
            <FigureCells cells={ctx.visibleMonths.map(() => null)} />
          </div>
          {/* the text sits on the page colour so the month lines stop behind it */}
          <p className={`${ROW_INDENT['in-folder']} self-center px-2 py-1 text-xs text-muted-foreground [grid-area:1/1]`}>
            <span className="relative bg-background pr-1">{t('budgets.page.budget.structure.empty_folder.note')}</span>
          </p>
        </div>
      ) : (
        children
      )}
    </div>
  )
}

// A folder with zero members (neutral, per folderSides) still renders — its line and,
// when expanded, the same empty-folder hint the budget view shows: a folder only
// disappears if it doesn't exist, not because it currently has no side.
function FolderRows({
  section,
  ctx,
  sums,
  folded,
  collapsed,
  onToggleFold,
  menu,
  indicator,
}: {
  section: PlanFolderSection
  ctx: GridCtx
  /** one per visible month; null for a folder with no rows */
  sums: ReactNode[] | null
  folded: boolean
  /** a folder drag is in flight: hide every folder's rows so the lines reorder as
   *  compact blocks. Distinct from `folded`, which is the user's own fold state and
   *  still drives the chevron and aria-expanded. */
  collapsed: boolean
  onToggleFold: (key: string) => void
  menu?: MenuAction[]
  indicator: DropIndicator | null
}) {
  const visibleRows = collapsed || folded ? [] : section.rows
  return (
    <FolderGroup
      foldKey={section.folder.id}
      name={section.folder.name}
      ctx={ctx}
      sums={sums}
      folded={folded}
      onToggleFold={onToggleFold}
      handle={ctx.drag ? <FolderGrip name={section.folder.name} /> : null}
      menu={menu}
      empty={!folded && !collapsed && section.rows.length === 0}
    >
      <PlanRowList rows={visibleRows} ctx={ctx} indicator={indicator} />
    </FolderGroup>
  )
}

// Same flattening the renderer walks (folders -> loose, income then savings then
// neutral folders then expense, then archived), so Up/Down can never reach a row that isn't on
// screen. Section and folder lines are no stops; a folder contributes its visible
// members only. Only root element rows — the ones a limit can be set on — are in the order: an expanded envelope's
// children are read-only breakdown lines and are stepped over.
type FlatRow = { kind: 'element'; rowKey: string; el: PlanElementDto }

// The open in-cell editor, with what its commit needs captured when it opened: the
// window can move before the commit lands (Tab at the last column pages it), and the
// write must go to the month that was being edited.
interface CellEditState extends PlanCellEdit {
  elementId: Id
  monthIndex: number
  /** the plan as it stood when editing began; committing it unchanged writes nothing */
  planned: string
}

const unsetPlan = (planned: string): boolean => planned === '' || isZero(planned)

// The Budget view's fold keys for a section's No folder group, so a fold carries over
const NO_FOLDER_KEY: Record<'income' | 'expense', string> = { income: '__income__no_folder__', expense: '__no_folder__' }

/** a section names its folder-less rows only next to real folders, as the Budget view
 *  does; while one of its rows drags, an empty No folder shows too: the drop target
 *  for taking a row out of its folder or envelope */
const hasNoFolderLine = (band: PlanRows['income'], rowDragging: boolean): boolean =>
  band.folders.length > 0 && (band.loose.length > 0 || rowDragging)

/** the section a drag runs in: each has its own DndContext */
type DragSide = 'income' | 'savings' | 'neutral' | 'expense'

interface DragState {
  /** a row or category drags in this section */
  rowIn: DragSide | null
  /** a folder drags in this section: its folders show their lines only */
  folderIn: DragSide | null
}

function buildFlatRows(rows: PlanRows, savingsRows: PlanRow[], folded: (key: string) => boolean, drag: DragState): FlatRow[] {
  const flatRows: FlatRow[] = []
  const pushRow = (r: PlanRow) => {
    flatRows.push({ kind: 'element', rowKey: rowKey(r), el: r.element })
  }
  // section and folder lines are not stops: the cursor walks rows, and a folded
  // group's rows are skipped
  const pushGroup = (foldKey: string, groupRows: PlanRow[], side: DragSide) => {
    if (!folded(foldKey) && drag.folderIn !== side) {
      groupRows.forEach(pushRow)
    }
  }
  const pushSide = (side: 'income' | 'expense') => {
    const band = rows[side]
    band.folders.forEach((f) => pushGroup(f.folder.id, f.rows, side))
    if (hasNoFolderLine(band, drag.rowIn === side)) {
      pushGroup(NO_FOLDER_KEY[side], band.loose, side)
    } else {
      band.loose.forEach(pushRow)
    }
    if (band.uncategorized) {
      pushRow(band.uncategorized)
    }
  }
  if (!folded('income')) {
    pushSide('income')
  }
  if (!folded('savings')) {
    savingsRows.forEach(pushRow)
  }
  rows.neutral.forEach((f) => pushGroup(f.folder.id, f.rows, 'neutral'))
  if (!folded('expense')) {
    pushSide('expense')
  }
  if (rows.archived.length > 0 && !folded('archived')) {
    rows.archived.forEach(pushRow)
  }
  return flatRows
}

export function PlanSheet({ budget, currencies, userId, editMode, onOpenSettings, viewSwitch }: PlanSheetProps) {
  const { t, i18n } = useTranslation()
  const isCompact = useIsCompact()
  const [planLimitTarget, setPlanLimitTarget] = useState<PlanLimitTarget | null>(null)
  const [sheetCellTarget, setSheetCellTarget] = useState<PlanLimitTarget | null>(null)
  const openSheet = useCallback((target: PlanLimitTarget) => setSheetCellTarget(target), [])
  // a dropped order, held until the refetched plan replaces it so nothing snaps back
  const [dragArrangement, setDragArrangement] = useState<ElementContainer[] | null>(null)
  // a category on its way into or out of an envelope: hidden until the refetched
  // plan shows it in its new place
  const [pendingMemberId, setPendingMemberId] = useState<string | null>(null)
  // the row or category being dragged, the folder being dragged, and the section the
  // drag runs in
  const [dragActiveId, setDragActiveId] = useState<string | null>(null)
  const [draggingFolderId, setDraggingFolderId] = useState<Id | null>(null)
  const [dragSide, setDragSide] = useState<DragSide | null>(null)
  // where the dragged row or category would land: the insertion line
  const [dropIndicator, setDropIndicator] = useState<DropIndicator | null>(null)
  const dragState: DragState = useMemo(
    () => ({ rowIn: dragActiveId !== null ? dragSide : null, folderIn: draggingFolderId !== null ? dragSide : null }),
    [dragActiveId, draggingFolderId, dragSide],
  )
  // the open comment thread: anchored to its cell on desktop/tablet, a sheet on a phone
  const [commentsDialogTarget, setCommentsDialogTarget] = useState<(PlanLimitTarget & { anchor: HTMLElement | null }) | null>(null)
  const commentsOpen = commentsDialogTarget !== null
  // A modal opened from the grid (an actual or a Transfers figure) has no trigger
  // Radix can hand focus back to — the keyboard has none, and a figure button
  // re-renders away — so on close focus would fall to <body> and the
  // arrow keys go dead. Remember that the grid opened it and reclaim focus once it
  // closes; mouse-opened dialogs (row menu) leave focus alone as before.
  const editorFromGrid = useRef(false)
  const moveElement = useMoveElement()
  const orderFolders = useMoveBudgetFolder()
  const moveIntoEnvelope = useMoveIntoEnvelope()
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  // the totals drill-down: which bucket, and which column's month
  const [transactionsTarget, setTransactionsTarget] = useState<{ target: BudgetTransactionsTarget; month: string } | null>(null)
  const openTotalsTransactions = useCallback(
    (month: string) => {
      const target: BudgetTransactionsTarget = {
        id: TRANSFERS_TARGET_ID,
        type: 'transfers',
        name: t('budgets.page.plan.totals.transfers'),
        icon: 'sync_alt',
        currencyId: null,
      }
      editorFromGrid.current = true
      setTransactionsTarget({ target, month })
    },
    [t],
  )
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
  // ResizeObserver never fires in jsdom, so width stays 0 there — the same
  // floor a real narrow viewport would collapse to (planVisibleCount<3 -> 1).
  // the name column's width: the one being dragged, else the remembered one. Months
  // re-fit to the remembered width only, so they don't come and go mid-drag.
  const storedNameWidth = useBudgetPeriodStore((s) => s.planNameWidth)
  const setPlanNameWidth = useBudgetPeriodStore((s) => s.setPlanNameWidth)
  const [dragNameWidth, setDragNameWidth] = useState<number | null>(null)
  const nameResize = useRef<{ startX: number; startWidth: number } | null>(null)
  const committedNameWidth = storedNameWidth ?? PLAN_NAME_COL_PX
  const nameWidth = dragNameWidth ?? committedNameWidth
  const commitNameWidth = (px: number | null) => {
    setPlanNameWidth(px === null ? null : clampPlanNameWidth(px))
    trackEvent(METRICS.BUDGET_PLAN_RESIZE_NAME_COLUMN)
  }
  const visible = width > 0 ? planVisibleCount(width, committedNameWidth) : 3

  const startedAt = budget.meta.startedAt
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const stepPeriod = useBudgetPeriodStore((s) => s.stepPeriod)
  const setPeriod = useBudgetPeriodStore((s) => s.setPeriod)
  const planFolds = useBudgetPeriodStore((s) => s.planFolds)
  const togglePlanFold = useBudgetPeriodStore((s) => s.togglePlanFold)
  const folded = useCallback((key: string): boolean => !!planFolds[key], [planFolds])
  const [selection, setSelection] = useState<PlanSelection | null>(null)
  const [fillDrag, setFillDrag] = useState<FillDrag | null>(null)
  const [editing, setEditing] = useState<CellEditState | null>(null)

  // Keyboard navigation moves the selection without moving the scroller, so the
  // cursor walks off screen. Scroll the minimum needed to bring it back, measuring
  // against the sticky month header and balance row that float over the scroller's edges.
  useEffect(() => {
    const scroller = containerRef.current
    if (!selection || !scroller) {
      return
    }
    const cell = scroller.querySelector<HTMLElement>(`#${CSS.escape(selectionDomId(selection))}`)
    if (!cell) {
      return
    }
    const view = scroller.getBoundingClientRect()
    const box = cell.getBoundingClientRect()
    const header = scroller.querySelector<HTMLElement>('[data-testid="plan-month-header"]')
    const footer = scroller.querySelector<HTMLElement>('[data-testid="plan-balance-row"]')
    const topEdge = header ? header.getBoundingClientRect().bottom : view.top
    const bottomEdge = footer ? footer.getBoundingClientRect().top : view.bottom
    if (box.top < topEdge) {
      scroller.scrollTop -= topEdge - box.top
    } else if (box.bottom > bottomEdge) {
      scroller.scrollTop += box.bottom - bottomEdge
    }
  }, [selection])
  const firstMonth = planWindow(selectedDate, visible, startedAt, budget.meta.endedAt)
  const atStart = firstMonth <= startedAt.slice(0, 7) + '-01'
  const atEnd = !!budget.meta.endedAt && addMonths(firstMonth, visible - 1) >= budget.meta.endedAt.slice(0, 7) + '-01'
  // The window is derived from the selected month, and a clamped window (at the start or
  // end month) does not follow it one to one; pick the selected month that moves the
  // window itself by one, so the cursor's column lands on the neighbouring month.
  const shiftWindow = (delta: number) => {
    const first = addMonths(firstMonth, delta)
    stepPeriod(monthDiff(selectedDate, visible > 1 ? addMonths(first, 1) : first))
  }

  const { data: plan, isPending, isError, refetch, planKey, fetchFrom } = useBudgetPlan(budget.meta.id, firstMonth, visible)
  const setLimit = usePlanSetLimit(planKey)
  const fillCells = useFillPlannedCells(planKey)
  const menus = useBudgetLineMenus({ budget, plan, userId, onOpenSettings })
  const editorOpen = menus.editorOpen || commentsDialogTarget !== null || planLimitTarget !== null || transactionsTarget !== null
  useEffect(() => {
    if (!editorOpen && editorFromGrid.current) {
      editorFromGrid.current = false
      containerRef.current?.focus()
    }
  }, [editorOpen])
  // the plan's OWN window, so both caches cover exactly the same months
  const { byCell: commentsByCell, truncated: commentsTruncated } = useBudgetComments(budget.meta.id, fetchFrom, planFetchWindow(firstMonth, visible).months)

  // The optimistic drop (an order, or a category hidden on its way into or out of an
  // envelope) is released only when genuinely fresh plan data arrives: a refetch yields
  // a new object, so keying on identity hands over in one frame with no window where
  // the stale server order is rendered.
  const arrangedFrom = useRef<BudgetPlanDto | null | undefined>(undefined)
  useEffect(() => {
    if (arrangedFrom.current === undefined) {
      return
    }
    if (plan !== arrangedFrom.current) {
      arrangedFrom.current = undefined
      setDragArrangement(null)
      setPendingMemberId(null)
    }
  }, [plan])

  // clicking a cell must land keyboard focus on the grid too, or the arrow keys that
  // follow a click are dead until the user tabs in manually (F2). A cell click also
  // bubbles from any interactive descendant it contains (the actual's transactions
  // link, the row's ⋮ menu trigger, the expand chevron) — grabbing grid
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
  // a stored month outside the budget (before its start, after its end) has no
  // column to tint
  const selectedCol = visibleMonths.indexOf(selectedDate)
  // An editor whose month left the window (a resize, the strip) closes rather than
  // reappearing, stale, when that month scrolls back in. Leaving its cell by click or
  // key has already committed it.
  useEffect(() => {
    if (editing && !visibleMonths.includes(editing.month)) {
      setEditing(null)
    }
  }, [editing, visibleMonths])
  // the selected cell's month column, marked down the whole grid
  const crosshairCol = selection ? selection.col : -1
  const monthIndex = useCallback((m: string): number => (plan ? plan.months.indexOf(m) : -1), [plan])
  // The uncategorized row's synthetic id names no real element the server would
  // accept, so it gets no comment entry point at all — guarded here too since the
  // keyboard (Shift+Enter / Shift+F2) reaches this without the marker's gate.
  const openComments = useCallback(
    (target: PlanLimitTarget, opts: { fromGrid?: boolean; anchor?: HTMLElement | null } = {}) => {
      if (target.el.id === UNCATEGORIZED_ID) {
        return
      }
      const col = visibleMonths.indexOf(target.month)
      const anchor =
        opts.anchor !== undefined ? opts.anchor : col >= 0 ? document.getElementById(cellDomId(`${target.el.id}:${target.el.type}`, col)) : null
      if (opts.fromGrid) {
        editorFromGrid.current = true
      }
      setCommentsDialogTarget({ ...target, anchor })
    },
    [visibleMonths],
  )
  const openTransactions = useCallback(
    (el: PlanElementDto, month: string) => {
      editorFromGrid.current = true
      setTransactionsTarget({
        target: { id: el.id, type: el.type, name: elementDisplayName(el.id, el.name, t), icon: el.icon, currencyId: el.currencyId },
        month,
      })
    },
    [t],
  )
  // same wording as the budget view's period strip
  const monthLabel = useMemo(() => {
    const label = periodLabeler(i18n.language)
    return (m: string) => label(monthDate(m))
  }, [i18n.language])
  const layout = useMemo(
    () => ({ kind: 'plan' as const, cols: visible, selectedCol, crosshairCol, months: visibleMonths, monthLabels: visibleMonths.map(monthLabel) }),
    [visible, selectedCol, crosshairCol, visibleMonths, monthLabel],
  )
  const sheetPlanTarget = sheetCellTarget ? { kind: 'plan' as const, cell: planCellFigures(sheetCellTarget.el, sheetCellTarget.monthIndex) } : null
  const sheetEdit = sheetPlanTarget ? menus.editAccess(sheetPlanTarget) : null
  // With a mouse every line's ⋮ menu shows on hover; a touch screen has no hover, so
  // there they show on every line while edit mode is on, and not at all without it.
  const lineControls: LineControls | null = isCompact ? (editMode ? 'always' : null) : 'hover'
  const hoverMenus = lineControls !== null
  // grips show with the menus, for whoever may arrange the budget; an archived
  // budget is read-only whatever the role
  const dragEnabled = hoverMenus && budget.meta.isArchived === 0 && canConfigureBudget(budget.meta, userId)
  const selectedIndex = monthIndex(selectedDate)
  const rowMenu = hoverMenus ? (el: PlanElementDto) => menus.planRowMenu(el, selectedIndex) : undefined
  const childMenu = hoverMenus ? menus.envelopeChildMenu : undefined

  const fillStart = useCallback(
    (rk: string, el: PlanElementDto, col: number, e: ReactPointerEvent<HTMLElement>) => {
      const colWidth = (e.currentTarget.closest('[role="gridcell"]') as HTMLElement | null)?.getBoundingClientRect().width ?? 0
      e.currentTarget.setPointerCapture(e.pointerId)
      e.preventDefault()
      // no stopPropagation here: the native pointerdown must still reach Radix's
      // document-level DismissableLayer listener, so an open popover or menu
      // dismisses through the fill drag, same as any other outside click. Cell-selection clicks are guarded separately by the handle's own
      // onClick stopPropagation below.
      const month = visibleMonths[col]
      const idx = month !== undefined ? monthIndex(month) : -1
      const amount = sourceAmount(el, idx)
      setFillDrag({ source: 'pointer', rowKey: rk, elementId: el.id, amount, startCol: col, targetCol: col, startX: e.clientX, colWidth })
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
    // the drag may have started from a merely hovered cell: the cell whose value was
    // copied becomes the selection, so the keyboard picks up from where the mouse left
    select(fillDrag.rowKey, fillDrag.startCol)
  }, [fillDrag, visibleMonths, monthIndex, fillCells, budget.meta.id, select])

  const fillCancel = useCallback(() => setFillDrag(null), [])

  const rows = useMemo(() => {
    if (!plan) {
      return null
    }
    if (!dragArrangement && !pendingMemberId) {
      return bucketPlanRows(plan)
    }
    let elements = plan.structure.elements
    if (pendingMemberId) {
      elements = elements
        .filter((el) => el.id !== pendingMemberId)
        .map((el) => (el.children.some((c) => c.id === pendingMemberId) ? { ...el, children: el.children.filter((c) => c.id !== pendingMemberId) } : el))
    }
    if (dragArrangement) {
      elements = placeElements(elements, dragArrangement)
    }
    return bucketPlanRows({ ...plan, structure: { ...plan.structure, elements } })
  }, [plan, dragArrangement, pendingMemberId])

  // Live savings rows by position, then any deleted account's rows: those stay in
  // this section as read-only history rather than joining the Archived band. A
  // dropped reorder is held through the same dragArrangement the other bands use —
  // a savings band arrangement names account ids only, so it never re-places an
  // element, and vice versa.
  const savingsRows = useMemo((): PlanRow[] => {
    if (!plan) {
      return []
    }
    const all = (plan.structure.savings ?? []).map((row) => savingsAsPlanElement(projectSavingsClosings(row, plan.months)))
    const placed = dragArrangement ? placeElements(all, dragArrangement) : all
    const byPosition = (a: PlanElementDto, b: PlanElementDto) => a.position - b.position
    return [
      ...placed.filter((el) => el.isArchived === 0).sort(byPosition),
      ...placed.filter((el) => el.isArchived !== 0).sort(byPosition),
    ].map((element) => ({ element }))
  }, [plan, dragArrangement])

  // Rows that earn their place per VISIBLE window, not per fetched cells (the fetch
  // carries buffer months either side): an uncategorized row is dropped when every
  // visible column's actual is zero, and an archived row when no visible column has
  // an actual or a plan — the archive is history, so an all-dash row there is noise
  // (the budget view's Archive section applies the same rule per month). Both can
  // still have values outside the window, so they reappear once navigation brings
  // that month into view. Derived once here (not per-section, not in buildFlatRows)
  // so the render sections and the keyboard flat-row list can never disagree on
  // which rows are on screen.
  const shownRows = useMemo(() => {
    if (!rows) {
      return null
    }
    const hasVisible = (r: PlanRow, test: (c: PlanCellDto) => boolean): boolean =>
      visibleMonths.some((m) => {
        const i = monthIndex(m)
        const c = i >= 0 ? r.element.cells[i] : undefined
        return c !== undefined && test(c)
      })
    const visibleUncat = (r: PlanRow | null): PlanRow | null => (r && hasVisible(r, (c) => !isZero(c.actual)) ? r : null)
    return {
      ...rows,
      income: { ...rows.income, uncategorized: visibleUncat(rows.income.uncategorized) },
      expense: { ...rows.expense, uncategorized: visibleUncat(rows.expense.uncategorized) },
      archived: rows.archived.filter((r) => hasVisible(r, (c) => !isZero(c.actual) || c.planned !== '')),
    }
  }, [rows, visibleMonths, monthIndex])

  // the expensive per-render work the arrow keys used to re-trigger on every keystroke:
  // FX conversion, totals, running balance, and the flattened keyboard-nav row list all
  // depend on the plan/currencies/fold state, never on `selection` — memoizing them means
  // moving the cursor no longer recomputes any of this (F7)
  const ex: MonthExchange | null = useMemo(() => (plan ? makePlanExchange(plan, currencies) : null), [plan, currencies])
  const totals = useMemo(() => (plan && ex ? planTotals(plan, ex) : []), [plan, ex])
  const balance = useMemo(() => (plan && ex ? balanceRow(plan, totals, ex) : []), [plan, ex, totals])
  // The Savings section and its totals line are tied to ROWS: there is nothing to list
  // or total without one. The balance split is tied to DATA instead (planHasSavingsData):
  // a deleted savings account (still a flagged member) can leave a pre-window opening
  // balance or flow with no row to show for it, and that money is still not everyday money.
  const hasSavings = savingsRows.length > 0
  const hasSavingsData = plan ? planHasSavingsData(plan) : false
  const savingsBalance = useMemo(
    () => (plan && ex && hasSavingsData ? savingsBalanceRow(plan, totals, ex) : null),
    [plan, ex, totals, hasSavingsData],
  )
  const everydayBalance = useMemo(
    () => (savingsBalance ? everydayBalanceRow(balance, savingsBalance) : balance),
    [balance, savingsBalance],
  )
  const flatRows = useMemo(
    () => (shownRows ? buildFlatRows(shownRows, savingsRows, folded, dragState) : []),
    [shownRows, savingsRows, folded, dragState],
  )

  // Touch keeps the item sheet (and its amount dialog); a read-only cell never opens an editor.
  const startEdit = useCallback(
    (rk: string, col: number, opts: { replace: boolean; text?: string }) => {
      const entry = flatRows.find((r) => r.rowKey === rk)
      const month = visibleMonths[col]
      if (isCompact || entry?.kind !== 'element' || month === undefined) {
        return
      }
      const idx = monthIndex(month)
      if (!isEditableCell(entry.el, month, idx, budget.meta, userId)) {
        return
      }
      const planned = entry.el.cells[idx]?.planned ?? ''
      const initial = opts.replace ? (opts.text ?? '') : unsetPlan(planned) ? '' : normalizeNumber(planned)
      setSelection({ rowKey: rk, col })
      // a second double-click inside an open editor must not reset what was typed
      setEditing((cur) =>
        cur && cur.rowKey === rk && cur.month === month
          ? cur
          : { rowKey: rk, col, month, initial, replace: opts.replace, elementId: entry.el.id, monthIndex: idx, planned },
      )
    },
    [flatRows, visibleMonths, isCompact, monthIndex, budget.meta, userId],
  )
  // The commit/cancel bodies move the selection with the same rules as the arrow keys,
  // which live below the loading guard; the grid context gets stable forwarders.
  const editActions = useRef<{ finish: (raw: string, move: CellMove) => void; cancel: () => void } | null>(null)
  const finishEdit = useCallback((raw: string, move: CellMove) => editActions.current?.finish(raw, move), [])
  const cancelEdit = useCallback(() => editActions.current?.cancel(), [])
  // Per-month sums of every section and folder line, keyed by fold key. Memoized
  // like the totals: they convert every cell, and the arrow keys re-render the grid.
  const groupSums = useMemo(() => {
    const sums = new Map<string, GroupSums>()
    if (!shownRows || !ex) {
      return sums
    }
    const sumOf = (rows: PlanRow[]) => planGroupSums(rows.map((r) => r.element), visibleMonths, monthIndex, ex)
    for (const side of ['income', 'expense'] as const) {
      const band = shownRows[side]
      for (const f of band.folders) {
        sums.set(f.folder.id, sumOf(f.rows))
      }
      sums.set(NO_FOLDER_KEY[side], sumOf(band.loose))
      sums.set(side, sumOf([...band.folders.flatMap((f) => f.rows), ...band.loose, ...(band.uncategorized ? [band.uncategorized] : [])]))
    }
    sums.set('savings', planGroupSums(savingsRows.map((r) => r.element), visibleMonths, monthIndex, ex, 'balance'))
    sums.set('archived', sumOf(shownRows.archived))
    return sums
  }, [shownRows, savingsRows, visibleMonths, monthIndex, ex])

  const gridCtx: GridCtx | null = useMemo(() => {
    if (!plan) {
      return null
    }
    return {
      visibleMonths,
      monthIndex,
      selected: selectedDate,
      selectedCol,
      crosshairCol,
      currencies,
      baseCurrencyId: budget.meta.currencyId,
      meta: budget.meta,
      userId,
      isCompact,
      monthLabel,
      openSheet,
      openTransactions,
      commentsByCell,
      openComments,
      commentsOpen,
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
      drag: dragEnabled,
      dragActiveId,
      editing,
      startEdit,
      finishEdit,
      cancelEdit,
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    plan,
    visibleMonths,
    monthIndex,
    selectedDate,
    selectedCol,
    crosshairCol,
    currencies,
    budget.meta,
    userId,
    isCompact,
    monthLabel,
    commentsByCell,
    openComments,
    openTransactions,
    commentsOpen,
    selection,
    select,
    fillDrag,
    fillStart,
    fillMove,
    fillEnd,
    fillCancel,
    editMode,
    dragEnabled,
    dragActiveId,
    editing,
    startEdit,
    finishEdit,
    cancelEdit,
  ])
  // The ⋮ menus read live rights, categories and tags, so they are rebuilt on every
  // render rather than memoized; moving the selection re-renders the rows anyway.
  const ctx: GridCtx | null = gridCtx && { ...gridCtx, rowMenu, childMenu }

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

  const incomeFolded = folded('income')
  const expenseFolded = folded('expense')
  const noFolderLine = (side: 'income' | 'expense'): boolean => hasNoFolderLine(shownRows[side], dragState.rowIn === side)
  // the folder-less rows on screen: none while their section, or their No folder line,
  // is folded, nor while a folder of their section drags
  const looseShown = (side: 'income' | 'expense', sectionFolded: boolean): PlanRow[] =>
    sectionFolded || dragState.folderIn === side || (noFolderLine(side) && folded(NO_FOLDER_KEY[side])) ? [] : shownRows[side].loose
  const incomeLoose = looseShown('income', incomeFolded)
  const expenseLoose = looseShown('expense', expenseFolded)

  const savingsFolded = folded('savings')
  const savingsLive = savingsRows.filter(isDraggableRow)
  const savingsDeleted = savingsRows.filter((r) => !isDraggableRow(r))

  const fmtBudget = (v: string) => moneyFormat(v, planCurrency, { showCurrency: false, useNativePrecision: false })
  const sumCells = (sums: GroupSums | undefined, balance = false): ReactNode[] =>
    visibleMonths.map((m, i) => (
      <SumCell key={m} index={i} sum={sums && monthIndex(m) >= 0 ? sums[i] : null} month={m} ctx={ctx} fmt={fmtBudget} balance={balance} />
    ))

  const folderMenu = (f: PlanFolderSection, side: BudgetFolderSide) =>
    hoverMenus ? menus.folderActionsFor({ id: f.folder.id, name: f.folder.name }, f.rows.length === 0, side) : undefined

  // the drop lands in this folder (null: the folder-less rows), whose rows are not shown
  const folderIndicator = (folderId: Id | null): boolean => dropIndicator?.kind === 'folder' && (dropIndicator.folderId ?? null) === folderId

  // A folder's line and rows: sortable among the section's folders and a drop target
  // for rows while drag is on.
  const folderSection = (f: PlanFolderSection, side: DragSide, menu: MenuAction[] | undefined) => {
    const collapsed = dragState.folderIn === side
    const isFolded = folded(f.folder.id)
    const node = (
      <FolderRows
        section={f}
        ctx={ctx}
        sums={f.rows.length > 0 ? sumCells(groupSums.get(f.folder.id)) : null}
        folded={isFolded}
        collapsed={collapsed}
        onToggleFold={togglePlanFold}
        menu={menu}
        indicator={dropIndicator}
      />
    )
    return dragEnabled ? (
      <DragFolder
        key={f.folder.id}
        sortableId={f.folder.id}
        dropId={`bfolder:${f.folder.id}`}
        rowIds={collapsed || isFolded ? [] : draggableIds(f.rows)}
        indicator={folderIndicator(f.folder.id)}
        folderDragging={draggingFolderId !== null}
      >
        {node}
      </DragFolder>
    ) : (
      <Fragment key={f.folder.id}>{node}</Fragment>
    )
  }

  // a section's folder-less rows: under a No folder line next to real folders, at
  // that line's step on their own otherwise
  const looseGroup = (side: 'income' | 'expense', rows: PlanRow[]) => {
    const list = <PlanRowList rows={rows} ctx={ctx} indicator={dropIndicator} />
    const key = NO_FOLDER_KEY[side]
    const all = shownRows[side].loose
    const node = !noFolderLine(side) ? (
      <RowLevelContext.Provider value="top">{list}</RowLevelContext.Provider>
    ) : (
      <FolderGroup
        foldKey={key}
        name={t('budgets.page.plan.menu.no_folder')}
        ctx={ctx}
        sums={all.length > 0 ? sumCells(groupSums.get(key)) : null}
        folded={folded(key)}
        onToggleFold={togglePlanFold}
        menu={hoverMenus ? menus.folderActionsFor(null, false, side) : undefined}
        empty={all.length === 0 && !folded(key) && dragState.folderIn !== side}
      >
        {list}
      </FolderGroup>
    )
    return dragEnabled ? (
      <DragFolder
        sortableId={null}
        dropId="bfolder:null"
        rowIds={draggableIds(rows)}
        indicator={folderIndicator(null)}
        folderDragging={draggingFolderId !== null}
      >
        {node}
      </DragFolder>
    ) : (
      node
    )
  }

  // The band's element buckets as the arrangement elementMove.ts operates on:
  // one container per folder plus the loose rows. Uncategorized and archived
  // rows are excluded — they carry no position the server would honour. A folded
  // folder keeps all its members: none of them can be under the pointer, and a drop
  // on the folder itself appends after the last one, as in the Budget view.
  function bandArrangement(side: 'income' | 'expense'): ElementContainer[] {
    const band = shownRows![side]
    return [
      ...band.folders.map((f) => ({ folderId: f.folder.id as Id | null, ids: draggableIds(f.rows) })),
      { folderId: null as Id | null, ids: draggableIds(band.loose) },
    ]
  }

  // The envelope each category sits in. A category may go into any other live
  // envelope of its own side; the section's own DndContext keeps the other side's
  // envelopes out of reach, and this keeps the rule explicit.
  const elementById = new Map(plan.structure.elements.map((el) => [el.id, el]))
  const envelopeOfCategory = new Map(
    plan.structure.elements.flatMap((el) => (isEnvelopeType(el.type) ? el.children.map((c) => [c.id, el.id] as const) : [])),
  )
  const sideOf = (el: PlanElementDto): 'income' | 'expense' => (isIncomeType(el.type) ? 'income' : 'expense')
  const CATEGORY_TYPE = { income: BudgetElementType.INCOME_CATEGORY, expense: BudgetElementType.CATEGORY } as const
  const isCategoryOf = (side: 'income' | 'expense', id: string): boolean => {
    const envelope = elementById.get(envelopeOfCategory.get(id) ?? '')
    return envelope ? sideOf(envelope) === side : elementById.get(id)?.type === CATEGORY_TYPE[side]
  }
  const canEnterEnvelope = (side: 'income' | 'expense') => (activeId: string, envelopeId: string): boolean => {
    const envelope = elementById.get(envelopeId)
    return (
      !!envelope &&
      isEnvelopeType(envelope.type) &&
      envelope.isArchived === 0 &&
      sideOf(envelope) === side &&
      isCategoryOf(side, activeId) &&
      envelopeOfCategory.get(activeId) !== envelopeId
    )
  }
  const folderIds = new Set(plan.structure.folders.map((f) => f.id))

  const resetDrag = () => {
    setDragActiveId(null)
    setDraggingFolderId(null)
    setDragSide(null)
    setDropIndicator(null)
  }

  // Folder-ness is read off the dragged id rather than the drag state, which a drop
  // in the same tick as its start would not see yet.
  function handleDragStart(side: DragSide, { active }: DragStartEvent) {
    const activeId = String(active.id)
    setDragSide(side)
    if (folderIds.has(activeId)) {
      setDraggingFolderId(activeId)
      return
    }
    setDragActiveId(activeId)
  }

  // No DOM re-ordering happens DURING the drag: rows stay put, a floating copy
  // follows the pointer and the insertion line marks where the drop lands.
  // Everything applies once, on drop — mutating the row order mid-drag shifts
  // layout under the pointer and feedback-loops the drag-over → re-measure cycle.
  function handleDragOver(side: 'income' | 'expense', { active, over }: DragOverEvent) {
    const activeId = String(active.id)
    const overId = over ? String(over.id) : null
    const fromEnvelope = envelopeOfCategory.get(activeId)
    if (folderIds.has(activeId) || !overId || overId === activeId || overId === fromEnvelope) {
      setDropIndicator(null)
      return
    }
    setDropIndicator(
      dropIndicatorFor(bandArrangement(side), activeId, overId, {
        fromEnvelope: fromEnvelope !== undefined,
        isFolded: (folderId) => (folderId === null ? noFolderLine(side) && folded(NO_FOLDER_KEY[side]) : folded(folderId)),
      }),
    )
  }

  function handleDragEnd(side: 'income' | 'expense' | 'neutral', { active, over }: DragEndEvent) {
    resetDrag()
    const activeId = String(active.id)
    const overId = over ? String(over.id) : null
    if (folderIds.has(activeId)) {
      // order-folders takes one global sequence, so the anchor is read from the
      // full position-sorted folder list, not just this section's slice.
      const ordered = [...plan!.structure.folders].sort((a, b) => a.position - b.position).map((f) => f.id)
      const from = ordered.indexOf(activeId)
      const to = overId ? ordered.indexOf(overId.replace(/^bfolder:/, '')) : -1
      if (from === -1 || to === -1 || from === to) {
        return
      }
      const reordered = arrayMove(ordered, from, to)
      orderFolders.mutate({ budgetId: budget.meta.id, id: activeId, afterId: afterIdFromDrop(reordered, activeId) })
      return
    }
    // the neutral section holds only header-only folders: no rows to move
    if (side === 'neutral' || !overId || overId === activeId) {
      return
    }
    const envelopeId = envelopeOfDrop(overId)
    if (envelopeId !== null) {
      if (canEnterEnvelope(side)(activeId, envelopeId)) {
        holdMember(activeId)
        moveIntoEnvelope.mutate({ budgetId: budget.meta.id, id: activeId, envelopeId }, { onError: releaseHold })
      }
      return
    }
    const fromEnvelope = envelopeOfCategory.get(activeId)
    if (fromEnvelope) {
      // dropped back on its own envelope: it stays where it is
      const item = overId === fromEnvelope ? null : placeFromEnvelope(bandArrangement(side), activeId, overId)
      if (item) {
        holdMember(activeId)
        moveElement.mutate({ budgetId: budget.meta.id, item }, { onError: releaseHold })
      }
      return
    }
    commitElementMove(bandArrangement(side), activeId, overId)
  }

  function holdMember(id: string) {
    arrangedFrom.current = plan
    setPendingMemberId(id)
  }

  // A failed move drops every held preview, not just its own: arrangedFrom is shared,
  // so a hold left behind would never be released by the next plan.
  function releaseHold() {
    arrangedFrom.current = undefined
    setDragArrangement(null)
    setPendingMemberId(null)
  }

  function handleSavingsDragOver({ active, over }: DragOverEvent) {
    const ids = savingsLive.map((r) => r.element.id)
    const overId = over ? String(over.id) : null
    if (!overId || overId === String(active.id) || !ids.includes(overId)) {
      setDropIndicator(null)
      return
    }
    setDropIndicator(dropIndicatorFor([{ folderId: null, ids }], String(active.id), overId, { fromEnvelope: false, isFolded: () => false }))
  }

  // The savings band is one folder-less list: the only valid target is another
  // live savings row, so the move always carries folderId null.
  function handleSavingsDragEnd(event: DragEndEvent) {
    resetDrag()
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    const ids = savingsLive.map((r) => r.element.id)
    const overId = String(over.id)
    if (!ids.includes(overId)) {
      return
    }
    commitElementMove([{ folderId: null, ids }], String(active.id), overId)
  }

  function commitElementMove(base: ElementContainer[], activeId: string, target: string) {
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
      { onError: releaseHold },
    )
  }

  // the floating copy of what is being dragged: a row, or a category from an envelope
  const draggedItem = (() => {
    if (!dragActiveId) {
      return null
    }
    for (const el of [...plan.structure.elements, ...savingsRows.map((r) => r.element)]) {
      if (el.id === dragActiveId) {
        return { icon: el.icon, name: elementDisplayName(el.id, el.name, t) }
      }
      const child = el.children.find((c) => c.id === dragActiveId)
      if (child) {
        return { icon: child.icon, name: elementDisplayName(child.id, child.name, t) }
      }
    }
    return null
  })()

  // One DndContext per section, and that is what enforces the two hard constraints:
  // an element's side comes from its type and a folder's from its members, so neither
  // may cross the divider. A drag started in one section has no droppable in another —
  // the invalid drop cannot be expressed, rather than being rejected after the fact
  // (the server would answer CodeBudgetFolderSideMixed for elements, and order-folders
  // persists position only, so a cross-section folder move would silently snap back).
  const sectionDnd = (
    side: DragSide,
    collisionDetection: CollisionDetection,
    sortableFolderIds: string[],
    handlers: { onDragOver?: (e: DragOverEvent) => void; onDragEnd: (e: DragEndEvent) => void },
    children: ReactNode,
  ) =>
    dragEnabled ? (
      <DndContext
        sensors={sensors}
        collisionDetection={collisionDetection}
        // rows collapse on drag start, so drop-zone rects must re-measure mid-drag and
        // the grabbed node re-anchors to the pointer
        measuring={{ droppable: { strategy: MeasuringStrategy.Always } }}
        modifiers={[centerRowOnPointer]}
        onDragStart={(e) => handleDragStart(side, e)}
        onDragOver={handlers.onDragOver}
        onDragEnd={handlers.onDragEnd}
        onDragCancel={resetDrag}
      >
        <SortableContext items={sortableFolderIds} strategy={verticalListSortingStrategy}>
          {children}
        </SortableContext>
        {/* no drop animation: the moved row shows in its new place instead */}
        <DragOverlay dropAnimation={null} modifiers={[besidePointer]}>
          {draggedItem ? <DragGhost icon={draggedItem.icon} name={draggedItem.name} /> : null}
        </DragOverlay>
      </DndContext>
    ) : (
      children
    )

  function handleEnter(entry: FlatRow, col: number) {
    const month = visibleMonths[col]
    if (month === undefined) {
      return
    }
    const idx = monthIndex(month)
    if (isCompact) {
      // touch: Enter opens the same item sheet a tap would — any non-uncategorized
      // cell outside edit-structure mode, editable or not.
      if (!editMode && entry.el.id !== UNCATEGORIZED_ID && idx >= 0) {
        openSheet({ el: entry.el, month, monthIndex: idx })
      }
      return
    }
    startEdit(entry.rowKey, col, { replace: false })
  }

  // The element row under the roving selection (null for none), and that row's month
  // cell as the clipboard and keyboard-fill actions need it.
  function selectedElementRow(): (FlatRow & { kind: 'element' }) | null {
    return (selection && flatRows.find((r) => r.rowKey === selection.rowKey)) || null
  }

  function selectedMonthCell(): { entry: FlatRow & { kind: 'element' }; col: number; month: string; idx: number } | null {
    const entry = selectedElementRow()
    const month = selection ? visibleMonths[selection.col] : undefined
    if (!entry || !selection || month === undefined) {
      return null
    }
    return { entry, col: selection.col, month, idx: monthIndex(month) }
  }

  // Shift+Enter / Shift+F2: the keyboard route into the same thread the marker opens.
  function openCommentsFromGrid(cell: NonNullable<ReturnType<typeof selectedMonthCell>>) {
    openComments({ el: cell.entry.el, month: cell.month, monthIndex: cell.idx }, { fromGrid: true })
  }

  // Cmd/Ctrl+C on the focused grid: the browser fires `copy` on the grid even with no
  // text selection, so the selected cell's raw value goes to the system clipboard —
  // pasteable back here or into a spreadsheet. An inner input owns its own copy.
  function handleCopy(e: ClipboardEvent<HTMLDivElement>) {
    if ((e.target as HTMLElement).closest(KEYDOWN_ESCAPE_SELECTOR)) {
      return
    }
    const entry = selectedElementRow()
    if (!entry || !selection) {
      return
    }
    const cell = selectedMonthCell()
    const text = cell ? sourceAmount(cell.entry.el, cell.idx) : null
    if (text === null) {
      return
    }
    e.clipboardData.setData('text/plain', text)
    e.preventDefault()
  }

  // Cmd/Ctrl+V: a single amount into the selected cell, parsed by the exact rule the
  // cell editor applies (empty clears, formulas allowed). Blocked targets say why
  // instead of silently eating the paste; a fixed toast id keeps repeats from stacking.
  function handlePaste(e: ClipboardEvent<HTMLDivElement>) {
    if ((e.target as HTMLElement).closest(KEYDOWN_ESCAPE_SELECTOR)) {
      return
    }
    const cell = selectedMonthCell()
    if (!cell) {
      return
    }
    e.preventDefault()
    if (!isEditableCell(cell.entry.el, cell.month, cell.idx, budget.meta, userId)) {
      toast.error(t('budgets.page.plan.paste.not_editable'), { id: 'plan-paste-blocked' })
      return
    }
    const parsed = limitAmountFromInput(e.clipboardData.getData('text/plain'))
    if (!parsed.ok) {
      toast.error(t('budgets.page.plan.paste.invalid'), { id: 'plan-paste-blocked' })
      return
    }
    commit(cell.entry.el.id, cell.month, cell.idx, parsed.amount)
    trackEvent(METRICS.BUDGET_PLAN_PASTE_CELL)
  }

  // Shift+ArrowRight on an editable month cell arms a keyboard fill covering the next
  // column (clamped to the visible window — what is highlighted is what gets written,
  // same as the pointer drag); the source cell stays selected. Non-editable sources
  // arm nothing.
  function startKeyboardFill() {
    const cell = selectedMonthCell()
    if (!cell || !isEditableCell(cell.entry.el, cell.month, cell.idx, budget.meta, userId)) {
      return
    }
    setFillDrag({
      source: 'keyboard',
      rowKey: cell.entry.rowKey,
      elementId: cell.entry.el.id,
      amount: sourceAmount(cell.entry.el, cell.idx),
      startCol: cell.col,
      targetCol: Math.min(cell.col + 1, visible - 1),
      startX: 0,
      colWidth: 0,
    })
  }

  // Releasing Shift commits an armed keyboard fill (a range shrunk back to the source
  // writes nothing — fillEnd already skips targetCol === startCol).
  function handleKeyUp(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === 'Shift' && fillDrag?.source === 'keyboard') {
      fillEnd()
    }
  }

  // Shift released while focus is elsewhere never reaches handleKeyUp, so an armed
  // keyboard fill must die with the grid's focus rather than linger until the next
  // Shift release commits it unseen. Pointer drags hold pointer capture and are unaffected.
  function handleBlur() {
    if (fillDrag?.source === 'keyboard') {
      setFillDrag(null)
    }
  }

  // The arrow keys' walk, shared with the in-cell editor's Enter/Tab/↑/↓. At the
  // window's edges ← from the first month and → from the last page the window by a
  // month (clamped at the budget's start and end); the column stays put.
  function moveSelection(from: PlanSelection, dir: CellMove) {
    const idx = flatRows.findIndex((r) => r.rowKey === from.rowKey)
    switch (dir) {
      case 'up':
        if (idx > 0) {
          select(flatRows[idx - 1].rowKey, from.col)
        }
        break
      case 'down':
        if (idx >= 0 && idx < flatRows.length - 1) {
          select(flatRows[idx + 1].rowKey, from.col)
        }
        break
      case 'left':
        if (from.col === 0) {
          if (!atStart) {
            shiftWindow(-1)
          }
          select(from.rowKey, 0)
        } else {
          select(from.rowKey, from.col - 1)
        }
        break
      case 'right':
        if (from.col >= visible - 1) {
          if (!atEnd) {
            shiftWindow(1)
          }
          select(from.rowKey, visible - 1)
        } else {
          select(from.rowKey, from.col + 1)
        }
        break
      case 'none':
        break
    }
  }

  editActions.current = {
    // The write targets the month captured when editing began, and is sent before the
    // move: a Tab at the last column pages the window, and the column then names the
    // next month.
    finish: (raw, move) => {
      const ed = editing
      setEditing(null)
      if (!ed) {
        return
      }
      const parsed = limitAmountFromInput(raw)
      const before = unsetPlan(ed.planned) ? null : ed.planned
      const changed = parsed.ok && (before === null || parsed.amount === null ? before !== parsed.amount : cmp(before, parsed.amount) !== 0)
      if (parsed.ok && changed) {
        // the optimistic patch indexes the plan as it is now; -1 (the month is no
        // longer in it) patches nothing and the server write still goes by period
        commit(ed.elementId, ed.month, monthIndex(ed.month), parsed.amount)
      }
      // a click elsewhere ('none') leaves focus and the selection to that click
      if (move !== 'none') {
        containerRef.current?.focus()
        moveSelection({ rowKey: ed.rowKey, col: ed.col }, move)
      }
    },
    cancel: () => {
      setEditing(null)
      // Esc hands focus back to the grid; a blur has already sent it somewhere else
      if (containerRef.current?.contains(document.activeElement)) {
        containerRef.current.focus()
      }
    },
  }

  function clearSelectedCell() {
    const cell = selectedMonthCell()
    if (isCompact || !cell || !isEditableCell(cell.entry.el, cell.month, cell.idx, budget.meta, userId)) {
      return
    }
    if (unsetPlan(cell.entry.el.cells[cell.idx]?.planned ?? '')) {
      return
    }
    // per-call mutate callbacks only fire for the latest call, so a second clear made
    // before the first lands would swallow the first's event; the promise is per call.
    // A failure is already handled by the mutation, which rolls the cell back.
    setLimit
      .mutateAsync({ budgetId: budget.meta.id, elementId: cell.entry.el.id, period: cell.month, amount: null, monthIndex: cell.idx })
      .then(() => trackEvent(METRICS.BUDGET_PLAN_CLEAR_CELL))
      .catch(() => {})
  }

  function handleKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    // a control already handled it (a ⋮ trigger opens its menu on Enter or ArrowDown)
    if (e.defaultPrevented) {
      return
    }
    if ((e.target as HTMLElement).closest(CONTROL_SELECTOR) && !GRID_KEYS_ON_CONTROL.includes(e.key)) {
      return
    }
    if (fillDrag) {
      if (e.key === 'Escape') {
        setFillDrag(null)
        return
      }
      if (fillDrag.source === 'keyboard' && e.shiftKey && (e.key === 'ArrowRight' || e.key === 'ArrowLeft')) {
        e.preventDefault()
        const targetCol =
          e.key === 'ArrowRight' ? Math.min(fillDrag.targetCol + 1, visible - 1) : Math.max(fillDrag.targetCol - 1, fillDrag.startCol)
        setFillDrag({ ...fillDrag, targetCol })
        return
      }
      // A fill is modal-ish: any grid-navigation key arriving mid-fill (a plain
      // ArrowRight before pointerup / Shift release) must be a no-op, or the selection/
      // window shifts under the still-in-flight fill and desyncs the eventual commit's
      // target columns from what's visually covered.
      if (e.key === 'ArrowUp' || e.key === 'ArrowDown' || e.key === 'ArrowLeft' || e.key === 'ArrowRight' || e.key === 'Enter') {
        e.preventDefault()
      }
      return
    }
    // Radix portals render popover/dialog/drawer/dropdown-menu content outside the
    // grid's DOM subtree, but React re-dispatches the event through the component
    // tree, so it still reaches this handler. Without this guard, typing in the
    // in-cell editor, a dialog, or an open row menu gets its Arrow/Enter keys
    // hijacked by grid navigation (Enter closing the dialog without committing,
    // ArrowLeft not moving the caret, ArrowDown moving the grid selection instead of
    // the menu highlight).
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
    // Shift+Arrow left/right belongs to the keyboard fill: swallowed even when nothing
    // arms (a non-editable source) so the selection never jumps under a held
    // Shift. Shift+ArrowLeft with no fill armed is a no-op — the fill only grows right.
    if (e.shiftKey && (e.key === 'ArrowRight' || e.key === 'ArrowLeft')) {
      e.preventDefault()
      if (e.key === 'ArrowRight') {
        startKeyboardFill()
      }
      return
    }
    // Excel's "edit comment" shortcut, alongside the older Shift+Enter
    if (e.shiftKey && e.key === 'F2') {
      e.preventDefault()
      const cell = selectedMonthCell()
      if (cell) {
        openCommentsFromGrid(cell)
      }
      return
    }
    const entry = flatRows.find((r) => r.rowKey === selection.rowKey)
    if (!entry) {
      return
    }
    // A month cell edits like a spreadsheet: typing replaces the value, F2 edits it,
    // Delete/Backspace clears it. Touch keeps the item sheet even with a hardware
    // keyboard, and read-only cells take none of it (startEdit and clearSelectedCell
    // both check).
    if (!isCompact && !e.ctrlKey && !e.metaKey && !e.altKey) {
      if (/^[0-9.,-]$/.test(e.key)) {
        e.preventDefault()
        startEdit(entry.rowKey, selection.col, { replace: true, text: e.key })
        return
      }
      if (e.key === 'F2' && !e.shiftKey) {
        e.preventDefault()
        startEdit(entry.rowKey, selection.col, { replace: false })
        return
      }
      if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault()
        clearSelectedCell()
        return
      }
    }
    switch (e.key) {
      case 'ArrowUp':
        e.preventDefault()
        moveSelection(selection, 'up')
        break
      case 'ArrowDown':
        e.preventDefault()
        moveSelection(selection, 'down')
        break
      case 'ArrowLeft':
        e.preventDefault()
        moveSelection(selection, 'left')
        break
      case 'ArrowRight':
        e.preventDefault()
        moveSelection(selection, 'right')
        break
      case 'Enter':
        e.preventDefault()
        if (e.shiftKey) {
          const cell = selectedMonthCell()
          if (cell) {
            openCommentsFromGrid(cell)
          }
          return
        }
        handleEnter(entry, selection.col)
        break
      default:
        break
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div
        ref={attachContainer}
        role="grid"
        tabIndex={0}
        onKeyDown={handleKeyDown}
        onKeyUp={handleKeyUp}
        onBlur={handleBlur}
        onCopy={handleCopy}
        onPaste={handlePaste}
        aria-activedescendant={selection ? selectionDomId(selection) : undefined}
        className="flex min-h-0 flex-1 flex-col overflow-y-auto"
        style={{ '--plan-name-col': `${nameWidth}px` } as CSSProperties}
        data-testid="plan-sheet"
      >
        <LineLayoutContext.Provider value={layout}>
        {/* a touch screen outside edit mode has no menus or grips to hide, and no hover:
            what controls it has (the sums' Σ) show at once */}
        <LineControlsContext.Provider value={lineControls ?? (isCompact ? 'always' : 'hover')}>
        {/* The month row is the Plan view's month selector: a click makes that month
            the selected one, and ‹ › move the window a month at a time. */}
        <div role="row" data-testid="plan-month-header" className={`sticky top-0 z-20 ${PLAN_LINE} border-b bg-background`}>
          <span className={`${PLAN_NAME_COL} relative gap-1!`}>
            {viewSwitch}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-7"
              aria-label={t('budgets.page.budget.nav.prev')}
              disabled={atStart}
              onClick={() => shiftWindow(-1)}
            >
              <ChevronLeft className="size-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-7"
              aria-label={t('budgets.page.budget.nav.next')}
              disabled={atEnd}
              onClick={() => shiftWindow(1)}
            >
              <ChevronRight className="size-4" />
            </Button>
            {/* the name column's edge: drag (or ← →) to resize, double-click to reset */}
            <div
              role="separator"
              aria-orientation="vertical"
              aria-label={t('budgets.page.plan.name_column.resize')}
              aria-valuemin={PLAN_NAME_COL_MIN_PX}
              aria-valuemax={PLAN_NAME_COL_MAX_PX}
              aria-valuenow={nameWidth}
              title={t('budgets.page.plan.name_column.resize_hint')}
              tabIndex={0}
              data-testid="plan-name-resize"
              className="absolute top-0 -right-1.5 z-10 h-full w-3 cursor-col-resize touch-none rounded-sm hover:bg-ring/30 focus-visible:bg-ring/40 focus-visible:outline-none"
              onPointerDown={(e) => {
                e.currentTarget.setPointerCapture?.(e.pointerId)
                e.preventDefault()
                nameResize.current = { startX: e.clientX, startWidth: nameWidth }
              }}
              onPointerMove={(e) => {
                const drag = nameResize.current
                if (drag) {
                  setDragNameWidth(clampPlanNameWidth(drag.startWidth + e.clientX - drag.startX))
                }
              }}
              onPointerUp={(e) => {
                const drag = nameResize.current
                if (drag) {
                  nameResize.current = null
                  setDragNameWidth(null)
                  commitNameWidth(drag.startWidth + e.clientX - drag.startX)
                }
              }}
              onPointerCancel={() => {
                nameResize.current = null
                setDragNameWidth(null)
              }}
              onDoubleClick={() => commitNameWidth(null)}
              onKeyDown={(e) => {
                // the grid's arrows move the selection; here they belong to the edge
                if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
                  e.preventDefault()
                  e.stopPropagation()
                  commitNameWidth(nameWidth + (e.key === 'ArrowRight' ? 16 : -16))
                }
              }}
            />
          </span>
          {visibleMonths.map((m, i) => {
            const selected = i === selectedCol
            return (
              <div
                key={m}
                role="columnheader"
                data-month={m}
                data-col={i}
                data-selected-col={selected ? 'true' : undefined}
                data-crosshair={i === crosshairCol ? 'col' : undefined}
                aria-selected={selected}
                className={`${monthColClass(i, selectedCol)}${i === crosshairCol ? ` ${PLAN_CROSSHAIR}` : ''} p-0!`}
              >
                <button
                  type="button"
                  aria-pressed={selected}
                  className={`h-full w-full px-2 py-2 text-right text-xs uppercase tracking-wide ${selected ? 'text-foreground' : 'text-muted-foreground hover:text-foreground'}`}
                  onClick={() => {
                    if (!selected) {
                      setPeriod(m)
                    }
                  }}
                >
                  {monthLabel(m)}
                </button>
              </div>
            )
          })}
        </div>
        {/* Sections sit flush, split by a hairline, so the selected month's tint runs
            unbroken from the month header down to the Balance line. */}
        <section role="rowgroup" data-testid="plan-section-income" className="plan-band plan-band-income flex flex-col">
          <MonthSectionHeader
            foldKey="income"
            sumsOnDemand
            testId="plan-section-line-income"
            label={t('budgets.page.plan.section.income')}
            headings={[]}
            sums={sumCells(groupSums.get('income'))}
            actionsColumn={false}
            menu={hoverMenus ? menus.sectionMenu('income') : undefined}
          />
          {!incomeFolded
            ? sectionDnd(
                'income',
                envelopeCollisions(canEnterEnvelope('income')),
                shownRows.income.folders.map((f) => f.folder.id),
                { onDragOver: (e) => handleDragOver('income', e), onDragEnd: (e) => handleDragEnd('income', e) },
                <>
                  {shownRows.income.folders.map((f) => folderSection(f, 'income', folderMenu(f, 'income')))}
                  {looseGroup('income', incomeLoose)}
                  <RowLevelContext.Provider value="top">
                    {shownRows.income.uncategorized ? (
                      <ElementRow key={rowKey(shownRows.income.uncategorized)} row={shownRows.income.uncategorized} ctx={ctx} />
                    ) : null}
                  </RowLevelContext.Provider>
                </>,
              )
            : null}
        </section>

        {hasSavings ? (
          // Its own drag context: a savings row reorders among savings rows only and
          // can never reach a folder, which the server refuses for it anyway.
          <section role="rowgroup" data-testid="plan-section-savings" className={`plan-band plan-band-savings flex flex-col ${PLAN_SECTION_RULE}`}>
            <MonthSectionHeader
              foldKey="savings"
              sumsOnDemand
              testId="plan-section-line-savings"
              label={t('budgets.page.plan.section.savings')}
              headings={[]}
              sums={sumCells(groupSums.get('savings'), true)}
              actionsColumn={false}
              menu={hoverMenus ? menus.savingsSectionMenu : undefined}
            />
            {!savingsFolded ? (
              <RowLevelContext.Provider value="top">
                {sectionDnd(
                  'savings',
                  preferRowCollisions,
                  [],
                  { onDragOver: handleSavingsDragOver, onDragEnd: handleSavingsDragEnd },
                  dragEnabled ? (
                    <DragFolder sortableId={null} dropId="bfolder:null" rowIds={draggableIds(savingsLive)}>
                      <PlanRowList rows={savingsLive} ctx={ctx} indicator={dropIndicator} />
                    </DragFolder>
                  ) : (
                    <PlanRowList rows={savingsLive} ctx={ctx} indicator={null} />
                  ),
                )}
                {savingsDeleted.map((r) => (
                  <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
                ))}
              </RowLevelContext.Provider>
            ) : null}
          </section>
        ) : null}

        {shownRows.neutral.length > 0 ? (
          // Member-less folders belong to neither side yet, so they sit between the
          // bands rather than defaulting into one. Their own drag context keeps folder
          // reordering available; rows reach them via "Move to folder…" (a neutral
          // folder is offered to both sides there), never by a cross-band drag.
          <section role="rowgroup" data-testid="plan-section-neutral" className={`plan-band plan-band-neutral flex flex-col ${PLAN_SECTION_RULE}`}>
            {sectionDnd(
              'neutral',
              preferRowCollisions,
              shownRows.neutral.map((f) => f.folder.id),
              { onDragEnd: (e) => handleDragEnd('neutral', e) },
              shownRows.neutral.map((f) => folderSection(f, 'neutral', folderMenu(f, f.folder.side ?? 'expense'))),
            )}
          </section>
        ) : null}

        <section role="rowgroup" data-testid="plan-section-expense" className={`plan-band plan-band-expense flex flex-col ${PLAN_SECTION_RULE}`}>
          <MonthSectionHeader
            foldKey="expense"
            sumsOnDemand
            testId="plan-section-line-expense"
            label={t('budgets.page.plan.section.expenses')}
            headings={[]}
            sums={sumCells(groupSums.get('expense'))}
            actionsColumn={false}
            menu={hoverMenus ? menus.sectionMenu('expense') : undefined}
          />
          {!expenseFolded
            ? sectionDnd(
                'expense',
                envelopeCollisions(canEnterEnvelope('expense')),
                shownRows.expense.folders.map((f) => f.folder.id),
                { onDragOver: (e) => handleDragOver('expense', e), onDragEnd: (e) => handleDragEnd('expense', e) },
                <>
                  {shownRows.expense.folders.map((f) => folderSection(f, 'expense', folderMenu(f, 'expense')))}
                  {looseGroup('expense', expenseLoose)}
                  <RowLevelContext.Provider value="top">
                    {shownRows.expense.uncategorized ? (
                      <ElementRow key={rowKey(shownRows.expense.uncategorized)} row={shownRows.expense.uncategorized} ctx={ctx} />
                    ) : null}
                  </RowLevelContext.Provider>
                </>,
              )
            : null}
        </section>

        {shownRows.archived.length > 0 ? (
          <section role="rowgroup" data-testid="plan-section-archived" className={`plan-band plan-band-archived flex flex-col ${PLAN_SECTION_RULE}`}>
            <MonthSectionHeader
              foldKey="archived"
              sumsOnDemand
              testId="plan-section-line-archived"
              label={t('budgets.page.plan.section.archived')}
              headings={[]}
              sums={sumCells(groupSums.get('archived'))}
              actionsColumn={false}
            />
            {!folded('archived') ? (
              <RowLevelContext.Provider value="top">
                {shownRows.archived.map((r) => (
                  <ElementRow key={rowKey(r)} row={r} ctx={ctx} />
                ))}
              </RowLevelContext.Provider>
            ) : null}
          </section>
        ) : null}

        <PlanTotals
          visibleMonths={visibleMonths}
          monthIndex={monthIndex}
          currency={planCurrency}
          totals={totals}
          showSavings={hasSavings}
          savingsBalance={savingsBalance}
          onTransfersClick={openTotalsTransactions}
        />

        <PlanBalanceRow visibleMonths={visibleMonths} monthIndex={monthIndex} currency={planCurrency} balance={everydayBalance} />
        </LineControlsContext.Provider>
        </LineLayoutContext.Provider>
      </div>

      {/* the totals drill-down lists the CLICKED column's month, not the budget page's period */}
      <BudgetTransactionsDialog
        budget={budget}
        element={transactionsTarget?.target ?? null}
        periodStart={transactionsTarget?.month}
        onClose={() => setTransactionsTarget(null)}
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
        plan={planLimitTarget ? isPlannedType(planLimitTarget.el.type) : false}
      />

      <ElementSheet
        target={sheetPlanTarget}
        month={sheetCellTarget?.month ?? ''}
        baseCurrencyId={budget.meta.currencyId}
        currencies={currencies}
        exchange={plan && sheetCellTarget ? planMonthExchange(plan, currencies, sheetCellTarget.monthIndex) : (_from, _to, amount) => amount}
        comments={sheetCellTarget ? commentsByCell.get(commentCellKey(sheetCellTarget.el.id, sheetCellTarget.month)) ?? [] : []}
        commentsReadOnly={sheetCellTarget ? commentsReadOnly(budget.meta, sheetCellTarget.month) : true}
        canSetAmount={sheetCellTarget ? isEditableCell(sheetCellTarget.el, sheetCellTarget.month, sheetCellTarget.monthIndex, budget.meta, userId) : false}
        onClose={() => setSheetCellTarget(null)}
        onSetAmount={() => {
          if (sheetCellTarget) {
            setPlanLimitTarget(sheetCellTarget)
            setSheetCellTarget(null)
          }
        }}
        onOpenComments={() => {
          if (sheetCellTarget) {
            openComments(sheetCellTarget, { anchor: null })
            setSheetCellTarget(null)
          }
        }}
        onShowTransactions={
          sheetCellTarget && sheetCellTarget.el.id !== UNCATEGORIZED_ID
            ? () => {
                openTransactions(sheetCellTarget.el, sheetCellTarget.month)
                setSheetCellTarget(null)
              }
            : undefined
        }
        onEdit={
          sheetPlanTarget && sheetEdit !== null
            ? () => {
                // the element's own edit dialog replaces the sheet
                setSheetCellTarget(null)
                menus.editFromSheet(sheetPlanTarget)
              }
            : undefined
        }
        canEdit={sheetEdit === true}
      />

      <CommentsPanel
        open={commentsDialogTarget !== null}
        onClose={() => setCommentsDialogTarget(null)}
        title={commentsDialogTarget ? elementDisplayName(commentsDialogTarget.el.id, commentsDialogTarget.el.name, t) : ''}
        anchor={commentsDialogTarget?.anchor ?? null}
        budgetId={budget.meta.id}
        elementId={commentsDialogTarget?.el.id ?? ''}
        period={commentsDialogTarget?.month ?? ''}
        comments={commentsDialogTarget ? commentsByCell.get(commentCellKey(commentsDialogTarget.el.id, commentsDialogTarget.month)) ?? [] : []}
        currentUserId={userId}
        canModerate={canConfigureBudget(budget.meta, userId)}
        readOnly={commentsDialogTarget ? commentsReadOnly(budget.meta, commentsDialogTarget.month) : true}
        truncated={commentsTruncated}
      />

      {menus.dialogs}
    </div>
  )
}
