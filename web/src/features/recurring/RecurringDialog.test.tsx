import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureCategories, fixtureFolders, fixturePayees, fixtureTags } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { formatDateTime } from '@/lib/datetime'
import { useUiStore } from '@/app/uiStore'
import type { RecurringDto } from '@/api/dto/recurring'
import { RecurringDialog } from './RecurringDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  // In the real app the dialog only becomes reachable once the app shell has
  // finished its initial load, so accounts/categories/etc. are already cached
  // by the time RecurringDialog mounts — prime the cache here to match.
  queryClient.setQueryData(queryKeys.accounts, fixtureAccounts)
  queryClient.setQueryData(queryKeys.folders, fixtureFolders)
  queryClient.setQueryData(queryKeys.categories, fixtureCategories)
  queryClient.setQueryData(queryKeys.payees, fixturePayees)
  queryClient.setQueryData(queryKeys.tags, fixtureTags)
  const router = createMemoryRouter([{ path: '/', element: <RecurringDialog /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

// a year out, so the "future" assumptions this fixture relies on never age into failures
const futurePaymentAt = formatDateTime(new Date(Date.now() + 365 * 24 * 3600 * 1000))

const wireRecurringAsDto = {
  id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: 50.5, categoryId: 'cat-food', payeeId: null, tagId: null, description: 'rent',
  schedule: 'weekly', nextPaymentAt: futurePaymentAt,
} as unknown as RecurringDto

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
  useUiStore.setState({ recurringModal: null })
})

it('creates a template', async () => {
  server.use(
    http.post('*/api/v1/recurring/create-recurring-transaction', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { ...body, ownerUserId: 'u1', amount: String(body.amount) } } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openRecurringModal({})
  await screen.findByRole('heading', { name: 'Add transaction' })

  await user.type(await screen.findByLabelText('Amount'), '10')
  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  await user.click(await screen.findByText('Food'))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(useUiStore.getState().recurringModal).toBeNull())
})

it('edit mode prefills the amount and schedule, under the add-transaction headers', async () => {
  renderDialog()
  useUiStore.getState().openRecurringModal({ recurring: wireRecurringAsDto })
  // same headers as TransactionDialog: the template form is that dialog + one row
  await screen.findByRole('heading', { name: 'Edit transaction' })
  expect(screen.getByRole('button', { name: 'Update' })).toBeInTheDocument()
  expect(screen.getByLabelText('Amount')).toHaveValue('50.5')
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('Weekly')
})

it('the next payment is the header date chip, not a separate row', async () => {
  renderDialog()
  useUiStore.getState().openRecurringModal({ recurring: wireRecurringAsDto })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  // same control as the add-transaction dialog: a chip plus a back-a-day arrow
  expect(screen.getByRole('button', { name: 'date' })).toHaveTextContent(wireRecurringAsDto.nextPaymentAt.slice(0, 10))
  expect(screen.getByRole('button', { name: 'previous day' })).toBeInTheDocument()
  expect(screen.queryByLabelText('Next payment')).toBeNull()
})

it('the back-a-day arrow moves the next payment date', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openRecurringModal({ recurring: wireRecurringAsDto })
  const chip = await screen.findByRole('button', { name: 'date' })
  const before = chip.textContent
  await user.click(screen.getByRole('button', { name: 'previous day' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'date' }).textContent).not.toBe(before))
})

it('tags render as the transaction dialog pill row, toggling the template tag', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openRecurringModal({ recurring: wireRecurringAsDto })
  // a pill (role=checkbox), not an option in a select
  const pill = await screen.findByRole('checkbox', { name: 'vacation' })
  expect(pill).toHaveAttribute('aria-checked', 'false')
  await user.click(pill)
  expect(await screen.findByRole('checkbox', { name: 'vacation' })).toHaveAttribute('aria-checked', 'true')
  await user.click(pill)
  expect(await screen.findByRole('checkbox', { name: 'vacation' })).toHaveAttribute('aria-checked', 'false')
})

it('Repeats is the only row the add-transaction dialog does not have', async () => {
  renderDialog()
  useUiStore.getState().openRecurringModal({ recurring: wireRecurringAsDto })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  const labels = Array.from(document.querySelectorAll('form label')).map((el) => el.textContent)
  expect(labels).toContain('Repeats')
  expect(labels).not.toContain('Next payment')
})
