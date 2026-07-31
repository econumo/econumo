import { exchange } from './exchange'

const usd = { id: 'usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }
const jpy = { id: 'jpy', code: 'JPY', name: 'Yen', symbol: '¥', fractionDigits: 0 }
const currencies = [usd, eur, jpy]

// base currency is USD: rate = units of currency per 1 USD
const rates = [
  { currencyId: 'usd', baseCurrencyId: 'usd', rate: '1', updatedAt: '2026-07-01 00:00:00' },
  { currencyId: 'eur', baseCurrencyId: 'usd', rate: '0.9', updatedAt: '2026-07-01 00:00:00' },
  { currencyId: 'jpy', baseCurrencyId: 'usd', rate: '150', updatedAt: '2026-07-01 00:00:00' },
]

it('returns the amount unchanged for same currency', () => {
  expect(exchange('usd', 'usd', '42.5', rates, currencies)).toBe('42.5')
})

it('converts through the base currency both directions', () => {
  // 90 EUR -> USD: 90 / 0.9 = 100
  expect(exchange('eur', 'usd', '90', rates, currencies)).toBe('100')
  // 100 USD -> EUR: 100 * 0.9 = 90
  expect(exchange('usd', 'eur', '100', rates, currencies)).toBe('90')
  // 90 EUR -> JPY: 90 / 0.9 * 150 = 15000, rounded to 0 digits
  expect(exchange('eur', 'jpy', '90', rates, currencies)).toBe('15000')
})

it('rounds to the target currency fraction digits', () => {
  expect(exchange('usd', 'eur', '1', rates, currencies)).toBe('0.9')
  expect(exchange('eur', 'jpy', '1', rates, currencies)).toBe('167')
})

it('falls back to the raw amount when rates or currencies are missing', () => {
  expect(exchange('usd', 'eur', '10', [], currencies)).toBe('10')
  expect(exchange('usd', 'xxx', '10', rates, currencies)).toBe('10')
  expect(exchange('xxx', 'eur', '10', rates, currencies)).toBe('10')
})

it('keeps large amounts exact through conversion', () => {
  // 12345678901234567.89 USD -> EUR at 0.9
  expect(exchange('usd', 'eur', '12345678901234567.89', rates, currencies)).toBe('11111111011111111.1')
})
