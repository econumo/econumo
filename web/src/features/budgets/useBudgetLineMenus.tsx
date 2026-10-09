import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { PromptDialog } from '@/components/PromptDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidBudgetFolderName } from '@/lib/validation'
import type {
  BudgetDto,
  BudgetElementDto,
  BudgetFolderSide,
  BudgetPlanDto,
  BudgetSavingsElementDto,
  LabelSpendDto,
  PlanElementDto,
} from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { Id } from '@/api/types'
import { useUiStore } from '@/app/uiStore'
import { useAccounts } from '@/features/accounts/queries'
import { CategoryDialog } from '@/features/classifications/CategoryDialog'
import { TagDialog } from '@/features/classifications/TagDialog'
import type { TagDialogItem } from '@/features/classifications/TagDialog'
import { useUpdateCategory } from '@/features/classifications/queries'
import {
  canConfigureBudget,
  canDeleteEnvelope,
  canEditBudget,
  useAddSavingsAccount,
  useChangeElementCurrency,
  useCreateBudgetFolder,
  useCreateEnvelope,
  useDeleteBudgetFolder,
  useDeleteEnvelope,
  useMoveElement,
  useUpdateBudgetFolder,
  useUpdateEnvelope,
} from './queries'
import { useClassificationMenu } from './classificationMenu'
import type { ClassificationRef } from './classificationMenu'
import { EnvelopeDialog } from './EnvelopeDialog'
import type { EnvelopeDialogTarget } from './EnvelopeDialog'
import { elementEditAccess, isEnvelopeType } from './elementEdit'
import type { MenuAction } from './monthLayout'
import { planCellFigures, sheetElement } from './phoneMonth'
import type { PlanCellFigures, SheetTarget } from './phoneMonth'
import { folderSides } from './planMath'

export interface BudgetLineMenus {
  /** a row of the Budget view (get-budget) */
  expenseRowMenu: (element: BudgetElementDto) => MenuAction[]
  incomeRowMenu: (cell: PlanCellFigures) => MenuAction[]
  /** a row of the Plan grid: any side, envelopes and savings included */
  planRowMenu: (el: PlanElementDto, monthIndex: number) => MenuAction[]
  envelopeChildMenu: (child: { id: Id; type: BudgetElementType; name: string; icon: string; ownerUserId: Id | null }) => MenuAction[]
  savingsRowMenu: (row: BudgetSavingsElementDto) => MenuAction[]
  labelMenu: (label: LabelSpendDto) => MenuAction[]
  folderActionsFor: (folder: { id: Id; name: string } | null, empty: boolean, side: BudgetFolderSide) => MenuAction[] | undefined
  sectionMenu: (side: BudgetFolderSide) => MenuAction[] | undefined
  savingsSectionMenu: MenuAction[] | undefined
  /** whether the caller may open a line's own edit dialog; null: it has none */
  editAccess: (target: SheetTarget) => boolean | null
  /** the element's own edit dialog (sheet pencil, Enter on a name cell) */
  editFromSheet: (target: SheetTarget) => void
  /** an element's own edit dialog is open */
  editorOpen: boolean
  /** every dialog the menus open; render once */
  dialogs: ReactNode
}

export function useBudgetLineMenus({
  budget,
  plan,
  userId,
  onOpenSettings,
}: {
  budget: BudgetDto | null | undefined
  /** the plan window the caller has loaded: income folders for Move to folder */
  plan: BudgetPlanDto | null | undefined
  userId: Id | undefined
  /** opens Budget settings (savings accounts) */
  onOpenSettings: () => void
}): BudgetLineMenus {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const openAccountModal = useUiStore((s) => s.openAccountModal)
  const classificationMenu = useClassificationMenu(budget?.meta.id ?? '')
  const createEnvelope = useCreateEnvelope()
  const updateEnvelope = useUpdateEnvelope()
  const updateCategory = useUpdateCategory()
  const deleteEnvelope = useDeleteEnvelope()
  const createFolder = useCreateBudgetFolder()
  const updateFolder = useUpdateBudgetFolder()
  const deleteFolder = useDeleteBudgetFolder()
  const moveElement = useMoveElement()
  const addSavingsAccount = useAddSavingsAccount()
  const changeCurrency = useChangeElementCurrency()

  // the section a folder is being created in; null while the prompt is closed
  const [createFolderSide, setCreateFolderSide] = useState<BudgetFolderSide | null>(null)
  const [renameFolder, setRenameFolder] = useState<{ id: Id; name: string } | null>(null)
  const [envelopeDialog, setEnvelopeDialog] = useState<{ open: boolean; envelope: EnvelopeDialogTarget | null; folderId: Id | null; side?: 'expense' | 'income' }>({ open: false, envelope: null, folderId: null })
  const [categoryTarget, setCategoryTarget] = useState<Pick<CategoryDto, 'id' | 'name' | 'type' | 'icon'> | null>(null)
  const [tagTarget, setTagTarget] = useState<TagDialogItem | null>(null)
  const [deleteEnvelopeTarget, setDeleteEnvelopeTarget] = useState<{ id: Id } | null>(null)
  const [deleteFolderTarget, setDeleteFolderTarget] = useState<{ id: Id; name: string } | null>(null)
  const [currencyTarget, setCurrencyTarget] = useState<{ id: Id; currencyId: Id | null } | null>(null)
  const [moveFolderTarget, setMoveFolderTarget] = useState<{ id: Id; side: BudgetFolderSide } | null>(null)

  // An archived budget is read-only regardless of role: archived wins over
  // whatever the caller's grant would otherwise allow (the server enforces the
  // same rule with a coded 403).
  const archived = budget?.meta.isArchived === 1
  const configure = budget && !archived ? canConfigureBudget(budget.meta, userId) : false
  const editDetails = budget && !archived ? canEditBudget(budget.meta, userId) : false

  const folderNameValidator = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('budgets.form.budget.folder_name.validation.required_field')
    }
    if (!isValidBudgetFolderName(value)) {
      return t('budgets.form.budget.folder_name.validation.invalid_name')
    }
    return null
  }

  const editAccess = (target: SheetTarget): boolean | null => {
    if (target.kind === 'label') {
      // update-label answers anyone but the tag's owner with NotFound
      return !!userId && target.label.ownerUserId === userId
    }
    return elementEditAccess(sheetElement(target), userId, editDetails, accounts)
  }
  const editFromSheet = (target: SheetTarget) => {
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
  const editAction = (target: SheetTarget): MenuAction[] => {
    const access = editAccess(target)
    if (access === null) {
      return []
    }
    return [
      {
        label: t('common.button.edit.label'),
        disabled: !access,
        // a category, tag or reporting tag another member owns: only its owner edits it
        reason: access ? undefined : t('budgets.page.plan.menu.no_access'),
        onSelect: () => editFromSheet(target),
      },
    ]
  }
  const structureActions = (el: { id: Id; type: BudgetElementType; currencyId: Id | null; isArchived: 0 | 1 }, side: BudgetFolderSide): MenuAction[] => {
    if (!configure || !budget) {
      return []
    }
    const remove: MenuAction[] =
      isEnvelopeType(el.type) && canDeleteEnvelope(budget.meta, userId)
        ? [{ label: t('common.button.delete.label'), destructive: true, onSelect: () => setDeleteEnvelopeTarget({ id: el.id }) }]
        : []
    // an archived row is history: it can still be opened (an envelope unarchives there) or removed
    if (el.isArchived === 1) {
      return remove
    }
    return [
      { label: t('budgets.page.budget.structure.element.action.change_currency'), onSelect: () => setCurrencyTarget({ id: el.id, currencyId: el.currencyId }) },
      { label: t('budgets.page.plan.menu.move_to_folder'), onSelect: () => setMoveFolderTarget({ id: el.id, side }) },
      ...remove,
    ]
  }
  // the category or tag a line stands for (an envelope stands for neither)
  const classificationOf = (el: { id: Id; type: BudgetElementType }): ClassificationRef | null =>
    el.type === BudgetElementType.CATEGORY || el.type === BudgetElementType.INCOME_CATEGORY
      ? { kind: 'category', id: el.id }
      : el.type === BudgetElementType.TAG
        ? { kind: 'tag', id: el.id }
        : null
  const classificationActions = (el: { id: Id; type: BudgetElementType }): MenuAction[] => {
    const ref = el.id === UNCATEGORIZED_ID ? null : classificationOf(el)
    return ref ? classificationMenu.actionsFor(ref) : []
  }
  // the transaction list opens from a row's figures (Spent, Received, Saved), not from the menu
  const expenseRowMenu = (element: BudgetElementDto): MenuAction[] =>
    element.id === UNCATEGORIZED_ID
      ? []
      : [...editAction({ kind: 'expense', element }), ...structureActions(element, 'expense'), ...classificationActions(element)]
  const incomeRowMenu = (cell: PlanCellFigures): MenuAction[] => {
    const target: SheetTarget = { kind: 'plan', cell }
    // income Uncategorized has nothing to edit and no list of its own
    if (cell.element.id === UNCATEGORIZED_ID) {
      return []
    }
    return [...editAction(target), ...structureActions(cell.element, 'income'), ...classificationActions(cell.element)]
  }
  const planRowMenu = (el: PlanElementDto, monthIndex: number): MenuAction[] => {
    if (el.id === UNCATEGORIZED_ID) {
      return []
    }
    // a savings row is an account: it lives in its own section, never in a folder
    if (el.type === BudgetElementType.SAVINGS) {
      return editAction({ kind: 'plan', cell: planCellFigures(el, monthIndex) })
    }
    const side: BudgetFolderSide = isIncomeType(el.type) ? 'income' : 'expense'
    return [...editAction({ kind: 'plan', cell: planCellFigures(el, monthIndex) }), ...structureActions(el, side), ...classificationActions(el)]
  }
  // a category inside an envelope: edited, archived, merged or deleted in place
  const envelopeChildMenu = (child: { id: Id; type: BudgetElementType; name: string; icon: string; ownerUserId: Id | null }): MenuAction[] => {
    const own = !!userId && child.ownerUserId === userId
    return [
      {
        label: t('common.button.edit.label'),
        disabled: !own,
        reason: own ? undefined : t('budgets.page.plan.menu.no_access'),
        onSelect: () => setCategoryTarget({ id: child.id, name: child.name, icon: child.icon, type: isIncomeType(child.type) ? 'income' : 'expense' }),
      },
      ...classificationActions(child),
    ]
  }
  const savingsRowMenu = (row: BudgetSavingsElementDto): MenuAction[] => editAction({ kind: 'savings', row })
  const labelMenu = (label: LabelSpendDto): MenuAction[] => editAction({ kind: 'label', label })
  const newEnvelopeAction = (folderId: Id | null, side: BudgetFolderSide): MenuAction => ({
    label: t('budgets.modal.create_envelope_form.header'),
    onSelect: () => setEnvelopeDialog({ open: true, envelope: null, folderId, side }),
  })
  const folderActionsFor = (folder: { id: Id; name: string } | null, empty: boolean, side: BudgetFolderSide): MenuAction[] | undefined => {
    if (!configure) {
      return undefined
    }
    if (!folder) {
      return [newEnvelopeAction(null, side), classificationMenu.newCategoryAction(side, null)]
    }
    return [
      newEnvelopeAction(folder.id, side),
      classificationMenu.newCategoryAction(side, folder.id),
      { label: t('common.button.edit.label'), onSelect: () => setRenameFolder({ id: folder.id, name: folder.name }) },
      {
        label: t('budgets.page.budget.structure.action.delete_folder'),
        destructive: true,
        disabled: !empty,
        reason: empty ? undefined : t('budgets.page.plan.menu.not_empty'),
        onSelect: () => setDeleteFolderTarget({ id: folder.id, name: folder.name }),
      },
    ]
  }
  const sectionMenu = (side: BudgetFolderSide): MenuAction[] | undefined =>
    configure
      ? [
          { label: t('budgets.page.budget.structure.action.create_folder'), onSelect: () => setCreateFolderSide(side) },
          newEnvelopeAction(null, side),
          classificationMenu.newCategoryAction(side, null),
        ]
      : undefined
  const savingsSectionMenu: MenuAction[] | undefined =
    editDetails && budget
      ? [
          {
            label: t('accounts.modal.create_form.header'),
            // a new account made here is a savings account of this budget
            onSelect: () => openAccountModal({ onCreated: (account) => addSavingsAccount.mutate({ budgetId: budget.meta.id, accountId: account.id }) }),
          },
          { label: t('budgets.modal.budget_form.savings.label'), onSelect: onOpenSettings },
        ]
      : undefined
  // Move to folder offers the folders of the row's own side, a side-less (empty) one
  // to both: the server refuses a mixed folder. The plan knows every folder's side;
  // get-budget lists only the expense ones.
  const moveTargetFolders = (side: BudgetFolderSide): { id: Id; name: string }[] => {
    if (plan) {
      const sides = folderSides(plan)
      const other = side === 'income' ? 'expense' : 'income'
      return [...plan.structure.folders].sort((a, b) => a.position - b.position).filter((f) => sides.get(f.id) !== other)
    }
    return side === 'expense' && budget ? budget.structure.folders : []
  }

  const dialogs = budget ? (
    <>
      <PromptDialog
        open={createFolderSide !== null}
        onClose={() => setCreateFolderSide(null)}
        onSubmit={(name) =>
          createFolder.mutate(
            { budgetId: budget.meta.id, id: uuidv7(), name, side: createFolderSide ?? 'expense' },
            { onSuccess: () => setCreateFolderSide(null) },
          )
        }
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
              { onSuccess: close },
            )
          }
        }}
      />

      {classificationMenu.dialogs}

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
        <ResponsiveDialog open onOpenChange={(o) => !o && setMoveFolderTarget(null)} title={t('budgets.page.plan.menu.move_to_folder')}>
          <ul className="flex max-h-72 flex-col overflow-y-auto scrollbar-slim">
            {moveTargetFolders(moveFolderTarget.side).map((f) => (
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
    </>
  ) : null

  return {
    expenseRowMenu,
    incomeRowMenu,
    planRowMenu,
    envelopeChildMenu,
    savingsRowMenu,
    labelMenu,
    folderActionsFor,
    sectionMenu,
    savingsSectionMenu,
    editAccess,
    editFromSheet,
    editorOpen: envelopeDialog.open || categoryTarget !== null || tagTarget !== null,
    dialogs,
  }
}
