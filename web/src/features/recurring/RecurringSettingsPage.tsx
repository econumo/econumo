import { MoreVertical, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecurringDto } from '@/api/dto/recurring'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useIsCompact } from '@/hooks/useIsCompact'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees } from '@/features/classifications/queries'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useUserData } from '@/features/user/queries'
import { moneyFormat } from '@/lib/money'
import { dayKey, isFuture } from '@/lib/datetime'
import { useDeleteRecurring, useRecurring, useSkipRecurring } from './queries'

export function RecurringSettingsPage() {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const { data: recurring = [] } = useRecurring()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: user } = useUserData()
  const openRecurringModal = useUiStore((s) => s.openRecurringModal)
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const skipRecurring = useSkipRecurring()
  const deleteRecurring = useDeleteRecurring()
  const [deleteTarget, setDeleteTarget] = useState<RecurringDto | null>(null)
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  // compact rows open a bottom-sheet action menu instead of the tiny kebab
  const [sheetItem, setSheetItem] = useState<RecurringDto | null>(null)

  const scheduleLabel = (rt: RecurringDto) => t(`recurring.schedule.${rt.schedule}`)
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

  const post = (rt: RecurringDto) => openTransactionModal({ postRecurring: rt })
  const skip = (rt: RecurringDto) => skipRecurring.mutate(rt.id)
  const edit = (rt: RecurringDto) => openRecurringModal({ recurring: rt })

  const createLabel = t('settings.recurring.create')

  return (
    <SettingsShell
      title={t('settings.recurring.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        isCompact ? (
          <Button type="button" size="icon" aria-label={createLabel} title={createLabel} data-testid="recurring-create" onClick={() => openRecurringModal({})}>
            <Plus className="size-4" />
          </Button>
        ) : (
          <Button type="button" size="sm" data-testid="recurring-create" onClick={() => openRecurringModal({})}>
            <Plus className="size-4" />
            {createLabel}
          </Button>
        )
      }
    >
      {recurring.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('settings.recurring.empty')}</p>
      ) : (
        recurring.map((rt) => {
          const canChange = canChangeRecurring(rt)
          return (
            <div
              key={rt.id}
              data-testid={`recurring-${rt.id}`}
              className={`flex items-center rounded-md px-1 ${
                isCompact ? 'gap-3 py-3 active:bg-accent' : 'gap-2 py-1.5 cursor-pointer hover:bg-accent'
              }`}
              onClick={() => (isCompact ? setSheetItem(rt) : setOpenMenuId(rt.id))}
            >
              <EntityIcon
                name={rt.type === 'transfer' ? 'sync_alt' : (categories?.find((c) => c.id === rt.categoryId)?.icon ?? 'question_mark')}
                className="text-base text-muted-foreground"
              />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm">{title(rt)}</p>
                {/* a past next-payment date means the template needs attention (post or skip) */}
                <p
                  data-testid={`recurring-summary-${rt.id}`}
                  className={`text-sm ${isFuture(rt.nextPaymentAt) ? 'text-muted-foreground' : 'text-destructive'}`}
                >
                  <span>{scheduleLabel(rt)}</span> · <span>{dayKey(rt.nextPaymentAt)}</span>
                </p>
              </div>
              <p className="text-sm tabular-nums">
                {moneyFormat(rt.amount, accountOf(rt)?.currency, { useNativePrecision: false })}
              </p>
              {!isCompact ? (
                <DropdownMenu open={openMenuId === rt.id} onOpenChange={(open) => setOpenMenuId(open ? rt.id : null)}>
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`actions ${title(rt)}`}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <MoreVertical className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  {/* portaled content still bubbles React clicks to the row — don't reopen the menu */}
                  <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
                    <DropdownMenuItem disabled={!canChange} onSelect={() => post(rt)}>
                      {t('recurring.preview.post')}
                    </DropdownMenuItem>
                    <DropdownMenuItem disabled={!canChange || skipRecurring.isPending} onSelect={() => skip(rt)}>
                      {t('recurring.preview.skip')}
                    </DropdownMenuItem>
                    <DropdownMenuItem disabled={!canChange} onSelect={() => edit(rt)}>
                      {t('common.button.edit.label')}
                    </DropdownMenuItem>
                    <DropdownMenuItem variant="destructive" disabled={!canChange} onSelect={() => setDeleteTarget(rt)}>
                      {t('common.button.delete.label')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : null}
            </div>
          )
        })
      )}

      {/* compact tap-on-row action sheet */}
      <ResponsiveDialog open={sheetItem !== null} onOpenChange={(open) => !open && setSheetItem(null)} title={sheetItem ? title(sheetItem) : ''}>
        <div className="flex flex-col gap-2 [&_button]:h-11">
          <Button
            type="button"
            disabled={!sheetItem || !canChangeRecurring(sheetItem)}
            onClick={() => {
              if (sheetItem) {
                post(sheetItem)
              }
              setSheetItem(null)
            }}
          >
            {t('recurring.preview.post')}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={!sheetItem || !canChangeRecurring(sheetItem) || skipRecurring.isPending}
            onClick={() => {
              if (sheetItem) {
                skip(sheetItem)
              }
              setSheetItem(null)
            }}
          >
            {t('recurring.preview.skip')}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={!sheetItem || !canChangeRecurring(sheetItem)}
            onClick={() => {
              if (sheetItem) {
                edit(sheetItem)
              }
              setSheetItem(null)
            }}
          >
            {t('common.button.edit.label')}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="text-destructive hover:text-destructive"
            disabled={!sheetItem || !canChangeRecurring(sheetItem)}
            onClick={() => {
              setDeleteTarget(sheetItem)
              setSheetItem(null)
            }}
          >
            {t('common.button.delete.label')}
          </Button>
        </div>
      </ResponsiveDialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteRecurring.mutate(deleteTarget.id, { onSettled: () => setDeleteTarget(null) })
          }
        }}
        question={t('settings.recurring.delete_question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
