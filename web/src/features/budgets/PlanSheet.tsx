import { createContext, Fragment, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ClipboardEvent, KeyboardEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DndContext, MeasuringStrategy, PointerSensor, pointerWithin, rectIntersection, useDroppable, useSensor, useSensors } from '@dnd-kit/core'
import type { CollisionDetection, DragEndEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
// aliased: a bare `CSS` import would shadow the global CSS object, whose
// CSS.escape the selection scroll-into-view effect below depends on
import { CSS as DndCSS } from '@dnd-kit/utilities'
import type { SortableHandleProps } from '@/components/SortableList'
import { afterIdFromDrop } from '@/lib/ordering'
import { GripVertical, MoreVertical } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { CoinLoader } from '@/components/CoinLoader'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PromptDialog } from '@/components/PromptDialog'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import { isNotEmpty, isValidBudgetFolderName } from '@/lib/validation'
import type {
  BudgetDto,
  BudgetFolderDto,
  BudgetPlanDto,
  PlanCellDto,
  PlanElementDto,
} from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, isPlannedType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { useIsCompact } from '@/hooks/useIsCompact'
import { CategoryDialog } from '@/features/classifications/CategoryDialog'
import { TagDialog } from '@/features/classifications/TagDialog'
import type { TagDialogItem } from '@/features/classifications/TagDialog'
import { useUpdateCategory } from '@/features/classifications/queries'
import { useAccounts } from '@/features/accounts/queries'
import { useUiStore } from '@/app/uiStore'
import { elementDisplayName, periodLabeler } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import { BudgetTransactionsDialog, TRANSFERS_TARGET_ID } from './BudgetTransactionsDialog'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import {
  canConfigureBudget,
  canDeleteEnvelope,
  canEditBudget,
  commentCellKey,
  planFetchWindow,
  useBudgetComments,
  useBudgetPlan,
  useChangeElementCurrency,
  useCreateBudgetFolder,
  useDeleteBudgetFolder,
  useDeleteEnvelope,
  useFillPlannedCells,
  useMoveBudgetFolder,
  useMoveElement,
  usePlanSetLimit,
  useUpdateBudgetFolder,
  useUpdateEnvelope,
} from './queries'
import { arrangementItem, moveElementInArrangement, placeElements } from './elementMove'
import type { ElementContainer } from './elementMove'
import { CommentsPanel } from './CommentsPanel'
import { ElementSheet } from './ElementSheet'
import { planCellFigures } from './phoneMonth'
import { elementEditAccess, isEnvelopeType } from './elementEdit'
import { EnvelopeDialog } from './EnvelopeDialog'
import { PlanCreateFolderDialog } from './PlanCreateFolderDialog'
import { limitAmountFromInput } from './limitAmount'
import { METRICS, trackEvent } from '@/lib/metrics'
import { SetLimitDialog } from './SetLimitDialog'
import {
  PLAN_ACTUALS_MIN_COL_PX,
  PLAN_NAME_COL_PX,
  addMonths,
  balanceRow,
  bucketPlanRows,
  everydayBalanceRow,
  fillTargetCol,
  folderSides,
  makePlanExchange,
  monthDate,
  monthDiff,
  planHasSavingsData,
  projectSavingsClosings,
  planMonthExchange,
  planGroupSums,
  planTotals,
  planVisibleCount,
  planWindow,
  savingsAsPlanElement,
  savingsBalanceRow,
} from './planMath'
import type { FolderSide, MonthExchange, PlanFolderSection, PlanRow, PlanRows } from './planMath'
import type { MenuAction } from './monthLayout'
import { LineControlsContext, LineLayoutContext, PLAN_FIGURE_COL, PLAN_LINE, PLAN_NAME_COL, PLAN_SELECTED_TINT, ROW_INDENT, RowLevelContext } from './monthLayout'
import { FigureCells, FolderLine, MonthSectionHeader } from './monthLines'
import { ElementRow, SumCell, cellDomId, commentsReadOnly, isEditableCell, selectedClass, sourceAmount } from './PlanRows'
import { PlanBalanceRow, PlanTotals } from './PlanTotalsLines'
import type { GridCtx, PlanCellEdit, PlanLimitTarget, PlanSelection } from './PlanRows'
import type { CellMove } from './PlanCellInput'

export interface PlanSheetProps {
  /** the ALREADY-LOADED budget (meta for permissions/currency); plan data is fetched inside */
  budget: BudgetDto
  currencies: CurrencyDto[]
  userId: Id | undefined
  editMode: boolean
}

const rowKey = (r: PlanRow): string => `${r.element.id}:${r.element.type}`
// A folder header is a selectable row of the grid too (fold/unfold by keyboard); it
// shares the selection's rowKey space with the element rows under a distinct prefix.
const folderRowKey = (folderId: Id): string => `pfolder:${folderId}`
const isFolderRowKey = (rk: string): boolean => rk.startsWith('pfolder:')

// A folder header has a single cell, so whatever column the selection carries (kept so
// Up/Down through a header lands back on the same month), its DOM cell is the -1 one.
const selectionDomId = (sel: PlanSelection): string => cellDomId(sel.rowKey, isFolderRowKey(sel.rowKey) ? -1 : sel.col)

type GroupSums = ReturnType<typeof planGroupSums>

const monthColClass = (col: number, selectedCol: number): string => `${PLAN_FIGURE_COL}${col === selectedCol ? ` ${PLAN_SELECTED_TINT}` : ''}`

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
// the actual's transactions link, a still-open popover's input, an open dropdown menu's items —
// must not steal focus back onto the grid; those controls already manage their own
// focus, and the grid regains it naturally once they close.
const CLICK_ESCAPE_SELECTOR = `button, [role="button"], ${KEYDOWN_ESCAPE_SELECTOR}`

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
  actions,
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
  actions?: ReactNode
  /** unfolded and holding no rows: the empty-folder hint shows instead of them */
  empty: boolean
  children: ReactNode
}) {
  const { t } = useTranslation()
  const rk = folderRowKey(foldKey)
  const selected = ctx.selection?.rowKey === rk
  return (
    <div data-testid={`plan-folder-${foldKey}`}>
      {/* FolderLine folds on a click anywhere on it; the same click selects the
          line, so the arrow keys pick up from here (ArrowLeft/ArrowRight
          fold/unfold, Up/Down walk the rows). Its own controls (the grip, the
          actions menu and its portalled items) keep their action and select nothing. */}
      <div
        role="row"
        className="border-b border-border/60"
        onClick={(e) => {
          const target = e.target as HTMLElement
          if (!e.currentTarget.contains(target)) {
            return
          }
          const control = target.closest('button, a, input, [role="menuitem"]')
          if (control && !control.hasAttribute('data-fold')) {
            return
          }
          ctx.select(rk, -1, e)
        }}
      >
        <FolderLine
          name={name}
          folded={folded}
          onToggle={() => onToggleFold(foldKey)}
          sums={sums}
          handle={handle}
          actions={actions}
          actionsColumn={false}
          nameCell={{ role: 'gridcell', id: cellDomId(rk, -1), 'aria-selected': selected, className: selectedClass(selected) }}
        />
      </div>
      {empty ? (
        // stacked over blank month cells, so the selected month's tint runs through it
        <div className="grid">
          <div className={`${PLAN_LINE} min-h-7 [grid-area:1/1]`}>
            <span className={PLAN_NAME_COL} />
            <FigureCells cells={ctx.visibleMonths.map(() => null)} />
          </div>
          <p className={`${ROW_INDENT['in-folder']} px-2 py-1 text-xs text-muted-foreground [grid-area:1/1]`}>
            {t('budgets.page.budget.structure.empty_folder.note')}
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
  onRename,
  onDelete,
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
  onRename: (folder: BudgetFolderDto) => void
  onDelete: (folder: BudgetFolderDto) => void
}) {
  const { t } = useTranslation()
  const visibleRows = collapsed || folded ? [] : section.rows
  return (
    <FolderGroup
      foldKey={section.folder.id}
      name={section.folder.name}
      ctx={ctx}
      sums={sums}
      folded={folded}
      onToggleFold={onToggleFold}
      handle={ctx.editMode ? <PlanFolderGrip name={section.folder.name} /> : null}
      actions={
        ctx.editMode ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" variant="ghost" size="icon" className="size-7" aria-label={`budget folder actions ${section.folder.name}`}>
                <MoreVertical className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => onRename(section.folder)}>{t('common.button.edit.label')}</DropdownMenuItem>
              {/* same rule as the budget view: only a member-less folder is deletable here —
                  the server would drop a populated one and strand its members */}
              {section.rows.length === 0 ? (
                <DropdownMenuItem variant="destructive" onSelect={() => onDelete(section.folder)}>
                  {t('budgets.page.budget.structure.action.delete_folder')}
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null
      }
      empty={!folded && !collapsed && section.rows.length === 0}
    >
      <PlanRowList rows={visibleRows} ctx={ctx} />
    </FolderGroup>
  )
}

// Same flattening the renderer walks (folders -> loose, income then savings then
// neutral folders then expense, then archived), so Up/Down can never reach a row that isn't on
// screen. A folder contributes its header (a selectable row of its own, so it can be
// folded/unfolded by keyboard) followed by its visible members. Only root element
// rows — the ones a limit can be set on — are in the order: an expanded envelope's
// children are read-only breakdown lines and are stepped over.
type FlatRow = { kind: 'element'; rowKey: string; el: PlanElementDto } | { kind: 'folder'; rowKey: string; foldKey: string }

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

/** a section names its folder-less rows only next to real folders, as the Budget view does */
const hasNoFolderLine = (band: PlanRows['income']): boolean => band.folders.length > 0 && band.loose.length > 0

function buildFlatRows(rows: PlanRows, savingsRows: PlanRow[], folded: (key: string) => boolean): FlatRow[] {
  const flatRows: FlatRow[] = []
  const pushRow = (r: PlanRow) => {
    flatRows.push({ kind: 'element', rowKey: rowKey(r), el: r.element })
  }
  const pushGroup = (foldKey: string, groupRows: PlanRow[]) => {
    flatRows.push({ kind: 'folder', rowKey: folderRowKey(foldKey), foldKey })
    if (!folded(foldKey)) {
      groupRows.forEach(pushRow)
    }
  }
  const pushFolder = (f: PlanFolderSection) => pushGroup(f.folder.id, f.rows)
  const pushSide = (side: 'income' | 'expense') => {
    const band = rows[side]
    band.folders.forEach(pushFolder)
    if (hasNoFolderLine(band)) {
      pushGroup(NO_FOLDER_KEY[side], band.loose)
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
  rows.neutral.forEach(pushFolder)
  if (!folded('expense')) {
    pushSide('expense')
  }
  if (rows.archived.length > 0 && !folded('archived')) {
    rows.archived.forEach(pushRow)
  }
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
  const [sheetCellTarget, setSheetCellTarget] = useState<PlanLimitTarget | null>(null)
  const openSheet = useCallback((target: PlanLimitTarget) => setSheetCellTarget(target), [])
  const [dragArrangement, setDragArrangement] = useState<ElementContainer[] | null>(null)
  const [draggingFolder, setDraggingFolder] = useState(false)
  const [moveFolderTarget, setMoveFolderTarget] = useState<PlanElementDto | null>(null)
  const [currencyTarget, setCurrencyTarget] = useState<PlanElementDto | null>(null)
  const [createFolderOpen, setCreateFolderOpen] = useState(false)
  const [envelopeTarget, setEnvelopeTarget] = useState<PlanElementDto | null>(null)
  const [deleteEnvelopeTarget, setDeleteEnvelopeTarget] = useState<PlanElementDto | null>(null)
  const [categoryTarget, setCategoryTarget] = useState<Pick<CategoryDto, 'id' | 'name' | 'type' | 'icon'> | null>(null)
  const [tagTarget, setTagTarget] = useState<TagDialogItem | null>(null)
  const [renameFolderTarget, setRenameFolderTarget] = useState<BudgetFolderDto | null>(null)
  const [deleteFolderTarget, setDeleteFolderTarget] = useState<BudgetFolderDto | null>(null)
  // the open comment thread: anchored to its cell on desktop/tablet, a sheet on a phone
  const [commentsDialogTarget, setCommentsDialogTarget] = useState<(PlanLimitTarget & { anchor: HTMLElement | null }) | null>(null)
  const commentsOpen = commentsDialogTarget !== null
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
  const updateFolder = useUpdateBudgetFolder()
  const deleteFolder = useDeleteBudgetFolder()
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
  const editorOpen =
    envelopeTarget !== null || categoryTarget !== null || tagTarget !== null || commentsDialogTarget !== null || planLimitTarget !== null
  useEffect(() => {
    if (!editorOpen && editorFromGrid.current) {
      editorFromGrid.current = false
      containerRef.current?.focus()
    }
  }, [editorOpen])
  // ResizeObserver never fires in jsdom, so width stays 0 there — the same
  // floor a real narrow viewport would collapse to (planVisibleCount<3 -> 1).
  const visible = width > 0 ? planVisibleCount(width) : 3
  // jsdom's 0 width keeps every actual, as a wide screen would
  const showActuals = width === 0 || (width - PLAN_NAME_COL_PX) / visible >= PLAN_ACTUALS_MIN_COL_PX

  const startedAt = budget.meta.startedAt
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const stepPeriod = useBudgetPeriodStore((s) => s.stepPeriod)
  const planFolds = useBudgetPeriodStore((s) => s.planFolds)
  const togglePlanFold = useBudgetPeriodStore((s) => s.togglePlanFold)
  const folded = useCallback((key: string): boolean => !!planFolds[key], [planFolds])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const unfoldedElements = useBudgetPeriodStore((s) => s.unfoldedElements)
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
  const { first: firstMonth } = planWindow(selectedDate, visible, startedAt, budget.meta.endedAt)
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
  // the plan's OWN window, so both caches cover exactly the same months
  const { byCell: commentsByCell, truncated: commentsTruncated } = useBudgetComments(budget.meta.id, fetchFrom, planFetchWindow(firstMonth, visible).months)

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
  // by month, not by planWindow's column: a stored month outside the budget (before
  // its start, after its end) has no column to tint
  const selectedCol = visibleMonths.indexOf(selectedDate)
  // An editor whose month left the window (a resize, the strip) closes rather than
  // reappearing, stale, when that month scrolls back in. Leaving its cell by click or
  // key has already committed it.
  useEffect(() => {
    if (editing && !visibleMonths.includes(editing.month)) {
      setEditing(null)
    }
  }, [editing, visibleMonths])
  const layout = useMemo(() => ({ kind: 'plan' as const, cols: visible, selectedCol }), [visible, selectedCol])
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
    (el: PlanElementDto, month: string) =>
      setTransactionsTarget({
        target: { id: el.id, type: el.type, name: elementDisplayName(el.id, el.name, t), icon: el.icon, currencyId: el.currencyId },
        month,
      }),
    [t],
  )
  // same wording as the budget view's period strip
  const monthLabel = useMemo(() => {
    const label = periodLabeler(i18n.language)
    return (m: string) => label(monthDate(m))
  }, [i18n.language])
  const canEdit = canEditBudget(budget.meta, userId)
  const { data: accounts = [] } = useAccounts()
  const openAccountModal = useUiStore((s) => s.openAccountModal)
  const sheetEdit = sheetCellTarget ? elementEditAccess(sheetCellTarget.el, userId, canEdit && budget.meta.isArchived === 0, accounts) : null
  // the sheet's pencil: the element's own edit dialog replaces the sheet
  const editFromSheet = (el: PlanElementDto) => {
    setSheetCellTarget(null)
    if (isEnvelopeType(el.type)) {
      setEnvelopeTarget(el)
    } else if (el.type === BudgetElementType.SAVINGS) {
      const account = accounts.find((a) => a.id === el.id)
      if (account) {
        openAccountModal({ account })
      }
    } else if (el.type === BudgetElementType.TAG) {
      setTagTarget({ id: el.id, name: el.name, kind: 'tag', icon: el.icon })
    } else {
      setCategoryTarget({ id: el.id, name: el.name, icon: el.icon, type: isIncomeType(el.type) ? 'income' : 'expense' })
    }
  }
  const canDeleteEnvelopes = canDeleteEnvelope(budget.meta, userId)
  // edit mode's element actions; the hover menus outside it come with the shared Budget-view menus
  const rowMenu = useMemo(() => {
    if (!editMode) {
      return undefined
    }
    return (el: PlanElementDto): MenuAction[] | undefined => {
      if (el.id === UNCATEGORIZED_ID) {
        return undefined
      }
      const actions: MenuAction[] = [{ label: t('budgets.page.budget.structure.element.action.change_currency'), onSelect: () => setCurrencyTarget(el) }]
      // a savings row lives in its own section and never in a folder (the server
      // refuses one with budget.savings_folder_not_allowed)
      if (el.type !== BudgetElementType.SAVINGS) {
        actions.push({ label: t('budgets.page.plan.menu.move_to_folder'), onSelect: () => setMoveFolderTarget(el) })
      }
      // The budget view's wire response strips income envelopes and income-sided
      // folders, so the plan sheet is the only surface where an income envelope is
      // reachable: Edit/Delete must live here or one could never be changed.
      if (isEnvelopeType(el.type)) {
        actions.push({ label: t('common.button.edit.label'), onSelect: () => setEnvelopeTarget(el) })
        if (canDeleteEnvelopes) {
          actions.push({ label: t('common.button.delete.label'), onSelect: () => setDeleteEnvelopeTarget(el), destructive: true })
        }
      }
      return actions
    }
  }, [editMode, canDeleteEnvelopes, t])
  const folderNameValidator = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('budgets.form.budget.folder_name.validation.required_field')
    }
    if (!isValidBudgetFolderName(value)) {
      return t('budgets.form.budget.folder_name.validation.invalid_name')
    }
    return null
  }

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
    if (!dragArrangement) {
      return bucketPlanRows(plan)
    }
    const structure = { ...plan.structure, elements: placeElements(plan.structure.elements, dragArrangement) }
    return bucketPlanRows({ ...plan, structure })
  }, [plan, dragArrangement])

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
    () => (shownRows ? buildFlatRows(shownRows, savingsRows, folded) : []),
    [shownRows, savingsRows, folded],
  )
  const folderSideMap = useMemo(() => (plan ? folderSides(plan) : new Map<Id, FolderSide>()), [plan])

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
    sums.set('savings', sumOf(savingsRows))
    sums.set('archived', sumOf(shownRows.archived))
    return sums
  }, [shownRows, savingsRows, visibleMonths, monthIndex, ex])

  const ctx: GridCtx | null = useMemo(() => {
    if (!plan) {
      return null
    }
    return {
      visibleMonths,
      monthIndex,
      selected: selectedDate,
      selectedCol,
      showActuals,
      currencies,
      baseCurrencyId: budget.meta.currencyId,
      meta: budget.meta,
      userId,
      isCompact,
      monthLabel,
      commit,
      openSheet,
      openTransactions,
      commentsByCell,
      commentsTruncated,
      openComments,
      commentsOpen,
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
      editing,
      startEdit,
      finishEdit,
      cancelEdit,
      rowMenu,
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    plan,
    visibleMonths,
    monthIndex,
    selectedDate,
    selectedCol,
    showActuals,
    currencies,
    budget.meta,
    userId,
    isCompact,
    monthLabel,
    commit,
    commentsByCell,
    commentsTruncated,
    openComments,
    openTransactions,
    commentsOpen,
    canEdit,
    selection,
    select,
    fillDrag,
    fillStart,
    fillMove,
    fillEnd,
    fillCancel,
    editMode,
    editing,
    startEdit,
    finishEdit,
    cancelEdit,
    rowMenu,
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

  const incomeFolded = folded('income')
  const expenseFolded = folded('expense')
  // the folder-less rows on screen: none while their section, or their No folder line, is folded
  const looseShown = (side: 'income' | 'expense', sectionFolded: boolean): PlanRow[] =>
    sectionFolded || (hasNoFolderLine(shownRows[side]) && folded(NO_FOLDER_KEY[side])) ? [] : shownRows[side].loose
  const incomeLoose = looseShown('income', incomeFolded)
  const expenseLoose = looseShown('expense', expenseFolded)

  const savingsFolded = folded('savings')
  const savingsLive = savingsRows.filter(isDraggableRow)
  const savingsDeleted = savingsRows.filter((r) => !isDraggableRow(r))

  const fmtBudget = (v: string) => moneyFormat(v, planCurrency, { showCurrency: false, useNativePrecision: false })
  const sumCells = (sums: GroupSums | undefined): ReactNode[] =>
    visibleMonths.map((m, i) => (
      <SumCell key={m} index={i} sum={sums && monthIndex(m) >= 0 ? sums[i] : null} month={m} ctx={ctx} fmt={fmtBudget} />
    ))

  // a section's folder-less rows: under a No folder line next to real folders, at
  // that line's step on their own otherwise
  const looseGroup = (side: 'income' | 'expense', rows: PlanRow[]) => {
    const list = editMode ? <LooseRowsContainer rows={rows} ctx={ctx} /> : <PlanRowList rows={rows} ctx={ctx} />
    if (!hasNoFolderLine(shownRows[side])) {
      return <RowLevelContext.Provider value="top">{list}</RowLevelContext.Provider>
    }
    const key = NO_FOLDER_KEY[side]
    return (
      <FolderGroup
        foldKey={key}
        name={t('budgets.page.plan.menu.no_folder')}
        ctx={ctx}
        sums={sumCells(groupSums.get(key))}
        folded={folded(key)}
        onToggleFold={togglePlanFold}
        empty={false}
      >
        {list}
      </FolderGroup>
    )
  }

  // The band's element buckets as the arrangement elementMove.ts operates on:
  // one container per folder plus the loose rows. Uncategorized and archived
  // rows are excluded — they carry no position the server would honour.
  //
  // Mirrors the RENDER-side filtering exactly (FolderRows' fold, and the
  // incomeLoose/expenseLoose above for the loose bucket): only rows actually on
  // screen can be under the pointer during a drag, so afterId must be read from that
  // same set — anchoring to a folded-away row would silently place the moved element
  // after something the user never saw.
  function bandArrangement(side: 'income' | 'expense'): ElementContainer[] {
    const band = shownRows![side]
    const loose = side === 'income' ? incomeLoose : expenseLoose
    return [
      ...band.folders.map((f) => ({
        folderId: f.folder.id as Id | null,
        ids: (folded(f.folder.id) ? [] : f.rows)
          .filter(isDraggableRow)
          .map((r) => r.element.id),
      })),
      { folderId: null as Id | null, ids: loose.filter(isDraggableRow).map((r) => r.element.id) },
    ]
  }

  function handleBandDragStart(event: DragStartEvent) {
    setDraggingFolder(String(event.active.id).startsWith('pfolder:'))
  }

  function handleBandDragEnd(side: 'income' | 'expense' | 'neutral', event: DragEndEvent) {
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
    if (side === 'neutral') {
      // the neutral band holds only header-only folders — no rows to move
      return
    }

    // a row dropped on a folder header lands in that folder, appended
    const target = overId.startsWith('pfolder:') ? `bfolder:${overId.slice('pfolder:'.length)}` : overId
    commitElementMove(bandArrangement(side), activeId, target)
  }

  // The savings band is one folder-less list: the only valid target is another
  // live savings row, so the move always carries folderId null.
  function handleSavingsDragEnd(event: DragEndEvent) {
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
  function openElementEditor(entry: Extract<FlatRow, { kind: 'element' }>) {
    const target = entry.el
    // a savings row is an account, edited from the accounts screen, not here
    if (target.id === UNCATEGORIZED_ID || target.type === BudgetElementType.SAVINGS) {
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
    if (entry.kind === 'folder') {
      togglePlanFold(entry.foldKey)
      return
    }
    if (col === -1) {
      openElementEditor(entry)
      return
    }
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

  // The element row under the roving selection (null for none / a folder row), and
  // that row's month cell as the clipboard and keyboard-fill actions need it (null on
  // the name cell too).
  function selectedElementRow(): (FlatRow & { kind: 'element' }) | null {
    const entry = selection ? flatRows.find((r) => r.rowKey === selection.rowKey) : undefined
    return entry?.kind === 'element' ? entry : null
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
    const text = selection.col === -1 ? entry.el.name : cell ? sourceAmount(cell.entry.el, cell.idx) : null
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
  // and the name cell arm nothing.
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
  // window's edges ← from the name cell and → from the last column page the window by
  // a month (clamped at the budget's start and end); the column stays put.
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
        if (from.col === -1) {
          // -1 is the leftmost reachable column, so ← here shifts the window instead
          // of going nowhere: the name cell stays reachable while the window can
          // still be paged from it
          if (!atStart) {
            shiftWindow(-1)
          }
          select(from.rowKey, -1)
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
        commit(ed.elementId, ed.month, ed.monthIndex, parsed.amount)
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
    if (!cell || !isEditableCell(cell.entry.el, cell.month, cell.idx, budget.meta, userId)) {
      return
    }
    if (unsetPlan(cell.entry.el.cells[cell.idx]?.planned ?? '')) {
      return
    }
    setLimit.mutate(
      { budgetId: budget.meta.id, elementId: cell.entry.el.id, period: cell.month, amount: null, monthIndex: cell.idx },
      { onSuccess: () => trackEvent(METRICS.BUDGET_PLAN_CLEAR_CELL) },
    )
  }

  function handleKeyDown(e: KeyboardEvent<HTMLDivElement>) {
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
    // amount dialog, any other dialog, or an open row menu gets its Arrow/Enter keys
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
    // arms (name cell, non-editable source) so the selection never jumps under a held
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
    // Left/Right on a highlighted folder header fold/unfold it and never move the
    // selection or page the window — a header has no month cells to walk. Enter and
    // Space toggle it too (Enter has no edit action on a folder outside its menu).
    if (entry.kind === 'folder') {
      const isFolded = folded(entry.foldKey)
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
          if (!isFolded) {
            togglePlanFold(entry.foldKey)
          }
          break
        case 'ArrowRight':
          e.preventDefault()
          if (isFolded) {
            togglePlanFold(entry.foldKey)
          }
          break
        case 'Enter':
        case ' ':
          e.preventDefault()
          togglePlanFold(entry.foldKey)
          break
        default:
          break
      }
      return
    }
    // On a highlighted name cell of a row with children, Left/Right fold/unfold the
    // breakdown first: ArrowRight expands a collapsed row (and only then walks into
    // the months), ArrowLeft collapses an expanded one (and only then pages the window).
    const expandable = entry.el.children.length > 0
    const unfolded = !!unfoldedElements[entry.el.id]
    // A month cell edits like a spreadsheet: typing replaces the value, F2 edits it,
    // Delete/Backspace clears it. Read-only cells take none of it (startEdit and
    // clearSelectedCell both check).
    if (selection.col >= 0 && !e.ctrlKey && !e.metaKey && !e.altKey) {
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
        if (selection.col === -1 && expandable && unfolded) {
          toggleElement(entry.el.id)
          break
        }
        moveSelection(selection, 'left')
        break
      case 'ArrowRight':
        e.preventDefault()
        if (selection.col === -1 && expandable && !unfolded) {
          toggleElement(entry.el.id)
          break
        }
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
      case ' ':
        // Space on the name cell folds/unfolds the element's children (Enter is the
        // edit shortcut); preventDefault so the scroller does not page down.
        if (selection.col === -1) {
          e.preventDefault()
          if (expandable) {
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
        data-testid="plan-sheet"
      >
        <LineLayoutContext.Provider value={layout}>
        <LineControlsContext.Provider value={editMode ? 'always' : 'hover'}>
        <div role="row" data-testid="plan-month-header" className={`sticky top-0 z-20 ${PLAN_LINE} border-b bg-background`}>
          <span className={PLAN_NAME_COL} />
          {visibleMonths.map((m, i) => {
            const selected = i === selectedCol
            return (
              <div
                key={m}
                role="columnheader"
                data-month={m}
                data-col={i}
                data-selected-col={selected ? 'true' : undefined}
                className={`${monthColClass(i, selectedCol)} py-1.5 text-[10.5px] uppercase tracking-wider ${selected ? 'text-foreground' : 'text-muted-foreground'}`}
              >
                {monthLabel(m)}
              </div>
            )
          })}
        </div>
        {/* Sections sit flush, split by a hairline, so the selected month's tint runs
            unbroken from the month header down to the Balance line. */}
        <section role="rowgroup" data-testid="plan-section-income" className="plan-band plan-band-income flex flex-col">
          <MonthSectionHeader
            foldKey="income"
            testId="plan-section-line-income"
            label={t('budgets.page.plan.section.income')}
            headings={[]}
            sums={sumCells(groupSums.get('income'))}
            actionsColumn={false}
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
                    sums={f.rows.length > 0 ? sumCells(groupSums.get(f.folder.id)) : null}
                    folded={folded(f.folder.id)}
                    collapsed={draggingFolder}
                    onToggleFold={togglePlanFold}
                    onRename={setRenameFolderTarget}
                    onDelete={setDeleteFolderTarget}
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
              {looseGroup('income', incomeLoose)}
              <RowLevelContext.Provider value="top">
                {shownRows.income.uncategorized ? (
                  <ElementRow key={rowKey(shownRows.income.uncategorized)} row={shownRows.income.uncategorized} ctx={ctx} />
                ) : null}
              </RowLevelContext.Provider>
            </PlanBand>
          ) : null}
        </section>

        {hasSavings ? (
          // Its own drag context: a savings row reorders among savings rows only and
          // can never reach a folder, which the server refuses for it anyway.
          <section role="rowgroup" data-testid="plan-section-savings" className="plan-band plan-band-savings flex flex-col border-t">
            <MonthSectionHeader
              foldKey="savings"
              testId="plan-section-line-savings"
              label={t('budgets.page.plan.section.savings')}
              headings={[]}
              sums={sumCells(groupSums.get('savings'))}
              actionsColumn={false}
            />
            {!savingsFolded ? (
              <RowLevelContext.Provider value="top">
                <PlanBand
                  editMode={editMode}
                  sensors={sensors}
                  folderIds={[]}
                  onDragStart={handleBandDragStart}
                  onDragEnd={handleSavingsDragEnd}
                  onDragCancel={() => setDraggingFolder(false)}
                >
                  <PlanRowList rows={savingsLive} ctx={ctx} />
                </PlanBand>
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
          <section role="rowgroup" data-testid="plan-section-neutral" className="plan-band plan-band-neutral flex flex-col border-t">
            <PlanBand
              editMode={editMode}
              sensors={sensors}
              folderIds={shownRows.neutral.map((f) => f.folder.id)}
              onDragStart={handleBandDragStart}
              onDragEnd={(e) => handleBandDragEnd('neutral', e)}
              onDragCancel={() => setDraggingFolder(false)}
            >
              {shownRows.neutral.map((f) => {
                const section = (
                  <FolderRows
                    section={f}
                    ctx={ctx}
                    sums={f.rows.length > 0 ? sumCells(groupSums.get(f.folder.id)) : null}
                    folded={folded(f.folder.id)}
                    collapsed={draggingFolder}
                    onToggleFold={togglePlanFold}
                    onRename={setRenameFolderTarget}
                    onDelete={setDeleteFolderTarget}
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
            </PlanBand>
          </section>
        ) : null}

        <section role="rowgroup" data-testid="plan-section-expense" className="plan-band plan-band-expense flex flex-col border-t">
          <MonthSectionHeader
            foldKey="expense"
            testId="plan-section-line-expense"
            label={t('budgets.page.plan.section.expenses')}
            headings={[]}
            sums={sumCells(groupSums.get('expense'))}
            actionsColumn={false}
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
                    sums={f.rows.length > 0 ? sumCells(groupSums.get(f.folder.id)) : null}
                    folded={folded(f.folder.id)}
                    collapsed={draggingFolder}
                    onToggleFold={togglePlanFold}
                    onRename={setRenameFolderTarget}
                    onDelete={setDeleteFolderTarget}
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
              {looseGroup('expense', expenseLoose)}
              <RowLevelContext.Provider value="top">
                {shownRows.expense.uncategorized ? (
                  <ElementRow key={rowKey(shownRows.expense.uncategorized)} row={shownRows.expense.uncategorized} ctx={ctx} />
                ) : null}
              </RowLevelContext.Provider>
            </PlanBand>
          ) : null}
        </section>

        {shownRows.archived.length > 0 ? (
          <section role="rowgroup" data-testid="plan-section-archived" className="plan-band plan-band-archived flex flex-col border-t">
            <MonthSectionHeader
              foldKey="archived"
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
        target={sheetCellTarget ? { kind: 'plan', cell: planCellFigures(sheetCellTarget.el, sheetCellTarget.monthIndex) } : null}
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
        onEdit={sheetCellTarget && sheetEdit !== null ? () => editFromSheet(sheetCellTarget.el) : undefined}
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
        onSubmit={({ name, side, memberIds }) => {
          const id = uuidv7()
          createFolder.mutate(
            { budgetId: budget.meta.id, id, name, side },
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

      <PromptDialog
        open={renameFolderTarget !== null}
        onClose={() => setRenameFolderTarget(null)}
        onSubmit={(name) => {
          if (renameFolderTarget) {
            updateFolder.mutate({ budgetId: budget.meta.id, id: renameFolderTarget.id, name }, { onSuccess: () => setRenameFolderTarget(null) })
          }
        }}
        title={t('budgets.modal.update_folder_form.header')}
        inputLabel={t('budgets.form.budget.folder_name.label')}
        initialValue={renameFolderTarget?.name ?? ''}
        validate={folderNameValidator}
        submitLabel={t('common.button.update.label')}
        cancelLabel={t('common.button.cancel.label')}
      />

      <ConfirmDialog
        open={deleteFolderTarget !== null}
        onClose={() => setDeleteFolderTarget(null)}
        onConfirm={() => {
          if (deleteFolderTarget) {
            deleteFolder.mutate({ budgetId: budget.meta.id, id: deleteFolderTarget.id }, { onSettled: () => setDeleteFolderTarget(null) })
          }
        }}
        title={t('budgets.modal.delete_folder.header')}
        question={t('budgets.modal.delete_folder.question', { name: deleteFolderTarget?.name ?? '' })}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
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
