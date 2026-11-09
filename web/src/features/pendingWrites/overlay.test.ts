import { describe, expect, it } from 'vitest'
import type { AccountDto } from '@/api/dto/account'
import type { BudgetDto, BudgetPlanDto, BudgetTransactionDto } from '@/api/dto/budget'
import type { CreateTransactionDto, TransactionDto } from '@/api/dto/transaction'
import type { UserDto } from '@/api/dto/user'
import { compareTransactions } from '../transactions/order'
import {
  applyLimitToBudget,
  overlayAccounts,
  overlayBudget,
  overlayBudgetTransactions,
  overlayPlan,
  overlayTransactions,
  PENDING_SORT_KEY,
} from './overlay'
import type { OpChange, PendingOp } from './types'

const me: UserDto = { id: 'me', name: 'Me', avatar: 'face:fuchsia' }
const other: UserDto = { id: 'u2', name: 'Other', avatar: 'face:blue' }

let seq = 0
function op(change: OpChange, extra: Partial<PendingOp> = {}): PendingOp {
  seq += 1
  return { key: `k${seq}`, seq, attempted: false, status: 'waiting', retries: 0, notBefore: 0, ...change, ...extra } as PendingOp
}

function pay(id: string, over: Partial<CreateTransactionDto> = {}): CreateTransactionDto {
  return {
    id,
    type: 'expense',
    accountId: 'a1',
    accountRecipientId: null,
    amount: '10',
    amountRecipient: null,
    categoryId: null,
    description: 'd',
    payeeId: null,
    tagId: null,
    labelIds: [],
    date: '2026-11-08 10:00:00',
    ...over,
  }
}

function row(id: string, over: Partial<TransactionDto> = {}): TransactionDto {
  return { ...pay(id), author: other, recurringId: 'r1', isImported: 1, sortKey: 'Zz', recipientSortKey: null, ...over }
}

function account(id: string, balance: string): AccountDto {
  return { id, balance } as AccountDto
}

describe('overlayTransactions', () => {
  it('adds a created row on top of its day, authored by me', () => {
    const server = [row('s1')]
    const out = overlayTransactions(server, [op({ kind: 'tx-create', txId: 't1', payload: pay('t1') })], me)
    expect(out.find((r) => r.id === 't1')).toMatchObject({ author: me, recurringId: null, isImported: 0, sortKey: PENDING_SORT_KEY })
    expect([...out].sort((a, b) => compareTransactions(a, b))[0].id).toBe('t1')
  })

  it('updates payload fields and keeps server-owned ones', () => {
    const out = overlayTransactions([row('s1')], [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { description: 'new', amount: '5' }) })], me)
    expect(out[0]).toMatchObject({ description: 'new', amount: '5', author: other, recurringId: 'r1', isImported: 1, sortKey: 'Zz' })
  })

  it('gives a row moved to another day the pending key', () => {
    const out = overlayTransactions([row('s1')], [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { date: '2026-11-07 10:00:00' }) })], me)
    expect(out[0].sortKey).toBe(PENDING_SORT_KEY)
  })

  it('removes a deleted row and applies later ops to a created temp id', () => {
    const ops = [
      op({ kind: 'tx-create', txId: 't1', payload: pay('t1') }),
      op({ kind: 'tx-update', txId: 't1', payload: pay('t1', { description: 'edited' }) }),
      op({ kind: 'tx-delete', txId: 's1', previous: null }),
    ]
    const out = overlayTransactions([row('s1')], ops, me)
    expect(out.map((r) => r.id)).toEqual(['t1'])
    expect(out[0].description).toBe('edited')
    const gone = overlayTransactions([], [...ops.slice(0, 1), op({ kind: 'tx-delete', txId: 't1', previous: null })], me)
    expect(gone).toEqual([])
  })

  it('still overlays a failed op and never mutates its input', () => {
    const server = [row('s1')]
    const snapshot = JSON.stringify(server)
    const ops = [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { description: 'x' }) }, { status: 'failed' })]
    const a = overlayTransactions(server, ops, me)
    const b = overlayTransactions(server, ops, me)
    expect(a).toEqual(b)
    expect(a[0].description).toBe('x')
    expect(JSON.stringify(server)).toBe(snapshot)
  })
})

describe('overlayAccounts', () => {
  const accounts = [account('a1', '100'), account('a2', '50')]

  it('subtracts expenses, adds income, and splits transfers', () => {
    const ops = [
      op({ kind: 'tx-create', txId: 't1', payload: pay('t1', { amount: '10' }) }),
      op({ kind: 'tx-create', txId: 't2', payload: pay('t2', { type: 'income', amount: '3' }) }),
      op({ kind: 'tx-create', txId: 't3', payload: pay('t3', { type: 'transfer', amount: '20', accountRecipientId: 'a2', amountRecipient: '25' }) }),
    ]
    const out = overlayAccounts(accounts, ops, [])
    expect(out.map((a) => a.balance)).toEqual(['73', '75'])
    expect(accounts.map((a) => a.balance)).toEqual(['100', '50'])
  })

  it('reverses the server row on update and delete', () => {
    const server = [row('s1', { amount: '10' }), row('s2', { amount: '4' })]
    const out = overlayAccounts(
      accounts,
      [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { amount: '15' }) }), op({ kind: 'tx-delete', txId: 's2', previous: null })],
      server,
    )
    expect(out[0].balance).toBe('99')
  })

  it('replaces an attempted waiting add rather than reversing a server row', () => {
    const ops = [
      op({ kind: 'tx-create', txId: 't1', payload: pay('t1', { amount: '10' }) }, { attempted: true }),
      op({ kind: 'tx-update', txId: 't1', payload: pay('t1', { amount: '12' }) }),
    ]
    expect(overlayAccounts(accounts, ops, [])[0].balance).toBe('88')
    expect(overlayAccounts(accounts, [...ops, op({ kind: 'tx-delete', txId: 't1', previous: null })], [])[0].balance).toBe('100')
  })

  it('ignores a row on an account missing from the list', () => {
    const out = overlayAccounts(accounts, [op({ kind: 'tx-create', txId: 't1', payload: pay('t1', { accountId: 'gone' }) })], [])
    expect(out.map((a) => a.balance)).toEqual(['100', '50'])
  })
})

function budgetOf(): BudgetDto {
  return {
    meta: { id: 'b1', periodStartDay: 1 },
    structure: {
      elements: [
        { id: 'e1', budgeted: '5', isBudgeted: 1 },
        { id: 'e2', budgeted: '7', isBudgeted: 1 },
      ],
    },
  } as unknown as BudgetDto
}

describe('budget overlays', () => {
  it('changes only the matching budget, element and period', () => {
    const budget = budgetOf()
    const ops = [
      op({ kind: 'limit', budgetId: 'b1', elementId: 'e1', period: '2026-11-01', amount: '9' }),
      op({ kind: 'limit', budgetId: 'b2', elementId: 'e2', period: '2026-11-01', amount: '1' }),
      op({ kind: 'limit', budgetId: 'b1', elementId: 'e2', period: '2026-10-01', amount: '1' }),
    ]
    const out = overlayBudget(budget, ops, 'b1', '2026-11-01')
    expect(out.structure.elements.map((e) => e.budgeted)).toEqual(['9', '7'])
    expect(budget.structure.elements[0].budgeted).toBe('5')
  })

  it('clears a plan on a null amount', () => {
    const out = applyLimitToBudget(budgetOf(), { elementId: 'e1', period: '2026-11-01', amount: null })
    expect(out.structure.elements[0]).toMatchObject({ budgeted: '', isBudgeted: 0 })
  })

  it('patches the plan cell of the matching month', () => {
    const plan = {
      months: ['2026-10-01', '2026-11-01'],
      structure: { elements: [{ id: 'e1', cells: [{ planned: '1' }, { planned: '2' }] }] },
    } as unknown as BudgetPlanDto
    const out = overlayPlan(
      plan,
      [
        op({ kind: 'limit', budgetId: 'b1', elementId: 'e1', period: '2026-11-01', amount: '8' }),
        op({ kind: 'limit', budgetId: 'b2', elementId: 'e1', period: '2026-10-01', amount: '8' }),
      ],
      'b1',
    )
    expect(out.structure.elements[0].cells.map((c) => c.planned)).toEqual(['1', '8'])
    const cleared = overlayPlan(plan, [op({ kind: 'limit', budgetId: 'b1', elementId: 'e1', period: '2026-10-01', amount: null })], 'b1')
    expect(cleared.structure.elements[0].cells.map((c) => c.planned)).toEqual(['', '2'])
    expect(plan.structure.elements[0].cells[1].planned).toBe('2')
  })
})

describe('overlayBudgetTransactions', () => {
  const rows = [
    { id: 'x1', amount: '10', description: 'a', spentAt: '2026-11-08 09:00:00', category: { id: 'c1', name: 'C', icon: '' }, payee: null, tag: null },
    { id: 'x2', amount: '3', description: 'b', spentAt: '2026-11-08 09:00:00', category: null, payee: null, tag: null },
  ] as unknown as BudgetTransactionDto[]

  it('applies updates and deletes by id and ignores creates', () => {
    const out = overlayBudgetTransactions(
      rows,
      [
        op({ kind: 'tx-update', txId: 'x1', payload: pay('x1', { amount: '11', description: 'z', date: '2026-11-07 00:00:00', categoryId: 'c1' }) }),
        op({ kind: 'tx-delete', txId: 'x2', previous: null }),
        op({ kind: 'tx-create', txId: 't1', payload: pay('t1') }),
      ],
    )
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ amount: '11', description: 'z', spentAt: '2026-11-07 09:00:00' })
    expect(out[0].category?.id).toBe('c1')
    expect(rows[0].amount).toBe('10')
  })
})
