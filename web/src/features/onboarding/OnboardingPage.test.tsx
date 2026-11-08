import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { OnboardingPage } from './OnboardingPage'

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
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as { id: string; name: string }
      calls.push({ path: 'create-account', body })
      const item = { id: `acc-${accounts.length + 1}`, name: body.name, owner: { id: fixtureUser.id } }
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

it('/ renders onboarding for a not-yet-onboarded user', async () => {
  const { HomePage } = await import('@/features/home/HomePage')
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
