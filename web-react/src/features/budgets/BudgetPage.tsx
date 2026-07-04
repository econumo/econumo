import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { DndContext, PointerSensor, closestCenter, useDraggable, useDroppable, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent, DragOverEvent } from '@dnd-kit/core'
import { Check, ChevronLeft, FolderPlus, GripVertical, MoreVertical, Plus, Settings2 } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PromptDialog } from '@/components/PromptDialog'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useLongPress } from '@/hooks/useLongPress'
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
import { CurrencySelect } from '@/components/CurrencySelect'
import {
  useBudget,
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
import { BudgetDialog } from './BudgetDialog'
import { useCreateBudget } from './queries'
import type { ElementContainer } from './elementMove'
import { applyArrangement, arrangementFromBuckets, arrangementItem, moveElementInArrangement } from './elementMove'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

function DraggableElement({ id, children }: { id: string; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id })
  const { setNodeRef: setDropRef } = useDroppable({ id })
  return (
    <div ref={setDropRef} className={isDragging ? 'opacity-50' : undefined}>
      <div className="flex items-center gap-1">
        <button
          type="button"
          ref={setNodeRef}
          aria-label={`move ${id}`}
          className="cursor-grab text-muted-foreground"
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

function DroppableSection({ id, children }: { id: string; children: ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id })
  return (
    <div ref={setNodeRef} className={isOver ? 'rounded-md ring-2 ring-ring' : undefined}>
      {children}
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
  const { data: budget, isLoading } = useBudget()
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
  const [currencyTarget, setCurrencyTarget] = useState<BudgetElementDto | null>(null)
  const [limitTarget, setLimitTarget] = useState<BudgetElementDto | null>(null)
  const [transactionsTarget, setTransactionsTarget] = useState<BudgetElementDto | null>(null)

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  // Live drag preview: while an element drag is in flight (and until the
  // refetched budget lands) the table renders this arrangement, so the row
  // moves across folders during the drag and never snaps back on drop.
  const [dragArrangement, setDragArrangement] = useState<ElementContainer[] | null>(null)
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
  const limitsEditable = budget ? canUpdateLimits(budget.meta, user?.id, selectedDate) : false

  const folderNameValidator = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('modules.budget.form.budget.folder_name.validation.required_field')
    }
    if (!isValidBudgetFolderName(value)) {
      return t('modules.budget.form.budget.folder_name.validation.invalid_name')
    }
    return null
  }

  if (!isLoading && !budget) {
    // no default budget — the onboarding empty state (Vue's BudgetOnboarding)
    const hasAccounts = accounts.length > 0
    const hasCategories = categories.length > 0
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center" data-testid="budget-empty">
        <h1 className="text-xl font-semibold">{t('modules.budget.page.budget.empty.header')}</h1>
        <p className="text-sm text-muted-foreground">{t('modules.budget.page.budget.empty.no_budget')}</p>
        {hasAccounts && hasCategories ? (
          <>
            <p className="max-w-md text-sm text-muted-foreground">{t('modules.budget.page.budget.empty.description')}</p>
            <Button type="button" onClick={() => setCreateBudgetOpen(true)}>
              {t('modules.budget.page.budget.empty.create_budget')}
            </Button>
          </>
        ) : (
          <>
            <p className="max-w-md text-sm text-muted-foreground">{t('modules.budget.page.budget.empty.initial_setup')}</p>
            <Button type="button" onClick={() => openAccountModal({ folderId: null })}>
              {t('modules.budget.page.budget.empty.create_account')}
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
    return null
  }

  const budgetCurrencyIds = budget.balances.map((b) => b.currencyId)

  const handleDragStart = () => {
    setDragArrangement(arrangementFromBuckets(buckets))
  }

  const handleDragOver = (event: DragOverEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    setDragArrangement((prev) => moveElementInArrangement(prev ?? arrangementFromBuckets(buckets), String(active.id), String(over.id)))
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    const base = arrangementFromBuckets(serverBuckets ?? buckets)
    const final =
      over && active.id !== over.id
        ? moveElementInArrangement(dragArrangement ?? base, String(active.id), String(over.id))
        : dragArrangement ?? base
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

  const folderActions = (bucket: FolderBucket, index: number, total: number) =>
    editMode && bucket.folder ? (
      <span className="flex items-center gap-1">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`create envelope ${bucket.folder.name}`}
          onClick={() => setEnvelopeDialog({ open: true, envelope: null, folderId: bucket.folder!.id })}
        >
          <Plus className="size-4" />
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="icon" aria-label={`budget folder actions ${bucket.folder.name}`}>
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => setRenameFolder({ id: bucket.folder!.id, name: bucket.folder!.name })}>
              {t('elements.button.edit.label')}
            </DropdownMenuItem>
            {index > 0 ? (
              <DropdownMenuItem
                onSelect={() => {
                  const items = [
                    { id: bucket.folder!.id, position: index - 1 },
                    { id: buckets.withFolder[index - 1].folder!.id, position: index },
                  ]
                  orderFolders.mutate({ budgetId: budget.meta.id, items })
                }}
              >
                {t('elements.button.up.label')}
              </DropdownMenuItem>
            ) : null}
            {index < total - 1 ? (
              <DropdownMenuItem
                onSelect={() => {
                  const items = [
                    { id: bucket.folder!.id, position: index + 1 },
                    { id: buckets.withFolder[index + 1].folder!.id, position: index },
                  ]
                  orderFolders.mutate({ budgetId: budget.meta.id, items })
                }}
              >
                {t('elements.button.down.label')}
              </DropdownMenuItem>
            ) : null}
            {bucket.elements.length === 0 ? (
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => deleteFolder.mutate({ budgetId: budget.meta.id, id: bucket.folder!.id })}
              >
                {t('modules.budget.page.budget.structure.action.delete_folder')}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
    ) : null

  const elementActions = (element: BudgetElementDto) =>
    editMode ? (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="ghost" size="icon" aria-label={`element actions ${element.name}`}>
            <MoreVertical className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {element.type !== BudgetElementType.ENVELOPE ? (
            <DropdownMenuItem onSelect={() => setCurrencyTarget(element)}>
              {t('modules.budget.page.budget.structure.element.action.change_currency')}
            </DropdownMenuItem>
          ) : (
            <>
              <DropdownMenuItem onSelect={() => setEnvelopeDialog({ open: true, envelope: element, folderId: element.folderId })}>
                {t('elements.button.edit.label')}
              </DropdownMenuItem>
              {canDeleteEnvelope(budget.meta, user?.id) ? (
                <DropdownMenuItem variant="destructive" onSelect={() => setDeleteEnvelopeTarget(element)}>
                  {t('elements.button.delete.label')}
                </DropdownMenuItem>
              ) : null}
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    ) : null

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      <header className="flex items-center gap-2">
        {isCompact ? (
          <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(RouterPage.HOME)}>
            <ChevronLeft className="size-5" />
          </Button>
        ) : null}
        <h1 className="min-w-0 flex-1 truncate text-[22px] uppercase tracking-wide" title={budget.meta.name}>
          {budget.meta.name}
        </h1>
        <span className="flex items-center gap-1">
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
        {editMode ? (
          <Button type="button" size="sm" onClick={() => setEditMode(false)}>
            <Check className="size-4" />
            {t('modules.budget.page.budget.settings.menu.edit_structure_done')}
          </Button>
        ) : (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="uppercase tracking-wide text-muted-foreground"
                aria-label={t('modules.budget.page.budget.settings.button')}
              >
                <Settings2 className="size-4" />
                <span className="hidden sm:inline">{t('modules.budget.page.budget.settings.button')}</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => setUpdateBudgetOpen(true)}>
                {t('modules.budget.page.budget.settings.menu.edit')}
              </DropdownMenuItem>
              <DropdownMenuItem disabled={!configure} onSelect={() => setEditMode(true)}>
                {t('modules.budget.page.budget.settings.menu.edit_structure')}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => navigate(RouterPage.SETTINGS_BUDGETS)}>
                {t('modules.budget.page.budget.settings.menu.budget_list')}
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
            {t('modules.budget.page.budget.structure.action.create_folder')}
          </Button>
        </div>
      ) : null}

      {selectedCurrencyId ? <ExpenseWidget budget={budget} currencyId={selectedCurrencyId} /> : null}

      <div className="min-h-0 flex-1 overflow-y-auto">
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={handleDragStart}
          onDragOver={handleDragOver}
          onDragEnd={handleDragEnd}
          onDragCancel={() => setDragArrangement(null)}
        >
          <BudgetTable
            budget={budget}
            buckets={buckets}
            renderFolderActions={folderActions}
            renderActions={elementActions}
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
                ? (bucket, _key, node) => <DroppableSection id={`bfolder:${bucket.folder ? bucket.folder.id : 'null'}`}>{node}</DroppableSection>
                : undefined
            }
            onElementClick={editMode ? undefined : (element) => setTransactionsTarget(element)}
          />
        </DndContext>
      </div>

      <PromptDialog
        open={createFolderOpen}
        onClose={() => setCreateFolderOpen(false)}
        onSubmit={(name) => createFolder.mutate({ budgetId: budget.meta.id, id: uuidv7(), name }, { onSuccess: () => setCreateFolderOpen(false) })}
        title={t('modules.budget.modal.create_folder_form.header')}
        inputLabel={t('modules.budget.form.budget.folder_name.label')}
        validate={folderNameValidator}
        submitLabel={t('elements.button.create.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />

      <PromptDialog
        open={renameFolder !== null}
        onClose={() => setRenameFolder(null)}
        onSubmit={(name) => {
          if (renameFolder) {
            updateFolder.mutate({ budgetId: budget.meta.id, id: renameFolder.id, name }, { onSuccess: () => setRenameFolder(null) })
          }
        }}
        title={t('modules.budget.modal.update_folder_form.header')}
        inputLabel={t('modules.budget.form.budget.folder_name.label')}
        initialValue={renameFolder?.name ?? ''}
        validate={folderNameValidator}
        submitLabel={t('elements.button.update.label')}
        cancelLabel={t('elements.button.cancel.label')}
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
        title={t('modules.budget.modal.delete_envelope.header')}
        question={t('modules.budget.modal.delete_envelope.question')}
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />

      {currencyTarget ? (
        <ResponsiveDialog open onOpenChange={(o) => !o && setCurrencyTarget(null)} title={t('modules.budget.modal.change_element_currency_form.header')} description={currencyTarget.name}>
          <div className="flex flex-col gap-4">
            <CurrencySelect
              aria-label={t('modules.budget.form.budget_envelope.currency.label')}
              value={currencyTarget.currencyId ?? budget.meta.currencyId}
              onChange={(currencyId) => {
                changeCurrency.mutate({ budgetId: budget.meta.id, elementId: currencyTarget.id, currencyId }, { onSuccess: () => setCurrencyTarget(null) })
              }}
            />
          </div>
        </ResponsiveDialog>
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
