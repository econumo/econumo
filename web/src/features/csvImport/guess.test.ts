import { describe, expect, it } from 'vitest'
import { profileColumns } from './columns'
import { applyAiGuess, guessAnswers, isEconumoExport } from './guess'
import { parseCsvText } from './parse'
import type { StepId } from './prepare'

const ctx = { hasWritableAccounts: true, userCurrencyId: 'cur-eur', locale: 'de', defaultAccountName: 'Statement' }

function guess(text: string, overrides: Partial<Parameters<typeof guessAnswers>[2]> = {}) {
  const file = parseCsvText(text)
  return guessAnswers(file, profileColumns(file), { ...ctx, ...overrides })
}

const EXPORT_TEXT =
  'transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date\n' +
  'a1,Cash,USD,Food,Lunch,,,Cafe,-4.5,2026-03-04 14:05:09\n' +
  'a2,Bank,EUR,Salary,,,a;b,,1000,2026-03-05 09:00:00\n' +
  'a3,cash,EUR,Food,,,,,-1,2026-03-06 09:00:00\n' +
  'a4,Card,XYZ,,,,,,-2,2026-03-07 09:00:00\n' +
  'a5,Constructor,USD,,,,,,-3,2026-03-08 09:00:00\n'

describe('guessAnswers', () => {
  it('maps a German statement by its header names', () => {
    const a = guess('Buchungstag;Betrag;Verwendungszweck\n04.03.2026;-1.234,56;Miete\n05.03.2026;12,00;Kaffee\n')
    expect(a.date.column).toBe(0)
    expect(a.date.order).toBe('dmy')
    expect(a.amount).toMatchObject({ mode: 'single', column: 1, flip: false, sep: ',' })
    expect(a.description).toBe(2)
  })

  it('does not pick a Date column that holds text', () => {
    const a = guess('Date,Posting date,Amount\nhello,2026-03-04,1.00\nworld,2026-03-05,2.00\n')
    expect(a.date.column).toBe(1)
    expect(a.date.order).toBe('ymd')
  })

  it('switches to two amount columns for Debit and Credit', () => {
    const a = guess('Date,Description,Debit,Credit\n03/14/2026,Shop,4.50,\n03/15/2026,Pay,,100.00\n')
    expect(a.amount).toMatchObject({ mode: 'dual', inColumn: 3, outColumn: 2 })
    expect(a.date.order).toBe('mdy')
    expect(a.description).toBe(1)
  })

  it('reads money in and money out from columns named after the amount', () => {
    const a = guess('Дата;Сумма (приход);Сумма (расход);Описание\n04.03.2026;;10,00;Кафе\n05.03.2026;100,00;;ЗП\n')
    expect(a.amount).toMatchObject({ mode: 'dual', inColumn: 1, outColumn: 2, sep: ',' })
    expect(a.description).toBe(3)
  })

  it('claims a column once and matches exact names before substrings', () => {
    const a = guess('Payee name,Payee,Date,Amount\nA,B,2026-03-04,1\n')
    expect(a.extras.payee).toBe(1)
  })

  it('prefers the better synonym over the earlier column', () => {
    expect(guess('Reference,Description,Date,Amount\nX1,Lunch,2026-03-04,1\n').description).toBe(1)
  })

  it('defaults the account by whether the user has writable accounts', () => {
    const text = 'Date,Amount\n2026-03-04,1\n'
    expect(guess(text).account).toBeNull()
    expect(guess(text, { hasWritableAccounts: false }).account).toEqual({
      kind: 'new',
      name: 'Statement',
      currencyId: 'cur-eur',
    })
  })

  it('falls back to the locale date order when no date column is found', () => {
    expect(guess('x,y\na,b\n', { locale: 'en-US' }).date).toEqual({ column: null, order: 'mdy' })
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
      currencies: { cash: 'cur-usd', bank: 'cur-eur', card: 'cur-eur', constructor: 'cur-usd' },
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
  const header = ['c0', 'c1', 'c2', 'c3']
  const text = 'c0,c1,c2,c3\n2026-03-04,1,Lunch,Cafe\n'

  it('leaves a touched step alone and fills an untouched one', () => {
    const base = guess(text)
    const touched = new Set<StepId>(['date'])
    const withDate = { ...base, date: { column: 0, order: base.date.order } }
    const next = applyAiGuess(withDate, { date: 'c2', description: 'c2' }, header, touched)
    expect(next.date.column).toBe(0)
    expect(next.description).toBe(2)
  })

  it('ignores names that are not in the header', () => {
    const base = guess(text)
    const next = applyAiGuess({ ...base, description: 2 }, { description: 'Nope' }, header, new Set())
    expect(next.description).toBe(2)
  })

  it('switches to two amount columns only when both are given', () => {
    const base = guess(text)
    expect(applyAiGuess(base, { amountIn: 'c1' }, header, new Set()).amount.mode).toBe('single')
    const dual = applyAiGuess(base, { amountIn: 'c1', amountOut: 'c3' }, header, new Set())
    expect(dual.amount).toMatchObject({ mode: 'dual', inColumn: 1, outColumn: 3 })
  })

  it('never reuses a column a touched step holds, and frees a column it reassigns', () => {
    const base = { ...guess(text), description: 2, extras: { ...guess(text).extras, payee: 3 } }
    const touchedDescription = applyAiGuess(base, { payee: 'c2' }, header, new Set<StepId>(['description']))
    expect(touchedDescription.extras.payee).toBe(3)
    const moved = applyAiGuess(base, { payee: 'c2' }, header, new Set())
    expect(moved.extras.payee).toBe(2)
    expect(moved.description).toBeNull()
  })
})
