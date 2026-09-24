import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureConnections, fixtureOwner } from '@/test/fixtures'
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
  expect(body!.balance).toBe('25')
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
  expect(await screen.findByText('Account name must be 3-64 characters')).toBeInTheDocument()
})

it('currency row opens the search picker; the picked currency is posted', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/account/update-account', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureAccounts[0], transaction: null } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openAccountModal({ account: fixtureAccounts[0] as unknown as AccountDto })

  await screen.findByText('Edit account')
  const currencyRow = await screen.findByRole('button', { name: /Currency/ })
  expect(currencyRow).toHaveTextContent('USD')
  await user.click(currencyRow)
  await user.click(await screen.findByText('EUR, €, Euro'))
  expect(screen.getByRole('button', { name: /Currency/ })).toHaveTextContent('EUR')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.currencyId).toBe('cur-eur')
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

  await screen.findByText('Edit account')
  expect(screen.getByLabelText('Balance')).toHaveValue('1234.50')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.id).toBe(account.id)
  expect(body!.updatedAt).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
  expect(body!.currencyId).toBe('cur-usd')
})

it('edit mode shows the access row for the owner; the share flow posts the grant', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    ...coreHandlers({ connections: fixtureConnections }),
    http.post('*/api/v1/account/grant-access', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openAccountModal({ account: fixtureAccounts[0] as unknown as AccountDto })

  await screen.findByText('Edit account')
  await user.click(await screen.findByRole('button', { name: /Access control/ }))
  // ShareAccessDialog: the connection appears with its current (no) access
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  // AccessLevelDialog: grant admin
  await user.click(await screen.findByRole('button', { name: 'Full control' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ accountId: 'a1', userId: 'u2', role: 'admin' })
})

it('create mode has no access row', async () => {
  renderDialog()
  useUiStore.getState().openAccountModal({ folderId: 'f1' })
  await screen.findByText('New account')
  await screen.findByRole('button', { name: /Currency/ })
  expect(screen.queryByRole('button', { name: /Access control/ })).toBeNull()
})

it('edit mode hides the access row from a non-admin member', async () => {
  const foreign = {
    ...fixtureAccounts[0],
    id: 'a-foreign',
    owner: { id: 'u2', avatar: 'pets:sky', name: 'Partner' },
    sharedAccess: [{ user: fixtureOwner, role: 'user', isAccepted: 1 }],
  }
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, foreign], connections: fixtureConnections }))
  renderDialog()
  useUiStore.getState().openAccountModal({ account: foreign as unknown as AccountDto })
  await screen.findByText('Edit account')
  await screen.findByRole('button', { name: /Currency/ })
  expect(screen.queryByRole('button', { name: /Access control/ })).toBeNull()
})

describe('savings account switch', () => {
  function captureCreate() {
    const bodies: Record<string, unknown>[] = []
    server.use(
      http.post('*/api/v1/account/create-account', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        bodies.push(body)
        return HttpResponse.json({
          success: true, message: '', data: { item: { ...fixtureAccounts[0], id: body.id, type: body.type }, transaction: null },
        })
      }),
    )
    return bodies
  }

  function captureUpdate() {
    const bodies: Record<string, unknown>[] = []
    server.use(
      http.post('*/api/v1/account/update-account', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        bodies.push(body)
        return HttpResponse.json({
          success: true, message: '', data: { item: { ...fixtureAccounts[0], id: body.id, type: body.type }, transaction: null },
        })
      }),
    )
    return bodies
  }

  const savingsAccount = { ...fixtureAccounts[0], type: 3 } as unknown as AccountDto

  function planSavingsFor(queryClient: QueryClient, accountId: string) {
    queryClient.setQueryData([...queryKeys.budget, 'b1', '2026-09-01'], {
      meta: { id: 'b1', ownerUserId: 'u1', name: 'Family', startedAt: '2026-01-01 00:00:00', endedAt: '', currencyId: 'cur-usd', isArchived: 0, access: [] },
      filters: { periodStart: '2026-09-01 00:00:00', periodEnd: '2026-10-01 00:00:00' },
      balances: [],
      currencyRates: [],
      structure: {
        folders: [], elements: [],
        savings: [{
          id: accountId, type: 5, name: 'Cash', icon: 'wallet', currencyId: 'cur-usd', ownerUserId: 'u1',
          isArchived: 0, position: 0, budgeted: '100', spent: '0', available: '100',
        }],
      },
    })
  }

  it('create sends type 3 with the switch on', async () => {
    const bodies = captureCreate()
    const user = userEvent.setup()
    renderDialog()
    useUiStore.getState().openAccountModal({ folderId: 'f1' })
    await screen.findByText('New account')
    expect(screen.getByText('Plan how much goes into this account in your budgets')).toBeInTheDocument()
    await user.type(screen.getByLabelText('Name'), 'Rainy day')
    const savings = screen.getByRole('switch', { name: 'Savings account' })
    expect(savings).not.toBeChecked()
    await user.click(savings)
    await user.click(screen.getByRole('button', { name: 'Add' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].type).toBe(3)
  })

  it('create sends type 2 with the switch off', async () => {
    const bodies = captureCreate()
    const user = userEvent.setup()
    renderDialog()
    useUiStore.getState().openAccountModal({ folderId: 'f1' })
    await screen.findByText('New account')
    await user.type(screen.getByLabelText('Name'), 'Wallet')
    await user.click(screen.getByRole('button', { name: 'Add' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].type).toBe(2)
  })

  it('update of a type-1 account sends 3 when switched on', async () => {
    const bodies = captureUpdate()
    const user = userEvent.setup()
    renderDialog()
    useUiStore.getState().openAccountModal({ account: fixtureAccounts[0] as unknown as AccountDto })
    await screen.findByText('Edit account')
    await user.click(screen.getByRole('switch', { name: 'Savings account' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].type).toBe(3)
  })

  it('update of a type-1 account keeps type 1 when switched on then off', async () => {
    const bodies = captureUpdate()
    const user = userEvent.setup()
    renderDialog()
    useUiStore.getState().openAccountModal({ account: fixtureAccounts[0] as unknown as AccountDto })
    await screen.findByText('Edit account')
    const savings = screen.getByRole('switch', { name: 'Savings account' })
    await user.click(savings)
    await user.click(savings)
    await user.click(screen.getByRole('button', { name: 'Update' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].type).toBe(1)
  })

  it('turning a savings account off with no budget planning it updates directly with type 2', async () => {
    const bodies = captureUpdate()
    const user = userEvent.setup()
    const queryClient = renderDialog()
    planSavingsFor(queryClient, 'some-other-account')
    useUiStore.getState().openAccountModal({ account: savingsAccount })
    await screen.findByText('Edit account')
    const savings = screen.getByRole('switch', { name: 'Savings account' })
    expect(savings).toBeChecked()
    await user.click(savings)
    await user.click(screen.getByRole('button', { name: 'Update' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].type).toBe(2)
    expect(screen.queryByText('Turn off savings?')).toBeNull()
  })

  it('turning a planned savings account off asks first; cancel sends nothing', async () => {
    const bodies = captureUpdate()
    const user = userEvent.setup()
    const queryClient = renderDialog()
    planSavingsFor(queryClient, savingsAccount.id)
    useUiStore.getState().openAccountModal({ account: savingsAccount })
    await screen.findByText('Edit account')
    await user.click(screen.getByRole('switch', { name: 'Savings account' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))
    expect(await screen.findByText('Turn off savings?')).toBeInTheDocument()
    expect(
      screen.getByText('Planned savings for this account will be removed from 1 budget. Saved amounts and transactions are not affected.'),
    ).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: 'Cancel' }).at(-1)!)
    await waitFor(() => expect(screen.queryByText('Turn off savings?')).toBeNull())
    expect(bodies).toHaveLength(0)
    expect(screen.getByText('Edit account')).toBeInTheDocument()
  })

  it('turning a planned savings account off sends type 2 once confirmed', async () => {
    const bodies = captureUpdate()
    const user = userEvent.setup()
    const queryClient = renderDialog()
    planSavingsFor(queryClient, savingsAccount.id)
    useUiStore.getState().openAccountModal({ account: savingsAccount })
    await screen.findByText('Edit account')
    await user.click(screen.getByRole('switch', { name: 'Savings account' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))
    await user.click(await screen.findByRole('button', { name: 'Turn off' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].type).toBe(2)
  })
})
