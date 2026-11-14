import type { DebtDto, DebtRuleDto } from '@/api/dto/debt'

/** a whole-category rule on the fixture user's (u1, Ada) Food */
export const wireRule: DebtRuleDto = {
  id: 'r1',
  categoryId: 'cat-food',
  categoryName: 'Food',
  categoryIcon: 'restaurant',
  ownerUserId: 'u1',
  ownerName: 'Ada',
  payeeId: null,
  payeeName: '',
  tagId: null,
  tagName: '',
  labelId: null,
  labelName: '',
}

/** a 12-payment, interest-free car loan of 1200 taken on 2026-11-05, in the fixture's Main budget */
export const wireDebt: DebtDto = {
  id: 'd1',
  budgetId: 'b1',
  budgetName: 'Main budget',
  canEdit: 1,
  name: 'Car loan',
  icon: 'directions_car',
  currencyId: 'cur-usd',
  rules: [wireRule],
  startDate: '2026-11-05',
  amountBorrowed: '1200',
  balance: '1200',
  payment: '100',
  schedule: 'monthly',
  lastPaymentDate: '2027-11-05',
  currentBalance: '1200',
  paymentsLeft: 12,
  periodsPaid: 0,
  moneyLeft: '1200',
  overpaymentLeft: '0',
  overpaidSoFar: '0',
  extraPaid: '0',
  payoffDate: '2027-11-05',
  nextDueDate: '2026-12-05',
  periodRate: '0',
  annualRate: '0',
  impliedAnnualRate: '0',
  isPaidOff: 0,
  isSetUp: 1,
}

/** the car loan before its terms are entered: payment and schedule only */
export const wireDebtNotSetUp: DebtDto = {
  ...wireDebt,
  startDate: '',
  amountBorrowed: '',
  balance: '',
  lastPaymentDate: '',
  currentBalance: '',
  paymentsLeft: 0,
  moneyLeft: '',
  overpaymentLeft: '',
  overpaidSoFar: '',
  extraPaid: '',
  payoffDate: '',
  nextDueDate: '',
  periodRate: '',
  annualRate: '',
  impliedAnnualRate: '',
  isSetUp: 0,
}

/** get-debt-rule-options for the fixture budget: the caller (Ada) and a partner (Anna) */
export const wireRuleOptions = {
  participants: [
    {
      userId: 'u1',
      name: 'Ada',
      categories: [{ id: 'cat-food', name: 'Food', icon: 'restaurant' }],
      payees: [{ id: 'p1', name: 'Grocer', icon: '' }],
      tags: [{ id: 'tag1', name: 'vacation', icon: 'tag' }],
      labels: [{ id: 'label1', name: 'health', icon: 'sell' }],
    },
    {
      userId: 'u2',
      name: 'Anna',
      categories: [{ id: 'cat-mortgage', name: 'Mortgage', icon: 'home' }],
      payees: [{ id: 'p-bank', name: 'Bank', icon: '' }],
      tags: [],
      labels: [],
    },
  ],
}
