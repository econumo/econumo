import { describe, expect, it } from 'vitest'
import { currencyForRegion, guessCurrencyCode } from './regionCurrency'

describe('regionCurrency', () => {
  it('maps regions to currencies', () => {
    expect(currencyForRegion('CH')).toBe('CHF')
    expect(currencyForRegion('DE')).toBe('EUR')
    expect(currencyForRegion('us')).toBe('USD')
    expect(currencyForRegion('ZZ')).toBeNull()
  })

  it('guesses from the first locale with a region', () => {
    expect(guessCurrencyCode(['en', 'de-CH'])).toBe('CHF')
    expect(guessCurrencyCode(['en'])).toBeNull()
    expect(guessCurrencyCode(['not a locale', 'pt-BR'])).toBe('BRL')
    expect(guessCurrencyCode([])).toBeNull()
  })
})
