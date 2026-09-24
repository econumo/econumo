import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { trackEvent } from '@/lib/metrics'
import { useBudgetComments, useCreateComment, commentCellKey } from './queries'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

const comment = {
  id: 'cm1',
  elementId: 'e1',
  period: '2026-05-01',
  comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-05-17 09:00:00',
  updatedAt: '2026-05-17 09:00:00',
}

beforeEach(() => {
  localStorage.clear()
  server.use(...coreHandlers())
  vi.mocked(toast.error).mockClear()
})

it('groups comments by element and period', async () => {
  server.use(
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: false } }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useBudgetComments('b1', '2026-05-01', 1), { wrapper })
  await waitFor(() => expect(result.current.byCell.size).toBe(1))
  expect(result.current.byCell.get(commentCellKey('e1', '2026-05-01'))).toHaveLength(1)
  expect(result.current.truncated).toBe(false)
})

it('sends the window as query parameters', async () => {
  let url = ''
  server.use(
    http.get('*/api/v1/budget/get-comment-list', ({ request }) => {
      url = request.url
      return HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } })
    }),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useBudgetComments('b1', '2026-05-01', 4), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(url).toContain('budgetId=b1')
  expect(url).toContain('from=2026-05-01')
  expect(url).toContain('months=4')
})

it('mints the comment id, fires the metric and rolls back on error', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } }),
    ),
    http.post('*/api/v1/budget/create-comment', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: comment } })
    }),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useCreateComment('b1'), { wrapper })
  result.current.mutate({ elementId: 'e1', period: '2026-05-01', comment: 'Trip to Lisbon' })
  await waitFor(() => expect(body).toBeDefined())
  expect(typeof body!.id).toBe('string')
  expect((body!.id as string).length).toBeGreaterThan(30)
  await waitFor(() => expect(vi.mocked(trackEvent)).toHaveBeenCalledWith('appBudgetCreateComment'))
})

it('rolls back the optimistic row and toasts when create fails', async () => {
  server.use(
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } }),
    ),
    // a small delay keeps the optimistic row observable before the mocked
    // rejection rolls it back — an instant mock response can otherwise resolve
    // (and roll back) before waitFor gets a chance to see the interim state
    http.post('*/api/v1/budget/create-comment', async () => {
      await new Promise((resolve) => setTimeout(resolve, 100))
      return HttpResponse.json({ success: false, message: 'Bad request', code: 400, errors: {} }, { status: 400 })
    }),
  )
  const { wrapper } = makeWrapper()
  const listHook = renderHook(() => useBudgetComments('b1', '2026-05-01', 1), { wrapper })
  await waitFor(() => expect(listHook.result.current.isSuccess).toBe(true))

  const mutationHook = renderHook(() => useCreateComment('b1'), { wrapper })
  mutationHook.result.current.mutate({ elementId: 'e1', period: '2026-05-01', comment: 'Trip to Lisbon' })

  await waitFor(() => expect(listHook.result.current.byCell.size).toBe(1))
  await waitFor(() => expect(mutationHook.result.current.isError).toBe(true))
  await waitFor(() => expect(listHook.result.current.byCell.size).toBe(0))
  expect(toast.error).toHaveBeenCalledWith(expect.any(String), { id: 'budget-comment-error' })
})
