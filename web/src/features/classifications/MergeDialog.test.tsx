import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MergeDialog } from './MergeDialog'
import type { ClassificationItem } from './ClassificationList'

const food: ClassificationItem = { id: 'c1', name: 'Food', position: 0, isArchived: 0 }
const groceries: ClassificationItem = { id: 'c2', name: 'Groceries', position: 1, isArchived: 0 }

function renderDialog(props: Partial<Parameters<typeof MergeDialog>[0]> = {}) {
  const onConfirm = vi.fn()
  const onClose = vi.fn()
  render(
    <MergeDialog
      open
      source={food}
      candidates={[food, groceries]}
      warning="All transactions from “Food” will be moved, and “Food” will be deleted."
      onClose={onClose}
      onConfirm={onConfirm}
      {...props}
    />,
  )
  return { onConfirm, onClose }
}

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

// The acted-on row is the one that disappears, so offering it as its own target
// would be an instant, irreversible mistake (and the server refuses it anyway).
it('excludes the source from the candidate list', () => {
  renderDialog()
  expect(screen.queryByRole('option', { name: 'Food' })).not.toBeInTheDocument()
  expect(screen.getByRole('option', { name: 'Groceries' })).toBeInTheDocument()
})

it('confirms with the chosen target id', async () => {
  const { onConfirm } = renderDialog()
  await userEvent.click(screen.getByRole('option', { name: 'Groceries' }))
  await userEvent.click(screen.getByRole('button', { name: /merge/i }))
  expect(onConfirm).toHaveBeenCalledWith('c2')
})

// There is no undo, so a stray click on an unfocused dialog must not merge.
it('keeps the confirm disabled until a target is picked', async () => {
  const { onConfirm } = renderDialog()
  const confirm = screen.getByRole('button', { name: /merge/i })
  expect(confirm).toBeDisabled()
  await userEvent.click(confirm)
  expect(onConfirm).not.toHaveBeenCalled()
})

// The copy names the deletion explicitly — that IS the irreversibility warning,
// so it must survive any rewording of the string.
it('shows the warning naming what moves and what is deleted', () => {
  renderDialog()
  expect(screen.getByText(/will be moved.*will be deleted/i)).toBeInTheDocument()
})

it('filters candidates by the search field', async () => {
  renderDialog({ candidates: [food, groceries, { id: 'c3', name: 'Transport', position: 2, isArchived: 0 }] })
  await userEvent.type(screen.getByLabelText(/search/i), 'trans')
  expect(screen.getByRole('option', { name: 'Transport' })).toBeInTheDocument()
  expect(screen.queryByRole('option', { name: 'Groceries' })).not.toBeInTheDocument()
})
