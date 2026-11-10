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

function matchesWords(name: string, words: string): boolean {
  const haystack = name.toLowerCase()
  return words
    .split('|')
    .map((w) => w.trim().toLowerCase())
    .filter(Boolean)
    .some((w) => haystack.includes(w))
}

// Only cash is guessed from the name; savings is never guessed (the AI may flag it in the budget).
export function guessAccountIcon(name: string, words: { cash: string }): string {
  if (matchesWords(name, words.cash)) return 'payments'
  return 'credit_card'
}
