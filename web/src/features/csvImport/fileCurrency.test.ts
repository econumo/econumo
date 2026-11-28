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
      trip: { currencyId: 'cur-eur', unknownCode: null },
      home: { currencyId: 'cur-usd', unknownCode: 'AUD' },
      mixed: { currencyId: 'cur-usd', unknownCode: null },
      bare: { currencyId: 'cur-usd', unknownCode: null },
    })
  })

  it('without a currency column every account takes the choice’s currency; an account’s own pick wins either way', () => {
    const none = withAccountCurrencies(file, { ...choice, currencyColumn: null, currencyId: 'cur-eur', overrides: { home: 'cur-usd' } }, currencies)
    expect(none.currencies).toEqual({ trip: 'cur-eur', home: 'cur-usd', mixed: 'cur-eur', bare: 'cur-eur' })
    expect(withAccountCurrencies(file, { ...choice, overrides: { trip: 'cur-usd' } }, currencies).currencies.trip).toBe('cur-usd')
  })
})
