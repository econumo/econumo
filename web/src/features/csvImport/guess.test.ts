import { describe, expect, it } from 'vitest'
import { profileColumns } from './columns'
import { applyAiGuess, guessAnswers, isEconumoExport, redetectFormats } from './guess'
import { parseCsvText } from './parse'
import type { StepId } from './prepare'

const ctx = { writableAccountIds: ['acc-1', 'acc-2'], userCurrencyId: 'cur-eur', locale: 'de' }

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
  it('maps a German statement from its values', () => {
    const a = guess(
      'Buchungstag;Betrag;Verwendungszweck\n04.03.2026;-1.234,56;Miete März\n05.03.2026;12,00;Kaffee am Bahnhof\n06.03.2026;-3,10;Bäckerei Müller\n',
    )
    expect(a.date.column).toBe(0)
    expect(a.date.order).toBe('dmy')
    expect(a.amount).toMatchObject({ mode: 'single', column: 1, flip: false, sep: ',' })
    expect(a.description).toBe(2)
  })

  it('never reads a header word: the same file headed by anything maps the same', () => {
    const a = guess('x;y;z\n04.03.2026;-1.234,56;Miete\n05.03.2026;12,00;Kaffee\n')
    expect(a.date.column).toBe(0)
    expect(a.amount).toMatchObject({ mode: 'single', column: 1 })
    expect(a.description).toBe(2)
  })

  it('does not pick a column headed Date that holds text', () => {
    const a = guess('Date,Posting date,Amount\nhello,2026-03-04,1.00\nworld,2026-03-05,2.00\n')
    expect(a.date.column).toBe(1)
    expect(a.date.order).toBe('ymd')
  })

  it('takes the date column with the most readable dates, else the leftmost', () => {
    expect(guess('a,b,c\n2026-03-04,,1\n2026-03-05,2026-03-05,2\n2026-03-06,2026-03-06,3\n').date.column).toBe(0)
    expect(guess('a,b,c\n2026-03-04,2026-03-04,1\nsoon,2026-03-05,2\n2026-03-06,2026-03-06,3\n2026-03-07,2026-03-07,4\n2026-03-08,2026-03-08,5\n').date.column).toBe(1)
  })

  it('reads a US statement with separate debit and credit columns, the busier one as spending', () => {
    const a = guess('Date,Description,Debit,Credit\n03/14/2026,Coffee Shop,4.50,\n03/15/2026,Payroll,,100.00\n03/16/2026,Grocery Store,20.00,\n')
    expect(a.amount).toMatchObject({ mode: 'dual', inColumn: 3, outColumn: 2 })
    expect(a.date.order).toBe('mdy')
    expect(a.description).toBe(1)
  })

  it('takes the column with negatives as money out', () => {
    const a = guess('d,in,out\n2026-03-04,10.00,\n2026-03-05,,-4.50\n2026-03-06,12.00,\n')
    expect(a.amount).toMatchObject({ mode: 'dual', inColumn: 1, outColumn: 2 })
  })

  it('leaves the two sides for the step to ask when nothing tells them apart', () => {
    const a = guess('d,p,q\n2026-03-04,4.50,\n2026-03-05,,100.00\n')
    expect(a.amount).toMatchObject({ mode: 'dual', inColumn: null, outColumn: null })
  })

  it('prefers a signed column and skips a running balance', () => {
    const a = guess('d,balance,amount,memo\n2026-03-04,100.00,100.00,Opening deposit\n2026-03-05,95.50,-4.50,Coffee\n2026-03-06,75.50,-20.00,Groceries\n2026-03-07,175.50,100.00,Salary\n')
    expect(a.amount).toMatchObject({ mode: 'single', column: 2 })
  })

  it('skips a balance in a file sorted newest first', () => {
    const a = guess('d,amount,balance\n2026-03-07,100.00,175.50\n2026-03-06,20.00,75.50\n2026-03-05,4.50,55.50\n2026-03-04,51.00,51.00\n')
    expect(a.amount).toMatchObject({ mode: 'single', column: 1 })
  })

  it('maps a Russian file: the longest varied text is the description, repeated short names the category', () => {
    const a = guess(
      'Дата;Сумма;Категория;Описание;Получатель\n' +
        '04.03.2026;-350,00;Кафе;Обед с коллегами в центре;Шоколадница\n' +
        '05.03.2026;-1 200,00;Продукты;Закупка на неделю в гипермаркете;Ашан\n' +
        '06.03.2026;-90,00;Кафе;Кофе по дороге на работу;Кофемания\n' +
        '07.03.2026;-560,00;Продукты;Овощи и фрукты на рынке;Рынок\n' +
        '08.03.2026;-120,00;Кафе;Пирожное к чаю после работы;Буше\n' +
        '09.03.2026;-800,00;Продукты;Молочные продукты и хлеб;Пятёрочка\n',
    )
    expect(a.date).toEqual({ column: 0, order: 'dmy' })
    expect(a.amount).toMatchObject({ mode: 'single', column: 1, sep: ',' })
    expect(a.description).toBe(3)
    expect(a.extras).toMatchObject({ category: 2, payee: 4, tag: null, labels: null })
  })

  it('takes the repeating column with fewer names for the account, the other for the category', () => {
    const rows = ['Card,Food', 'Card,Fuel', 'Cash,Food', 'Card,Rent', 'Cash,Fuel', 'Card,Food', 'Card,Rent', 'Cash,Food', 'Card,Fuel']
    const a = guess('d,v,acct,cat,memo\n' + rows.map((r, i) => `2026-03-0${i + 1},-${i + 1}.00,${r},paid at shop ${'abcdefghi'[i]}`).join('\n') + '\n')
    expect(a.extras.category).toBe(3)
    expect(a.description).toBe(4)
  })

  it('never guesses a constant column or a bank reference', () => {
    const a = guess('d,v,acct,ref\n2026-03-04,-1.00,Main,AB1234567890\n2026-03-05,-2.00,Main,AB1234567891\n2026-03-06,-3.00,Main,AB1234567892\n')
    expect(a.extras.category).toBeNull()
    expect(a.extras.payee).toBeNull()
    expect(a.description).toBeNull()
  })

  describe('the account column of a file with no currency', () => {
    const text =
      'Date,Amount,Account,Description\n2026-11-01,-12.50,Everyday Card,Coffee\n2026-11-02,-40.00,Everyday Card,Groceries\n' +
      '2026-11-03,2500.00,Cash,Salary\n2026-11-04,-8.00,Travel Wallet,Bus\n2026-11-05,-3.20,Travel Wallet,Snack\n'

    it('keeps a lone repeating column as the category unless it names the user accounts', () => {
      const a = guess(text)
      expect(a.account).toBeNull()
      expect(a.extras.category).toBe(2)
      const named = guess(text, { writableAccountNames: ['cash', 'Other'] })
      expect(named.account).toMatchObject({ kind: 'column', column: 2, currencyColumn: null })
      expect(named.extras.category).toBeNull()
    })

    it('prefers the column whose values are the names of existing accounts', () => {
      const rows = ['Food,Main', 'Fuel,main', 'Food,Savings', 'Fuel,Main', 'Food,Main', 'Fuel,Main', 'Food,Savings', 'Fuel,Main']
      const f = 'd,v,kind,acct,memo\n' + rows.map((r, i) => `2026-03-0${i + 1},-${i + 1}.00,${r},paid at shop ${'abcdefgh'[i]}`).join('\n') + '\n'
      const a = guess(f, { writableAccountIds: ['acc-1', 'acc-2'], writableAccountNames: ['Main', 'Savings'] })
      expect(a.account).toMatchObject({ kind: 'column', column: 3 })
      expect(a.extras.category).toBe(2)
    })

    it('keeps the only account when no column names it', () => {
      const a = guess(text, { writableAccountIds: ['acc-1'], writableAccountNames: ['Wallet'] })
      expect(a.account).toEqual({ kind: 'existing', accountId: 'acc-1' })
    })
  })

  it('defaults the account by how many writable accounts the user has', () => {
    const text = 'Date,Amount\n2026-03-04,1\n'
    expect(guess(text).account).toBeNull()
    expect(guess(text, { writableAccountIds: ['acc-1'] }).account).toEqual({ kind: 'existing', accountId: 'acc-1' })
    expect(guess(text, { writableAccountIds: [] }).account).toEqual({
      kind: 'new',
      name: '',
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
    const base = guessAnswers(f, profileColumns(f), ctx)
    expect(base.date).toEqual({ column: 0, order: 'dmy' })
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
    ...guessAnswers(f, profileColumns(f), ctx),
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
