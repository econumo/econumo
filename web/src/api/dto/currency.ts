import type { Id } from '../types'

export interface CurrencyDto {
  id: Id
  code: string
  name: string
  symbol: string
  fractionDigits: number
}

export interface CurrencyRateDto {
  currencyId: Id
  baseCurrencyId: Id
  /** decimal string (wire format, kept verbatim) */
  rate: string
  updatedAt: string
}
