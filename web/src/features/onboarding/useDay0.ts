import { useCallback, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { createAccount } from '@/api/account'
import { createBudget } from '@/api/budget'
import { createCategory } from '@/api/category'
import { getCurrencyList, hideAllCurrencies, showCurrency } from '@/api/currency'
import { completeOnboarding, updateCurrency, updateGettingStarted } from '@/api/user'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { useAccounts } from '@/features/accounts/queries'
import { isOnboardingCompleted, userCurrencyId, useUserData } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { clearDraft, loadDraft, saveDraft, type AccountRow, type CategoryRow, type Day0Draft } from './draft'

async function ignoreClientError(call: () => Promise<void>): Promise<void> {
  try {
    await call()
  } catch (err) {
    const status = isAxiosError(err) ? err.response?.status : undefined
    if (status === undefined || status < 400 || status >= 500) {
      throw err
    }
  }
}

function opId(draft: Day0Draft, key: string): Id {
  if (!draft.ops[key]) {
    draft.ops[key] = uuidv7()
  }
  return draft.ops[key]
}

export function useDay0() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const { data: accounts } = useAccounts()
  const [version, setVersion] = useState(0)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const userId = user?.id

  const draft = useMemo<Day0Draft | null>(() => {
    void version
    if (!userId) {
      return null
    }
    const stored = loadDraft(userId)
    if (stored) {
      return stored
    }
    const owned = (accounts ?? []).filter((a) => a.owner.id === userId)
    if (owned.length > 0 && !isOnboardingCompleted(user)) {
      return {
        step: 'categories',
        ops: {},
        accountIds: owned.map((a) => a.id),
        savingsAccountIds: [],
        mainCurrencyId: userCurrencyId(user),
      }
    }
    return null
  }, [userId, user, accounts, version])

  const current = useCallback((): Day0Draft => {
    return (
      (userId ? loadDraft(userId) : null) ??
      draft ?? { step: 'accounts', ops: {}, accountIds: [], savingsAccountIds: [], mainCurrencyId: null }
    )
  }, [userId, draft])

  const persist = useCallback(
    (d: Day0Draft) => {
      if (userId) {
        saveDraft(userId, d)
        setVersion((v) => v + 1)
      }
    },
    [userId],
  )

  const run = useCallback(async (work: () => Promise<void>) => {
    setPending(true)
    setError(null)
    try {
      await work()
    } catch (err) {
      setError(err)
      throw err
    } finally {
      setPending(false)
    }
  }, [])

  const saveAccounts = useCallback(
    (rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id) =>
      run(async () => {
        const d = current()
        for (const row of rows) {
          opId(d, `account:${row.key}`)
        }
        d.mainCurrencyId = mainCurrencyId
        persist(d)

        const created = new Map<string, Id>()
        for (const row of rows) {
          const item = await createAccount({
            id: d.ops[`account:${row.key}`],
            name: row.name,
            currencyId: row.currencyId,
            balance: row.balance === '' ? '0' : row.balance,
            icon: row.icon,
            folderId: null,
          })
          created.set(row.key, item.item.id)
        }

        const currencies = await getCurrencyList()
        const main = currencies.find((c) => c.id === mainCurrencyId)
        if (main) {
          await updateCurrency(main.code)
        }
        const selected = [...new Set([mainCurrencyId, ...extraCurrencyIds, ...rows.map((r) => r.currencyId)])]
        await ignoreClientError(hideAllCurrencies)
        for (const id of selected) {
          await ignoreClientError(() => showCurrency(id))
        }

        d.accountIds = rows.map((r) => created.get(r.key)!)
        d.savingsAccountIds = rows.filter((r) => r.savings).map((r) => created.get(r.key)!)
        d.step = 'categories'
        persist(d)
        void queryClient.invalidateQueries()
      }),
    [run, current, persist, queryClient],
  )

  const finish = useCallback(
    (categories: CategoryRow[]) =>
      run(async () => {
        if (!userId) {
          return
        }
        const d = current()
        const checked = categories.filter((c) => c.checked)
        for (const c of checked) {
          opId(d, `category:${c.key}`)
        }
        const budgetId = opId(d, 'budget')
        persist(d)

        for (const c of checked) {
          await createCategory({ id: d.ops[`category:${c.key}`], name: c.name, type: c.type, icon: c.icon })
        }
        const currencyId = d.mainCurrencyId ?? userCurrencyId(user)
        if (currencyId && d.accountIds.length > 0) {
          await createBudget({
            id: budgetId,
            name: t('onboarding.budget_name'),
            startDate: '',
            currencyId,
            accountIds: d.accountIds,
            savingsAccountIds: d.savingsAccountIds,
          })
        }
        await updateGettingStarted('')
        const updated = await completeOnboarding()
        clearDraft(userId)
        queryClient.setQueryData(queryKeys.user, updated)
        void queryClient.invalidateQueries()
        trackEvent(METRICS.USER_COMPLETE_ONBOARDING)
      }),
    [run, current, persist, userId, user, t, queryClient],
  )

  const skip = useCallback(
    () =>
      run(async () => {
        if (!userId) {
          return
        }
        const step = current().step
        await updateGettingStarted('')
        const updated = await completeOnboarding()
        clearDraft(userId)
        queryClient.setQueryData(queryKeys.user, updated)
        trackEvent(METRICS.ONBOARDING_SKIP, { step })
      }),
    [run, current, userId, queryClient],
  )

  return { draft, ready: Boolean(user), saveAccounts, finish, skip, pending, error }
}
