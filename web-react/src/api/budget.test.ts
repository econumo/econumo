import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import * as budgetApi from './budget'
import { fixtureOwner } from '@/test/fixtures'

export const wireBudget = {
  meta: {
    id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
  },
  filters: { periodStart: '2026-07-01 00:00:00', periodEnd: '2026-08-01 00:00:00', excludedAccountsIds: ['a-excluded'] },
  balances: [
    { currencyId: 'cur-usd', startBalance: '100.5', endBalance: null, income: '500', expenses: '-45.5', exchanges: '0', holdings: '0' },
    { currencyId: 'cur-eur', startBalance: null, endBalance: null, income: null, expenses: null, exchanges: null, holdings: '10' },
  ],
  currencyRates: [{ currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.9', periodStart: '2026-07-01', periodEnd: '2026-08-01' }],
  structure: {
    folders: [{ id: 'bf1', name: 'Essentials', position: 0 }],
    elements: [
      {
        id: 'cat-food', type: 1, name: 'Food', icon: 'restaurant', currencyId: null, isArchived: 0,
        folderId: 'bf1', position: 0, budgeted: '200', available: '154.5', spent: '-45.5', budgetSpent: '-45.5',
        ownerUserId: 'u1', children: [],
      },
      {
        id: 'env-1', type: 0, name: 'Living', icon: 'home', currencyId: 'cur-eur', isArchived: 0,
        folderId: null, position: 1, budgeted: '90', available: '90', spent: '0', budgetSpent: '0',
        ownerUserId: null,
        children: [{ id: 'cat-rent', type: 1, name: 'Rent', icon: 'house', isArchived: 0, spent: '0', budgetSpent: '0', ownerUserId: 'u1' }],
      },
      {
        id: 'tag-old', type: 2, name: 'zzz-archived', icon: 'tag', currencyId: null, isArchived: 1,
        folderId: null, position: 2, budgeted: '0', available: '0', spent: '0', budgetSpent: '0',
        ownerUserId: 'u1', children: [],
      },
    ],
  },
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('get-budget coerces every decimal-string field, null-preserving for balances', async () => {
  let url = ''
  server.use(
    http.get('*/api/v1/budget/get-budget', ({ request }) => {
      url = request.url
      return HttpResponse.json({ success: true, message: '', data: { item: wireBudget } })
    }),
  )
  const budget = await budgetApi.getBudget('b1', '2026-07-01')
  expect(url).toContain('id=b1')
  expect(url).toContain('date=2026-07-01')
  expect(budget.balances[0].startBalance).toBe(100.5)
  expect(budget.balances[0].endBalance).toBeNull()
  expect(budget.balances[1].income).toBeNull()
  expect(budget.balances[1].holdings).toBe(10)
  expect(budget.currencyRates[0].rate).toBe(0.9)
  const food = budget.structure.elements[0]
  expect(food.budgeted).toBe(200)
  expect(food.available).toBe(154.5)
  expect(food.spent).toBe(-45.5)
  expect(food.budgetSpent).toBe(-45.5)
  expect(budget.structure.elements[1].children[0].spent).toBe(0)
})

it('set-limit posts amount null verbatim (clear) and strings otherwise', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await budgetApi.setLimit({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '150.5' })
  await budgetApi.setLimit({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: null })
  expect(bodies[0]).toEqual({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '150.5' })
  expect(bodies[1]).toEqual({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: null })
})

it('move-element-list and exclude-account post the exact wire shapes', async () => {
  let moveBody: unknown
  let excludeBody: unknown
  server.use(
    http.post('*/api/v1/budget/move-element-list', async ({ request }) => {
      moveBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/budget/exclude-account', async ({ request }) => {
      excludeBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: wireBudget.meta } })
    }),
  )
  await budgetApi.moveElements('b1', [{ id: 'cat-food', folderId: null, position: 0 }])
  expect(moveBody).toEqual({ budgetId: 'b1', items: [{ id: 'cat-food', folderId: null, position: 0 }] })
  await budgetApi.excludeAccount('b1', 'a1')
  // the budget id travels under "id" on this endpoint
  expect(excludeBody).toEqual({ id: 'b1', accountId: 'a1' })
})

it('budget transactions pass the element param and coerce amounts', async () => {
  let url = ''
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      url = request.url
      return HttpResponse.json({
        success: true, message: '',
        data: { items: [{ id: 't1', author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '9.99', amountRecipient: '9.99', categoryId: 'cat-food', description: '', payeeId: null, tagId: null, date: '2026-07-02 09:30:00' }] },
      })
    }),
  )
  const items = await budgetApi.getBudgetTransactions({ budgetId: 'b1', periodStart: '2026-07-01', categoryId: 'cat-food' })
  expect(url).toContain('categoryId=cat-food')
  expect(url).not.toContain('tagId')
  expect(items[0].amount).toBe(9.99)
})
