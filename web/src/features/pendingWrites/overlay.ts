import type { AccountDto } from '@/api/dto/account'
import type { BudgetTransactionsParams } from '@/api/budget'
import type { BudgetDto, BudgetMetaDto, BudgetPlanDto, BudgetTransactionDto, PlanCellDto } from '@/api/dto/budget'
import type { CreateTransactionDto, TransactionDto } from '@/api/dto/transaction'
import type { UserDto } from '@/api/dto/user'
import type { Id } from '@/api/types'
import { add, cmp, sub } from '@/lib/decimal'
import { parseDateTime } from '@/lib/datetime'
import { currentPeriod, periodStartDayOf, periodWindow } from '../budgets/period'
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
      // once read back the server's row stands in for it, under the id later changes still name
      out = [created, ...out.filter((r) => r.id !== op.txId && r.id !== op.landed?.realId)]
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
  const addEffect = (op: PendingOp, tx: Effect, sign: 1 | -1) => {
    // the raw balance already counts it; the op still tells what the ones behind it start from
    if (op.landed || parseDateTime(tx.date).getTime() >= tomorrow.getTime()) {
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
      addEffect(op, op.payload, 1)
      current.set(op.txId, op.payload)
    } else if (op.kind === 'tx-update') {
      const prior = stateOf(op.txId)
      // with no known prior state either half alone would double-count or drop money
      if (prior) {
        addEffect(op, prior, -1)
        addEffect(op, op.payload, 1)
        current.set(op.txId, op.payload)
      }
    } else if (op.kind === 'tx-delete') {
      const prior = current.has(op.txId) ? stateOf(op.txId) : (stateOf(op.txId) ?? op.previous)
      if (prior) {
        addEffect(op, prior, -1)
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

export function applyLimitToPlan(plan: BudgetPlanDto, form: { elementId: Id; period: string; amount: string | null }): BudgetPlanDto {
  const index = plan.months.indexOf(form.period)
  if (index === -1) {
    return plan
  }
  const planned = form.amount ?? ''
  return patchPlanCells(plan, form.elementId, (c, i) => (i === index ? { ...c, planned } : c)) ?? plan
}

export function overlayPlan(plan: BudgetPlanDto, ops: PendingOp[], budgetId: Id): BudgetPlanDto {
  let out = plan
  for (const op of bySeq(ops)) {
    if (op.kind === 'limit' && op.budgetId === budgetId) {
      out = applyLimitToPlan(out, op)
    }
  }
  return out
}

type Named = { id: Id; name: string }

export interface ClassificationLookups {
  categories?: (Named & { icon: string })[]
  payees?: Named[]
  tags?: Named[]
}

// A row shows its classification by name, which an op does not carry. An id the
// list does not hold stays with an empty name: the row falls back on its other text.
function renamed<T extends Named>(id: Id | null, current: T | null, list: T[] | undefined, blank: (id: Id) => T): T | null {
  if (id === null) {
    return null
  }
  return current?.id === id ? current : (list?.find((item) => item.id === id) ?? blank(id))
}

export interface BudgetListing {
  /** Every query the list was read through; a merged list holds a row while any of them would. */
  params: BudgetTransactionsParams[]
  /** The budget of the list; without it the period is not judged. */
  meta?: Pick<BudgetMetaDto, 'periodStartDay' | 'endedAt'>
}

// A period past the budget's end is served as its last one, so its key says nothing.
function inPeriod(date: string, params: BudgetTransactionsParams, meta: BudgetListing['meta']): boolean {
  if (!meta || (meta.endedAt && params.periodStart.slice(0, 7) > meta.endedAt.slice(0, 7))) {
    return true
  }
  const { start, end } = periodWindow(params.periodStart, periodStartDayOf(meta))
  return day(date) >= start && day(date) < end
}

// The server's selectors (internal/budget/txlist.go), in its order. What the client
// cannot judge passes: which accounts are the budget's everyday ones or cross its
// boundary, which categories an envelope holds or have an income row of their own.
function matchesSelector(p: CreateTransactionDto, params: BudgetTransactionsParams): boolean {
  const { accountId, transfers, income, categoryId, tagId, envelopeId, uncategorized, labelId } = params
  if (accountId) {
    return p.accountId === accountId || (p.type === 'transfer' && p.accountRecipientId === accountId)
  }
  if (transfers) {
    return p.type === 'transfer'
  }
  if (income) {
    if (p.type !== 'income') {
      return false
    }
    return categoryId ? p.categoryId === categoryId : envelopeId ? p.categoryId !== null : true
  }
  if (p.type !== 'expense') {
    return false
  }
  if (labelId || tagId) {
    if (labelId ? !p.labelIds.includes(labelId) : p.tagId !== tagId) {
      return false
    }
    return categoryId ? p.categoryId === categoryId : uncategorized ? p.categoryId === null : true
  }
  if (categoryId) {
    return p.categoryId === categoryId && p.tagId === null
  }
  if (uncategorized) {
    return p.categoryId === null && p.tagId === null
  }
  return envelopeId ? p.categoryId !== null && p.tagId === null : true
}

/** Drill-down rows with waiting edits and deletes. A waiting create is absent (it has no row yet), and
 *  so is a row moved into this list by an edit; one an edit moved out of `listing` leaves it. */
export function overlayBudgetTransactions<T extends BudgetTransactionDto>(rows: T[], ops: PendingOp[], lookups: ClassificationLookups = {}, listing?: BudgetListing): T[] {
  let out = rows
  const edited = new Map<Id, CreateTransactionDto>()
  let dateMoved = false
  for (const op of bySeq(ops)) {
    if (op.kind === 'tx-update') {
      const p = op.payload
      out = out.map((r) => {
        if (r.id !== op.txId) {
          return r
        }
        edited.set(r.id, p)
        dateMoved ||= day(r.spentAt) !== day(p.date)
        return {
          ...r,
          // a row carries the amount of its own side
          amount: p.type === 'transfer' && r.direction === 'in' ? (p.amountRecipient ?? p.amount) : p.amount,
          description: p.description,
          spentAt: `${day(p.date)}${r.spentAt.slice(10)}`,
          labelIds: [...p.labelIds],
          category: renamed(p.categoryId, r.category, lookups.categories, (id) => ({ id, name: '', icon: '' })),
          payee: renamed(p.payeeId, r.payee, lookups.payees, (id) => ({ id, name: '' })),
          tag: renamed(p.tagId, r.tag, lookups.tags, (id) => ({ id, name: '' })),
        }
      })
    } else if (op.kind === 'tx-delete') {
      out = out.filter((r) => r.id !== op.txId)
    }
  }
  // Judged on the last change only: an edit behind a sent one may undo its move.
  if (listing && edited.size > 0) {
    out = out.filter((r) => {
      const p = edited.get(r.id)
      return !p || listing.params.some((params) => matchesSelector(p, params) && inPeriod(p.date, params, listing.meta))
    })
  }
  // The server orders by day (its sort keys within one), and the list heads each run of a day once.
  return dateMoved ? [...out].sort((a, b) => day(b.spentAt).localeCompare(day(a.spentAt))) : out
}
