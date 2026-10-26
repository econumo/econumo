import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { CurrencyPickerDialog } from './CurrencyPickerDialog'

function mockPhone(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPicker() {
  const onPick = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <CurrencyPickerDialog open title="Currency" value={null} onClose={() => {}} onPick={onPick} />
    </QueryClientProvider>,
  )
  return { onPick }
}

beforeEach(() => {
  server.use(...coreHandlers())
})

// the sheet rests on the keyboard, so its pinned bottom row sits right above the keys
it('on a phone the search sits below the list, takes focus, and Enter picks the first match', async () => {
  mockPhone(true)
  const user = userEvent.setup()
  const { onPick } = renderPicker()
  const option = await screen.findByRole('option', { name: /Euro/ })
  const search = screen.getByRole('textbox', { name: 'Search' })
  expect(option.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(search).toHaveFocus()

  await user.keyboard('eur{Enter}')
  expect(onPick).toHaveBeenCalledWith('cur-eur')
})

it('on a desktop the search stays above the list', async () => {
  mockPhone(false)
  renderPicker()
  const option = await screen.findByRole('option', { name: /Euro/ })
  const search = screen.getByRole('combobox')
  expect(search.compareDocumentPosition(option) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
