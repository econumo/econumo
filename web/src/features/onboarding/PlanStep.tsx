import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import { getBudgetPlan, setLimit } from '@/api/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories } from '@/features/classifications/queries'
import { useBudgets } from '@/features/budgets/queries'
import { budgetCurrentPeriod, localDate } from '@/features/budgets/period'
import { useCurrencies } from '@/features/currencies/queries'
import { useExchange } from '@/features/currencies/useExchange'
import { useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { add, cmp } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { moneyFormat } from '@/lib/money'
import { decimalInput, parseBalance } from './balance'
import { rowTile } from './CategoriesStep'
import type { StepBodyProps } from './OnboardingPage'
import { StepLoadError } from './StepLoadError'
import { useDay0 } from './useDay0'

export function PlanStep({ onDone, onSkip, onGoTo }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const day0 = useDay0()
  const { data: user } = useUserData()
  const accounts = useAccounts()
  const currencies = useCurrencies()
  const exchange = useExchange()
  const categories = useCategories()
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
      // the cached list may predate the budget (a fresh create, or a stale cache): its meta names the period
      .then(async (id) => {
        await queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
        setBudgetId(id)
      })
      .catch((error: unknown) => setEnsureFailure({ error, needsAccount: !isAxiosError(error) }))
  }, [ensureBudget, queryClient])

  useEffect(() => {
    if (!day0.ready || started.current) return
    started.current = true
    ensure()
  }, [day0.ready, ensure])

  // a reused budget may start its months on another day: its current period is the one to plan
  const budgets = useBudgets()
  const meta = budgets.data?.find((b) => b.id === budgetId)
  const period = meta ? budgetCurrentPeriod(meta) : null
  // get-budget lists expense categories only; the plan carries both kinds,
  // with "" for a category that has no plan this month
  const plan = useQuery({
    queryKey: [...queryKeys.budgetPlan, budgetId ?? 'none', period ?? 'none', 1],
    queryFn: () => getBudgetPlan(budgetId as Id, period as string, 1),
    enabled: budgetId !== null && period !== null,
  })

  const pending = saving || skipping
  // every state keeps Skip reachable, so a step that cannot load never traps the user
  const frame = (body: ReactNode, save?: { onSave: () => void; disabled: boolean }) => (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.plan_step.heading')}</h2>
      {body}
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
        <Button type="button" disabled={pending || !save || save.disabled} onClick={save?.onSave}>
          {t('onboarding.plan_step.save')}
        </Button>
      </div>
    </section>
  )

  if (ensureFailure?.needsAccount) {
    return frame(
      <div className="flex flex-col items-start gap-3">
        <p className="text-sm text-muted-foreground">{t('onboarding.plan_step.needs_account')}</p>
        <Button type="button" variant="outline" onClick={() => onGoTo?.('accounts')}>
          {t('onboarding.plan_step.add_account')}
        </Button>
      </div>,
    )
  }
  if (ensureFailure) return frame(<StepLoadError error={ensureFailure.error} onRetry={ensure} />)
  if (budgets.isError) return frame(<StepLoadError error={budgets.error} onRetry={() => void budgets.refetch()} />)
  // ensure() refetched the list before naming the budget, so a budget still missing from it is an error
  if (budgetId !== null && budgets.data !== undefined && !budgets.isFetching && meta === undefined) {
    return frame(<StepLoadError error={new Error('budget missing from the list')} onRetry={ensure} />)
  }
  if (plan.isError) return frame(<StepLoadError error={plan.error} onRetry={() => void plan.refetch()} />)
  if (categories.isError) return frame(<StepLoadError error={categories.error} onRetry={() => void categories.refetch()} />)
  if (!plan.data || !categories.data || period === null) return frame(null)

  // a category is its own element id; one inside an envelope is planned with the envelope
  const planned = new Map(plan.data.structure.elements.map((e) => [e.id, e.cells[0]?.planned ?? '']))
  const inEnvelopes = new Set(plan.data.structure.elements.flatMap((e) => e.children.map((c) => c.id)))
  const own = categories.data.filter((c) => c.ownerUserId === user?.id && c.isArchived === 0 && !inEnvelopes.has(c.id))
  const income = own.filter((c) => c.type === 'income')
  const expenses = own.filter((c) => c.type === 'expense')
  const rows = [...income, ...expenses]

  const stored = (c: CategoryDto) => planned.get(c.id) ?? ''
  const valueOf = (c: CategoryDto) => edits[c.id] ?? decimalInput(stored(c), i18n.language)
  const parsed = (c: CategoryDto): string | null => {
    const raw = valueOf(c).trim()
    if (raw === '') return ''
    // a plan is an amount to spend or to earn, never below zero
    return raw.startsWith('-') ? null : parseBalance(raw, i18n.language)
  }
  const invalid = new Set(rows.filter((c) => parsed(c) === null).map((c) => c.id))
  const sum = (list: CategoryDto[]) => list.reduce((acc, c) => add(acc, parsed(c) || '0'), '0')

  const budgetCurrencyId = plan.data.meta.currencyId
  const currency = (currencies.data ?? []).find((c) => c.id === budgetCurrencyId)
  const plannedTotal = sum(expenses)
  const expected = sum(income)
  const hasIncome = cmp(expected, '0') > 0
  const inAccounts = (accounts.data ?? [])
    .filter((a) => a.owner.id === user?.id)
    .reduce((acc, a) => add(acc, exchange(a.currency.id, budgetCurrencyId, a.balance)), '0')
  const month = new Intl.DateTimeFormat(i18n.language, { month: 'long' }).format(localDate(period))

  const save = async () => {
    if (invalid.size > 0 || budgetId === null) return
    setSaving(true)
    setSaveError(null)
    try {
      let written = 0
      for (const c of rows) {
        const amount = parsed(c)
        // a blank is never written; a typed zero is a plan of its own unless it is the stored one
        if (!amount || (stored(c) !== '' && cmp(amount, stored(c)) === 0)) continue
        // set-limit adds the element row a category created after the budget lacks
        await setLimit({ budgetId, elementId: c.id, period, amount })
        written++
      }
      if (written > 0) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
        void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
      }
      await onDone()
      trackEvent(METRICS.ONBOARDING_PLAN_COMPLETE, { rows: written })
    } catch (err) {
      setSaveError(err)
    } finally {
      setSaving(false)
    }
  }

  const group = (list: CategoryDto[], title: string, testId: string) =>
    list.length === 0 ? null : (
      <section data-testid={testId}>
        <h3 className="mt-4 mb-1.5 text-xs tracking-wider text-muted-foreground uppercase">{title}</h3>
        <ul className="divide-y divide-border/60 overflow-hidden rounded-xl border bg-card">
          {list.map((c) => (
            <li key={c.id} className="flex items-center gap-3 px-3 py-2">
              <span className={rowTile}>
                <EntityIcon name={c.icon} className="text-lg" />
              </span>
              <span className="min-w-0 flex-1 truncate px-1">{c.name}</span>
              <input
                className="w-24 shrink-0 rounded-lg border bg-transparent px-2 py-1.5 text-right text-sm tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-ring/50 aria-invalid:border-destructive sm:w-32"
                aria-label={c.name}
                inputMode="decimal"
                placeholder="0"
                value={valueOf(c)}
                aria-invalid={invalid.has(c.id)}
                onChange={(ev) => setEdits((prev) => ({ ...prev, [c.id]: ev.target.value }))}
              />
            </li>
          ))}
        </ul>
      </section>
    )

  return frame(
    <>
      <p className="text-sm text-muted-foreground">{t('onboarding.plan_step.subtext')}</p>

      {rows.length === 0 && (
        <div className="mt-4 flex flex-col items-start gap-3 rounded-xl border bg-card p-4">
          <p className="text-sm text-muted-foreground">{t('onboarding.plan_step.no_categories')}</p>
          <Button type="button" variant="outline" onClick={() => onGoTo?.('categories')}>
            {t('onboarding.plan_step.add_categories')}
          </Button>
        </div>
      )}
      {group(income, t('onboarding.plan_step.income', { month }), 'group-income')}
      {group(expenses, t('onboarding.plan_step.expenses', { month }), 'group-expense')}

      <p className="mt-3 text-sm text-muted-foreground">
        {t(hasIncome ? 'onboarding.plan_step.planned_of_income' : 'onboarding.plan_step.planned_of_accounts', {
          planned: moneyFormat(plannedTotal, currency),
          total: moneyFormat(hasIncome ? expected : inAccounts, currency),
        })}
      </p>
      {invalid.size > 0 && <p className="text-sm text-destructive">{t('onboarding.plan_step.amount_invalid')}</p>}
      {saveError != null && (
        <p role="alert" className="text-sm text-destructive">
          {apiErrorMessage(saveError)}
        </p>
      )}
    </>,
    { onSave: () => void save(), disabled: invalid.size > 0 || rows.length === 0 },
  )
}
