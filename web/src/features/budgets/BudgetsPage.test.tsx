import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureUser } from '@/test/fixtures'
import { BudgetsPage } from './BudgetsPage'

const UUID_V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings/budgets', element: <BudgetsPage /> },
      { path: '/budget', element: <div>BUDGET ROUTE</div> },
    ],
    { initialEntries: ['/settings/budgets'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

const userWithDefaultBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
})

it('rows are name-sorted with the default bookmark marked', async () => {
  server.use(...coreHandlers({ user: userWithDefaultBudget }))
  renderPage()
  const rows = await screen.findAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Alpha plan')
  expect(rows[1]).toHaveTextContent('Main budget')
  expect(await screen.findByLabelText('default budget Main budget')).toBeDisabled()
  expect(screen.getByLabelText('set default Alpha plan')).toBeEnabled()
})

it('set-as-default posts {value:id}', async () => {
  server.use(...coreHandlers({ user: userWithDefaultBudget }))
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-budget', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: userWithDefaultBudget } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByLabelText('set default Alpha plan'))
  await waitFor(() => expect(body).toEqual({ value: 'b2' }))
})

it('creates a budget with excluded accounts and appends the row', async () => {
  server.use(...coreHandlers())
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: {
          item: {
            meta: {
              id: body.id, ownerUserId: 'u1', name: body.name, startedAt: '2026-07-01 00:00:00',
              currencyId: body.currencyId, access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
            },
          },
        },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Main budget')
  await user.click(screen.getByRole('button', { name: /Create budget/ }))
  await screen.findByRole('dialog')
  await user.type(screen.getByLabelText('Name'), 'Vacation')
  // the currency picker row seeds from the user's default currency
  await waitFor(() => expect(screen.getByRole('button', { name: /^Currency/ })).toHaveTextContent('USD'))
  await user.click(screen.getByRole('switch', { name: 'include Bank' }))
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.id).toMatch(UUID_V7)
  expect(body!.name).toBe('Vacation')
  expect(body!.startDate).toBe('')
  expect(body!.excludedAccounts).toEqual(['a2'])
  expect(await screen.findByText('Vacation')).toBeInTheDocument()
})

it('delete confirm removes the budget; go-to navigates', async () => {
  server.use(...coreHandlers({ user: userWithDefaultBudget }))
  server.use(
    http.post('*/api/v1/budget/delete-budget', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Alpha plan')
  await user.click(screen.getByRole('button', { name: 'budget actions Alpha plan' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Are you sure you want to delete “Alpha plan”?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(screen.queryByText('Alpha plan')).not.toBeInTheDocument())

  await user.click(screen.getByRole('button', { name: 'budget actions Main budget' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Open budget' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
})

const partner = { id: 'u2', avatar: 'https://avatars.test/partner', name: 'Partner' }
const ownedShared = {
  id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
  access: [
    { user: fixtureOwner, role: 'owner', isAccepted: 1 },
    { user: partner, role: 'user', isAccepted: 1 },
  ],
}
const incoming = {
  id: 'b3', ownerUserId: 'u2', name: 'Partner plan', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
  access: [
    { user: partner, role: 'owner', isAccepted: 1 },
    { user: fixtureOwner, role: 'user', isAccepted: 0 },
  ],
}

it('accept: incoming budget row accepts and the subtitle disappears', async () => {
  let body: unknown
  const accepted = { ...incoming, access: [incoming.access[0], { ...incoming.access[1], isAccepted: 1 }] }
  server.use(
    ...coreHandlers({ budgets: [ownedShared, incoming] }),
    http.post('*/api/v1/budget/accept-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [ownedShared, accepted] } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText(/Manage budget - invitation pending/)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'budget actions Partner plan' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Accept' }))
  await waitFor(() => expect(body).toEqual({ budgetId: 'b3' }))
  await waitFor(() => expect(screen.queryByText(/not accepted/)).not.toBeInTheDocument())
})

it('decline: confirm dialog posts decline-access', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ budgets: [ownedShared, incoming] }),
    http.post('*/api/v1/budget/decline-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Partner plan')
  await user.click(screen.getByRole('button', { name: 'budget actions Partner plan' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Decline' }))
  expect(await screen.findByText('Are you sure you want to decline access to the budget “Partner plan”?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Decline' }))
  await waitFor(() => expect(body).toEqual({ budgetId: 'b3' }))
})

it('access control: grant and revoke via the share dialogs', async () => {
  let granted: unknown
  let revoked: unknown
  server.use(
    ...coreHandlers({ budgets: [ownedShared], connections: [{ user: partner, sharedAccounts: [] }] }),
    http.post('*/api/v1/budget/grant-access', async ({ request }) => {
      granted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [ownedShared] } })
    }),
    http.post('*/api/v1/budget/revoke-access', async ({ request }) => {
      revoked = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Main budget')
  await user.click(screen.getByRole('button', { name: 'budget actions Main budget' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Access control' }))
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  await user.click(await screen.findByRole('button', { name: 'Manage budget' }))
  await waitFor(() => expect(granted).toEqual({ budgetId: 'b1', userId: 'u2', role: 'user' }))

  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  await user.click(await screen.findByRole('button', { name: 'Revoke access' }))
  const revokeConfirm = await screen.findByRole('dialog', { name: 'Revoke access?' })
  await user.click(within(revokeConfirm).getByRole('button', { name: 'Revoke access' }))
  await waitFor(() => expect(revoked).toEqual({ budgetId: 'b1', userId: 'u2' }))
})

it('delete is offered on an admin-shared budget, not only owned ones', async () => {
  const adminShared = {
    id: 'b4', ownerUserId: 'u2', name: 'Admin plan', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
    access: [
      { user: partner, role: 'owner', isAccepted: 1 },
      { user: fixtureOwner, role: 'admin', isAccepted: 1 },
    ],
  }
  server.use(...coreHandlers({ budgets: [adminShared] }))
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Admin plan')
  await user.click(screen.getByRole('button', { name: 'budget actions Admin plan' }))
  expect(await screen.findByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()
})
