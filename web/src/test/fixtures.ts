import { http, HttpResponse } from 'msw'
import type { ConnectionDto } from '@/api/dto/connection'

export const fixtureUser = {
  id: 'u1',
  name: 'Ada',
  email: 'ada@example.test',
  avatar: 'face:emerald',
  accessLevel: 'full' as const,
  accessUntil: '',
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

export const fixtureUsd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2, scope: 'global', isHidden: 0, isDeleted: 0 }
export const fixtureEur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2, scope: 'global', isHidden: 0, isDeleted: 0 }

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
  { id: 'tag1', ownerUserId: 'u1', name: 'vacation', icon: 'tag', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
]

// icon intentionally differs from DEFAULT_ICON.label ('label') so tests can tell
// a saved row's STORED icon apart from the create dialog's preview-only default.
export const fixtureLabels = [
  { id: 'label1', ownerUserId: 'u1', name: 'health', icon: 'sell', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
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
        folderId: 'bf1', position: 0, budgeted: '200', available: '154.5', spent: '45.5', budgetSpent: '45.5',
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
    labels: [],
  },
}

// Shared fixture for the budget plan-view sheet: 4 months, one of every element
// shape (folder-bound expense envelope + child, standalone expense category, tag,
// income envelope + child, standalone income category, both uncategorized rows),
// one non-budget-currency row (FX rendering), and one all-zero dormant row
// (hide-empty tests). Reused by every plan-view task, not just this one.
export const fixtureWirePlan = {
  meta: {
    id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
  },
  months: ['2026-05-01', '2026-06-01', '2026-07-01', '2026-08-01'],
  openingBalances: [{ currencyId: 'cur-usd', amount: '500' }],
  currencyRates: [
    { period: '2026-05-01', rates: [{ currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.90', periodStart: '2026-05-01', periodEnd: '2026-06-01' }] },
    { period: '2026-06-01', rates: [{ currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.91', periodStart: '2026-06-01', periodEnd: '2026-07-01' }] },
    { period: '2026-07-01', rates: [{ currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.92', periodStart: '2026-07-01', periodEnd: '2026-08-01' }] },
    { period: '2026-08-01', rates: [{ currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '0.93', periodStart: '2026-08-01', periodEnd: '2026-09-01' }] },
  ],
  structure: {
    folders: [{ id: 'bf1', name: 'Essentials', position: 0 }],
    elements: [
      {
        id: 'pe1', type: 0, name: 'Living', icon: 'home', currencyId: 'cur-usd', isArchived: 0,
        folderId: 'bf1', position: 0, ownerUserId: null,
        cells: [
          { actual: '50', planned: '200' },
          { actual: '60', planned: '200' },
          { actual: '45', planned: '250' },
          { actual: '0', planned: '' },
        ],
        children: [
          {
            id: 'cat-rent', type: 1, name: 'Rent', icon: 'house', isArchived: 0, ownerUserId: 'u1',
            cells: [{ actual: '50' }, { actual: '60' }, { actual: '45' }, { actual: '0' }],
          },
        ],
      },
      {
        id: 'cat-food', type: 1, name: 'Food', icon: 'restaurant', currencyId: 'cur-usd', isArchived: 0,
        folderId: null, position: 1, ownerUserId: 'u1',
        cells: [
          { actual: '120', planned: '150' },
          { actual: '130', planned: '150' },
          { actual: '125', planned: '' },
          { actual: '0', planned: '' },
        ],
        children: [],
      },
      {
        id: 'tag1', type: 2, name: 'vacation', icon: 'tag', currencyId: 'cur-usd', isArchived: 0,
        folderId: null, position: 2, ownerUserId: 'u1',
        cells: [
          { actual: '20', planned: '' },
          { actual: '25', planned: '' },
          { actual: '15', planned: '50' },
          { actual: '0', planned: '' },
        ],
        children: [],
      },
      {
        id: 'ie1', type: 4, name: 'Salaries', icon: 'payments', currencyId: 'cur-usd', isArchived: 0,
        folderId: null, position: 3, ownerUserId: null,
        cells: [
          { actual: '2000', planned: '' },
          { actual: '2000', planned: '2000' },
          { actual: '0', planned: '2000' },
          { actual: '0', planned: '' },
        ],
        children: [
          {
            id: 'cat-salary', type: 3, name: 'Salary', icon: 'payments', isArchived: 0, ownerUserId: 'u1',
            cells: [{ actual: '2000' }, { actual: '2000' }, { actual: '0' }, { actual: '0' }],
          },
        ],
      },
      {
        id: 'cat-freelance', type: 3, name: 'Freelance', icon: 'work', currencyId: 'cur-usd', isArchived: 0,
        folderId: null, position: 4, ownerUserId: 'u1',
        cells: [
          { actual: '300', planned: '' },
          { actual: '0', planned: '500' },
          { actual: '400', planned: '500' },
          { actual: '0', planned: '' },
        ],
        children: [],
      },
      {
        id: 'uncategorized', type: 1, name: 'Uncategorized', icon: 'question_mark', currencyId: 'cur-usd', isArchived: 0,
        folderId: null, position: 5, ownerUserId: null,
        cells: [
          { actual: '10', planned: '' },
          { actual: '0', planned: '' },
          { actual: '5', planned: '' },
          { actual: '0', planned: '' },
        ],
        children: [],
      },
      {
        id: 'uncategorized', type: 3, name: 'Uncategorized', icon: 'question_mark', currencyId: 'cur-usd', isArchived: 0,
        folderId: null, position: 6, ownerUserId: null,
        cells: [
          { actual: '0', planned: '' },
          { actual: '50', planned: '' },
          { actual: '0', planned: '' },
          { actual: '0', planned: '' },
        ],
        children: [],
      },
      {
        id: 'env-eur', type: 0, name: 'Euro Stash', icon: 'savings', currencyId: 'cur-eur', isArchived: 0,
        folderId: null, position: 7, ownerUserId: null,
        cells: [
          { actual: '30', planned: '100' },
          { actual: '40', planned: '100' },
          { actual: '35', planned: '' },
          { actual: '0', planned: '' },
        ],
        children: [],
      },
      {
        id: 'cat-dormant', type: 1, name: 'Unused', icon: 'inbox', currencyId: 'cur-usd', isArchived: 0,
        folderId: null, position: 8, ownerUserId: 'u1',
        cells: [
          { actual: '0', planned: '' },
          { actual: '0', planned: '' },
          { actual: '0', planned: '' },
          { actual: '0', planned: '' },
        ],
        children: [],
      },
    ],
  },
}

export function planHandler(plan: unknown = fixtureWirePlan) {
  return http.get('*/api/v1/budget/get-budget-plan', () => envelope({ item: plan }))
}

export const fixtureConnections: ConnectionDto[] = [
  { user: { id: 'u2', avatar: 'pets:sky', name: 'Partner' }, accessLevel: 'full', accessUntil: '', sharedAccounts: [] },
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
    labels: fixtureLabels,
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
    http.get('*/api/v1/label/get-label-list', () => envelope({ items: data.labels })),
    http.get('*/api/v1/currency/get-currency-list', () => envelope({ items: data.currencies })),
    http.get('*/api/v1/currency/get-currency-rate-list', () => envelope({ items: data.rates })),
    http.get('*/api/v1/user/get-user-data', () => envelope({ user: data.user })),
    http.get('*/api/v1/budget/get-budget-list', () => envelope({ items: data.budgets })),
    http.get('*/api/v1/recurring/get-recurring-transaction-list', () => envelope({ items: data.recurring })),
    http.get('*/api/v1/system/get-update-info', () => envelope({ version: 'v0.0.0', url: 'https://econumo.com/releases/v0.0.0/' })),
  ]
}
