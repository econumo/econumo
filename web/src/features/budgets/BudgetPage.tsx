import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactElement, ReactNode } from 'react'
import { DndContext, MeasuringStrategy, PointerSensor, pointerWithin, rectIntersection, useDroppable, useSensor, useSensors } from '@dnd-kit/core'
import type { CollisionDetection, DragEndEvent, DragOverEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { snapRowToPointer } from '@/lib/dnd'
import { afterIdFromDrop } from '@/lib/ordering'
import type { SortableHandleProps } from '@/components/SortableList'
import { Check, ChevronLeft, FolderPlus, GripVertical, MoreVertical, Plus, Settings2 } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { InfoBox } from '@/components/InfoBox'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LogoutEscapeButton } from '@/features/auth/LogoutEscapeButton'
import { PromptDialog } from '@/components/PromptDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useIsPhone } from '@/hooks/useIsPhone'
import { useLogoutEscape } from '@/hooks/useLogoutEscape'
import { useScrollMemory } from '@/hooks/useScrollMemory'
import { isNotEmpty, isValidBudgetFolderName } from '@/lib/validation'
import type { BudgetElementDto } from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { Id } from '@/api/types'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userOption } from '@/features/user/queries'
import { UserOptions } from '@/api/dto/user'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useUpdateCategory } from '@/features/classifications/queries'
import { CategoryDialog } from '@/features/classifications/CategoryDialog'
import { TagDialog } from '@/features/classifications/TagDialog'
import type { TagDialogItem } from '@/features/classifications/TagDialog'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import {
  useBudget,
  useBudgets,
  useBudgetComments,
  useBudgetPlan,
  usePlanSetLimit,
  useSetLimit,
  useCreateEnvelope,
  useUpdateEnvelope,
  useDeleteEnvelope,
  useCreateBudgetFolder,
  useUpdateBudgetFolder,
  useDeleteBudgetFolder,
  useMoveBudgetFolder,
  useMoveElement,
  useChangeElementCurrency,
  canConfigureBudget,
  canEditBudget,
  canUpdateLimits,
  canDeleteEnvelope,
  commentCellKey,
} from './queries'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetMode } from './budgetStore'
import { bucketElements, budgetTotals, elementDisplayName, makeBudgetExchange } from './budgetMath'
import type { FolderBucket } from './budgetMath'
import { currentMonth, monthDiff } from './planMath'
import { BudgetTable, BudgetTotals } from './BudgetTable'
import { PeriodStrip } from './PeriodStrip'
import { PlanSheet, commentsReadOnly } from './PlanSheet'
import { LimitEditor } from './LimitEditor'
import { SetLimitDialog } from './SetLimitDialog'
import { CommentMarker } from './CommentThread'
import { CommentsPanel } from './CommentsPanel'
import { CellShell } from './CellShell'
import { ElementSheet } from './ElementSheet'
import { planMonthFigures, sheetCell, sheetElement, sheetSetsPlan, type SheetTarget } from './phoneMonth'
import { PhoneMonthView } from './PhoneMonthView'
import { ViewSwitch } from './ViewSwitch'
import { MonthFlows, MonthTotalsLines } from './MonthFlows'
import type { FlowTarget } from './MonthFlows'
import { COMMENT_ANCHOR_ATTR, commentAnchorOf } from './cellDom'
import { EnvelopeDialog } from './EnvelopeDialog'
import type { EnvelopeDialogTarget } from './EnvelopeDialog'
import { elementEditAccess, isEnvelopeType } from './elementEdit'
import { BudgetUpdateDialog } from './BudgetUpdateDialog'
import { BudgetTransactionsDialog } from './BudgetTransactionsDialog'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { BudgetDialog } from './BudgetDialog'
import { useCreateBudget } from './queries'
import type { ElementContainer } from './elementMove'
import { applyArrangement, arrangementFromBuckets, arrangementItem, moveElementInArrangement } from './elementMove'
import { CoinLoader } from '@/components/CoinLoader'
import { METRICS, trackEvent } from '@/lib/metrics'

function DraggableElement({ id, children }: { id: string; children: ReactNode }) {
  // sortable row (accounts-settings pattern): the whole row moves with the
  // drag transform, the grip is just the activation handle
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging ? 'opacity-60' : undefined}
    >
      {/* items-start + fixed grip offset: an unfolded element grows downwards,
          the grip must stay centered on the ROOT row, not the whole block */}
      <div className="flex items-start gap-1">
        <button
          type="button"
          aria-label={`move ${id}`}
          className="mt-3 cursor-grab touch-none text-muted-foreground"
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-4" />
        </button>
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </div>
  )
}

// Rows are nested inside their section droppable, and the dragged row itself
// travels under the pointer (its own rect always wins a pointer test) — so:
// ignore the active row, prefer whatever OTHER row the pointer is inside, and
// fall back to sections (empty folders, gaps between rows).
const preferRowCollisions: CollisionDetection = (args) => {
  const collisions = pointerWithin(args)
  const candidates = (collisions.length > 0 ? collisions : rectIntersection(args)).filter((c) => c.id !== args.active.id)
  const row = candidates.find((c) => !String(c.id).startsWith('bfolder:'))
  return row ? [row] : candidates
}

type CellTarget = Pick<BudgetElementDto, 'id' | 'name' | 'budgeted'>

const BUDGET_MODE_ROUTE: Record<BudgetMode, string> = {
  budget: RouterPage.BUDGET,
  plan: RouterPage.PLAN,
}

// The section is a sortable item itself (folder reorder); the grip lives in
// the header rendered by BudgetTable, so the handle props travel via context.
const FolderHandleContext = createContext<SortableHandleProps | null>(null)

function FolderGrip({ name }: { name: string }) {
  const handle = useContext(FolderHandleContext)
  if (!handle) {
    return null
  }
  return (
    <button
      type="button"
      aria-label={`move folder ${name}`}
      // cancel the header's inner padding so folder grips line up with row grips
      className="-ml-1.5 cursor-grab touch-none text-muted-foreground sm:-ml-2"
      {...handle.attributes}
      {...(handle.listeners ?? {})}
    >
      <GripVertical className="size-4" />
    </button>
  )
}

function SortableSection({
  bucket,
  id,
  highlighted,
  folderDragging,
  children,
}: {
  bucket: FolderBucket
  id: string
  highlighted: boolean
  /** a folder drag is in flight: element drop zones pause */
  folderDragging: boolean
  children: ReactNode
}) {
  // real folders are sortable; the default bucket only receives elements
  const sortable = useSortable({ id: bucket.folder?.id ?? '__no_folder__', disabled: !bucket.folder })
  const { setNodeRef: setDroppableRef, isOver } = useDroppable({ id, disabled: folderDragging })
  return (
    <div
      ref={(el) => {
        sortable.setNodeRef(el)
        setDroppableRef(el)
      }}
      style={{ transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }}
      className={`${isOver || highlighted ? 'rounded-md ring-2 ring-ring' : ''} ${sortable.isDragging ? 'opacity-60' : ''}`}
    >
      <FolderHandleContext.Provider value={bucket.folder ? { attributes: sortable.attributes, listeners: sortable.listeners } : null}>
        <SortableContext items={bucket.elements.map((el) => el.id)} strategy={verticalListSortingStrategy}>
          {children}
        </SortableContext>
      </FolderHandleContext.Provider>
    </div>
  )
}


export function BudgetPage({ mode }: { mode: BudgetMode }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const isPhone = useIsPhone()
  const { data: user } = useUserData()
  // isPending covers the whole cold boot (incl. the disabled phase while the
  // user record loads); month switches show the previous period as placeholder
  // data (isPlaceholderData) while the new one loads
  const { data: budget, isPending, isPlaceholderData, isFetching, isError, error, refetch } = useBudget()
  const { data: budgetList } = useBudgets()
  const showLogoutEscape = useLogoutEscape(isPending)
  const { data: currencies = [] } = useCurrencies()
  const { data: accounts = [] } = useAccounts()
  const { data: categories = [] } = useCategories()
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const planHideEmpty = useBudgetPeriodStore((s) => s.planHideEmpty)
  const togglePlanHideEmpty = useBudgetPeriodStore((s) => s.togglePlanHideEmpty)
  const openAccountModal = useUiStore((s) => s.openAccountModal)
  const [editMode, setEditMode] = useState(false)
  const phoneView = isPhone && !editMode
  // the monthly view's own one-month window (also the phone view's, on both
  // routes); PlanSheet fetches its own — passing null on that route skips the
  // fetch instead of duplicating it. Keyed off the user's stored default budget
  // id, not `budget.meta.id`: waiting on the budget fetch to resolve first would
  // chain the comments fetch behind it instead of firing both together.
  // Latent coupling: every comment writer and CommentThread key off `budget.meta.id`
  // (and budgetCommentsFilter matches the query cache on that same id), not this
  // option-derived value. The two cannot diverge today only because useBudget() reads
  // this identical option internally — if it ever gains a non-option budget source,
  // this derivation must follow, or the markers below silently stop tracking writes.
  const budgetId = userOption(user, UserOptions.BUDGET)
  const { byCell: commentsByCell, truncated: commentsTruncated } = useBudgetComments(mode === 'budget' || phoneView ? budgetId : null, selectedDate, 1)
  // an element or a savings row: both dialogs need only the cell's id and name
  const [commentsTarget, setCommentsTarget] = useState<{ el: CellTarget; anchor: HTMLElement | null } | null>(null)
  const openComments = (el: CellTarget, anchor: HTMLElement | null = null) => setCommentsTarget({ el, anchor })

  const setLimit = useSetLimit()
  const createEnvelope = useCreateEnvelope()
  const updateEnvelope = useUpdateEnvelope()
  const updateCategory = useUpdateCategory()
  const deleteEnvelope = useDeleteEnvelope()
  const createFolder = useCreateBudgetFolder()
  const updateFolder = useUpdateBudgetFolder()
  const deleteFolder = useDeleteBudgetFolder()
  const orderFolders = useMoveBudgetFolder()
  const moveElement = useMoveElement()
  const changeCurrency = useChangeElementCurrency()
  const createBudget = useCreateBudget()
  // the month view's income, Balance and Total savings (phone and desktop alike): the
  // Plan view's own figures for the selected month; null until that month's window
  // has really loaded.
  // The window starts no later than the current month: the server books only
  // what precedes the window, so a future month's Balance needs every unmet plan
  // from the current month on inside it.
  const monthPlanFirst = selectedDate < currentMonth() ? selectedDate : currentMonth()
  const monthPlan = useBudgetPlan(mode === 'budget' || phoneView ? budgetId : null, monthPlanFirst, monthDiff(monthPlanFirst, selectedDate) + 1)
  const planSetLimit = usePlanSetLimit(monthPlan.planKey)
  const planMonth = useMemo(
    () => (monthPlan.data && !monthPlan.isPlaceholderData ? planMonthFigures(monthPlan.data, currencies, selectedDate) : null),
    [monthPlan.data, monthPlan.isPlaceholderData, currencies, selectedDate],
  )

  const setLastMode = useBudgetPeriodStore((s) => s.setLastMode)
  useEffect(() => {
    setLastMode(mode)
    if (mode === 'plan') {
      trackEvent(METRICS.BUDGET_PLAN_OPEN)
    }
  }, [mode, setLastMode])
  // the two views are separate routes; the keyed remount ends edit structure
  const switchBudgetMode = (m: BudgetMode) => {
    if (m !== mode) {
      navigate(BUDGET_MODE_ROUTE[m])
    }
  }
  // phones have one view for both routes, so no switch
  const viewSwitch = isPhone ? null : <ViewSwitch mode={mode} onSwitch={switchBudgetMode} />
  const [createBudgetOpen, setCreateBudgetOpen] = useState(false)
  const [updateBudgetOpen, setUpdateBudgetOpen] = useState(false)
  const [createFolderOpen, setCreateFolderOpen] = useState(false)
  const [renameFolder, setRenameFolder] = useState<{ id: Id; name: string } | null>(null)
  const [envelopeDialog, setEnvelopeDialog] = useState<{ open: boolean; envelope: EnvelopeDialogTarget | null; folderId: Id | null; side?: 'expense' | 'income' }>({ open: false, envelope: null, folderId: null })
  const [categoryTarget, setCategoryTarget] = useState<Pick<CategoryDto, 'id' | 'name' | 'type' | 'icon'> | null>(null)
  const [tagTarget, setTagTarget] = useState<TagDialogItem | null>(null)
  const [deleteEnvelopeTarget, setDeleteEnvelopeTarget] = useState<BudgetElementDto | null>(null)
  const [deleteFolderTarget, setDeleteFolderTarget] = useState<{ id: Id; name: string } | null>(null)
  const [currencyTarget, setCurrencyTarget] = useState<BudgetElementDto | null>(null)
  const [moveFolderTarget, setMoveFolderTarget] = useState<BudgetElementDto | null>(null)
  const [limitTarget, setLimitTarget] = useState<(CellTarget & { viaPlan?: boolean; setsPlan?: boolean }) | null>(null)
  const [transactionsTarget, setTransactionsTarget] = useState<BudgetTransactionsTarget | null>(null)
  const [sheetTarget, setSheetTarget] = useState<SheetTarget | null>(null)


  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  // Per budget AND period: the table remounts on route changes and month
  // switches; only the same month of the same budget gets its spot back.
  // Also shields against modal-induced resets (full-screen envelope form /
  // set-limit drawer on phones).
  const tableScrollRef = useScrollMemory(`budget:${budget?.meta.id ?? ''}:${selectedDate}`)

  // Every month switch surfaces the loader for a beat: cache hits (persisted
  // months, local fetches) resolve in a frame or two, which reads as "nothing
  // happened" — a short guaranteed hold makes the reload perceivable, and a
  // slow fetch keeps it up until the period actually lands. Mutations refetch
  // the SAME period, so they never trip this.
  const PERIOD_LOADER_HOLD_MS = 400
  const [periodSwitching, setPeriodSwitching] = useState(false)
  const shownPeriod = useRef(selectedDate)
  useEffect(() => {
    if (shownPeriod.current !== selectedDate) {
      shownPeriod.current = selectedDate
      setPeriodSwitching(true)
    }
  }, [selectedDate])
  useEffect(() => {
    if (periodSwitching && !isFetching && !isPending) {
      const id = setTimeout(() => setPeriodSwitching(false), PERIOD_LOADER_HOLD_MS)
      return () => clearTimeout(id)
    }
  }, [periodSwitching, isFetching, isPending])

  // Live drag preview: while an element drag is in flight (and until the
  // refetched budget lands) the table renders this arrangement, so the row
  // moves across folders during the drag and never snaps back on drop.
  const [dragArrangement, setDragArrangement] = useState<ElementContainer[] | null>(null)
  // true only for the drag gesture itself — children collapse for its duration
  const [dragInProgress, setDragInProgress] = useState(false)
  // folder key ('null' for the default bucket) the drag currently targets across folders
  const [dropFolderKey, setDropFolderKey] = useState<string | null>(null)
  // a FOLDER is being dragged: every section renders header-only
  const [draggingFolderId, setDraggingFolderId] = useState<Id | null>(null)
  useEffect(() => {
    setDragArrangement(null)
  }, [budget])

  const serverBuckets = useMemo(() => {
    if (!budget) {
      return null
    }
    return bucketElements(budget, makeBudgetExchange(budget, currencies), i18n.language)
  }, [budget, currencies, i18n.language])

  const buckets = useMemo(() => {
    if (!budget || !serverBuckets) {
      return serverBuckets
    }
    if (!dragArrangement) {
      return serverBuckets
    }
    return bucketElements(applyArrangement(budget, dragArrangement), makeBudgetExchange(budget, currencies), i18n.language)
  }, [budget, serverBuckets, dragArrangement, currencies, i18n.language])

  // the Total row sums the expenses only, as on the phone: income and savings have
  // their own sections and totals lines
  const totals = useMemo(() => (buckets ? budgetTotals(buckets) : null), [buckets])

  // An archived budget is read-only regardless of role: archived wins over
  // whatever the caller's grant would otherwise allow (the server enforces the
  // same rule with a coded 403).
  const archived = budget?.meta.isArchived === 1
  const configure = budget && !archived ? canConfigureBudget(budget.meta, user?.id) : false
  const editDetails = budget && !archived ? canEditBudget(budget.meta, user?.id) : false
  const limitsEditable = budget && !archived ? canUpdateLimits(budget.meta, user?.id, selectedDate) : false

  const folderNameValidator = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('budgets.form.budget.folder_name.validation.required_field')
    }
    if (!isValidBudgetFolderName(value)) {
      return t('budgets.form.budget.folder_name.validation.invalid_name')
    }
    return null
  }

  // The default budget can 403/404 while still being the stored option: access
  // was revoked or the budget deleted. keepPreviousData would otherwise pin
  // isPlaceholderData forever (the fetch never succeeds), so the error must be
  // handled before the loader branches below.
  const errorStatus = isAxiosError(error) ? error.response?.status : undefined
  const budgetUnavailable = isError && (errorStatus === 403 || errorStatus === 404)

  // Unresolved list counts as "has budgets": the budgets page is the recovery
  // surface either way; only a confirmed-empty list falls to onboarding below.
  if (budgetUnavailable && (budgetList === undefined || budgetList.length > 0)) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center" data-testid="budget-unavailable">
        <h1 className="text-xl font-semibold">{t('budgets.page.budget.unavailable.header')}</h1>
        <p className="max-w-md text-sm text-muted-foreground">{t('budgets.page.budget.unavailable.no_access')}</p>
        <Button type="button" onClick={() => navigate(RouterPage.SETTINGS_BUDGETS)}>
          {t('budgets.page.budget.unavailable.choose_budget')}
        </Button>
      </div>
    )
  }
  if (isError && !budgetUnavailable) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center" data-testid="budget-error">
        <p className="max-w-md text-sm text-muted-foreground">{t('common.app.error')}</p>
        <Button type="button" onClick={() => void refetch()}>
          {t('budgets.page.budget.error.retry')}
        </Button>
      </div>
    )
  }

  if (!isPending && (!budget || budgetUnavailable)) {
    // no default budget — the onboarding empty state (Vue's BudgetOnboarding)
    const hasAccounts = accounts.length > 0
    const hasCategories = categories.length > 0
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center" data-testid="budget-empty">
        <h1 className="text-xl font-semibold">{t('budgets.page.budget.empty.header')}</h1>
        <p className="text-sm text-muted-foreground">{t('budgets.page.budget.empty.no_budget')}</p>
        {hasAccounts && hasCategories ? (
          <>
            <p className="max-w-md text-sm text-muted-foreground">{t('budgets.page.budget.empty.description')}</p>
            <Button type="button" onClick={() => setCreateBudgetOpen(true)}>
              {t('budgets.page.budget.empty.create_budget')}
            </Button>
          </>
        ) : (
          <>
            <p className="max-w-md text-sm text-muted-foreground">{t('budgets.page.budget.empty.initial_setup')}</p>
            <Button type="button" onClick={() => openAccountModal({ folderId: null })}>
              {t('budgets.page.budget.empty.create_account')}
            </Button>
          </>
        )}
        <BudgetDialog
          open={createBudgetOpen}
          onClose={() => setCreateBudgetOpen(false)}
          onSubmit={(form) => {
            createBudget.mutate(
              { id: uuidv7(), name: form.name, startDate: '', currencyId: form.currencyId, accountIds: form.accountIds, savingsAccountIds: form.savingsAccountIds, ownerUserId: user?.id },
              { onSuccess: () => setCreateBudgetOpen(false) },
            )
          }}
        />
      </div>
    )
  }

  if (!budget || !buckets) {
    // cold load only — month switches keep the previous period via keepPreviousData
    return isPending ? (
      <div className="relative flex h-full items-center justify-center" data-testid="budget-loading">
        <CoinLoader label={t('common.app.modal.loading.data_loading')} />
        {showLogoutEscape ? <LogoutEscapeButton placement="container" /> : null}
      </div>
    ) : null
  }

  const handleDragStart = (event: { active: { id: string | number } }) => {
    const activeId = String(event.active.id)
    if (budget.structure.folders.some((f) => f.id === activeId)) {
      setDraggingFolderId(activeId)
      return
    }
    setDragInProgress(true)
  }

  // container key of the folder the pointer is over (cross-folder move pending)
  const folderKeyOf = (arrangement: ElementContainer[], id: string): string | null => {
    if (id.startsWith('bfolder:')) {
      return id.slice('bfolder:'.length)
    }
    const container = arrangement.find((c) => c.ids.includes(id))
    return container ? String(container.folderId) : null
  }

  // No DOM re-ordering happens DURING the drag: within a folder the sortable
  // strategy previews the move with pure transforms, a cross-folder target is
  // only highlighted. Everything applies once, on drop — mutating the row
  // order mid-drag shifts layout under the pointer and feedback-loops the
  // drag-over → re-measure cycle. Folder drags preview the same way (sortable
  // sections) while every section renders collapsed.
  const handleDragOver = (event: DragOverEvent) => {
    const { active, over } = event
    if (draggingFolderId || !over || active.id === over.id) {
      return
    }
    const base = arrangementFromBuckets(buckets)
    const sourceKey = folderKeyOf(base, String(active.id))
    const targetKey = folderKeyOf(base, String(over.id))
    setDropFolderKey(targetKey !== sourceKey ? targetKey : null)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    setDragInProgress(false)
    setDropFolderKey(null)
    const { active, over } = event
    if (draggingFolderId) {
      setDraggingFolderId(null)
      const folders = [...budget.structure.folders].sort((a, b) => a.position - b.position)
      const folderIds = folders.map((f) => f.id)
      const overId = over ? String(over.id).replace(/^bfolder:/, '') : null
      const from = folderIds.indexOf(draggingFolderId)
      const to = overId ? folderIds.indexOf(overId) : -1
      if (from === -1 || to === -1 || from === to) {
        return
      }
      const reordered = arrayMove(folderIds, from, to)
      orderFolders.mutate({
        budgetId: budget.meta.id,
        id: draggingFolderId,
        afterId: afterIdFromDrop(reordered, draggingFolderId),
      })
      return
    }
    const base = arrangementFromBuckets(serverBuckets ?? buckets)
    const final =
      over && active.id !== over.id ? moveElementInArrangement(base, String(active.id), String(over.id)) : base
    const item = arrangementItem(final, String(active.id))
    const before = arrangementItem(base, String(active.id))
    if (!item || (before && before.folderId === item.folderId && before.position === item.position)) {
      setDragArrangement(null)
      return
    }
    // keep the preview until the refetched budget replaces it (or rolls it back)
    setDragArrangement(final)
    moveElement.mutate({ budgetId: budget.meta.id, item })
  }

  // Desktop inline limit editing, shared by the table's budgeted cells and the
  // Savings block's planned cells; compact viewports use SetLimitDialog instead.
  const inlineLimitEditor =
    limitsEditable && !editMode && !isCompact
      ? (cell: Pick<BudgetElementDto, 'id' | 'name' | 'budgeted' | 'currencyId'>) => (
          <LimitEditor
            id={cell.id}
            name={cell.name}
            value={cell.budgeted}
            currency={currencies.find((c) => c.id === (cell.currencyId ?? budget.meta.currencyId))}
            onCommit={(amount) => setLimit.mutate({ budgetId: budget.meta.id, elementId: cell.id, period: selectedDate, amount })}
          />
        )
      : undefined

  const transactionsTargetOf = (element: BudgetElementDto): BudgetTransactionsTarget => ({
    id: element.id,
    type: element.type,
    name: elementDisplayName(element.id, element.name, t),
    icon: element.icon,
    currencyId: element.currencyId,
  })
  // the income Uncategorized row has no list: it gathers income booked in expense
  // categories too, which no selector can name
  const sheetTransactionsTargetOf = (target: SheetTarget): BudgetTransactionsTarget | null => {
    switch (target.kind) {
      case 'expense':
        return transactionsTargetOf(target.element)
      case 'savings':
        return { id: target.row.id, type: BudgetElementType.SAVINGS, name: target.row.name, icon: target.row.icon, currencyId: target.row.currencyId }
      case 'plan': {
        const el = target.cell.element
        return el.id === UNCATEGORIZED_ID ? null : { id: el.id, type: el.type, name: elementDisplayName(el.id, el.name, t), icon: el.icon, currencyId: el.currencyId }
      }
      case 'label':
        return { id: target.label.id, type: 'label', name: target.label.name, icon: target.label.icon, currencyId: null }
    }
  }
  const sheetTransactions = sheetTarget ? sheetTransactionsTargetOf(sheetTarget) : null
  const sheetCanSetAmount = (target: SheetTarget): boolean => {
    if (!limitsEditable) {
      return false
    }
    switch (target.kind) {
      case 'expense':
        return target.element.isArchived === 0 && target.element.id !== UNCATEGORIZED_ID
      case 'savings':
        return target.row.isArchived === 0
      case 'plan':
        // an income row exists only while its plan window is loaded
        return target.cell.element.isArchived === 0 && target.cell.element.id !== UNCATEGORIZED_ID && planMonth !== null
      case 'label':
        return false
    }
  }
  const sheetCellTarget = (target: SheetTarget): CellTarget => {
    const cell = sheetCell(target, budget.meta.currencyId)
    return { id: cell.id, name: cell.name, budgeted: cell.amount }
  }
  const sheetEditAccess = (target: SheetTarget): boolean | null => {
    if (target.kind === 'label') {
      // update-label answers anyone but the tag's owner with NotFound
      return !!user && target.label.ownerUserId === user.id
    }
    return elementEditAccess(sheetElement(target), user?.id, editDetails, accounts)
  }
  const sheetEdit = sheetTarget ? sheetEditAccess(sheetTarget) : null
  // the sheet's pencil: the element's own edit dialog replaces the sheet
  const editFromSheet = (target: SheetTarget) => {
    setSheetTarget(null)
    if (target.kind === 'label') {
      setTagTarget({ id: target.label.id, name: target.label.name, kind: 'label', icon: target.label.icon })
      return
    }
    const el = sheetElement(target)
    if (isEnvelopeType(el.type)) {
      setEnvelopeDialog({ open: true, envelope: el, folderId: null, side: isIncomeType(el.type) ? 'income' : 'expense' })
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
  // phones get no hover corner; edit mode owns the pointer for dragging
  const cellActionsDisabled = isPhone || editMode
  // the hover-only corner that starts a thread on a cell with none yet
  const canAddComment = !cellActionsDisabled && !commentsReadOnly(budget.meta, selectedDate)
  const wrapBudgetCell = (element: BudgetElementDto, cell: ReactElement) => (
    <CellShell
      comments={commentsByCell.get(commentCellKey(element.id, selectedDate)) ?? []}
      previewDisabled={commentsTarget !== null || editMode}
      shortcutDisabled={editMode}
      onOpenComments={(anchor) => openComments(element, anchor)}
    >
      {cell}
    </CellShell>
  )

  const commitPlanned = (target: FlowTarget, amount: string | null) => {
    const elementId = sheetCell(target, budget.meta.currencyId).id
    // an income plan lives only in the plan window, so it patches that cache
    if (target.kind === 'plan' && planMonth) {
      planSetLimit.mutate({ budgetId: budget.meta.id, elementId, period: selectedDate, amount, monthIndex: planMonth.index })
      return
    }
    setLimit.mutate({ budgetId: budget.meta.id, elementId, period: selectedDate, amount })
  }
  const renderFlowPlanned = (target: FlowTarget, text: string) => {
    const cell = sheetCell(target, budget.meta.currencyId)
    const name = elementDisplayName(cell.id, cell.name, t)
    if (editMode) {
      return text
    }
    if (isCompact) {
      return (
        <button type="button" className="w-full text-right underline-offset-2 hover:underline" aria-label={`details ${name}`} onClick={() => setSheetTarget(target)}>
          {text}
        </button>
      )
    }
    const thread = sheetCellTarget(target)
    const cellComments = commentsByCell.get(commentCellKey(cell.id, selectedDate)) ?? []
    return (
      <CellShell comments={cellComments} previewDisabled={commentsTarget !== null} onOpenComments={(anchor) => openComments(thread, anchor)}>
        <span {...{ [COMMENT_ANCHOR_ATTR]: '' }} className="group/cell relative block">
          {sheetCanSetAmount(target) ? (
            <LimitEditor
              id={cell.id}
              name={name}
              value={cell.amount}
              currency={currencies.find((c) => c.id === cell.currencyId)}
              onCommit={(amount) => commitPlanned(target, amount)}
            />
          ) : (
            <button
              type="button"
              className="w-full text-right underline-offset-2 hover:underline"
              aria-label={`comments ${name}`}
              onClick={(e) => openComments(thread, commentAnchorOf(e.currentTarget))}
            >
              {text}
            </button>
          )}
          {cellComments.length > 0 || canAddComment ? (
            <CommentMarker count={cellComments.length} placement="outset" onOpen={(anchor) => openComments(thread, anchor)} />
          ) : null}
        </span>
      </CellShell>
    )
  }

  // Edit mode: the plus and the folder menu follow the folder's name, so the sums
  // keep the row columns; folder ordering moved to dragging.
  const folderActions = (bucket: FolderBucket, _index: number, _total: number) => {
    if (!editMode) {
      return null
    }
    const name = bucket.folder?.name ?? t('budgets.page.budget.structure.no_folder')
    const plus = (
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-6"
        aria-label={`create envelope ${name}`}
        title={t('budgets.modal.create_envelope_form.header')}
        onClick={() => setEnvelopeDialog({ open: true, envelope: null, folderId: bucket.folder?.id ?? null })}
      >
        <Plus className="size-4" />
      </Button>
    )
    if (!bucket.folder) {
      return plus
    }
    return (
      <span className="flex items-center gap-1.5 sm:gap-2">
        {plus}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="icon" aria-label={`budget folder actions ${bucket.folder.name}`}>
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => setRenameFolder({ id: bucket.folder!.id, name: bucket.folder!.name })}>
              {t('common.button.edit.label')}
            </DropdownMenuItem>
            {bucket.elements.length === 0 ? (
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => setDeleteFolderTarget({ id: bucket.folder!.id, name: bucket.folder!.name })}
              >
                {t('budgets.page.budget.structure.action.delete_folder')}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
    )
  }

  const elementActions = (element: BudgetElementDto) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" aria-label={`element actions ${element.name}`}>
          <MoreVertical className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => setCurrencyTarget(element)}>
          {t('budgets.page.budget.structure.element.action.change_currency')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setMoveFolderTarget(element)}>
          {t('budgets.page.plan.menu.move_to_folder')}
        </DropdownMenuItem>
        {element.type === BudgetElementType.ENVELOPE ? (
          <>
            <DropdownMenuItem onSelect={() => setEnvelopeDialog({ open: true, envelope: element, folderId: element.folderId })}>
              {t('common.button.edit.label')}
            </DropdownMenuItem>
            {canDeleteEnvelope(budget.meta, user?.id) ? (
              <DropdownMenuItem variant="destructive" onSelect={() => setDeleteEnvelopeTarget(element)}>
                {t('common.button.delete.label')}
              </DropdownMenuItem>
            ) : null}
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )

  return (
    <div className="flex h-full flex-col gap-3 p-2.5 sm:p-4">
      <header className="flex items-center gap-2">
        {isCompact ? (
          <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(RouterPage.HOME)}>
            <ChevronLeft className="size-5" />
          </Button>
        ) : null}
        <h1
          className={isPhone ? 'min-w-0 shrink truncate text-lg font-medium' : 'min-w-0 shrink truncate text-xl'}
          title={budget.meta.name}
        >
          {budget.meta.name}
        </h1>
        <span className="flex-1" />
        {editMode ? (
          <Button type="button" size="sm" onClick={() => setEditMode(false)}>
            <Check className="size-4" />
            {t('budgets.page.budget.settings.menu.edit_structure_done')}
          </Button>
        ) : (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="uppercase tracking-wide text-muted-foreground"
                aria-label={t('budgets.page.budget.settings.button')}
              >
                <Settings2 className="size-4" />
                <span className="hidden sm:inline">{t('budgets.page.budget.settings.button')}</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem disabled={!editDetails} onSelect={() => setUpdateBudgetOpen(true)}>
                {t('budgets.page.budget.settings.menu.edit')}
              </DropdownMenuItem>
              <DropdownMenuItem disabled={!configure} onSelect={() => setEditMode(true)}>
                {t('budgets.page.budget.settings.menu.edit_structure')}
              </DropdownMenuItem>
              {mode === 'plan' && !phoneView ? (
                <>
                  <DropdownMenuCheckboxItem checked={planHideEmpty} onCheckedChange={() => togglePlanHideEmpty()}>
                    {t('budgets.page.plan.density.hide_empty')}
                  </DropdownMenuCheckboxItem>
                  <DropdownMenuSeparator />
                </>
              ) : null}
              <DropdownMenuItem onSelect={() => navigate(RouterPage.SETTINGS_BUDGETS)}>
                {t('budgets.page.budget.settings.menu.budget_list')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </header>

      {phoneView ? (
        <>
          {archived ? <InfoBox>{t('budgets.page.budget.archived_banner')}</InfoBox> : null}
          <PeriodStrip startedAt={budget.meta.startedAt} endedAt={budget.meta.endedAt} />
          {isPlaceholderData || periodSwitching ? (
            <div className="flex flex-1 items-center justify-center" data-testid="budget-loading">
              <CoinLoader label={t('common.app.modal.loading.data_loading')} />
            </div>
          ) : (
            <div ref={tableScrollRef} className="min-h-0 flex-1 overflow-y-auto">
              <PhoneMonthView
                budget={budget}
                buckets={buckets}
                currencies={currencies}
                selectedDate={selectedDate}
                planMonth={planMonth}
                commentsByCell={commentsByCell}
                onOpenSheet={setSheetTarget}
                onShowTransactions={setTransactionsTarget}
              />
            </div>
          )}
        </>
      ) : mode === 'plan' ? (
        <PlanSheet budget={budget} currencies={currencies} userId={user?.id} editMode={editMode} viewSwitch={viewSwitch} />
      ) : (
        <>
          {archived ? <InfoBox>{t('budgets.page.budget.archived_banner')}</InfoBox> : null}
          <PeriodStrip startedAt={budget.meta.startedAt} endedAt={budget.meta.endedAt} leading={viewSwitch} />

          {editMode ? (
            <div>
              <Button type="button" variant="secondary" size="sm" onClick={() => setCreateFolderOpen(true)}>
                <FolderPlus className="size-4" />
                {t('budgets.page.budget.structure.action.create_folder')}
              </Button>
            </div>
          ) : null}

          {isPlaceholderData || periodSwitching ? (
            // month switch in flight — the strip stays put, the stale table is
            // replaced by the loader until the new period lands
            <div className="flex flex-1 items-center justify-center" data-testid="budget-loading">
              <CoinLoader label={t('common.app.modal.loading.data_loading')} />
            </div>
          ) : (
            <>
              <div ref={tableScrollRef} className="min-h-0 flex-1 overflow-y-auto">
                <div className="flex flex-col">
                  <MonthFlows
                    budget={budget}
                    currencies={currencies}
                    planMonth={planMonth}
                    future={selectedDate > currentMonth()}
                    actionsColumn={editMode}
                    renderPlanned={renderFlowPlanned}
                    onShowTransactions={editMode ? undefined : setTransactionsTarget}
                  />
                </div>
                <DndContext
                  sensors={sensors}
                  collisionDetection={preferRowCollisions}
                  // rows collapse on drag start, so drop-zone rects must re-measure
                  // mid-drag and the grabbed node re-anchors to the pointer
                  measuring={{ droppable: { strategy: MeasuringStrategy.Always } }}
                  modifiers={[snapRowToPointer]}
                  onDragStart={handleDragStart}
                  onDragOver={handleDragOver}
                  onDragEnd={handleDragEnd}
                  onDragCancel={() => {
                    setDragInProgress(false)
                    setDraggingFolderId(null)
                    setDropFolderKey(null)
                    setDragArrangement(null)
                  }}
                >
                  <SortableContext
                    items={buckets.withFolder.map((b) => b.folder!.id)}
                    strategy={verticalListSortingStrategy}
                  >
                  <BudgetTable
                    budget={budget}
                    buckets={buckets}
                    future={selectedDate > currentMonth()}
                    hideTotals
                    hideChildren={dragInProgress}
                    hideContents={draggingFolderId !== null}
                    renderFolderHandle={editMode ? (bucket) => (bucket.folder ? <FolderGrip name={bucket.folder.name} /> : null) : undefined}
                    // only in edit mode
                    renderFolderActions={editMode ? folderActions : undefined}
                    renderActions={editMode ? elementActions : undefined}
                    renderBudgetCell={inlineLimitEditor}
                    // touch viewports reach set budget, comments and transactions through the item sheet
                    onBudgetCellDetails={isCompact && !editMode ? (element) => setSheetTarget({ kind: 'expense', element }) : undefined}
                    onBudgetCellComments={!editMode && !isCompact ? (element, anchor) => openComments(element, anchor) : undefined}
                    wrapBudgetCell={wrapBudgetCell}
                    renderBudgetCellMarker={(element) => {
                      const cellComments = commentsByCell.get(commentCellKey(element.id, selectedDate)) ?? []
                      if (cellComments.length === 0 && (!canAddComment || element.id === UNCATEGORIZED_ID)) {
                        return null
                      }
                      return <CommentMarker count={cellComments.length} placement="outset" onOpen={(anchor) => openComments(element, anchor)} />
                    }}
                    renderRowWrapper={
                      editMode
                        ? (element, _bucket, row) => (
                            <DraggableElement key={element.id} id={element.id}>
                              {row}
                            </DraggableElement>
                          )
                        : undefined
                    }
                    sectionWrapper={
                      editMode
                        ? (bucket, _key, node) => {
                            const folderKey = bucket.folder ? String(bucket.folder.id) : 'null'
                            return (
                              <SortableSection
                                bucket={bucket}
                                id={`bfolder:${folderKey}`}
                                highlighted={dropFolderKey === folderKey}
                                folderDragging={draggingFolderId !== null}
                              >
                                {node}
                              </SortableSection>
                            )
                          }
                        : undefined
                    }
                    onSpentClick={editMode ? undefined : setTransactionsTarget}
                  />
                  </SortableContext>
                </DndContext>
                <div className="mt-1 mb-4 flex flex-col">
                  {totals ? <BudgetTotals budget={budget} totals={totals} actionsColumn={editMode} future={selectedDate > currentMonth()} /> : null}
                  {totals ? (
                    <MonthTotalsLines
                      budget={budget}
                      currencies={currencies}
                      planMonth={planMonth}
                      expensesSpent={totals.spent}
                      future={selectedDate > currentMonth()}
                      actionsColumn={editMode}
                    />
                  ) : null}
                </div>
              </div>
            </>
          )}
        </>
      )}

      <PromptDialog
        open={createFolderOpen}
        onClose={() => setCreateFolderOpen(false)}
        onSubmit={(name) => createFolder.mutate({ budgetId: budget.meta.id, id: uuidv7(), name }, { onSuccess: () => setCreateFolderOpen(false) })}
        title={t('budgets.modal.create_folder_form.header')}
        inputLabel={t('budgets.form.budget.folder_name.label')}
        validate={folderNameValidator}
        submitLabel={t('common.button.create.label')}
        cancelLabel={t('common.button.cancel.label')}
      />

      <PromptDialog
        open={renameFolder !== null}
        onClose={() => setRenameFolder(null)}
        onSubmit={(name) => {
          if (renameFolder) {
            updateFolder.mutate({ budgetId: budget.meta.id, id: renameFolder.id, name }, { onSuccess: () => setRenameFolder(null) })
          }
        }}
        title={t('budgets.modal.update_folder_form.header')}
        inputLabel={t('budgets.form.budget.folder_name.label')}
        initialValue={renameFolder?.name ?? ''}
        validate={folderNameValidator}
        submitLabel={t('common.button.update.label')}
        cancelLabel={t('common.button.cancel.label')}
      />

      <EnvelopeDialog
        open={envelopeDialog.open}
        envelope={envelopeDialog.envelope}
        budgetCurrencyId={budget.meta.currencyId}
        side={envelopeDialog.side ?? 'expense'}
        onClose={() => setEnvelopeDialog({ open: false, envelope: null, folderId: null })}
        onSubmit={(form) => {
          const close = () => setEnvelopeDialog({ open: false, envelope: null, folderId: null })
          if (envelopeDialog.envelope) {
            updateEnvelope.mutate(
              { budgetId: budget.meta.id, id: envelopeDialog.envelope.id, name: form.name, icon: form.icon, currencyId: form.currencyId, isArchived: form.isArchived, categories: form.categories },
              { onSuccess: close },
            )
          } else {
            createEnvelope.mutate(
              { budgetId: budget.meta.id, id: uuidv7(), name: form.name, icon: form.icon, currencyId: form.currencyId, folderId: envelopeDialog.folderId, categories: form.categories },
              { onSuccess: close },
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
            updateCategory.mutate({ id: categoryTarget.id, name: form.name, icon: form.icon }, { onSuccess: () => setCategoryTarget(null) })
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

      {/* the same search-first currency picker the account form uses */}
      {currencyTarget ? (
        <CurrencyPickerDialog
          open
          title={t('budgets.modal.change_element_currency_form.header')}
          value={currencyTarget.currencyId ?? budget.meta.currencyId}
          onClose={() => setCurrencyTarget(null)}
          onPick={(currencyId) => {
            changeCurrency.mutate({ budgetId: budget.meta.id, elementId: currencyTarget.id, currencyId }, { onSuccess: () => setCurrencyTarget(null) })
          }}
        />
      ) : null}

      {moveFolderTarget ? (
        <ResponsiveDialog
          open
          onOpenChange={(o) => !o && setMoveFolderTarget(null)}
          title={t('budgets.page.plan.menu.move_to_folder')}
        >
          <ul className="flex max-h-72 flex-col overflow-y-auto scrollbar-slim">
            {budget.structure.folders.map((f) => (
              <li key={f.id}>
                <button
                  type="button"
                  className="w-full truncate rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover"
                  onClick={() => {
                    moveElement.mutate({
                      budgetId: budget.meta.id,
                      item: { id: moveFolderTarget.id, folderId: f.id, position: 0, afterId: null },
                    })
                    setMoveFolderTarget(null)
                  }}
                >
                  {f.name}
                </button>
              </li>
            ))}
            <li>
              <button
                type="button"
                className="w-full rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover"
                onClick={() => {
                  moveElement.mutate({
                    budgetId: budget.meta.id,
                    item: { id: moveFolderTarget.id, folderId: null, position: 0, afterId: null },
                  })
                  setMoveFolderTarget(null)
                }}
              >
                {t('budgets.page.plan.menu.no_folder')}
              </button>
            </li>
          </ul>
        </ResponsiveDialog>
      ) : null}

      <SetLimitDialog
        target={limitTarget ? { id: limitTarget.id, name: elementDisplayName(limitTarget.id, limitTarget.name, t), value: limitTarget.budgeted } : null}
        plan={limitTarget?.setsPlan}
        onClose={() => setLimitTarget(null)}
        onCommit={(elementId, amount) => {
          // an income plan lives only in the plan window, so it patches that cache;
          // the plain write (which also refreshes the plan) covers a window gone meanwhile
          if (limitTarget?.viaPlan && planMonth) {
            planSetLimit.mutate({ budgetId: budget.meta.id, elementId, period: selectedDate, amount, monthIndex: planMonth.index })
            return
          }
          setLimit.mutate({ budgetId: budget.meta.id, elementId, period: selectedDate, amount })
        }}
      />

      <ElementSheet
        target={sheetTarget}
        month={selectedDate}
        baseCurrencyId={budget.meta.currencyId}
        currencies={currencies}
        exchange={makeBudgetExchange(budget, currencies)}
        comments={sheetTarget ? commentsByCell.get(commentCellKey(sheetCell(sheetTarget, budget.meta.currencyId).id, selectedDate)) ?? [] : []}
        commentsReadOnly={commentsReadOnly(budget.meta, selectedDate)}
        canSetAmount={sheetTarget ? sheetCanSetAmount(sheetTarget) : false}
        onClose={() => setSheetTarget(null)}
        onSetAmount={() => {
          if (sheetTarget) {
            setLimitTarget({ ...sheetCellTarget(sheetTarget), viaPlan: sheetTarget.kind === 'plan', setsPlan: sheetSetsPlan(sheetTarget) })
            setSheetTarget(null)
          }
        }}
        onOpenComments={() => {
          if (sheetTarget) {
            openComments(sheetCellTarget(sheetTarget))
            setSheetTarget(null)
          }
        }}
        onShowTransactions={
          sheetTransactions
            ? () => {
                setTransactionsTarget(sheetTransactions)
                setSheetTarget(null)
              }
            : undefined
        }
        onEdit={sheetTarget && sheetEdit !== null ? () => editFromSheet(sheetTarget) : undefined}
        canEdit={sheetEdit === true}
      />

      <CommentsPanel
        open={commentsTarget !== null}
        onClose={() => setCommentsTarget(null)}
        title={commentsTarget ? elementDisplayName(commentsTarget.el.id, commentsTarget.el.name, t) : ''}
        anchor={commentsTarget?.anchor ?? null}
        budgetId={budget.meta.id}
        elementId={commentsTarget?.el.id ?? ''}
        period={selectedDate}
        comments={commentsTarget ? commentsByCell.get(commentCellKey(commentsTarget.el.id, selectedDate)) ?? [] : []}
        currentUserId={user?.id}
        canModerate={canConfigureBudget(budget.meta, user?.id)}
        readOnly={commentsTarget ? commentsReadOnly(budget.meta, selectedDate) : true}
        truncated={commentsTruncated}
      />

      <BudgetUpdateDialog open={updateBudgetOpen} budget={budget} onClose={() => setUpdateBudgetOpen(false)} />

      <BudgetTransactionsDialog budget={budget} element={transactionsTarget} onClose={() => setTransactionsTarget(null)} />
    </div>
  )
}
