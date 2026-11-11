import { useId, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { OfflineNotice } from '@/components/OfflineNotice'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/datetime'
import { add } from '@/lib/decimal'
import type { DebtDetailsDto, DebtSnapshotDto } from '@/api/dto/debt'
import { useCurrencies } from '@/features/currencies/queries'
import { TransactionDatePicker } from '@/features/transactions/TransactionDatePicker'
import { debtDay } from './debtDates'
import { AmountField } from './debtFields'
import { useDeleteDebtReconciliation, useReconcileDebt } from './queries'
import { amountEcho, amountError, plainAmount } from './useDebtForm'

const negate = (v: string) => (v.startsWith('-') ? v.slice(1) : `-${v}`)

/** a statement of 0 is the debt marked paid off: no difference to show */
const paidOffMark = (s: DebtSnapshotDto) => s.isOrigin !== 1 && Number(s.balance) === 0

/** what the statement says less what Econumo had worked out for its date */
function statementDifference(snapshot: DebtSnapshotDto): string {
  return add(snapshot.balance, negate(snapshot.computedBalance))
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1.5">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="text-right text-sm">{children}</dd>
    </div>
  )
}

/**
 * The debt's Balance tab: reconciling it with a lender's statement. It shows
 * what Econumo works out for today and, once there is a statement, the latest
 * one with how far Econumo was off on its date; a form states a new statement
 * balance as of a date between the start date and today (a second one on the
 * same date replaces the first); the history lists the origin (the amount
 * borrowed, kept) and every statement, each deletable.
 */
export function DebtBalanceTab({ details, amount, lang }: { details: DebtDetailsDto; amount: (value: string) => string; lang: string }) {
  const { t } = useTranslation()
  const formId = useId()
  const online = useOnline()
  const { item, snapshots } = details
  const { data: currencies } = useCurrencies()
  const currency = currencies?.find((c) => c.id === item.currencyId)
  const reconcile = useReconcileDebt()
  const deleteReconciliation = useDeleteDebtReconciliation()
  const today = formatDate(new Date())
  const [balance, setBalance] = useState('')
  const [date, setDate] = useState(today)
  const [errors, setErrors] = useState<{ balance?: string; date?: string }>({})
  const [serverError, setServerError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<DebtSnapshotDto | null>(null)

  const statements = snapshots.filter((s) => s.isOrigin !== 1)
  const latest = statements.length > 0 ? statements.reduce((a, b) => (b.asOf >= a.asOf ? b : a)) : null

  const submit = async () => {
    setServerError(null)
    const next = {
      balance: amountError(balance, t, false),
      // the start day's balance is the amount borrowed, corrected in Settings
      date: date <= item.startDate || date > today ? t('debts.balance.form.date_range') : undefined,
    }
    setErrors(next)
    if (next.balance || next.date) {
      return
    }
    try {
      await reconcile.mutateAsync({ id: item.id, balance: plainAmount(balance) ?? balance.trim(), balanceDate: date })
      setBalance('')
      setDate(today)
    } catch (err) {
      const named = { balance: apiFieldErrors(err, 'balance')?.[0], date: apiFieldErrors(err, 'balanceDate')?.[0] }
      setErrors(named)
      if (!named.balance && !named.date) {
        setServerError(apiErrorMessage(err))
      }
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <OfflineNotice />
      <dl data-testid="debt-balance-summary" className="divide-y divide-border rounded-lg bg-econumo-card px-4 py-1.5">
        <Row label={t('debts.balance.econumo')}>{amount(item.currentBalance)}</Row>
        {latest ? (
          <>
            <Row label={t('debts.balance.last_statement')}>{t('debts.balance.statement_on', { amount: amount(latest.balance), date: debtDay(latest.asOf, lang) })}</Row>
            {paidOffMark(latest) ? null : <Row label={t('debts.balance.difference')}>{amount(statementDifference(latest))}</Row>}
          </>
        ) : null}
      </dl>

      <section aria-labelledby={`${formId}-title`} className="flex flex-col gap-3">
        <h3 id={`${formId}-title`} className="text-sm uppercase tracking-wide text-muted-foreground">
          {t('debts.balance.form.title')}
        </h3>
        <p className="text-sm text-muted-foreground">{t('debts.balance.form.hint')}</p>
        <form
          className="flex flex-col gap-3"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
            <div className="min-w-0 flex-1">
              <AmountField
                id={`${formId}-balance`}
                label={t('debts.balance.form.balance')}
                value={balance}
                symbol={currency?.symbol ?? ''}
                error={errors.balance}
                echo={amountEcho(balance, currency)}
                onChange={(value) => {
                  setErrors((prev) => ({ ...prev, balance: undefined }))
                  setBalance(value)
                }}
              />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <div className="flex flex-col gap-1 rounded-lg bg-econumo-card px-3 py-2.5 focus-within:ring-1 focus-within:ring-ring">
                <span className="text-[11px] text-muted-foreground">{t('debts.balance.form.date')}</span>
                <TransactionDatePicker
                  ariaLabel={t('debts.balance.form.date')}
                  value={date}
                  onChange={(day) => {
                    setErrors((prev) => ({ ...prev, date: undefined }))
                    setDate(formatDate(day))
                  }}
                />
              </div>
              {errors.date ? <p className="px-1 text-sm text-destructive">{errors.date}</p> : null}
            </div>
          </div>
          {serverError ? (
            <p role="alert" className="text-sm text-destructive">
              {serverError}
            </p>
          ) : null}
          <div className="flex justify-end">
            <Button type="submit" disabled={reconcile.isPending || !online}>
              {t('debts.balance.form.save')}
            </Button>
          </div>
        </form>
      </section>

      <section aria-labelledby={`${formId}-history`}>
        <h3 id={`${formId}-history`} className="mb-2 text-sm uppercase tracking-wide text-muted-foreground">
          {t('debts.balance.history')}
        </h3>
        <div className="rounded-lg bg-econumo-card px-4 py-1.5">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-xs text-muted-foreground">
                <th scope="col" className="py-1 text-left font-normal">
                  {t('debts.balance.column.date')}
                </th>
                <th scope="col" className="py-1 text-right font-normal">
                  {t('debts.balance.column.statement')}
                </th>
                <th scope="col" className="hidden py-1 text-right font-normal sm:table-cell">
                  {t('debts.balance.column.econumo')}
                </th>
                <th scope="col" className="py-1 text-right font-normal">
                  {t('debts.balance.column.difference')}
                </th>
                <th scope="col" className="w-9 py-1">
                  <span className="sr-only">{t('common.button.delete.label')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {snapshots.map((s) => (
                <tr key={s.id} data-testid="debt-reconciliation" className="border-t border-border">
                  <td className="py-1.5 text-left">
                    {debtDay(s.asOf, lang)}
                    {s.isOrigin === 1 ? <span className="ml-1.5 text-xs text-muted-foreground">{t('debts.balance.origin')}</span> : null}
                    {paidOffMark(s) ? <span className="ml-1.5 text-xs text-muted-foreground">{t('debts.balance.paid_off')}</span> : null}
                  </td>
                  <td className="py-1.5 text-right">{amount(s.balance)}</td>
                  <td className="hidden py-1.5 text-right sm:table-cell">{s.isOrigin === 1 ? '—' : amount(s.computedBalance)}</td>
                  <td className="py-1.5 text-right">{s.isOrigin === 1 || paidOffMark(s) ? '—' : amount(statementDifference(s))}</td>
                  <td className="py-1.5 text-right">
                    {s.isOrigin === 1 ? null : (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="size-8"
                        disabled={!online || deleteReconciliation.isPending}
                        aria-label={t('debts.balance.delete', { date: debtDay(s.asOf, lang) })}
                        onClick={() => setDeleting(s)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <ConfirmDialog
        open={deleting !== null}
        onClose={() => setDeleting(null)}
        onConfirm={() => {
          const target = deleting
          setDeleting(null)
          if (target) {
            setServerError(null)
            deleteReconciliation.mutate({ id: item.id, reconciliationId: target.id }, { onError: (err) => setServerError(apiErrorMessage(err)) })
          }
        }}
        title={t(deleting && paidOffMark(deleting) ? 'debts.balance.delete_paid_off_confirm.title' : 'debts.balance.delete_confirm.title')}
        question={t(deleting && paidOffMark(deleting) ? 'debts.balance.delete_paid_off_confirm.question' : 'debts.balance.delete_confirm.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
