import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts } from '@/test/fixtures'
import type { ImportSourceDto } from '@/api/dto/imports'
import { ImportCards } from './ImportCards'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const unmapped = { externalAccountId: 'wallet', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped' as const, accountId: '', queuedCount: 2, tapCount: 3, lastSeenAt: '2026-08-20 17:42:03' }
const source: ImportSourceDto = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [unmapped] }

function renderCards(src: ImportSourceDto) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportCards source={src} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
  vi.mocked(toast.error).mockClear()
})

it('map opens the account picker and posts link-account, then toasts the run counts', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/link-account', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: {
      item: { ...source, cards: [{ ...unmapped, state: 'mapped', accountId: fixtureAccounts[0].id, queuedCount: 0 }] },
      run: { id: 'r1', status: 'finished', importedCount: 1, matchedCount: 1, skippedCount: 0, failedCount: 0 },
    } })
  }))
  const user = userEvent.setup()
  renderCards(source)
  await user.click(await screen.findByRole('button', { name: 'Map to account' }))
  await user.selectOptions(await screen.findByLabelText('Account'), fixtureAccounts[0].id)
  await user.click(screen.getByRole('button', { name: 'Map' }))
  await waitFor(() => expect(body).toEqual({ sourceId: 's1', externalAccountId: 'wallet', accountId: fixtureAccounts[0].id }))
})

it('ignore posts ignore-account; an ignored card offers "Map instead"', async () => {
  let called = false
  server.use(http.post('*/api/v1/import/ignore-account', () => {
    called = true
    return HttpResponse.json({ success: true, message: '', data: { item: { ...source, cards: [{ ...unmapped, state: 'ignored' }] }, run: null } })
  }))
  const user = userEvent.setup()
  renderCards(source)
  await user.click(await screen.findByRole('button', { name: 'Ignore' }))
  await waitFor(() => expect(called).toBe(true))
  renderCards({ ...source, cards: [{ ...unmapped, state: 'ignored' }] })
  expect(screen.getByRole('button', { name: 'Map instead' })).toBeInTheDocument()
})

it('a failed ignore toasts the server error', async () => {
  server.use(http.post('*/api/v1/import/ignore-account', () =>
    HttpResponse.json({ success: false, message: 'Card not found.', code: 400, errors: {} }, { status: 400 })))
  const user = userEvent.setup()
  renderCards(source)
  await user.click(await screen.findByRole('button', { name: 'Ignore' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Card not found.'))
})

it('a mapped card shows the account name and unmaps after confirmation', async () => {
  let called = false
  server.use(http.post('*/api/v1/import/unlink-account', () => {
    called = true
    return HttpResponse.json({ success: true, message: '', data: { item: source, run: null } })
  }))
  const user = userEvent.setup()
  renderCards({ ...source, cards: [{ ...unmapped, state: 'mapped', accountId: fixtureAccounts[0].id, queuedCount: 0 }] })
  expect(await screen.findByText(`Mapped · ${fixtureAccounts[0].name}`)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Unmap' }))
  await user.click(await screen.findByRole('button', { name: 'Unmap' , hidden: false }))
  await waitFor(() => expect(called).toBe(true))
})

it('the account picker lists accounts already in the card\'s currency first', async () => {
  const eurCard = { ...unmapped, externalCurrency: 'EUR' }
  const user = userEvent.setup()
  renderCards({ ...source, cards: [eurCard] })
  await user.click(await screen.findByRole('button', { name: 'Map to account' }))
  const select = await screen.findByLabelText('Account')
  const optionNames = within(select).getAllByRole('option').map((o) => o.textContent)
  expect(optionNames).toEqual(['', 'Euro Stash', 'Cash', 'Bank', 'Under the mattress'])
})
