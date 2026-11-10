import { hashKey, type QueryClient, type QueryKey } from '@tanstack/react-query'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { applyLimitToBudget, patchPlanCells } from './overlay'

export interface LimitPatch {
  /** The change did not stick (queued or refused): the cell goes back to what it held without it. */
  withdraw: () => void
  /** The change stands: nothing to undo any more. */
  confirm: () => void
}

interface Cell<D, V> {
  read: (data: D) => V | undefined
  write: (data: D, value: V) => D
  same: (a: V, b: V) => boolean
}

// Oldest first, per cached cell, for the optimistic edits still unanswered. A whole
// snapshot taken in onMutate would hold the unconfirmed edits of the mutations
// before it and give them, or the state before them, back to the cache.
type Open = { before: unknown; after: unknown }
const openByClient = new WeakMap<QueryClient, Map<string, Open[]>>()

function patchCell<D, V>(client: QueryClient, queryKey: QueryKey, cellId: string, cell: Cell<D, V>, apply: (data: D) => D): LimitPatch {
  const data = client.getQueryData<D>(queryKey)
  const next = data ? apply(data) : data
  const before = data ? cell.read(data) : undefined
  const after = next ? cell.read(next) : undefined
  if (!next || before === undefined || after === undefined) {
    return { withdraw: () => {}, confirm: () => {} }
  }
  client.setQueryData<D>(queryKey, next)
  const stacks = openByClient.get(client) ?? new Map<string, Open[]>()
  openByClient.set(client, stacks)
  const stackKey = `${hashKey(queryKey)}/${cellId}`
  const stack = stacks.get(stackKey) ?? []
  stacks.set(stackKey, stack)
  const own: Open = { before, after }
  stack.push(own)

  const close = (): number | null => {
    const at = stack.indexOf(own)
    if (at === -1) {
      return null
    }
    stack.splice(at, 1)
    if (stack.length === 0) {
      stacks.delete(stackKey)
    }
    return at
  }
  return {
    confirm: () => { close() },
    withdraw: () => {
      const at = close()
      if (at === null) {
        return
      }
      const later = stack[at]
      if (later) {
        // a later edit holds the cell and stays; undoing it must now skip this one
        later.before = own.before
        return
      }
      client.setQueryData<D>(queryKey, (current) => {
        if (!current) {
          return current
        }
        const value = cell.read(current)
        // anything else there came from a read of the server
        return value !== undefined && cell.same(value, after) ? cell.write(current, own.before as V) : current
      })
    },
  }
}

type BudgetCell = { budgeted: string; isBudgeted: 0 | 1; available?: string; closingBalance?: string }

function budgetCell(elementId: Id): Cell<BudgetDto, BudgetCell> {
  return {
    read: (budget) => {
      const element = budget.structure.elements.find((el) => el.id === elementId)
      if (element) {
        return { budgeted: element.budgeted, isBudgeted: element.isBudgeted } as BudgetCell
      }
      const row = budget.structure.savings?.find((r) => r.id === elementId)
      if (!row) {
        return undefined
      }
      return {
        budgeted: row.budgeted,
        isBudgeted: row.isBudgeted,
        available: row.available,
        ...(row.closingBalance !== undefined ? { closingBalance: row.closingBalance } : {}),
      } as BudgetCell
    },
    write: (budget, value) => ({
      ...budget,
      structure: {
        ...budget.structure,
        elements: budget.structure.elements.map((el) => (el.id === elementId ? { ...el, ...value } : el)),
        ...(budget.structure.savings
          ? { savings: budget.structure.savings.map((row) => (row.id === elementId ? { ...row, ...value } : row)) }
          : {}),
      },
    }),
    same: (a, b) => a.budgeted === b.budgeted,
  }
}

function planCell(elementId: Id, monthIndex: number): Cell<BudgetPlanDto, string> {
  return {
    read: (plan) => {
      const row = plan.structure.elements.find((el) => el.id === elementId) ?? plan.structure.savings?.find((r) => r.id === elementId)
      return row?.cells[monthIndex]?.planned
    },
    write: (plan, planned) => patchPlanCells(plan, elementId, (c, i) => (i === monthIndex ? { ...c, planned } : c)) ?? plan,
    same: (a, b) => a === b,
  }
}

/** Patches one cached budget month's limit until the change is answered. */
export function patchBudgetLimit(client: QueryClient, queryKey: QueryKey, form: { elementId: Id; period: string; amount: string | null }): LimitPatch {
  return patchCell(client, queryKey, form.elementId, budgetCell(form.elementId), (budget: BudgetDto) => applyLimitToBudget(budget, form))
}

/** Patches one cell of a cached plan window until the change is answered. */
export function patchPlanLimit(client: QueryClient, queryKey: QueryKey, form: { elementId: Id; monthIndex: number; planned: string }): LimitPatch {
  const cell = planCell(form.elementId, form.monthIndex)
  return patchCell(client, queryKey, `${form.elementId}/${form.monthIndex}`, cell, (plan: BudgetPlanDto) => cell.write(plan, form.planned))
}
