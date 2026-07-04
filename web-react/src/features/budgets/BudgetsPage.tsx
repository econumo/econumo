import { useState } from 'react'
import { Bookmark, MoreVertical, Plus } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { UserOptions } from '@/api/dto/user'
import type { BudgetMetaDto } from '@/api/dto/budget'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useUserData, useUpdateDefaultBudget, userOption } from '@/features/user/queries'
import { useBudgets, useCreateBudget, useDeleteBudget } from './queries'
import { BudgetDialog } from './BudgetDialog'

export function BudgetsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: user } = useUserData()
  const { data: budgets = [] } = useBudgets()
  const createBudget = useCreateBudget()
  const deleteBudget = useDeleteBudget()
  const updateDefaultBudget = useUpdateDefaultBudget()

  const [createOpen, setCreateOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<BudgetMetaDto | null>(null)
  const [errorOpen, setErrorOpen] = useState(false)

  const defaultBudgetId = userOption(user, UserOptions.BUDGET)

  const myAccess = (budget: BudgetMetaDto) => budget.access.find((a) => a.user.id === user?.id)
  const isAccepted = (budget: BudgetMetaDto) => myAccess(budget)?.isAccepted === 1

  const goTo = (budget: BudgetMetaDto) => {
    if (defaultBudgetId !== budget.id) {
      updateDefaultBudget.mutate(budget.id)
    }
    navigate(RouterPage.BUDGET)
  }

  return (
    <SettingsShell
      title={t('modules.budget.page.settings.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        <Button type="button" size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          <span className="hidden sm:inline">{t('modules.budget.page.settings.create_budget')}</span>
        </Button>
      }
    >
      {budgets.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('blocks.list.list_empty')}</p>
      ) : (
        <ul className="flex flex-col">
          {budgets.map((budget) => {
            const access = myAccess(budget)
            const accepted = isAccepted(budget)
            const isDefault = defaultBudgetId === budget.id
            return (
              <li key={budget.id} className="flex items-center gap-2 rounded-md px-1 py-2">
                <button
                  type="button"
                  aria-label={isDefault ? `default budget ${budget.name}` : `set default ${budget.name}`}
                  disabled={isDefault || !accepted}
                  className="text-muted-foreground disabled:opacity-100"
                  onClick={() => updateDefaultBudget.mutate(budget.id)}
                >
                  <Bookmark className={`size-4 ${isDefault ? 'fill-current text-primary' : ''}`} />
                </button>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className={`truncate text-sm ${!accepted ? 'text-muted-foreground' : ''}`} title={budget.name}>
                    {budget.name}
                  </span>
                  {!accepted && access ? (
                    <span className="text-xs text-muted-foreground">
                      {t(`modules.budget.page.settings.level.${access.role}`)} - {t('modules.budget.page.settings.not_accepted')}
                    </span>
                  ) : null}
                </span>
                {budget.access.length > 1 ? (
                  <span className="flex items-center gap-0.5">
                    {budget.access.map((entry) => (
                      <img key={entry.user.id} src={`${entry.user.avatar}?s=30`} alt={entry.user.name} className="size-4 rounded-full" />
                    ))}
                  </span>
                ) : null}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" variant="ghost" size="icon" aria-label={`budget actions ${budget.name}`}>
                      <MoreVertical className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    {accepted ? (
                      <DropdownMenuItem onSelect={() => goTo(budget)}>
                        {t('modules.budget.page.settings.list_actions.go_to')}
                      </DropdownMenuItem>
                    ) : null}
                    {budget.ownerUserId === user?.id ? (
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(budget)}>
                        {t('elements.button.delete.label')}
                      </DropdownMenuItem>
                    ) : null}
                  </DropdownMenuContent>
                </DropdownMenu>
              </li>
            )
          })}
        </ul>
      )}

      <BudgetDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onSubmit={(form) => {
          createBudget.mutate(
            { id: uuidv7(), name: form.name, startDate: '', currencyId: form.currencyId, excludedAccounts: form.excludedAccounts, ownerUserId: user?.id },
            {
              onSuccess: () => setCreateOpen(false),
              onError: () => setErrorOpen(true),
            },
          )
        }}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteBudget.mutate(deleteTarget.id, { onSettled: () => setDeleteTarget(null) })
          }
        }}
        title={t('modules.budget.page.settings.delete_modal.title')}
        question={t('modules.budget.page.settings.delete_modal.question', { name: deleteTarget?.name ?? '' })}
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />

      <ResponsiveDialog
        open={errorOpen}
        onOpenChange={(o) => !o && setErrorOpen(false)}
        title={t('modules.budget.modal.generic_error.header')}
        description={t('modules.budget.modal.generic_error.description')}
      >
        <Button type="button" className="w-full" onClick={() => setErrorOpen(false)}>
          {t('elements.button.close.label')}
        </Button>
      </ResponsiveDialog>
    </SettingsShell>
  )
}
