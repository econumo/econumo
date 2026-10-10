import type { ReactNode } from 'react'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'

// dnd-kit stand-in: every DndContext's onDragEnd is captured in render order; the
// expenses table's context renders last
let captured: { onDragEnd: (e: never) => void }[] = []
vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  return {
    ...actual,
    DndContext: ({ onDragEnd, children }: { onDragEnd: (e: never) => void; children: ReactNode }) => {
      captured.push({ onDragEnd })
      return children
    },
  }
})

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

function renderPage() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const moves: Record<string, unknown>[] = []
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(fixtureWirePlan),
    http.post('*/api/v1/budget/move-element', async ({ request }) => {
      moves.push((await request.json()) as Record<string, unknown>)
      // the refetch would show the new place; holding the answer keeps the drop's
      // own rendering in view
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/budget', element: <BudgetPage mode="budget" /> }], { initialEntries: ['/budget'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return moves
}

const expenseDrop = () => captured[captured.length - 1]

beforeEach(() => {
  captured = []
  localStorage.clear()
  window.econumoConfig = {}
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: { 'env-1': true }, foldBudgetId: 'b1', planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('an unfolded envelope lists its categories with grips, inside its drop zone', async () => {
  renderPage()
  const drop = await screen.findByTestId('envelope-drop-env-1')
  expect(within(drop).getByTestId('child-cat-rent')).toBeInTheDocument()
  expect(within(drop).getByRole('button', { name: 'move cat-rent' })).toBeInTheDocument()
})

it('a category dragged out of its envelope is placed where it is dropped', async () => {
  const moves = renderPage()
  await screen.findByTestId('child-cat-rent')
  act(() => expenseDrop().onDragEnd({ active: { id: 'cat-rent' }, over: { id: 'cat-food' } } as never))
  await waitFor(() => expect(moves).toEqual([{ budgetId: 'b1', id: 'cat-rent', folderId: 'bf1', afterId: null }]))
  // hidden from the envelope until the refetched budget shows it in its new place
  expect(screen.queryByTestId('child-cat-rent')).toBeNull()
})

it('a category dropped on an envelope list joins that envelope', async () => {
  const moves = renderPage()
  await screen.findByTestId('element-cat-food')
  act(() => expenseDrop().onDragEnd({ active: { id: 'cat-food' }, over: { id: 'benv:env-1' } } as never))
  await waitFor(() => expect(moves).toEqual([{ budgetId: 'b1', id: 'cat-food', folderId: null, afterId: null, envelopeId: 'env-1' }]))
  expect(screen.queryByTestId('element-cat-food')).toBeNull()
})

it('nothing moves for a drop back on its own envelope, or a non-category on a list', async () => {
  const moves = renderPage()
  await screen.findByTestId('child-cat-rent')
  act(() => expenseDrop().onDragEnd({ active: { id: 'cat-rent' }, over: { id: 'benv:env-1' } } as never))
  act(() => expenseDrop().onDragEnd({ active: { id: 'cat-rent' }, over: { id: 'env-1' } } as never))
  act(() => expenseDrop().onDragEnd({ active: { id: 'env-1' }, over: { id: 'benv:env-1' } } as never))
  await new Promise((r) => setTimeout(r, 50))
  expect(moves).toEqual([])
  expect(screen.getByTestId('child-cat-rent')).toBeInTheDocument()
})

it('a folded envelope takes a category dropped on the middle of its row', async () => {
  useBudgetPeriodStore.setState({ unfoldedElements: {} })
  const moves = renderPage()
  const envelope = await screen.findByTestId('element-env-1')
  expect(within(envelope).getByTestId('envelope-head-drop-env-1')).toBeInTheDocument()
  act(() => expenseDrop().onDragEnd({ active: { id: 'cat-food' }, over: { id: 'benvh:env-1' } } as never))
  await waitFor(() => expect(moves).toEqual([{ budgetId: 'b1', id: 'cat-food', folderId: null, afterId: null, envelopeId: 'env-1' }]))
})

it('an unfolded envelope keeps its list as the drop zone, not its row', async () => {
  renderPage()
  const envelope = await screen.findByTestId('element-env-1')
  expect(within(envelope).queryByTestId('envelope-head-drop-env-1')).toBeNull()
})
