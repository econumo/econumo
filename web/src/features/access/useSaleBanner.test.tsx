import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '@/app/i18n'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { getSaleBanner } from '@/lib/billingSdk'
import { useSaleBanner } from './useSaleBanner'

vi.mock('@/lib/billingSdk', () => ({ getSaleBanner: vi.fn() }))

const HOUR = 3_600_000

const blackFriday = {
  id: 'black-friday-2026',
  text: 'Black Friday: 40% off',
  cta: 'Get 40% off',
  startsAt: '2026-11-27T00:00:00Z',
  endsAt: '2026-11-30T00:00:00Z',
}

const cyberMonday = {
  id: 'cyber-monday-2026',
  text: 'Cyber Monday: 30% off',
  cta: 'Get 30% off',
  startsAt: '2026-11-30T00:00:00Z',
  endsAt: '2026-12-01T00:00:00Z',
}

function renderSaleBanner() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, ...renderHook(() => useSaleBanner(), { wrapper }) }
}

beforeEach(() => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  vi.mocked(getSaleBanner).mockReset()
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
})

it('shows an active banner', async () => {
  vi.setSystemTime(new Date('2026-11-28T12:00:00Z'))
  server.use(...coreHandlers())
  vi.mocked(getSaleBanner).mockResolvedValue(blackFriday)
  const { result } = renderSaleBanner()
  await waitFor(() => expect(result.current).toEqual(blackFriday))
})

it('is hidden before startsAt and shows once it arrives', async () => {
  vi.setSystemTime(new Date('2026-11-26T23:00:00Z'))
  server.use(...coreHandlers())
  vi.mocked(getSaleBanner).mockResolvedValue(blackFriday)
  const { result } = renderSaleBanner()
  await waitFor(() => expect(getSaleBanner).toHaveBeenCalled())
  await act(() => vi.advanceTimersByTimeAsync(0))
  expect(result.current).toBeNull()
  await act(() => vi.advanceTimersByTimeAsync(HOUR))
  expect(result.current).toEqual(blackFriday)
})

it('hides at endsAt without refetching', async () => {
  vi.setSystemTime(new Date('2026-11-28T12:00:00Z'))
  server.use(...coreHandlers())
  vi.mocked(getSaleBanner)
    .mockResolvedValueOnce(blackFriday)
    .mockReturnValue(new Promise(() => {}))
  const { result } = renderSaleBanner()
  await waitFor(() => expect(result.current).toEqual(blackFriday))
  // 36h out: the first timer is clamped to 24h, then re-armed for the rest
  await act(() => vi.advanceTimersByTimeAsync(24 * HOUR))
  expect(result.current).toEqual(blackFriday)
  expect(getSaleBanner).toHaveBeenCalledTimes(1)
  await act(() => vi.advanceTimersByTimeAsync(12 * HOUR))
  expect(result.current).toBeNull()
  expect(getSaleBanner).toHaveBeenCalledTimes(2)
})

it('refetches at endsAt and shows the next sale', async () => {
  vi.setSystemTime(new Date('2026-11-29T23:00:00Z'))
  server.use(...coreHandlers())
  vi.mocked(getSaleBanner).mockResolvedValueOnce(blackFriday).mockResolvedValueOnce(cyberMonday)
  const { result } = renderSaleBanner()
  await waitFor(() => expect(result.current).toEqual(blackFriday))
  await act(() => vi.advanceTimersByTimeAsync(HOUR))
  expect(result.current).toEqual(cyberMonday)
  expect(getSaleBanner).toHaveBeenCalledTimes(2)
})

it('passes the user and connection register dates as registeredAt, and the UI locale', async () => {
  vi.setSystemTime(new Date('2026-11-28T12:00:00Z'))
  server.use(
    ...coreHandlers({
      user: {
        ...fixtureUser,
        accessUntil: '2026-12-01 00:00:00',
        createdAt: '2025-03-04 05:06:07',
      },
      connections: [
        {
          user: { id: 'u2', avatar: 'pets:sky', name: 'Megan' },
          accessLevel: 'readonly',
          accessUntil: '',
          createdAt: '2025-08-09 10:11:12',
          sharedAccounts: [],
        },
      ],
    }),
  )
  vi.mocked(getSaleBanner).mockResolvedValue(null)
  renderSaleBanner()
  await waitFor(() => expect(getSaleBanner).toHaveBeenCalled())
  expect(getSaleBanner).toHaveBeenCalledWith({
    locale: i18n.language,
    user: {
      accessLevel: 'full',
      accessUntil: '2026-12-01 00:00:00',
      registeredAt: '2025-03-04 05:06:07',
    },
    connections: [
      {
        accessLevel: 'readonly',
        accessUntil: '',
        registeredAt: '2025-08-09 10:11:12',
      },
    ],
  })
  await act(() => vi.advanceTimersByTimeAsync(0))
  expect(getSaleBanner).toHaveBeenCalledTimes(1)
})

it('asks again only when access data changes, not on a same-content refetch', async () => {
  vi.setSystemTime(new Date('2026-11-28T12:00:00Z'))
  server.use(...coreHandlers())
  vi.mocked(getSaleBanner).mockResolvedValue(null)
  const { queryClient } = renderSaleBanner()
  await waitFor(() => expect(getSaleBanner).toHaveBeenCalledTimes(1))
  await act(() => queryClient.refetchQueries())
  await act(() => vi.advanceTimersByTimeAsync(0))
  expect(getSaleBanner).toHaveBeenCalledTimes(1)
  act(() => queryClient.setQueryData(queryKeys.user, { ...fixtureUser, accessLevel: 'readonly' }))
  await waitFor(() => expect(getSaleBanner).toHaveBeenCalledTimes(2))
  expect(getSaleBanner).toHaveBeenLastCalledWith(
    expect.objectContaining({ user: expect.objectContaining({ accessLevel: 'readonly' }) }),
  )
})

it('never asks for a banner when billing is disabled', async () => {
  window.econumoConfig = {}
  vi.setSystemTime(new Date('2026-11-28T12:00:00Z'))
  server.use(...coreHandlers())
  vi.mocked(getSaleBanner).mockResolvedValue(blackFriday)
  const { result } = renderSaleBanner()
  await act(() => vi.advanceTimersByTimeAsync(50))
  expect(result.current).toBeNull()
  expect(getSaleBanner).not.toHaveBeenCalled()
})
