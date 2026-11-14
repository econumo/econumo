import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { OfflineNotice } from '@/components/OfflineNotice'
import { useFormErrors } from '@/hooks/useFormErrors'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { useUiStore } from '@/app/uiStore'
import type { OpenDebtParams } from '@/app/uiStore'
import type { DebtDto } from '@/api/dto/debt'
import { useQuery } from '@tanstack/react-query'
import { budgetQueryOptions, useBudgets } from '@/features/budgets/queries'
import { useBudgetPeriodStore } from '@/features/budgets/budgetStore'
import { useCategories } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useCreateDebt } from './queries'
import { PaymentField, StartDateField } from './debtFields'
import type { FieldErrors } from './debtFields'
import { amountEcho, amountError, buildCreatePayload, useDebtCreateForm } from './useDebtForm'

// the fields a server refusal can name; anything else lands in the form's alert
const SERVER_FIELDS = ['elementId', 'startDate', 'payment', 'schedule']

/** A new debt on a budget element: payment and schedule, nothing else. The
    element's own name, icon and currency are the debt's; the terms come
    later, with Finish setup on the debt's own screen. The start date is asked
    only when the server says the loan cannot start before the end of the
    element's previous debt. */
function DebtCreateForm({ params, onDone }: { params: OpenDebtParams; onDone: () => void }) {
  const { t } = useTranslation()
  const online = useOnline()
  const { data: categories = [] } = useCategories()
  const { data: currencies } = useCurrencies()
  const { data: budgets } = useBudgets()
  // the row's own budget, not the default one: an envelope is read from its structure
  const selectedDate = useBudgetPeriodStore((st) => st.selectedDate)
  const { data: openBudget } = useQuery(budgetQueryOptions(params.budgetId, selectedDate))
  const createDebt = useCreateDebt()
  const openDebtDetails = useUiStore((s) => s.openDebtDetails)

  const { form, patch } = useDebtCreateForm()
  const { errors, setErrors, clear: clearError } = useFormErrors<FieldErrors>()
  const [serverError, setServerError] = useState<string | null>(null)
  const [askStartDate, setAskStartDate] = useState(false)

  const budget = budgets?.find((b) => b.id === params.budgetId)
  // a category by its id, an envelope from the budget's structure
  const element = categories.find((c) => c.id === params.elementId) ?? openBudget?.structure.elements.find((e) => e.id === params.elementId)
  // the element's currency, else the budget's
  const currencyId = (element && 'currencyId' in element ? element.currencyId : null) ?? budget?.currencyId ?? null
  const currency = currencies?.find((c) => c.id === currencyId)

  const validate = (): boolean => {
    const next: FieldErrors = { payment: amountError(form.payment, t, true) }
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
      saved = await createDebt.mutateAsync(buildCreatePayload(form, params))
    } catch (err) {
      const named: FieldErrors = {}
      for (const field of SERVER_FIELDS) {
        named[field] = apiFieldErrors(err, field)?.[0]
      }
      setErrors(named)
      // the loan cannot start before the previous debt on the element ended
      // (debt.start_before_previous_end): a start date resolves it
      if (named.startDate) {
        setAskStartDate(true)
      }
      if (!Object.values(named).some(Boolean)) {
        setServerError(apiErrorMessage(err))
      } else if (named.elementId) {
        // the element has no field to show its refusal under
        setServerError(named.elementId)
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
      title={element?.name ?? t('debts.form.header')}
      titleIcon={element ? <EntityIcon name={element.icon} className="text-xl text-muted-foreground" /> : undefined}
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

        {serverError ? (
          <p role="alert" className="text-sm text-destructive">
            {serverError}
          </p>
        ) : null}
      </form>
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
  return <DebtCreateForm key={`${params.budgetId}:${params.elementId}`} params={params} onDone={close} />
}
