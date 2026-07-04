import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureFolders } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { AccountsSettingsPage } from './AccountsSettingsPage'

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
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
  expect(await screen.findByText('The folder name must be between 3 and 64 characters')).toBeInTheDocument()
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
  expect(await screen.findByText('Do you want to delete the folder «Savings»?')).toBeInTheDocument()
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
  expect(await screen.findByText('Are you sure you want to remove the account «Cash»?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deleted).toEqual({ id: 'a1' }))
  await waitFor(() => expect(screen.queryByText('Cash')).not.toBeInTheDocument())

  await user.click(screen.getByRole('button', { name: 'account actions Bank' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(useUiStore.getState().accountModal?.account?.id).toBe('a2')
})
