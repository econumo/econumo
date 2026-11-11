import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useIconSuggestion, type IconSuggestionOptions } from './useIconSuggestion'

let asked: URLSearchParams[]

function serve(icon: (name: string) => string = () => 'house') {
  asked = []
  server.use(...coreHandlers())
  // added later, so it answers ahead of the core handlers
  server.use(
    http.get('*/api/v1/onboarding/suggest-icon', ({ request }) => {
      const params = new URL(request.url).searchParams
      asked.push(params)
      return HttpResponse.json({ success: true, message: '', data: { icon: icon(params.get('name') ?? '') } })
    }),
  )
}

function renderSuggestion(initial: IconSuggestionOptions) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  return renderHook((opts: IconSuggestionOptions) => useIconSuggestion(opts), { initialProps: initial, wrapper })
}

// the debounce alone takes 600 ms
const eventually = (check: () => void) => waitFor(check, { timeout: 3000 })
const settle = (ms = 900) => new Promise((resolve) => setTimeout(resolve, ms))

beforeEach(() => {
  window.econumoConfig = { AI_ENABLED: true }
})

afterEach(() => {
  window.econumoConfig = {}
  onlineManager.setOnline(true)
})

it('asks once the typing pauses, for the last name only', async () => {
  serve()
  const { result, rerender } = renderSuggestion({ kind: 'category', name: 'Mo', type: 'expense', enabled: true })
  rerender({ kind: 'category', name: 'Mort', type: 'expense', enabled: true })
  rerender({ kind: 'category', name: ' Mortgage ', type: 'expense', enabled: true })
  expect(result.current).toBeNull()
  await eventually(() => expect(result.current).toBe('house'))
  expect(asked).toHaveLength(1)
  expect(Object.fromEntries(asked[0])).toEqual({ kind: 'category', name: 'Mortgage', type: 'expense' })
})

it('sends an account currency and answers each name from the session cache after the first time', async () => {
  serve((name) => (name === 'Cash' ? 'payments' : 'credit_card'))
  const { result, rerender } = renderSuggestion({ kind: 'account', name: 'Cash', currency: 'EUR', enabled: true })
  await eventually(() => expect(result.current).toBe('payments'))
  expect(asked[0].get('currency')).toBe('EUR')
  rerender({ kind: 'account', name: 'Revolut', currency: 'EUR', enabled: true })
  await eventually(() => expect(result.current).toBe('credit_card'))
  rerender({ kind: 'account', name: 'cash', currency: 'EUR', enabled: true })
  await eventually(() => expect(result.current).toBe('payments'))
  expect(asked).toHaveLength(2)
})

it('asks nothing for a short name, while disabled, offline, or without AI', async () => {
  serve()
  const { result, rerender } = renderSuggestion({ kind: 'category', name: 'M', enabled: true })
  await settle()
  rerender({ kind: 'category', name: 'Mortgage', enabled: false })
  await settle()
  onlineManager.setOnline(false)
  rerender({ kind: 'category', name: 'Mortgage', enabled: true })
  await settle()
  onlineManager.setOnline(true)
  window.econumoConfig = {}
  rerender({ kind: 'category', name: 'Rent', enabled: true })
  await settle()
  expect(asked).toHaveLength(0)
  expect(result.current).toBeNull()
})

it('an empty answer is no suggestion', async () => {
  serve(() => '')
  const { result } = renderSuggestion({ kind: 'category', name: 'Misc', enabled: true })
  await eventually(() => expect(asked).toHaveLength(1))
  await settle(100)
  expect(result.current).toBeNull()
})
