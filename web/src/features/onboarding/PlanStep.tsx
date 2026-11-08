import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
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
import { useBudgets } from '@/features/budgets/queries'
import { budgetCurrentPeriod, localDate } from '@/features/budgets/period'
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

const byPosition = (a: BudgetElementDto, b: BudgetElementDto) => a.position - b.position

export function PlanStep({ onDone, onSkip, onGoTo }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const day0 = useDay0()
  const { data: user } = useUserData()
  const accounts = useAccounts()
  const currencies = useCurrencies()
  const exchange = useExchange()
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
  const budget = useQuery({
    queryKey: [...queryKeys.budget, budgetId ?? 'none', period ?? 'none'],
    queryFn: () => getBudget(budgetId as Id, period as string),
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
  if (budget.isError) return frame(<StepLoadError error={budget.error} onRetry={() => void budget.refetch()} />)
  if (!budget.data || period === null) return frame(null)

  const elements = budget.data.structure.elements.filter((e) => e.isArchived === 0)
  const income = elements.filter((e) => e.type === BudgetElementType.INCOME_CATEGORY).sort(byPosition)
  const expenses = elements.filter((e) => e.type === BudgetElementType.CATEGORY).sort(byPosition)
  const rows = [...income, ...expenses]

  const prefill = (e: BudgetElementDto) =>
    e.budgeted === '' ? '' : new Intl.NumberFormat(i18n.language, { useGrouping: false, maximumFractionDigits: 8 }).format(Number(e.budgeted))
  const valueOf = (e: BudgetElementDto) => edits[e.id] ?? prefill(e)
  const parsed = (e: BudgetElementDto): string | null => {
    const raw = valueOf(e).trim()
    if (raw === '') return ''
    // a plan is an amount to spend or to earn, never below zero
    return raw.startsWith('-') ? null : parseBalance(raw, i18n.language)
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
  const month = new Intl.DateTimeFormat(i18n.language, { month: 'long' }).format(localDate(period))

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
      await onDone()
      trackEvent(METRICS.ONBOARDING_PLAN_COMPLETE, { rows: written })
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
          planned: moneyFormat(planned, currency),
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
