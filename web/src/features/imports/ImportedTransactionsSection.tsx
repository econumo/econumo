import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ImportTransactionSuggestionDto } from '@/api/dto/imports'
import type { TransactionDto } from '@/api/dto/transaction'
import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { useTransactions } from '@/features/transactions/queries'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { enrichTransaction, useTransactionLookups, type TransactionLookups, type ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { toastWriteError } from '@/lib/apiError'
import { useDismissImportTransactionSuggestion, useFillImportTransactionSuggestion } from './queries'

// The transaction as Fill in would leave it: the suggested values over the transaction's own, read
// from the downloaded row when this device has it, else from what the suggestion says about it.
function suggestedTransaction(item: ImportTransactionSuggestionDto, stored: TransactionDto | undefined, lookups: TransactionLookups): ViewTransaction {
  const base = stored ?? {
    id: item.transactionId,
    type: item.type,
    accountId: item.accountId,
    accountRecipientId: null,
    amount: item.amount,
    amountRecipient: null,
    categoryId: item.transactionCategoryId,
    payeeId: item.transactionPayeeId,
    tagId: null,
    labelIds: [],
    description: item.description,
    date: item.date,
    recurringId: null,
    isImported: 1 as const,
  }
  return enrichTransaction({ ...base, categoryId: item.categoryId ?? base.categoryId, payeeId: item.payeeId ?? base.payeeId }, lookups)
}

export function ImportedTransactionsSection({ items }: { items: ImportTransactionSuggestionDto[] }) {
  const { t } = useTranslation()
  const online = useOnline()
  const { data: transactions } = useTransactions()
  const lookups = useTransactionLookups()
  const fill = useFillImportTransactionSuggestion()
  const dismiss = useDismissImportTransactionSuggestion()
  const [previewId, setPreviewId] = useState<string | null>(null)
  // resolved from the live list, so a suggestion gone meanwhile closes the sheet
  const preview = previewId ? items.find((item) => item.id === previewId) : undefined
  const pending = fill.isPending || dismiss.isPending

  const asTransaction = (item: ImportTransactionSuggestionDto) =>
    suggestedTransaction(item, transactions?.find((tx) => tx.id === item.transactionId), lookups)
  // Only on the row: the bank's text names the merchant where nothing else would. The sheet shows
  // the stored description, and the bank's text in its import card.
  const asRow = (item: ImportTransactionSuggestionDto) => {
    const tx = asTransaction(item)
    return { ...tx, description: tx.description || item.externalPayee }
  }
  const onError = toastWriteError
  const close = () => setPreviewId(null)

  return (
    <div className="flex flex-col gap-2">
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          data-testid={`transaction-suggestion-${item.id}`}
          className="w-full rounded-lg bg-econumo-card text-left hover:bg-econumo-hover"
          onClick={() => setPreviewId(item.id)}
        >
          {/* no pageAccount: the cross-account form names the account, which a list mixing accounts needs */}
          <TransactionRow transaction={asRow(item)} titleNote={t('imports.transaction_suggestions.suggested')} />
        </button>
      ))}

      {preview ? (
        <ViewTransactionDialog
          transaction={asTransaction(preview)}
          onClose={close}
          // never reached: the footer below replaces the row that would call these
          onEdit={() => {}}
          onDelete={() => {}}
          canChange={online}
          isShared={false}
          suggested={{ category: preview.categoryId !== null, payee: preview.payeeId !== null }}
          footer={
            /* dismiss | fill in — small, wide, as the recurring preview's footer */
            <div className="flex gap-3 [&_button]:h-11">
              <Button
                type="button"
                variant="secondary"
                size="icon"
                className="size-11"
                disabled={!online || pending}
                aria-label={t('imports.transaction_suggestions.dismiss')}
                title={t('imports.transaction_suggestions.dismiss')}
                onClick={() => dismiss.mutate(preview.id, { onSuccess: close, onError })}
              >
                <X className="size-4" />
              </Button>
              <Button
                type="button"
                className="flex-1"
                disabled={!online || pending}
                onClick={() => fill.mutate(preview.id, { onSuccess: close, onError })}
              >
                {t('imports.transaction_suggestions.fill')}
              </Button>
            </div>
          }
        />
      ) : null}
    </div>
  )
}
