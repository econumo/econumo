import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureBudgets, fixtureConnections, fixtureOwner } from '@/test/fixtures'
import { PreviewConnectionDialog } from './PreviewConnectionDialog'

const partner = fixtureConnections[0].user

const sharedAccounts = fixtureAccounts.map((a) =>
  a.id === 'a1' ? { ...a, sharedAccess: [{ user: partner, role: 'user' }] } : a,
)
const budgetsWithShared = [
  fixtureBudgets[0],
  {
    id: 'b2', ownerUserId: 'u2', name: 'Partner plan', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [
      { user: partner, role: 'owner', isAccepted: 1 },
      { user: fixtureOwner, role: 'guest', isAccepted: 1 },
    ],
  },
]

function renderDialog(onDelete = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <PreviewConnectionDialog open connection={fixtureConnections[0]} onDelete={onDelete} onClose={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { queryClient, onDelete }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('lists shared items with ownership badges and roles', async () => {
  server.use(...coreHandlers({ accounts: sharedAccounts, budgets: budgetsWithShared, connections: fixtureConnections }))
  renderDialog()
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.getByText(/Your account/)).toBeInTheDocument()
  expect(screen.getByText(/Manage transactions/)).toBeInTheDocument()
  expect(await screen.findByText('Partner plan')).toBeInTheDocument()
  expect(screen.getByText(/Shared with you/)).toBeInTheDocument()
  expect(screen.getAllByText('Select an item to manage access').length).toBeGreaterThan(0)
  // each row's owner avatar still names the owner (title on the wrapper)
  expect(screen.getByTitle('Ada')).toBeInTheDocument()
  expect(screen.getByTitle('Partner')).toBeInTheDocument()
})

it('shows empty states without shared items', async () => {
  server.use(...coreHandlers({ connections: fixtureConnections }))
  renderDialog()
  expect(await screen.findByText('No shared budgets')).toBeInTheDocument()
  expect(await screen.findByText('No shared accounts')).toBeInTheDocument()
})

it('owned account row opens the level dialog; picking a role posts set-account-access', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ accounts: sharedAccounts, budgets: budgetsWithShared, connections: fixtureConnections }),
    http.post('*/api/v1/connection/set-account-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByRole('button', { name: /Cash/ }))
  await user.click(await screen.findByRole('button', { name: 'View only' }))
  await waitFor(() => expect(body).toEqual({ accountId: 'a1', userId: 'u2', role: 'guest' }))
})

it('shared-with-me budget row opens decline; declining posts decline-access', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ accounts: sharedAccounts, budgets: budgetsWithShared, connections: fixtureConnections }),
    http.post('*/api/v1/budget/decline-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByRole('button', { name: /Partner plan/ }))
  await user.click(await screen.findByRole('button', { name: 'Decline access' }))
  await waitFor(() => expect(body).toEqual({ budgetId: 'b2' }))
})

it('delete button hands the connected user id to onDelete', async () => {
  server.use(...coreHandlers({ connections: fixtureConnections }))
  const user = userEvent.setup()
  const { onDelete } = renderDialog()
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  expect(onDelete).toHaveBeenCalledWith('u2')
})
