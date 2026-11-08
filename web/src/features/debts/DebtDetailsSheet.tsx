import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { dayKey } from '@/lib/datetime'
import { useUiStore } from '@/app/uiStore'
import type { DebtDto } from '@/api/dto/debt'
import { useDebt, useDeleteDebt, useFetchDebtForEdit, useMarkDebtPaidOff } from './queries'
import { useDebtMoney, usePaymentsLeft } from './useDebtMoney'

function Figure({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1.5">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="text-right text-sm">{children}</dd>
    </div>
  )
}

function SectionHeading({ children }: { children: ReactNode }) {
  return <h3 className="mt-5 mb-2 text-sm uppercase tracking-wide text-muted-foreground">{children}</h3>
}

type Confirm = 'paid-off' | 'delete' | null

function DebtDetails({ debtId, onClose }: { debtId: string; onClose: () => void }) {
  const { t } = useTranslation()
  const { data, error: loadError } = useDebt(debtId)
  const money = useDebtMoney()
  const paymentsLeft = usePaymentsLeft()
  const openDebtModal = useUiStore((s) => s.openDebtModal)
  const fetchDebtForEdit = useFetchDebtForEdit()
  const markPaidOff = useMarkDebtPaidOff()
  const deleteDebt = useDeleteDebt()
  const [confirm, setConfirm] = useState<Confirm>(null)
  // the server's own wording of what deleting drops, once it has refused
  const [removalMessage, setRemovalMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const item: DebtDto | undefined = data?.item
  // a debt not set up sends its figures empty
  const amount = (value: string, debt: DebtDto) => (value === '' ? '—' : money(value, debt))

  const edit = () => {
    setError(null)
    fetchDebtForEdit(debtId)
      .then((fresh) => {
        onClose()
        openDebtModal({ debt: fresh.item })
      })
      .catch((err: unknown) => setError(apiErrorMessage(err)))
  }

  const sendDelete = (confirmDataRemoval: boolean) => {
    setError(null)
    deleteDebt.mutate(
      { id: debtId, ...(confirmDataRemoval ? { confirmDataRemoval: true } : {}) },
      {
        onSuccess: onClose,
        onError: (err) => {
          const refusal = apiFieldErrors(err, 'confirmDataRemoval')?.[0]
          if (!confirmDataRemoval && refusal) {
            setRemovalMessage(refusal)
            return
          }
          setRemovalMessage(null)
          setError(apiErrorMessage(err))
        },
      },
    )
  }

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={item?.name ?? t('debts.details.header')}
      titleIcon={item ? <EntityIcon name={item.icon} className="text-xl text-muted-foreground" /> : undefined}
      fullScreen
      footer={
        item ? (
          <div className="flex flex-wrap gap-2 [&_button]:h-11 [&_button]:flex-1">
            <Button type="button" variant="secondary" onClick={edit}>
              {item.isSetUp === 1 ? t('common.button.edit.label') : t('debts.terms.finish_setup')}
            </Button>
            <Button
              type="button"
              variant="secondary"
              disabled={item.isSetUp !== 1 || item.isPaidOff === 1 || markPaidOff.isPending}
              onClick={() => setConfirm('paid-off')}
            >
              {t('debts.details.mark_paid_off')}
            </Button>
            <Button type="button" variant="outline" className="text-destructive hover:text-destructive" onClick={() => setConfirm('delete')}>
              {t('common.button.delete.label')}
            </Button>
          </div>
        ) : undefined
      }
    >
      {item && data ? (
        <div className="flex flex-col">
          {item.isPaidOff === 1 ? (
            <div className="mb-2">
              <Badge variant="secondary">{t('debts.list.paid_off')}</Badge>
            </div>
          ) : null}
          <dl className="divide-y divide-border rounded-lg bg-econumo-card px-4 py-1.5">
            <Figure label={t('debts.details.current_balance')}>{amount(item.currentBalance, item)}</Figure>
            <Figure label={t('debts.details.payments_left')}>{item.isSetUp === 1 ? paymentsLeft(item.paymentsLeft) : '—'}</Figure>
            <Figure label={t('debts.details.money_left')}>{amount(item.moneyLeft, item)}</Figure>
            <Figure label={t('debts.details.overpayment_left')}>{amount(item.overpaymentLeft, item)}</Figure>
            <Figure label={t('debts.details.overpaid_so_far')}>{amount(item.overpaidSoFar, item)}</Figure>
            <Figure label={t('debts.details.payoff_date')}>{item.payoffDate || '—'}</Figure>
          </dl>

          <SectionHeading>{t('debts.details.snapshots')}</SectionHeading>
          <ul className="flex flex-col gap-1.5" aria-label={t('debts.details.snapshots')}>
            {data.snapshots.map((snapshot, index) => (
              <li key={`${snapshot.asOf}-${index}`} className="rounded-lg bg-econumo-card px-4 py-2 text-sm" data-testid="debt-snapshot">
                <div className="flex items-baseline justify-between gap-3">
                  <span className="text-muted-foreground">{snapshot.asOf}</span>
                  <span>{money(snapshot.balance, item)}</span>
                </div>
                <div className="text-xs text-muted-foreground">
                  {t('debts.details.snapshot_terms', {
                    payment: money(snapshot.payment, item),
                    schedule: t(`recurring.schedule.${snapshot.schedule}`),
                    date: snapshot.lastPaymentDate,
                  })}
                </div>
              </li>
            ))}
          </ul>

          <SectionHeading>{t('debts.details.payments')}</SectionHeading>
          {data.payments.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('debts.details.payments_empty')}</p>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="text-xs text-muted-foreground">
                  <th scope="col" className="py-1 text-left font-normal">
                    {t('debts.details.column.date')}
                  </th>
                  <th scope="col" className="py-1 text-right font-normal">
                    {t('debts.details.column.amount')}
                  </th>
                  <th scope="col" className="py-1 text-right font-normal">
                    {t('debts.details.column.interest')}
                  </th>
                  <th scope="col" className="py-1 text-right font-normal">
                    {t('debts.details.column.principal')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.payments.map((payment) => (
                  <tr key={payment.transactionId} data-testid="debt-payment" className="border-t border-border">
                    <td className="py-1.5 text-left">{dayKey(payment.date)}</td>
                    <td className="py-1.5 text-right">{money(payment.amount, item)}</td>
                    <td className="py-1.5 text-right">{money(payment.interest, item)}</td>
                    <td className="py-1.5 text-right">{money(payment.principal, item)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {error ? (
            <p role="alert" className="mt-3 text-sm text-destructive">
              {error}
            </p>
          ) : null}
        </div>
      ) : null}
      {!data && loadError ? (
        <p role="alert" className="text-sm text-destructive">
          {apiErrorMessage(loadError)}
        </p>
      ) : null}

      <ConfirmDialog
        open={confirm === 'paid-off'}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          setConfirm(null)
          setError(null)
          markPaidOff.mutate(debtId, { onError: (err) => setError(apiErrorMessage(err)) })
        }}
        title={t('debts.details.paid_off_confirm.title')}
        question={t('debts.details.paid_off_confirm.question')}
        confirmLabel={t('debts.details.mark_paid_off')}
        cancelLabel={t('common.button.cancel.label')}
      />
      <ConfirmDialog
        open={confirm === 'delete'}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          setConfirm(null)
          sendDelete(false)
        }}
        title={t('debts.details.delete_confirm.title')}
        question={t('debts.details.delete_confirm.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
      <ConfirmDialog
        open={removalMessage !== null}
        onClose={() => setRemovalMessage(null)}
        onConfirm={() => {
          setRemovalMessage(null)
          sendDelete(true)
        }}
        title={t('debts.details.removal_confirm.title')}
        question={removalMessage ?? ''}
        confirmLabel={t('debts.details.removal_confirm.action')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </ResponsiveDialog>
  )
}

/** The owner's view of one debt, opened with `useUiStore().openDebtDetails(id)` */
export function DebtDetailsSheet() {
  const debtId = useUiStore((s) => s.debtDetailsId)
  const close = useUiStore((s) => s.closeDebtDetails)

  if (!debtId) {
    return null
  }

  return <DebtDetails key={debtId} debtId={debtId} onClose={close} />
}
