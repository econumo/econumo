import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureBudgets } from '@/test/fixtures'
import { BudgetDialog } from './BudgetDialog'
import { toggleInPicker } from './budgetFormTestUtils'

function renderDialog(onSubmit = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <BudgetDialog open onClose={vi.fn()} onSubmit={onSubmit} />
    </QueryClientProvider>,
  )
  return onSubmit
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('sends savingsAccountIds as a subset of accountIds', async () => {
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Vacation')
  await toggleInPicker(user, 'accounts', 'include Cash')
  await toggleInPicker(user, 'accounts', 'include Bank')
  await toggleInPicker(user, 'savings', 'Cash is a savings account')
  await toggleInPicker(user, 'savings', 'Bank is a savings account')
  // excluding a savings account drops it from the savings set too
  await toggleInPicker(user, 'accounts', 'include Cash')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  const form = onSubmit.mock.calls[0][0]
  expect(form.accountIds).toEqual(['a2'])
  expect(form.savingsAccountIds).toEqual(['a2'])
})

it('sends an empty savings set when no account is marked savings', async () => {
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Vacation')
  await toggleInPicker(user, 'accounts', 'include Cash')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  expect(onSubmit.mock.calls[0][0].savingsAccountIds).toEqual([])
})

it('submits the default period start day 1', async () => {
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Vacation')
  await toggleInPicker(user, 'accounts', 'include Cash')
  expect(screen.getByRole('combobox', { name: 'Month starts on day' })).toHaveTextContent('1')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  expect(onSubmit.mock.calls[0][0].periodStartDay).toBe(1)
})

it('submits the picked period start day', async () => {
  const user = userEvent.setup()
  const onSubmit = renderDialog()
  await user.type(screen.getByLabelText('Name'), 'Vacation')
  await toggleInPicker(user, 'accounts', 'include Cash')
  await user.click(screen.getByRole('combobox', { name: 'Month starts on day' }))
  await user.click(await screen.findByRole('option', { name: '28' }))
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  expect(onSubmit.mock.calls[0][0].periodStartDay).toBe(28)
  expect(screen.queryByText(/Existing plans stay/)).toBeNull()
})

describe('a server that does not know the period start day', () => {
  // resolves once the list has been served, so a test can tell "still loading" from "loaded"
  function budgetList(items: unknown[]) {
    let served = false
    server.use(
      http.get('*/api/v1/budget/get-budget-list', () => {
        served = true
        return HttpResponse.json({ success: true, message: '', data: { items } })
      }),
    )
    return () => served
  }
  const picker = () => screen.queryByRole('combobox', { name: 'Month starts on day' })
  const settled = async (served: () => boolean) => {
    await waitFor(() => expect(served()).toBe(true))
    await new Promise((r) => setTimeout(r, 50))
  }

  it('hides the picker when the loaded budgets carry no day', async () => {
    const served = budgetList(fixtureBudgets.map(({ periodStartDay: _d, ...b }) => b))
    renderDialog()
    await settled(served)
    await waitFor(() => expect(picker()).toBeNull())
  })

  it('shows the picker when the budgets carry a day', async () => {
    const served = budgetList(fixtureBudgets)
    renderDialog()
    await settled(served)
    expect(picker()).toBeInTheDocument()
  })

  it('shows the picker when there is no budget yet', async () => {
    const served = budgetList([])
    renderDialog()
    await settled(served)
    expect(picker()).toBeInTheDocument()
  })
})
