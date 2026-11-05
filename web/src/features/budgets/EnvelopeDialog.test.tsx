import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { EnvelopeDialog } from './EnvelopeDialog'

function renderDialog() {
  const onSubmit = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <EnvelopeDialog open budgetCurrencyId="cur-usd" side="expense" onClose={() => {}} onSubmit={onSubmit} />
    </QueryClientProvider>,
  )
  return { onSubmit }
}

beforeEach(() => {
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

// the categories sit behind a picker row, as a budget's accounts do: the form
// keeps its size however many categories there are
it('the categories row opens a checklist of the side’s categories; the ticked ones are submitted', async () => {
  const user = userEvent.setup()
  const { onSubmit } = renderDialog()
  const row = await screen.findByTestId('envelope-categories-field')
  expect(row).toHaveTextContent('0 selected')

  await user.click(row)
  const picker = await screen.findByRole('dialog', { name: 'Categories' })
  expect(within(picker).queryByRole('option', { name: 'Salary' })).not.toBeInTheDocument()
  await user.click(await within(picker).findByRole('option', { name: 'Food' }))
  await user.click(within(picker).getByRole('button', { name: 'OK' }))
  expect(screen.getByTestId('envelope-categories-field')).toHaveTextContent('Food')

  await user.type(screen.getByLabelText('Name'), 'Groceries')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ name: 'Groceries', categories: ['cat-food'] }))
})

// a short checklist has no search: its list takes focus, so a keyboard alone can
// tick categories, and each row reports whether it is ticked
it('the checklist works from the keyboard and reports each row’s tick', async () => {
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByTestId('envelope-categories-field'))
  const picker = await screen.findByRole('dialog', { name: 'Categories' })
  const food = await within(picker).findByRole('option', { name: 'Food' })
  expect(food).toHaveAttribute('aria-checked', 'false')
  expect(within(picker).getByRole('listbox').closest('[cmdk-root]')).toHaveFocus()

  await user.keyboard('{Enter}')
  expect(food).toHaveAttribute('aria-checked', 'true')
})
