import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import type { ViewTransaction } from './useAccountTransactions'

interface ViewTransactionDialogProps {
  transaction: ViewTransaction
  onClose: () => void
  onEdit: () => void
  onDelete: () => void
  canChange: boolean
}

export function ViewTransactionDialog({ transaction: tx, onClose, onEdit, onDelete, canChange }: ViewTransactionDialogProps) {
  const { t } = useTranslation()
  const rows: { label: string; content: React.ReactNode }[] = []

  const accountRow = (account: ViewTransaction['account'], amount: number | null) => (
    <span className="flex items-center gap-2">
      <EntityIcon name={account?.icon} className="text-base text-muted-foreground" />
      <span className="flex-1 truncate">{account?.name ?? t('elements.account.name_hidden')}</span>
      <span>{amount !== null ? moneyFormat(amount, account?.currency, { useNativePrecision: false }) : ''}</span>
    </span>
  )

  if (tx.type === 'transfer') {
    rows.push({ label: t('pages.account.preview_transaction_modal.sender.label'), content: accountRow(tx.account, tx.amount) })
    rows.push({
      label: t('pages.account.preview_transaction_modal.recipient.label'),
      content: accountRow(tx.accountRecipient, tx.amountRecipient),
    })
  } else {
    const label =
      tx.type === 'expense'
        ? t('pages.account.preview_transaction_modal.sender.label')
        : t('pages.account.preview_transaction_modal.recipient.label')
    rows.push({ label, content: accountRow(tx.account, tx.amount) })
  }
  if (tx.category) {
    rows.push({ label: t('pages.account.preview_transaction_modal.category.label'), content: tx.category.name })
  }
  if (tx.description) {
    rows.push({ label: t('pages.account.preview_transaction_modal.description.label'), content: tx.description })
  }
  if (tx.payee) {
    rows.push({ label: t('pages.account.preview_transaction_modal.payee.label'), content: tx.payee.name })
  }
  if (tx.tag) {
    rows.push({ label: t('pages.account.preview_transaction_modal.tags.label'), content: <Badge variant="secondary">{tx.tag.name}</Badge> })
  }
  if (tx.author) {
    rows.push({
      label: t('pages.account.preview_transaction_modal.author.label'),
      content: (
        <span className="flex items-center gap-2">
          <img src={`${tx.author.avatar}?s=30`} alt="" className="size-4 rounded-full" />
          {tx.author.name}
        </span>
      ),
    })
  }
  rows.push({ label: t('pages.account.preview_transaction_modal.created_at.label'), content: tx.date })

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('pages.account.preview_transaction_modal.header')}
      description={t(`pages.account.preview_transaction_modal.type.${tx.type}`)}
    >
      <dl className="flex flex-col gap-3">
        {rows.map((row) => (
          <div key={row.label} className="flex flex-col gap-0.5">
            <dt className="text-xs text-muted-foreground">{row.label}</dt>
            <dd className="text-sm">{row.content}</dd>
          </div>
        ))}
      </dl>
      <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:justify-end">
        <Button type="button" variant="secondary" onClick={onClose}>
          {t('elements.button.cancel.label')}
        </Button>
        <Button type="button" variant="destructive" disabled={!canChange} onClick={onDelete}>
          {t('elements.button.delete.label')}
        </Button>
        <Button type="button" disabled={!canChange} onClick={onEdit}>
          {t('elements.button.edit.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
