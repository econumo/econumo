import { ChevronDown, Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { RecurringDto } from '@/api/dto/recurring'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardField } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import { dayKey } from '@/lib/datetime'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'

export interface ViewRecurringDialogProps {
  recurring: RecurringDto
  onClose: () => void
  onPost?: () => void
  onSkip: () => void
  onEdit: () => void
  onDelete: () => void
  canChange: boolean
  dismissible?: boolean
  skipPending?: boolean
}

export function ViewRecurringDialog({
  recurring,
  onClose,
  onPost,
  onSkip,
  onEdit,
  onDelete,
  canChange,
  dismissible = true,
  skipPending = false,
}: ViewRecurringDialogProps) {
  const { t } = useTranslation()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()

  const isTransfer = recurring.type === 'transfer'
  const typeLabel = t(`accounts.page.preview_transaction_modal.type.${recurring.type}`)

  const account = accounts?.find((a) => a.id === recurring.accountId)
  const accountRecipient = accounts?.find((a) => a.id === recurring.accountRecipientId)
  const category = categories?.find((c) => c.id === recurring.categoryId)
  const payee = payees?.find((p) => p.id === recurring.payeeId)
  const tag = tags?.find((tg) => tg.id === recurring.tagId)

  const scheduleLabel = t(`recurring.schedule.${recurring.schedule}`)
  const heroIcon = isTransfer ? 'sync_alt' : (category?.icon ?? 'question_mark')
  const heroName = isTransfer ? typeLabel : (category?.name ?? typeLabel)

  const accountRow = (a: typeof account) => (
    <span className="flex items-center gap-2 text-sm">
      <EntityIcon name={a?.icon} className="text-base text-muted-foreground" />
      <span className="flex-1 truncate">{a?.name ?? t('accounts.account.name_hidden')}</span>
    </span>
  )

  const cards: { label: string; content: React.ReactNode }[] = []
  if (isTransfer) {
    cards.push({ label: t('accounts.page.preview_transaction_modal.sender.label'), content: accountRow(account) })
    cards.push({ label: t('accounts.page.preview_transaction_modal.recipient.label'), content: accountRow(accountRecipient) })
  } else {
    const label =
      recurring.type === 'expense'
        ? t('accounts.page.preview_transaction_modal.sender.label')
        : t('accounts.page.preview_transaction_modal.recipient.label')
    cards.push({ label, content: accountRow(account) })
    if (category) {
      cards.push({
        label: t('accounts.page.preview_transaction_modal.category.label'),
        content: (
          <span className="flex items-center gap-2 text-sm">
            <EntityIcon name={category.icon} className="text-base text-muted-foreground" />
            <span className="truncate">{category.name}</span>
          </span>
        ),
      })
    }
  }
  if (payee) {
    const payeeLabel =
      recurring.type === 'expense'
        ? t('accounts.page.preview_transaction_modal.recipient.label')
        : t('accounts.page.preview_transaction_modal.sender.label')
    cards.push({ label: payeeLabel, content: <span className="text-sm">{payee.name}</span> })
  }
  if (tag) {
    cards.push({
      label: t('accounts.page.preview_transaction_modal.tags.label'),
      content: (
        <span className="flex">
          <Badge variant="secondary">{tag.name}</Badge>
        </span>
      ),
    })
  }
  if (recurring.description) {
    cards.push({
      label: t('accounts.page.preview_transaction_modal.description.label'),
      content: <span className="break-words text-sm">{recurring.description}</span>,
    })
  }
  cards.push({ label: t('recurring.preview.schedule'), content: <span className="text-sm">{scheduleLabel}</span> })
  cards.push({ label: t('recurring.preview.next_payment'), content: <span className="text-sm">{dayKey(recurring.nextPaymentAt)}</span> })

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('recurring.preview.header')}
      hideHeader
      showClose
      dismissible={dismissible}
      footer={
        <div className="flex gap-3 [&_button]:h-11">
          <Button
            type="button"
            variant="secondary"
            size="icon"
            className="size-11"
            aria-label={t('common.button.cancel.label')}
            title={t('common.button.cancel.label')}
            onClick={onClose}
          >
            <ChevronDown className="size-4" />
          </Button>
          {onPost ? (
            <Button type="button" className="flex-1" disabled={!canChange} onClick={onPost}>
              {t('recurring.preview.post')}
            </Button>
          ) : null}
          <Button type="button" variant="secondary" className={onPost ? '' : 'flex-1'} disabled={!canChange || skipPending} onClick={onSkip}>
            {t('recurring.preview.skip')}
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="icon"
            className="size-11"
            disabled={!canChange}
            aria-label={t('common.button.edit.label')}
            title={t('common.button.edit.label')}
            onClick={onEdit}
          >
            <Pencil className="size-4" />
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="icon"
            className="size-11"
            disabled={!canChange}
            aria-label={t('common.button.delete.label')}
            title={t('common.button.delete.label')}
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      }
    >
      <div className="flex flex-col items-center gap-1 pb-4 pt-1 text-center">
        <span className="grid size-14 place-items-center rounded-full bg-econumo-card">
          <EntityIcon name={heroIcon} className="text-3xl text-[#666666]" />
        </span>
        <span className="mt-1 max-w-full truncate text-base font-medium" title={heroName}>
          {heroName}
        </span>
        <span className="text-2xl font-semibold tabular-nums">
          {moneyFormat(recurring.amount, account?.currency, { useNativePrecision: false })}
        </span>
        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span>{scheduleLabel}</span>
          <span aria-hidden="true">·</span>
          <span>{dayKey(recurring.nextPaymentAt)}</span>
        </span>
      </div>

      <div className="flex flex-col gap-2">
        {cards.map((card) => (
          <CardField key={card.label} label={card.label}>
            {card.content}
          </CardField>
        ))}
      </div>
    </ResponsiveDialog>
  )
}
