import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { dialogActionsClass } from '@/components/ResponsiveDialog'
import { useFormErrors } from '@/hooks/useFormErrors'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { isNotEmpty } from '@/lib/validation'
import type { DebtDto } from '@/api/dto/debt'
import { useCurrencies } from '@/features/currencies/queries'
import { useUpdateDebt } from './queries'
import { AmountField, DatesRow, PaymentField } from './debtFields'
import type { FieldErrors } from './debtFields'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import { amountEcho, amountError, buildTermsPayload, useDebtTermsForm } from './useDebtForm'

// the fields a server refusal can name here; anything else lands in the form's alert
const SERVER_FIELDS = ['name', 'categoryId', 'payeeId', 'tagId', 'labelId', 'currencyId', 'balance', 'payment', 'schedule', 'lastPaymentDate', 'startDate']

export interface DebtSettingsFormProps {
  /** the debt as just loaded: the form starts from what is owed now */
  debt: DebtDto
  onSaved: (saved: DebtDto) => void
  onDirtyChange?: (dirty: boolean) => void
  /** Finish setup: focus the first term still missing, then report it with onFocused */
  focusMissing?: boolean
  onFocused?: () => void
}

/**
 * The debt's Settings tab: name, which payments count, what is owed now, the
 * payment and its repeats, last payment date, start date and currency (the
 * amounts are read in it, never converted), saved in ONE update. Editing the
 * balance or payment of a set-up debt re-fills the last payment date at the
 * debt's own rate; a debt not set up has no rate yet, so its date is typed.
 * Saving a debt not set up with a balance and a last payment date finishes its
 * setup; without them the other fields still save.
 */
export function DebtSettingsForm({ debt, onSaved, onDirtyChange, focusMissing = false, onFocused }: DebtSettingsFormProps) {
  const { t } = useTranslation()
  const formId = useId()
  const { data: currencies } = useCurrencies()
  const settingUp = debt.isSetUp !== 1
  const updateDebt = useUpdateDebt({ finishingSetup: settingUp })
  const { form, initial, patch, setBalance, setPayment, reset, dirty, projection } = useDebtTermsForm(
    debt,
    (id) => currencies?.find((c) => c.id === id)?.fractionDigits,
  )
  const currency = currencies?.find((c) => c.id === form.currencyId)
  const { errors, setErrors, clear: clearError } = useFormErrors<FieldErrors>()
  const [serverError, setServerError] = useState<string | null>(null)
  const [currencyOpen, setCurrencyOpen] = useState(false)
  const balanceRef = useRef<HTMLDivElement>(null)
  const lastPaymentRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => {
    if (!focusMissing) {
      return
    }
    const target = !isNotEmpty(form.balance) ? balanceRef.current?.querySelector('input') : !form.lastPaymentDate ? lastPaymentRef.current?.querySelector('button') : null
    target?.focus()
    onFocused?.()
    // only when asked: typing must not move the focus
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusMissing])

  const symbol = currency?.symbol ?? ''
  const currencyLabel = t('debts.form.currency.label')
  const currencyName = currency ? fullCurrencyLabel(currency) : '—'

  const validate = (): boolean => {
    const next: FieldErrors = {}
    const required = t('common.validation.required_field')
    next.name = isNotEmpty(form.name.trim()) ? undefined : required
    next.categoryId = form.categoryId ? undefined : required
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
    <>
      <form
        id={formId}
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <CardField label={t('debts.form.name.label')} htmlFor={`${formId}-name`} error={errors.name}>
          <Input
            id={`${formId}-name`}
            className={cardFieldControlClass}
            autoComplete="off"
            value={form.name}
            onChange={(e) => {
              clearError('name')
              patch({ name: e.target.value })
            }}
          />
        </CardField>

        <PaymentsFromEditor
          value={{ categoryId: form.categoryId, payeeId: form.payeeId, tagId: form.tagId, labelId: form.labelId }}
          onChange={(next) => {
            for (const key of ['categoryId', 'payeeId', 'tagId', 'labelId'] as const) {
              clearError(key)
            }
            patch(next)
          }}
          errors={{ categoryId: errors.categoryId, payeeId: errors.payeeId, tagId: errors.tagId, labelId: errors.labelId }}
        />

        <div ref={balanceRef}>
          <AmountField
            id={`${formId}-balance`}
            label={t('debts.form.balance.label')}
            value={form.balance}
            symbol={symbol}
            error={errors.balance}
            echo={amountEcho(form.balance, currency)}
            onChange={(balance) => {
              clearError('balance')
              setBalance(balance)
            }}
          />
        </div>
        <PaymentField
          id={`${formId}-payment`}
          value={form.payment}
          symbol={symbol}
          schedule={form.schedule}
          error={errors.payment ?? errors.schedule ?? neverPaysOff}
          echo={amountEcho(form.payment, currency)}
          onChange={(payment) => {
            clearError('payment')
            setPayment(payment)
          }}
          onScheduleChange={(schedule) => {
            clearError('schedule')
            patch({ schedule })
          }}
        />
        <DatesRow
          startDate={form.startDate}
          lastPaymentDate={form.lastPaymentDate}
          startError={errors.startDate}
          lastPaymentError={errors.lastPaymentDate}
          lastPaymentRef={lastPaymentRef}
          onStartDateChange={(startDate) => {
            clearError('startDate')
            patch({ startDate })
          }}
          onLastPaymentDateChange={(lastPaymentDate) => {
            clearError('lastPaymentDate')
            patch({ lastPaymentDate })
          }}
        />

        <CardField label={currencyLabel} error={errors.currencyId}>
          <button
            type="button"
            aria-label={`${currencyLabel}: ${currencyName}`}
            className="min-w-0 truncate text-left text-sm text-econumo-link"
            onClick={() => setCurrencyOpen(true)}
          >
            {currencyName}
          </button>
        </CardField>

        {serverError ? (
          <p role="alert" className="text-sm text-destructive">
            {serverError}
          </p>
        ) : null}

        <div className={dialogActionsClass}>
          <Button
            type="button"
            variant="secondary"
            onClick={() => {
              reset()
              setErrors({})
              setServerError(null)
            }}
          >
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={updateDebt.isPending}>
            {t('common.button.save.label')}
          </Button>
        </div>
      </form>
      <CurrencyPickerDialog
        open={currencyOpen}
        title={currencyLabel}
        value={form.currencyId}
        onClose={() => setCurrencyOpen(false)}
        onPick={(currencyId) => {
          clearError('currencyId')
          patch({ currencyId })
        }}
      />
    </>
  )
}
