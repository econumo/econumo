import { describe, expect, it } from 'vitest'
import { applyAiGuess, initialAnswers, isEconumoExport, redetectFormats } from './guess'
import { parseCsvText } from './parse'
import type { StepId } from './prepare'

const ctx = { writableAccountIds: ['acc-1', 'acc-2'], userCurrencyId: 'cur-eur', locale: 'de' }

function guess(text: string, overrides: Partial<Parameters<typeof initialAnswers>[1]> = {}) {
  return initialAnswers(parseCsvText(text), { ...ctx, ...overrides })
}

const EXPORT_TEXT =
  'transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date\n' +
  'a1,Cash,USD,Food,Lunch,,,Cafe,-4.5,2026-03-04 14:05:09\n' +
  'a2,Bank,EUR,Salary,,,a;b,,1000,2026-03-05 09:00:00\n' +
  'a3,cash,EUR,Food,,,,,-1,2026-03-06 09:00:00\n' +
  'a4,Card,XYZ,,,,,,-2,2026-03-07 09:00:00\n' +
  'a5,Constructor,USD,,,,,,-3,2026-03-08 09:00:00\n'

describe('initialAnswers', () => {
  it.each([
    ['a German statement', 'Buchungstag;Betrag;Verwendungszweck\n04.03.2026;-1.234,56;Miete März\n05.03.2026;12,00;Kaffee am Bahnhof\n'],
    ['separate debit and credit columns', 'Date,Description,Debit,Credit\n03/14/2026,Coffee Shop,4.50,\n03/15/2026,Payroll,,100.00\n'],
    [
      'repeating names beside free text',
      'Дата;Сумма;Категория;Описание;Счёт\n04.03.2026;-350,00;Кафе;Обед;Карта\n05.03.2026;-1 200,00;Продукты;Закупка;Карта\n06.03.2026;-90,00;Кафе;Кофе;Наличные\n',
    ],
    ['a currency column', 'Date,Amount,Currency,Account\n2026-03-04,-1.00,EUR,Main\n2026-03-05,-2.00,EUR,Main\n'],
  ])('assigns no column of %s: every step is left for the user', (_, text) => {
    const a = guess(text, { currencies: [{ id: 'cur-eur', code: 'EUR' }] })
    expect(a.account).toBeNull()
    expect(a.date.column).toBeNull()
    expect(a.amount).toMatchObject({ mode: 'single', column: null, inColumn: null, outColumn: null })
    expect(a.description).toBeNull()
    expect(a.extras).toEqual({ payee: null, category: null, tag: null, labels: null, labelsSeparator: ';' })
  })

  it('leaves the account unpicked with one writable account or several; with none it is a new account to name', () => {
    const text = 'Date,Amount\n2026-03-04,1\n'
    expect(guess(text).account).toBeNull()
    expect(guess(text, { writableAccountIds: ['acc-1'] }).account).toBeNull()
    expect(guess(text, { writableAccountIds: [] }).account).toEqual({ kind: 'new', name: '', currencyId: 'cur-eur' })
  })

  it('a new account is in the user’s currency, whatever currency a part of the file names', () => {
    const a = guess('Date,Amount,Currency\n2026-03-04,1,USD\n', { writableAccountIds: [], currencies: [{ id: 'cur-usd', code: 'USD' }] })
    expect(a.account).toEqual({ kind: 'new', name: '', currencyId: 'cur-eur' })
  })

  it('starts on the locale date order', () => {
    expect(guess('x,y\n2026-03-04,b\n', { locale: 'en-US' }).date).toEqual({ column: null, order: 'mdy' })
  })

  it('maps every role of an Econumo export and takes currencies from account_currency', () => {
    const a = guess(EXPORT_TEXT, {
      currencies: [
        { id: 'cur-usd', code: 'USD' },
        { id: 'cur-eur', code: 'EUR' },
      ],
    })
    expect(a.account).toEqual({
      kind: 'column',
      column: 1,
      currencyColumn: 2,
      currencyId: 'cur-eur',
      overrides: {},
      // "Cash" and "cash" name two currencies, so the account takes the default
      currencies: { cash: 'cur-eur', bank: 'cur-eur', card: 'cur-eur', constructor: 'cur-usd' },
    })
    expect(a.date).toEqual({ column: 9, order: 'ymd' })
    expect(a.amount).toMatchObject({ mode: 'single', column: 8, flip: false, sep: '.' })
    expect(a.description).toBe(4)
    expect(a.extras).toEqual({ payee: 7, category: 3, tag: 5, labels: 6, labelsSeparator: ';' })
  })
})

describe('isEconumoExport', () => {
  it('needs the whole export header', () => {
    expect(isEconumoExport(parseCsvText(EXPORT_TEXT))).toBe(true)
    expect(isEconumoExport(parseCsvText('date,amount,tag,description\n2026-03-04,1,,\n'))).toBe(false)
  })
})

describe('applyAiGuess', () => {
  const text = 'c0,c1,c2,c3\n2026-03-04,1,Lunch,Cafe\n'
  const file = parseCsvText(text)

  it('leaves a touched step alone and fills an untouched one', () => {
    const base = guess(text)
    const touched = new Set<StepId>(['date'])
    const withDate = { ...base, date: { column: 0, order: base.date.order } }
    const next = applyAiGuess(withDate, { date: 'c2', description: 'c2' }, file, 'de', touched)
    expect(next.date.column).toBe(0)
    expect(next.description).toBe(2)
  })

  it('fills every untouched step it names on a file the wizard itself assigns nothing in', () => {
    const f = parseCsvText('Wann,Wieviel,Was,Wer,Art\n2026-03-04,-1,Lunch,Cafe,Food\n2026-03-05,-2,Tea,Shop,Food\n')
    const next = applyAiGuess(initialAnswers(f, ctx), { date: 'Wann', amount: 'Wieviel', description: 'Was', payee: 'Wer', category: 'Art' }, f, 'de', new Set())
    expect(next.date).toEqual({ column: 0, order: 'ymd' })
    expect(next.amount).toMatchObject({ mode: 'single', column: 1, sep: '.' })
    expect(next.description).toBe(2)
    expect(next.extras).toMatchObject({ payee: 3, category: 4, tag: null, labels: null })
    expect(next.account).toBeNull()
  })

  it('ignores names that are not in the header', () => {
    const base = guess(text)
    const next = applyAiGuess({ ...base, description: 2 }, { description: 'Nope' }, file, 'de', new Set())
    expect(next.description).toBe(2)
  })

  it('switches to two amount columns only when both are given', () => {
    const base = guess(text)
    expect(applyAiGuess(base, { amountIn: 'c1' }, file, 'de', new Set()).amount.mode).toBe('single')
    const dual = applyAiGuess(base, { amountIn: 'c1', amountOut: 'c3' }, file, 'de', new Set())
    expect(dual.amount).toMatchObject({ mode: 'dual', inColumn: 1, outColumn: 3 })
  })

  it('never reuses a column a touched step holds, and frees a column it reassigns', () => {
    const base = { ...guess(text), description: 2, extras: { ...guess(text).extras, payee: 3 } }
    const touchedDescription = applyAiGuess(base, { payee: 'c2' }, file, 'de', new Set<StepId>(['description']))
    expect(touchedDescription.extras.payee).toBe(3)
    const moved = applyAiGuess(base, { payee: 'c2' }, file, 'de', new Set())
    expect(moved.extras.payee).toBe(2)
    expect(moved.description).toBeNull()
  })

  it('re-detects date order and decimal separator for the columns it picks', () => {
    const f = parseCsvText('a;b;c;d\n04.03.2026;03/14/2026;1,50;x\n05.03.2026;03/15/2026;2,00;y\n')
    const base = { ...initialAnswers(f, ctx), date: { column: 0, order: 'dmy' as const } }
    const next = applyAiGuess(base, { date: 'b', amount: 'c' }, f, 'de', new Set())
    expect(next.date).toEqual({ column: 1, order: 'mdy' })
    expect(next.amount).toMatchObject({ mode: 'single', column: 2, sep: ',' })
    const touched = applyAiGuess(base, { date: 'b' }, f, 'de', new Set<StepId>(['date']))
    expect(touched.date).toEqual(base.date)
  })
})

describe('redetectFormats', () => {
  const f = parseCsvText('a;b;c;d\n03/14/2026;1,50;;2.000,00\n03/15/2026;2,00;10;\n')
  const start = {
    ...initialAnswers(f, ctx),
    date: { column: 0, order: 'dmy' as const },
  }

  it('re-runs only the parts that changed', () => {
    const amountOnly = redetectFormats({ ...start, amount: { ...start.amount, mode: 'single' as const, column: 1, sep: '.' as const } }, f, 'de', { amount: true })
    expect(amountOnly.date.order).toBe('dmy')
    expect(amountOnly.amount.sep).toBe(',')
    expect(redetectFormats(start, f, 'de', { date: true }).date.order).toBe('mdy')
  })

  it('reads both columns in two-column mode and falls back to the locale without a date column', () => {
    const dual = { ...start, amount: { ...start.amount, mode: 'dual' as const, inColumn: 2, outColumn: 3, sep: '.' as const } }
    expect(redetectFormats(dual, f, 'de', { amount: true }).amount.sep).toBe(',')
    const noDate = { ...start, date: { column: null, order: 'ymd' as const } }
    expect(redetectFormats(noDate, f, 'en-US', { date: true }).date.order).toBe('mdy')
  })
})
