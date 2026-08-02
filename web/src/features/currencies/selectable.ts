import type { CurrencyListItemDto } from '@/api/dto/currency'

// Dropdown-eligible currencies: visible, non-deleted globals plus the user's
// own visible, non-deleted customs. Foreign (shared-visible), hidden and
// deleted entries stay out, except the entity's current value so an edit form
// cannot self-corrupt -- which is how an account denominated in a deleted
// currency keeps being editable.
export function selectableCurrencies(items: CurrencyListItemDto[] | undefined, currentId?: string): CurrencyListItemDto[] {
  return (items ?? []).filter(
    (c) => c.id === currentId || (c.scope !== 'shared' && c.isHidden === 0 && c.isDeleted === 0),
  )
}
