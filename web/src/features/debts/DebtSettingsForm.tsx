import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { dialogActionsClass } from '@/components/ResponsiveDialog'
import { OfflineNotice } from '@/components/OfflineNotice'
import { useFormErrors } from '@/hooks/useFormErrors'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { isNotEmpty } from '@/lib/validation'
import type { DebtDto } from '@/api/dto/debt'
import { useCurrencies } from '@/features/currencies/queries'
import { useUpdateDebt } from './queries'
import { AmountField, DatesRow, PaymentField } from './debtFields'
import type { FieldErrors } from './debtFields'
import { amountEcho, amountError, buildTermsPayload, factsAsked, useDebtTermsForm } from './useDebtForm'

// the fields a server refusal can name here; anything else lands in the form's alert
const SERVER_FIELDS = [
  'amountBorrowed',
  'payment',
  'schedule',
  'lastPaymentDate',
  'startDate',
]

export interface DebtSettingsFormProps {
  /** the debt as just loaded: the form starts from the loan's facts as stored now */
  debt: DebtDto
  onSaved: (saved: DebtDto) => void
  onDirtyChange?: (dirty: boolean) => void
  /** Finish setup: focus the first term still missing, then report it with onFocused */
  focusMissing?: boolean
  onFocused?: () => void
}

/**
 * The debt's Settings tab: the loan's facts from its contract (amount
 * borrowed, start and last payment dates, the payment and its repeats), in the
 * element's currency, saved in ONE update. Saving a debt not set
 * up with the loan's facts finishes its setup; without them the other fields
 * still save.
 */
export function DebtSettingsForm({ debt, onSaved, onDirtyChange, focusMissing = false, onFocused }: DebtSettingsFormProps) {
  const { t } = useTranslation()
  const online = useOnline()
  const formId = useId()
  const { data: currencies } = useCurrencies()
  const settingUp = debt.isSetUp !== 1
  const updateDebt = useUpdateDebt({ finishingSetup: settingUp })
  const { form, initial, patch, reset, dirty } = useDebtTermsForm(debt)
  const currency = currencies?.find((c) => c.id === debt.currencyId)
  const { errors, setErrors, clear: clearError } = useFormErrors<FieldErrors>()
  const [serverError, setServerError] = useState<string | null>(null)
  const borrowedRef = useRef<HTMLDivElement>(null)
  const datesRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => {
    if (!focusMissing) {
      return
    }
    // the row's two date chips, start first
    const chip = (index: number) => datesRef.current?.querySelectorAll('[data-date-chip]')[index]?.querySelector('button')
    const target = !isNotEmpty(form.amountBorrowed)
      ? borrowedRef.current?.querySelector('input')
      : !form.startDate
        ? chip(0)
        : !form.lastPaymentDate
          ? chip(1)
          : null
    target?.focus()
    onFocused?.()
    // only when asked: typing must not move the focus
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusMissing])

  const symbol = currency?.symbol ?? ''

  const validate = (): boolean => {
    const next: FieldErrors = {}
    const required = t('common.validation.required_field')
    next.payment = amountError(form.payment, t, true)
    // a debt not set up may be saved without the loan's facts, never with only some
    if (factsAsked(form, debt)) {
      next.amountBorrowed = amountError(form.amountBorrowed, t, true)
      next.startDate = form.startDate ? undefined : required
      next.lastPaymentDate = form.lastPaymentDate ? undefined : required
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
        <OfflineNotice />
        <div ref={borrowedRef}>
          <AmountField
            id={`${formId}-borrowed`}
            label={t('debts.form.amount_borrowed.label')}
            value={form.amountBorrowed}
            symbol={symbol}
            error={errors.amountBorrowed}
            echo={amountEcho(form.amountBorrowed, currency)}
            onChange={(amountBorrowed) => {
              clearError('amountBorrowed')
              patch({ amountBorrowed })
            }}
          />
        </div>
        <div ref={datesRef}>
          <DatesRow
            startDate={form.startDate}
            lastPaymentDate={form.lastPaymentDate}
            startError={errors.startDate}
            lastPaymentError={errors.lastPaymentDate}
            // a set-up debt always has its start: only a debt not set up drops it
            clearable={settingUp}
            onStartDateChange={(startDate) => {
              clearError('startDate')
              patch({ startDate })
            }}
            onLastPaymentDateChange={(lastPaymentDate) => {
              clearError('lastPaymentDate')
              patch({ lastPaymentDate })
            }}
          />
        </div>
        <PaymentField
          id={`${formId}-payment`}
          value={form.payment}
          symbol={symbol}
          schedule={form.schedule}
          error={errors.payment ?? errors.schedule}
          echo={amountEcho(form.payment, currency)}
          onChange={(payment) => {
            clearError('payment')
            patch({ payment })
          }}
          onScheduleChange={(schedule) => {
            clearError('schedule')
            patch({ schedule })
          }}
        />
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
          <Button type="submit" disabled={updateDebt.isPending || !online}>
            {t('common.button.save.label')}
          </Button>
        </div>
      </form>
    </>
  )
}
