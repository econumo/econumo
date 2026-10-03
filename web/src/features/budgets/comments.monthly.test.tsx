import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent, { PointerEventsCheckLevel } from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

const guestWireBudget = {
  ...fixtureWireBudget,
  meta: {
    ...fixtureWireBudget.meta,
    ownerUserId: 'u9',
    access: [
      { user: { id: 'u9', avatar: 'face:sky', name: 'Owner' }, role: 'owner', isAccepted: 1 },
      { user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'guest', isAccepted: 1 },
    ],
  },
}

const comment = {
  id: 'cm1',
  elementId: 'cat-food',
  period: '2026-07-01',
  comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-07-17 09:00:00',
  updatedAt: '2026-07-17 09:00:00',
}

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function mockTabletViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage(initialPath: '/budget' | '/plan' = '/budget') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
    ],
    { initialEntries: [initialPath] },
  )
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function registerMonthlyHandlers(wireBudget: unknown = fixtureWireBudget) {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: wireBudget } })),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: false } }),
    ),
  )
}

function renderGuestPage(initialPath: '/budget' | '/plan' = '/budget') {
  registerMonthlyHandlers(guestWireBudget)
  return renderPage(initialPath)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planHideEmpty: false })
})

it('opens the thread from the marker as a popover beside the cell, and keeps the amount editor comment-free', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(within(row).getByLabelText(/^limit /))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
  await user.keyboard('{Escape}')

  await user.click(within(row).getByTestId('comment-marker'))
  const popover = await screen.findByTestId('comments-popover')
  expect(within(popover).getByText('Trip to Lisbon')).toBeInTheDocument()
})

it('returns focus to the marker when Escape closes a thread opened from it', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const marker = within(await screen.findByTestId('element-cat-food')).getByTestId('comment-marker')
  act(() => marker.focus())
  await user.keyboard('{Enter}')
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByTestId('comments-popover')).toBeNull())
  await waitFor(() => expect(marker).toHaveFocus())
})

it('opens the thread with Shift+F2 from the focused amount of a budgeted cell', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-env-1')
  expect(within(row).queryByTestId('comment-marker')).toBeNull()
  act(() => within(row).getByLabelText(/^limit /).focus())
  await user.keyboard('{Shift>}{F2}{/Shift}')
  expect(await screen.findByTestId('comments-popover')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Post' })).toBeInTheDocument()
  expect(screen.queryByLabelText('Budget')).toBeNull()
})

it('opens the hover preview from the mouse only, not from keyboard focus on the amount', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const cell = within(await screen.findByTestId('element-cat-food')).getByTestId('cell-budgeted')
  act(() => within(cell).getByLabelText(/^limit /).focus())
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByTestId('comment-preview')).toBeNull()
  await user.hover(cell)
  expect(await screen.findByTestId('comment-preview', {}, { timeout: 1500 })).toHaveTextContent('Trip to Lisbon')
})

it('shows no hover preview over an open amount editor when the pointer drifts back over the cell', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const cell = within(await screen.findByTestId('element-cat-food')).getByTestId('cell-budgeted')
  await user.click(within(cell).getByLabelText(/^limit /))
  const input = await screen.findByLabelText('Budget')
  await user.hover(input)
  await user.hover(cell)
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByTestId('comment-preview')).toBeNull()
  expect(screen.getByLabelText('Budget')).toBeInTheDocument()
})

it('switches the thread when another marker is clicked', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: { items: [comment, { ...comment, id: 'cm2', elementId: 'env-1', comment: 'Envelope note' }], truncated: false },
      }),
    ),
  )
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  await user.click(within(screen.getByTestId('element-env-1')).getByTestId('comment-marker'))
  expect(await screen.findByText('Envelope note')).toBeInTheDocument()
  expect(screen.queryByText('Trip to Lisbon')).toBeNull()
})

it('leaves a right-click on a budgeted cell to the browser: no app menu', async () => {
  registerMonthlyHandlers()
  mockViewport()
  renderPage('/budget')

  const cell = within(await screen.findByTestId('element-cat-food')).getByTestId('cell-budgeted')
  // fireEvent returns false when a handler called preventDefault
  expect(fireEvent.contextMenu(cell)).toBe(true)
  await new Promise((r) => setTimeout(r, 100))
  expect(screen.queryByRole('menu')).toBeNull()
})

it('offers a hover-only "Add comment" corner on a budgeted cell without comments, opening an empty thread beside it', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  // a commented cell keeps its purple marker and gets no add corner
  const commented = within(await screen.findByTestId('element-cat-food')).getByTestId('cell-budgeted')
  expect(within(commented).queryByTestId('comment-marker-add')).toBeNull()

  const cell = within(screen.getByTestId('element-env-1')).getByTestId('cell-budgeted')
  expect(cell).toHaveClass('group/cell')
  expect(within(cell).queryByTestId('comment-marker')).toBeNull()
  const add = within(cell).getByTestId('comment-marker-add')
  expect(add).toHaveAccessibleName('Add comment')
  expect(add).toHaveClass('invisible', 'group-hover/cell:visible')
  await user.click(add)
  const popover = await screen.findByTestId('comments-popover')
  expect(within(popover).getByText('No comments yet.')).toBeInTheDocument()
  expect(within(popover).getByRole('button', { name: 'Post' })).toBeInTheDocument()
  expect(screen.queryByLabelText('Budget')).toBeNull()
})

it('offers no add-comment corner in edit-structure mode', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-env-1')
  expect(within(row).getByTestId('comment-marker-add')).toBeInTheDocument()
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await waitFor(() => expect(within(screen.getByTestId('element-env-1')).queryByTestId('comment-marker-add')).toBeNull())
})

it('offers no add-comment corner on an archived budget (its threads are read-only)', async () => {
  registerMonthlyHandlers({ ...fixtureWireBudget, meta: { ...fixtureWireBudget.meta, isArchived: 1 } })
  mockViewport()
  renderPage('/budget')

  const row = await screen.findByTestId('element-env-1')
  expect(within(row).getByLabelText(/^comments /)).toBeInTheDocument()
  expect(within(row).queryByTestId('comment-marker-add')).toBeNull()
  // the existing thread stays reachable
  expect(within(screen.getByTestId('element-cat-food')).getByTestId('comment-marker')).toBeInTheDocument()
})

it('offers no add-comment corner on the uncategorized row', async () => {
  registerMonthlyHandlers({
    ...fixtureWireBudget,
    structure: {
      ...fixtureWireBudget.structure,
      elements: [
        ...fixtureWireBudget.structure.elements,
        {
          id: 'uncategorized', type: 1, name: 'Uncategorized', icon: 'question_mark', currencyId: null, isArchived: 0,
          folderId: null, position: 99, budgeted: '0', available: '-12', spent: '12', budgetSpent: '12',
          ownerUserId: null, children: [],
        },
      ],
    },
  })
  mockViewport()
  renderPage('/budget')

  const row = await screen.findByTestId('element-uncategorized')
  expect(within(row).getByTestId('cell-budgeted')).toBeInTheDocument()
  expect(within(row).queryByTestId('comment-marker-add')).toBeNull()
  expect(within(screen.getByTestId('element-env-1')).getByTestId('comment-marker-add')).toBeInTheDocument()
})

it('lets a guest add a comment from the corner', async () => {
  mockViewport()
  const user = userEvent.setup()
  renderGuestPage('/budget')

  await user.click(within(await screen.findByTestId('element-env-1')).getByTestId('comment-marker-add'))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})

it('opens the thread as a popover from a marker tap on a tablet', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByTestId('comment-marker'))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a tablet tap on the budgeted amount opens the item sheet, whose Comments opens the thread', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).queryByLabelText(/^comments /)).toBeNull()
  await user.click(within(row).getByRole('button', { name: 'details Food' }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).getByTestId('sheet-comment')).toHaveTextContent('Trip to Lisbon')
  await user.click(within(sheet).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByText('Trip to Lisbon', { selector: 'p' })).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a tablet sheet’s Set budget opens the amount dialog with no comments in it', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByRole('button', { name: 'details Food' }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set budget' }))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
})

it('a guest’s tablet sheet has no Set budget but reaches the thread', async () => {
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderGuestPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByRole('button', { name: 'details Food' }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).queryByRole('button', { name: 'Set budget' })).toBeNull()
  await user.click(within(sheet).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})

it('a tablet tap on an archived element’s amount opens its sheet without Set budget', async () => {
  const archivedElementBudget = {
    ...fixtureWireBudget,
    structure: {
      ...fixtureWireBudget.structure,
      elements: fixtureWireBudget.structure.elements.map((el) => (el.id === 'tag-old' ? { ...el, budgeted: '50', available: '50' } : el)),
    },
  }
  registerMonthlyHandlers(archivedElementBudget)
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/budget')
  await user.click(within(await screen.findByTestId('element-tag-old')).getByRole('button', { name: 'details zzz-archived' }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).queryByRole('button', { name: 'Set budget' })).toBeNull()
  expect(within(sheet).getByRole('button', { name: 'Add comment' })).toBeInTheDocument()
})

it('the tablet Available pill is plain text', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  renderPage('/budget')
  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).getByTestId('cell-available').closest('button')).toBeNull()
})

it('lets a guest open a read-only thread on a cell they cannot edit', async () => {
  mockViewport()
  const user = userEvent.setup()
  renderGuestPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  await user.click(within(row).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Post' })).toBeInTheDocument()
})

// Review round 1 gap: a guest has no LimitEditor to click, but must still be
// able to START a thread on a cell nobody has commented on yet — the marker
// only exists once a thread already has an entry.
it('lets a guest start a thread on a cell with no existing comments, on desktop', async () => {
  mockViewport()
  const user = userEvent.setup()
  renderGuestPage('/budget')

  const row = await screen.findByTestId('element-env-1')
  expect(within(row).queryByTestId('comment-marker')).toBeNull()
  await user.click(within(row).getByLabelText(/^comments /))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})

it('shows the truncated notice when get-comment-list reports its cap was hit', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: true } }),
    ),
  )
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  await user.click(within(row).getByTestId('comment-marker'))
  expect(await screen.findByText('Showing the latest 2000 comments.')).toBeInTheDocument()
})

it('shows no marker on a cell without comments', async () => {
  registerMonthlyHandlers()
  mockViewport()
  renderPage('/budget')
  const row = await screen.findByTestId('element-env-1')
  expect(within(row).queryByTestId('comment-marker')).toBeNull()
})

// Review round 1 gap: the Archive section stripped every extra down to
// `onSpentClick`, so an individually-archived element's existing thread had
// no marker at all on the Budget page — the note became unreachable from
// that view even though the element (and its comments) still exist.
it('keeps the marker reachable on an individually-archived element', async () => {
  const archivedElementBudget = {
    ...fixtureWireBudget,
    structure: {
      ...fixtureWireBudget.structure,
      elements: fixtureWireBudget.structure.elements.map((el) =>
        el.id === 'tag-old' ? { ...el, budgeted: '50', available: '50' } : el,
      ),
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: archivedElementBudget } })),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [{ ...comment, elementId: 'tag-old' }], truncated: false } }),
    ),
  )
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-tag-old')
  expect(within(row).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')
  await user.click(within(row).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})

// An editable budget gives its live rows the limit editor (which hosts the
// thread), but the Archive section strips that editor: without a popover of its
// own, an archived row with no comments yet had no desktop entry point at all.
it('lets a desktop user start a thread on an archived element with no comments, in an editable budget', async () => {
  const archivedElementBudget = {
    ...fixtureWireBudget,
    structure: {
      ...fixtureWireBudget.structure,
      elements: fixtureWireBudget.structure.elements.map((el) =>
        el.id === 'tag-old' ? { ...el, budgeted: '50', available: '50' } : el,
      ),
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: archivedElementBudget } })),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } }),
    ),
  )
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-tag-old')
  expect(within(row).queryByTestId('comment-marker')).toBeNull()
  await user.click(within(row).getByLabelText(/^comments /))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})
