import type { ReactNode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureUsd, fixtureWireBudget } from '@/test/fixtures'
import type { BudgetCommentDto, BudgetDto, BudgetSavingsElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { SavingsBlock } from './SavingsBlock'
import { useBudgetPeriodStore } from './budgetStore'
import { commentCellKey } from './queries'

// dnd-kit stand-in: every DndContext's onDragEnd is captured so a drop can be fired
// directly; droppables the block registers itself and its sortable item lists are recorded.
let capturedDragEnds: ((event: { active: { id: string }; over: { id: string } | null }) => void)[] = []
let droppableIds: string[] = []
vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  return {
    ...actual,
    DndContext: ({ onDragEnd, children }: { onDragEnd: (event: never) => void; children: ReactNode }) => {
      capturedDragEnds.push(onDragEnd as never)
      return children
    },
    useDroppable: (args: { id: string }) => {
      droppableIds.push(String(args.id))
      return actual.useDroppable(args as never)
    },
  }
})

let sortableItems: string[][] = []
vi.mock('@dnd-kit/sortable', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/sortable')>()
  return {
    ...actual,
    SortableContext: (props: { items: string[]; children: ReactNode }) => {
      sortableItems.push(props.items.map(String))
      return actual.SortableContext(props as never)
    },
  }
})

const s1: BudgetSavingsElementDto = {
  id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
  budgeted: '100', spent: '120', available: '-20',
}
const s2: BudgetSavingsElementDto = {
  id: 'acc-s2', type: 5, name: 'Holiday fund', icon: 'beach_access', currencyId: 'cur-eur', ownerUserId: 'u1', isArchived: 0, position: 1,
  budgeted: '90', spent: '45', available: '45',
}
const s3: BudgetSavingsElementDto = {
  id: 'acc-s3', type: 5, name: 'Closed deposit', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 1, position: 2,
  budgeted: '0', spent: '10', available: '-10',
}

const currencies = [fixtureUsd, fixtureEur] as CurrencyDto[]

function budgetWith(savings: BudgetSavingsElementDto[] | undefined): BudgetDto {
  const budget = JSON.parse(JSON.stringify(fixtureWireBudget)) as BudgetDto
  if (savings !== undefined) {
    budget.structure.savings = savings
  }
  return budget
}

const comment: BudgetCommentDto = {
  id: 'cm-s2',
  elementId: 'acc-s2',
  period: '2026-07-01',
  comment: 'Flights',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-07-17 09:00:00',
  updatedAt: '2026-07-17 09:00:00',
}

function renderBlock(overrides: Partial<Parameters<typeof SavingsBlock>[0]> = {}) {
  const props = {
    budget: budgetWith([s2, s1, s3]),
    currencies,
    selectedDate: '2026-07-01',
    canEdit: true,
    editMode: false,
    commentsByCell: new Map<string, BudgetCommentDto[]>(),
    onEditPlanned: vi.fn(),
    onOpenComments: vi.fn(),
    onMove: vi.fn(),
    ...overrides,
  }
  const view = render(<SavingsBlock {...props} />)
  return { ...view, props }
}

beforeEach(() => {
  capturedDragEnds = []
  droppableIds = []
  sortableItems = []
  localStorage.clear()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', planFolds: {} })
})

it('renders nothing when the budget has no savings rows', () => {
  const { container } = renderBlock({ budget: budgetWith(undefined) })
  expect(container).toBeEmptyDOMElement()
  const empty = renderBlock({ budget: budgetWith([]) })
  expect(empty.container).toBeEmptyDOMElement()
})

it('renders one row per savings account in position order, deleted accounts last', () => {
  renderBlock()
  const block = screen.getByTestId('budget-savings-block')
  expect(within(block).getByText('Savings')).toBeInTheDocument()
  const ids = within(block)
    .getAllByTestId(/^savings-row-/)
    .map((el) => el.getAttribute('data-testid'))
  expect(ids).toEqual(['savings-row-acc-s1', 'savings-row-acc-s2', 'savings-row-acc-s3'])
  expect(within(block).getByText('Planned')).toBeInTheDocument()
  expect(within(block).getByText('Saved')).toBeInTheDocument()
  expect(within(block).getByText('Balance')).toBeInTheDocument()
})

it('below sm the column labels wrap instead of truncating; from sm up they truncate', () => {
  renderBlock()
  for (const label of ['Planned', 'Saved', 'Balance']) {
    const el = within(screen.getByTestId('budget-savings-block')).getByText(label)
    expect(el.className).not.toMatch(/(?:^|\s)truncate(?:\s|$)/)
    expect(el.className).toMatch(/(?:^|\s)sm:truncate(?:\s|$)/)
  }
})

it('shows Planned / Saved / Balance in the row currency, the balance as the server projects it', () => {
  renderBlock({ budget: budgetWith([{ ...s1, closingBalance: '1234.5' }, { ...s2, closingBalance: '0' }]) })
  const r1 = screen.getByTestId('savings-row-acc-s1')
  expect(within(r1).getByTestId('savings-planned')).toHaveTextContent('100.00')
  expect(within(r1).getByTestId('savings-saved')).toHaveTextContent('120.00')
  expect(within(r1).getByTestId('savings-balance')).toHaveTextContent('1,234.50')
  expect(within(r1).getByText('$')).toBeInTheDocument()

  const r2 = screen.getByTestId('savings-row-acc-s2')
  expect(within(r2).getByTestId('savings-planned')).toHaveTextContent('90.00')
  expect(within(r2).getByTestId('savings-saved')).toHaveTextContent('45.00')
  expect(within(r2).getByTestId('savings-balance')).toHaveTextContent('0.00')
  expect(within(r2).getByText('€')).toBeInTheDocument()
  // no remaining pill and no second line under the name any more
  expect(screen.queryByTestId('savings-remaining')).toBeNull()
  expect(within(r1).queryByText(/Balance on the 1st/)).toBeNull()
})

it('shows a dash for the balance when the server sends none', () => {
  renderBlock({ budget: budgetWith([s1]) })
  expect(screen.getByTestId('savings-balance')).toHaveTextContent('—')
})

it('folds and persists the fold under planFolds["monthly-savings"]', async () => {
  const user = userEvent.setup()
  renderBlock()
  expect(screen.getByText('Planned')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /Savings/ }))
  expect(screen.queryByTestId('savings-row-acc-s1')).not.toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().planFolds['monthly-savings']).toBe(true)
  // folded, the column labels would read as the Total row's header: they go too
  expect(screen.queryByText('Planned')).toBeNull()
  expect(screen.queryByText('Balance')).toBeNull()
  await user.click(screen.getByRole('button', { name: /Savings/ }))
  expect(screen.getByTestId('savings-row-acc-s1')).toBeInTheDocument()
  expect(screen.getByText('Planned')).toBeInTheDocument()
})

it('a phone shows two figures like the budget table (Planned, Saved); Balance joins from sm up', () => {
  renderBlock({ budget: budgetWith([{ ...s1, closingBalance: '5' }]) })
  const row = screen.getByTestId('savings-row-acc-s1')
  const phoneOnlyHidden = /(?:^|\s)hidden(?:\s|$)/
  expect(within(row).getByTestId('savings-balance').className).toMatch(phoneOnlyHidden)
  expect(within(row).getByTestId('savings-balance').className).toMatch(/(?:^|\s)sm:block(?:\s|$)/)
  expect(screen.getByText('Balance').className).toMatch(phoneOnlyHidden)
  for (const id of ['savings-planned', 'savings-saved']) {
    expect(within(row).getByTestId(id).className).not.toMatch(phoneOnlyHidden)
    // the table's phone column width, so the figures line up under Spent / Available
    expect(within(row).getByTestId(id).className).toMatch(/(?:^|\s)w-20(?:\s|$)/)
  }
})

it('clicking Planned opens the planned editor when the cell is editable', async () => {
  const user = userEvent.setup()
  const { props } = renderBlock()
  await user.click(within(screen.getByTestId('savings-row-acc-s2')).getByRole('button', { name: 'planned Holiday fund' }))
  expect(props.onEditPlanned).toHaveBeenCalledWith(s2)
  expect(props.onOpenComments).not.toHaveBeenCalled()
})

it('with an inline editor supplied, editable Planned cells render it; read-only cells keep the comments entry point', () => {
  const renderPlannedEditor = vi.fn((row: BudgetSavingsElementDto) => <span data-testid="inline-editor">{row.id}</span>)
  const view = renderBlock({ renderPlannedEditor })
  expect(within(screen.getByTestId('savings-row-acc-s1')).getByTestId('inline-editor')).toHaveTextContent('acc-s1')
  expect(within(screen.getByTestId('savings-row-acc-s2')).getByTestId('inline-editor')).toHaveTextContent('acc-s2')
  expect(screen.queryByRole('button', { name: /^planned / })).not.toBeInTheDocument()
  // a deleted account's row never gets the editor
  expect(within(screen.getByTestId('savings-row-acc-s3')).queryByTestId('inline-editor')).not.toBeInTheDocument()
  expect(within(screen.getByTestId('savings-row-acc-s3')).getByRole('button', { name: 'comments Closed deposit' })).toBeInTheDocument()
  view.unmount()

  renderBlock({ renderPlannedEditor, canEdit: false })
  expect(screen.queryByTestId('inline-editor')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'comments Rainy day' })).toBeInTheDocument()
})

it('a non-editable Planned cell never opens the editor: it falls back to the comments entry point', async () => {
  const user = userEvent.setup()
  const { props } = renderBlock({ canEdit: false })
  await user.click(within(screen.getByTestId('savings-row-acc-s1')).getByRole('button', { name: 'comments Rainy day' }))
  expect(props.onEditPlanned).not.toHaveBeenCalled()
  expect(props.onOpenComments).toHaveBeenCalledWith(s1, expect.any(HTMLElement))
  expect(screen.queryByRole('button', { name: /^planned / })).not.toBeInTheDocument()
})

it('a deleted account row is never editable, even when limits are', async () => {
  const user = userEvent.setup()
  const { props } = renderBlock()
  const row = screen.getByTestId('savings-row-acc-s3')
  expect(within(row).queryByRole('button', { name: 'planned Closed deposit' })).not.toBeInTheDocument()
  await user.click(within(row).getByRole('button', { name: 'comments Closed deposit' }))
  expect(props.onEditPlanned).not.toHaveBeenCalled()
  expect(props.onOpenComments).toHaveBeenCalledWith(s3, expect.any(HTMLElement))
})

it('a row with comments shows the marker; clicking it opens the thread', async () => {
  const user = userEvent.setup()
  const { props } = renderBlock({ commentsByCell: new Map([[commentCellKey('acc-s2', '2026-07-01'), [comment]]]) })
  expect(within(screen.getByTestId('savings-row-acc-s1')).queryByTestId('comment-marker')).not.toBeInTheDocument()
  await user.click(within(screen.getByTestId('savings-row-acc-s2')).getByTestId('comment-marker'))
  expect(props.onOpenComments).toHaveBeenCalledWith(s2, expect.any(HTMLElement))
  expect(props.onEditPlanned).not.toHaveBeenCalled()
})

it('drag handles appear only in edit mode, never on a deleted row', () => {
  const view = renderBlock()
  expect(screen.queryByRole('button', { name: /^move / })).not.toBeInTheDocument()
  view.unmount()
  renderBlock({ editMode: true })
  expect(screen.getByRole('button', { name: 'move acc-s1' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'move acc-s2' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'move acc-s3' })).not.toBeInTheDocument()
})

it('edit mode: dropping S2 above S1 moves it first; the block has its own DndContext with no folder drop zones', () => {
  const { props } = renderBlock({ editMode: true })
  expect(capturedDragEnds.length).toBeGreaterThan(0)
  // the only sortable items are the live rows: no folder, no deleted row
  expect(sortableItems[sortableItems.length - 1]).toEqual(['acc-s1', 'acc-s2'])
  expect(droppableIds.some((id) => id.startsWith('bfolder:'))).toBe(false)
  const onDragEnd = capturedDragEnds[capturedDragEnds.length - 1]

  // anything but another live savings row is not a target
  onDragEnd({ active: { id: 'acc-s2' }, over: { id: 'bfolder:null' } })
  onDragEnd({ active: { id: 'acc-s2' }, over: { id: 'acc-s3' } })
  onDragEnd({ active: { id: 'acc-s2' }, over: null })
  expect(props.onMove).not.toHaveBeenCalled()

  onDragEnd({ active: { id: 'acc-s2' }, over: { id: 'acc-s1' } })
  expect(props.onMove).toHaveBeenCalledWith('acc-s2', null)
})

describe('BudgetPage wiring', () => {
  it('a savings drop in edit mode posts move-element with folderId null', async () => {
    const { createMemoryRouter, RouterProvider } = await import('react-router')
    const { http, HttpResponse } = await import('msw')
    const { fixtureUser } = await import('@/test/fixtures')
    const { BudgetPage } = await import('./BudgetPage')
    let body: unknown
    server.use(
      ...coreHandlers({
        user: { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)) },
      }),
      http.get('*/api/v1/budget/get-budget', () =>
        HttpResponse.json({ success: true, message: '', data: { item: budgetWith([s1, s2]) } }),
      ),
      http.post('*/api/v1/budget/move-element', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ success: true, message: '', data: {} })
      }),
    )
    window.matchMedia = vi.fn().mockImplementation((q: string) => ({
      matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    }))
    const user = userEvent.setup()
    const router = createMemoryRouter([{ path: '/budget', element: <BudgetPage mode="budget" /> }], { initialEntries: ['/budget'] })
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    )
    await screen.findByTestId('budget-savings-block')
    await user.click(screen.getByRole('button', { name: 'Configure' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
    await screen.findByRole('button', { name: 'move acc-s2' })
    // the block renders after the table, so its DndContext is the last one captured
    capturedDragEnds[capturedDragEnds.length - 1]({ active: { id: 'acc-s2' }, over: { id: 'acc-s1' } })
    await waitFor(() => expect(body).toEqual({ budgetId: 'b1', id: 'acc-s2', folderId: null, afterId: null }))
  })
})
