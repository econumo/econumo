import { useId, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/datetime'
import type { DebtDetailsDto, DebtRateChangeDto } from '@/api/dto/debt'
import { debtDay } from './debtDates'
import { AmountField, DateField } from './debtFields'
import { useAddDebtRateChange, useDeleteDebtRateChange } from './queries'
import { amountEcho, amountError, plainAmount } from './useDebtForm'

const percent = (rate: string) => (rate === '' ? '' : `${Number(rate).toFixed(2)}%`)

function RateForm({ details, symbol, amountEchoOf, rate: initialRate, payment: initialPayment }: { details: DebtDetailsDto; symbol: string; amountEchoOf: (raw: string) => string | null; rate: string; payment: string }) {
  const { t } = useTranslation()
  const formId = useId()
  const online = useOnline()
  const add = useAddDebtRateChange()
  const today = formatDate(new Date())
  const [rate, setRate] = useState(initialRate)
  const [payment, setPayment] = useState(initialPayment)
  const [date, setDate] = useState(today)
  const [errors, setErrors] = useState<{ rate?: string; payment?: string; date?: string }>({})
  const [serverError, setServerError] = useState<string | null>(null)

  const submit = async () => {
    setServerError(null)
    const next = { rate: amountError(rate, t, false), payment: amountError(payment, t, true), date: undefined }
    setErrors(next)
    if (next.rate || next.payment) {
      return
    }
    try {
      await add.mutateAsync({ id: details.item.id, date, annualRate: plainAmount(rate) ?? rate.trim(), payment: plainAmount(payment) ?? payment.trim() })
      setDate(today)
    } catch (err) {
      const named = { rate: apiFieldErrors(err, 'annualRate')?.[0], payment: apiFieldErrors(err, 'payment')?.[0], date: apiFieldErrors(err, 'date')?.[0] }
      setErrors(named)
      if (!named.rate && !named.payment && !named.date) {
        setServerError(apiErrorMessage(err))
      }
    }
  }

  return (
    <form
      className="flex flex-col gap-3"
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <h4 className="text-sm text-muted-foreground">{t('debts.rates.form.title')}</h4>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
        <DateField
          label={t('debts.rates.form.date')}
          value={date}
          error={errors.date}
          onChange={(day) => {
            setErrors((prev) => ({ ...prev, date: undefined }))
            setDate(day)
          }}
        />
        <div className="min-w-0 flex-1">
          <AmountField
            id={`${formId}-rate`}
            label={t('debts.rates.form.rate')}
            value={rate}
            symbol="%"
            error={errors.rate}
            onChange={(value) => {
              setErrors((prev) => ({ ...prev, rate: undefined }))
              setRate(value)
            }}
          />
        </div>
        <div className="min-w-0 flex-1">
          <AmountField
            id={`${formId}-payment`}
            label={t('debts.rates.form.payment')}
            value={payment}
            symbol={symbol}
            error={errors.payment}
            echo={amountEchoOf(payment)}
            onChange={(value) => {
              setErrors((prev) => ({ ...prev, payment: undefined }))
              setPayment(value)
            }}
          />
        </div>
      </div>
      {serverError ? (
        <p role="alert" className="text-sm text-destructive">
          {serverError}
        </p>
      ) : null}
      <div className="flex justify-end">
        <Button type="submit" disabled={add.isPending || !online}>
          {t('debts.rates.form.save')}
        </Button>
      </div>
    </form>
  )
}

/**
 * The Balance tab's rate changes: the yearly rate and regular payment from a
 * date on. With no stored row on the start date the first line is derived from
 * the loan's own terms (the implied rate and the origin's payment) and cannot
 * be deleted; every stored row can. `readOnly` keeps the table only.
 */
export function RateChangesCard({ details, amount, symbol, currency, lang, readOnly }: { details: DebtDetailsDto; amount: (value: string) => string; symbol: string; currency: Parameters<typeof amountEcho>[1]; lang: string; readOnly: boolean }) {
  const { t } = useTranslation()
  const formId = useId()
  const online = useOnline()
  const remove = useDeleteDebtRateChange()
  const [deleting, setDeleting] = useState<DebtRateChangeDto | null>(null)
  const [serverError, setServerError] = useState<string | null>(null)
  const { item, snapshots, rateChanges } = details
  const today = formatDate(new Date())
  const originPayment = snapshots.find((s) => s.isOrigin === 1)?.payment ?? item.payment
  const inForce = rateChanges.filter((r) => r.date <= today).at(-1)
  const derived = !rateChanges.some((r) => r.date === item.startDate)
  const prefillRate = item.annualRate === '' ? '' : Number(item.annualRate).toFixed(2)
  const prefillPayment = inForce?.payment ?? originPayment

  return (
    <section aria-labelledby={`${formId}-rates`} className="flex flex-col gap-3">
      <h3 id={`${formId}-rates`} className="text-sm uppercase tracking-wide text-muted-foreground">
        {t('debts.rates.title')}
      </h3>
      <div className="rounded-lg bg-econumo-card px-4 py-1.5">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-muted-foreground">
              <th scope="col" className="py-1 text-left font-normal">
                {t('debts.rates.column.date')}
              </th>
              <th scope="col" className="py-1 text-right font-normal">
                {t('debts.rates.column.rate')}
              </th>
              <th scope="col" className="py-1 text-right font-normal">
                {t('debts.rates.column.payment')}
              </th>
              {readOnly ? null : (
                <th scope="col" className="w-9 py-1">
                  <span className="sr-only">{t('common.button.delete.label')}</span>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {derived ? (
              <tr data-testid="debt-rate-change" className="border-t border-border">
                <td className="py-1.5 text-left">
                  {t('debts.rates.from_start')}
                  <span className="ml-1.5 text-xs text-muted-foreground">{t('debts.rates.from_terms')}</span>
                </td>
                <td className="whitespace-nowrap py-1.5 pl-2 text-right">{percent(item.impliedAnnualRate)}</td>
                <td className="whitespace-nowrap py-1.5 pl-2 text-right">{amount(originPayment)}</td>
                {readOnly ? null : <td />}
              </tr>
            ) : null}
            {rateChanges.map((r) => (
              <tr key={r.id} data-testid="debt-rate-change" className="border-t border-border">
                <td className="py-1.5 text-left">{debtDay(r.date, lang)}</td>
                <td className="whitespace-nowrap py-1.5 pl-2 text-right">{percent(r.annualRate)}</td>
                <td className="whitespace-nowrap py-1.5 pl-2 text-right">{amount(r.payment)}</td>
                {readOnly ? null : (
                  <td className="py-1.5 text-right">
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-8"
                      disabled={!online || remove.isPending}
                      aria-label={t('debts.rates.delete', { date: debtDay(r.date, lang) })}
                      onClick={() => setDeleting(r)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {readOnly ? null : (
        <RateForm key={`${prefillRate}|${prefillPayment}`} details={details} symbol={symbol} amountEchoOf={(raw) => amountEcho(raw, currency)} rate={prefillRate} payment={prefillPayment} />
      )}
      {serverError ? (
        <p role="alert" className="text-sm text-destructive">
          {serverError}
        </p>
      ) : null}
      <ConfirmDialog
        open={deleting !== null}
        onClose={() => setDeleting(null)}
        onConfirm={() => {
          const target = deleting
          setDeleting(null)
          if (target) {
            setServerError(null)
            remove.mutate({ id: item.id, rateChangeId: target.id }, { onError: (err) => setServerError(apiErrorMessage(err)) })
          }
        }}
        title={t('debts.rates.delete_confirm.title')}
        question={t('debts.rates.delete_confirm.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </section>
  )
}
