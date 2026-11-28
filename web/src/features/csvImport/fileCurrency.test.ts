import { accountCurrencies, withAccountCurrencies } from './fileCurrency'
import { parseCsvText } from './parse'

const currencies = [
  { id: 'cur-usd', code: 'USD' },
  { id: 'cur-eur', code: 'EUR' },
]

describe('accountCurrencies', () => {
  const file = parseCsvText('name,cur\nTrip,EUR\ntrip,EUR\nHome,AUD\nMixed,EUR\nMixed,USD\nBare,\n')
  const choice = { kind: 'column' as const, column: 0, currencyColumn: 1, currencyId: 'cur-usd', overrides: {}, currencies: {} }

  it('an account takes the one known code its rows name, else the choice’s currency, with an unknown code kept for a note', () => {
    expect(Object.fromEntries(accountCurrencies(file, choice, currencies))).toEqual({
      trip: { currencyId: 'cur-eur', unknownCode: null, explicit: true },
      home: { currencyId: 'cur-usd', unknownCode: 'AUD', explicit: false },
      mixed: { currencyId: 'cur-usd', unknownCode: null, explicit: false },
      bare: { currencyId: 'cur-usd', unknownCode: null, explicit: false },
    })
  })

  it('without a currency column every account takes the choice’s currency; an account’s own pick wins either way', () => {
    const none = withAccountCurrencies(file, { ...choice, currencyColumn: null, currencyId: 'cur-eur', overrides: { home: 'cur-usd' } }, currencies)
    expect(none.currencies).toEqual({ home: 'cur-usd' })
    const resolved = (currencyId: string) => accountCurrencies(file, { ...choice, currencyColumn: null, currencyId, overrides: { home: 'cur-usd' } }, currencies)
    expect(resolved('cur-eur').get('trip')?.currencyId).toBe('cur-eur')
    expect(resolved('cur-usd').get('trip')?.currencyId).toBe('cur-usd')
    expect(resolved('cur-eur').get('home')?.currencyId).toBe('cur-usd')
    expect(withAccountCurrencies(file, { ...choice, overrides: { trip: 'cur-usd' } }, currencies).currencies.trip).toBe('cur-usd')
  })
})
