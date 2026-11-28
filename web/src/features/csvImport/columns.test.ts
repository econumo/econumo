import { describe, expect, it } from 'vitest'
import { headerHasValues, profileColumns } from './columns'
import { parseCsvText } from './parse'

describe('profileColumns', () => {
  it('types dates, numbers, text and empty columns', () => {
    const p = parseCsvText('Datum;Betrag;Name;Leer\n01.02.2026;-4,50;Shop;\n02.02.2026;10,00;Cafe;\n')
    const cols = profileColumns(p)
    expect(cols.map((c) => c.type)).toEqual(['date', 'number', 'text', 'empty'])
    expect(cols[1].negatives).toBe(true)
    expect(cols[0].negatives).toBe(false)
    expect(cols[0].index).toBe(0)
    expect(cols[2].name).toBe('Name')
  })

  it('caps samples at 3 distinct values and truncates long ones', () => {
    const long = 'x'.repeat(40)
    const p = parseCsvText(`n\na\na\nb\n${long}\nd\n`)
    expect(profileColumns(p)[0].samples).toEqual(['a', 'b', `${'x'.repeat(25)}…`])
  })

  it('needs an 80 percent majority', () => {
    const rows = ['1', '2', '3', '4', 'abc'].join('\n')
    expect(profileColumns(parseCsvText(`n\n${rows}\n`))[0].type).toBe('number')
    expect(profileColumns(parseCsvText('n\n1\n2\nabc\ndef\n'))[0].type).toBe('text')
  })

  it('does not type long digit references as dates', () => {
    const p = parseCsvText('ref;amt\n1234567890;1\n1234567891;2\n1234567892;3\n')
    expect(profileColumns(p)[0].type).toBe('number')
  })

  it('types 8-digit compact dates as dates but not other 8-digit numbers', () => {
    expect(profileColumns(parseCsvText('d;x\n20260201;1\n20260215;2\n'))[0].type).toBe('date')
    expect(profileColumns(parseCsvText('d;x\n12345678;1\n12345679;2\n'))[0].type).toBe('number')
  })

  it('marks ids, long codes and long reference numbers as internal, and nothing a person would pick', () => {
    const p = parseCsvText(
      'id;ref;code;memo;amount;date;mixed\n' +
        '01a1be03-f8df-7603-b2b1-2c3d4e5f6a7b;123456789012;TXN-2026-000123;Coffee;-4.50;2026-01-02;REF123456789\n' +
        'b298d99e-4fb1-42ca-97a2-9f00d01aeb19;123456789013;TXN-2026-000124;Rent;-900;2026-01-03;Lunch\n',
    )
    expect(profileColumns(p).map((c) => c.internal)).toEqual([true, true, true, false, false, false, false])
  })

  it('never marks a date the reader could not parse as internal', () => {
    const p = parseCsvText('when;also\n02-Jan-2026;2026.01.02.\n03-Jan-2026;2026.01.03.\n')
    expect(profileColumns(p).map((c) => c.internal)).toEqual([false, false])
  })
})

describe('profileColumns counts', () => {
  it('counts filled cells, different values in any case, their average length and currency codes', () => {
    const p = profileColumns(parseCsvText('Wallet,Cur,Memo\nCash,CAD,ab\ncash,CAD,abcd\nVisa,eur,\n'))
    expect(p[0]).toMatchObject({ filled: 3, distinct: 2, averageLength: 4, codes: false })
    expect(p[1]).toMatchObject({ codes: true })
    expect(p[2]).toMatchObject({ filled: 2, averageLength: 3, codes: false })
  })
})

describe('header checks', () => {
  it('notes a header that is also one of its own column\'s values', () => {
    const p = profileColumns(parseCsvText('Groceries,Memo\ngroceries,a\nFuel,b\n'))
    expect(p.map((c) => c.headerInValues)).toEqual([true, false])
  })

  it('reads a header as a value only when it is a date or a number with at most a currency', () => {
    const has = (header: string) => headerHasValues(profileColumns(parseCsvText(`${header}\nx\n`)))
    expect(has('2024-01-02')).toBe(true)
    expect(has('-12.50 EUR')).toBe(true)
    expect(has('€12,50')).toBe(true)
    expect(has('Column 3')).toBe(false)
    expect(has('Amount (2)')).toBe(false)
    expect(has('Memo 2')).toBe(false)
  })
})
