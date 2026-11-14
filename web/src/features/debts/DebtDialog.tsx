import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CardField } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { OfflineNotice } from '@/components/OfflineNotice'
import { useFormErrors } from '@/hooks/useFormErrors'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { isNotEmpty } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import type { OpenDebtParams } from '@/app/uiStore'
import type { DebtDto, DebtRuleDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import type { CategoryDto } from '@/api/dto/category'
import { useBudgets } from '@/features/budgets/queries'
import { useCategories } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useCreateDebt, useDebtRuleOptions } from './queries'
import { DebtNameField, PaymentField, StartDateField } from './debtFields'
import type { FieldErrors } from './debtFields'
import { RulesEditor } from './RulesEditor'
import { amountEcho, amountError, buildCreatePayload, useDebtCreateForm } from './useDebtForm'

// model.DefaultIcon: a debt with no icon and no rule category
const DEFAULT_CATEGORY_ICON = 'local_offer'

// the fields a server refusal can name; anything else lands in the form's alert
const SERVER_FIELDS = ['name', 'budgetId', 'currencyId', 'rules', 'startDate', 'payment', 'schedule']

/** the row's own category, named while the budget's options load */
function lockedFallback(categoryId: Id | undefined, categories: CategoryDto[]): DebtRuleDto[] {
  const category = categories.find((c) => c.id === categoryId)
  if (!category) {
    return []
  }
  return [{
    id: '', categoryId: category.id, categoryName: category.name, categoryIcon: category.icon, ownerUserId: category.ownerUserId, ownerName: '',
    payeeId: null, payeeName: '', tagId: null, tagName: '', labelId: null, labelName: '',
  }]
}

/** A new debt: what identifies it, and nothing else. The terms come later,
    with Finish setup on the debt's own screen. It goes into the budget row's
    budget. The currency is not asked: it is the category row's when
    converting one, else the budget's, and can be changed on the debt's
    screen; only a refused one (deleted, or no longer shared) brings up a
    currency row to pick another. */
function DebtCreateForm({ params, onDone }: { params: OpenDebtParams; onDone: () => void }) {
  const { t } = useTranslation()
  const online = useOnline()
  const { data: categories = [] } = useCategories()
  const { data: currencies } = useCurrencies()
  const { data: budgets } = useBudgets()
  const createDebt = useCreateDebt()
  const openDebtDetails = useUiStore((s) => s.openDebtDetails)

  const { form, patch } = useDebtCreateForm(params, categories)
  const { errors, setErrors, clear: clearError } = useFormErrors<FieldErrors>()
  const [serverError, setServerError] = useState<string | null>(null)
  // asked only once another debt turns out to have the same filters and start
  // date: a start date then tells a second loan in the same category apart
  const [askStartDate, setAskStartDate] = useState(false)
  const [askCurrency, setAskCurrency] = useState(false)
  const [currencyOpen, setCurrencyOpen] = useState(false)
  const [nameTouched, setNameTouched] = useState(false)
  const budgetId = params.budgetId
  const budget = budgets?.find((b) => b.id === budgetId)
  const { data: options } = useDebtRuleOptions(budgetId)

  // a partner's category is named only by the budget's options
  useEffect(() => {
    const name = options?.participants.flatMap((p) => p.categories).find((c) => c.id === params.categoryId)?.name
    if (name && !nameTouched && form.name === '') {
      patch({ name })
    }
    // only when the options arrive: typing must not bring the name back
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [options])

  // until one is picked the debt shows its first rule's category icon
  const firstCategoryId = form.rules[0]?.categoryId
  const firstCategoryIcon = options?.participants.flatMap((p) => p.categories).find((c) => c.id === firstCategoryId)?.icon
    || categories.find((c) => c.id === firstCategoryId)?.icon
    || DEFAULT_CATEGORY_ICON

  const currencyId = form.currencyId ?? budget?.currencyId ?? null
  const currency = currencies?.find((c) => c.id === currencyId)

  const validate = (): boolean => {
    const required = t('common.validation.required_field')
    const next: FieldErrors = {
      name: isNotEmpty(form.name.trim()) ? undefined : required,
      rules: form.rules.every((r) => r.categoryId) ? undefined : required,
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
      saved = await createDebt.mutateAsync(buildCreatePayload(form, budgetId))
    } catch (err) {
      const named: FieldErrors = {}
      for (const field of SERVER_FIELDS) {
        named[field] = apiFieldErrors(err, field)?.[0]
      }
      setErrors(named)
      // identical filters and start date as another debt (debt.start_date_taken):
      // a start date tells the loans apart. A partial overlap (debt.key_conflict,
      // on rules) needs other filters, so it only shows its message.
      if (named.startDate) {
        setAskStartDate(true)
      }
      if (named.currencyId) {
        setAskCurrency(true)
      }
      // the budget has no field to show its refusal under
      const shown = Object.entries(named).some(([field, message]) => message && field !== 'budgetId')
      if (!shown) {
        setServerError(named.budgetId ?? apiErrorMessage(err))
      }
      return
    }
    onDone()
    openDebtDetails(saved.id, 'overview')
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
          <Button type="submit" form="debt-dialog-form" disabled={createDebt.isPending || !online}>
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
        <OfflineNotice />
        <p className="text-sm text-muted-foreground">{t('debts.form.intro')}</p>

        <DebtNameField
          id="debt-name"
          name={form.name}
          icon={form.icon || firstCategoryIcon}
          error={errors.name}
          onNameChange={(name) => {
            clearError('name')
            setNameTouched(true)
            patch({ name })
          }}
          onIconChange={(icon) => patch({ icon })}
        />

        <RulesEditor
          value={form.rules}
          onChange={(rules) => {
            clearError('rules')
            patch({ rules })
          }}
          options={options}
          initialRules={lockedFallback(params.categoryId, categories)}
          lockedCategoryId={params.categoryId}
          error={errors.rules}
        />

        <PaymentField
          id="debt-payment"
          value={form.payment}
          symbol={currency?.symbol ?? ''}
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

        {askCurrency ? (
          <CardField label={t('debts.form.currency.label')} error={errors.currencyId}>
            <button
              type="button"
              aria-label={`${t('debts.form.currency.label')}: ${currency ? fullCurrencyLabel(currency) : '—'}`}
              className="min-w-0 truncate text-left text-sm text-econumo-link"
              onClick={() => setCurrencyOpen(true)}
            >
              {currency ? fullCurrencyLabel(currency) : '—'}
            </button>
          </CardField>
        ) : null}

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
  return <DebtCreateForm key={`${params.budgetId}:${params.categoryId ?? ''}:${params.currencyId ?? ''}`} params={params} onDone={close} />
}
