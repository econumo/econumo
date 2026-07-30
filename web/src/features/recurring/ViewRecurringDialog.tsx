import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { RecurringDto } from '@/api/dto/recurring'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'

export interface ViewRecurringDialogProps {
  recurring: RecurringDto
  onClose: () => void
  onPost?: () => void
  onSkip: () => void
  onEdit: () => void
  canChange: boolean
  dismissible?: boolean
  skipPending?: boolean
}

/**
 * A recurring template previewed with the transaction preview's own body: the
 * two used to be separate implementations of the same hero + card layout and
 * had visibly drifted (uncoloured amount, a redundant "Repeats" card, a
 * "Next payment" card duplicating the date already under the amount). The
 * template is shaped into a ViewTransaction — the same mapping the account
 * list already does for its unposted virtual rows — so only the action row
 * differs: hide | post | skip.
 */
export function ViewRecurringDialog({
  recurring,
  onClose,
  onPost,
  onSkip,
  onEdit,
  canChange,
  dismissible = true,
  skipPending = false,
}: ViewRecurringDialogProps) {
  const { t } = useTranslation()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()

  const asTransaction = {
    id: recurring.id,
    type: recurring.type,
    accountId: recurring.accountId,
    accountRecipientId: recurring.accountRecipientId,
    amount: recurring.amount,
    // a template carries one amount; the recipient leg mirrors it
    amountRecipient: recurring.type === 'transfer' ? recurring.amount : null,
    categoryId: recurring.categoryId,
    payeeId: recurring.payeeId,
    tagId: recurring.tagId,
    description: recurring.description,
    // the date under the amount IS the next payment, which is why no separate
    // "next payment" card is needed
    date: recurring.nextPaymentAt,
    account: accounts?.find((a) => a.id === recurring.accountId),
    accountRecipient: recurring.accountRecipientId
      ? accounts?.find((a) => a.id === recurring.accountRecipientId)
      : undefined,
    category: recurring.categoryId ? categories?.find((c) => c.id === recurring.categoryId) : undefined,
    payee: recurring.payeeId ? payees?.find((p) => p.id === recurring.payeeId) : undefined,
    tag: recurring.tagId ? tags?.find((tg) => tg.id === recurring.tagId) : undefined,
    isInFuture: false,
    // drives the hero's recurring glyph; the schedule row below comes from the
    // recurringSchedule prop
    recurringId: recurring.id,
    recurring,
  } as unknown as ViewTransaction

  return (
    <ViewTransactionDialog
      transaction={asTransaction}
      onClose={onClose}
      // never reached: the recurring footer replaces the row that would call these
      onEdit={onEdit}
      onDelete={() => {}}
      canChange={canChange}
      isShared={false}
      dismissible={dismissible}
      recurringSchedule={recurring.schedule}
      onOpenRecurring={onEdit}
      footer={
        /* hide | post | skip — small, wide, small. Post leads because a due
           template is here to be acted on. Editing is the schedule row in the
           body, and deleting lives on the settings list, not here. */
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
            <>
              <Button type="button" className="flex-1" disabled={!canChange} onClick={onPost}>
                {t('recurring.preview.post')}
              </Button>
              <Button
                type="button"
                variant="secondary"
                size="icon"
                className="size-11"
                disabled={!canChange || skipPending}
                aria-label={t('recurring.preview.skip')}
                title={t('recurring.preview.skip')}
                onClick={onSkip}
              >
                <EntityIcon name="step_over" className="text-xl" />
              </Button>
            </>
          ) : (
            /* no Post (the settings list, where a template need not be due):
               skip carries the row as a labelled button instead */
            <Button
              type="button"
              variant="secondary"
              className="flex-1"
              disabled={!canChange || skipPending}
              onClick={onSkip}
            >
              {t('recurring.preview.skip')}
            </Button>
          )}
        </div>
      }
    />
  )
}
