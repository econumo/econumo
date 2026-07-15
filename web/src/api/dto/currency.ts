import type { Id } from '../types'

export interface CurrencyDto {
  id: Id
  code: string
  name: string
  symbol: string
  fractionDigits: number
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
