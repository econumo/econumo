import { ChevronDown, ChevronRight, Repeat, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { RecurringSchedule } from '@/api/dto/recurring'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardField } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { UserAvatar } from '@/components/UserAvatar'
import { moneyFormat } from '@/lib/money'
import type { CurrencyLike } from '@/lib/money'
import type { ViewTransaction } from './useAccountTransactions'

interface ViewTransactionDialogProps {
  transaction: ViewTransaction
  onClose: () => void
  onEdit: () => void
  onDelete: () => void
  canChange: boolean
  /** whether the PAGE account is shared — the author avatar only makes sense then (same gate as the list rows) */
  isShared: boolean
  /** shield against dismissal while a stacked dialog (delete confirm) is open */
  dismissible?: boolean
  /** amount currency when the account isn't visible to the caller (budget rows) */
  fallbackCurrency?: CurrencyLike | null
  /** callback to make a recurring transaction from this one */
  onMakeRecurring?: () => void
  /** the schedule to show in the recurring row. For a posted transaction this is
      the template it came from, and the row is omitted when that template can't
      be resolved (deleted, or on an account the caller can't see) rather than
      rendering a dead end. */
  recurringSchedule?: RecurringSchedule
  /** opens the editor for the schedule above (the recurring row's click) */
  onOpenRecurring?: () => void
  /** replaces the whole action row. The recurring dialog shows the same body with
      its own actions (hide/post/skip), so the layout lives in one component. */
  footer?: React.ReactNode
}

/** A transaction posted from a template already has a schedule behind it, so
    offering "make recurring" on it invites accidental duplicate templates. The
    control is hidden rather than disabled: a disabled icon button explains
    nothing on touch, where there is no hover tooltip. */
function canMakeRecurring(tx: ViewTransaction, onMakeRecurring?: () => void): boolean {
  return onMakeRecurring !== undefined && !tx.recurringId
}

export function ViewTransactionDialog({ transaction: tx, onClose, onEdit, onDelete, canChange, isShared, dismissible = true, fallbackCurrency, onMakeRecurring, recurringSchedule, onOpenRecurring, footer }: ViewTransactionDialogProps) {
  const { t } = useTranslation()
  const showMakeRecurring = canMakeRecurring(tx, onMakeRecurring)
  // Indicator only, independent of whether the template itself resolved: the
  // transaction's own recurringId is enough to state that it came from a
  // schedule, even when the template is gone (so the row below is hidden).
  const isPostedFromRecurring = Boolean(tx.recurringId)
  const isTransfer = tx.type === 'transfer'
  const typeLabel = t(`accounts.page.preview_transaction_modal.type.${tx.type}`)

  const heroIcon = isTransfer ? 'sync_alt' : tx.category?.icon || 'question_mark'
  const heroName = isTransfer ? typeLabel : (tx.category?.name ?? t('common.uncategorized'))
  const sign = tx.type === 'expense' ? '-' : tx.type === 'income' ? '+' : ''
  const amountClass = tx.type === 'expense' ? 'text-expense' : tx.type === 'income' ? 'text-income' : ''

  const accountRow = (account: ViewTransaction['account'], amount: string | null) => (
    <span className="flex items-center gap-2 text-sm">
      <EntityIcon name={account?.icon} className="text-base text-muted-foreground" />
      <span className="flex-1 truncate">{account?.name ?? t('accounts.account.name_hidden')}</span>
      <span className="tabular-nums">
        {amount !== null ? moneyFormat(amount, account?.currency ?? fallbackCurrency, { useNativePrecision: false }) : ''}
      </span>
    </span>
  )

  const cards: { label: string; content: React.ReactNode }[] = []
  if (isTransfer) {
    cards.push({ label: t('accounts.page.preview_transaction_modal.sender.label'), content: accountRow(tx.account, tx.amount) })
    cards.push({
      label: t('accounts.page.preview_transaction_modal.recipient.label'),
      content: accountRow(tx.accountRecipient, tx.amountRecipient),
    })
  } else {
    const label =
      tx.type === 'expense'
        ? t('accounts.page.preview_transaction_modal.sender.label')
        : t('accounts.page.preview_transaction_modal.recipient.label')
    cards.push({ label, content: accountRow(tx.account, tx.amount) })
  }
  if (tx.description) {
    cards.push({
      label: t('accounts.page.preview_transaction_modal.description.label'),
      content: <span className="break-words text-sm">{tx.description}</span>,
    })
  }
  if (tx.payee) {
    // the payee sits on the opposite side of the money flow from the account:
    // an expense pays TO the payee, an income comes FROM it
    const payeeLabel =
      tx.type === 'expense'
        ? t('accounts.page.preview_transaction_modal.recipient.label')
        : t('accounts.page.preview_transaction_modal.sender.label')
    cards.push({ label: payeeLabel, content: <span className="text-sm">{tx.payee.name}</span> })
  }
  if (tx.tag) {
    cards.push({
      label: t('accounts.page.preview_transaction_modal.tags.label'),
      content: (
        <span className="flex">
          <Badge variant="secondary">{tx.tag.name}</Badge>
        </span>
      ),
    })
  }

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('accounts.page.preview_transaction_modal.header')}
      hideHeader
      showClose
      dismissible={dismissible}
      footer={
        footer ?? (
        /* dismiss on the left, actions on the right: collapse icon | wide Edit | delete icon.
           "Make recurring" is deliberately NOT here — it creates a new template rather than
           acting on this transaction, so it sits under the hero instead */
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
          <Button type="button" className="flex-1" disabled={!canChange} onClick={onEdit}>
            {t('common.button.edit.label')}
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
        )
      }
    >
      {/* hero: the category identity + the money, everything else is detail */}
      <div className="flex flex-col items-center gap-1 pb-4 pt-1 text-center">
        <span className="relative grid size-14 place-items-center rounded-full bg-econumo-card">
          <EntityIcon name={heroIcon} className="text-3xl text-[#666666]" />
          {isShared && tx.author ? (
            // the tooltip is the only place the preview names the author (no Author row)
            <span title={tx.author.name} className="absolute -bottom-1 -right-1.5">
              <UserAvatar avatar={tx.author.avatar} size="xs" className="size-6 border-2 border-background" />
            </span>
          ) : null}
        </span>
        <span className="mt-1 max-w-full truncate text-base font-medium" title={heroName}>
          {heroName}
        </span>
        {/* Whatever sits beside the amount, an equal-width spacer opposite keeps
            the amount optically centred. The two are mutually exclusive: only a
            hand-entered transaction offers the action, only a posted one carries
            the indicator (a plain glyph — there is nothing to press). */}
        <span className="flex items-center gap-2">
          {showMakeRecurring ? <span aria-hidden="true" className="size-11 shrink-0" /> : null}
          {isPostedFromRecurring ? <span aria-hidden="true" className="size-6 shrink-0" /> : null}
          <span className={`text-2xl font-semibold tabular-nums ${amountClass}`}>
            {sign}
            {moneyFormat(tx.amount, tx.account?.currency ?? fallbackCurrency, { useNativePrecision: false })}
          </span>
          {showMakeRecurring ? (
            <Button
              type="button"
              variant="secondary"
              size="icon"
              className="size-11 shrink-0"
              disabled={!canChange}
              aria-label={t('recurring.make_recurring')}
              title={t('recurring.make_recurring')}
              onClick={onMakeRecurring}
            >
              <Repeat className="size-4" />
            </Button>
          ) : null}
          {isPostedFromRecurring ? (
            <span
              title={t('recurring.preview.header')}
              className="flex size-6 shrink-0 items-center justify-center text-muted-foreground"
            >
              <Repeat className="size-5" aria-label={t('recurring.preview.header')} />
            </span>
          ) : null}
        </span>
        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span>{typeLabel}</span>
          <span aria-hidden="true">·</span>
          <span>{tx.date}</span>
        </span>
      </div>

      <div className="flex flex-col gap-2">
        {cards.map((card) => (
          <CardField key={card.label} label={card.label}>
            {card.content}
          </CardField>
        ))}
        {recurringSchedule && onOpenRecurring ? (
          // provenance, and a way through to the template that owns the schedule
          <button
            type="button"
            onClick={onOpenRecurring}
            title={t('recurring.preview.header')}
            className="flex w-full items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover"
          >
            <span className="flex min-w-0 flex-col gap-0.5">
              <span className="text-[11px] text-muted-foreground">{t('recurring.preview.header')}</span>
              <span className="flex items-center gap-2 text-sm">
                <Repeat className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <span className="truncate">{t(`recurring.schedule.${recurringSchedule}`)}</span>
              </span>
            </span>
            <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          </button>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}
