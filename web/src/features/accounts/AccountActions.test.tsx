import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureUsd } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { AccountDto } from '@/api/dto/account'
import { AccountActionsMenu } from './AccountActions'

const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
const owned = fixtureAccounts[0] as AccountDto
const sharedWithMe = {
  id: 'a-foreign', owner: partner, folderId: 'f1', name: 'Shared wallet', position: 5,
  currency: fixtureUsd, balance: '10', type: 1, icon: 'wallet',
  sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'user', isAccepted: 1 }],
} as unknown as AccountDto

function renderMenu(account: AccountDto) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        {/* the menu sits inside a selectable row in the search dialog */}
        <div data-testid="row" onClick={rowClick} onPointerDown={rowPointerDown}>
          <AccountActionsMenu account={account} />
        </div>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const rowClick = vi.fn()
const rowPointerDown = vi.fn()

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedWithMe], connections: [{ user: partner, sharedAccounts: [] }] }))
  useUiStore.setState({ accountModal: null })
  rowClick.mockClear()
  rowPointerDown.mockClear()
})

it('owned account: Edit, Access control and Delete; delete confirms then posts', async () => {
  let deleted: unknown
  server.use(
    http.post('*/api/v1/account/delete-account', async ({ request }) => {
      deleted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderMenu(owned)
  await user.click(screen.getByRole('button', { name: 'account actions Cash' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  // pointerdown must reach ancestors: an enclosing dialog's outside-click tracking relies on it
  expect(rowPointerDown).toHaveBeenCalled()
  // the owner holds admin rights, which unlocks the access control entry
  await waitFor(() => expect(screen.getByRole('menuitem', { name: 'Access control' })).toBeInTheDocument())
  expect(screen.queryByRole('menuitem', { name: 'Decline' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Are you sure you want to delete the account “Cash”?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deleted).toEqual({ id: 'a1' }))
  // neither the trigger, the menu nor the dialog may select the surrounding row
  expect(rowClick).not.toHaveBeenCalled()
})

it('shared-with-me account as a plain user: Edit and Decline, no Access control', async () => {
  let posted: unknown
  server.use(
    http.post('*/api/v1/account/delete-account', async ({ request }) => {
      posted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderMenu(sharedWithMe)
  await user.click(screen.getByRole('button', { name: 'account actions Shared wallet' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Decline' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Access control' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Decline' }))
  expect(await screen.findByText('Are you sure you want to decline access to the account “Shared wallet”?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Decline' }))
  await waitFor(() => expect(posted).toEqual({ id: 'a-foreign' }))
})

it('Edit opens the account modal for that account', async () => {
  const user = userEvent.setup()
  renderMenu(owned)
  await user.click(screen.getByRole('button', { name: 'account actions Cash' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(useUiStore.getState().accountModal?.account?.id).toBe(owned.id)
  expect(rowClick).not.toHaveBeenCalled()
})

it('Access control opens the share dialog and grants through the level picker', async () => {
  let granted: unknown
  server.use(
    http.post('*/api/v1/account/grant-access', async ({ request }) => {
      granted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderMenu(owned)
  await user.click(screen.getByRole('button', { name: 'account actions Cash' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Access control' }))
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  await user.click(await screen.findByRole('button', { name: 'Full control' }))
  await waitFor(() => expect(granted).toEqual({ accountId: 'a1', userId: 'u2', role: 'admin' }))
  expect(rowClick).not.toHaveBeenCalled()
})
