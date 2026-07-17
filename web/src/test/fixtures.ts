import { http, HttpResponse } from 'msw'

export const fixtureUser = {
  id: 'u1',
  name: 'Ada',
  email: 'ada@example.test',
  avatar: 'face:emerald',
  options: [
    { name: 'currency', value: 'USD' },
    { name: 'currency_id', value: 'cur-usd' },
    { name: 'report_period', value: 'monthly' },
    { name: 'budget', value: null },
    { name: 'onboarding', value: 'completed' },
  ],
  currency: 'USD',
  reportPeriod: 'monthly',
}

export const fixtureOwner = { id: 'u1', avatar: 'face:emerald', name: 'Ada' }

export const fixtureUsd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
export const fixtureEur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

export const fixtureFolders = [
  { id: 'f1', name: 'General', position: 0, isVisible: 1 },
  { id: 'f2', name: 'Savings', position: 1, isVisible: 1 },
  { id: 'f-hidden', name: 'Hidden', position: 2, isVisible: 0 },
]

export const fixtureAccounts = [
  {
    id: 'a1', owner: fixtureOwner, folderId: 'f1', name: 'Cash', position: 0,
    currency: fixtureUsd, balance: '100.5', type: 1, icon: 'wallet', sharedAccess: [],
  },
  {
    id: 'a2', owner: fixtureOwner, folderId: 'f2', name: 'Bank', position: 1,
    currency: fixtureUsd, balance: '2000', type: 1, icon: 'account_balance', sharedAccess: [],
  },
  {
    id: 'a3', owner: fixtureOwner, folderId: 'f2', name: 'Euro Stash', position: 2,
    currency: fixtureEur, balance: '90', type: 1, icon: 'savings', sharedAccess: [],
  },
  {
    id: 'a-hidden', owner: fixtureOwner, folderId: 'f-hidden', name: 'Under the mattress', position: 3,
    currency: fixtureUsd, balance: '5', type: 1, icon: 'bed', sharedAccess: [],
  },
]

export const fixtureCategories = [
  { id: 'cat-food', ownerUserId: 'u1', name: 'Food', position: 0, type: 'expense', icon: 'restaurant', isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
  { id: 'cat-salary', ownerUserId: 'u1', name: 'Salary', position: 1, type: 'income', icon: 'payments', isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
  { id: 'cat-archived', ownerUserId: 'u1', name: 'Old', position: 2, type: 'expense', icon: 'delete', isArchived: 1, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
]

export const fixturePayees = [
  { id: 'p1', ownerUserId: 'u1', name: 'Grocer', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
]

export const fixtureTags = [
  { id: 'tag1', ownerUserId: 'u1', name: 'vacation', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
]

export const fixtureTransactions = [
  {
    id: 't1', author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: '9.99', amountRecipient: '9.99', categoryId: 'cat-food', description: 'coffee beans',
    payeeId: 'p1', tagId: null, date: '2026-07-02 09:30:00',
  },
  {
    id: 't2', author: fixtureOwner, type: 'income', accountId: 'a1', accountRecipientId: null,
    amount: '500', amountRecipient: '500', categoryId: 'cat-salary', description: '',
    payeeId: null, tagId: null, date: '2026-07-01 08:00:00',
  },
]

export const fixtureBudgets = [
  {
    id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
  },
  {
    id: 'b2', ownerUserId: 'u1', name: 'Alpha plan', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
  },
]

export const fixtureRates = [
  { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', updatedAt: '2026-07-01 00:00:00' },
  { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.9', updatedAt: '2026-07-01 00:00:00' },
]

export const fixtureWireBudget = {
  meta: {
    id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
  },
  filters: { periodStart: '2026-07-01 00:00:00', periodEnd: '2026-08-01 00:00:00', excludedAccountsIds: ['a-excluded'] },
  balances: [
    { currencyId: 'cur-usd', startBalance: '100.5', endBalance: null, income: '500', expenses: '-45.5', exchanges: '0', holdings: '0' },
    { currencyId: 'cur-eur', startBalance: null, endBalance: null, income: null, expenses: null, exchanges: null, holdings: '10' },
  ],
  currencyRates: [
    { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: '2026-07-01', periodEnd: '2026-08-01' },
    { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.9', periodStart: '2026-07-01', periodEnd: '2026-08-01' },
  ],
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

export const fixtureConnections = [
  { user: { id: 'u2', avatar: 'pets:sky', name: 'Partner' }, sharedAccounts: [] },
]

const envelope = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

export function coreHandlers(overrides: Partial<Record<string, unknown>> = {}) {
  const data = {
    accounts: fixtureAccounts,
    folders: fixtureFolders,
    transactions: fixtureTransactions,
    categories: fixtureCategories,
    payees: fixturePayees,
    tags: fixtureTags,
    currencies: [fixtureUsd, fixtureEur],
    rates: fixtureRates,
    budgets: fixtureBudgets,
    user: fixtureUser,
    connections: [],
    recurring: [],
    ...overrides,
  }
  return [
    http.get('*/api/v1/connection/get-connection-list', () => envelope({ items: data.connections })),
    http.get('*/api/v1/account/get-account-list', () => envelope({ items: data.accounts })),
    http.get('*/api/v1/account/get-folder-list', () => envelope({ items: data.folders })),
    http.get('*/api/v1/transaction/get-transaction-list', () => envelope({ items: data.transactions })),
    http.get('*/api/v1/category/get-category-list', () => envelope({ items: data.categories })),
    http.get('*/api/v1/payee/get-payee-list', () => envelope({ items: data.payees })),
    http.get('*/api/v1/tag/get-tag-list', () => envelope({ items: data.tags })),
    http.get('*/api/v1/currency/get-currency-list', () => envelope({ items: data.currencies })),
    http.get('*/api/v1/currency/get-currency-rate-list', () => envelope({ items: data.rates })),
    http.get('*/api/v1/user/get-user-data', () => envelope({ user: data.user })),
    http.get('*/api/v1/budget/get-budget-list', () => envelope({ items: data.budgets })),
    http.get('*/api/v1/recurring/get-recurring-transaction-list', () => envelope({ items: data.recurring })),
  ]
}
