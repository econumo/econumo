import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { SharingRequestsDialog } from './SharingRequestsDialog'

const owner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }

const pendingAccount = {
  id: 'a-pending', owner, folderId: null, name: 'Shared cash', position: 0,
  currency: { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  balance: '0', type: 1, icon: 'wallet',
  sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'user', isAccepted: 0 }],
}

const pendingBudget = {
  id: 'b-pending', ownerUserId: 'u2', name: 'Shared budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
  access: [
    { user: owner, role: 'owner', isAccepted: 1 },
    { user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'admin', isAccepted: 0 },
  ],
}

const testFolders = [
  { id: 'f1', name: 'General', position: 0, isVisible: 1 },
  { id: 'f2', name: 'Savings', position: 1, isVisible: 1 },
]

function renderDialog(onClose = () => {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <SharingRequestsDialog open onClose={onClose} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('shows the empty state without pending invites', async () => {
  server.use(...coreHandlers())
  renderDialog()
  expect(await screen.findByText('No pending requests')).toBeInTheDocument()
})

it('lists pending account and budget invites with owner, kind and role', async () => {
  server.use(...coreHandlers({ accounts: [pendingAccount], budgets: [pendingBudget], folders: testFolders }))
  renderDialog()
  expect(await screen.findAllByText('Partner invited you')).toHaveLength(2)
  expect(screen.getByText('Shared cash')).toBeInTheDocument()
  expect(screen.getByText('Shared budget')).toBeInTheDocument()
  expect(screen.getByText('Manage transactions')).toBeInTheDocument()
  expect(screen.getByText('Full control')).toBeInTheDocument()
})

it('account accept reveals a folder select preselected to the last folder, then posts accountId+folderId', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ accounts: [pendingAccount], folders: testFolders }),
    http.post('*/api/v1/account/accept-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await screen.findByText('Shared cash')
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  expect(await screen.findByText('Choose a folder for this account')).toBeInTheDocument()
  expect(screen.getByRole('combobox')).toHaveTextContent('Savings')
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  await waitFor(() => expect(body).toEqual({ accountId: 'a-pending', folderId: 'f2' }))
})

it('account accept with zero folders shows a disabled general-folder option and omits folderId', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ accounts: [pendingAccount], folders: [] }),
    http.post('*/api/v1/account/accept-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await screen.findByText('Shared cash')
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  expect(await screen.findByText('Choose a folder for this account')).toBeInTheDocument()
  await user.click(screen.getByRole('combobox'))
  expect(await screen.findByRole('option', { name: 'General (will be created)' })).toHaveAttribute('aria-disabled', 'true')
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  await waitFor(() => expect(body).toEqual({ accountId: 'a-pending', folderId: '' }))
})

it('budget accept posts immediately', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ budgets: [pendingBudget] }),
    http.post('*/api/v1/budget/accept-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await screen.findByText('Shared budget')
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  await waitFor(() => expect(body).toEqual({ budgetId: 'b-pending' }))
})

it('account decline confirms then posts accountId', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ accounts: [pendingAccount] }),
    http.post('*/api/v1/account/decline-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await screen.findByText('Shared cash')
  await user.click(screen.getByRole('button', { name: 'Decline' }))
  expect(await screen.findByText('Decline access to "Shared cash"?')).toBeInTheDocument()
  await user.click(screen.getAllByRole('button', { name: 'Decline' }).slice(-1)[0])
  await waitFor(() => expect(body).toEqual({ accountId: 'a-pending' }))
})

it('budget decline confirms then posts budgetId', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ budgets: [pendingBudget] }),
    http.post('*/api/v1/budget/decline-access', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await screen.findByText('Shared budget')
  await user.click(screen.getByRole('button', { name: 'Decline' }))
  expect(await screen.findByText('Decline access to "Shared budget"?')).toBeInTheDocument()
  await user.click(screen.getAllByRole('button', { name: 'Decline' }).slice(-1)[0])
  await waitFor(() => expect(body).toEqual({ budgetId: 'b-pending' }))
})
