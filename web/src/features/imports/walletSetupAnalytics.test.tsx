import { StrictMode, type ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ImportSourceDto } from '@/api/dto/imports'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import { STEPS, useWalletSetup } from './walletSetup'

vi.mock('@/lib/metrics', async (original) => ({ ...await original<typeof import('@/lib/metrics')>(), trackEvent: vi.fn() }))

const source: ImportSourceDto = {
  id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00',
  lastSyncedAt: '', lastRunStatus: '', lastRunAt: '', lastRunError: '', lastRunErrorAccountId: '', credentialCiphertext: '', cards: [],
}
const paidSource: ImportSourceDto = {
  ...source, cards: [{ externalAccountId: 'private-card-id', externalName: 'Private card name', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 1, tapCount: 1, lastSeenAt: '2026-08-20 10:42:03' }],
}
const events = (metric: string) => vi.mocked(trackEvent).mock.calls.filter(([name]) => name === metric).map(([, data]) => data!)

function renderSetup(src = source) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  function Wrapper({ children }: { children: ReactNode }) {
    return <StrictMode><QueryClientProvider client={qc}>{children}</QueryClientProvider></StrictMode>
  }
  return renderHook(({ item }) => useWalletSetup(item), { initialProps: { item: src }, wrapper: Wrapper })
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(trackEvent).mockClear()
  window.econumoConfig = { IMPORT_APPLE_WALLET: true }
  server.use(...coreHandlers({ importSources: [source], importQueue: { queued: [], skipped: [], failed: [] } }))
})

it('tracks every reached and completed step once, including the automatic first payment', async () => {
  const { result, rerender } = renderSetup()
  expect(events(METRICS.IMPORT_SETUP_STEP_VIEW)).toEqual([{ provider: 'apple-wallet', step: 'install', step_number: 1, completed_count: 0 }])
  rerender({ item: { ...source } })
  expect(events(METRICS.IMPORT_SETUP_STEP_VIEW)).toHaveLength(1)
  for (const id of ['install', 'configure', 'automate'] as const) {
    act(() => result.current.tick(id))
  }
  act(() => result.current.tick('payment'))
  act(() => result.current.tick('immediate'))
  expect(events(METRICS.IMPORT_SETUP_STEP)).toHaveLength(3)
  rerender({ item: paidSource })
  act(() => result.current.tick('immediate'))
  expect(events(METRICS.IMPORT_SETUP_STEP).map((data) => [data.step, data.step_number, data.method, data.done])).toEqual([
    ['install', 1, 'manual', true], ['configure', 2, 'manual', true], ['automate', 3, 'manual', true],
    ['payment', 4, 'automatic', true], ['immediate', 5, 'manual', true],
  ])
  expect(events(METRICS.IMPORT_SETUP_STEP_VIEW).map((data) => data.step)).toEqual(STEPS)
  expect(events(METRICS.IMPORT_SETUP_COMPLETE)).toEqual([{ provider: 'apple-wallet', completed_count: 5 }])
  rerender({ item: { ...paidSource } })
  act(() => result.current.tick('immediate'))
  expect(events(METRICS.IMPORT_SETUP_STEP)).toHaveLength(5)
  expect(events(METRICS.IMPORT_SETUP_COMPLETE)).toHaveLength(1)
  await waitFor(() => expect(result.current.current).toBeNull())
})

it('tracks automatic shortcut check-in completion', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  server.use(
    http.get('*/api/v1/import/get-queued-event-list', () => HttpResponse.json({ success: true, data: { queued: [], skipped: [], failed: [{ eventId: 'e1', sourceId: 's1', receivedAt: '', error: 'account is required', payload: '{}' }] } })),
    http.post('*/api/v1/import/discard-event', () => HttpResponse.json({ success: true, data: { queued: [], skipped: [], failed: [] } })),
  )
  const { result } = renderSetup()
  await waitFor(() => expect(result.current.current).toBe('automate'))
  expect(events(METRICS.IMPORT_SETUP_STEP)).toEqual([{ provider: 'apple-wallet', step: 'configure', step_number: 2, done: true, method: 'automatic', completed_count: 2 }])
})

it('counts all newly inferred steps when the first payment arrives, without sending payment data', () => {
  const { rerender } = renderSetup()
  rerender({ item: paidSource })
  expect(events(METRICS.IMPORT_SETUP_STEP).map((data) => data.step)).toEqual(STEPS.slice(0, 4))
  expect(events(METRICS.IMPORT_SETUP_COMPLETE)).toHaveLength(0)
  const payload = JSON.stringify(vi.mocked(trackEvent).mock.calls)
  for (const secret of ['private-card-id', 'Private card name', 'USD', 's1']) expect(payload).not.toContain(secret)
})

it('does not replay old completions on mount, and records a successful restart and fresh progress', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['immediate']))
  server.use(...coreHandlers({ importSources: [paidSource], importQueue: { queued: [], skipped: [], failed: [] } }))
  const { result } = renderSetup(paidSource)
  expect(events(METRICS.IMPORT_SETUP_STEP)).toHaveLength(0)
  expect(events(METRICS.IMPORT_SETUP_COMPLETE)).toHaveLength(0)
  await act(() => result.current.reset())
  expect(events(METRICS.IMPORT_SETUP_RESTART)).toEqual([{ provider: 'apple-wallet' }])
  expect(events(METRICS.IMPORT_SETUP_STEP_VIEW).at(-1)).toMatchObject({ step: 'install', completed_count: 0 })
  act(() => result.current.tick('install'))
  expect(events(METRICS.IMPORT_SETUP_STEP)).toEqual([{ provider: 'apple-wallet', step: 'install', step_number: 1, done: true, method: 'manual', completed_count: 1 }])
})

it('reports the current step for resumed setup and does not count failed reset requests', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure', 'automate']))
  const { result } = renderSetup()
  expect(events(METRICS.IMPORT_SETUP_STEP_VIEW)).toEqual([{ provider: 'apple-wallet', step: 'payment', step_number: 4, completed_count: 3 }])
  expect(events(METRICS.IMPORT_SETUP_STEP)).toHaveLength(0)
  server.use(http.get('*/api/v1/import/get-source-list', () => HttpResponse.json({ message: 'unavailable' }, { status: 500 })))
  await act(async () => { await expect(result.current.reset()).rejects.toThrow() })
  expect(events(METRICS.IMPORT_SETUP_RESTART)).toHaveLength(0)
  expect(result.current.current).toBe('payment')
})
