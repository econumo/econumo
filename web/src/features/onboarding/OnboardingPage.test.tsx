import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { HomePage } from '@/features/home/HomePage'
import { OnboardingPage } from './OnboardingPage'

let importedRows = 0
vi.mock('@/features/transactions/ImportCsvDialog', () => ({
  ImportCsvDialog: ({ open, onComplete }: { open: boolean; onComplete: (r: { imported: number }) => void }) =>
    open ? <button onClick={() => onComplete({ imported: importedRows })}>finish import</button> : null,
}))

const userWith = (gettingStarted: string, onboarding = '') => ({
  ...fixtureUser,
  options: [
    ...fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: onboarding } : o)),
    { name: 'getting_started', value: gettingStarted },
  ],
})

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
}

const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })

let posts: { path: string; value?: string }[]

// get-user-data serves the stored option, a getting-started POST replaces it
function serve(initial: string) {
  posts = []
  const state = { stored: initial, completed: false, failComplete: false }
  server.use(...coreHandlers({ budgets: [], categories: [] }))
  // added later, so these answer ahead of the core handlers
  server.use(
    http.get('*/api/v1/user/get-user-data', () => ok({ user: userWith(state.stored, state.completed ? 'completed' : '') })),
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      const { value } = (await request.json()) as { value: string }
      posts.push({ path: 'update-getting-started', value })
      state.stored = value
      return ok({ user: userWith(value) })
    }),
    http.post('*/api/v1/user/complete-onboarding', () => {
      if (state.failComplete) {
        state.failComplete = false
        return HttpResponse.json({ success: false, message: 'Try again', code: 400, errors: {} }, { status: 400 })
      }
      posts.push({ path: 'complete-onboarding' })
      state.completed = true
      return ok({ user: userWith(state.stored, 'completed') })
    }),
  )
  return state
}

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport(false)
})

const heading = (name: string) => screen.findByRole('heading', { name })

it('opens on the first open step with the stepper above it', async () => {
  serve('import')
  renderPage()
  expect(await heading('Categories')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Categories/ })).toHaveAttribute('aria-current', 'step')
  expect(screen.getByRole('button', { name: /Import/ })).toHaveAttribute('data-state', 'done')
  expect(screen.getByRole('button', { name: 'Skip setup' })).toBeInTheDocument()
})

it('skipping a step records it and moves to the next open step', async () => {
  serve('')
  const user = userEvent.setup()
  renderPage()
  expect(await heading('Bring in your history')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: "I don't have one" }))
  expect(await heading('Categories')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'update-getting-started', value: '!import' }])
})

it('a revisited step moves on to the next open step after it', async () => {
  serve('')
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: /Accounts/ }))
  expect(await heading('Accounts')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Skip' }))
  expect(await heading('Budget & plan')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'update-getting-started', value: '!accounts' }])
})

it('resolving the last open step completes onboarding and lands on the budget', async () => {
  serve('import,!categories,accounts')
  const user = userEvent.setup()
  renderPage()
  expect(await heading('Budget & plan')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts).toEqual([
    { path: 'update-getting-started', value: 'import,!categories,accounts,plan' },
    { path: 'complete-onboarding' },
  ])
})

it('Skip setup skips every open step, then completes onboarding', async () => {
  const state = serve('import')
  const user = userEvent.setup()
  renderPage()
  await heading('Categories')
  await user.click(screen.getByRole('button', { name: 'Skip setup' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts.at(-1)).toEqual({ path: 'complete-onboarding' })
  await waitFor(() => expect(state.stored.split(',').sort()).toEqual(['!accounts', '!categories', '!plan', 'import']))
})

it('an import with rows settles import, categories and accounts in one request', async () => {
  serve('')
  importedRows = 12
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.click(await screen.findByRole('button', { name: 'finish import' }))
  expect(await heading('Budget & plan')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'update-getting-started', value: 'import,categories,accounts' }])
})

it('an import that brought nothing in marks nothing', async () => {
  serve('')
  importedRows = 0
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.click(await screen.findByRole('button', { name: 'finish import' }))
  expect(screen.getByRole('heading', { name: 'Bring in your history' })).toBeInTheDocument()
  expect(posts).toEqual([])
})

// HomePage pulls in the whole budget page, which a loaded machine can take a while to render
it('/ renders the getting started page for a not-yet-onboarded user', { timeout: 15000 }, async () => {
  serve('')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <HomePage /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await heading('Bring in your history')).toBeInTheDocument()
})

it('completes at once when every step was already answered', async () => {
  serve('import,!categories,accounts,!plan')
  renderPage()
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'complete-onboarding' }])
})

it('a failed completion shows the error and a Finish button that retries', async () => {
  const state = serve('import,!categories,accounts,!plan')
  state.failComplete = true
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByRole('alert')).toHaveTextContent('Try again')
  expect(posts).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Finish' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'complete-onboarding' }])
})

it('a phone gets a menu button that opens the navigation without finishing', async () => {
  mockViewport(true)
  serve('')
  renderPage()
  await heading('Bring in your history')
  expect(screen.getByRole('link', { name: 'Menu' })).toHaveAttribute('href', '/?menu=1')
})
