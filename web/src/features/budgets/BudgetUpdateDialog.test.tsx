import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner } from '@/test/fixtures'
import type { BudgetDto } from '@/api/dto/budget'
import { BudgetUpdateDialog } from './BudgetUpdateDialog'

const baseBudget: BudgetDto = {
  meta: {
    id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
  },
  filters: {
    periodStart: '2026-07-01 00:00:00',
    periodEnd: '2026-08-01 00:00:00',
    accounts: [
      { id: 'a1', removable: false },
      { id: 'a2', removable: true },
    ],
  },
  balances: [],
  currencyRates: [],
  structure: { folders: [], elements: [] },
}

function renderDialog(budget: BudgetDto) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <BudgetUpdateDialog open budget={budget} onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('renders the locked member disabled and submits the unchanged membership', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: baseBudget.meta } })
    }),
  )
  const user = userEvent.setup()
  renderDialog(baseBudget)
  const cash = await screen.findByRole('switch', { name: 'include Cash' })
  expect(cash).toBeChecked()
  expect(cash).toBeDisabled()
  const bank = screen.getByRole('switch', { name: 'include Bank' })
  expect(bank).toBeChecked()
  expect(bank).not.toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountIds).toEqual(['a1', 'a2'])
})

it('toggling an unlocked member off drops it from the submitted accountIds', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: baseBudget.meta } })
    }),
  )
  const user = userEvent.setup()
  renderDialog(baseBudget)
  await screen.findByRole('switch', { name: 'include Cash' })
  await user.click(screen.getByRole('switch', { name: 'include Bank' }))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountIds).toEqual(['a1'])
})

it('a deleted member absent from the live account list still round-trips in accountIds', async () => {
  const budgetWithDeletedMember: BudgetDto = {
    ...baseBudget,
    filters: {
      ...baseBudget.filters,
      accounts: [
        { id: 'a1', removable: false },
        { id: 'a-deleted', removable: false },
      ],
    },
  }
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: baseBudget.meta } })
    }),
  )
  const user = userEvent.setup()
  renderDialog(budgetWithDeletedMember)
  await screen.findByRole('switch', { name: 'include Cash' })
  // no row is rendered for the deleted member — it has no matching live account
  expect(screen.queryByText('a-deleted')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountIds).toEqual(['a1', 'a-deleted'])
})

// A server older than the membership release omits filters.accounts (and older
// still, filters entirely). The dialog must degrade to "no locked rows", not
// crash the whole page with a render error.
it('renders when the server omits filters.accounts', async () => {
  const legacy = { ...baseBudget, filters: { ...baseBudget.filters, accounts: undefined } } as unknown as BudgetDto
  renderDialog(legacy)
  await waitFor(() => expect(screen.getByDisplayValue('Main budget')).toBeInTheDocument())
})

it('renders when the server omits filters entirely', async () => {
  const legacy = { ...baseBudget, filters: undefined } as unknown as BudgetDto
  renderDialog(legacy)
  await waitFor(() => expect(screen.getByDisplayValue('Main budget')).toBeInTheDocument())
})
