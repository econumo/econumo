import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportStep } from './ImportStep'

const CSV = 'Account,Date,Amount\nCash,2026-01-02,-5.5\n'

function renderStep(onSkip = vi.fn().mockResolvedValue(undefined)) {
  server.use(...coreHandlers())
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportStep onDone={vi.fn()} onSkip={onSkip} />
    </QueryClientProvider>,
  )
  return screen.getByRole('button', { name: 'Choose a CSV file' })
}

const files = (file: File) => ({ dataTransfer: { files: [file], types: ['Files'], dropEffect: 'none' } })

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('highlights the zone while a file is dragged over it', () => {
  const zone = renderStep()
  const file = new File([CSV], 'bank.csv', { type: 'text/csv' })
  fireEvent.dragOver(zone, files(file))
  expect(zone).toHaveAttribute('data-dragging', 'true')
  fireEvent.dragLeave(zone, { relatedTarget: document.body })
  expect(zone).not.toHaveAttribute('data-dragging')
})

it('a dropped CSV opens the import dialog with the file already parsed', async () => {
  const zone = renderStep()
  fireEvent.drop(zone, files(new File([CSV], 'bank.csv', { type: 'text/csv' })))
  expect(await screen.findByText(/Map the columns from your CSV file/)).toBeInTheDocument()
  expect(screen.getByText('bank.csv')).toBeInTheDocument()
})

it('ignores a dropped file that is not a CSV, with a hint', () => {
  const zone = renderStep()
  fireEvent.drop(zone, files(new File(['x'], 'photo.png', { type: 'image/png' })))
  expect(screen.getByRole('alert')).toHaveTextContent('Only CSV files can be imported here.')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('a click on the zone opens the dialog with the file picker', async () => {
  const zone = renderStep()
  await userEvent.setup().click(zone)
  expect(await screen.findByLabelText('CSV File')).toBeInTheDocument()
  expect(screen.queryByText(/Map the columns/)).not.toBeInTheDocument()
})

it('the footer has Skip on the left and "I don\'t have one" on the right, both skipping the step, and no Import button', async () => {
  const onSkip = vi.fn().mockResolvedValue(undefined)
  renderStep(onSkip)
  const user = userEvent.setup()
  const skipButton = screen.getByRole('button', { name: 'Skip' })
  const none = screen.getByRole('button', { name: "I don't have one" })
  expect(skipButton.parentElement).toBe(none.parentElement)
  expect(skipButton.compareDocumentPosition(none) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Import' })).not.toBeInTheDocument()
  await user.click(skipButton)
  await user.click(none)
  expect(onSkip).toHaveBeenCalledTimes(2)
})
