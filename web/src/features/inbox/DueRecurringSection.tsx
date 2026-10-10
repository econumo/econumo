import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecurringDto } from '@/api/dto/recurring'
import { useUiStore } from '@/app/uiStore'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { useExchange } from '@/features/currencies/useExchange'
import { recurringAsTransaction } from '@/features/recurring/asTransaction'
import { recurringPostPayload } from '@/features/recurring/postPayload'
import { usePostRecurring, useRecurring, useSkipRecurring } from '@/features/recurring/queries'
import { ViewRecurringDialog } from '@/features/recurring/ViewRecurringDialog'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { isToday, dayKey, parseDateTime } from '@/lib/datetime'

export function DueRecurringSection({ items }: { items: RecurringDto[] }) {
  const { t, i18n } = useTranslation()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()
  const { data: labels } = useLabels()
  const { data: recurringList } = useRecurring()
  const exchangeFn = useExchange()
  const postRecurring = usePostRecurring()
  const skipRecurring = useSkipRecurring()
  const openRecurringModal = useUiStore((s) => s.openRecurringModal)
  const [previewId, setPreviewId] = useState<string | null>(null)
  // resolved from the live list so the sheet never acts on a stale template
  const preview = previewId ? recurringList?.find((rt) => rt.id === previewId) : undefined

  const dueNote = (rt: RecurringDto) => {
    const day = dayKey(rt.nextPaymentAt)
    if (isToday(day)) return <span>{t('inbox.due.today')}</span>
    const date = new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' }).format(parseDateTime(day))
    return <span className="text-destructive">{t('inbox.due.overdue', { date })}</span>
  }

  return (
    <div className="flex flex-col gap-2">
      {items.map((rt) => (
        <button
          key={rt.id}
          type="button"
          data-testid={`due-${rt.id}`}
          className="w-full rounded-lg bg-econumo-card text-left hover:bg-econumo-hover"
          onClick={() => setPreviewId(rt.id)}
        >
          {/* no pageAccount: the cross-account form names the account the
              template posts to, which a list mixing accounts needs */}
          <TransactionRow
            transaction={recurringAsTransaction(rt, { accounts, categories, payees, tags, labels })}
            dimmed={false}
            titleNote={t(`recurring.schedule.${rt.schedule}`)}
            amountNote={dueNote(rt)}
          />
        </button>
      ))}

      {preview ? (
        <ViewRecurringDialog
          recurring={preview}
          onClose={() => setPreviewId(null)}
          onPost={() => {
            postRecurring.mutate(recurringPostPayload(preview, accounts ?? [], exchangeFn), {
              onSuccess: () => setPreviewId(null),
            })
          }}
          onSkip={() => {
            skipRecurring.mutate(preview.id, { onSuccess: () => setPreviewId(null) })
          }}
          onEdit={() => {
            setPreviewId(null)
            openRecurringModal({ recurring: preview })
          }}
          // the Inbox only lists templates on accounts the caller can write to
          canChange
          skipPending={skipRecurring.isPending}
          postPending={postRecurring.isPending}
        />
      ) : null}
    </div>
  )
}
