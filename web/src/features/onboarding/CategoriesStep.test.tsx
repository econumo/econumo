import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CategoriesStep } from './CategoriesStep'
import type { CategoryRow } from './draft'

function renderStep(props: Partial<React.ComponentProps<typeof CategoriesStep>> = {}) {
  const onContinue = vi.fn()
  const onSkip = vi.fn()
  render(<CategoriesStep onContinue={onContinue} onSkip={onSkip} pending={false} {...props} />)
  return { onContinue, onSkip }
}

const submitted = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[0][0] as CategoryRow[]

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('lists all 12 defaults checked and submits them', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  expect(screen.getAllByRole('checkbox')).toHaveLength(12)
  screen.getAllByRole('checkbox').forEach((c) => expect(c).toBeChecked())
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const rows = submitted(onContinue)
  expect(rows).toHaveLength(12)
  expect(rows.filter((r) => r.type === 'income').map((r) => r.name)).toEqual(['Salary', 'Other income'])
})

it('leaves an unticked category out', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.click(screen.getByRole('checkbox', { name: 'Kids' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const rows = submitted(onContinue)
  expect(rows).toHaveLength(11)
  expect(rows.map((r) => r.name)).not.toContain('Kids')
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
  expect(submitted(onContinue)[0]).toMatchObject({ name: 'Food', icon: 'account_balance_wallet', type: 'expense', checked: true })
})

it('adds your own expense and income rows on Enter and blur', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'Pets{Enter}')
  await user.type(within(screen.getByTestId('group-income')).getByPlaceholderText('Add your own'), 'Gifts')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const rows = submitted(onContinue)
  expect(rows.find((r) => r.name === 'Pets')).toMatchObject({ type: 'expense', checked: true })
  expect(rows.find((r) => r.name === 'Gifts')).toMatchObject({ type: 'income', checked: true })
})

it('merges duplicate names within a type, case-insensitively', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'groceries{Enter}')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).filter((r) => r.name.toLowerCase() === 'groceries')).toHaveLength(1)
})

it('disables Continue when nothing is ticked', async () => {
  const user = userEvent.setup()
  renderStep()
  for (const c of screen.getAllByRole('checkbox')) await user.click(c)
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('seeds from initialRows keeping keys, and locks created rows', async () => {
  const user = userEvent.setup()
  const rows: CategoryRow[] = [
    { key: 'k1', name: 'Food', type: 'expense', icon: 'cake', checked: true },
    { key: 'k2', name: 'Fun', type: 'expense', icon: 'movie', checked: true },
  ]
  const { onContinue } = renderStep({ initialRows: rows, createdKeys: new Set(['k1']) })
  expect(screen.getAllByRole('checkbox')).toHaveLength(2)
  const first = screen.getAllByTestId('category-row')[0]
  expect(within(first).getByLabelText('Category name')).toHaveAttribute('readonly')
  expect(within(first).getByRole('checkbox')).toBeDisabled()
  expect(within(first).getByRole('checkbox')).toBeChecked()
  expect(within(first).getByRole('button', { name: 'Change icon' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).map((r) => r.key)).toEqual(['k1', 'k2'])
})

it('disables Skip and Continue while pending, and Skip calls onSkip', async () => {
  const user = userEvent.setup()
  const { onSkip } = renderStep()
  await user.click(screen.getByRole('button', { name: 'Skip setup' }))
  expect(onSkip).toHaveBeenCalled()
})

it('is inert while pending', () => {
  renderStep({ pending: true })
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Skip setup' })).toBeDisabled()
})
