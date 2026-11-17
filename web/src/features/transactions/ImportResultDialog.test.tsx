import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { onlineManager } from '@tanstack/react-query'
import { ImportResultDialog } from './ImportResultDialog'
import type { ImportResult } from '@/features/csvImport/upload'

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

const renderResult = (result: ImportResult) =>
  render(<ImportResultDialog open result={result} onClose={() => {}} />)

it('full success: title + pluralized imported count', () => {
  renderResult({ imported: 3, failed: 0, errors: [], failedChunks: [] })
  expect(screen.getByText('Import complete')).toBeInTheDocument()
  expect(screen.getByText('3 transactions imported')).toBeInTheDocument()
  expect(screen.queryByText('Error details')).not.toBeInTheDocument()
})

it('partial success: both counts and error rows formatting', () => {
  renderResult({
    imported: 1,
    failed: 3,
    errors: [
      { message: 'Invalid amount format', rows: [4] },
      { message: "Invalid date format 'x'", rows: [2, 3] },
    ],
    failedChunks: [],
  })
  expect(screen.getByText('Import completed with errors')).toBeInTheDocument()
  expect(screen.getByText('1 transaction imported')).toBeInTheDocument()
  expect(screen.getByText('3 transactions failed')).toBeInTheDocument()
  expect(screen.getByText('Error details')).toBeInTheDocument()
  expect(screen.getByText(/Row 4/)).toBeInTheDocument()
  expect(screen.getByText(/Rows 2, 3/)).toBeInTheDocument()
})

it('total failure: error title, long row lists truncated, extra error groups counted', () => {
  const manyRows = Array.from({ length: 14 }, (_, i) => i + 2)
  const errors = [
    { message: 'e1', rows: manyRows },
    { message: 'e2', rows: [2] },
    { message: 'e3', rows: [3] },
    { message: 'e4', rows: [4] },
    { message: 'e5', rows: [5] },
    { message: 'e6', rows: [6] },
    { message: 'e7', rows: [7] },
  ]
  renderResult({ imported: 0, failed: 20, errors, failedChunks: [] })
  expect(screen.getByText('Import failed')).toBeInTheDocument()
  // first 10 rows + "+4 more"
  expect(screen.getByText(/Rows 2, 3, 4, 5, 6, 7, 8, 9, 10, 11 \+4 more/)).toBeInTheDocument()
  // only 5 error groups shown, remainder summarized
  expect(screen.queryByText('e6')).not.toBeInTheDocument()
  expect(screen.getByText('and 2 more error(s)')).toBeInTheDocument()
})

it('rows the file could not give are explained in plain words', () => {
  renderResult({
    imported: 1,
    failed: 1,
    errors: [{ message: 'transactions.import_wizard.skipped.no_date', rows: [3] }],
    failedChunks: [],
  })
  expect(screen.getByText('No readable date')).toBeInTheDocument()
})

it('Try the rest again is offered only while some chunks failed, and calls back', async () => {
  const onRetry = vi.fn(async () => {})
  const result: ImportResult = { imported: 500, failed: 100, errors: [], failedChunks: [1] }
  const { rerender } = render(<ImportResultDialog open result={result} onClose={() => {}} onRetry={onRetry} />)
  await userEvent.click(screen.getByRole('button', { name: 'Try the rest again' }))
  expect(onRetry).toHaveBeenCalledTimes(1)
  rerender(<ImportResultDialog open result={{ ...result, failedChunks: [] }} onClose={() => {}} onRetry={onRetry} />)
  expect(screen.queryByRole('button', { name: 'Try the rest again' })).not.toBeInTheDocument()
})

it('Try the rest again is disabled offline', () => {
  onlineManager.setOnline(false)
  try {
    render(<ImportResultDialog open result={{ imported: 0, failed: 5, errors: [], failedChunks: [0] }} onClose={() => {}} onRetry={async () => {}} />)
    expect(screen.getByRole('button', { name: 'Try the rest again' })).toBeDisabled()
  } finally {
    onlineManager.setOnline(true)
  }
})
