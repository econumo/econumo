import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories } from '@/test/fixtures'
import { CategoriesStep, CategoriesStepBody, type ExistingCategory } from './CategoriesStep'
import { emptyDraft, loadDraft, saveDraft, type CategoryRow } from './draft'

function renderStep(props: Partial<React.ComponentProps<typeof CategoriesStep>> = {}) {
  const onContinue = vi.fn()
  const onSkip = vi.fn()
  render(<CategoriesStep existing={[]} onContinue={onContinue} onSkip={onSkip} pending={false} {...props} />)
  return { onContinue, onSkip }
}

const submitted = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[0][0] as CategoryRow[]
const chosen = (fn: ReturnType<typeof vi.fn>) => submitted(fn).filter((r) => r.checked)

const food: ExistingCategory = { id: 'cat-1', name: 'Groceries', type: 'expense', icon: 'cake' }
const pay: ExistingCategory = { id: 'cat-2', name: 'Paycheck', type: 'income', icon: 'work' }

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('lists all 12 defaults ticked, checkbox after the name, in one card per type', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  expect(screen.getAllByRole('checkbox')).toHaveLength(12)
  screen.getAllByRole('checkbox').forEach((c) => expect(c).toBeChecked())
  const first = screen.getAllByTestId('category-row')[0]
  const name = within(first).getByLabelText('Category name')
  expect(name.compareDocumentPosition(within(first).getByRole('checkbox')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(within(screen.getByTestId('group-expense')).getAllByTestId('category-row')).toHaveLength(10)
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const rows = chosen(onContinue)
  expect(rows).toHaveLength(12)
  expect(rows.filter((r) => r.type === 'income').map((r) => r.name)).toEqual(['Salary', 'Other income'])
})

it('submits an unticked category unchecked, so a reload keeps it unticked', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.click(screen.getByRole('checkbox', { name: 'Kids' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(chosen(onContinue).map((r) => r.name)).not.toContain('Kids')
  expect(submitted(onContinue).find((r) => r.name === 'Kids')).toMatchObject({ checked: false })
})

it('passes a rename and an icon change through', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  const first = screen.getAllByTestId('category-row')[0]
  const input = within(first).getByLabelText('Category name')
  await user.clear(input)
  await user.type(input, '  Food ')
  await user.click(within(first).getByRole('button', { name: 'Change icon' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('option', { name: 'account_balance_wallet' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(chosen(onContinue)[0]).toMatchObject({ name: 'Food', icon: 'account_balance_wallet', type: 'expense', checked: true })
})

it('adds your own expense and income rows on Enter and blur', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'Pets{Enter}')
  await user.type(within(screen.getByTestId('group-income')).getByPlaceholderText('Add your own'), 'Gifts')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const rows = chosen(onContinue)
  expect(rows.find((r) => r.name === 'Pets')).toMatchObject({ type: 'expense', checked: true })
  expect(rows.find((r) => r.name === 'Gifts')).toMatchObject({ type: 'income', checked: true })
})

it('merges duplicate names within a type, case-insensitively', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'groceries{Enter}')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(chosen(onContinue).filter((r) => r.name.toLowerCase() === 'groceries')).toHaveLength(1)
})

it('disables Continue when nothing is ticked on a fresh account', async () => {
  const user = userEvent.setup()
  renderStep()
  for (const c of screen.getAllByRole('checkbox')) await user.click(c)
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('lists existing categories first, ticked and locked, and omits defaults they already cover', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [food, pay] })
  const expense = within(screen.getByTestId('group-expense')).getAllByTestId('category-row')
  expect(expense[0]).toHaveTextContent('Groceries')
  expect(within(expense[0]).queryByLabelText('Category name')).not.toBeInTheDocument()
  expect(within(expense[0]).getByRole('checkbox')).toBeChecked()
  expect(within(expense[0]).getByRole('checkbox')).toBeDisabled()
  expect(within(expense[0]).queryByRole('button', { name: 'Change icon' })).not.toBeInTheDocument()
  expect(screen.getAllByText('Groceries')).toHaveLength(1)
  expect(within(screen.getByTestId('group-income')).getAllByTestId('category-row')[0]).toHaveTextContent('Paycheck')
  // with categories already there the remaining defaults are offered, not pre-ticked
  expect(screen.getByRole('checkbox', { name: 'Housing' })).not.toBeChecked()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(chosen(onContinue)).toEqual([])
})

it('never submits a new row named like an existing category', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [food] })
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'GROCERIES{Enter}')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(chosen(onContinue)).toEqual([])
})

it('reseeds from a stored draft keeping keys, showing created ones as existing only', async () => {
  const user = userEvent.setup()
  const rows: CategoryRow[] = [
    { key: 'k1', name: 'Groceries', type: 'expense', icon: 'cake', checked: true },
    { key: 'k2', name: 'Fun', type: 'expense', icon: 'movie', checked: true },
    { key: 'k3', name: 'Kids', type: 'expense', icon: 'child_care', checked: false },
  ]
  const { onContinue } = renderStep({ initialRows: rows, existing: [food] })
  expect(screen.getAllByTestId('category-row')).toHaveLength(3)
  expect(screen.getByRole('checkbox', { name: 'Kids' })).not.toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(chosen(onContinue).map((r) => r.key)).toEqual(['k2'])
})

it('Skip calls onSkip', async () => {
  const user = userEvent.setup()
  const { onSkip } = renderStep()
  await user.click(screen.getByRole('button', { name: 'Skip' }))
  expect(onSkip).toHaveBeenCalled()
})

it('is inert while pending', () => {
  renderStep({ pending: true })
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Skip' })).toBeDisabled()
})

it('flags a renamed or added category outside 3-64 characters and blocks Continue until it is fixed or unticked', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  const first = screen.getAllByTestId('category-row')[0]
  const input = within(first).getByLabelText('Category name')
  await user.clear(input)
  await user.type(input, 'Fo')
  expect(screen.queryByText('Category name must be 3-64 characters')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.tab()
  expect(within(first).getByText('Category name must be 3-64 characters')).toBeInTheDocument()
  expect(input).toHaveAttribute('aria-invalid', 'true')

  await user.type(input, 'od')
  expect(screen.queryByText('Category name must be 3-64 characters')).not.toBeInTheDocument()

  await user.type(within(screen.getByTestId('group-income')).getByPlaceholderText('Add your own'), 'Tx{Enter}')
  const added = screen.getAllByTestId('category-row').at(-1)!
  expect(within(added).getByText('Category name must be 3-64 characters')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.click(within(added).getByRole('checkbox'))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(chosen(onContinue).map((r) => r.name)).not.toContain('Tx')
})

describe('CategoriesStepBody', () => {
  const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })
  let created: string[]

  function renderBody(categories: unknown[]) {
    created = []
    server.use(
      ...coreHandlers({ categories }),
      http.post('*/api/v1/category/create-category', async ({ request }) => {
        const body = (await request.json()) as { id: string; name: string; type: string }
        created.push(body.name)
        return ok({ item: { ...fixtureCategories[0], id: `new-${created.length}`, name: body.name, type: body.type } })
      }),
    )
    const onDone = vi.fn(() => Promise.resolve())
    const onSkip = vi.fn(() => Promise.resolve())
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <CategoriesStepBody onDone={onDone} onSkip={onSkip} />
      </QueryClientProvider>,
    )
    return { onDone, onSkip }
  }

  beforeEach(() => localStorage.clear())

  it('a stored draft with some rows created resumes without duplicates, then moves on', async () => {
    const rows: CategoryRow[] = [
      { key: 'k1', name: 'Food', type: 'expense', icon: 'restaurant', checked: true },
      { key: 'k2', name: 'Fun', type: 'expense', icon: 'movie', checked: true },
    ]
    saveDraft('u1', { ...emptyDraft(), ops: { 'category:k1': 'op1', 'category:k2': 'op2' }, categoryRows: rows, createdCategories: { k1: 'cat-food' } })
    const user = userEvent.setup()
    const { onDone } = renderBody(fixtureCategories)
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    expect(screen.getAllByText('Food')).toHaveLength(1)
    expect(screen.getByRole('checkbox', { name: 'Food' })).toBeDisabled()
    expect(screen.queryByText('Old')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(created).toEqual(['Fun'])
    expect(loadDraft('u1')!.createdCategories).toEqual({ k1: 'cat-food', k2: 'new-1' })
  })

  it('a failed save shows the error and stays on the step', async () => {
    const user = userEvent.setup()
    const { onDone } = renderBody([])
    server.use(http.post('*/api/v1/category/create-category', () => HttpResponse.json({ success: false, message: 'Nope', code: 400, errors: {} }, { status: 500 })))
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Nope')
    expect(onDone).not.toHaveBeenCalled()
  })
})
