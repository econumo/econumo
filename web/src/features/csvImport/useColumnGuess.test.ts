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
  ])('asks when a date or money column is headed by a word for it (%s)', (_, text) => {
    guess(text)
    expect(suggestImportColumns).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['a date', '2024-01-01,Coffee shop,Lunch\n2024-01-02,Tea,Snack\n'],
    ['an amount', 'Coffee shop,-12.50\nTea,-3.00\n'],
    ['a bank date and amount', '01.02.2024;-12,50;Bäckerei Müller\n02.02.2024;-3,00;Café\n'],
    ['an amount with a currency', '2 Mar 2024,-12.50 EUR,Coffee\n2024-03-03,-3.00,Tea\n2024-03-04,-4.00,Cake\n'],
    ['an unreadable transaction', 'someday,n/a,medical memo\n2024-01-02,-10,Coffee\n2024-01-03,-5,Tea\n'],
    ['words that only contain a header word', 'update fee,summary,medical memo\n2024-01-02,-10,Coffee\n2024-01-03,-5,Tea\n'],
  ])('asks nothing when the header row holds %s', (_, text) => {
    guess(text)
    expect(suggestImportColumns).not.toHaveBeenCalled()
  })
})
