import { useCallback, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { createAccount, getAccountList } from '@/api/account'
import { createBudget, getBudgetList } from '@/api/budget'
import { createCategory, getCategoryList } from '@/api/category'
import { getCurrencyList, hideAllCurrencies, showCurrency } from '@/api/currency'
import { completeOnboarding, updateCurrency, updateGettingStarted } from '@/api/user'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { useAccounts } from '@/features/accounts/queries'
import { isOnboardingCompleted, userCurrencyId, useUserData } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { clearDraft, emptyDraft, loadDraft, saveDraft, type AccountRow, type CategoryRow, type Day0Draft } from './draft'

const isBadRequest = (err: unknown) => isAxiosError(err) && err.response?.status === 400

async function ignoreBadRequest(call: () => Promise<void>): Promise<void> {
  try {
    await call()
  } catch (err) {
    if (!isBadRequest(err)) {
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

const sameName = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase()

export function useDay0() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const accountsQuery = useAccounts()
  const accounts = accountsQuery.data
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
        ...emptyDraft(),
        step: 'categories',
        accountIds: owned.map((a) => a.id),
        mainCurrencyId: userCurrencyId(user),
      }
    }
    return null
  }, [userId, user, accounts, version])

  const current = useCallback((): Day0Draft => (userId ? loadDraft(userId) : null) ?? draft ?? emptyDraft(), [userId, draft])

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
        if (!userId) {
          return
        }
        const d = current()
        const selected = new Set([mainCurrencyId, ...extraCurrencyIds, ...rows.map((r) => r.currencyId)])
        d.rows = rows
        d.extraCurrencyIds = extraCurrencyIds
        d.mainCurrencyId = mainCurrencyId
        for (const row of rows) {
          opId(d, `account:${row.key}`)
        }
        persist(d)

        for (const row of rows) {
          if (d.created[row.key]) {
            continue
          }
          try {
            const item = await createAccount({
              id: d.ops[`account:${row.key}`],
              name: row.name,
              currencyId: row.currencyId,
              balance: row.balance === '' ? '0' : row.balance,
              icon: row.icon,
              folderId: null,
            })
            d.created[row.key] = item.item.id
          } catch (err) {
            // A 400 is the operation guard answering a replay whose first response was lost.
            if (!isBadRequest(err)) {
              throw err
            }
            const known = new Set(Object.values(d.created))
            const found = (await getAccountList()).find((a) => a.owner.id === userId && !known.has(a.id) && sameName(a.name, row.name))
            if (!found) {
              throw err
            }
            d.created[row.key] = found.id
          }
          persist(d)
        }

        if (!d.currenciesDone) {
          const currencies = await getCurrencyList()
          const main = currencies.find((c) => c.id === mainCurrencyId)
          if (main) {
            await updateCurrency(main.code)
          }
          await ignoreBadRequest(hideAllCurrencies)
          for (const id of selected) {
            await ignoreBadRequest(() => showCurrency(id))
          }
          d.currenciesDone = true
          persist(d)
        }

        d.accountIds = rows.map((r) => d.created[r.key])
        d.savingsAccountIds = rows.filter((r) => r.savings).map((r) => d.created[r.key])
        d.step = 'categories'
        persist(d)
        void queryClient.invalidateQueries()
        trackEvent(METRICS.ONBOARDING_ACCOUNTS_COMPLETE, { accounts: rows.length, currencies: selected.size })
      }),
    [run, current, persist, userId, queryClient],
  )

  const finish = useCallback(
    (categories: CategoryRow[]) =>
      run(async () => {
        if (!userId) {
          return
        }
        const d = current()
        const currencyId = d.mainCurrencyId ?? userCurrencyId(user)
        if (d.accountIds.length === 0 || !currencyId) {
          throw new Error('Day 0 finish needs accounts and a main currency')
        }
        const checked = categories.filter((c) => c.checked)
        d.categoryRows = categories
        for (const c of checked) {
          opId(d, `category:${c.key}`)
        }
        const budgetId = opId(d, 'budget')
        persist(d)

        for (const c of checked) {
          if (d.createdCategories[c.key]) {
            continue
          }
          try {
            const item = await createCategory({ id: d.ops[`category:${c.key}`], name: c.name, type: c.type, icon: c.icon })
            d.createdCategories[c.key] = item.id
          } catch (err) {
            if (!isBadRequest(err)) {
              throw err
            }
            const known = new Set(Object.values(d.createdCategories))
            const found = (await getCategoryList()).find((x) => !known.has(x.id) && x.type === c.type && sameName(x.name, c.name))
            if (!found) {
              throw err
            }
            d.createdCategories[c.key] = found.id
          }
          persist(d)
        }

        if (!d.budgetDone) {
          try {
            await createBudget({
              id: budgetId,
              name: t('onboarding.budget_name'),
              startDate: '',
              currencyId,
              accountIds: d.accountIds,
              savingsAccountIds: d.savingsAccountIds,
            })
          } catch (err) {
            if (!isBadRequest(err) || !(await getBudgetList()).some((b) => b.id === budgetId)) {
              throw err
            }
          }
          d.budgetDone = true
          persist(d)
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

  return { draft, ready: Boolean(user) && !accountsQuery.isPending, saveAccounts, finish, skip, pending, error }
}
