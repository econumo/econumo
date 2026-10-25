import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { BudgetUpcomingDto } from '@/api/dto/budget'
import type { RecurringDto } from '@/api/dto/recurring'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { recurringAsTransaction, upcomingAsTransaction } from './asTransaction'

interface TemplateViewProps {
  readOnly?: false
  recurring: RecurringDto
  onClose: () => void
  onPost?: () => void
  onSkip: () => void
  onEdit: () => void
  canChange: boolean
  dismissible?: boolean
  skipPending?: boolean
  postPending?: boolean
}

/** one projected occurrence from the budget list the caller can only look at:
    a later cycle, or a template they cannot act on */
interface OccurrenceViewProps {
  readOnly: true
  occurrence: BudgetUpcomingDto
  onClose: () => void
  /** the template's editor; omit when the caller cannot edit it */
  onEdit?: () => void
  /** muted line above the actions (why this payment cannot be posted yet) */
  note?: string
  dismissible?: boolean
}

export type ViewRecurringDialogProps = TemplateViewProps | OccurrenceViewProps

/**
 * A recurring template previewed with the transaction preview's own body: the
 * two used to be separate implementations of the same hero + card layout and
 * had visibly drifted (uncoloured amount, a redundant "Repeats" card, a
 * "Next payment" card duplicating the date already under the amount). The
 * template is shaped into a ViewTransaction — the same mapping the account
 * list already does for its unposted virtual rows — so only the action row
 * differs: hide | post | skip.
 */
export function ViewRecurringDialog(props: ViewRecurringDialogProps) {
  const { t } = useTranslation()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()
  const { data: labels } = useLabels()
  const { data: currencies } = useCurrencies()

  if (props.readOnly) {
    const { occurrence, onClose, onEdit, note, dismissible = true } = props
    return (
      <ViewTransactionDialog
        transaction={upcomingAsTransaction(occurrence, { accounts, currencies })}
        onClose={onClose}
        onEdit={onEdit ?? (() => {})}
        onDelete={() => {}}
        canChange={false}
        isShared={false}
        dismissible={dismissible}
        fallbackCurrency={currencies?.find((c) => c.id === occurrence.currencyId)}
        recurringSchedule={occurrence.schedule}
        onOpenRecurring={onEdit}
        footer={
          <div className="flex flex-col gap-3">
            {note ? <p className="text-center text-xs text-muted-foreground">{note}</p> : null}
            <div className="flex gap-3 [&_button]:h-11">
              <Button
                type="button"
                variant="secondary"
                size={onEdit ? 'icon' : 'default'}
                className={onEdit ? 'size-11' : 'flex-1'}
                aria-label={t('common.button.cancel.label')}
                title={t('common.button.cancel.label')}
                onClick={onClose}
              >
                <ChevronDown className="size-4" />
              </Button>
              {onEdit ? (
                <Button type="button" className="flex-1" onClick={onEdit}>
                  {t('common.button.edit.label')}
                </Button>
              ) : null}
            </div>
          </div>
        }
      />
    )
  }

  const { recurring, onClose, onPost, onSkip, onEdit, canChange, dismissible = true, skipPending = false, postPending = false } = props
  const asTransaction = recurringAsTransaction(recurring, { accounts, categories, payees, tags, labels })

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
           template is here to be acted on, and on this sheet it posts outright
           rather than opening a prefilled form: a due template is already the
           transaction the user meant. Editing is the schedule row in the body,
           and deleting lives on the settings list, not here. */
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
              <Button type="button" className="flex-1" disabled={!canChange || postPending || skipPending} onClick={onPost}>
                {t('recurring.preview.post')}
              </Button>
              <Button
                type="button"
                variant="secondary"
                size="icon"
                className="size-11"
                disabled={!canChange || skipPending || postPending}
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
