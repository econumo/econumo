import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { CurrencySelect, fuzzyMatch, fullCurrencyLabel } from './CurrencySelect'

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

it('fuzzy-matches subsequences like the Vue component', () => {
  expect(fuzzyMatch('US Dollar', 'usd')).toBe(true)
  expect(fuzzyMatch('Euro', 'eo')).toBe(true)
  expect(fuzzyMatch('Euro', 'x')).toBe(false)
})

it('builds the deduped option label', () => {
  expect(fullCurrencyLabel({ id: '1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2, scope: 'global' as const, isArchived: 0 as const, isHidden: 0 as const })).toBe('USD, $, US Dollar')
  expect(fullCurrencyLabel({ id: '2', code: 'X', name: 'X', symbol: 'X', fractionDigits: 2, scope: 'global' as const, isArchived: 0 as const, isHidden: 0 as const })).toBe('X')
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
