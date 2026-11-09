import type { AccountDto } from '@/api/dto/account'
import type { BudgetDto, BudgetPlanDto, BudgetTransactionDto, PlanCellDto } from '@/api/dto/budget'
import type { CreateTransactionDto, TransactionDto } from '@/api/dto/transaction'
import type { UserDto } from '@/api/dto/user'
import type { Id } from '@/api/types'
import { add, cmp, sub } from '@/lib/decimal'
import { parseDateTime } from '@/lib/datetime'
import { currentPeriod, periodStartDayOf } from '../budgets/period'
import type { PendingOp } from './types'

/** Sorts above every stored key (base-62 keys are all below '~'), so a waiting row tops its day. */
export const PENDING_SORT_KEY = '~'

const bySeq = (ops: PendingOp[]) => [...ops].sort((a, b) => a.seq - b.seq)

const day = (date: string) => date.slice(0, 10)

function applyPayload(row: TransactionDto, payload: CreateTransactionDto): TransactionDto {
  const sourceMoved = day(row.date) !== day(payload.date) || row.accountId !== payload.accountId
  const recipientMoved = day(row.date) !== day(payload.date) || row.accountRecipientId !== payload.accountRecipientId
  const next: TransactionDto = { ...row, ...payload, labelIds: [...payload.labelIds] }
  if (sourceMoved) {
    next.sortKey = PENDING_SORT_KEY
  }
  if (payload.type !== 'transfer') {
    next.recipientSortKey = null
  } else if (recipientMoved || row.recipientSortKey == null) {
    next.recipientSortKey = PENDING_SORT_KEY
  }
  return next
}

/** Server rows with the waiting changes applied in queue order. Never mutates its input. */
export function overlayTransactions(rows: TransactionDto[], ops: PendingOp[], me: UserDto): TransactionDto[] {
  let out = rows
  for (const op of bySeq(ops)) {
    if (op.kind === 'tx-create') {
      const created: TransactionDto = {
        ...op.payload,
        labelIds: [...op.payload.labelIds],
        author: me,
        recurringId: null,
        isImported: 0,
        sortKey: PENDING_SORT_KEY,
        recipientSortKey: op.payload.type === 'transfer' ? PENDING_SORT_KEY : null,
      }
      out = [created, ...out.filter((r) => r.id !== op.txId)]
    } else if (op.kind === 'tx-update') {
      out = out.map((r) => (r.id === op.txId ? applyPayload(r, op.payload) : r))
    } else if (op.kind === 'tx-delete') {
      out = out.filter((r) => r.id !== op.txId)
    }
  }
  return out
}

type Effect = Pick<CreateTransactionDto, 'type' | 'accountId' | 'accountRecipientId' | 'amount' | 'amountRecipient' | 'date'>

function effectOf(tx: Effect, sign: 1 | -1): Map<Id, string> {
  const deltas = new Map<Id, string>()
  const put = (id: Id, amount: string) => deltas.set(id, add(deltas.get(id) ?? '0', amount))
  const signed = (amount: string, positive: boolean) => (positive === (sign === 1) ? amount : sub('0', amount))
  if (tx.type === 'income') {
    put(tx.accountId, signed(tx.amount, true))
  } else {
    put(tx.accountId, signed(tx.amount, false))
    if (tx.type === 'transfer' && tx.accountRecipientId) {
      put(tx.accountRecipientId, signed(tx.amountRecipient ?? tx.amount, true))
    }
  }
  return deltas
}

/** Account balances with the waiting transactions' effect applied; `serverRows` are the stored rows
 *  whose effect an update or delete reverses. The server balance counts only rows through the end of
 *  the caller's today, so a row dated after `now`'s day has no effect. Never mutates its input. */
export function overlayAccounts(accounts: AccountDto[], ops: PendingOp[], serverRows: TransactionDto[], now: Date): AccountDto[] {
  const tomorrow = new Date(now)
  tomorrow.setHours(0, 0, 0, 0)
  tomorrow.setDate(tomorrow.getDate() + 1)
  const deltas = new Map<Id, string>()
  const addEffect = (tx: Effect, sign: 1 | -1) => {
    if (parseDateTime(tx.date).getTime() >= tomorrow.getTime()) {
      return
    }
    for (const [id, amount] of effectOf(tx, sign)) {
      deltas.set(id, add(deltas.get(id) ?? '0', amount))
    }
  }
  // the transaction as the earlier ops of the chain left it: absent = not yet seen, null = removed
  const current = new Map<Id, Effect | null>()
  const stateOf = (txId: Id): Effect | null => {
    if (current.has(txId)) {
      return current.get(txId) ?? null
    }
    return serverRows.find((r) => r.id === txId) ?? null
  }
  for (const op of bySeq(ops)) {
    if (op.kind === 'tx-create') {
      addEffect(op.payload, 1)
      current.set(op.txId, op.payload)
    } else if (op.kind === 'tx-update') {
      const prior = stateOf(op.txId)
      // with no known prior state either half alone would double-count or drop money
      if (prior) {
        addEffect(prior, -1)
        addEffect(op.payload, 1)
        current.set(op.txId, op.payload)
      }
    } else if (op.kind === 'tx-delete') {
      const prior = current.has(op.txId) ? stateOf(op.txId) : (stateOf(op.txId) ?? op.previous)
      if (prior) {
        addEffect(prior, -1)
      }
      current.set(op.txId, null)
    }
  }
  if (deltas.size === 0) {
    return accounts
  }
  return accounts.map((a) => {
    const delta = deltas.get(a.id)
    return delta === undefined ? a : { ...a, balance: add(a.balance, delta) }
  })
}

export function applyLimitToBudget(
  budget: BudgetDto,
  form: { elementId: Id; period: string; amount: string | null },
): BudgetDto {
  const budgeted = form.amount ?? ''
  const isBudgeted = form.amount === null ? (0 as const) : (1 as const)
  const savings = budget.structure.savings
  // from the current month on a savings row's closing balance still expects
  // its unmet plan, so the edit moves it by the change in that gap
  const projected = form.period >= currentPeriod(periodStartDayOf(budget.meta))
  const gap = (planned: string, spent: string) => (cmp(planned, spent) > 0 ? sub(planned, spent) : '0')
  return {
    ...budget,
    structure: {
      ...budget.structure,
      elements: budget.structure.elements.map((el) => (el.id === form.elementId ? { ...el, budgeted, isBudgeted } : el)),
      // a savings row carries no carry-over: its available is planned minus saved
      ...(savings
        ? {
            savings: savings.map((row) =>
              row.id === form.elementId
                ? {
                    ...row,
                    budgeted,
                    isBudgeted,
                    available: sub(budgeted, row.spent),
                    ...(projected && row.isArchived === 0 && row.closingBalance !== undefined
                      ? { closingBalance: add(sub(row.closingBalance, gap(row.budgeted, row.spent)), gap(budgeted, row.spent)) }
                      : {}),
                  }
                : row,
            ),
          }
        : {}),
    },
  }
}

export function overlayBudget(budget: BudgetDto, ops: PendingOp[], budgetId: Id, period: string): BudgetDto {
  let out = budget
  for (const op of bySeq(ops)) {
    if (op.kind === 'limit' && op.budgetId === budgetId && op.period === period) {
      out = applyLimitToBudget(out, op)
    }
  }
  return out
}

// A plan row's id is either an element's or, for a savings row, the savings
// account's; the two sets never collide, so one patch covers both arrays.
export function patchPlanCells(
  plan: BudgetPlanDto | null | undefined,
  elementId: Id,
  patch: (cell: PlanCellDto, monthIndex: number) => PlanCellDto,
): BudgetPlanDto | null | undefined {
  if (!plan) {
    return plan
  }
  const patchRow = <T extends { id: Id; cells: PlanCellDto[] }>(row: T): T =>
    row.id === elementId ? { ...row, cells: row.cells.map(patch) } : row
  const { savings } = plan.structure
  return {
    ...plan,
    structure: {
      ...plan.structure,
      elements: plan.structure.elements.map(patchRow),
      ...(savings ? { savings: savings.map(patchRow) } : {}),
    },
  }
}

export function overlayPlan(plan: BudgetPlanDto, ops: PendingOp[], budgetId: Id): BudgetPlanDto {
  let out = plan
  for (const op of bySeq(ops)) {
    if (op.kind !== 'limit' || op.budgetId !== budgetId) {
      continue
    }
    const index = out.months.indexOf(op.period)
    if (index === -1) {
      continue
    }
    const planned = op.amount ?? ''
    out = patchPlanCells(out, op.elementId, (c, i) => (i === index ? { ...c, planned } : c)) ?? out
  }
  return out
}

/** Drill-down rows with waiting edits and deletes; a waiting create is absent (it has no row yet).
 *  Category/payee/tag keep their stored name when the id is unchanged, since a name is not in the op. */
export function overlayBudgetTransactions(rows: BudgetTransactionDto[], ops: PendingOp[]): BudgetTransactionDto[] {
  let out = rows
  for (const op of bySeq(ops)) {
    if (op.kind === 'tx-update') {
      const p = op.payload
      out = out.map((r) =>
        r.id !== op.txId
          ? r
          : {
              ...r,
              ...(r.type === 'transfer' ? {} : { amount: p.amount }),
              description: p.description,
              spentAt: `${day(p.date)}${r.spentAt.slice(10)}`,
              labelIds: [...p.labelIds],
              category: p.categoryId === null ? null : r.category,
              payee: p.payeeId === null ? null : r.payee,
              tag: p.tagId === null ? null : r.tag,
            },
      )
    } else if (op.kind === 'tx-delete') {
      out = out.filter((r) => r.id !== op.txId)
    }
  }
  return out
}
