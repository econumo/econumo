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

// The real ImportCsvDialog: its success path is what settles the import step during onboarding.

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
  await user.upload(await screen.findByLabelText('CSV File'), new File([CSV], 'bank.csv', { type: 'text/csv' }))
  await screen.findByText(/Map the columns/)
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import' }))
  expect(await screen.findByRole('heading', { name: 'What do you spend on?' }, { timeout: 5000 })).toBeInTheDocument()
  await screen.findByRole('button', { name: 'Archive Food' })
  expect(writes[0]).toEqual({ path: 'finish-import', body: { id: expect.any(String) } })
  expect(writes.filter((w) => w.path === 'finish-import')).toHaveLength(1)
  expect(writes.filter((w) => w.path === 'update-onboarding' && String((w.body as { value: string }).value).split(',').includes('import'))).toEqual([])
  await waitFor(() => expect(screen.getByRole('button', { name: /Import/ })).toHaveAttribute('data-state', 'done'))
})
