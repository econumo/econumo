import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner } from '@/test/fixtures'
import type { BudgetMetaDto } from '@/api/dto/budget'
import { CompleteBudgetDialog } from './CompleteBudgetDialog'
import { DuplicateBudgetDialog } from './DuplicateBudgetDialog'

// On 20 October a day-28 budget is in the period named September, but this
// budget starts with October: no dialog may send a month before its start.
const lateStart: BudgetMetaDto = {
  id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-10-01 00:00:00', endedAt: '', currencyId: 'cur-usd',
  periodStartDay: 28, isArchived: 0, access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
}

function renderInApp(ui: React.ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 9, 20, 12, 0, 0))
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

afterEach(() => {
  vi.useRealTimers()
})

it('Complete never ends or continues a budget before its start', async () => {
  let update: Record<string, unknown> | undefined
  let clone: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      update = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: lateStart } })
    }),
    http.post('*/api/v1/budget/archive-budget', () =>
      HttpResponse.json({ success: true, message: '', data: { item: { ...lateStart, isArchived: 1 } } }),
    ),
    http.post('*/api/v1/budget/clone-budget', async ({ request }) => {
      clone = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { meta: { ...lateStart, id: 'copy' } } } })
    }),
  )
  const user = userEvent.setup()
  renderInApp(<CompleteBudgetDialog budget={lateStart} onClose={vi.fn()} />)
  await user.click(await screen.findByRole('button', { name: 'Save' }))
  await waitFor(() => expect(clone).toBeDefined())
  expect(update!.endDate).toBe('2026-10-01')
  expect(clone!.startDate).toBe('2026-10-01')
})

it('Duplicate starting this month never starts before the source', async () => {
  let clone: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/clone-budget', async ({ request }) => {
      clone = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { meta: { ...lateStart, id: 'copy' } } } })
    }),
  )
  const user = userEvent.setup()
  renderInApp(<DuplicateBudgetDialog budget={lateStart} onClose={vi.fn()} />)
  await user.click(await screen.findByRole('switch', { name: /Start from this month/ }))
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(clone).toBeDefined())
  expect(clone!.startDate).toBe('2026-10-01')
})
