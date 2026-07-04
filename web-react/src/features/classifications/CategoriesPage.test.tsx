import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { CategoriesPage } from './CategoriesPage'

const UUID_V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/categories', element: <CategoriesPage /> }], {
    initialEntries: ['/settings/categories'],
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockViewport()
})

it('lists own categories flat with the archived sublabel inline', async () => {
  renderPage()
  expect(await screen.findByText('Food')).toBeInTheDocument()
  expect(screen.getByText('Salary')).toBeInTheDocument()
  expect(screen.getByText('Old')).toBeInTheDocument()
  expect(screen.getByText('Archived (inactive)')).toBeInTheDocument()
})

it('creates a category with type and icon (no accountId)', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { id: 'cat-new', ownerUserId: 'u1', name: 'Books', position: 9, type: 'income', icon: 'home', isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' } },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Food')
  await user.click(screen.getByRole('button', { name: /Create category/ }))
  await screen.findByText('New category')
  await user.click(screen.getByRole('radio', { name: 'Income' }))
  await user.type(screen.getByLabelText('Name'), 'Books')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.id).toMatch(UUID_V7)
  expect(body!.type).toBe('income')
  expect(body!.icon).toBe('home')
  expect(body!.accountId).toBeUndefined()
  expect(await screen.findByText('Books')).toBeInTheDocument()
})

it('edit posts id/name/icon without type and the type toggle is frozen', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/category/update-category', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Food')
  await user.click(screen.getByRole('button', { name: 'actions Food' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  await screen.findByText('Edit')
  expect(screen.getByRole('radio', { name: 'Expense' })).toBeDisabled()
  const nameInput = screen.getByLabelText('Name')
  await user.clear(nameInput)
  await user.type(nameInput, 'Groceries')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toEqual({ id: 'cat-food', name: 'Groceries', icon: 'restaurant' }))
})

it('archive toggle hits archive/unarchive endpoints', async () => {
  const calls: string[] = []
  server.use(
    http.post('*/api/v1/category/archive-category', () => {
      calls.push('archive')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/category/unarchive-category', () => {
      calls.push('unarchive')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Food')
  await user.click(screen.getByRole('switch', { name: 'archive Food' }))
  await waitFor(() => expect(calls).toEqual(['archive']))
  await user.click(screen.getByRole('switch', { name: 'archive Old' }))
  await waitFor(() => expect(calls).toEqual(['archive', 'unarchive']))
})

it('delete posts mode=delete and scrubs the category from cached transactions', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/category/delete-category', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()
  queryClient.setQueryData(queryKeys.transactions, [
    { id: 't1', categoryId: 'cat-food' },
    { id: 't2', categoryId: 'cat-salary' },
  ])
  await screen.findByText('Food')
  await user.click(screen.getByRole('button', { name: 'actions Food' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Delete category?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(body).toEqual({ id: 'cat-food', mode: 'delete' }))
  await waitFor(() => {
    const txs = queryClient.getQueryData<{ id: string; categoryId: string | null }[]>(queryKeys.transactions)
    expect(txs!.find((t) => t.id === 't1')!.categoryId).toBeNull()
  })
})

it('A-Z sort posts the changed positions', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/category/order-category-list', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('Food')
  await user.click(screen.getByRole('button', { name: /Reorder list/ }))
  await user.click(await screen.findByRole('button', { name: 'Alphabetically (A-Z)' }))
  // alphabetical: Food(0) Old(1) Salary(2); current: Food(0) Salary(1) Old(2)
  await waitFor(() =>
    expect(body).toEqual({
      changes: [
        { id: 'cat-archived', position: 1 },
        { id: 'cat-salary', position: 2 },
      ],
    }),
  )
})
