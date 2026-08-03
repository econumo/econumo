import type { CurrencyListItemDto } from '@/api/dto/currency'

// Dropdown-eligible currencies: visible, non-deleted globals plus the user's
// non-deleted customs. Own customs have no hide affordance, so isHidden is
// ignored for them -- a row left hidden by a previous UI version, or by an
// API/MCP client calling hide-currency directly, must not go stranded out of
// every picker with no way back in. Foreign (shared-visible) and deleted
// entries stay out, except the entity's current value so an edit form cannot
// self-corrupt -- which is how an account denominated in a deleted currency
// keeps being editable.
export function selectableCurrencies(items: CurrencyListItemDto[] | undefined, currentId?: string): CurrencyListItemDto[] {
  return (items ?? []).filter(
    (c) => c.id === currentId || (c.scope !== 'shared' && c.isDeleted === 0 && (c.scope === 'own' || c.isHidden === 0)),
  )
}
