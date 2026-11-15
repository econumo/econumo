import { useId, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
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
import { debtDay, shiftDay } from './debtDates'
import { AmountField, DateField } from './debtFields'
import { useDeleteDebtReconciliation, useReconcileDebt } from './queries'
import { RateChangesCard } from './RateChangesCard'
import { amountEcho, amountError, isNoFixed, plainAmount } from './useDebtForm'

// the card is econumo-card, so its fields take the dialog's own surface
const raisedField = 'bg-background'

const negate = (v: string) => (v.startsWith('-') ? v.slice(1) : `-${v}`)

/** a statement of 0 is the debt marked paid off: no difference to show */
const paidOffMark = (s: DebtSnapshotDto) => s.isOrigin !== 1 && Number(s.balance) === 0

/** what the statement says less what Econumo had worked out for its date */
function statementDifference(snapshot: DebtSnapshotDto): string {
  return add(snapshot.balance, negate(snapshot.computedBalance))
}

/**
 * The debt's Balance tab: reconciling it with a lender's statement. It shows
 * what Econumo works out for today, a link under the table opening a form that states a new
 * statement balance as of a date between the start date and today (a second
 * one on the same date replaces the first), and the table of the origin (the
 * amount borrowed, kept) and every statement with how far Econumo was off,
 * each deletable. All in one card. `readOnly` (a guest) keeps the figures and
 * the table, without the button or the deletes.
 */
export function DebtBalanceTab({ details, amount, lang, readOnly = false }: { details: DebtDetailsDto; amount: (value: string) => string; lang: string; readOnly?: boolean }) {
  const { t } = useTranslation()
  const formId = useId()
  const online = useOnline()
  const { item, snapshots } = details
  const { data: currencies } = useCurrencies()
  const currency = currencies?.find((c) => c.id === item.currencyId)
  const reconcile = useReconcileDebt()
  const deleteReconciliation = useDeleteDebtReconciliation()
  const today = formatDate(new Date())
  const [formOpen, setFormOpen] = useState(false)
  const [balance, setBalance] = useState('')
  const [date, setDate] = useState(today)
  const [errors, setErrors] = useState<{ balance?: string; date?: string }>({})
  const [serverError, setServerError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<DebtSnapshotDto | null>(null)


  const closeForm = () => {
    setFormOpen(false)
    setBalance('')
    setDate(today)
    setErrors({})
    setServerError(null)
  }

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
      closeForm()
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
      {readOnly ? null : <OfflineNotice />}
      <div data-testid="debt-balance-card" className="rounded-lg bg-econumo-card px-4 py-1.5">
        <div data-testid="debt-balance-summary" className="flex items-center justify-between gap-3 pb-3 pt-1.5">
          <div className="flex min-w-0 flex-col gap-0.5">
            <span className="text-sm text-muted-foreground">{t('debts.balance.econumo')}</span>
            <p data-testid="debt-balance-hint" className="text-sm text-muted-foreground">
              {t(isNoFixed(item.schedule) ? 'debts.balance.hint_none' : 'debts.balance.hint')}
            </p>
          </div>
          <span className="shrink-0 text-right text-2xl tabular-nums">{amount(item.currentBalance)}</span>
        </div>
        <table className="w-full border-t border-border text-sm">
        <thead>
          <tr className="text-xs text-muted-foreground">
            <th scope="col" className="py-1 text-left font-normal">
              {t('debts.balance.column.date')}
            </th>
            <th scope="col" className="py-1 pl-2 text-right font-normal">
              {t('debts.balance.column.statement')}
            </th>
            <th scope="col" className="hidden py-1 pl-2 text-right font-normal sm:table-cell">
              {t('debts.balance.column.econumo')}
            </th>
            <th scope="col" className="py-1 pl-2 text-right font-normal">
              {t('debts.balance.column.difference')}
            </th>
            {readOnly ? null : (
              <th scope="col" className="w-10 py-1">
                <span className="sr-only">{t('common.button.delete.label')}</span>
              </th>
            )}
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
              <td className="whitespace-nowrap py-1.5 pl-2 text-right">{amount(s.balance)}</td>
              <td className="hidden whitespace-nowrap py-1.5 pl-2 text-right sm:table-cell">{s.isOrigin === 1 ? '—' : amount(s.computedBalance)}</td>
              <td className="whitespace-nowrap py-1.5 pl-2 text-right">{s.isOrigin === 1 || paidOffMark(s) ? '—' : amount(statementDifference(s))}</td>
              {readOnly ? null : (
                <td className="w-10 py-1.5 text-right">
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
              )}
            </tr>
          ))}
        </tbody>
        </table>
        {readOnly ? null : formOpen ? (
          <section aria-label={t('debts.balance.enter')} className="border-t border-border py-3">
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
                    cardClassName={raisedField}
                    echo={amountEcho(balance, currency)}
                    onChange={(value) => {
                      setErrors((prev) => ({ ...prev, balance: undefined }))
                      setBalance(value)
                    }}
                  />
                </div>
                <DateField
                  label={t('debts.balance.form.date')}
                  value={date}
                  min={item.startDate ? shiftDay(item.startDate, 1) : undefined}
                  max={today}
                  error={errors.date}
                  cardClassName={raisedField}
                  onChange={(day) => {
                    setErrors((prev) => ({ ...prev, date: undefined }))
                    setDate(day)
                  }}
                />
              </div>
              {serverError ? (
                <p role="alert" className="text-sm text-destructive">
                  {serverError}
                </p>
              ) : null}
              <div className="flex justify-end gap-2">
                <Button type="button" variant="ghost" onClick={closeForm}>
                  {t('common.button.cancel.label')}
                </Button>
                <Button type="submit" disabled={reconcile.isPending || !online}>
                  {t('debts.balance.form.save')}
                </Button>
              </div>
            </form>
          </section>
        ) : (
          <div className="border-t border-border py-1.5">
            <button
              type="button"
              disabled={!online}
              className="inline-flex items-center gap-1 text-sm text-econumo-link disabled:opacity-50"
              onClick={() => setFormOpen(true)}
            >
              <Plus className="size-4" aria-hidden="true" />
              {t('debts.balance.enter')}
            </button>
          </div>
        )}
      </div>

      {isNoFixed(item.schedule) ? null : (
        <RateChangesCard details={details} amount={amount} symbol={currency?.symbol ?? ''} currency={currency} lang={lang} readOnly={readOnly} />
      )}

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
