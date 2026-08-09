import type { ReactNode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureWireBudget } from '@/test/fixtures'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import type { BudgetElementDto } from '@/api/dto/budget'
import { BudgetTable } from './BudgetTable'
import type { ElementRowExtras } from './BudgetTable'
import type { FolderBucket } from './budgetMath'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetDto } from '@/api/dto/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

type TableExtras = ElementRowExtras & {
  hideChildren?: boolean
  sectionWrapper?: (bucket: FolderBucket, sectionKey: string, node: ReactNode) => ReactNode
  renderFolderActions?: (bucket: FolderBucket, index: number, total: number) => ReactNode
}

function renderTable(mutate?: (budget: BudgetDto) => void, extras: TableExtras = {}) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  mutate?.(budget)
  const buckets = bucketElements(budget, makeBudgetExchange(budget, [usd, eur]))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BudgetTable budget={budget} buckets={buckets} {...extras} />
    </QueryClientProvider>,
  )
  return budget
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
})

it('renders column headers, folder, default and archived sections with aligned stat cells', async () => {
  renderTable()
  const headers = screen.getByTestId('column-headers')
  expect(headers).toHaveTextContent('Budget')
  expect(headers).toHaveTextContent('Spent')
  expect(headers).toHaveTextContent('Available')
  const essentials = await screen.findByTestId('budget-folder-Essentials')
  expect(within(essentials).getByText('Food')).toBeInTheDocument()
  await waitFor(() => expect(within(essentials).getByTestId('stat-line')).toHaveTextContent('200.00'))
  expect(within(essentials).getByTestId('stat-line')).toHaveTextContent('45.50')
  expect(within(essentials).getByTestId('stat-line')).not.toHaveTextContent('-45.50')
  expect(within(essentials).getByTestId('stat-line')).toHaveTextContent('354.50')
  const noFolder = screen.getByTestId('budget-folder-Default folder')
  expect(within(noFolder).getByText('Living')).toBeInTheDocument()
  // the only archived element is all-zero, so the whole Archived section hides
  expect(screen.queryByTestId('budget-folder-Archived')).not.toBeInTheDocument()
})

it('archived elements with a nonzero number stay listed; all-zero ones hide', async () => {
  renderTable((budget) => {
    budget.structure.elements.push({ ...budget.structure.elements[2], id: 'tag-carry', name: 'aaa-carry', available: '7' })
  })
  const archive = await screen.findByTestId('budget-folder-Archived')
  expect(within(archive).getByText('aaa-carry')).toBeInTheDocument()
  expect(within(archive).queryByText('zzz-archived')).not.toBeInTheDocument()
})

it('an empty Default folder hides outside edit mode', async () => {
  renderTable((budget) => {
    budget.structure.elements[1].folderId = 'bf1'
  })
  await screen.findByTestId('budget-folder-Essentials')
  expect(screen.queryByTestId('budget-folder-Default folder')).not.toBeInTheDocument()
})

it('edit mode keeps the empty Default folder as a drop target', async () => {
  renderTable(
    (budget) => {
      budget.structure.elements[1].folderId = 'bf1'
    },
    { renderFolderActions: () => null } as never,
  )
  await screen.findByTestId('budget-folder-Essentials')
  expect(screen.getByTestId('budget-folder-Default folder')).toBeInTheDocument()
})

it('shows spent as-is and available+budgeted as a sign-colored pill', async () => {
  renderTable()
  const food = await screen.findByTestId('element-cat-food')
  // exact match: the wire value is positive and must NOT be rendered negated
  await waitFor(() => expect(within(food).getByTestId('cell-spent')).toHaveTextContent(/^45\.50$/))
  expect(within(food).getByTestId('cell-available')).toHaveTextContent('354.50')
  expect(within(food).getByTestId('cell-available').className).toContain('text-income')
  expect(within(food).getByTestId('cell-available').className).toContain('rounded-full')
})

it('rounds float noise in cells to the currency precision', async () => {
  renderTable((budget) => {
    const food = budget.structure.elements[0]
    food.spent = '45.4999999934'
    food.budgetSpent = '45.4999999934'
  })
  const food = await screen.findByTestId('element-cat-food')
  await waitFor(() => expect(within(food).getByTestId('cell-spent')).toHaveTextContent('45.50'))
  expect(within(food).getByTestId('cell-spent')).not.toHaveTextContent('45.4999')
})

it('fold toggle expands children and persists in the store', async () => {
  const user = userEvent.setup()
  renderTable()
  const living = await screen.findByTestId('element-env-1')
  expect(screen.queryByTestId('child-cat-rent')).not.toBeInTheDocument()
  await user.click(within(living).getByText('Living'))
  expect(await screen.findByTestId('child-cat-rent')).toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().unfoldedElements['env-1']).toBe(true)
})

it('hideContents renders sections header-only (folder drag in progress)', async () => {
  renderTable(undefined, { hideContents: true } as never)
  const essentials = await screen.findByTestId('budget-folder-Essentials')
  expect(within(essentials).queryByText('Food')).not.toBeInTheDocument()
  expect(within(essentials).getByText('Essentials')).toBeInTheDocument()
})

it('hideChildren renders unfolded elements collapsed (element drag in progress)', async () => {
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: { 'env-1': true }, foldBudgetId: null })
  renderTable(undefined, { hideChildren: true })
  await screen.findByTestId('element-env-1')
  expect(screen.queryByTestId('child-cat-rent')).not.toBeInTheDocument()
})

it('clicking the name of a childless element does nothing', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(undefined, { onSpentClick })
  const food = await screen.findByTestId('element-cat-food')
  await user.click(within(food).getByText('Food'))
  expect(onSpentClick).not.toHaveBeenCalled()
  expect(useBudgetPeriodStore.getState().unfoldedElements['cat-food']).toBeUndefined()
})

it('clicking the spent amount reports the element as a transactions target', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(undefined, { onSpentClick })
  const food = await screen.findByTestId('element-cat-food')
  await user.click(within(food).getByRole('button', { name: 'transactions Food' }))
  expect(onSpentClick).toHaveBeenCalledWith({ id: 'cat-food', type: 1, name: 'Food', icon: 'restaurant', currencyId: null })
})

it('clicking a child spent amount reports the child category with the parent currency', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(undefined, { onSpentClick })
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  await user.click(await screen.findByRole('button', { name: 'transactions Rent' }))
  expect(onSpentClick).toHaveBeenCalledWith({
    id: 'cat-rent', type: 1, name: 'Rent', icon: 'house', currencyId: 'cur-eur',
    parent: { id: 'env-1', type: 0 },
  })
})

it('clicking a tag child spent amount reports the parent tag', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable((budget) => {
    budget.structure.elements.push({
      id: 'tag-travel', type: 2, name: 'Travel', icon: 'flight', currencyId: null, isArchived: 0,
      folderId: null, position: 3, budgeted: '0', available: '-30', spent: '30', budgetSpent: '30',
      ownerUserId: 'u1',
      children: [{ id: 'cat-hotel', type: 1, name: 'Hotel', icon: 'hotel', isArchived: 0, spent: '30', budgetSpent: '30', ownerUserId: 'u1' }],
    })
  }, { onSpentClick })
  const travel = await screen.findByTestId('element-tag-travel')
  await user.click(within(travel).getByText('Travel'))
  await user.click(await screen.findByRole('button', { name: 'transactions Hotel' }))
  // Without the parent the dialog sends categoryId alone, and the backend
  // returns the untagged complement of what this row displays.
  expect(onSpentClick).toHaveBeenCalledWith({
    id: 'cat-hotel', type: 1, name: 'Hotel', icon: 'hotel', currencyId: null,
    parent: { id: 'tag-travel', type: 2 },
  })
})

it('children show spent and the owner badge only in a multi-user budget', async () => {
  const user = userEvent.setup()
  renderTable((budget) => {
    budget.meta.access.push({ user: { id: 'u2', avatar: 'pets:sky', name: 'Partner' }, role: 'user', isAccepted: 1 })
    budget.structure.elements[1].children[0].spent = '12.5'
    budget.structure.elements[1].children[0].ownerUserId = 'u2'
  })
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  const child = await screen.findByTestId('child-cat-rent')
  await waitFor(() => expect(within(child).getByTestId('child-spent')).toHaveTextContent(/^12\.50$/))
  // the owner name is rendered (revealed on row hover via CSS)
  expect(within(child).getByText('Partner')).toBeInTheDocument()
})

it('children hide the owner badge in a single-user budget', async () => {
  const user = userEvent.setup()
  renderTable()
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  const child = await screen.findByTestId('child-cat-rent')
  expect(within(child).queryByText('Ada')).not.toBeInTheDocument()
})

it('tapping the available pill reports the element when onAvailableClick is wired (compact set-limit path)', async () => {
  const user = userEvent.setup()
  const onAvailableClick = vi.fn()
  renderTable(undefined, { onAvailableClick })
  const food = await screen.findByTestId('element-cat-food')
  await user.click(within(food).getByRole('button', { name: 'limit Food' }))
  expect(onAvailableClick).toHaveBeenCalledWith(expect.objectContaining({ id: 'cat-food' }))
})

it('the available pill is not a button without onAvailableClick', async () => {
  renderTable()
  const food = await screen.findByTestId('element-cat-food')
  expect(within(food).queryByRole('button', { name: 'limit Food' })).not.toBeInTheDocument()
})

it('edit mode reserves the actions column on headers, children, archive rows and totals', async () => {
  const user = userEvent.setup()
  // seed a nonzero archived element so the Archived section is visible (all-zero archive hides)
  renderTable(
    (budget) => {
      budget.structure.elements.push({ ...budget.structure.elements[2], id: 'tag-carry', name: 'aaa-carry', available: '7' })
    },
    {
      renderActions: (element) => <button type="button" aria-label={`element actions ${element.name}`} className="size-8" />,
    },
  )
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  const child = await screen.findByTestId('child-cat-rent')
  expect(within(child).getByTestId('actions-spacer')).toBeInTheDocument()
  expect(within(screen.getByTestId('column-headers')).getByTestId('actions-spacer')).toBeInTheDocument()
  expect(within(screen.getByTestId('budget-totals')).getByTestId('actions-spacer')).toBeInTheDocument()
  // archive rows carry no per-element actions menu — they pad the slot instead
  const archive = screen.getByTestId('budget-folder-Archived')
  expect(within(archive).getAllByTestId('actions-spacer').length).toBeGreaterThan(0)
})

it('without renderActions no actions-column spacers render', async () => {
  const user = userEvent.setup()
  renderTable()
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  await screen.findByTestId('child-cat-rent')
  expect(screen.queryAllByTestId('actions-spacer')).toHaveLength(0)
})

it('totals row sums all buckets in the budget currency', async () => {
  renderTable()
  const totals = await screen.findByTestId('budget-totals')
  expect(totals).toHaveTextContent('Total')
  await waitFor(() => expect(totals).toHaveTextContent('300.00'))
  expect(totals).toHaveTextContent('45.50')
  expect(totals).not.toHaveTextContent('-45.50')
  expect(totals).toHaveTextContent('554.50')
})

it('phone totals unfold into labeled budget/spent/available lines', async () => {
  renderTable()
  const totals = await screen.findByTestId('budget-totals-mobile')
  expect(totals).toHaveTextContent('Total')
  await waitFor(() => expect(totals).toHaveTextContent('300.00'))
  expect(totals).toHaveTextContent('Budget')
  expect(totals).toHaveTextContent('Spent')
  expect(totals).toHaveTextContent('Available')
  expect(totals).toHaveTextContent('45.50')
  expect(totals).toHaveTextContent('554.50')
})

function pushUncategorized(budget: BudgetDto) {
  budget.structure.elements.push({
    id: UNCATEGORIZED_ID,
    type: 1,
    // deliberately mismatched with the translation, to prove the row
    // ignores the wire's raw name
    name: 'zzz-wire-name-should-not-render',
    icon: 'question_mark',
    currencyId: null,
    isArchived: 0,
    folderId: null,
    position: 999,
    budgeted: '0',
    available: '-30',
    spent: '30',
    budgetSpent: '30',
    ownerUserId: null,
    children: [],
  })
}

it('the Uncategorized row is read-only, mirroring an archive row', async () => {
  renderTable(pushUncategorized, {
    renderBudgetCell: () => <span data-testid="editor-marker">edit</span>,
    renderActions: (element) => <button type="button" aria-label={`element actions ${element.name}`} />,
    onAvailableClick: vi.fn(),
  })
  const row = await screen.findByTestId(`element-${UNCATEGORIZED_ID}`)
  expect(within(row).queryByTestId('editor-marker')).not.toBeInTheDocument()
  expect(within(row).queryByRole('button', { name: /^element actions /i })).not.toBeInTheDocument()
  expect(within(row).queryByRole('button', { name: /^limit /i })).not.toBeInTheDocument()
})

it('the Uncategorized row spent amount is still clickable', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(pushUncategorized, { onSpentClick })
  const row = await screen.findByTestId(`element-${UNCATEGORIZED_ID}`)
  await user.click(within(row).getByRole('button', { name: /^transactions /i }))
  expect(onSpentClick).toHaveBeenCalledWith({ id: UNCATEGORIZED_ID, type: 1, name: 'Uncategorized', icon: 'question_mark', currencyId: null })
})

it('the Uncategorized row displays the translated name, not the wire literal', async () => {
  renderTable(pushUncategorized)
  const row = await screen.findByTestId(`element-${UNCATEGORIZED_ID}`)
  expect(within(row).getByText('Uncategorized')).toBeInTheDocument()
  expect(within(row).queryByText('zzz-wire-name-should-not-render')).not.toBeInTheDocument()
})

it('a no-folder row alongside Uncategorized still gets its budget cell and actions', async () => {
  renderTable(pushUncategorized, {
    renderBudgetCell: () => <span data-testid="editor-marker">edit</span>,
    renderActions: (element) => <button type="button" aria-label={`element actions ${element.name}`} />,
  })
  const living = await screen.findByTestId('element-env-1')
  expect(within(living).getByTestId('editor-marker')).toBeInTheDocument()
  expect(within(living).getByRole('button', { name: 'element actions Living' })).toBeInTheDocument()
})

it('Uncategorized lives in its own section, not the no-folder one', async () => {
  renderTable(pushUncategorized)
  const section = await screen.findByTestId('budget-folder-Uncategorized')
  expect(within(section).getByTestId(`element-${UNCATEGORIZED_ID}`)).toBeInTheDocument()
  // the no-folder section keeps its own rows and does NOT hold Uncategorized
  const noFolder = screen.getByTestId('budget-folder-Default folder')
  expect(within(noFolder).queryByTestId(`element-${UNCATEGORIZED_ID}`)).not.toBeInTheDocument()
  expect(within(noFolder).getByTestId('element-env-1')).toBeInTheDocument()
})

it('Uncategorized hides its icon on desktop but keeps it on mobile', async () => {
  renderTable(pushUncategorized)
  const row = await screen.findByTestId(`element-${UNCATEGORIZED_ID}`)
  const icon = row.querySelector('.material-icon')
  expect(icon).not.toBeNull()
  // mobile-only: the wrapper drops the icon from the sm breakpoint up
  expect(icon!.parentElement).toHaveClass('sm:hidden')
  // a normal row shows its icon at every width
  const living = screen.getByTestId('element-env-1')
  expect(living.querySelector('.material-icon')!.parentElement).not.toHaveClass('sm:hidden')
})

it('Uncategorized renders as ONE level: a bare row, with no section header above it', async () => {
  renderTable(pushUncategorized)
  const section = await screen.findByTestId('budget-folder-Uncategorized')
  // no header/stat-line wrapper — the row itself is the whole section
  expect(within(section).queryByTestId('stat-line')).not.toBeInTheDocument()
  expect(section.querySelector('header')).toBeNull()
  // and the label appears exactly once, on the row
  expect(within(section).getAllByText('Uncategorized')).toHaveLength(1)
})

it('Uncategorized shows a dash for budget and available, and keeps its spent amount', async () => {
  renderTable(pushUncategorized)
  const row = await screen.findByTestId(`element-${UNCATEGORIZED_ID}`)
  expect(within(row).getByTestId('cell-budgeted')).toHaveTextContent('—')
  expect(within(row).getByTestId('cell-available')).toHaveTextContent('—')
  // the dash replaces the value, so no stray zero/negative amount survives
  expect(within(row).getByTestId('cell-available')).not.toHaveTextContent('30')
  expect(within(row).getByTestId('cell-spent')).toHaveTextContent('30')
})

const kidA = {
  id: 'label-kid-a',
  name: 'kid-A',
  icon: 'label',
  isArchived: 0 as const,
  spent: '50.00',
  ownerUserId: 'u1',
  children: [
    { id: 'cat-groceries', type: 1 as const, name: 'Groceries', icon: 'local_grocery_store', isArchived: 0 as const, spent: '30.00', budgetSpent: '30.00', ownerUserId: 'u1' },
    { id: UNCATEGORIZED_ID, type: 1 as const, name: 'wire-name-ignored', icon: 'question_mark', isArchived: 0 as const, spent: '20.00', budgetSpent: '20.00', ownerUserId: 'u1' },
  ],
}

function withLabels(budget: BudgetDto) {
  budget.structure.labels = [kidA]
}

async function openFolder(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByTestId('budget-labels-heading'))
}

it('renders the labels folder collapsed by default, hiding the tags', async () => {
  renderTable(withLabels)
  const section = await screen.findByTestId('budget-labels-section')
  expect(within(section).getByTestId('budget-labels-heading')).toBeInTheDocument()
  expect(screen.queryByText('kid-A')).not.toBeInTheDocument()
})

it('expanding the folder reveals the tags, tags still collapsed', async () => {
  const user = userEvent.setup()
  renderTable(withLabels)
  await openFolder(user)
  const section = await screen.findByTestId('budget-labels-section')
  expect(within(section).getByText('kid-A')).toBeInTheDocument()
  await waitFor(() => expect(within(section).getByTestId('budget-label-label-kid-a')).toHaveTextContent('50.00'))
  expect(screen.queryByText('Groceries')).not.toBeInTheDocument()
})

it('explains reporting tags behind an info button, without toggling the fold', async () => {
  const user = userEvent.setup()
  renderTable(withLabels)
  const info = await screen.findByRole('button', { name: 'About' })
  // the description is on demand, not a standing caveat above the numbers
  expect(screen.queryByTestId('budget-labels-info-note')).not.toBeInTheDocument()
  await user.click(info)
  expect(await screen.findByTestId('budget-labels-info-note')).toBeInTheDocument()
  // the info button sits outside the collapsible trigger, so the fold stays shut
  expect(screen.queryByText('kid-A')).not.toBeInTheDocument()
})

it("expanding a tag reveals that tag's category breakdown", async () => {
  const user = userEvent.setup()
  renderTable(withLabels)
  await openFolder(user)
  await user.click(screen.getByText('kid-A'))
  expect(await screen.findByTestId('label-child-cat-groceries')).toBeInTheDocument()
  expect(screen.getByText('Groceries')).toBeInTheDocument()
  // the uncategorized bucket renders under its translated name, like elsewhere
  expect(within(screen.getByTestId(`label-child-${UNCATEGORIZED_ID}`)).getByText('Uncategorized')).toBeInTheDocument()
  expect(screen.queryByText('wire-name-ignored')).not.toBeInTheDocument()
})

it('both fold levels persist in the budget period store', async () => {
  const user = userEvent.setup()
  renderTable(withLabels)
  await openFolder(user)
  await user.click(screen.getByText('kid-A'))
  const unfolded = useBudgetPeriodStore.getState().unfoldedElements
  expect(unfolded['__reporting_tags__']).toBe(true)
  expect(unfolded['label-kid-a']).toBe(true)
})

it('a reporting tag with no children renders but is not expandable', async () => {
  const user = userEvent.setup()
  renderTable((budget) => {
    budget.structure.labels = [{ ...kidA, children: [] }]
  })
  await openFolder(user)
  const row = await screen.findByTestId('budget-label-label-kid-a')
  expect(within(row).getByText('kid-A')).toBeInTheDocument()
  // the only button on the row is the spend drill-down, never a fold toggle
  expect(within(row).queryByRole('button', { name: /collapse|expand/i })).not.toBeInTheDocument()
})

it('the reporting tags folder is never part of the edit-structure surface', async () => {
  const user = userEvent.setup()
  const sectionWrapper = vi.fn((_bucket, _key, node) => node)
  renderTable(withLabels, {
    sectionWrapper,
    renderFolderActions: () => <button type="button" aria-label="folder actions" />,
    renderActions: (element: BudgetElementDto) => <button type="button" aria-label={`element actions ${element.name}`} />,
    renderRowWrapper: (element: BudgetElementDto, _bucket: FolderBucket, row: ReactNode) => (
      <div key={element.id} data-testid={`wrapped-${element.id}`}>
        {row}
      </div>
    ),
  })
  await openFolder(user)
  const section = await screen.findByTestId('budget-labels-section')
  // an ephemeral folder: not renameable/movable/deletable, never a drop target
  expect(sectionWrapper.mock.calls.map((c) => c[1])).not.toContain('__reporting_tags__')
  expect(within(section).queryByRole('button', { name: 'folder actions' })).not.toBeInTheDocument()
  expect(within(section).queryByRole('button', { name: /^element actions /i })).not.toBeInTheDocument()
  expect(screen.queryByTestId('wrapped-label-kid-a')).not.toBeInTheDocument()
})

it('omits the labels block entirely when there are no labels', async () => {
  renderTable((budget) => {
    budget.structure.labels = []
  })
  await screen.findByTestId('budget-table')
  expect(screen.queryByTestId('budget-labels-section')).not.toBeInTheDocument()
  // the heading itself carries its own testid, so this fails if the section
  // ever renders headless (the untranslated catalog-key text is not a stable
  // thing to assert against, before or after Task 8 lands real copy)
  expect(screen.queryByTestId('budget-labels-heading')).not.toBeInTheDocument()
})

it('the labels block sits directly after Uncategorized and before Archive, never after Total', async () => {
  renderTable((budget) => {
    pushUncategorized(budget)
    // a nonzero archived element, so Archive is visible and could otherwise sit
    // between Uncategorized and the labels block
    budget.structure.elements.push({ ...budget.structure.elements[2], id: 'tag-carry', name: 'aaa-carry', available: '7' })
    withLabels(budget)
  })
  const uncategorized = await screen.findByTestId('budget-folder-Uncategorized')
  const labels = await screen.findByTestId('budget-labels-section')
  const archive = await screen.findByTestId('budget-folder-Archived')
  const totals = screen.getByTestId('budget-totals')
  const isBefore = (a: Element, b: Element) => !!(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING)
  // Total must read as its own row, not a sum of the overlapping label amounts
  // right above it -- so the labels block cannot be the section immediately
  // preceding Total.
  expect(isBefore(uncategorized, labels)).toBe(true)
  expect(isBefore(labels, archive)).toBe(true)
  expect(isBefore(archive, totals)).toBe(true)
})

it('clicking a label spend reports it as a transactions target with the label discriminant', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(withLabels, { onSpentClick })
  await openFolder(user)
  const section = await screen.findByTestId('budget-labels-section')
  await user.click(within(section).getByRole('button', { name: 'transactions kid-A' }))
  expect(onSpentClick).toHaveBeenCalledWith({ id: 'label-kid-a', type: 'label', name: 'kid-A', icon: 'label', currencyId: null })
})

it('clicking a category child under a tag reports tag and category together', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(withLabels, { onSpentClick })
  await openFolder(user)
  await user.click(screen.getByText('kid-A'))
  await user.click(await screen.findByRole('button', { name: 'transactions Groceries' }))
  // without the parent the dialog would send categoryId alone, which is the
  // whole category rather than its slice under this reporting tag
  expect(onSpentClick).toHaveBeenCalledWith({
    id: 'cat-groceries', type: 1, name: 'Groceries', icon: 'local_grocery_store', currencyId: null,
    parent: { id: 'label-kid-a', type: 'label' },
  })
})

it("clicking a tag's uncategorized child reports the label parent too", async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(withLabels, { onSpentClick })
  await openFolder(user)
  await user.click(screen.getByText('kid-A'))
  await user.click(await screen.findByRole('button', { name: 'transactions Uncategorized' }))
  expect(onSpentClick).toHaveBeenCalledWith(
    expect.objectContaining({ id: UNCATEGORIZED_ID, parent: { id: 'label-kid-a', type: 'label' } }),
  )
})

it('the Uncategorized section is never a drag container and its row has no handle', async () => {
  const sectionWrapper = vi.fn((_bucket, _key, node) => node)
  renderTable(pushUncategorized, {
    renderRowWrapper: (element, _bucket, row) => (
      <div key={element.id} data-testid={`wrapped-${element.id}`}>
        {row}
      </div>
    ),
    sectionWrapper,
  })
  await screen.findByTestId('budget-folder-Uncategorized')
  // the row is rendered plain — renderRowWrapper (the drag handle in edit mode)
  // never wraps it, while a normal row still gets wrapped
  expect(screen.queryByTestId(`wrapped-${UNCATEGORIZED_ID}`)).not.toBeInTheDocument()
  expect(screen.getByTestId('wrapped-env-1')).toBeInTheDocument()
  expect(sectionWrapper.mock.calls.map((c) => c[1])).not.toContain('__uncategorized__')
})

it('explains the uncategorized bucket behind its own info button', async () => {
  const user = userEvent.setup()
  renderTable(pushUncategorized)
  const row = await screen.findByTestId(`element-${UNCATEGORIZED_ID}`)
  expect(screen.queryByTestId('budget-uncategorized-info-note')).not.toBeInTheDocument()
  await user.click(within(row).getByRole('button', { name: 'About' }))
  expect(await screen.findByTestId('budget-uncategorized-info-note')).toBeInTheDocument()
})
