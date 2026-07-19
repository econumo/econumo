import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts as fixtureAccountsForAccess, fixtureFolders, fixtureUsd } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { AccountsSettingsPage } from './AccountsSettingsPage'

function mockViewport(compact = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/accounts', element: <AccountsSettingsPage /> }], {
    initialEntries: ['/settings/accounts'],
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockViewport()
  useUiStore.setState({ accountModal: null })
})

it('renders all folders (hidden marked) with their accounts', async () => {
  renderPage()
  expect(await screen.findByTestId('folder-General')).toBeInTheDocument()
  expect(screen.getByTestId('folder-Savings')).toBeInTheDocument()
  const hidden = screen.getByTestId('folder-Hidden')
  expect(within(hidden).getByLabelText('hidden')).toBeInTheDocument()
  expect(within(hidden).getByText('Under the mattress')).toBeInTheDocument()
  expect(within(screen.getByTestId('folder-General')).getByText('Cash')).toBeInTheDocument()
})

it('creates and renames folders via the prompt dialog', async () => {
  let created: unknown
  let updated: unknown
  server.use(
    http.post('*/api/v1/account/create-folder', async ({ request }) => {
      created = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'f-new', name: 'Investments', position: 3, isVisible: 1 } } })
    }),
    http.post('*/api/v1/account/update-folder', async ({ request }) => {
      updated = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: { ...fixtureFolders[0], name: 'Renamed' } } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')

  await user.click(screen.getByRole('button', { name: /Create folder/ }))
  // Vue quirk kept: the validator allows 2-64 even though the message says 3-64,
  // so only a 1-char name trips the client-side error
  await user.type(await screen.findByLabelText('Name'), 'a')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(await screen.findByText('Folder name must be 3-64 characters')).toBeInTheDocument()
  await user.clear(screen.getByLabelText('Name'))
  await user.type(screen.getByLabelText('Name'), 'Investments')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(created).toEqual({ name: 'Investments' }))

  await user.click(screen.getByRole('button', { name: 'folder actions General' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const input = await screen.findByLabelText('Name')
  expect(input).toHaveValue('General')
  await user.clear(input)
  await user.type(input, 'Renamed')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(updated).toEqual({ id: 'f1', name: 'Renamed' }))
})

it('move down posts the swapped positions', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/account/order-folder-list', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: fixtureFolders } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByRole('button', { name: 'folder actions General' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Move down' }))
  await waitFor(() =>
    expect(body).toEqual({
      changes: [
        { id: 'f2', position: 0 },
        { id: 'f1', position: 1 },
      ],
    }),
  )
})

it('deleting a folder posts replace with the last other folder as fallback', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/account/replace-folder', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-Savings')
  await user.click(screen.getByRole('button', { name: 'folder actions Savings' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Are you sure you want to delete the folder “Savings”?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(body).toEqual({ id: 'f2', replaceId: 'f-hidden' }))
})

it('first folder has no move-up and no delete', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByRole('button', { name: 'folder actions General' }))
  expect(await screen.findByRole('menuitem', { name: 'Move down' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Move up' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
})

it('account delete confirm posts and removes; edit opens the account modal', async () => {
  let deleted: unknown
  server.use(
    http.post('*/api/v1/account/delete-account', async ({ request }) => {
      deleted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByRole('button', { name: 'account actions Cash' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Are you sure you want to delete the account “Cash”?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deleted).toEqual({ id: 'a1' }))
  await waitFor(() => expect(screen.queryByText('Cash')).not.toBeInTheDocument())

  await user.click(screen.getByRole('button', { name: 'account actions Bank' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(useUiStore.getState().accountModal?.account?.id).toBe('a2')
})

it('shared-with-me account row offers Decline instead of Delete; confirming posts delete-account', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  const foreign = {
    id: 'a-foreign', owner: partner, folderId: 'f1', name: 'Shared wallet', position: 5,
    currency: fixtureUsd, balance: '10', type: 1, icon: 'wallet',
    sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'user' }],
  }
  let posted: unknown
  server.use(
    ...coreHandlers({
      accounts: [...fixtureAccountsForAccess, foreign],
      connections: [{ user: partner, sharedAccounts: [] }],
    }),
    http.post('*/api/v1/account/delete-account', async ({ request }) => {
      posted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByRole('button', { name: 'account actions Shared wallet' }))
  expect(await screen.findByRole('menuitem', { name: 'Decline' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Decline' }))
  expect(await screen.findByText('Are you sure you want to decline access to the account “Shared wallet”?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Decline' }))
  await waitFor(() => expect(posted).toEqual({ id: 'a-foreign' }))
  await waitFor(() => expect(screen.queryByText('Shared wallet')).not.toBeInTheDocument())
})

it('access control: shared avatars, grant and revoke through the dialogs', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  const sharedAccounts = [
    { ...fixtureAccountsForAccess[0], sharedAccess: [{ user: partner, role: 'user', isAccepted: 1 }] },
    ...fixtureAccountsForAccess.slice(1),
  ]
  let granted: unknown
  let revoked: unknown
  server.use(
    ...coreHandlers({
      accounts: sharedAccounts,
      connections: [{ user: partner, sharedAccounts: [] }],
    }),
    http.post('*/api/v1/account/grant-access', async ({ request }) => {
      granted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/account/revoke-access', async ({ request }) => {
      revoked = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  const cluster = screen.getByTestId('shared-avatars-Cash')
  // each avatar in the cluster still names its user (title on the wrapper)
  expect(within(cluster).getByTitle('Ada')).toBeInTheDocument()
  expect(within(cluster).getByTitle('Partner')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'account actions Cash' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Access control' }))
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  await user.click(await screen.findByRole('button', { name: 'Full control' }))
  await waitFor(() => expect(granted).toEqual({ accountId: 'a1', userId: 'u2', role: 'admin' }))

  // the optimistic cache update relabels the row; revoke through the same path
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  await user.click(await screen.findByRole('button', { name: 'Revoke access' }))
  const revokeConfirm = await screen.findByRole('dialog', { name: 'Revoke access?' })
  await user.click(within(revokeConfirm).getByRole('button', { name: 'Revoke access' }))
  await waitFor(() => expect(revoked).toEqual({ accountId: 'a1', userId: 'u2' }))
})

it('clicking an account row opens its context menu (desktop)', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByText('Cash'))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(useUiStore.getState().accountModal?.account?.id).toBe('a1')
  // the click on the portaled menu item must not bubble back to the row and reopen the menu
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
})

it('clicking a folder header opens the folder menu (desktop)', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-Savings')
  await user.click(screen.getByText('Savings'))
  expect(await screen.findByRole('menuitem', { name: 'Move up' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Hide' })).toBeInTheDocument()
})

it('hidden folders are visually distinct', async () => {
  renderPage()
  const hidden = await screen.findByTestId('folder-Hidden')
  expect(hidden).toHaveClass('border-dashed')
  expect(screen.getByTestId('folder-General')).not.toHaveClass('border-dashed')
})

it('folder reorder applies optimistically before the server responds', async () => {
  server.use(
    http.post('*/api/v1/account/order-folder-list', async () => {
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByRole('button', { name: 'folder actions General' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Move down' }))
  await waitFor(() => {
    const order = screen.getAllByTestId(/^folder-/).map((el) => el.dataset.testid)
    expect(order).toEqual(['folder-Savings', 'folder-General', 'folder-Hidden'])
  })
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
})

it('hiding a folder marks it immediately, before the server responds', async () => {
  server.use(
    http.post('*/api/v1/account/hide-folder', async () => {
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByRole('button', { name: 'folder actions General' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Hide' }))
  await waitFor(() => {
    const general = screen.getByTestId('folder-General')
    expect(within(general).getByLabelText('hidden')).toBeInTheDocument()
    expect(general).toHaveClass('border-dashed')
  })
})

it('shows the folders info box', async () => {
  renderPage()
  await screen.findByTestId('folder-General')
  expect(screen.getByText(/Accounts are organized into folders/)).toBeInTheDocument()
})

it('collapses and expands a folder, persisting the choice', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  expect(screen.getByText('Cash')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'toggle folder General' }))
  expect(screen.queryByText('Cash')).not.toBeInTheDocument()
  // collapsed header shows the account count instead of the list
  expect(screen.getByTestId('folder-count-General')).toHaveTextContent('1')
  expect(JSON.parse(localStorage.getItem('settings.accounts.collapsedFolders') ?? '[]')).toEqual(['f1'])

  await user.click(screen.getByRole('button', { name: 'toggle folder General' }))
  expect(screen.getByText('Cash')).toBeInTheDocument()
  expect(JSON.parse(localStorage.getItem('settings.accounts.collapsedFolders') ?? '[]')).toEqual([])
})

it('starts collapsed for folders remembered in localStorage', async () => {
  localStorage.setItem('settings.accounts.collapsedFolders', JSON.stringify(['f2']))
  renderPage()
  await screen.findByTestId('folder-Savings')
  expect(screen.queryByText('Bank')).not.toBeInTheDocument()
  expect(screen.getByText('Cash')).toBeInTheDocument()
})

it('each folder header offers an add-account button preset to that folder', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByRole('button', { name: 'add account to Savings' }))
  expect(useUiStore.getState().accountModal?.folderId).toBe('f2')
})

it('compact: the preview sheet lists connections and grants access in place', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  let granted: unknown
  server.use(
    ...coreHandlers({ connections: [{ user: partner, sharedAccounts: [] }] }),
    http.post('*/api/v1/account/grant-access', async ({ request }) => {
      granted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByText('Cash'))
  await screen.findByText('Account details')
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  await user.click(await screen.findByRole('button', { name: 'Full control' }))
  await waitFor(() => expect(granted).toEqual({ accountId: 'a1', userId: 'u2', role: 'admin' }))
})

it('compact: the preview sheet shows the empty hint when the owner has no connections', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByText('Cash'))
  await screen.findByText('Account details')
  expect(await screen.findByText('No connections found')).toBeInTheDocument()
})

it('compact: the preview sheet shows a read-only access list to a non-admin member', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  const foreign = {
    id: 'a-foreign', owner: partner, folderId: 'f1', name: 'Shared wallet', position: 5,
    currency: fixtureUsd, balance: '10', type: 1, icon: 'wallet',
    sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'user', isAccepted: 1 }],
  }
  server.use(...coreHandlers({
    accounts: [...fixtureAccountsForAccess, foreign],
    connections: [{ user: partner, sharedAccounts: [] }],
  }))
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByText('Shared wallet'))
  await screen.findByText('Account details')
  expect(await screen.findByText('Owner')).toBeInTheDocument()
  expect(screen.getByText('Manage transactions')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Partner/ })).toBeNull()
  expect(screen.getByRole('button', { name: 'Decline' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
})
