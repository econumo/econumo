import { useCallback, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import type { CurrentUserDto } from '@/api/dto/user'
import { saveOnboardingAccounts, saveOnboardingCategories, saveOnboardingPlan } from '@/api/onboarding'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { claimedElsewhere, finishImportStep, notUser } from './finishImport'
import { emptyDraft, loadDraft, saveDraft, type AccountRow, type CategoryRow, type Day0Draft, type Day0Step } from './draft'

export type CategoryReview = { archive: Id[]; restore: Id[] }
export type AccountReview = { hide: Id[]; unhide: Id[] }

// The user query is answered by the step itself, so it is never refetched over that answer (notUser).

// Each step is one server call that writes everything and marks the step done.
// The client only keeps the submitted form and the operation id in the draft:
// the id is minted on the first Continue and replayed by every retry until the
// server answers, so a response lost on the way back never writes twice.
export function useDay0() {
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const accountsQuery = useAccounts()
  const [version, setVersion] = useState(0)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const userId = user?.id

  const draft = useMemo<Day0Draft | null>(() => {
    void version
    return userId ? loadDraft(userId) : null
  }, [userId, version])

  const current = useCallback((): Day0Draft => (userId ? loadDraft(userId) : null) ?? emptyDraft(), [userId])

  const persist = useCallback(
    (d: Day0Draft) => {
      if (userId) {
        saveDraft(userId, d)
        setVersion((v) => v + 1)
      }
    },
    [userId],
  )

  // `submit` records the form in the draft; `call` sends it under the step's operation id.
  const runStep = useCallback(
    async <T extends { user: CurrentUserDto }>(
      step: Day0Step,
      submit: (d: Day0Draft) => void,
      call: (id: Id) => Promise<T>,
      answered?: (d: Day0Draft, result: T) => void,
    ): Promise<T | undefined> => {
      if (!userId) {
        return undefined
      }
      setPending(true)
      setError(null)
      try {
        const d = current()
        submit(d)
        const opId = (d.ops[step] ??= uuidv7())
        persist(d)
        let result: T
        try {
          result = await call(opId)
        } catch (err) {
          // the step may have committed with its answer lost: every list it writes is refetched,
          // and the refetched user shows the step done
          void queryClient.invalidateQueries()
          if (claimedElsewhere(err)) {
            // the id was taken by another write, so replaying it would be refused forever
            const d2 = current()
            delete d2.ops[step]
            persist(d2)
          }
          throw err
        }
        queryClient.setQueryData(queryKeys.user, result.user)
        const done = current()
        answered?.(done, result)
        delete done.ops[step]
        persist(done)
        return result
      } catch (err) {
        setError(err)
        throw err
      } finally {
        setPending(false)
      }
    },
    [userId, current, persist, queryClient],
  )

  // A CSV import that brought rows in: the shared helper settles the import step (the dialog
  // itself already called it, so this only retries one that failed there).
  const finishImport = useCallback(async () => {
    setPending(true)
    setError(null)
    try {
      await finishImportStep(queryClient)
    } catch (err) {
      setError(err)
      throw err
    } finally {
      setPending(false)
      setVersion((v) => v + 1)
    }
  }, [queryClient])

  // `review` holds existing categories whose archived state the user flipped.
  const saveCategories = useCallback(
    async (rows: CategoryRow[], order?: string[], offered?: string[], review: CategoryReview = { archive: [], restore: [] }) => {
      let offeredDefaults: string[] = []
      const result = await runStep(
        'categories',
        (d) => {
          d.categoryRows = rows
          d.categoryOrder = order ?? []
          d.offeredDefaults ??= offered ?? []
          offeredDefaults = d.offeredDefaults
        },
        (id) =>
          saveOnboardingCategories({
            id,
            categories: rows.map((r) => ({ key: r.key, name: r.name, type: r.type, icon: r.icon })),
            ...(order && order.length > 0 ? { order } : {}),
            ...(review.archive.length > 0 ? { archive: review.archive } : {}),
            ...(review.restore.length > 0 ? { restore: review.restore } : {}),
          }),
        (d, res) => {
          d.budgetId = res.budgetId || d.budgetId
        },
      )
      if (!result) {
        return
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.categories })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
      if (order && order.length > 0) {
        trackEvent(METRICS.CATEGORY_ORDER_LIST)
      }
      // a default removed and re-added under its own name counts as removed plus custom
      const kept = new Set(rows.flatMap((r) => (r.defaultKey ? [r.defaultKey] : [])))
      for (const category of offeredDefaults.filter((k) => !kept.has(k))) {
        trackEvent(METRICS.ONBOARDING_CATEGORY_REMOVED, { category })
      }
      trackEvent(METRICS.ONBOARDING_CATEGORIES_COMPLETE, {
        renamed: rows.filter((r) => r.defaultKey && !r.labelKey).map((r) => r.defaultKey).join(','),
        custom: rows.filter((r) => !r.defaultKey).length,
        offered: offeredDefaults.length,
        archived: review.archive.length,
        restored: review.restore.length,
      })
    },
    [runStep, queryClient],
  )

  // `rows` lists the user's existing accounts too (with accountId): their currencies stay
  // visible, but only the new rows are created.
  const saveAccounts = useCallback(
    async (rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id, review: AccountReview = { hide: [], unhide: [] }) => {
      const fresh = rows.filter((r) => !r.accountId)
      const result = await runStep(
        'accounts',
        (d) => {
          d.rows = rows
          d.extraCurrencyIds = extraCurrencyIds
          d.mainCurrencyId = mainCurrencyId
        },
        (id) =>
          saveOnboardingAccounts({
            id,
            accounts: fresh.map((r) => ({ key: r.key, name: r.name, balance: r.balance, currencyId: r.currencyId, icon: r.icon })),
            currencyIds: [...new Set([...extraCurrencyIds, ...rows.map((r) => r.currencyId)])],
            mainCurrencyId,
            ...(review.hide.length > 0 ? { hide: review.hide } : {}),
            ...(review.unhide.length > 0 ? { unhide: review.unhide } : {}),
          }),
        (d, res) => {
          d.budgetId = res.budgetId || d.budgetId
        },
      )
      if (!result) {
        return
      }
      // accounts, folders, visible currencies, the profile currency and the budget all moved
      void queryClient.invalidateQueries({ predicate: notUser })
      const selected = new Set([mainCurrencyId, ...extraCurrencyIds, ...rows.map((r) => r.currencyId)])
      trackEvent(METRICS.ONBOARDING_ACCOUNTS_COMPLETE, {
        accounts: fresh.length,
        currencies: selected.size,
        hidden: review.hide.length,
        shown: review.unhide.length,
      })
    },
    [runStep, queryClient],
  )

  // `amounts` are the rows to write: blank and unchanged ones are left out by the caller.
  const savePlan = useCallback(
    async (budgetId: Id, amounts: { categoryId: Id; amount: string }[]) => {
      const result = await runStep(
        'plan',
        () => undefined,
        (id) => saveOnboardingPlan({ id, budgetId, amounts }),
        (d, res) => {
          d.budgetId = res.budgetId
        },
      )
      if (!result) {
        return
      }
      if (amounts.length > 0) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
        void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
      }
      trackEvent(METRICS.ONBOARDING_PLAN_COMPLETE, { rows: amounts.length })
    },
    [runStep, queryClient],
  )

  return { draft, ready: Boolean(user) && !accountsQuery.isPending, finishImport, saveCategories, saveAccounts, savePlan, pending, error }
}
