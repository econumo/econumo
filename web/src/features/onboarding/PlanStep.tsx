import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import { getBudget, setLimit } from '@/api/budget'
import { BudgetElementType, type BudgetElementDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useExchange } from '@/features/currencies/useExchange'
import { useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { add, cmp } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { moneyFormat } from '@/lib/money'
import { parseBalance } from './balance'
import { rowTile } from './CategoriesStep'
import type { StepBodyProps } from './OnboardingPage'
import { StepLoadError } from './StepLoadError'
import { useDay0 } from './useDay0'

function firstOfThisMonth(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
}

const byPosition = (a: BudgetElementDto, b: BudgetElementDto) => a.position - b.position

export function PlanStep({ onDone, onSkip, onGoTo }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const day0 = useDay0()
  const { data: user } = useUserData()
  const accounts = useAccounts()
  const currencies = useCurrencies()
  const exchange = useExchange()
  const [period] = useState(firstOfThisMonth)
  const [budgetId, setBudgetId] = useState<Id | null>(null)
  // a non-HTTP refusal: no own account or no main currency to build the budget from
  const [ensureFailure, setEnsureFailure] = useState<{ error: unknown; needsAccount: boolean } | null>(null)
  const [edits, setEdits] = useState<Record<Id, string>>({})
  const [saving, setSaving] = useState(false)
  const [skipping, setSkipping] = useState(false)
  const [saveError, setSaveError] = useState<unknown>(null)
  const started = useRef(false)
  const { ensureBudget } = day0

  const ensure = useCallback(() => {
    setEnsureFailure(null)
    ensureBudget()
      .then(setBudgetId)
      .catch((error: unknown) => setEnsureFailure({ error, needsAccount: !isAxiosError(error) }))
  }, [ensureBudget])

  useEffect(() => {
    if (!day0.ready || started.current) return
    started.current = true
    ensure()
  }, [day0.ready, ensure])

  const budget = useQuery({
    queryKey: [...queryKeys.budget, budgetId ?? 'none', period],
    queryFn: () => getBudget(budgetId as Id, period),
    enabled: budgetId !== null,
  })

  if (ensureFailure?.needsAccount) {
    return (
      <section className="flex flex-col items-start gap-3">
        <h2 className="text-2xl font-normal">{t('onboarding.plan_step.heading')}</h2>
        <p className="text-sm text-muted-foreground">{t('onboarding.plan_step.needs_account')}</p>
        <Button type="button" onClick={() => onGoTo?.('accounts')}>
          {t('onboarding.plan_step.add_account')}
        </Button>
      </section>
    )
  }
  if (ensureFailure) return <StepLoadError error={ensureFailure.error} onRetry={ensure} />
  if (budget.isError) return <StepLoadError error={budget.error} onRetry={() => void budget.refetch()} />
  if (!budget.data) return null

  const elements = budget.data.structure.elements.filter((e) => e.isArchived === 0)
  const income = elements.filter((e) => e.type === BudgetElementType.INCOME_CATEGORY).sort(byPosition)
  const expenses = elements.filter((e) => e.type === BudgetElementType.CATEGORY).sort(byPosition)
  const rows = [...income, ...expenses]

  const prefill = (e: BudgetElementDto) =>
    e.budgeted === '' ? '' : new Intl.NumberFormat(i18n.language, { useGrouping: false, maximumFractionDigits: 8 }).format(Number(e.budgeted))
  const valueOf = (e: BudgetElementDto) => edits[e.id] ?? prefill(e)
  const parsed = (e: BudgetElementDto): string | null => {
    const raw = valueOf(e).trim()
    return raw === '' ? '' : parseBalance(raw, i18n.language)
  }
  const invalid = new Set(rows.filter((e) => parsed(e) === null).map((e) => e.id))
  const sum = (list: BudgetElementDto[]) => list.reduce((acc, e) => add(acc, parsed(e) || '0'), '0')

  const budgetCurrencyId = budget.data.meta.currencyId
  const currency = (currencies.data ?? []).find((c) => c.id === budgetCurrencyId)
  const planned = sum(expenses)
  const expected = sum(income)
  const hasIncome = cmp(expected, '0') > 0
  const inAccounts = (accounts.data ?? [])
    .filter((a) => a.owner.id === user?.id)
    .reduce((acc, a) => add(acc, exchange(a.currency.id, budgetCurrencyId, a.balance)), '0')
  const month = new Intl.DateTimeFormat(i18n.language, { month: 'long' }).format(new Date(`${period}T00:00:00`))

  const save = async () => {
    if (invalid.size > 0 || budgetId === null) return
    setSaving(true)
    setSaveError(null)
    try {
      let written = 0
      for (const e of rows) {
        const amount = parsed(e)
        if (!amount || (e.budgeted !== '' && cmp(amount, e.budgeted) === 0)) continue
        await setLimit({ budgetId, elementId: e.id, period, amount })
        written++
      }
      if (written > 0) void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.ONBOARDING_PLAN_COMPLETE, { planned: written })
      await onDone()
    } catch (err) {
      setSaveError(err)
    } finally {
      setSaving(false)
    }
  }

  const group = (list: BudgetElementDto[], title: string, testId: string) =>
    list.length === 0 ? null : (
      <section data-testid={testId}>
        <h3 className="mt-4 mb-1.5 text-xs tracking-wider text-muted-foreground uppercase">{title}</h3>
        <ul className="divide-y divide-border/60 overflow-hidden rounded-xl border bg-card">
          {list.map((e) => (
            <li key={e.id} className="flex items-center gap-3 px-3 py-2">
              <span className={rowTile}>
                <EntityIcon name={e.icon} className="text-lg" />
              </span>
              <span className="min-w-0 flex-1 truncate px-1">{e.name}</span>
              <input
                className="w-24 shrink-0 rounded-lg border bg-transparent px-2 py-1.5 text-right text-sm tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-ring/50 aria-invalid:border-destructive sm:w-32"
                aria-label={e.name}
                inputMode="decimal"
                placeholder="0"
                value={valueOf(e)}
                aria-invalid={invalid.has(e.id)}
                onChange={(ev) => setEdits((prev) => ({ ...prev, [e.id]: ev.target.value }))}
              />
            </li>
          ))}
        </ul>
      </section>
    )

  const pending = saving || skipping
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.plan_step.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.plan_step.subtext')}</p>

      {group(income, t('onboarding.plan_step.income', { month }), 'group-income')}
      {group(expenses, t('onboarding.plan_step.expenses', { month }), 'group-expense')}

      <p className="mt-3 text-sm text-muted-foreground">
        {t(hasIncome ? 'onboarding.plan_step.planned_of_income' : 'onboarding.plan_step.planned_of_accounts', {
          planned: moneyFormat(planned, currency),
          total: moneyFormat(hasIncome ? expected : inAccounts, currency),
        })}
      </p>
      {invalid.size > 0 && <p className="text-sm text-destructive">{t('onboarding.accounts.balance_invalid')}</p>}
      {saveError != null && (
        <p role="alert" className="text-sm text-destructive">
          {apiErrorMessage(saveError)}
        </p>
      )}

      <div className="mt-6 flex items-center justify-between">
        <Button
          type="button"
          variant="ghost"
          disabled={pending}
          onClick={() => {
            setSkipping(true)
            void onSkip().finally(() => setSkipping(false))
          }}
        >
          {t('onboarding.plan_step.skip')}
        </Button>
        <Button type="button" disabled={pending || invalid.size > 0} onClick={() => void save()}>
          {t('onboarding.plan_step.save')}
        </Button>
      </div>
    </section>
  )
}
