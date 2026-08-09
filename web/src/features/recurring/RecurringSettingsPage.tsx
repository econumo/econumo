import { MoreVertical, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { AccountDto } from '@/api/dto/account'
import type { RecurringDto } from '@/api/dto/recurring'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useIsCompact } from '@/hooks/useIsCompact'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { useUserData } from '@/features/user/queries'
import { dayKey, isFuture } from '@/lib/datetime'
import { recurringAsTransaction } from './asTransaction'
import { useDeleteRecurring, useRecurring } from './queries'

export function RecurringSettingsPage() {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const { data: recurring = [] } = useRecurring()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()
  const { data: labels } = useLabels()
  const { data: user } = useUserData()
  const openRecurringModal = useUiStore((s) => s.openRecurringModal)
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const deleteRecurring = useDeleteRecurring()
  const [deleteTarget, setDeleteTarget] = useState<RecurringDto | null>(null)
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  // compact rows open a bottom-sheet action menu instead of the tiny kebab
  const [sheetItem, setSheetItem] = useState<RecurringDto | null>(null)

  const scheduleLabel = (rt: RecurringDto) => t(`recurring.schedule.${rt.schedule}`)
  const accountOf = (rt: RecurringDto) => accounts?.find((a) => a.id === rt.accountId)
  const asTransaction = (rt: RecurringDto) => recurringAsTransaction(rt, { accounts, categories, payees, tags, labels })
  // menu/sheet headings and aria labels still need a plain-text name; the row
  // itself derives its own title from the shaped transaction
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
  const edit = (rt: RecurringDto) => openRecurringModal({ recurring: rt })

  // Within a group, soonest next payment first (the wire datetime format
  // sorts lexicographically).
  const sortGroup = (items: RecurringDto[]) =>
    [...items].sort((a, b) => a.nextPaymentAt.localeCompare(b.nextPaymentAt) || a.id.localeCompare(b.id))
  // Grouped by account, in the account list's own order; templates on an
  // account the caller can't see trail behind under the hidden-name label.
  const byAccount = new Map<string, RecurringDto[]>()
  for (const rt of recurring) {
    byAccount.set(rt.accountId, [...(byAccount.get(rt.accountId) ?? []), rt])
  }
  const knownIds = new Set((accounts ?? []).map((a) => a.id))
  const groups = [
    ...(accounts ?? [])
      .filter((a) => byAccount.has(a.id))
      .map((a) => ({ key: a.id, label: a.name, items: sortGroup(byAccount.get(a.id) as RecurringDto[]) })),
    ...[...byAccount.entries()]
      .filter(([id]) => !knownIds.has(id))
      .map(([id, items]) => ({ key: id, label: t('accounts.account.name_hidden'), items: sortGroup(items) })),
  ]

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
      <InfoBox>{t('settings.recurring.info')}</InfoBox>
      {recurring.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('settings.recurring.empty')}</p>
      ) : (
        groups.map((group) => (
          <div key={group.key}>
            {/* the rows themselves never name the account, so the caption is
                informative even when a single group is visible */}
            <div className="mt-2 mb-1 flex items-center gap-3 px-1 pt-3 first:mt-0">
              <span className="text-sm font-semibold uppercase tracking-wide">{group.label}</span>
              <span className="h-px flex-1 bg-border" aria-hidden="true" />
            </div>
            {group.items.map((rt) => {
          const canChange = canChangeRecurring(rt)
          return (
            <div
              key={rt.id}
              data-testid={`recurring-${rt.id}`}
              /* padding comes from TransactionRow, exactly as on the account page,
                 so the two lists sit on the same grid */
              className={`flex items-start rounded-md ${isCompact ? 'active:bg-accent' : 'cursor-pointer hover:bg-accent'}`}
              onClick={() => (isCompact ? setSheetItem(rt) : setOpenMenuId(rt.id))}
            >
              {/* the same row the account list renders, so a template looks like
                  the transaction it will become. The schedule and next-payment
                  date are deliberately not repeated here — they live in the
                  preview and the edit form. */}
              <div className="min-w-0 flex-1">
                {accountOf(rt) ? (
                  <TransactionRow
                    transaction={asTransaction(rt)}
                    pageAccount={accountOf(rt) as AccountDto}
                    dimmed={false}
                    titleNote={scheduleLabel(rt)}
                    amountNote={
                      /* a past next payment means the template is due — post or skip it */
                      <span data-testid={`recurring-next-${rt.id}`} className={isFuture(rt.nextPaymentAt) ? undefined : 'text-destructive'}>
                        {dayKey(rt.nextPaymentAt)}
                      </span>
                    }
                  />
                ) : null}
              </div>
              {!isCompact ? (
                <DropdownMenu open={openMenuId === rt.id} onOpenChange={(open) => setOpenMenuId(open ? rt.id : null)}>
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="mt-0.5"
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
        })}
          </div>
        ))
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
