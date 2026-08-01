import type { CurrencyListItemDto } from '@/api/dto/currency'

// Dropdown-eligible currencies: visible globals plus the user's own visible
// customs. Foreign (shared-visible) and hidden entries stay out, except the
// entity's current value so an edit form cannot self-corrupt.
export function selectableCurrencies(items: CurrencyListItemDto[] | undefined, currentId?: string): CurrencyListItemDto[] {
  return (items ?? []).filter(
    (c) => c.id === currentId || (c.scope !== 'shared' && c.isHidden === 0),
  )
}
