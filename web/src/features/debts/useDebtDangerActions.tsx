import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import type { DebtDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { useDeleteDebt, useMarkDebtPaidOff } from './queries'

export interface DebtDangerActions {
  askMarkPaidOff: (id: Id) => void
  askDelete: (id: Id) => void
  markingPaidOff: boolean
  /** the confirms; render once */
  dialogs: ReactNode
}

/** Mark as paid off and Delete, each behind its confirm, plus the second question
 *  a delete gets when the server says it would drop plans or comments. One flow
 *  for the review screen's Danger zone and the budget's debt row menu; without
 *  onError a refusal is a toast. */
export function useDebtDangerActions({
  onPaidOff,
  onDeleted,
  onError,
}: {
  onPaidOff?: (saved: DebtDto) => void
  onDeleted?: (id: Id) => void
  onError?: (message: string) => void
} = {}): DebtDangerActions {
  const { t } = useTranslation()
  const markPaidOff = useMarkDebtPaidOff()
  const deleteDebt = useDeleteDebt()
  const [confirm, setConfirm] = useState<{ kind: 'paid-off' | 'delete'; id: Id } | null>(null)
  // the server's own wording of what deleting drops, once it has refused
  const [removal, setRemoval] = useState<{ id: Id; message: string } | null>(null)

  const fail = (err: unknown) => {
    const message = apiErrorMessage(err)
    if (onError) {
      onError(message)
    } else {
      toast.error(message, { id: 'debt-action-error' })
    }
  }

  const sendDelete = (id: Id, confirmDataRemoval: boolean) => {
    deleteDebt.mutate(
      { id, ...(confirmDataRemoval ? { confirmDataRemoval: true } : {}) },
      {
        onSuccess: () => onDeleted?.(id),
        onError: (err) => {
          const refusal = apiFieldErrors(err, 'confirmDataRemoval')?.[0]
          if (!confirmDataRemoval && refusal) {
            setRemoval({ id, message: refusal })
            return
          }
          setRemoval(null)
          fail(err)
        },
      },
    )
  }

  const dialogs = (
    <>
      <ConfirmDialog
        open={confirm?.kind === 'paid-off'}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          if (confirm) {
            markPaidOff.mutate(confirm.id, { onSuccess: (saved) => onPaidOff?.(saved), onError: fail })
          }
          setConfirm(null)
        }}
        title={t('debts.details.paid_off_confirm.title')}
        question={t('debts.details.paid_off_confirm.question')}
        confirmLabel={t('debts.details.mark_paid_off')}
        cancelLabel={t('common.button.cancel.label')}
      />
      <ConfirmDialog
        open={confirm?.kind === 'delete'}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          if (confirm) {
            sendDelete(confirm.id, false)
          }
          setConfirm(null)
        }}
        title={t('debts.details.delete_confirm.title')}
        question={t('debts.details.delete_confirm.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
      <ConfirmDialog
        open={removal !== null}
        onClose={() => setRemoval(null)}
        onConfirm={() => {
          if (removal) {
            sendDelete(removal.id, true)
          }
          setRemoval(null)
        }}
        title={t('debts.details.removal_confirm.title')}
        question={removal?.message ?? ''}
        confirmLabel={t('debts.details.removal_confirm.action')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </>
  )

  return {
    askMarkPaidOff: (id) => setConfirm({ kind: 'paid-off', id }),
    askDelete: (id) => setConfirm({ kind: 'delete', id }),
    markingPaidOff: markPaidOff.isPending,
    dialogs,
  }
}
