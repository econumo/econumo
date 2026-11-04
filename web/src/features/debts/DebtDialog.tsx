import { X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { OptionSelect } from '@/components/OptionSelect'
import { CalculatorInput } from '@/components/CalculatorInput'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useFormErrors } from '@/hooks/useFormErrors'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/datetime'
import { isNotEmpty, isValidFormula } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import type { OpenDebtParams } from '@/app/uiStore'
import type { RecurringSchedule } from '@/api/dto/recurring'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { SelectCard } from '@/features/transactions/SelectCard'
import { TransactionDatePicker } from '@/features/transactions/TransactionDatePicker'
import { categoryOptions } from '@/features/transactions/useTransactionForm'
import { useCreateDebt, useUpdateDebt } from './queries'
import { amountOf, buildDebtPayload, SCHEDULE_ORDER, useDebtForm } from './useDebtForm'
import type { UpdateDebtDto } from '@/api/dto/debt'

type Errors = Record<string, string | undefined>

// the fields a server refusal can name; anything else lands in the form's alert
const SERVER_FIELDS = ['name', 'currencyId', 'categoryId', 'payeeId', 'tagId', 'startDate', 'balance', 'payment', 'schedule', 'lastPaymentDate']

const amountInputClass = `${cardFieldControlClass} text-base`

function DebtForm({ params, onDone }: { params: OpenDebtParams; onDone: () => void }) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: currencies } = useCurrencies()
  const createDebt = useCreateDebt({ fromBudget: params.fromBudget })
  const updateDebt = useUpdateDebt()

  const debt = params.debt
  const defaultCurrencyId = userCurrencyId(user)
  const { form, initial, patch, setBalance, setPayment, projection } = useDebtForm(params, categories, defaultCurrencyId)
  const { errors, setErrors, clear: clearError } = useFormErrors<Errors>()
  const [serverError, setServerError] = useState<string | null>(null)
  const [currencyOpen, setCurrencyOpen] = useState(false)

  const currencyId = form.currencyId ?? defaultCurrencyId
  const currency = currencies?.find((c) => c.id === currencyId)
  const ownerId = user?.id

  // an edited debt keeps its own category, payee and tag selectable even when archived
  const categoryRows = categoryOptions(categories, 'expense', ownerId)
  if (form.categoryId && !categoryRows.some((c) => c.id === form.categoryId)) {
    const current = categories.find((c) => c.id === form.categoryId)
    if (current) {
      categoryRows.push(current)
    }
  }
  const payeeRows = payees.filter((p) => p.id === form.payeeId || (p.isArchived === 0 && (!ownerId || p.ownerUserId === ownerId)))
  const tagRows = tags.filter((tag) => tag.id === form.tagId || (tag.isArchived === 0 && (!ownerId || tag.ownerUserId === ownerId)))

  const validate = (): boolean => {
    const next: Errors = {}
    const required = t('common.validation.required_field')
    if (!isNotEmpty(form.name.trim())) {
      next.name = required
    }
    if (!currencyId) {
      next.currencyId = required
    }
    if (!form.categoryId) {
      next.categoryId = required
    }
    for (const field of ['balance', 'payment'] as const) {
      if (!isNotEmpty(form[field])) {
        next[field] = required
      } else if (!isValidFormula(form[field])) {
        next[field] = t('common.validation.invalid_formula')
      } else if (amountOf(form[field]) === null) {
        next[field] = t('common.validation.invalid_number')
      }
    }
    if (!next.balance && (amountOf(form.balance) ?? 0) < 0) {
      next.balance = t('common.validation.invalid_number')
    }
    if (!next.payment && (amountOf(form.payment) ?? 0) <= 0) {
      next.payment = t('common.validation.invalid_number')
    }
    if (!form.lastPaymentDate) {
      next.lastPaymentDate = required
    }
    setErrors(next)
    return Object.keys(next).length === 0
  }

  const submit = async () => {
    setServerError(null)
    if (!validate()) {
      return
    }
    const payload = buildDebtPayload({ ...form, currencyId }, initial, debt)
    try {
      if (debt) {
        await updateDebt.mutateAsync(payload as UpdateDebtDto)
      } else {
        await createDebt.mutateAsync(payload)
      }
      onDone()
    } catch (err) {
      const named: Errors = {}
      for (const field of SERVER_FIELDS) {
        named[field] = apiFieldErrors(err, field)?.[0]
      }
      setErrors(named)
      if (!Object.values(named).some(Boolean)) {
        setServerError(apiErrorMessage(err))
      }
    }
  }

  const pending = createDebt.isPending || updateDebt.isPending
  const title = t('debts.form.header')
  const symbol = currency?.symbol ?? ''
  const neverPaysOff = projection.status === 'never' ? t('debts.form.never_pays_off') : null
  const lastPaymentLabel = t('debts.form.last_payment_date.label')
  const startDateLabel = t('debts.form.start_date.label')

  return (
    <ResponsiveDialog
      open
      caps
      fullScreen
      dismissible={false}
      onOpenChange={(o) => !o && onDone()}
      title={title}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="debt-dialog-form" disabled={pending}>
            {debt ? t('common.button.update.label') : t('common.button.add.label')}
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

        <CardField label={t('debts.form.currency.label')} htmlFor="debt-currency" error={errors.currencyId}>
          <button
            type="button"
            id="debt-currency"
            // the currency never changes after creation
            disabled={!!debt}
            className="w-full truncate text-left text-sm disabled:cursor-not-allowed disabled:opacity-70"
            onClick={() => setCurrencyOpen(true)}
          >
            {currency ? fullCurrencyLabel(currency) : '—'}
          </button>
        </CardField>

        <SelectCard label={t('debts.form.category.label')} error={errors.categoryId}>
          <EntitySelect
            aria-label={t('debts.form.category.label')}
            value={form.categoryId}
            onChange={(id) => {
              clearError('categoryId')
              patch({ categoryId: id })
            }}
            options={categoryRows.map((c) => ({ value: c.id, label: c.name, icon: c.icon || 'pending' }))}
          />
        </SelectCard>

        <SelectCard label={t('debts.form.payee.label')} error={errors.payeeId}>
          <EntitySelect
            aria-label={t('debts.form.payee.label')}
            value={form.payeeId}
            onChange={(id) => {
              clearError('payeeId')
              patch({ payeeId: id })
            }}
            options={payeeRows.map((p) => ({ value: p.id, label: p.name }))}
            clearable
          />
        </SelectCard>

        <SelectCard label={t('debts.form.tag.label')} error={errors.tagId}>
          <EntitySelect
            aria-label={t('debts.form.tag.label')}
            value={form.tagId}
            onChange={(id) => {
              clearError('tagId')
              patch({ tagId: id })
            }}
            options={tagRows.map((tag) => ({ value: tag.id, label: tag.name, icon: tag.icon || 'sell' }))}
            clearable
          />
        </SelectCard>

        <CardField label={startDateLabel} error={errors.startDate}>
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">{t('debts.form.start_date.hint')}</span>
            <span className="flex shrink-0 items-center gap-1">
              <TransactionDatePicker
                ariaLabel={startDateLabel}
                placeholder={t('debts.form.start_date.none')}
                value={form.startDate}
                onChange={(day) => {
                  clearError('startDate')
                  patch({ startDate: formatDate(day) })
                }}
              />
              {form.startDate ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  aria-label={t('debts.form.start_date.clear')}
                  onClick={() => patch({ startDate: '' })}
                >
                  <X className="size-4" />
                </Button>
              ) : null}
            </span>
          </div>
        </CardField>

        <CardField label={t('debts.form.balance.label')} htmlFor="debt-balance" error={errors.balance}>
          <div className="flex items-start gap-2">
            <div className="min-w-0 flex-1">
              <CalculatorInput
                id="debt-balance"
                className={amountInputClass}
                value={form.balance}
                onChange={(balance) => {
                  clearError('balance')
                  setBalance(balance)
                }}
              />
            </div>
            <span className="pt-0.5 text-sm text-muted-foreground">{symbol}</span>
          </div>
        </CardField>

        <CardField label={t('debts.form.payment.label')} htmlFor="debt-payment" error={errors.payment ?? neverPaysOff}>
          <div className="flex items-start gap-2">
            <div className="min-w-0 flex-1">
              <CalculatorInput
                id="debt-payment"
                className={amountInputClass}
                value={form.payment}
                onChange={(payment) => {
                  clearError('payment')
                  setPayment(payment)
                }}
              />
            </div>
            <span className="pt-0.5 text-sm text-muted-foreground">{symbol}</span>
          </div>
        </CardField>

        <CardField label={t('debts.form.schedule.label')} htmlFor="debt-schedule" error={errors.schedule}>
          <div className="[&_button]:h-auto [&_button]:w-full [&_button]:border-0 [&_button]:bg-transparent [&_button]:p-0 [&_button]:text-sm [&_button]:shadow-none [&_button]:focus-visible:ring-0">
            <OptionSelect
              id="debt-schedule"
              aria-label={t('debts.form.schedule.label')}
              title={t('debts.form.schedule.label')}
              className="w-full"
              value={form.schedule}
              onValueChange={(v) => patch({ schedule: v as RecurringSchedule })}
              options={SCHEDULE_ORDER.map((schedule) => ({ value: schedule, label: t(`recurring.schedule.${schedule}`) }))}
            />
          </div>
        </CardField>

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
  return <DebtForm key={params.debt?.id ?? `${params.categoryId ?? ''}:${params.currencyId ?? ''}`} params={params} onDone={close} />
}
