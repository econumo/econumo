import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useFormErrors } from '@/hooks/useFormErrors'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { isNotEmpty } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import type { OpenDebtParams } from '@/app/uiStore'
import type { DebtDto } from '@/api/dto/debt'
import { useCategories } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { useCreateDebt } from './queries'
import { AmountField, ScheduleField, StartDateField } from './debtFields'
import type { FieldErrors } from './debtFields'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import { amountError, buildCreatePayload, useDebtCreateForm } from './useDebtForm'

// the fields a server refusal can name; anything else lands in the form's alert
const SERVER_FIELDS = ['name', 'currencyId', 'categoryId', 'payeeId', 'tagId', 'labelId', 'startDate', 'payment', 'schedule']

/** A new debt: what identifies it, and nothing else. The terms come later,
    with Finish setup on the debt's own screen. */
function DebtCreateForm({ params, onDone }: { params: OpenDebtParams; onDone: () => void }) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: categories = [] } = useCategories()
  const { data: currencies } = useCurrencies()
  const createDebt = useCreateDebt({ fromBudget: params.fromBudget })
  const openDebtDetails = useUiStore((s) => s.openDebtDetails)

  const defaultCurrencyId = userCurrencyId(user)
  const { form, patch } = useDebtCreateForm(params, categories, defaultCurrencyId)
  const { errors, setErrors, clear: clearError } = useFormErrors<FieldErrors>()
  const [serverError, setServerError] = useState<string | null>(null)
  const [currencyOpen, setCurrencyOpen] = useState(false)
  // asked only once another debt turns out to have the same filters and start
  // date: a start date then tells a second loan in the same category apart
  const [askStartDate, setAskStartDate] = useState(false)

  const currencyId = form.currencyId ?? defaultCurrencyId
  const currency = currencies?.find((c) => c.id === currencyId)

  const validate = (): boolean => {
    const required = t('common.validation.required_field')
    const next: FieldErrors = {
      name: isNotEmpty(form.name.trim()) ? undefined : required,
      currencyId: currencyId ? undefined : required,
      categoryId: form.categoryId ? undefined : required,
      payment: amountError(form.payment, t, true),
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
    let saved: DebtDto
    try {
      saved = await createDebt.mutateAsync(buildCreatePayload({ ...form, currencyId }))
    } catch (err) {
      const named: FieldErrors = {}
      for (const field of SERVER_FIELDS) {
        named[field] = apiFieldErrors(err, field)?.[0]
      }
      setErrors(named)
      // identical filters and start date as another debt (debt.start_date_taken):
      // a start date tells the loans apart. A partial overlap (debt.key_conflict,
      // on categoryId) needs other filters, so it only shows its message.
      if (named.startDate) {
        setAskStartDate(true)
      }
      if (!Object.values(named).some(Boolean)) {
        setServerError(apiErrorMessage(err))
      }
      return
    }
    onDone()
    openDebtDetails(saved.id)
  }

  return (
    <ResponsiveDialog
      open
      caps
      fullScreen
      dismissible={false}
      onOpenChange={(o) => !o && onDone()}
      title={t('debts.form.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="debt-dialog-form" disabled={createDebt.isPending}>
            {t('common.button.add.label')}
          </Button>
        </div>
      }
    >
      <form
        id="debt-dialog-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <p className="text-sm text-muted-foreground">{t('debts.form.intro')}</p>

        <CardField label={t('debts.form.name.label')} htmlFor="debt-name" error={errors.name}>
          <Input
            id="debt-name"
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
          categoryLocked={!!params.categoryId}
          errors={{ categoryId: errors.categoryId, payeeId: errors.payeeId, tagId: errors.tagId, labelId: errors.labelId }}
        />

        <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-[3fr_2fr]">
          <AmountField
            id="debt-payment"
            label={t('debts.form.payment.label')}
            value={form.payment}
            symbol={currency?.symbol ?? ''}
            error={errors.payment}
            onChange={(payment) => {
              clearError('payment')
              patch({ payment })
            }}
          />
          <ScheduleField id="debt-schedule" value={form.schedule} error={errors.schedule} onChange={(schedule) => patch({ schedule })} />
        </div>

        {askStartDate ? (
          <StartDateField
            value={form.startDate}
            error={errors.startDate}
            onChange={(startDate) => {
              clearError('startDate')
              patch({ startDate })
            }}
          />
        ) : null}

        <div className="flex flex-col gap-1">
          <div className="flex items-center justify-between gap-3 px-1 text-sm">
            <span className="text-muted-foreground">{t('debts.form.currency.label')}</span>
            <button
              type="button"
              aria-label={`${t('debts.form.currency.label')}: ${currency ? fullCurrencyLabel(currency) : '—'}`}
              className="min-w-0 truncate text-econumo-link"
              onClick={() => setCurrencyOpen(true)}
            >
              {currency ? fullCurrencyLabel(currency) : '—'}
            </button>
          </div>
          {errors.currencyId ? <p className="px-1 text-sm text-destructive">{errors.currencyId}</p> : null}
        </div>

        {serverError ? (
          <p role="alert" className="text-sm text-destructive">
            {serverError}
          </p>
        ) : null}
      </form>

      <CurrencyPickerDialog
        open={currencyOpen}
        title={t('debts.form.currency.label')}
        value={currencyId}
        onClose={() => setCurrencyOpen(false)}
        onPick={(id) => {
          clearError('currencyId')
          patch({ currencyId: id })
        }}
      />
    </ResponsiveDialog>
  )
}

export function DebtDialog() {
  const params = useUiStore((s) => s.debtModal)
  const close = useUiStore((s) => s.closeDebtModal)

  if (!params) {
    return null
  }
  // a different request is a different form
  return <DebtCreateForm key={`${params.categoryId ?? ''}:${params.currencyId ?? ''}`} params={params} onDone={close} />
}
