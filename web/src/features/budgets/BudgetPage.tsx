import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { DndContext, MeasuringStrategy, PointerSensor, pointerWithin, rectIntersection, useDroppable, useSensor, useSensors } from '@dnd-kit/core'
import type { CollisionDetection, DragEndEvent, DragOverEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { snapRowToPointer } from '@/lib/dnd'
import { getChangedPositions } from '@/lib/ordering'
import type { SortableHandleProps } from '@/components/SortableList'
import { Check, ChevronLeft, FolderPlus, GripVertical, MoreVertical, Plus, Settings2 } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LogoutEscapeButton } from '@/features/auth/LogoutEscapeButton'
import { PromptDialog } from '@/components/PromptDialog'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useLogoutEscape } from '@/hooks/useLogoutEscape'
import { useLongPress } from '@/hooks/useLongPress'
import { useScrollMemory } from '@/hooks/useScrollMemory'
import { isNotEmpty, isValidBudgetFolderName } from '@/lib/validation'
import type { BudgetElementDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData } from '@/features/user/queries'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories } from '@/features/classifications/queries'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import {
  useBudget,
  useBudgets,
  useSetLimit,
  useCreateEnvelope,
  useUpdateEnvelope,
  useDeleteEnvelope,
  useCreateBudgetFolder,
  useUpdateBudgetFolder,
  useDeleteBudgetFolder,
  useOrderBudgetFolders,
  useMoveElements,
  useChangeElementCurrency,
  canConfigureBudget,
  canEditBudget,
  canUpdateLimits,
  canDeleteEnvelope,
} from './queries'
import { useBudgetPeriodStore } from './budgetStore'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import type { FolderBucket } from './budgetMath'
import { BudgetTable } from './BudgetTable'
import { PeriodStrip } from './PeriodStrip'
import { ExpenseWidget } from './ExpenseWidget'
import { LimitEditor } from './LimitEditor'
import { SetLimitDialog } from './SetLimitDialog'
import { EnvelopeDialog } from './EnvelopeDialog'
import { BudgetUpdateDialog } from './BudgetUpdateDialog'
import { BudgetTransactionsDialog } from './BudgetTransactionsDialog'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { BudgetDialog } from './BudgetDialog'
import { useCreateBudget } from './queries'
import type { ElementContainer } from './elementMove'
import { applyArrangement, arrangementFromBuckets, arrangementItem, moveElementInArrangement } from './elementMove'
import { CoinLoader } from '@/components/CoinLoader'

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
          className="mt-[18px] cursor-grab touch-none text-muted-foreground"
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


function ElementLongPress({ element, onLongPress, children }: { element: BudgetElementDto; onLongPress: (el: BudgetElementDto) => void; children: ReactNode }) {
  const handlers = useLongPress(() => onLongPress(element))
  return <div {...handlers}>{children}</div>
}

export function BudgetPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
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
  const openAccountModal = useUiStore((s) => s.openAccountModal)

  const setLimit = useSetLimit()
  const createEnvelope = useCreateEnvelope()
  const updateEnvelope = useUpdateEnvelope()
  const deleteEnvelope = useDeleteEnvelope()
  const createFolder = useCreateBudgetFolder()
  const updateFolder = useUpdateBudgetFolder()
  const deleteFolder = useDeleteBudgetFolder()
  const orderFolders = useOrderBudgetFolders()
  const moveElements = useMoveElements()
  const changeCurrency = useChangeElementCurrency()
  const createBudget = useCreateBudget()

  const [editMode, setEditMode] = useState(false)
  const [selectedCurrencyId, setSelectedCurrencyId] = useState<Id | null>(null)
  const [createBudgetOpen, setCreateBudgetOpen] = useState(false)
  const [updateBudgetOpen, setUpdateBudgetOpen] = useState(false)
  const [createFolderOpen, setCreateFolderOpen] = useState(false)
  const [renameFolder, setRenameFolder] = useState<{ id: Id; name: string } | null>(null)
  const [envelopeDialog, setEnvelopeDialog] = useState<{ open: boolean; envelope: BudgetElementDto | null; folderId: Id | null }>({ open: false, envelope: null, folderId: null })
  const [deleteEnvelopeTarget, setDeleteEnvelopeTarget] = useState<BudgetElementDto | null>(null)
  const [deleteFolderTarget, setDeleteFolderTarget] = useState<{ id: Id; name: string } | null>(null)
  const [currencyTarget, setCurrencyTarget] = useState<BudgetElementDto | null>(null)
  const [limitTarget, setLimitTarget] = useState<BudgetElementDto | null>(null)
  const [transactionsTarget, setTransactionsTarget] = useState<BudgetTransactionsTarget | null>(null)

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
    return bucketElements(budget, makeBudgetExchange(budget, currencies))
  }, [budget, currencies])

  const buckets = useMemo(() => {
    if (!budget || !serverBuckets) {
      return serverBuckets
    }
    if (!dragArrangement) {
      return serverBuckets
    }
    return bucketElements(applyArrangement(budget, dragArrangement), makeBudgetExchange(budget, currencies))
  }, [budget, serverBuckets, dragArrangement, currencies])

  const configure = budget ? canConfigureBudget(budget.meta, user?.id) : false
  const editDetails = budget ? canEditBudget(budget.meta, user?.id) : false
  const limitsEditable = budget ? canUpdateLimits(budget.meta, user?.id, selectedDate) : false

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
              { id: uuidv7(), name: form.name, startDate: '', currencyId: form.currencyId, excludedAccounts: form.excludedAccounts, ownerUserId: user?.id },
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

  const budgetCurrencyIds = budget.balances.map((b) => b.currencyId)

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
      const changes = getChangedPositions(folders, arrayMove(folderIds, from, to))
      if (changes.length > 0) {
        orderFolders.mutate({ budgetId: budget.meta.id, items: changes })
      }
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
    moveElements.mutate({ budgetId: budget.meta.id, items: [item] })
  }

  // In edit mode the plus sits in the currency-symbol slot (w-6) so the stat
  // columns line up with the element rows; folder ordering moved to dragging.
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
      // the default bucket has no menu — pad to keep its numbers aligned
      return (
        <span className="flex items-center gap-1.5 sm:gap-2">
          {plus}
          <span className="size-8" />
        </span>
      )
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
        {element.type !== BudgetElementType.ENVELOPE ? (
          <DropdownMenuItem onSelect={() => setCurrencyTarget(element)}>
            {t('budgets.page.budget.structure.element.action.change_currency')}
          </DropdownMenuItem>
        ) : (
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
        )}
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
        <h1 className="min-w-0 shrink truncate text-[22px] uppercase tracking-wide" title={budget.meta.name}>
          {budget.meta.name}
        </h1>
        <span className="flex shrink-0 items-center gap-1">
          {budgetCurrencyIds.map((currencyId) => {
            const currency = currencies.find((c) => c.id === currencyId)
            const active = selectedCurrencyId === currencyId
            return (
              <button
                key={currencyId}
                type="button"
                aria-label={`currency ${currency?.code ?? currencyId}`}
                aria-pressed={active}
                title={currency?.name}
                className={`flex size-7 items-center justify-center rounded-full border text-xs ${active ? 'border-econumo-magenta bg-econumo-magenta text-white' : 'text-muted-foreground hover:bg-accent'}`}
                onClick={() => setSelectedCurrencyId(active ? null : currencyId)}
              >
                {currency?.symbol ?? '?'}
              </button>
            )
          })}
        </span>
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
              <DropdownMenuItem onSelect={() => navigate(RouterPage.SETTINGS_BUDGETS)}>
                {t('budgets.page.budget.settings.menu.budget_list')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </header>

      <PeriodStrip startedAt={budget.meta.startedAt} />

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
          {selectedCurrencyId ? <ExpenseWidget budget={budget} currencyId={selectedCurrencyId} /> : null}

          <div ref={tableScrollRef} className="min-h-0 flex-1 overflow-y-auto">
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
                hideChildren={dragInProgress}
                hideContents={draggingFolderId !== null}
                renderFolderHandle={editMode ? (bucket) => (bucket.folder ? <FolderGrip name={bucket.folder.name} /> : null) : undefined}
                // only in edit mode — its presence also swaps the folder currency symbol for the plus slot
                renderFolderActions={editMode ? folderActions : undefined}
                renderActions={editMode ? elementActions : undefined}
                renderBudgetCell={
                  limitsEditable && !editMode && !isCompact
                    ? (element) => (
                        <LimitEditor
                          element={element}
                          currency={currencies.find((c) => c.id === (element.currencyId ?? budget.meta.currencyId))}
                          onCommit={(amount) => setLimit.mutate({ budgetId: budget.meta.id, elementId: element.id, amount })}
                        />
                      )
                    : undefined
                }
                renderRowWrapper={
                  editMode
                    ? (element, _bucket, row) => (
                        <DraggableElement key={element.id} id={element.id}>
                          {row}
                        </DraggableElement>
                      )
                    : isCompact && limitsEditable
                      ? (element, _bucket, row) => (
                          <ElementLongPress key={element.id} element={element} onLongPress={setLimitTarget}>
                            {row}
                          </ElementLongPress>
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
                onAvailableClick={isCompact && limitsEditable && !editMode ? setLimitTarget : undefined}
              />
              </SortableContext>
            </DndContext>
          </div>
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

      <SetLimitDialog
        element={limitTarget}
        onClose={() => setLimitTarget(null)}
        onCommit={(elementId, amount) => setLimit.mutate({ budgetId: budget.meta.id, elementId, amount })}
      />

      <BudgetUpdateDialog open={updateBudgetOpen} budget={budget} onClose={() => setUpdateBudgetOpen(false)} />

      <BudgetTransactionsDialog budget={budget} element={transactionsTarget} onClose={() => setTransactionsTarget(null)} />
    </div>
  )
}
