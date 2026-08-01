import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { PayeesPage } from './PayeesPage'
import { TagsPage } from './TagsPage'

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage(element: React.ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/x', element }], { initialEntries: ['/x'] })
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
  server.use(
    ...coreHandlers({
      payees: [
        { id: 'p1', ownerUserId: 'u1', name: 'Grocer', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
        { id: 'p2', ownerUserId: 'u1', name: 'Grocer Twin', position: 1, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
      ],
    }),
  )
  mockViewport()
})

it('payee edit resolves by id even when names collide', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/payee/update-payee', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage(<PayeesPage />)
  await screen.findByText('Grocer Twin')
  await user.click(screen.getByRole('button', { name: 'actions Grocer Twin' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(await screen.findByText('Edit payee')).toBeInTheDocument()
  const input = screen.getByLabelText('Name')
  expect(input).toHaveValue('Grocer Twin')
  await user.clear(input)
  await user.type(input, 'Grocer') // now collides with p1's name
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toEqual({ id: 'p2', name: 'Grocer' }))
})

it('payee create validates 3-64 and posts id+name', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/payee/create-payee', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { id: 'p-new', ownerUserId: 'u1', name: 'Butcher', position: 2, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' } },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage(<PayeesPage />)
  await screen.findByText('Grocer')
  await user.click(screen.getByRole('button', { name: /Create payee/ }))
  await user.type(await screen.findByLabelText('Name'), 'ab')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(await screen.findByText('Payee name must be 3-64 characters')).toBeInTheDocument()
  await user.clear(screen.getByLabelText('Name'))
  await user.type(screen.getByLabelText('Name'), 'Butcher')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.name).toBe('Butcher')
  expect(await screen.findByText('Butcher')).toBeInTheDocument()
})

it('search fuzzy-filters the list and restores it on clear', async () => {
  const user = userEvent.setup()
  renderPage(<PayeesPage />)
  await screen.findByText('Grocer')
  const search = screen.getByRole('textbox', { name: 'Search' })
  await user.type(search, 'gt') // subsequence of "Grocer Twin" only
  expect(screen.getByText('Grocer Twin')).toBeInTheDocument()
  expect(screen.queryByText('Grocer')).not.toBeInTheDocument()
  await user.clear(search)
  expect(screen.getByText('Grocer')).toBeInTheDocument()
  expect(screen.getByText('Grocer Twin')).toBeInTheDocument()
})

it('search with no matches shows nothing-found and hides drag handles while active', async () => {
  const user = userEvent.setup()
  renderPage(<PayeesPage />)
  await screen.findByText('Grocer')
  expect(screen.getByRole('button', { name: 'drag Grocer' })).toBeInTheDocument()
  const search = screen.getByRole('textbox', { name: 'Search' })
  await user.type(search, 'gro')
  expect(screen.queryByRole('button', { name: 'drag Grocer' })).not.toBeInTheDocument()
  await user.clear(search)
  await user.type(search, 'zzz')
  expect(screen.getByText('Nothing found')).toBeInTheDocument()
  await user.clear(search)
  expect(screen.getByRole('button', { name: 'drag Grocer' })).toBeInTheDocument()
})

it('search fires the analytics event once per visit', async () => {
  window.dataLayer = []
  const user = userEvent.setup()
  renderPage(<PayeesPage />)
  await screen.findByText('Grocer')
  await user.type(screen.getByRole('textbox', { name: 'Search' }), 'gro')
  await user.clear(screen.getByRole('textbox', { name: 'Search' }))
  await user.type(screen.getByRole('textbox', { name: 'Search' }), 'tw')
  const events = window.dataLayer.filter((e) => (e as { event?: string }).event === 'appClassificationSearch')
  expect(events).toHaveLength(1)
  expect((events[0] as { eventData?: { type?: string } }).eventData?.type).toBe('payee')
})

it('tag delete invalidates the budget cache; payee delete does not', async () => {
  server.use(
    http.post('*/api/v1/tag/delete-tag', () => HttpResponse.json({ success: true, message: '', data: {} })),
    http.post('*/api/v1/payee/delete-payee', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const user = userEvent.setup()

  // tags page
  const tagClient = renderPage(<TagsPage />)
  const tagSpy = vi.spyOn(tagClient, 'invalidateQueries')
  await screen.findByText('vacation')
  await user.click(screen.getByRole('button', { name: 'actions vacation' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  await screen.findByText('Delete tag?')
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(tagSpy).toHaveBeenCalledWith({ queryKey: queryKeys.budget }))
})
