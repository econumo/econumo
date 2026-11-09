import { useCallback, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { createAccount, getAccountList, getFolderList } from '@/api/account'
import { addAccount, createBudget, getBudget, getBudgetList } from '@/api/budget'
import { createCategory, getCategoryList, sortCategoryList } from '@/api/category'
import { getCurrencyList, hideAllCurrencies, showCurrency } from '@/api/currency'
import { UserOptions } from '@/api/dto/user'
import { updateCurrency } from '@/api/user'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { useAccounts } from '@/features/accounts/queries'
import { userCurrencyId, userOption, useUserData } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { emptyDraft, loadDraft, saveDraft, type AccountRow, type CategoryRow, type Day0Draft } from './draft'

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

const today = () => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

// Accounts saved once the budget exists reach it: an account the flow created
// joins it, and a savings switch the user changed is applied. A switch on an
// account the user already had is applied only once they change it, so a
// guessed flag never overrides a reused budget. Turning savings off on a member
// with plans or comments is refused by the server, and that error is shown.
async function syncBudgetAccounts(d: Day0Draft, rows: AccountRow[], budgetId: Id): Promise<void> {
  const members = new Map(((await getBudget(budgetId, today())).filters.accounts ?? []).map((a) => [a.id, a.isSavings === true]))
  const submitted = new Set(d.accountIds)
  const wasSavings = new Set(d.savingsAccountIds)
  for (const row of rows) {
    const id = d.created[row.key]
    const byFlow = Boolean(d.ops[`account:${row.key}`])
    const member = members.get(id)
    if (member === undefined) {
      if (byFlow) await addAccount(budgetId, id, row.savings)
      continue
    }
    const changed = byFlow || (submitted.has(id) && wasSavings.has(id) !== row.savings)
    if (changed && member !== row.savings) await addAccount(budgetId, id, row.savings)
  }
}

// The step orders each type on its own, so the wanted ids take the slots their type's
// rows hold in the server list; archived and other rows keep their place. Re-sorting an
// already applied order sends nothing, which makes a retry safe.
async function applyCategoryOrder(userId: Id, wanted: Id[]): Promise<void> {
  const own = (await getCategoryList()).filter((c) => c.ownerUserId === userId)
  const typeOf = new Map(own.map((c) => [c.id, c.type]))
  const known = wanted.filter((id) => typeOf.has(id))
  const queue = { expense: known.filter((id) => typeOf.get(id) === 'expense'), income: known.filter((id) => typeOf.get(id) === 'income') }
  const placed = new Set(known)
  const next = own.map((c) => (placed.has(c.id) ? (queue[c.type].shift() ?? c.id) : c.id))
  if (next.every((id, i) => id === own[i].id)) {
    return
  }
  await sortCategoryList(next)
  trackEvent(METRICS.CATEGORY_ORDER_LIST)
}

export function useDay0() {
  const { t } = useTranslation()
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

  const run = useCallback(async <T,>(work: () => Promise<T>): Promise<T> => {
    setPending(true)
    setError(null)
    try {
      return await work()
    } catch (err) {
      setError(err)
      throw err
    } finally {
      setPending(false)
    }
  }, [])

  const saveCategories = useCallback(
    (rows: CategoryRow[], order?: string[]) =>
      run(async () => {
        if (!userId) {
          return
        }
        const d = current()
        const checked = rows.filter((c) => c.checked)
        // a default re-labelled by a language switch was sent under its earlier name
        const sentAs = new Map(d.categoryRows.map((c) => [c.key, c.name]))
        d.categoryRows = rows
        d.categoryOrder = order ?? []
        for (const c of checked) {
          opId(d, `category:${c.key}`)
        }
        persist(d)

        try {
          for (const c of checked) {
            if (d.createdCategories[c.key]) {
              continue
            }
            try {
              const item = await createCategory({ id: d.ops[`category:${c.key}`], name: c.name, type: c.type, icon: c.icon })
              d.createdCategories[c.key] = item.id
            } catch (err) {
              // A 400 is the operation guard answering a replay whose first response was lost.
              if (!isBadRequest(err)) {
                throw err
              }
              const known = new Set(Object.values(d.createdCategories))
              const found = (await getCategoryList()).find(
                (x) => !known.has(x.id) && x.type === c.type && (sameName(x.name, c.name) || sameName(x.name, sentAs.get(c.key) ?? c.name)),
              )
              if (!found) {
                throw err
              }
              d.createdCategories[c.key] = found.id
            }
            persist(d)
          }
          if (order && order.length > 0) {
            await applyCategoryOrder(userId, order.map((k) => d.createdCategories[k] ?? k))
          }
        } finally {
          void queryClient.invalidateQueries({ queryKey: queryKeys.categories })
        }
      }),
    [run, current, persist, userId, queryClient],
  )

  const saveAccounts = useCallback(
    (rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id) =>
      run(async () => {
        if (!userId) {
          return
        }
        const d = current()
        const selected = [...new Set([mainCurrencyId, ...extraCurrencyIds, ...rows.map((r) => r.currencyId)])]
        const sentAs = new Map(d.rows.map((r) => [r.key, r.name]))
        d.rows = rows
        d.extraCurrencyIds = extraCurrencyIds
        d.mainCurrencyId = mainCurrencyId
        for (const row of rows) {
          if (row.accountId) {
            d.created[row.key] = row.accountId
          } else {
            opId(d, `account:${row.key}`)
          }
        }
        persist(d)

        try {
          // The server accepts a blank folder only while the user has none (it then creates "General").
          let folderId: Id | null = null
          for (const row of rows) {
            if (d.created[row.key]) {
              continue
            }
            try {
              folderId ??= (await getFolderList())[0]?.id ?? null
              const item = await createAccount({
                id: d.ops[`account:${row.key}`],
                name: row.name,
                currencyId: row.currencyId,
                balance: row.balance === '' ? '0' : row.balance,
                icon: row.icon,
                folderId,
              })
              folderId = item.item.folderId ?? folderId
              d.created[row.key] = item.item.id
            } catch (err) {
              if (!isBadRequest(err)) {
                throw err
              }
              const known = new Set(Object.values(d.created))
              const found = (await getAccountList()).find(
                (a) => a.owner.id === userId && !known.has(a.id) && (sameName(a.name, row.name) || sameName(a.name, sentAs.get(row.key) ?? row.name)),
              )
              if (!found) {
                throw err
              }
              d.created[row.key] = found.id
            }
            persist(d)
          }

          const selection = JSON.stringify([mainCurrencyId, ...[...selected].sort()])
          if (d.currenciesApplied !== selection) {
            const currencies = await getCurrencyList()
            const main = currencies.find((c) => c.id === mainCurrencyId)
            if (main) {
              await updateCurrency(main.code)
            }
            await ignoreBadRequest(hideAllCurrencies)
            for (const id of selected) {
              await ignoreBadRequest(() => showCurrency(id))
            }
            d.currenciesApplied = selection
            persist(d)
          }
        } finally {
          void queryClient.invalidateQueries()
        }

        const budgetId = d.budgetId ?? (d.budgetDone ? (d.ops.budget ?? null) : null)
        if (budgetId) {
          await syncBudgetAccounts(d, rows, budgetId)
          void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
          void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
          void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
        }

        d.accountIds = rows.map((r) => d.created[r.key])
        d.savingsAccountIds = rows.filter((r) => r.savings).map((r) => d.created[r.key])
        persist(d)
        trackEvent(METRICS.ONBOARDING_ACCOUNTS_COMPLETE, { accounts: rows.filter((r) => !r.accountId).length, currencies: selected.length })
      }),
    [run, current, persist, userId, queryClient],
  )

  const ensureBudget = useCallback(
    () =>
      run(async (): Promise<Id> => {
        if (!userId) {
          throw new Error('Day 0 budget needs a signed-in user')
        }
        const d = current()
        if (d.budgetDone && d.ops.budget) {
          return d.ops.budget
        }
        const budgets = (await getBudgetList()).filter(
          (b) => b.ownerUserId === userId || b.access.some((a) => a.user.id === userId && a.isAccepted === 1),
        )
        // our own create whose response was lost is already in the list
        const ours = d.ops.budget && budgets.some((b) => b.id === d.ops.budget) ? d.ops.budget : null
        if (!ours) {
          const preferred = userOption(user, UserOptions.BUDGET)
          const existing = budgets.find((b) => b.id === preferred) ?? budgets[0]
          if (existing) {
            d.budgetId = existing.id
            persist(d)
            return existing.id
          }
          const currencyId = d.mainCurrencyId ?? userCurrencyId(user)
          const owned = (await getAccountList()).filter((a) => a.owner.id === userId).map((a) => a.id)
          if (owned.length === 0 || !currencyId) {
            throw new Error('Day 0 budget needs accounts and a main currency')
          }
          const budgetId = opId(d, 'budget')
          persist(d)
          try {
            await createBudget({
              id: budgetId,
              name: t('onboarding.budget_name'),
              startDate: '',
              currencyId,
              accountIds: owned,
              savingsAccountIds: d.savingsAccountIds.filter((id) => owned.includes(id)),
            })
          } catch (err) {
            if (!isBadRequest(err) || !(await getBudgetList()).some((b) => b.id === budgetId)) {
              throw err
            }
          }
        }
        d.budgetDone = true
        d.budgetId = d.ops.budget
        persist(d)
        void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
        return d.ops.budget
      }),
    [run, current, persist, userId, user, t, queryClient],
  )

  return { draft, ready: Boolean(user) && !accountsQuery.isPending, saveCategories, saveAccounts, ensureBudget, pending, error }
}
