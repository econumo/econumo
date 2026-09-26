import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { BudgetDialog } from './BudgetDialog'

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
  await user.click(await screen.findByRole('switch', { name: 'include Cash' }))
  await user.click(screen.getByRole('switch', { name: 'include Bank' }))
  await user.click(screen.getByRole('switch', { name: 'Cash is a savings account' }))
  await user.click(screen.getByRole('switch', { name: 'Bank is a savings account' }))
  // excluding a savings account drops it from the savings set too
  await user.click(screen.getByRole('switch', { name: 'include Cash' }))
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
  await user.click(await screen.findByRole('switch', { name: 'include Cash' }))
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  expect(onSubmit.mock.calls[0][0].savingsAccountIds).toEqual([])
})
