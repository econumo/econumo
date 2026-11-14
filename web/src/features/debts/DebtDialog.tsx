import { useEffect, useRef, useState } from 'react'
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
import { budgetPlanQueryOptions, budgetQueryOptions, useBudgets } from '@/features/budgets/queries'
import { budgetCurrentPeriod } from '@/features/budgets/period'
import { addMonths } from '@/features/budgets/planMath'
import { useBudgetPeriodStore } from '@/features/budgets/budgetStore'
import { useCategories } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useCreateDebt } from './queries'
import { DatesRow, PaymentField, StartDateField } from './debtFields'
import type { FieldErrors } from './debtFields'
import { suggestedPayment } from './suggestedPayment'
import { amountEcho, buildCreatePayload, isNoFixed, paymentError, returnByErrors, useDebtCreateForm } from './useDebtForm'

// the fields a server refusal can name; anything else lands in the form's alert
const SERVER_FIELDS = ['elementId', 'startDate', 'payment', 'schedule', 'amountBorrowed', 'lastPaymentDate']

/** A new debt on a budget element: payment and schedule, nothing else. The
    element's own name, icon and currency are the debt's; the terms come
    later, with Finish setup on the debt's own screen. The start date is asked
    only when the server says the loan cannot start before the end of the
    element's previous debt. Return by a date asks instead for the amount, the
    day borrowed and the return-by day, and creates the debt set up. */
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
  // the budget's own row first: it carries the row's currency; the category
  // list only names a category while the budget loads
  const row = openBudget?.structure.elements.find((e) => e.id === params.elementId)
  const category = categories.find((c) => c.id === params.elementId)
  const element = row ?? category
  // a row with no currency of its own is in the budget's
  const currencyId = row?.currencyId ?? budget?.currencyId ?? null
  const currency = currencies?.find((c) => c.id === currencyId)

  // the payment starts at this period's plan or spending, else the recent
  // average: one plan window, the current period and the three before it
  const meta = budget ?? openBudget?.meta
  const current = meta ? budgetCurrentPeriod(meta) : null
  const { data: plan } = useQuery({
    ...budgetPlanQueryOptions(params.budgetId, current ? addMonths(current, -3) : '', 4),
    enabled: current !== null,
  })
  const typed = useRef(false)
  const prefilled = useRef(false)
  const fractionDigits = currency?.fractionDigits
  useEffect(() => {
    if (prefilled.current || typed.current || !plan || !current || fractionDigits === undefined) {
      return
    }
    const cells = plan.structure.elements.find((e) => e.id === params.elementId)?.cells
    const at = plan.months.indexOf(current)
    if (!cells || at < 0) {
      return
    }
    prefilled.current = true
    const value = suggestedPayment({
      planned: cells[at]?.planned,
      actual: cells[at]?.actual,
      previous: cells.slice(Math.max(0, at - 3), at).map((c) => c.actual),
      fractionDigits,
    })
    if (value) {
      patch({ payment: value })
    }
  })

  const returnBy = isNoFixed(form.schedule)
  const validate = (): boolean => {
    const next: FieldErrors = returnBy ? returnByErrors(form, t) : { payment: paymentError(form.payment, form.schedule, t) }
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
      // (debt.start_before_previous_end): a start date resolves it; Return by
      // a date already shows its own
      if (named.startDate && !returnBy) {
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

        {returnBy ? (
          <PaymentField
            id="debt-amount"
            label={t('debts.form.amount.label')}
            value={form.amountBorrowed}
            symbol={currency?.symbol ?? ''}
            schedule={form.schedule}
            error={errors.amountBorrowed ?? errors.schedule}
            echo={amountEcho(form.amountBorrowed, currency)}
            onChange={(amountBorrowed) => {
              clearError('amountBorrowed')
              patch({ amountBorrowed })
            }}
            onScheduleChange={(schedule) => {
              clearError('schedule')
              patch({ schedule })
            }}
          />
        ) : (
          <PaymentField
            id="debt-payment"
            value={form.payment}
            symbol={currency?.symbol ?? ''}
            schedule={form.schedule}
            error={errors.payment ?? errors.schedule}
            echo={amountEcho(form.payment, currency)}
            onChange={(payment) => {
              clearError('payment')
              typed.current = true
              patch({ payment })
            }}
            onScheduleChange={(schedule) => {
              clearError('schedule')
              patch({ schedule })
            }}
          />
        )}

        {returnBy ? (
          <DatesRow
            returnBy
            startLabel={t('debts.form.borrowed_on.label')}
            startDate={form.borrowedOn}
            lastPaymentDate={form.returnBy}
            startError={errors.startDate}
            lastPaymentError={errors.lastPaymentDate}
            onStartDateChange={(borrowedOn) => {
              clearError('startDate')
              clearError('lastPaymentDate')
              patch({ borrowedOn })
            }}
            onLastPaymentDateChange={(day) => {
              clearError('lastPaymentDate')
              patch({ returnBy: day })
            }}
          />
        ) : askStartDate ? (
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
