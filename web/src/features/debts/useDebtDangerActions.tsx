import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/datetime'
import type { DebtDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { DateField } from './debtFields'
import { useDeleteDebt, useMarkDebtPaidOff, useReopenDebt } from './queries'

export interface DebtDangerActions {
  askMarkPaidOff: (id: Id) => void
  askDelete: (id: Id) => void
  /** a paid-off debt back to active, at once */
  reopen: (id: Id) => void
  markingPaidOff: boolean
  reopening: boolean
  /** the confirms; render once */
  dialogs: ReactNode
}

/** Mark as paid off (with its date), Reopen and Delete: one flow for the
 *  review screen's Danger zone and the budget's debt menus. Mark as paid off
 *  and Delete each ask first; a refusal is a toast unless onError takes it
 *  (a refused date shows under the date field). */
export function useDebtDangerActions({
  onPaidOff,
  onReopened,
  onDeleted,
  onError,
}: {
  onPaidOff?: (saved: DebtDto) => void
  onReopened?: (saved: DebtDto) => void
  onDeleted?: (id: Id) => void
  onError?: (message: string) => void
} = {}): DebtDangerActions {
  const { t } = useTranslation()
  const online = useOnline()
  const markPaidOff = useMarkDebtPaidOff()
  const reopenDebt = useReopenDebt()
  const deleteDebt = useDeleteDebt()
  const [confirmDelete, setConfirmDelete] = useState<Id | null>(null)
  const [paidOff, setPaidOff] = useState<{ id: Id; date: string; error?: string } | null>(null)

  const fail = (err: unknown) => {
    const message = apiErrorMessage(err)
    if (onError) {
      onError(message)
    } else {
      toast.error(message, { id: 'debt-action-error' })
    }
  }

  const sendPaidOff = () => {
    if (!paidOff) {
      return
    }
    const { id, date } = paidOff
    markPaidOff.mutate(
      { id, date },
      {
        onSuccess: (saved) => {
          setPaidOff(null)
          onPaidOff?.(saved)
        },
        onError: (err) => {
          const refused = apiFieldErrors(err, 'date')?.[0]
          if (refused) {
            setPaidOff({ id, date, error: refused })
            return
          }
          setPaidOff(null)
          fail(err)
        },
      },
    )
  }

  const dialogs = (
    <>
      <ResponsiveDialog
        open={paidOff !== null}
        onOpenChange={(o) => !o && setPaidOff(null)}
        title={t('debts.details.paid_off_confirm.title')}
        description={t('debts.details.paid_off_confirm.question')}
      >
        <div className="flex flex-col gap-4">
          <DateField
            label={t('debts.details.paid_off_confirm.date')}
            value={paidOff?.date ?? ''}
            error={paidOff?.error}
            onChange={(date) => setPaidOff((prev) => (prev ? { ...prev, date, error: undefined } : prev))}
          />
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={() => setPaidOff(null)}>
              {t('common.button.cancel.label')}
            </Button>
            <Button type="button" disabled={!online || markPaidOff.isPending || !paidOff?.date} onClick={sendPaidOff}>
              {t('debts.details.mark_paid_off')}
            </Button>
          </div>
        </div>
      </ResponsiveDialog>
      <ConfirmDialog
        open={confirmDelete !== null}
        onClose={() => setConfirmDelete(null)}
        onConfirm={() => {
          if (confirmDelete) {
            deleteDebt.mutate({ id: confirmDelete }, { onSuccess: () => onDeleted?.(confirmDelete), onError: fail })
          }
          setConfirmDelete(null)
        }}
        title={t('debts.details.delete_confirm.title')}
        question={t('debts.details.delete_confirm.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </>
  )

  return {
    askMarkPaidOff: (id) => setPaidOff({ id, date: formatDate(new Date()) }),
    askDelete: (id) => setConfirmDelete(id),
    reopen: (id) => reopenDebt.mutate(id, { onSuccess: (saved) => onReopened?.(saved), onError: fail }),
    markingPaidOff: markPaidOff.isPending,
    reopening: reopenDebt.isPending,
    dialogs,
  }
}
