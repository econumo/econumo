import { ChevronDown, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardField } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
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
}

export function ViewTransactionDialog({ transaction: tx, onClose, onEdit, onDelete, canChange, isShared, dismissible = true, fallbackCurrency }: ViewTransactionDialogProps) {
  const { t } = useTranslation()
  const isTransfer = tx.type === 'transfer'
  const typeLabel = t(`pages.account.preview_transaction_modal.type.${tx.type}`)

  const heroIcon = isTransfer ? 'sync_alt' : tx.category?.icon || 'question_mark'
  const heroName = isTransfer ? typeLabel : (tx.category?.name ?? typeLabel)
  const sign = tx.type === 'expense' ? '-' : tx.type === 'income' ? '+' : ''
  const amountClass = tx.type === 'expense' ? 'text-expense' : tx.type === 'income' ? 'text-income' : ''

  const accountRow = (account: ViewTransaction['account'], amount: number | null) => (
    <span className="flex items-center gap-2 text-sm">
      <EntityIcon name={account?.icon} className="text-base text-muted-foreground" />
      <span className="flex-1 truncate">{account?.name ?? t('elements.account.name_hidden')}</span>
      <span className="tabular-nums">
        {amount !== null ? moneyFormat(amount, account?.currency ?? fallbackCurrency, { useNativePrecision: false }) : ''}
      </span>
    </span>
  )

  const cards: { label: string; content: React.ReactNode }[] = []
  if (isTransfer) {
    cards.push({ label: t('pages.account.preview_transaction_modal.sender.label'), content: accountRow(tx.account, tx.amount) })
    cards.push({
      label: t('pages.account.preview_transaction_modal.recipient.label'),
      content: accountRow(tx.accountRecipient, tx.amountRecipient),
    })
  } else {
    const label =
      tx.type === 'expense'
        ? t('pages.account.preview_transaction_modal.sender.label')
        : t('pages.account.preview_transaction_modal.recipient.label')
    cards.push({ label, content: accountRow(tx.account, tx.amount) })
  }
  if (tx.description) {
    cards.push({
      label: t('pages.account.preview_transaction_modal.description.label'),
      content: <span className="break-words text-sm">{tx.description}</span>,
    })
  }
  if (tx.payee) {
    // the payee sits on the opposite side of the money flow from the account:
    // an expense pays TO the payee, an income comes FROM it
    const payeeLabel =
      tx.type === 'expense'
        ? t('pages.account.preview_transaction_modal.recipient.label')
        : t('pages.account.preview_transaction_modal.sender.label')
    cards.push({ label: payeeLabel, content: <span className="text-sm">{tx.payee.name}</span> })
  }
  if (tx.tag) {
    cards.push({
      label: t('pages.account.preview_transaction_modal.tags.label'),
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
      title={t('pages.account.preview_transaction_modal.header')}
      hideHeader
      showClose
      dismissible={dismissible}
      footer={
        /* dismiss on the left, actions on the right: collapse icon | wide Edit | delete icon */
        <div className="flex gap-3">
          <Button
            type="button"
            variant="secondary"
            size="icon"
            aria-label={t('elements.button.cancel.label')}
            title={t('elements.button.cancel.label')}
            onClick={onClose}
          >
            <ChevronDown className="size-4" />
          </Button>
          <Button type="button" className="flex-1" disabled={!canChange} onClick={onEdit}>
            {t('elements.button.edit.label')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="icon"
            disabled={!canChange}
            aria-label={t('elements.button.delete.label')}
            title={t('elements.button.delete.label')}
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      }
    >
      {/* hero: the category identity + the money, everything else is detail */}
      <div className="flex flex-col items-center gap-1 pb-4 pt-1 text-center">
        <span className="relative grid size-14 place-items-center rounded-full bg-econumo-card">
          <EntityIcon name={heroIcon} className="text-3xl text-[#666666]" />
          {isShared && tx.author ? (
            <img
              src={`${tx.author.avatar}?s=30`}
              alt={tx.author.name}
              title={tx.author.name}
              className="absolute -bottom-1 -right-1.5 size-6 rounded-full border-2 border-background"
            />
          ) : null}
        </span>
        <span className="mt-1 max-w-full truncate text-base font-medium" title={heroName}>
          {heroName}
        </span>
        <span className={`text-2xl font-semibold tabular-nums ${amountClass}`}>
          {sign}
          {moneyFormat(tx.amount, tx.account?.currency ?? fallbackCurrency, { useNativePrecision: false })}
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
      </div>
    </ResponsiveDialog>
  )
}
