import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureLabels, fixturePayees, fixtureTags } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import { DebtDialog } from './DebtDialog'
import { wireDebtNotSetUp } from './testDebt'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  queryClient.setQueryData(queryKeys.categories, fixtureCategories)
  queryClient.setQueryData(queryKeys.payees, fixturePayees)
  queryClient.setQueryData(queryKeys.tags, fixtureTags)
  queryClient.setQueryData(queryKeys.labels, fixtureLabels)
  const router = createMemoryRouter([{ path: '/', element: <DebtDialog /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

function captureCreate(answer: (body: Record<string, unknown>, call: number) => Response = () => ok({ ...wireDebtNotSetUp, id: 'd-new' })) {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/debt/create-debt', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      bodies.push(body)
      return answer(body, bodies.length)
    }),
  )
  return bodies
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
  useUiStore.setState({ debtModal: null, debtDetailsId: null })
})

async function openCreate(params = {}) {
  useUiStore.getState().openDebtModal(params)
  await screen.findByRole('heading', { name: 'Debt' })
}

it('create form asks only name, payments from, payment and repeats', async () => {
  renderDialog()
  await openCreate()

  expect(screen.getByText('Track a loan without an account. You can fine-tune it afterwards.')).toBeInTheDocument()
  expect(screen.getByLabelText('Name')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Category: choose a category' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Reporting tag (optional): any reporting tag' })).toBeInTheDocument()
  expect(screen.getByLabelText('Payment')).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('Monthly')
  await waitFor(() => expect(screen.getByRole('button', { name: /^Currency/ })).toHaveTextContent('USD'))

  expect(screen.queryByLabelText('Balance now')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Last payment date' })).toBeNull()
  expect(screen.queryByText('Start date (optional)')).toBeNull()
})

it('converting from a category locks the category and prefills the name', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food', currencyId: 'cur-eur', fromBudget: true })

  expect(screen.getByLabelText('Name')).toHaveValue('Food')
  expect(screen.getByRole('button', { name: 'Category: Food' })).toBeDisabled()
  await waitFor(() => expect(screen.getByRole('button', { name: /^Currency/ })).toHaveTextContent('EUR'))

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toEqual({
    name: 'Food', currencyId: 'cur-eur', categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null,
    startDate: null, payment: '50', schedule: 'monthly',
  })
})

it('saving opens the debt\'s review screen', async () => {
  captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(useUiStore.getState().debtModal).toBeNull())
  expect(useUiStore.getState().debtDetailsId).toBe('d-new')
})

it('a key conflict shows its message under the sentence and keeps the start date hidden', async () => {
  const conflict = 'Another debt could match the same payments.'
  const bodies = captureCreate(() =>
    HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { categoryId: [conflict] } }, { status: 400 }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText(conflict)).toBeInTheDocument()
  expect(bodies).toHaveLength(1)
  expect(screen.queryByRole('button', { name: 'Start date (optional)' })).toBeNull()
  expect(screen.queryByText('Start date (optional)')).toBeNull()
  expect(useUiStore.getState().debtModal).not.toBeNull()
})

it('a start date already taken reveals the start date field and resends with it', async () => {
  const taken = 'Another debt with the same category, payee, tag and reporting tag already starts on this date.'
  const bodies = captureCreate((_, call) =>
    call === 1
      ? HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { startDate: [taken] } }, { status: 400 })
      : ok({ ...wireDebtNotSetUp, id: 'd-new' }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText(taken)).toBeInTheDocument()
  expect(useUiStore.getState().debtModal).not.toBeNull()

  await user.click(screen.getByRole('button', { name: 'Start date (optional)' }))
  const now = new Date()
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  await user.click(await screen.findByRole('button', { name: new RegExp(`${month} 1st, ${now.getFullYear()}`) }))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[0].startDate).toBeNull()
  expect(bodies[1].startDate).toBe(`${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-01`)
  await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBe('d-new'))
})

it('picks the label in the sentence and sends it', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.click(screen.getByRole('button', { name: 'Reporting tag (optional): any reporting tag' }))
  const sheet = await screen.findByRole('dialog', { name: 'Reporting tag (optional)' })
  await user.click(within(sheet).getByText('health'))
  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0].labelId).toBe('label1')
})

it('requires the name, category and payment before sending', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findAllByText('Required field')).toHaveLength(3)
  expect(bodies).toHaveLength(0)
})
