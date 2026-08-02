import type { Id } from '../types'

export interface CurrencyDto {
  id: Id
  code: string
  name: string
  symbol: string
  fractionDigits: number
}

// get-currency-list items (and the create/update-currency item echo) carry
// scope/isHidden on top of the shared shape; account/transaction embeds
// never do, mirroring the Go CurrencyResult vs CurrencyListItem split.
export interface CurrencyListItemDto extends CurrencyDto {
  scope: 'global' | 'own' | 'shared'
  isHidden: 0 | 1
  // Deleted currencies are still returned: accounts and rates referencing them
  // must keep resolving. Every picker and list filters them out itself.
  isDeleted: 0 | 1
}

export interface CurrencyRateDto {
  currencyId: Id
  baseCurrencyId: Id
  /** decimal string (wire format, kept verbatim) */
  rate: string
  updatedAt: string
}
