import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { CategoryDto } from '@/api/dto/category'
import { useCreateCategory } from '@/features/classifications/queries'
import { METRICS, trackEvent } from '@/lib/metrics'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const created: CategoryDto = {
  id: 'cat-new', ownerUserId: 'u1', name: 'Mortgage', position: 9, type: 'expense', icon: 'home', isArchived: 0,
  createdAt: '2026-11-11 10:00:00', updatedAt: '2026-11-11 10:00:00',
} as CategoryDto

let suggested: () => Promise<string>
let asked: number
let updates: Record<string, unknown>[]

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  queryClient.setQueryData<CategoryDto[]>(queryKeys.categories, [])
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  const { result } = renderHook(() => useCreateCategory(), { wrapper })
  return { queryClient, result }
}

const applied = () => vi.mocked(trackEvent).mock.calls.filter(([name]) => name === METRICS.ICON_SUGGESTION_APPLIED)
const iconOf = (queryClient: QueryClient) => queryClient.getQueryData<CategoryDto[]>(queryKeys.categories)?.find((c) => c.id === created.id)?.icon

beforeEach(() => {
  window.econumoConfig = { AI_ENABLED: true }
  vi.mocked(trackEvent).mockClear()
  suggested = async () => 'house'
  asked = 0
  updates = []
  server.use(
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      const body = (await request.json()) as { icon?: string }
      return HttpResponse.json({ success: true, message: '', data: { item: { ...created, icon: body.icon ?? 'home' } } })
    }),
    http.get('*/api/v1/onboarding/suggest-icon', async () => {
      asked++
      return HttpResponse.json({ success: true, message: '', data: { icon: await suggested() } })
    }),
    http.post('*/api/v1/category/update-category', async ({ request }) => {
      updates.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
})

afterEach(() => {
  window.econumoConfig = {}
})

it('an inline create answers at once, then takes the suggested icon while it still has its first one', async () => {
  const { queryClient, result } = setup()
  const item = await result.current.mutateAsync({ name: 'Mortgage', type: 'expense' })
  expect(item.icon).toBe('home')
  await waitFor(() => expect(updates).toEqual([{ id: created.id, name: 'Mortgage', icon: 'house' }]))
  await waitFor(() => expect(iconOf(queryClient)).toBe('house'))
  expect(applied()).toEqual([[METRICS.ICON_SUGGESTION_APPLIED, { kind: 'category', source: 'inline' }]])
})

it('an icon changed while the suggestion was on its way stays', async () => {
  let answer: (icon: string) => void = () => undefined
  suggested = () => new Promise((resolve) => (answer = resolve))
  const { queryClient, result } = setup()
  await result.current.mutateAsync({ name: 'Mortgage', type: 'expense' })
  await waitFor(() => expect(asked).toBe(1))
  queryClient.setQueryData<CategoryDto[]>(queryKeys.categories, (prev) => (prev ?? []).map((c) => ({ ...c, icon: 'wallet' })))
  answer('house')
  await new Promise((resolve) => setTimeout(resolve, 100))
  expect(updates).toEqual([])
  expect(iconOf(queryClient)).toBe('wallet')
  expect(applied()).toEqual([])
})

it('no suggestion, no AI, or an icon given by a form changes nothing', async () => {
  suggested = async () => ''
  const { result } = setup()
  await result.current.mutateAsync({ name: 'Mortgage', type: 'expense' })
  await waitFor(() => expect(asked).toBe(1))
  await result.current.mutateAsync({ name: 'Vet', type: 'expense', icon: 'pets' })
  window.econumoConfig = {}
  await result.current.mutateAsync({ name: 'Rent', type: 'expense' })
  await new Promise((resolve) => setTimeout(resolve, 100))
  expect(asked).toBe(1)
  expect(updates).toEqual([])
  expect(applied()).toEqual([])
})
