export interface DefaultCategory {
  key: string
  icon: string
}

export const DEFAULT_EXPENSE_CATEGORIES: readonly DefaultCategory[] = [
  { key: 'groceries', icon: 'shopping_cart' },
  { key: 'housing', icon: 'home' },
  { key: 'utilities', icon: 'bolt' },
  { key: 'transport', icon: 'directions_bus' },
  { key: 'eating_out', icon: 'restaurant' },
  { key: 'health', icon: 'local_hospital' },
  { key: 'shopping', icon: 'shopping_bag' },
  { key: 'entertainment', icon: 'movie' },
  { key: 'kids', icon: 'child_care' },
  { key: 'pets', icon: 'pets' },
  { key: 'subscriptions', icon: 'cloud' },
]

export const DEFAULT_INCOME_CATEGORIES: readonly DefaultCategory[] = [
  { key: 'salary', icon: 'payments' },
  { key: 'bonus', icon: 'redeem' },
  { key: 'other_income', icon: 'paid' },
]
