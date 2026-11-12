import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import type { DebtDto } from '@/api/dto/debt'
import { METRICS, trackEvent } from '@/lib/metrics'
import { AddToDebtDialog } from './AddToDebtDialog'
import { wireDebt, wireRule } from './testDebt'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

const mortgage = (id: string, ownerUserId: string, ownerName: string) => ({ ...wireRule, id: `r-${id}`, categoryId: id, categoryName: 'Mortgage', categoryIcon: 'home', ownerUserId, ownerName })

// the house: his (the viewer's) Mortgage and Anna's
const house: DebtDto = { ...wireDebt, id: 'd-house', name: 'House', icon: 'home', rules: [mortgage('cat-m1', 'u1', 'Ada'), mortgage('cat-m2', 'u2', 'Anna')] }
// already counts the payments in Food
const car: DebtDto = { ...wireDebt, id: 'd-car', name: 'Car loan' }
const elsewhere: DebtDto = { ...wireDebt, id: 'd-other', name: 'Other budget loan', budgetId: 'b2', budgetName: 'Alpha plan' }

function renderDialog(debts: DebtDto[], categoryId = 'cat-food') {
  const onOpenChange = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  queryClient.setQueryData(queryKeys.debts, debts)
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  render(
    <QueryClientProvider client={queryClient}>
      <AddToDebtDialog budgetId="b1" categoryId={categoryId} open onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  )
  return { onOpenChange }
}

const option = (name: RegExp) => screen.findByRole('option', { name })

beforeEach(() => {
  vi.mocked(trackEvent).mockClear()
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  server.use(...coreHandlers())
})

it('lists only the budget\'s debts, each with its rules', async () => {
  renderDialog([house, car, elsewhere])
  expect(await screen.findByRole('dialog', { name: 'Add to a debt' })).toBeInTheDocument()
  const row = await option(/House/)
  expect(row).toHaveTextContent("Payments in Mortgage, Anna's Mortgage")
  expect(await option(/Car loan/)).toBeInTheDocument()
  expect(screen.queryByRole('option', { name: /Other budget loan/ })).toBeNull()
  // a short list needs no search
  expect(screen.queryByRole('combobox')).toBeNull()
})

it('a debt that already counts the category is disabled, with the reason', async () => {
  renderDialog([house, car])
  const row = await option(/Car loan/)
  expect(row).toHaveAttribute('aria-disabled', 'true')
  expect(row).toHaveTextContent('Already counts payments in this category')
})

it('a debt another debt would conflict with is disabled, with the reason', async () => {
  // a payee-narrowed rule on Food would overlap a whole-Food rule on the house
  const groceries: DebtDto = { ...wireDebt, id: 'd-groceries', name: 'Groceries', rules: [{ ...wireRule, payeeId: 'p1', payeeName: 'Grocer' }] }
  renderDialog([house, groceries])
  const row = await option(/House/)
  expect(row).toHaveAttribute('aria-disabled', 'true')
  expect(row).toHaveTextContent('Another debt already counts payments in this category')
})

it('picking one sends add-debt-rule, fires the metric and closes', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/debt/add-debt-rule', async ({ request }) => {
      bodies.push(await request.json())
      return ok(house)
    }),
  )
  const { onOpenChange } = renderDialog([house, car], 'cat-fuel')
  await userEvent.click(await option(/House/))
  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  expect(bodies).toEqual([{ id: 'd-house', rule: { categoryId: 'cat-fuel' } }])
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_RULE_ADD)
})

it('a refusal shows the server\'s message and the dialog stays open', async () => {
  server.use(
    http.post('*/api/v1/debt/add-debt-rule', () =>
      HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { rules: ['These payments are already counted by another debt.'] } }, { status: 400 }),
    ),
  )
  const { onOpenChange } = renderDialog([house])
  await userEvent.click(await option(/House/))
  expect(await screen.findByRole('alert')).toHaveTextContent('These payments are already counted by another debt.')
  expect(onOpenChange).not.toHaveBeenCalledWith(false)
  expect(screen.getByRole('dialog', { name: 'Add to a debt' })).toBeInTheDocument()
})

it('offline every entry is disabled', async () => {
  act(() => onlineManager.setOnline(false))
  onTestFinished(() => act(() => onlineManager.setOnline(true)))
  renderDialog([house])
  const row = await option(/House/)
  expect(row).toHaveAttribute('aria-disabled', 'true')
})

it('a long list is searchable', async () => {
  const many = ['Alpha', 'Bravo', 'Charlie', 'Delta', 'Echo', 'Foxtrot', 'Golf'].map((name, i) => ({ ...house, id: `d${i}`, name }))
  renderDialog(many)
  await option(/Golf/)
  await userEvent.type(screen.getByRole('combobox', { name: 'Search debts' }), 'golf')
  await waitFor(() => expect(screen.getAllByRole('option')).toHaveLength(1))
  expect(within(screen.getByRole('option')).getByText('Golf')).toBeInTheDocument()
})
