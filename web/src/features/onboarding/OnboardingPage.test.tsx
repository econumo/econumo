import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { HomePage } from '@/features/home/HomePage'
import { OnboardingPage } from './OnboardingPage'
import { emptyDraft, loadDraft, saveDraft, type Day0Draft } from './draft'

const pendingUser = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: '' } : o)),
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/onboarding', element: <OnboardingPage /> },
      { path: '/budget', element: <div>BUDGET ROUTE</div> },
    ],
    { initialEntries: ['/onboarding'] },
  )
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
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })
const userEnvelope = () => ok({ user: pendingUser })

let calls: { path: string; body: unknown }[]
let accounts: unknown[]

function mockWrites() {
  calls = []
  accounts = []
  const record = (path: string) => async ({ request }: { request: Request }) => {
    calls.push({ path, body: await request.json().catch(() => null) })
    return null
  }
  server.use(
    http.get('*/api/v1/account/get-account-list', () => ok({ items: accounts })),
    ...coreHandlers({ user: pendingUser, budgets: [], categories: [] }),
    http.get('*/api/v1/account/get-folder-list', () => ok({ items: accounts.length > 0 ? [{ id: 'folder-general', name: 'General', position: 0 }] : [] })),
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as { id: string; name: string; folderId?: string | null }
      calls.push({ path: 'create-account', body })
      if (!body.folderId && accounts.length > 0) {
        return HttpResponse.json({ success: false, message: 'invalid', code: 400, errors: { folderId: ['This value should not be blank.'] } }, { status: 400 })
      }
      const item = { id: `acc-${accounts.length + 1}`, name: body.name, owner: { id: fixtureUser.id }, folderId: 'folder-general' }
      accounts.push(item)
      return ok({ item, transaction: null })
    }),
    http.post('*/api/v1/user/update-currency', () => userEnvelope()),
    http.post('*/api/v1/currency/hide-all-currencies', () => ok()),
    http.post('*/api/v1/currency/show-currency', () => ok()),
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      const body = (await request.json()) as { id: string; name: string; type: string }
      calls.push({ path: 'create-category', body })
      return ok({ item: { id: `cat-${body.id}`, name: body.name, type: body.type } })
    }),
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      const body = (await request.json()) as { id: string }
      calls.push({ path: 'create-budget', body })
      return ok({ item: { meta: { id: body.id } } })
    }),
    http.post('*/api/v1/user/update-getting-started', async (ctx) => {
      await record('update-getting-started')(ctx)
      return userEnvelope()
    }),
    http.post('*/api/v1/user/complete-onboarding', async (ctx) => {
      await record('complete-onboarding')(ctx)
      return userEnvelope()
    }),
  )
}

const paths = () => calls.map((c) => c.path)

it('shows step 1 for a fresh user', async () => {
  mockWrites()
  renderPage()
  expect(await screen.findByText("Where's your money right now?")).toBeInTheDocument()
  expect(screen.getByText('Step 1 of 2')).toBeInTheDocument()
})

it('saves the accounts, then the categories and budget, and lands on the budget', async () => {
  mockWrites()
  const user = userEvent.setup()
  renderPage()
  await user.type(await screen.findByLabelText('Account name'), 'Card')
  await waitFor(() => expect(screen.getAllByLabelText('Currency')[0]).toHaveTextContent('USD'))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(await screen.findByText('What do you spend on?')).toBeInTheDocument()
  expect(screen.getByText('Step 2 of 2')).toBeInTheDocument()
  expect(paths()).toContain('create-account')

  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(paths()).toContain('create-category')
  expect(paths().slice(-3)).toEqual(['create-budget', 'update-getting-started', 'complete-onboarding'])
})

it('skip on step 1 completes onboarding and lands on the budget', async () => {
  mockWrites()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Skip setup' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(paths()).toEqual(['update-getting-started', 'complete-onboarding'])
})

it('shows the error and stays put when saving fails', async () => {
  mockWrites()
  server.use(http.post('*/api/v1/account/create-account', () => HttpResponse.json({ success: false, message: 'boom' }, { status: 500 })))
  const user = userEvent.setup()
  renderPage()
  await user.type(await screen.findByLabelText('Account name'), 'Card')
  await waitFor(() => expect(screen.getAllByLabelText('Currency')[0]).toHaveTextContent('USD'))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const alert = await screen.findByRole('alert')
  const cont = screen.getByRole('button', { name: 'Continue' })
  expect(alert.compareDocumentPosition(cont) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(screen.getByText("Where's your money right now?")).toBeInTheDocument()
})

// HomePage pulls in the whole budget page, which a loaded machine can take a while to render
it('/ renders onboarding for a not-yet-onboarded user', { timeout: 15000 }, async () => {
  mockWrites()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <HomePage /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await screen.findByText("Where's your money right now?")).toBeInTheDocument()
})

const storeDraft = (patch: Partial<Day0Draft>) => saveDraft(fixtureUser.id, { ...emptyDraft(), ...patch })

it('resumes step 1 from a stored draft: created rows are read-only and keep their keys', async () => {
  mockWrites()
  accounts.push({ id: 'acc-card', name: 'Card', owner: { id: fixtureUser.id } })
  storeDraft({
    step: 'accounts',
    rows: [
      { key: 'k-card', name: 'Card', balance: '10', currencyId: 'cur-usd', savings: false, icon: 'credit_card' },
      { key: 'k-wallet', name: 'Wallet', balance: '5', currencyId: 'cur-usd', savings: false, icon: 'credit_card' },
    ],
    ops: { 'account:k-card': 'op-card', 'account:k-wallet': 'op-wallet' },
    created: { 'k-card': 'acc-card' },
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Step 1 of 2')).toBeInTheDocument()
  const names = screen.getAllByLabelText('Account name')
  expect(names.map((n) => (n as HTMLInputElement).value)).toEqual(['Card', 'Wallet', ''])
  expect(names[0]).toHaveAttribute('readonly')
  expect(screen.getAllByLabelText('Balance')[0]).toHaveAttribute('readonly')
  expect(names[1]).not.toHaveAttribute('readonly')
  expect(within(screen.getAllByTestId('account-row')[0]).queryByRole('button', { name: 'Remove account' })).not.toBeInTheDocument()

  await waitFor(() => expect(screen.getAllByLabelText('Currency')[0]).toHaveTextContent('USD'))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(await screen.findByText('What do you spend on?')).toBeInTheDocument()
  // only the uncreated row is sent, under the operation id the draft already held
  expect(calls.filter((c) => c.path === 'create-account').map((c) => c.body)).toEqual([expect.objectContaining({ id: 'op-wallet', name: 'Wallet' })])
  const saved = loadDraft(fixtureUser.id)!
  expect(saved.rows.map((r) => r.key)).toEqual(['k-card', 'k-wallet'])
  expect(saved.created).toEqual({ 'k-card': 'acc-card', 'k-wallet': 'acc-2' })
})

it('resumes step 2 from a stored draft: created categories are locked and not sent again', async () => {
  mockWrites()
  storeDraft({
    step: 'categories',
    accountIds: ['acc-card'],
    mainCurrencyId: 'cur-usd',
    currenciesDone: true,
    categoryRows: [
      { key: 'c-food', name: 'Food', type: 'expense', icon: 'restaurant', checked: true },
      { key: 'c-fun', name: 'Fun stuff', type: 'expense', icon: 'celebration', checked: true },
      { key: 'c-pay', name: 'Pay', type: 'income', icon: 'payments', checked: true },
    ],
    ops: { 'category:c-food': 'op-food', 'category:c-fun': 'op-fun', 'category:c-pay': 'op-pay' },
    createdCategories: { 'c-food': 'cat-food' },
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Step 2 of 2')).toBeInTheDocument()
  const rows = screen.getAllByTestId('category-row')
  expect(rows).toHaveLength(3)
  expect(within(rows[0]).getByLabelText('Category name')).toHaveValue('Food')
  expect(within(rows[0]).getByLabelText('Category name')).toHaveAttribute('readonly')
  expect(within(rows[0]).getByRole('checkbox')).toBeDisabled()
  expect(within(rows[0]).getByRole('button', { name: 'Change icon' })).toBeDisabled()
  expect(within(rows[1]).getByLabelText('Category name')).not.toHaveAttribute('readonly')

  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(calls.filter((c) => c.path === 'create-category').map((c) => (c.body as { id: string }).id)).toEqual(['op-fun', 'op-pay'])
})
