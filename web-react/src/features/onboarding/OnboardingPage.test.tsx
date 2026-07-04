import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
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
  useUiStore.setState({ accountModal: null })
})

it('renders the welcome heading and all six steps with completion marks', async () => {
  server.use(...coreHandlers({ user: pendingUser, connections: [] }))
  renderPage()
  expect(await screen.findByText('Welcome to Econumo!')).toBeInTheDocument()
  expect(screen.getByText('Add your accounts')).toBeInTheDocument()
  expect(screen.getByText('Enter your first transaction')).toBeInTheDocument()
  expect(screen.getByText('Manage categories, tags, and payees')).toBeInTheDocument()
  expect(screen.getByText('Update your avatar')).toBeInTheDocument()
  expect(screen.getByText('Connect with your partner')).toBeInTheDocument()
  expect(screen.getByText('Create your budget')).toBeInTheDocument()
  // fixtures have accounts+categories+transactions+tags+budgets, no connections
  await waitFor(() => expect(screen.getByTestId('step-accounts')).toHaveAttribute('data-done', 'true'))
  expect(screen.getByTestId('step-transactions')).toHaveAttribute('data-done', 'true')
  expect(screen.getByTestId('step-classifications')).toHaveAttribute('data-done', 'true')
  expect(screen.getByTestId('step-connections')).toHaveAttribute('data-done', 'false')
  expect(screen.getByTestId('step-budget')).toHaveAttribute('data-done', 'true')
  // guide links
  expect(screen.getByRole('link', { name: 'User guide: accounts' })).toHaveAttribute(
    'href',
    'https://econumo.com/docs/user-guide/accounts',
  )
})

it('"Add an account" opens the account modal with the first folder', async () => {
  server.use(...coreHandlers({ user: pendingUser }))
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Add an account' }))
  await waitFor(() => expect(useUiStore.getState().accountModal?.folderId).toBe('f1'))
})

it('"Complete onboarding" posts, updates the user cache, and navigates to the budget', async () => {
  let posts = 0
  server.use(
    ...coreHandlers({ user: pendingUser }),
    http.post('*/api/v1/user/complete-onboarding', () => {
      posts += 1
      return HttpResponse.json({ success: true, message: '', data: { user: fixtureUser } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Complete onboarding' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts).toBe(1)
})

it('/ renders onboarding for a not-yet-onboarded user', async () => {
  const { HomePage } = await import('@/features/home/HomePage')
  server.use(...coreHandlers({ user: pendingUser }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <HomePage /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Welcome to Econumo!')).toBeInTheDocument()
})
