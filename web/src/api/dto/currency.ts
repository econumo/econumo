import type { Id } from '../types'

export interface CurrencyDto {
  id: Id
  code: string
  name: string
  symbol: string
  fractionDigits: number
}

// get-currency-list items (and the create/update-currency item echo) carry
// scope/isArchived/isHidden on top of the shared shape; account/transaction
// embeds never do, mirroring the Go CurrencyResult vs CurrencyListItem split.
export interface CurrencyListItemDto extends CurrencyDto {
  scope: 'global' | 'own' | 'shared'
  isArchived: 0 | 1
  isHidden: 0 | 1
}

export interface CurrencyRateDto {
  currencyId: Id
  baseCurrencyId: Id
  /** wire: decimal string, coerced */
  rate: number
  updatedAt: string
}
