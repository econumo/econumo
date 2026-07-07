export const queryKeys = {
  accounts: ['accounts'] as const,
  folders: ['folders'] as const,
  transactions: ['transactions'] as const,
  categories: ['categories'] as const,
  payees: ['payees'] as const,
  tags: ['tags'] as const,
  currencies: ['currencies'] as const,
  currencyRates: ['currencyRates'] as const,
  user: ['user'] as const,
  connections: ['connections'] as const,
  budget: ['budget'] as const,
  budgets: ['budgets'] as const,
  budgetTransactions: ['budgetTransactions'] as const,
}

export const TEN_MINUTES = 10 * 60_000
export const ONE_DAY = 24 * 3_600_000
