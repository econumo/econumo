import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureWireBudget } from '@/test/fixtures'
import { cancelAiRefresh } from './finishImport'
import { OnboardingPage } from './OnboardingPage'
import { userWith } from './onboardingRoutes.test-utils'

// The real import wizard: its success path is what settles the import step during onboarding.

const CSV = 'Account,Date,Amount,Category,Description\nCash,2026-01-02,-5.5,Food,coffee\nBank,2026-01-03,100,Salary,pay\n'
const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

afterEach(() => cancelAiRefresh())

// without AI the wizard assigns no column: the user points at each one the import needs
async function answerWizard(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /from “Account”/ }))
  await user.click(screen.getByRole('button', { name: /^Date/ }))
  await user.click(screen.getByRole('radio', { name: /from “Date”/ }))
  await user.click(screen.getByRole('button', { name: /^Amount/ }))
  await user.click(screen.getByRole('radio', { name: /from “Amount”/ }))
}

it('a successful import writes finish-import first and never marks import through update-onboarding', { timeout: 20000 }, async () => {
  let option = 'started'
  let imported = false
  const writes: { path: string; body: unknown }[] = []
  server.use(...coreHandlers({ user: userWith(option), budgets: [], categories: [] }))
  // added later, so these answer ahead of the core handlers
  server.use(
    http.get('*/api/v1/user/get-user-data', () => ok({ user: userWith(option) })),
    http.get('*/api/v1/category/get-category-list', () => ok({ items: imported ? fixtureCategories : [] })),
    http.post('*/api/v1/transaction/import-transaction-list', () => {
      imported = true
      return ok({ imported: 2, skipped: 0, errors: {} })
    }),
    http.post('*/api/v1/user/update-onboarding', async ({ request }) => {
      const body = (await request.json()) as { value: string }
      writes.push({ path: 'update-onboarding', body })
      option = body.value
      return ok({ user: userWith(option) })
    }),
    http.post('*/api/v1/onboarding/finish-import', async ({ request }) => {
      writes.push({ path: 'finish-import', body: await request.json() })
      option = 'import'
      return ok({ user: userWith(option), budgetId: fixtureWireBudget.meta.id })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/onboarding', element: <OnboardingPage /> }], { initialEntries: ['/onboarding'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Bring in your history' }, { timeout: 5000 })
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.upload(await screen.findByLabelText('Choose a file'), new File([CSV], 'bank.csv', { type: 'text/csv' }))
  await answerWizard(user)
  await user.click(screen.getByRole('button', { name: /^Check/ }))
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import 2 transactions' }))
  expect(await screen.findByRole('heading', { name: 'What do you spend on?' }, { timeout: 5000 })).toBeInTheDocument()
  await screen.findByRole('button', { name: 'Archive Food' })
  expect(writes[0]).toEqual({ path: 'finish-import', body: { id: expect.any(String) } })
  expect(writes.filter((w) => w.path === 'finish-import')).toHaveLength(1)
  expect(writes.filter((w) => w.path === 'update-onboarding' && String((w.body as { value: string }).value).split(',').includes('import'))).toEqual([])
  await waitFor(() => expect(screen.getByRole('button', { name: /Import/ })).toHaveAttribute('data-state', 'done'))
})

it('an import that left rows out shows the result, and Try the rest again sends them and settles the step', { timeout: 20000 }, async () => {
  let option = 'started'
  let calls = 0
  const writes: string[] = []
  server.use(...coreHandlers({ user: userWith(option), budgets: [], categories: [] }))
  server.use(
    http.get('*/api/v1/user/get-user-data', () => ok({ user: userWith(option) })),
    http.post('*/api/v1/transaction/import-transaction-list', () => {
      calls++
      if (calls === 1) return HttpResponse.json({ success: false, message: 'Server down', code: 0, exceptionType: 'x' }, { status: 500 })
      return ok({ imported: 2, skipped: 0, errors: {} })
    }),
    http.post('*/api/v1/onboarding/finish-import', () => {
      writes.push('finish-import')
      option = 'import'
      return ok({ user: userWith(option), budgetId: fixtureWireBudget.meta.id })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/onboarding', element: <OnboardingPage /> }], { initialEntries: ['/onboarding'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Bring in your history' }, { timeout: 5000 })
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.upload(await screen.findByLabelText('Choose a file'), new File([CSV], 'bank.csv', { type: 'text/csv' }))
  await answerWizard(user)
  await user.click(screen.getByRole('button', { name: /^Check/ }))
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import 2 transactions' }))
  expect(await screen.findByText('Import failed')).toBeInTheDocument()
  expect(writes).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Try the rest again' }))
  expect(await screen.findByText('Import complete')).toBeInTheDocument()
  expect(screen.getByText('2 transactions imported')).toBeInTheDocument()
  expect(calls).toBe(2)
  expect(writes).toEqual(['finish-import'])
  await user.click(screen.getByRole('button', { name: 'OK' }))
  expect(await screen.findByRole('heading', { name: 'What do you spend on?' }, { timeout: 5000 })).toBeInTheDocument()
})

it('a partial import as the last open step keeps its result on screen before onboarding completes', { timeout: 20000 }, async () => {
  let option = 'categories,accounts,plan,!home_screen,!connect'
  let completed = false
  server.use(...coreHandlers({ user: userWith(option), budgets: [], categories: [] }))
  server.use(
    http.get('*/api/v1/user/get-user-data', () => ok({ user: userWith(completed ? 'completed' : option) })),
    http.post('*/api/v1/transaction/import-transaction-list', () => ok({ imported: 1, skipped: 1, errors: { 'Invalid amount format': [3] } })),
    http.post('*/api/v1/onboarding/finish-import', () => {
      option = 'import,' + option
      return ok({ user: userWith(option), budgetId: fixtureWireBudget.meta.id })
    }),
    http.post('*/api/v1/user/complete-onboarding', () => {
      completed = true
      return ok({ user: userWith('completed') })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/onboarding', element: <OnboardingPage /> },
      { path: '/budget', element: <p>budget page</p> },
    ],
    { initialEntries: ['/onboarding'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Bring in your history' }, { timeout: 5000 })
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.upload(await screen.findByLabelText('Choose a file'), new File([CSV], 'bank.csv', { type: 'text/csv' }))
  await answerWizard(user)
  await user.click(screen.getByRole('button', { name: /^Check/ }))
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import 2 transactions' }))
  expect(await screen.findByText('Import completed with errors')).toBeInTheDocument()
  // the step settled behind the dialog, and the page waits for it
  await waitFor(() => expect(option.startsWith('import,')).toBe(true))
  await new Promise((resolve) => setTimeout(resolve, 200))
  expect(screen.getByText('Import completed with errors')).toBeInTheDocument()
  expect(screen.queryByText('budget page')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'OK' }))
  expect(await screen.findByText('budget page', {}, { timeout: 5000 })).toBeInTheDocument()
  expect(completed).toBe(true)
})
