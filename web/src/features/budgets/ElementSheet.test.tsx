import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget } from '@/test/fixtures'
import type { BudgetCommentDto, BudgetElementDto, PlanElementDto } from '@/api/dto/budget'
import { makeBudgetExchange } from './budgetMath'
import { ElementSheet } from './ElementSheet'
import type { ElementSheetProps } from './ElementSheet'
import { formatPlanMonth } from './planMath'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }
const budget = coerceBudgetFixture(fixtureWireBudget)
const food = budget.structure.elements.find((el) => el.id === 'cat-food')!
const living = budget.structure.elements.find((el) => el.id === 'env-1')!
// the one month-label rule: "July" in the current year, "Jul 2026" in any other
const july = formatPlanMonth('2026-07-01', 'en')

const comment = (id: string, text: string, createdAt: string): BudgetCommentDto => ({
  id, elementId: 'cat-food', period: '2026-07-01', comment: text,
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, createdAt, updatedAt: createdAt,
})
const planElement = (over: Partial<PlanElementDto>): PlanElementDto =>
  ({ id: 'x', type: 1, name: 'X', icon: 'tag', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 0, ownerUserId: null, cells: [], children: [], ...over }) as PlanElementDto

function renderSheet(overrides: Partial<ElementSheetProps> = {}) {
  const props: ElementSheetProps = {
    target: { kind: 'expense', element: food },
    month: '2026-07-01',
    baseCurrencyId: 'cur-usd',
    currencies: [usd, eur],
    exchange: makeBudgetExchange(budget, [usd, eur]),
    comments: [],
    commentsReadOnly: false,
    canSetAmount: true,
    onClose: vi.fn(),
    onSetAmount: vi.fn(),
    onOpenComments: vi.fn(),
    onShowTransactions: vi.fn(),
    ...overrides,
  }
  render(<ElementSheet {...props} />)
  return props
}

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('titles the sheet with the item and month and shows Budget, Spent and Available', () => {
  renderSheet()
  expect(screen.getByText(`Food · ${july}`)).toBeInTheDocument()
  expect(screen.getByTestId('sheet-figure-budget')).toHaveTextContent('Budget200.00')
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('Spent45.50')
  expect(screen.getByTestId('sheet-figure-available')).toHaveTextContent('Available354.50')
  expect(screen.queryByTestId('sheet-state')).toBeNull()
  expect(screen.queryByTestId('sheet-rate')).toBeNull()
})

it('explains a month over budget that carry-over still covers', () => {
  const over: BudgetElementDto = { ...food, budgeted: '700', spent: '801.37', budgetSpent: '801.37', available: '-50.68' }
  renderSheet({ target: { kind: 'expense', element: over } })
  expect(screen.getByTestId('sheet-state')).toHaveTextContent('Over by 101.37 — covered by 750.69 left from earlier months')
})

it('adds no sentence for an overspend: the red Available already says it', () => {
  const over: BudgetElementDto = { ...food, budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' }
  renderSheet({ target: { kind: 'expense', element: over } })
  expect(screen.queryByTestId('sheet-state')).toBeNull()
  expect(screen.getByTestId('sheet-figure-available')).toHaveTextContent('-20.00')
})

it('centres each figure in its column', () => {
  renderSheet()
  expect(screen.getByTestId('sheet-figure-budget').className).toContain('text-center')
})

it('shows a dash for Spent and no state sentence in a future month', () => {
  const over: BudgetElementDto = { ...food, budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' }
  renderSheet({ target: { kind: 'expense', element: over }, month: '2099-01-01' })
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('Spent—')
  expect(screen.queryByTestId('sheet-state')).toBeNull()
})

it('tags every amount of a foreign-currency item and adds the converted line and rate note', () => {
  const spending: BudgetElementDto = { ...living, spent: '10', budgetSpent: '11.11' }
  renderSheet({ target: { kind: 'expense', element: spending } })
  expect(screen.getByTestId('sheet-figure-budget')).toHaveTextContent('90.00 EUR')
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('10.00 EUR')
  expect(screen.getByTestId('sheet-converted')).toHaveTextContent('≈ 11.11 USD')
  expect(screen.getByTestId('sheet-rate')).toHaveTextContent(new RegExp(`^Average rate for ${july}: 1 USD = [\\d.,]+ EUR$`))
})

it('takes the rate note from the exchange it is given (a Plan column month)', () => {
  const eurRow = planElement({ id: 'env-eur', type: 0, name: 'Euro Stash', currencyId: 'cur-eur' })
  renderSheet({
    target: { kind: 'plan', cell: { element: eurRow, planned: '100', actual: '35' } },
    month: '2026-05-01',
    exchange: (from, to, amount) => (from === 'cur-usd' && to === 'cur-eur' ? '0.5' : amount),
  })
  expect(screen.getByTestId('sheet-rate')).toHaveTextContent(`Average rate for ${formatPlanMonth('2026-05-01', 'en')}: 1 USD = 0.5 EUR`)
})

it('previews the two latest comments, oldest first, and links to the thread', async () => {
  const props = renderSheet({
    comments: [
      comment('c3', 'Back to 700 next month', '2026-07-20 09:00:00'),
      comment('c1', 'First', '2026-07-01 09:00:00'),
      comment('c2', 'Second', '2026-07-10 09:00:00'),
    ],
  })
  const shown = screen.getAllByTestId('sheet-comment')
  expect(shown).toHaveLength(2)
  expect(shown[0]).toHaveTextContent('Ada Second')
  expect(shown[1]).toHaveTextContent('Ada Back to 700 next month')
  await userEvent.click(screen.getByRole('button', { name: 'Comments (3)' }))
  expect(props.onOpenComments).toHaveBeenCalled()
})

it('says there are no comments yet, in gray, and offers "Add comment" on a writable thread', () => {
  renderSheet()
  const empty = screen.getByTestId('sheet-no-comments')
  expect(empty).toHaveTextContent('No comments yet.')
  expect(empty.className).toContain('text-muted-foreground')
  expect(screen.getByRole('button', { name: 'Add comment' })).toBeInTheDocument()
})

it('a read-only empty thread says so but offers no link', () => {
  renderSheet({ commentsReadOnly: true })
  expect(screen.getByTestId('sheet-no-comments')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Add comment' })).toBeNull()
})

it('routes Set budget and Transactions', async () => {
  const props = renderSheet()
  await userEvent.click(screen.getByRole('button', { name: 'Set budget' }))
  expect(props.onSetAmount).toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Transactions' }))
  expect(props.onShowTransactions).toHaveBeenCalled()
})

it('has no Set budget when the amount cannot be set here', () => {
  renderSheet({ canSetAmount: false })
  expect(screen.queryByRole('button', { name: 'Set budget' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Transactions' })).toBeInTheDocument()
})

it('shows the uncategorized row as Spent only, with no comments link', () => {
  const uncategorized: BudgetElementDto = { ...food, id: 'uncategorized', name: 'Uncategorized', budgeted: '0', available: '0', spent: '12', budgetSpent: '12' }
  renderSheet({ target: { kind: 'expense', element: uncategorized }, canSetAmount: false })
  expect(screen.queryByTestId('sheet-figure-budget')).toBeNull()
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('12.00')
  expect(screen.queryByRole('button', { name: /comment/i })).toBeNull()
})

it('an income plan cell shows Planned and Received and offers Set plan', () => {
  const salaries = planElement({ id: 'ie1', type: 4, name: 'Salaries' })
  renderSheet({ target: { kind: 'plan', cell: { element: salaries, planned: '2000', actual: '400' } }, onShowTransactions: undefined })
  expect(screen.getByTestId('sheet-figure-planned')).toHaveTextContent('Planned2,000.00')
  expect(screen.getByTestId('sheet-figure-received')).toHaveTextContent('Received400.00')
  expect(screen.getByRole('button', { name: 'Set plan' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Transactions' })).toBeNull()
})

it('an expense plan cell shows Budget and Spent, without Available or a state sentence', () => {
  const pe1 = planElement({ id: 'pe1', type: 0, name: 'Living' })
  renderSheet({ target: { kind: 'plan', cell: { element: pe1, planned: '100', actual: '150' } } })
  expect(screen.getByTestId('sheet-figure-budget')).toHaveTextContent('100.00')
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('150.00')
  expect(screen.queryByTestId('sheet-figure-available')).toBeNull()
  expect(screen.queryByTestId('sheet-state')).toBeNull()
  expect(screen.getByRole('button', { name: 'Set budget' })).toBeInTheDocument()
})

it('a savings row (monthly or plan cell) shows Planned, Saved and the month-end Balance', () => {
  const saving = { id: 'acc-s1', type: 5 as const, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0 as const, position: 0, budgeted: '100', spent: '40', available: '60', closingBalance: '1040' }
  renderSheet({ target: { kind: 'savings', row: saving }, onShowTransactions: undefined })
  expect(screen.getByTestId('sheet-figure-planned')).toHaveTextContent('100.00')
  expect(screen.getByTestId('sheet-figure-saved')).toHaveTextContent('40.00')
  expect(screen.getByTestId('sheet-figure-balance')).toHaveTextContent('Balance1,040.00')
  expect(screen.getByRole('button', { name: 'Set plan' })).toBeInTheDocument()
})

it('a savings plan cell shows its closing balance too', () => {
  const s = planElement({ id: 'acc-s1', type: 5, name: 'Rainy day' })
  renderSheet({ target: { kind: 'plan', cell: { element: s, planned: '100', actual: '40', closingBalance: '1040' } }, onShowTransactions: undefined })
  expect(screen.getByTestId('sheet-figure-saved')).toHaveTextContent('40.00')
  expect(screen.getByTestId('sheet-figure-balance')).toHaveTextContent('1,040.00')
  expect(screen.getByRole('button', { name: 'Set plan' })).toBeInTheDocument()
})

it('renders nothing without a target', () => {
  renderSheet({ target: null })
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('shows the item icon beside the title', () => {
  renderSheet()
  const dialog = screen.getByRole('dialog', { name: `Food · ${july}` })
  expect(within(dialog).getByText('restaurant')).toHaveAttribute('aria-hidden', 'true')
})

it('offers an active Edit button when the item can be edited', async () => {
  const onEdit = vi.fn()
  renderSheet({ onEdit, canEdit: true })
  await userEvent.click(screen.getByRole('button', { name: 'Edit' }))
  expect(onEdit).toHaveBeenCalled()
})

it('keeps the Edit button visible but inactive without the right to edit', () => {
  renderSheet({ onEdit: vi.fn(), canEdit: false })
  expect(screen.getByRole('button', { name: 'Edit' })).toBeDisabled()
})

it('has no Edit button when there is nothing to edit', () => {
  renderSheet()
  expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
})

it('puts the primary action on the right of Transactions', () => {
  renderSheet()
  const buttons = screen.getAllByRole('button').map((b) => b.textContent)
  expect(buttons.indexOf('Transactions')).toBeLessThan(buttons.indexOf('Set budget'))
})
