import { describe, expect, it } from 'vitest'
import { parseCsvText } from './parse'
import { buildUpload, prepareRows, summarize } from './prepare'
import type { PreparedRow, RowFix, WizardAnswers } from './prepare'

function answers(overrides: Partial<WizardAnswers> = {}): WizardAnswers {
  return {
    account: { kind: 'existing', accountId: 'acc-1' },
    date: { column: 0, order: 'dmy' },
    amount: { mode: 'single', column: 1, inColumn: null, outColumn: null, flip: false, sep: ',' },
    description: 2,
    extras: { payee: null, category: null, tag: null, labels: null, labelsSeparator: ';' },
    ...overrides,
  }
}

const noFixes = new Map<number, RowFix>()

const STATEMENT = 'Datum;Betrag;Text\n04.03.2026;-4,50;Kaffee\nirgendwann;12,00;Miete\n'

describe('prepareRows', () => {
  it('turns readable rows into transactions numbered as file lines (header = 1) with the raw line', () => {
    const [row] = prepareRows(parseCsvText(STATEMENT), answers(), noFixes)
    expect(row).toEqual({
      fileRow: 2,
      raw: '04.03.2026;-4,50;Kaffee',
      status: 'ok',
      reason: null,
      date: '2026-03-04',
      time: null,
      amount: '-4.50',
      account: null,
      description: 'Kaffee',
      payee: '',
      category: '',
      tag: '',
      labels: '',
    })
  })

  it('skips a row whose date cannot be read, and a date fix makes it a transaction', () => {
    const file = parseCsvText(STATEMENT)
    expect(prepareRows(file, answers(), noFixes)[1]).toMatchObject({
      fileRow: 3,
      status: 'skipped',
      reason: 'no_date',
      date: null,
      amount: '12.00',
    })
    const fixed = prepareRows(file, answers(), new Map([[3, { date: '2026-03-04' }]]))[1]
    expect(fixed).toMatchObject({ status: 'ok', reason: null, date: '2026-03-04', time: null })
  })

  it('ignores a fix for a row that reads on its own', () => {
    const rows = prepareRows(parseCsvText(STATEMENT), answers(), new Map([[2, { date: '2026-01-01', amount: '9' }]]))
    expect(rows[0]).toMatchObject({ date: '2026-03-04', amount: '-4.50' })
  })

  it('takes an amount fix as a plain signed decimal, unflipped', () => {
    const file = parseCsvText('Datum;Betrag;Text\n04.03.2026;n/a;Kaffee\n')
    const a = answers({ amount: { ...answers().amount, flip: true } })
    expect(prepareRows(file, a, noFixes)[0]).toMatchObject({ status: 'skipped', reason: 'no_amount' })
    expect(prepareRows(file, a, new Map([[2, { amount: '-4.50' }]]))[0]).toMatchObject({ status: 'ok', amount: '-4.50' })
    expect(prepareRows(file, a, new Map([[2, { amount: '4,50' }]]))[0]).toMatchObject({ reason: 'no_amount' })
  })

  it('marks a row the user skipped', () => {
    const rows = prepareRows(parseCsvText(STATEMENT), answers(), new Map([[3, 'skip' as const]]))
    expect(rows[1]).toMatchObject({ status: 'skipped', reason: 'user_skipped' })
  })

  it('drops a skip once the row reads on its own', () => {
    const rows = prepareRows(parseCsvText(STATEMENT), answers(), new Map([[2, 'skip' as const]]))
    expect(rows[0]).toMatchObject({ status: 'ok', reason: null })
  })

  it('skips a footer line that has no date', () => {
    const rows = prepareRows(parseCsvText(`${STATEMENT}Closing balance;1.234,56\n`), answers(), noFixes)
    expect(rows[2]).toMatchObject({ fileRow: 4, status: 'skipped', reason: 'no_date', amount: '1234.56' })
  })

  it('leaves fully blank rows out entirely', () => {
    const rows = prepareRows(parseCsvText(`${STATEMENT};;\n\n`), answers(), noFixes)
    expect(rows).toHaveLength(2)
  })

  it('needs at least one of the two amount cells in two-column mode', () => {
    const file = parseCsvText('Date,In,Out\n2026-03-04,,\n2026-03-05,,4.50\n2026-03-06,10,\n')
    const a = answers({
      date: { column: 0, order: 'ymd' },
      amount: { mode: 'dual', column: null, inColumn: 1, outColumn: 2, flip: false, sep: '.' },
      description: null,
    })
    const rows = prepareRows(file, a, noFixes)
    expect(rows.map((r) => [r.reason, r.amount])).toEqual([
      ['no_amount', null],
      [null, '-4.50'],
      [null, '10'],
    ])
  })

  it('reads the account per row and skips rows without one', () => {
    const file = parseCsvText('d,a,acc\n2026-03-04,1, Cash \n2026-03-05,2,\n')
    const a = answers({
      account: { kind: 'column', column: 2, currencies: {} },
      date: { column: 0, order: 'ymd' },
      amount: { ...answers().amount, sep: '.' },
      description: null,
    })
    const rows = prepareRows(file, a, noFixes)
    expect(rows.map((r) => [r.account, r.reason])).toEqual([
      ['Cash', null],
      [null, 'no_account'],
    ])
  })

  it('skips every row when no date or amount column is chosen', () => {
    const file = parseCsvText(STATEMENT)
    expect(prepareRows(file, answers({ date: { column: null, order: 'dmy' } }), noFixes)[0].reason).toBe('no_date')
    const noAmount = answers({ amount: { ...answers().amount, column: null } })
    expect(prepareRows(file, noAmount, noFixes)[0].reason).toBe('no_amount')
  })
})

function row(overrides: Partial<PreparedRow>): PreparedRow {
  return {
    fileRow: 1,
    raw: '',
    status: 'ok',
    reason: null,
    date: '2026-03-04',
    time: null,
    amount: '1',
    account: null,
    description: '',
    payee: '',
    category: '',
    tag: '',
    labels: '',
    ...overrides,
  }
}

describe('summarize', () => {
  it('adds money in and out exactly and spans the dates of the transactions', () => {
    const s = summarize([
      row({ amount: '0.10', date: '2026-03-05' }),
      row({ amount: '0.20', date: '2026-03-01' }),
      row({ amount: '-0.10', date: '2026-03-09' }),
      row({ amount: '-0.20' }),
      row({ status: 'skipped', reason: 'no_date', date: null, amount: '100' }),
      row({ status: 'skipped', reason: 'no_date', date: '2027-01-01', amount: null }),
      row({ status: 'skipped', reason: 'user_skipped' }),
    ])
    expect(s).toEqual({
      total: 7,
      skipped: 3,
      byReason: { no_date: 2, user_skipped: 1 },
      firstDate: '2026-03-01',
      lastDate: '2026-03-09',
      moneyIn: '0.30',
      moneyOut: '0.30',
    })
  })

  it('reports no dates and zero money for nothing to import', () => {
    expect(summarize([])).toEqual({
      total: 0,
      skipped: 0,
      byReason: {},
      firstDate: null,
      lastDate: null,
      moneyIn: '0',
      moneyOut: '0',
    })
  })
})

describe('buildUpload', () => {
  it('uploads only transactions, keeps their file rows aligned and writes the time of day', () => {
    const rows = [
      row({ fileRow: 2, date: '2026-03-04', time: '14:05:09', amount: '-4.5', description: 'Lunch' }),
      row({ fileRow: 3, status: 'skipped', reason: 'no_date', date: null }),
      row({ fileRow: 4, date: '2026-03-05', amount: '10', description: 'Pay' }),
    ]
    const upload = buildUpload(rows, answers())
    expect(upload.header).toEqual(['date', 'amount', 'description'])
    expect(upload.rows).toEqual([
      ['2026-03-04 14:05:09', '-4.5', 'Lunch'],
      ['2026-03-05', '10', 'Pay'],
    ])
    expect(upload.fileRows).toEqual([2, 4])
    expect(upload.mapping).toEqual({
      account: null,
      date: 'date',
      amount: 'amount',
      amountInflow: null,
      amountOutflow: null,
      category: null,
      description: 'description',
      payee: null,
      tag: null,
      labels: null,
    })
    expect(upload.fields).toEqual({ accountId: 'acc-1', labelsSeparator: ';' })
  })

  it('sends the account per row and every mapped extra', () => {
    const a = answers({
      account: { kind: 'column', column: 5, currencies: {} },
      description: null,
      extras: { payee: 3, category: 4, tag: 6, labels: 7, labelsSeparator: '|' },
    })
    const upload = buildUpload(
      [row({ account: 'Cash', payee: 'Cafe', category: 'Food', tag: 'Trip', labels: 'a|b' })],
      a,
    )
    expect(upload.header).toEqual(['date', 'amount', 'account', 'payee', 'category', 'tag', 'labels'])
    expect(upload.rows).toEqual([['2026-03-04', '1', 'Cash', 'Cafe', 'Food', 'Trip', 'a|b']])
    expect(upload.mapping).toMatchObject({ account: 'account', payee: 'payee', category: 'category', tag: 'tag', labels: 'labels', description: null })
    expect(upload.fields).toEqual({ labelsSeparator: '|' })
  })

  it('leaves the account id of a new account to the caller', () => {
    const a = answers({ account: { kind: 'new', name: 'Bank', currencyId: 'cur' } })
    expect(buildUpload([row({})], a).fields).toEqual({ labelsSeparator: ';' })
  })
})
