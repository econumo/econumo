import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { TagsPage } from './TagsPage'

// These two catalogue keys are new in this change and not yet translated
// (that's Task 8's job) — react-i18next falls back to rendering the raw key.
const LABELS_SECTION_HEADER = 'classifications.labels.pages.settings.header'
const LABEL_DELETE_TITLE = 'classifications.labels.modals.delete.title'

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/tags', element: <TagsPage /> }], { initialEntries: ['/settings/tags'] })
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
  mockViewport()
})

it('lists tags and labels under separate section captions, each with its stored icon', async () => {
  server.use(
    ...coreHandlers({
      tags: [{ id: 'tag1', ownerUserId: 'u1', name: 'vacation', icon: 'flight', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }],
      labels: [{ id: 'label1', ownerUserId: 'u1', name: 'health', icon: 'sell', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }],
    }),
  )
  renderPage()
  expect(await screen.findByText('vacation')).toBeInTheDocument()
  expect(screen.getByText('health')).toBeInTheDocument()
  // 'Tags' appears twice: the page title (h1) and the section caption above the tag row
  expect(screen.getAllByText('Tags')).toHaveLength(2)
  expect(screen.getByText(LABELS_SECTION_HEADER)).toBeInTheDocument()
  // the row icon is the row's OWN stored value, not a kind-derived constant
  expect(screen.getByText('flight')).toBeInTheDocument()
  expect(screen.getByText('sell')).toBeInTheDocument()
})

it('deletes a tag via delete-tag and a label via delete-label', async () => {
  const calls: string[] = []
  server.use(
    ...coreHandlers({
      tags: [{ id: 'tag1', ownerUserId: 'u1', name: 'vacation', icon: 'flight', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }],
      labels: [{ id: 'label1', ownerUserId: 'u1', name: 'health', icon: 'sell', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }],
    }),
    http.post('*/api/v1/tag/delete-tag', () => {
      calls.push('tag')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/label/delete-label', () => {
      calls.push('label')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('vacation')

  await user.click(screen.getByRole('button', { name: 'actions health' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText(LABEL_DELETE_TITLE)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(calls).toEqual(['label']))

  await user.click(screen.getByRole('button', { name: 'actions vacation' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Delete tag?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(calls).toEqual(['label', 'tag']))
})

it('A-Z reorder posts each kind to its own endpoint with positions local to that kind', async () => {
  let tagBody: unknown
  let labelBody: unknown
  server.use(
    ...coreHandlers({
      tags: [
        { id: 'tag-z', ownerUserId: 'u1', name: 'zzz', icon: 'tag', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
        { id: 'tag-a', ownerUserId: 'u1', name: 'aaa', icon: 'tag', position: 1, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
      ],
      labels: [
        { id: 'label-z', ownerUserId: 'u1', name: 'zzz-label', icon: 'label', position: 0, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
        { id: 'label-a', ownerUserId: 'u1', name: 'aaa-label', icon: 'label', position: 1, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' },
      ],
    }),
    http.post('*/api/v1/tag/order-tag-list', async ({ request }) => {
      tagBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
    http.post('*/api/v1/label/order-label-list', async ({ request }) => {
      labelBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('zzz')
  await user.click(screen.getByRole('button', { name: /Reorder list/ }))
  await user.click(await screen.findByRole('button', { name: 'Alphabetically (A-Z)' }))
  await waitFor(() => expect(tagBody).toBeDefined())
  await waitFor(() => expect(labelBody).toBeDefined())
  // both kinds sort aaa* before zzz*; positions are 0-based WITHIN each kind,
  // not offset by the other kind's row count in the merged list
  expect(tagBody).toEqual({ changes: [{ id: 'tag-a', position: 0 }, { id: 'tag-z', position: 1 }] })
  expect(labelBody).toEqual({ changes: [{ id: 'label-a', position: 0 }, { id: 'label-z', position: 1 }] })
})
