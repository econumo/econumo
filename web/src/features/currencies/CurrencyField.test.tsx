import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { CurrencyField } from './CurrencyField'

function mockPhone(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderField() {
  const onChange = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <CurrencyField label="Currency" value="cur-usd" onChange={onChange} />
    </QueryClientProvider>,
  )
  return { onChange }
}

beforeEach(() => {
  server.use(...coreHandlers())
})

it('on a desktop it is an inline searchable dropdown, like a category', async () => {
  mockPhone(false)
  const user = userEvent.setup()
  const { onChange } = renderField()
  const field = screen.getByRole('combobox', { name: 'Currency' })
  expect(field.tagName).toBe('INPUT')
  await user.click(field)
  await user.keyboard('euro')
  await user.click(await screen.findByRole('option', { name: 'EUR, €, Euro' }))
  expect(onChange).toHaveBeenCalledWith('cur-eur')
})

// a currency is required: the list offers no row that clears it
it('on a phone it opens the picker sheet, with no clear row', async () => {
  mockPhone(true)
  const user = userEvent.setup()
  const { onChange } = renderField()
  await user.click(screen.getByRole('combobox', { name: 'Currency' }))
  const sheet = await screen.findByRole('dialog', { name: 'Currency' })
  expect(within(sheet).queryByRole('option', { name: '—' })).not.toBeInTheDocument()
  await user.click(within(sheet).getByRole('option', { name: 'EUR, €, Euro' }))
  expect(onChange).toHaveBeenCalledWith('cur-eur')
})
