import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { suggestImportColumns } from '@/api/transaction'
import { profileColumns } from './columns'
import { parseCsvText } from './parse'
import { useColumnGuess } from './useColumnGuess'

vi.mock('@/api/transaction', () => ({ suggestImportColumns: vi.fn(async () => ({})) }))

const guess = (text: string) => renderHook(() => useColumnGuess(profileColumns(parseCsvText(text)), vi.fn()))

describe('useColumnGuess', () => {
  beforeEach(() => vi.mocked(suggestImportColumns).mockClear())

  it('sends the header names and column types', () => {
    guess('Date,Amount,Memo\n2024-01-02,-10,Coffee\n2024-01-03,-5,Tea\n')
    expect(suggestImportColumns).toHaveBeenCalledWith(
      [
        { name: 'Date', type: 'date', negatives: false },
        { name: 'Amount', type: 'number', negatives: true },
        { name: 'Memo', type: 'text', negatives: false },
      ],
      expect.any(AbortSignal),
    )
  })

  it.each([
    ['German', 'Buchungstag;Betrag (EUR);Verwendungszweck\n01.02.2024;-12,50;x\n02.02.2024;-3,00;y\n'],
    ['a money-in and money-out pair', 'Posted Date,Debit,Credit,Memo\n2024-01-02,10,,x\n2024-01-03,,5,y\n'],
    ['Chinese', '交易日期,金额,摘要\n2024-01-02,-10,x\n2024-01-03,-5,y\n'],
    ['a trailing delimiter', 'Buchungstag;Betrag;Verwendungszweck;\n01.02.2024;-12,50;x;\n02.02.2024;-3,00;y;\n'],
    ['a repeated header', 'Date,Amount,Amount,Memo 2\n2024-01-02,-10,1,x\n2024-01-03,-5,2,y\n'],
    ['headers in no known language', 'Ngày,Số tiền,Ghi chú\n2024-01-02,-10,x\n2024-01-03,-5,y\n'],
  ])('asks when the header row does not read like a transaction (%s)', (_, text) => {
    guess(text)
    expect(suggestImportColumns).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['a date', '2024-01-01,Coffee shop,Lunch\n2024-01-02,Tea,Snack\n'],
    ['an amount', 'Coffee shop,-12.50\nTea,-3.00\n'],
    ['a bank date and amount', '01.02.2024;-12,50;Bäckerei Müller\n02.02.2024;-3,00;Café\n'],
    ['an amount with a currency', '2 Mar 2024,-12.50 EUR,Coffee\n2024-03-03,-3.00,Tea\n2024-03-04,-4.00,Cake\n'],
    ['a value its column repeats', '2 Mar 2024,Groceries,Tesco\n2024-03-03,Groceries,Tesco\n2024-03-04,Fuel,Shell\n'],
    ['a value its column repeats, in another case', 'foo;-1,00;rewe\nbar;-2,00;REWE\nbaz;-3,00;Aldi\n'],
    ['a bank reference', 'DE89370400440532013000,someday,Coffee\nGB29NWBK60161331926819,2024-01-02,Tea\nFR1420041010050500013M02606,2024-01-03,Cake\n'],
    ['nothing over a date column', ',Memo\n2024-01-02,Coffee\n2024-01-03,Tea\n'],
    ['nothing over an amount column', 'Memo,\nCoffee,-10\nTea,-5\n'],
  ])('asks nothing when the header row holds %s', (_, text) => {
    guess(text)
    expect(suggestImportColumns).not.toHaveBeenCalled()
  })

  // Without word lists, a first transaction whose date and amount cells are both unreadable
  // words looks like any header by content. Its other cells reach the AI; this pins that limit.
  it.each([
    ['unknown,Amount,Coffee\n2026-01-03,-5.00,Tea\n2026-01-04,-6.00,Cake\n'],
    ['Date,n/a,Coffee\n2026-01-03,-5.00,Tea\n2026-01-04,-6.00,Cake\n'],
    ['someday,n/a,medical memo\n2024-01-02,-10,Coffee\n2024-01-03,-5,Tea\n'],
    ['update fee,summary,medical memo\n2024-01-02,-10,Coffee\n2024-01-03,-5,Tea\n'],
  ])('cannot tell a first row of unreadable words from a header: %s', (text) => {
    guess(text)
    expect(suggestImportColumns).toHaveBeenCalledTimes(1)
  })
})
