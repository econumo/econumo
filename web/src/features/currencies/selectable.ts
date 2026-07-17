import type { CurrencyListItemDto } from '@/api/dto/currency'

// Dropdown-eligible currencies: visible globals plus the user's own active
// customs. Foreign (shared-visible) and archived/hidden entries stay out,
// except the entity's current value so an edit form cannot self-corrupt.
export function selectableCurrencies(items: CurrencyListItemDto[] | undefined, currentId?: string): CurrencyListItemDto[] {
  return (items ?? []).filter(
    (c) =>
      c.id === currentId ||
      (c.scope === 'global' && c.isHidden === 0) ||
      (c.scope === 'own' && c.isArchived === 0),
  )
}
