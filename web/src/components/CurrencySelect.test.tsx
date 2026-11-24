import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { CurrencySelect, fullCurrencyLabel } from './CurrencySelect'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
})

it('builds the deduped option label', () => {
  expect(fullCurrencyLabel({ id: '1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 })).toBe('USD, $, US Dollar')
  expect(fullCurrencyLabel({ id: '2', code: 'X', name: 'X', symbol: 'X', fractionDigits: 2 })).toBe('X')
})

it('shows the selected code and picks a currency from the list', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CurrencySelect value="cur-usd" onChange={onChange} aria-label="Currency" />
    </QueryClientProvider>,
  )
  await vi.waitFor(() => expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveTextContent('USD'))
  await user.click(screen.getByRole('combobox', { name: 'Currency' }))
  await user.click(await screen.findByText('EUR, €, Euro'))
  expect(onChange).toHaveBeenCalledWith('cur-eur')
})

it('search skips letters and matches the code, symbol or name', async () => {
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CurrencySelect value="cur-usd" onChange={() => {}} aria-label="Currency" />
    </QueryClientProvider>,
  )
  await vi.waitFor(() => expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveTextContent('USD'))
  await user.click(screen.getByRole('combobox', { name: 'Currency' }))
  await user.keyboard('ero')
  expect(await screen.findByText('EUR, €, Euro')).toBeInTheDocument()
  expect(screen.queryByText('USD, $, US Dollar')).not.toBeInTheDocument()
  await user.keyboard('{Backspace}{Backspace}{Backspace}$')
  expect(await screen.findByText('USD, $, US Dollar')).toBeInTheDocument()
  expect(screen.queryByText('EUR, €, Euro')).not.toBeInTheDocument()
})

it('the clear button empties the search and lists every currency again', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CurrencySelect value="cur-usd" onChange={onChange} aria-label="Currency" />
    </QueryClientProvider>,
  )
  await vi.waitFor(() => expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveTextContent('USD'))
  await user.click(screen.getByRole('combobox', { name: 'Currency' }))
  expect(screen.queryByRole('button', { name: 'Clear search' })).not.toBeInTheDocument()
  await user.keyboard('ero')
  expect(screen.queryByText('USD, $, US Dollar')).not.toBeInTheDocument()
  const clear = screen.getByRole('button', { name: 'Clear search' })
  expect(clear).toHaveAttribute('tabindex', '-1')
  await user.click(clear)
  expect(await screen.findByText('USD, $, US Dollar')).toBeInTheDocument()
  expect(screen.getByText('EUR, €, Euro')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Clear search' })).not.toBeInTheDocument()
  expect(onChange).not.toHaveBeenCalled()
})
