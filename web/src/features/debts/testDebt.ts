import type { DebtDto } from '@/api/dto/debt'

/** a 12-payment, interest-free car loan owed to the first user */
export const wireDebt: DebtDto = {
  id: 'd1',
  name: 'Car loan',
  icon: 'directions_car',
  currencyId: 'cur-usd',
  categoryId: 'cat-food',
  payeeId: null,
  tagId: null,
  labelId: null,
  startDate: '',
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
  payoffDate: '2027-11-05',
  nextDueDate: '2026-12-05',
  periodRate: '0',
  isPaidOff: 0,
  isSetUp: 1,
}

/** the car loan before its terms are entered: payment and schedule only */
export const wireDebtNotSetUp: DebtDto = {
  ...wireDebt,
  balance: '',
  lastPaymentDate: '',
  currentBalance: '',
  paymentsLeft: 0,
  moneyLeft: '',
  overpaymentLeft: '',
  overpaidSoFar: '',
  payoffDate: '',
  nextDueDate: '',
  periodRate: '',
  isSetUp: 0,
}
