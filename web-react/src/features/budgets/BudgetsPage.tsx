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
import { AccessLevelDialog } from '@/features/connections/AccessLevelDialog'
import { ShareAccessDialog } from '@/features/connections/ShareAccessDialog'
import type { ShareEntry } from '@/features/connections/shared'
import { buildShareEntries, hasBudgetAdminAccess } from '@/features/connections/shared'
import { useConnections } from '@/features/connections/queries'
import { useUserData, useUpdateDefaultBudget, userOption } from '@/features/user/queries'
import {
  useAcceptBudgetAccess,
  useBudgets,
  useCreateBudget,
  useDeclineBudgetAccess,
  useDeleteBudget,
  useGrantBudgetAccess,
  useRevokeBudgetAccess,
} from './queries'
import { BudgetDialog } from './BudgetDialog'

export function BudgetsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: user } = useUserData()
  const { data: budgets = [] } = useBudgets()
  const { data: connections = [] } = useConnections()
  const createBudget = useCreateBudget()
  const deleteBudget = useDeleteBudget()
  const updateDefaultBudget = useUpdateDefaultBudget()
  const acceptAccess = useAcceptBudgetAccess()
  const declineAccess = useDeclineBudgetAccess()
  const grantAccess = useGrantBudgetAccess()
  const revokeAccess = useRevokeBudgetAccess()

  const [createOpen, setCreateOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<BudgetMetaDto | null>(null)
  const [declineTarget, setDeclineTarget] = useState<BudgetMetaDto | null>(null)
  const [accessBudgetId, setAccessBudgetId] = useState<string | null>(null)
  const [levelEntry, setLevelEntry] = useState<ShareEntry | null>(null)
  const [errorOpen, setErrorOpen] = useState(false)

  // read the live cache copy so grant/revoke refreshes show in the open dialog
  const accessBudget = accessBudgetId ? budgets.find((b) => b.id === accessBudgetId) ?? null : null

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
                  <span className="flex items-center -space-x-2">
                    {budget.access.map((entry) => (
                      <img
                        key={entry.user.id}
                        src={`${entry.user.avatar}?s=50`}
                        alt={entry.user.name}
                        className="size-7 rounded-full ring-2 ring-background"
                      />
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
                    {!accepted ? (
                      <DropdownMenuItem onSelect={() => acceptAccess.mutate(budget.id)}>
                        {t('elements.button.accept.label')}
                      </DropdownMenuItem>
                    ) : null}
                    {accepted ? (
                      <DropdownMenuItem onSelect={() => goTo(budget)}>
                        {t('modules.budget.page.settings.list_actions.go_to')}
                      </DropdownMenuItem>
                    ) : null}
                    {user && hasBudgetAdminAccess(budget, user.id) ? (
                      <DropdownMenuItem onSelect={() => setAccessBudgetId(budget.id)}>
                        {t('modules.budget.page.settings.list_actions.access')}
                      </DropdownMenuItem>
                    ) : null}
                    {budget.ownerUserId !== user?.id ? (
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeclineTarget(budget)}>
                        {t('elements.button.decline.label')}
                      </DropdownMenuItem>
                    ) : null}
                    {user && hasBudgetAdminAccess(budget, user.id) ? (
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

      <ShareAccessDialog
        open={accessBudget !== null && levelEntry === null}
        title={accessBudget?.name ?? ''}
        kind="budgets"
        entries={accessBudget && user ? buildShareEntries(connections, accessBudget.access, user.id, accessBudget.ownerUserId) : []}
        onPick={(entry) => {
          if (entry.role !== 'owner') {
            setLevelEntry(entry)
          }
        }}
        onClose={() => setAccessBudgetId(null)}
      />

      <AccessLevelDialog
        open={levelEntry !== null}
        kind="budgets"
        user={levelEntry?.user ?? null}
        role={levelEntry?.role ?? null}
        onSelect={(role) => {
          if (levelEntry && accessBudgetId) {
            grantAccess.mutate({ budgetId: accessBudgetId, userId: levelEntry.user.id, role }, { onError: () => setErrorOpen(true) })
          }
          setLevelEntry(null)
        }}
        onRevoke={() => {
          if (levelEntry && accessBudgetId) {
            revokeAccess.mutate({ budgetId: accessBudgetId, userId: levelEntry.user.id })
          }
          setLevelEntry(null)
        }}
        onClose={() => setLevelEntry(null)}
      />

      <ConfirmDialog
        open={declineTarget !== null}
        onClose={() => setDeclineTarget(null)}
        onConfirm={() => {
          if (declineTarget) {
            declineAccess.mutate(declineTarget.id, { onSettled: () => setDeclineTarget(null) })
          }
        }}
        title={t('modules.budget.page.settings.decline_access_modal.title')}
        question={t('modules.budget.page.settings.decline_access_modal.question', { name: declineTarget?.name ?? '' })}
        confirmLabel={t('elements.button.decline.label')}
        cancelLabel={t('elements.button.cancel.label')}
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
