import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecurringDto } from '@/api/dto/recurring'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { Button } from '@/components/ui/button'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees } from '@/features/classifications/queries'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useUserData } from '@/features/user/queries'
import { moneyFormat } from '@/lib/money'
import { dayKey, isFuture } from '@/lib/datetime'
import { useDeleteRecurring, useRecurring, useSkipRecurring } from './queries'
import { ViewRecurringDialog } from './ViewRecurringDialog'

export function RecurringSettingsPage() {
  const { t } = useTranslation()
  const { data: recurring = [] } = useRecurring()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: user } = useUserData()
  const openRecurringModal = useUiStore((s) => s.openRecurringModal)
  const skipRecurring = useSkipRecurring()
  const deleteRecurring = useDeleteRecurring()
  const [selected, setSelected] = useState<RecurringDto | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<RecurringDto | null>(null)

  const scheduleLabel = (rt: RecurringDto) => t(`modals.recurring.schedule.${rt.schedule}`)
  const accountOf = (rt: RecurringDto) => accounts?.find((a) => a.id === rt.accountId)
  const title = (rt: RecurringDto) =>
    rt.description ||
    payees?.find((p) => p.id === rt.payeeId)?.name ||
    categories?.find((c) => c.id === rt.categoryId)?.name ||
    scheduleLabel(rt)
  const canChangeRecurring = (rt: RecurringDto): boolean => {
    const account = accountOf(rt)
    if (!account) {
      return false
    }
    const myRole = account.sharedAccess.find((access) => access.user.id === user?.id)?.role
    return account.owner.id === user?.id || myRole === 'admin' || myRole === 'user'
  }

  return (
    <SettingsShell
      title={t('pages.settings.recurring.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        <Button type="button" size="sm" data-testid="recurring-create" onClick={() => openRecurringModal({})}>
          {t('pages.settings.recurring.create')}
        </Button>
      }
    >
      {recurring.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('pages.settings.recurring.empty')}</p>
      ) : (
        recurring.map((rt) => (
          <div
            key={rt.id}
            data-testid={`recurring-${rt.id}`}
            className="flex cursor-pointer items-center gap-3 rounded-md p-2 hover:bg-accent"
            onClick={() => setSelected(rt)}
          >
            <EntityIcon
              name={rt.type === 'transfer' ? 'sync_alt' : (categories?.find((c) => c.id === rt.categoryId)?.icon ?? 'question_mark')}
              className="text-muted-foreground"
            />
            <div className="min-w-0 flex-1">
              <p className="truncate">{title(rt)}</p>
              {/* a past next-payment date means the template needs attention (post or skip) */}
              <p
                data-testid={`recurring-summary-${rt.id}`}
                className={`text-sm ${isFuture(rt.nextPaymentAt) ? 'text-muted-foreground' : 'text-destructive'}`}
              >
                <span>{scheduleLabel(rt)}</span> · <span>{dayKey(rt.nextPaymentAt)}</span>
              </p>
            </div>
            <p className="text-sm">
              {moneyFormat(rt.amount, accountOf(rt)?.currency, { useNativePrecision: false })}
            </p>
          </div>
        ))
      )}
      {selected ? (
        <ViewRecurringDialog
          recurring={selected}
          onClose={() => setSelected(null)}
          onSkip={() => skipRecurring.mutate(selected.id, { onSuccess: () => setSelected(null) })}
          onEdit={() => {
            setSelected(null)
            openRecurringModal({ recurring: selected })
          }}
          onDelete={() => {
            setDeleteTarget(selected)
            setSelected(null)
          }}
          canChange={canChangeRecurring(selected)}
          skipPending={skipRecurring.isPending}
        />
      ) : null}

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteRecurring.mutate(deleteTarget.id, { onSettled: () => setDeleteTarget(null) })
          }
        }}
        question={t('pages.settings.recurring.delete_question')}
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
