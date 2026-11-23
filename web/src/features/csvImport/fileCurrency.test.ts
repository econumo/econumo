import { currencyColumn, rowsCurrencyId, unknownRowsCurrency } from './fileCurrency'
import { parseCsvText } from './parse'

const currencies = [
  { id: 'cur-usd', code: 'USD' },
  { id: 'cur-eur', code: 'EUR' },
]

it('reads a currency column even when one of its codes is not set up here', () => {
  const file = parseCsvText('name,account_currency,amount\nTrip,EUR,1\nHome,CAD,2\n')
  const column = currencyColumn(file, currencies)
  expect(column).toBe(1)
  expect(rowsCurrencyId([file.rows[0]], column, currencies)).toBe('cur-eur')
  expect(rowsCurrencyId([file.rows[1]], column, currencies)).toBeNull()
  expect(unknownRowsCurrency([file.rows[1]], column, currencies)).toBe('CAD')
  expect(unknownRowsCurrency([file.rows[0]], column, currencies)).toBeNull()
})

it('a column of three-letter words is a currency column only when its header says so', () => {
  expect(currencyColumn(parseCsvText('name,code\nA,ABC\n'), currencies)).toBeNull()
  expect(currencyColumn(parseCsvText('name,Currency\nA,CAD\n'), currencies)).toBe(1)
})
