import { describe, expect, it } from 'vitest'
import type { BudgetTransactionsParams } from '@/api/budget'
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

describe('an add the server was found to hold, read back', () => {
  const landed = op({ kind: 'tx-create', txId: 'temp', payload: pay('temp') }, { landed: { realId: 'real' } })

  it('shows one row, under the temporary id later changes still name', () => {
    expect(overlayTransactions([row('real')], [landed], me).map((t) => t.id)).toEqual(['temp'])
  })

  it('does not count its money again', () => {
    expect(overlayAccounts([account('a1', '90')], [landed], [], new Date())[0].balance).toBe('90')
  })

  it('still lets a change behind it move the balance from its values', () => {
    const edit = op({ kind: 'tx-update', txId: 'temp', payload: pay('temp', { amount: '4' }) }, { key: 'k2', seq: 2 })
    const delta = Number(pay('temp').amount) - 4
    expect(Number(overlayAccounts([account('a1', '90')], [landed, edit], [], new Date())[0].balance)).toBe(90 + delta)
  })
})

describe('an edit or delete the server was found to hold, read back', () => {
  it('leaves the balance alone: it already counts the edit', () => {
    const edit = op({ kind: 'tx-update', txId: 't1', payload: pay('t1', { amount: '4' }) }, { landed: {} })
    expect(overlayAccounts([account('a1', '90')], [edit], [row('t1')], new Date())[0].balance).toBe('90')
  })

  it('leaves the balance alone: it already counts the delete', () => {
    const del = op({ kind: 'tx-delete', txId: 't1', previous: row('t1') }, { landed: {} })
    expect(overlayAccounts([account('a1', '90')], [del], [row('t1')], new Date())[0].balance).toBe('90')
  })
})

const NOW = new Date(2026, 10, 8, 12, 0, 0)
const FUTURE = '2026-11-09 08:00:00'

describe('overlayAccounts', () => {
  const accounts = [account('a1', '100'), account('a2', '50')]

  it('subtracts expenses, adds income, and splits transfers', () => {
    const ops = [
      op({ kind: 'tx-create', txId: 't1', payload: pay('t1', { amount: '10' }) }),
      op({ kind: 'tx-create', txId: 't2', payload: pay('t2', { type: 'income', amount: '3' }) }),
      op({ kind: 'tx-create', txId: 't3', payload: pay('t3', { type: 'transfer', amount: '20', accountRecipientId: 'a2', amountRecipient: '25' }) }),
    ]
    const out = overlayAccounts(accounts, ops, [], NOW)
    expect(out.map((a) => a.balance)).toEqual(['73', '75'])
    expect(accounts.map((a) => a.balance)).toEqual(['100', '50'])
  })

  it('reverses the server row on update and delete', () => {
    const server = [row('s1', { amount: '10' }), row('s2', { amount: '4' })]
    const out = overlayAccounts(
      accounts,
      [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { amount: '15' }) }), op({ kind: 'tx-delete', txId: 's2', previous: null })],
      server,
      NOW,
    )
    expect(out[0].balance).toBe('99')
  })

  it('an update or delete of an already-attempted waiting add replaces its effect', () => {
    const ops = [
      op({ kind: 'tx-create', txId: 't1', payload: pay('t1', { amount: '10' }) }, { attempted: true }),
      op({ kind: 'tx-update', txId: 't1', payload: pay('t1', { amount: '12' }) }),
    ]
    expect(overlayAccounts(accounts, ops, [], NOW)[0].balance).toBe('88')
    expect(overlayAccounts(accounts, [...ops, op({ kind: 'tx-delete', txId: 't1', previous: null })], [], NOW)[0].balance).toBe('100')
  })

  it('ignores a row on an account missing from the list', () => {
    const out = overlayAccounts(accounts, [op({ kind: 'tx-create', txId: 't1', payload: pay('t1', { accountId: 'gone' }) })], [], NOW)
    expect(out.map((a) => a.balance)).toEqual(['100', '50'])
  })

  const bal = (out: AccountDto[]) => out.map((a) => a.balance)

  it('ignores a future-dated add and a deleted future server row', () => {
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-create', txId: 't1', payload: pay('t1', { date: FUTURE }) })], [], NOW))).toEqual(['100', '50'])
    const server = [row('s1', { date: FUTURE })]
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-delete', txId: 's1', previous: null })], server, NOW))).toEqual(['100', '50'])
  })

  it('handles an edit crossing the end of today in both directions', () => {
    const past = row('s1', { amount: '10' })
    // past -> future: the stored effect is reversed, the new one not applied
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { date: FUTURE }) })], [past], NOW))[0]).toBe('110')
    // future -> past: nothing to reverse, the new one applies
    const future = row('s2', { amount: '10', date: FUTURE })
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-update', txId: 's2', payload: pay('s2') })], [future], NOW))[0]).toBe('90')
  })

  it('falls back to the delete previous and skips an update with no known row', () => {
    const previous = row('s1', { amount: '10' })
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-delete', txId: 's1', previous })], [], NOW))[0]).toBe('110')
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { amount: '15' }) })], [], NOW))).toEqual(['100', '50'])
  })

  it('moves an edit between accounts', () => {
    const out = overlayAccounts(accounts, [op({ kind: 'tx-update', txId: 's1', payload: pay('s1', { accountId: 'a2', amount: '10' }) })], [row('s1')], NOW)
    expect(bal(out)).toEqual(['110', '40'])
  })

  it('handles type changes between expense, income and transfer', () => {
    const upd = (payload: CreateTransactionDto, server: TransactionDto) => bal(overlayAccounts(accounts, [op({ kind: 'tx-update', txId: 's1', payload })], [server], NOW))
    expect(upd(pay('s1', { type: 'income', amount: '10' }), row('s1'))).toEqual(['120', '50'])
    expect(upd(pay('s1', { type: 'transfer', amount: '10', accountRecipientId: 'a2', amountRecipient: '12' }), row('s1'))).toEqual(['100', '62'])
    const stored = row('s1', { type: 'transfer', amount: '20', accountRecipientId: 'a2', amountRecipient: '25' })
    expect(upd(pay('s1', { amount: '10' }), stored)).toEqual(['110', '25'])
  })

  it('reverses a stored transfer on update and on delete, on both accounts', () => {
    const stored = row('s1', { type: 'transfer', amount: '20', accountRecipientId: 'a2', amountRecipient: '25' })
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-delete', txId: 's1', previous: null })], [stored], NOW))).toEqual(['120', '25'])
    const updated = pay('s1', { type: 'transfer', amount: '5', accountRecipientId: 'a2', amountRecipient: '6' })
    expect(bal(overlayAccounts(accounts, [op({ kind: 'tx-update', txId: 's1', payload: updated })], [stored], NOW))).toEqual(['115', '31'])
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

  const row = rows[0]
  const lookups = {
    categories: [{ id: 'c2', name: 'Rent', icon: 'home' }],
    payees: [{ id: 'p2', name: 'Landlord' }],
    tags: [{ id: 't2', name: 'Flat' }],
  }
  const edit = (over: Partial<CreateTransactionDto>) => op({ kind: 'tx-update', txId: 'x1', payload: pay('x1', over) })

  it('names a category, payee and tag changed to another id from the reference lists', () => {
    const [out] = overlayBudgetTransactions([row], [edit({ categoryId: 'c2', payeeId: 'p2', tagId: 't2' })], lookups)
    expect(out.category).toEqual({ id: 'c2', name: 'Rent', icon: 'home' })
    expect(out.payee).toEqual({ id: 'p2', name: 'Landlord' })
    expect(out.tag).toEqual({ id: 't2', name: 'Flat' })
  })

  it('keeps the stored object when the id did not change, and clears it when the edit does', () => {
    const [same] = overlayBudgetTransactions([row], [edit({ categoryId: 'c1' })], { categories: [] })
    expect(same.category).toBe(row.category)
    const [cleared] = overlayBudgetTransactions([row], [edit({ categoryId: null })], lookups)
    expect(cleared.category).toBeNull()
  })

  it('keeps the new id with an empty name when the list does not know it or was never loaded', () => {
    const [out] = overlayBudgetTransactions([row], [edit({ categoryId: 'c9', payeeId: 'p9', tagId: 't9' })], lookups)
    expect(out.category).toEqual({ id: 'c9', name: '', icon: '' })
    expect(out.payee).toEqual({ id: 'p9', name: '' })
    expect(out.tag).toEqual({ id: 't9', name: '' })
    const [none] = overlayBudgetTransactions([row], [edit({ categoryId: 'c2' })])
    expect(none.category).toEqual({ id: 'c2', name: '', icon: '' })
  })

  describe('a waiting transfer', () => {
    const transfer = (direction: 'in' | 'out') => ({
      id: 'x1', amount: '10', direction, type: 'transfer', description: '', spentAt: '2026-11-08 09:00:00', category: null, payee: null, tag: null,
    }) as unknown as BudgetTransactionDto
    const edited = op({ kind: 'tx-update', txId: 'x1', payload: pay('x1', { type: 'transfer', amount: '20', amountRecipient: '18', accountRecipientId: 'a2' }) })

    it('shows the recipient amount on the incoming side and the amount on the outgoing side', () => {
      expect(overlayBudgetTransactions([transfer('in')], [edited])[0].amount).toBe('18')
      expect(overlayBudgetTransactions([transfer('out')], [edited])[0].amount).toBe('20')
    })

    it('a row without a type is judged by the edit', () => {
      const bare = { ...transfer('in'), type: undefined } as BudgetTransactionDto
      expect(overlayBudgetTransactions([bare], [edited])[0].amount).toBe('18')
    })

    it('money arriving as income is the plain amount', () => {
      const income = { ...transfer('in'), type: 'income' } as BudgetTransactionDto
      const out = overlayBudgetTransactions([income], [op({ kind: 'tx-update', txId: 'x1', payload: pay('x1', { type: 'income', amount: '20' }) })])
      expect(out[0].amount).toBe('20')
    })
  })

  describe('a row an edit takes out of the listing', () => {
    const period = { budgetId: 'b1', periodStart: '2026-11-01' }
    const meta = { periodStartDay: 1, endedAt: '' }
    const tagged = { ...row, category: { id: 'c1', name: 'C', icon: '' }, tag: { id: 't1', name: 'T' } } as BudgetTransactionDto
    const list = (r: BudgetTransactionDto, over: Partial<CreateTransactionDto>, params: BudgetTransactionsParams, m: typeof meta = meta) =>
      overlayBudgetTransactions([r], [edit({ date: '2026-11-08 00:00:00', categoryId: 'c1', tagId: null, ...over })], {}, { params: [params], meta: m })
    const expense = { type: 'expense' as const }

    it('drops a date moved outside the period, and keeps one still inside', () => {
      const params = { ...period, categoryId: 'c1' }
      expect(list(row, { ...expense, date: '2026-12-01 00:00:00' }, params)).toHaveLength(0)
      expect(list(row, { ...expense, date: '2026-10-31 00:00:00' }, params)).toHaveLength(0)
      expect(list(row, { ...expense, date: '2026-11-30 00:00:00' }, params)).toHaveLength(1)
    })

    it('reads the period on the budget\'s own start day', () => {
      const params = { ...period, categoryId: 'c1' }
      const day5 = { periodStartDay: 5, endedAt: '' }
      expect(list(row, { ...expense, date: '2026-12-04 00:00:00' }, params, day5)).toHaveLength(1)
      expect(list(row, { ...expense, date: '2026-12-05 00:00:00' }, params, day5)).toHaveLength(0)
      expect(list(row, { ...expense, date: '2026-11-04 00:00:00' }, params, day5)).toHaveLength(0)
    })

    it('keeps the row when the period cannot be told', () => {
      const params = { ...period, categoryId: 'c1' }
      expect(overlayBudgetTransactions([row], [edit({ ...expense, date: '2027-03-01 00:00:00', categoryId: 'c1' })], {}, { params: [params] })).toHaveLength(1)
      // a period after the budget's end is served as its last one
      const ended = { periodStartDay: 1, endedAt: '2026-09-01 00:00:00' }
      expect(list(row, { ...expense, date: '2026-09-10 00:00:00' }, params, ended)).toHaveLength(1)
    })

    it('categoryId alone lists that category\'s untagged expenses', () => {
      const params = { ...period, categoryId: 'c1' }
      expect(list(row, { ...expense, categoryId: 'c2' }, params)).toHaveLength(0)
      expect(list(row, { ...expense, categoryId: null }, params)).toHaveLength(0)
      expect(list(row, { ...expense, tagId: 't1' }, params)).toHaveLength(0)
      expect(list(row, { type: 'income' }, params)).toHaveLength(0)
      expect(list(row, expense, params)).toHaveLength(1)
    })

    it('tagId lists that tag, narrowed by a category or by none', () => {
      expect(list(row, { ...expense, tagId: 't2' }, { ...period, tagId: 't1' })).toHaveLength(0)
      expect(list(row, { ...expense, tagId: 't1' }, { ...period, tagId: 't1' })).toHaveLength(1)
      expect(list(row, { ...expense, tagId: 't1', categoryId: 'c2' }, { ...period, tagId: 't1', categoryId: 'c1' })).toHaveLength(0)
      expect(list(row, { ...expense, tagId: 't1', categoryId: 'c2' }, { ...period, tagId: 't1', uncategorized: true })).toHaveLength(0)
      expect(list(row, { ...expense, tagId: 't1', categoryId: null }, { ...period, tagId: 't1', uncategorized: true })).toHaveLength(1)
    })

    it('uncategorized alone lists rows with neither category nor tag', () => {
      const params = { ...period, uncategorized: true }
      expect(list(row, { ...expense, categoryId: 'c1' }, params)).toHaveLength(0)
      expect(list(row, { ...expense, categoryId: null, tagId: 't1' }, params)).toHaveLength(0)
      expect(list(row, { ...expense, categoryId: null }, params)).toHaveLength(1)
    })

    it('labelId follows the row\'s labels, narrowed the way the server narrows it', () => {
      expect(list(row, { ...expense, labelIds: [] }, { ...period, labelId: 'l1' })).toHaveLength(0)
      expect(list(row, { ...expense, labelIds: ['l1'] }, { ...period, labelId: 'l1' })).toHaveLength(1)
      expect(list(row, { ...expense, labelIds: ['l1'], categoryId: 'c2' }, { ...period, labelId: 'l1', categoryId: 'c1' })).toHaveLength(0)
      expect(list(row, { ...expense, labelIds: ['l1'], categoryId: 'c2' }, { ...period, labelId: 'l1', uncategorized: true })).toHaveLength(0)
    })

    it('envelopeId keeps a row whose category it cannot look up, and drops what no envelope can hold', () => {
      const params = { ...period, envelopeId: 'e1' }
      expect(list(row, { ...expense, categoryId: 'c2' }, params)).toHaveLength(1)
      expect(list(row, { ...expense, categoryId: null }, params)).toHaveLength(0)
      expect(list(row, { ...expense, tagId: 't1' }, params)).toHaveLength(0)
    })

    it('income selectors read the type and, when named, the category', () => {
      expect(list(row, expense, { ...period, income: true, categoryId: 'c1' })).toHaveLength(0)
      expect(list(row, { type: 'income', categoryId: 'c2' }, { ...period, income: true, categoryId: 'c1' })).toHaveLength(0)
      expect(list(row, { type: 'income', categoryId: 'c1', tagId: 't1' }, { ...period, income: true, categoryId: 'c1' })).toHaveLength(1)
      expect(list(row, { type: 'income', categoryId: 'c2' }, { ...period, income: true, envelopeId: 'e1' })).toHaveLength(1)
      expect(list(row, expense, { ...period, income: true, uncategorized: true })).toHaveLength(0)
      expect(list(row, { type: 'income', categoryId: 'c2' }, { ...period, income: true, uncategorized: true })).toHaveLength(1)
    })

    it('transfers lists transfers, its boundary being the server\'s to judge', () => {
      expect(list(row, expense, { ...period, transfers: true })).toHaveLength(0)
      expect(list(row, { type: 'transfer', accountRecipientId: 'a2' }, { ...period, transfers: true })).toHaveLength(1)
    })

    it('accountId lists either side of the account', () => {
      const params = { ...period, accountId: 'acc' }
      expect(list(row, { ...expense, accountId: 'other' }, params)).toHaveLength(0)
      expect(list(row, { ...expense, accountId: 'acc' }, params)).toHaveLength(1)
      expect(list(row, { type: 'transfer', accountId: 'other', accountRecipientId: 'acc' }, params)).toHaveLength(1)
      expect(list(row, { ...expense, accountId: 'other', accountRecipientId: 'acc' }, params)).toHaveLength(0)
    })

    it('a row is listed while any of a merged list\'s queries still holds it', () => {
      const months = [{ ...period, accountId: 'acc' }, { budgetId: 'b1', periodStart: '2026-12-01', accountId: 'acc' }]
      const out = overlayBudgetTransactions([row], [edit({ ...expense, accountId: 'acc', date: '2026-12-08 00:00:00' })], {}, { params: months, meta })
      expect(out).toHaveLength(1)
    })

    it('judges the row by the last change waiting on it', () => {
      const params = { ...period, categoryId: 'c1' }
      const moved = op({ kind: 'tx-update', txId: 'x1', payload: pay('x1', { ...expense, categoryId: 'c2', date: '2026-11-08 00:00:00' }) })
      const back = op({ kind: 'tx-update', txId: 'x1', payload: pay('x1', { ...expense, categoryId: 'c1', tagId: null, date: '2026-11-08 00:00:00' }) })
      expect(overlayBudgetTransactions([row], [moved, back], {}, { params: [params], meta })).toHaveLength(1)
    })

    it('leaves rows no change touches', () => {
      const params = { ...period, categoryId: 'c1' }
      expect(overlayBudgetTransactions(rows, [], {}, { params: [params], meta })).toBe(rows)
      expect(overlayBudgetTransactions([tagged], [op({ kind: 'tx-delete', txId: 'other', previous: null })], {}, { params: [params], meta })).toEqual([tagged])
    })
  })

  describe('a date edit inside the period', () => {
    const on = (id: string, spentAt: string) => ({ ...row, id, spentAt }) as BudgetTransactionDto
    const list = [on('a', '2026-11-20 10:00:00'), on('b', '2026-11-15 10:00:00'), on('c', '2026-11-15 09:00:00'), on('d', '2026-11-03 10:00:00')]
    const editOf = (txId: string, date: string) => op({ kind: 'tx-update', txId, payload: pay(txId, { date, categoryId: 'c1' }) })

    it('puts the row under its new day, newest first', () => {
      const out = overlayBudgetTransactions(list, [editOf('d', '2026-11-17 00:00:00')])
      expect(out.map((r) => r.id)).toEqual(['a', 'd', 'b', 'c'])
    })

    it('keeps the order within a day, not the time of day', () => {
      const out = overlayBudgetTransactions(list, [editOf('c', '2026-11-15 00:00:00')])
      expect(out.map((r) => r.id)).toEqual(['a', 'b', 'c', 'd'])
    })

    it('leaves a list in another order alone when no date moved', () => {
      const ascending = [on('p', '2026-11-01 00:00:00'), on('q', '2026-11-02 00:00:00')]
      const out = overlayBudgetTransactions(ascending, [op({ kind: 'tx-update', txId: 'p', payload: pay('p', { date: '2026-11-01 00:00:00', categoryId: 'c1' }) })])
      expect(out.map((r) => r.id)).toEqual(['p', 'q'])
    })
  })
})
