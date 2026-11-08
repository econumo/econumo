import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CardField } from '@/components/CardField'
import { useFormErrors } from '@/hooks/useFormErrors'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/datetime'
import { isNotEmpty } from '@/lib/validation'
import type { DebtDto } from '@/api/dto/debt'
import { useCurrencies } from '@/features/currencies/queries'
import { TransactionDatePicker } from '@/features/transactions/TransactionDatePicker'
import { useUpdateDebt } from './queries'
import { AmountField, ScheduleField, StartDateField } from './debtFields'
import type { FieldErrors } from './debtFields'
import { amountError, buildTermsPayload, useDebtTermsForm } from './useDebtForm'

// the fields a server refusal can name here; anything else lands in the form's alert
const SERVER_FIELDS = ['balance', 'payment', 'schedule', 'lastPaymentDate', 'startDate']

export interface DebtTermsFormProps {
  debt: DebtDto
  /** setup: the debt has no terms yet and the save may finish its setup; edit: change the terms */
  mode: 'setup' | 'edit'
  onSaved: (saved: DebtDto) => void
  /** shows a Cancel button beside the save */
  onCancel?: () => void
}

/**
 * What is owed now, payment, repeats, last payment date and start date. Editing
 * the balance or payment of a set-up debt re-fills the last payment date at the
 * debt's own rate; a debt not set up has no rate yet, so its date is typed.
 * Saving a debt not set up with a balance and a last payment date finishes its
 * setup; without them it only updates payment, repeats and start date.
 */
export function DebtTermsForm({ debt, mode, onSaved, onCancel }: DebtTermsFormProps) {
  const { t } = useTranslation()
  const formId = useId()
  const { data: currencies } = useCurrencies()
  const settingUp = debt.isSetUp !== 1
  const updateDebt = useUpdateDebt({ finishingSetup: settingUp })
  const { form, initial, patch, setBalance, setPayment, projection } = useDebtTermsForm(debt)
  const { errors, setErrors, clear: clearError } = useFormErrors<FieldErrors>()
  const [serverError, setServerError] = useState<string | null>(null)

  const symbol = currencies?.find((c) => c.id === debt.currencyId)?.symbol ?? ''
  const lastPaymentLabel = t('debts.form.last_payment_date.label')

  const validate = (): boolean => {
    const next: FieldErrors = {}
    const required = t('common.validation.required_field')
    next.payment = amountError(form.payment, t, true)
    // a debt not set up may be saved without its terms, never with only one
    const termsAsked = !settingUp || isNotEmpty(form.balance) || form.lastPaymentDate !== ''
    if (termsAsked) {
      next.balance = amountError(form.balance, t, false)
      if (!form.lastPaymentDate) {
        next.lastPaymentDate = required
      }
    }
    const found = Object.fromEntries(Object.entries(next).filter(([, v]) => v))
    setErrors(found)
    return Object.keys(found).length === 0
  }

  const submit = async () => {
    setServerError(null)
    if (!validate()) {
      return
    }
    try {
      const saved = await updateDebt.mutateAsync(buildTermsPayload(form, initial, debt))
      onSaved(saved)
    } catch (err) {
      const named: FieldErrors = {}
      for (const field of SERVER_FIELDS) {
        named[field] = apiFieldErrors(err, field)?.[0]
      }
      setErrors(named)
      if (!Object.values(named).some(Boolean)) {
        setServerError(apiErrorMessage(err))
      }
    }
  }

  const neverPaysOff = projection.status === 'never' ? t('debts.form.never_pays_off') : null

  return (
    <form
      id={formId}
      className="flex flex-col gap-4"
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <AmountField
        id={`${formId}-balance`}
        label={t('debts.form.balance.label')}
        value={form.balance}
        symbol={symbol}
        error={errors.balance}
        onChange={(balance) => {
          clearError('balance')
          setBalance(balance)
        }}
      />
      <AmountField
        id={`${formId}-payment`}
        label={t('debts.form.payment.label')}
        value={form.payment}
        symbol={symbol}
        error={errors.payment ?? neverPaysOff}
        onChange={(payment) => {
          clearError('payment')
          setPayment(payment)
        }}
      />
      <ScheduleField id={`${formId}-schedule`} value={form.schedule} error={errors.schedule} onChange={(schedule) => patch({ schedule })} />
      <CardField label={lastPaymentLabel} error={errors.lastPaymentDate}>
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">{t('debts.form.last_payment_date.hint')}</span>
          <TransactionDatePicker
            ariaLabel={lastPaymentLabel}
            placeholder={t('debts.form.last_payment_date.choose')}
            value={form.lastPaymentDate}
            onChange={(day) => {
              clearError('lastPaymentDate')
              patch({ lastPaymentDate: formatDate(day) })
            }}
          />
        </div>
      </CardField>
      <StartDateField
        value={form.startDate}
        error={errors.startDate}
        onChange={(startDate) => {
          clearError('startDate')
          patch({ startDate })
        }}
      />

      {serverError ? (
        <p role="alert" className="text-sm text-destructive">
          {serverError}
        </p>
      ) : null}

      <div className={onCancel ? 'grid grid-cols-2 gap-3 [&_button]:h-11' : 'flex [&_button]:h-11 [&_button]:flex-1'}>
        {onCancel ? (
          <Button type="button" variant="secondary" onClick={onCancel}>
            {t('common.button.cancel.label')}
          </Button>
        ) : null}
        <Button type="submit" disabled={updateDebt.isPending}>
          {mode === 'setup' && settingUp ? t('debts.terms.finish_setup') : t('common.button.save.label')}
        </Button>
      </div>
    </form>
  )
}
