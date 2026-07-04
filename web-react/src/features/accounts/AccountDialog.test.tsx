import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import { AccountDialog } from './AccountDialog'
import type { AccountDto } from '@/api/dto/account'

const UUID_V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AccountDialog />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
  useUiStore.setState({ accountModal: null })
})

it('creates an account with a UUIDv7 op id and numeric balance; correction lands in the transactions cache', async () => {
  let body: Record<string, unknown> | undefined
  const createdAccount = { ...fixtureAccounts[0], id: 'a-created', name: 'New Wallet', balance: '25' }
  const correction = {
    id: 't-corr', author: { id: 'u1', avatar: '', name: 'Ada' }, type: 'income', accountId: 'a-created',
    accountRecipientId: null, amount: '25', amountRecipient: '25', categoryId: null, description: '',
    payeeId: null, tagId: null, date: '2026-07-03 10:00:00',
  }
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: createdAccount, transaction: correction } })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderDialog()
  queryClient.setQueryData(queryKeys.transactions, [])
  queryClient.setQueryData(queryKeys.folders, [{ id: 'f1', name: 'General', position: 0, isVisible: 1 }])
  useUiStore.getState().openAccountModal({ folderId: 'f1' })

  await screen.findByText('New account')
  await user.type(screen.getByLabelText('Name'), 'New Wallet')
  const balanceField = screen.getByLabelText('Balance')
  await user.clear(balanceField)
  await user.type(balanceField, '20+5=')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body!.id).toMatch(UUID_V7)
  expect(body!.balance).toBe(25)
  expect(body!.folderId).toBe('f1')
  expect(body!.name).toBe('New Wallet')
  await waitFor(() =>
    expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.transactions)!.map((t) => t.id)).toEqual(['t-corr']),
  )
})

it('shows the exact validation messages', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openAccountModal({ folderId: null })
  await screen.findByText('New account')
  const balanceField = screen.getByLabelText('Balance')
  await user.clear(balanceField)
  await user.click(screen.getByRole('button', { name: 'Add' }))
  expect(await screen.findByText('Required field')).toBeInTheDocument()
  await user.type(screen.getByLabelText('Name'), 'ab')
  await user.type(balanceField, '10')
  await user.click(screen.getByRole('button', { name: 'Add' }))
  expect(await screen.findByText('The account name must be between 3 and 64 characters')).toBeInTheDocument()
})

it('edit mode seeds the raw balance and posts updatedAt in Y-m-d H:i:s', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/account/update-account', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureAccounts[0], transaction: null } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  const account = { ...fixtureAccounts[0], balance: 1234.5 } as unknown as AccountDto
  useUiStore.getState().openAccountModal({ account })

  await screen.findByText('Update account')
  expect(screen.getByLabelText('Balance')).toHaveValue('1234.50')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.id).toBe(account.id)
  expect(body!.updatedAt).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
  expect(body!.currencyId).toBe('cur-usd')
})
