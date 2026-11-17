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
    ['a date', '2024-01-01,Coffee shop,Lunch\n2024-01-02,Tea,Snack\n'],
    ['an amount', 'Coffee shop,-12.50\nTea,-3.00\n'],
    ['a bank date and amount', '01.02.2024;-12,50;Bäckerei Müller\n02.02.2024;-3,00;Café\n'],
  ])('asks nothing when the header row holds %s', (_, text) => {
    guess(text)
    expect(suggestImportColumns).not.toHaveBeenCalled()
  })
})
