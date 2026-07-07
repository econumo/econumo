import { render, screen } from '@testing-library/react'
import { ImportResultDialog } from './ImportResultDialog'
import type { AggregatedImportResult } from './importCsv'

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

const renderResult = (result: AggregatedImportResult) =>
  render(<ImportResultDialog open result={result} onClose={() => {}} />)

it('full success: title + pluralized imported count', () => {
  renderResult({ imported: 3, failed: 0, errors: [] })
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
  })
  expect(screen.getByText('Import completed with errors')).toBeInTheDocument()
  expect(screen.getByText('1 transaction(s) imported')).toBeInTheDocument()
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
  renderResult({ imported: 0, failed: 20, errors })
  expect(screen.getByText('Import failed')).toBeInTheDocument()
  // first 10 rows + "+4 more"
  expect(screen.getByText(/Rows 2, 3, 4, 5, 6, 7, 8, 9, 10, 11 \+4 more/)).toBeInTheDocument()
  // only 5 error groups shown, remainder summarized
  expect(screen.queryByText('e6')).not.toBeInTheDocument()
  expect(screen.getByText('and 2 more error(s)')).toBeInTheDocument()
})
