import type { CurrencyDto } from '@/api/dto/currency'

// Dropdown-eligible currencies: visible globals plus the user's own active
// customs. Foreign (shared-visible) and archived/hidden entries stay out,
// except the entity's current value so an edit form cannot self-corrupt.
export function selectableCurrencies(items: CurrencyDto[] | undefined, currentId?: string): CurrencyDto[] {
  return (items ?? []).filter(
    (c) =>
      c.id === currentId ||
      (c.scope === 'global' && c.isHidden === 0) ||
      (c.scope === 'own' && c.isArchived === 0),
  )
}
