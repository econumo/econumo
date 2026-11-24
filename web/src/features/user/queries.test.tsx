import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useUpdateName, useUpdateAvatar, useUpdateDefaultBudget, useUpdatePassword, useAccessState, useUpdateAi, useUpdateAiSuggestions } from './queries'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

type Switches = { suggestRules: boolean; suggestFill: boolean }
const withSwitches = (rules: string, fill: string) => ({
  ...fixtureUser,
  options: [...fixtureUser.options, { name: 'ai_suggest_rules', value: rules }, { name: 'ai_suggest_fill', value: fill }],
})
const switchesOf = (queryClient: QueryClient) =>
  queryClient.getQueryData<typeof fixtureUser>(queryKeys.user)?.options.filter((o) => o.name.startsWith('ai_suggest_'))

describe('queued suggestion switch writes', () => {
  // each answer waits for its release; ok[n] says whether the n-th write is taken
  function switchServer(ok: boolean[]) {
    const releases: (() => void)[] = []
    let n = 0
    server.use(
      http.post('*/api/v1/user/update-ai-suggestions', async ({ request }) => {
        const body = (await request.json()) as Switches
        const taken = ok[n++]
        await new Promise<void>((resolve) => releases.push(resolve))
        if (!taken) return HttpResponse.json({ success: false, message: 'Not allowed', code: 0, errors: {} }, { status: 400 })
        return HttpResponse.json({ success: true, message: '', data: { user: withSwitches(body.suggestRules ? '1' : '0', body.suggestFill ? '1' : '0') } })
      }),
      // the refetch after a failure never answers, so only the restore can settle the cache
      http.get('*/api/v1/user/get-user-data', () => new Promise<never>(() => {})),
    )
    return releases
  }

  async function run(ok: boolean[]) {
    const releases = switchServer(ok)
    const { queryClient, wrapper } = makeWrapper()
    queryClient.setQueryData(queryKeys.user, withSwitches('1', '1'))
    const { result } = renderHook(() => [useUpdateAiSuggestions(), useUpdateAiSuggestions()] as const, { wrapper })
    const first = result.current[0].mutateAsync({ suggestRules: false, suggestFill: true }).catch(() => undefined)
    const second = result.current[1].mutateAsync({ suggestRules: false, suggestFill: false }).catch(() => undefined)
    await waitFor(() => expect(releases).toHaveLength(1))
    releases[0]()
    await waitFor(() => expect(releases).toHaveLength(2))
    releases[1]()
    await Promise.all([first, second])
    return queryClient
  }

  it('both refused: the cache is back to what the server had before the first', async () => {
    const queryClient = await run([false, false])
    expect(switchesOf(queryClient)).toEqual([{ name: 'ai_suggest_rules', value: '1' }, { name: 'ai_suggest_fill', value: '1' }])
  })

  it('the first taken, the second refused: the cache shows the first one\'s values', async () => {
    const queryClient = await run([true, false])
    expect(switchesOf(queryClient)).toEqual([{ name: 'ai_suggest_rules', value: '0' }, { name: 'ai_suggest_fill', value: '1' }])
  })
})

it('update-ai takes only the ai option from its answer, keeping a suggestion switch changed meanwhile', async () => {
  let release: (() => void) | null = null
  server.use(http.post('*/api/v1/user/update-ai', async () => {
    await new Promise<void>((resolve) => { release = resolve })
    // the user as this write saw it: the suggestion switch still on
    return HttpResponse.json({ success: true, message: '', data: { user: { ...withSwitches('1', '1'), options: [...withSwitches('1', '1').options, { name: 'ai', value: '0' }] } } })
  }))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.user, withSwitches('1', '1'))
  const { result } = renderHook(() => useUpdateAi(), { wrapper })
  const done = result.current.mutateAsync(false)
  await waitFor(() => expect(release).not.toBeNull())
  queryClient.setQueryData(queryKeys.user, withSwitches('0', '1'))
  release!()
  await done
  const options = queryClient.getQueryData<typeof fixtureUser>(queryKeys.user)!.options
  expect(options).toContainEqual({ name: 'ai', value: '0' })
  expect(options).toContainEqual({ name: 'ai_suggest_rules', value: '0' })
})

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('update-name replaces the user cache with the echoed user', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-name', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, name: 'Renamed' } } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  const { result } = renderHook(() => useUpdateName(), { wrapper })
  result.current.mutate('Renamed')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ name: 'Renamed' })
  expect(queryClient.getQueryData<{ name: string }>(queryKeys.user)!.name).toBe('Renamed')
})

it('update-avatar replaces the user cache and invalidates avatar-embedding lists', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-avatar', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, avatar: 'pets' } } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useUpdateAvatar(), { wrapper })
  result.current.mutate({ icon: 'pets', color: 'teal' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ icon: 'pets', color: 'teal' })
  expect(queryClient.getQueryData<{ avatar: string }>(queryKeys.user)!.avatar).toBe('pets')
  expect(spy).toHaveBeenCalled()
})

it('update-budget posts {value} and invalidates the budget cache', async () => {
  server.use(
    http.post('*/api/v1/user/update-budget', async ({ request }) => {
      expect(await request.json()).toEqual({ value: 'b1' })
      return HttpResponse.json({ success: true, message: '', data: { user: fixtureUser } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useUpdateDefaultBudget(), { wrapper })
  result.current.mutate('b1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(spy).toHaveBeenCalledWith({ queryKey: queryKeys.budget })
})

it('update-password rejects on the generic 400 envelope', async () => {
  server.use(
    http.post('*/api/v1/user/update-password', () =>
      HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useUpdatePassword(), { wrapper })
  result.current.mutate({ oldPassword: 'bad', newPassword: 'newpass' })
  await waitFor(() => expect(result.current.isError).toBe(true))
})

it('isOnboardingCompleted treats an absent option as completed (Vue parity)', async () => {
  const { isOnboardingCompleted } = await import('./queries')
  const base = { id: 'u1', name: 'A', email: 'a@x.test', avatar: '', accessLevel: 'full' as const, accessUntil: '', createdAt: '2024-01-01 00:00:00', hasPassword: true, currency: 'USD', reportPeriod: 'monthly' }
  expect(isOnboardingCompleted(undefined)).toBe(false)
  expect(isOnboardingCompleted({ ...base, options: [] })).toBe(true)
  expect(isOnboardingCompleted({ ...base, options: [{ name: 'onboarding', value: 'completed' }] })).toBe(true)
  expect(isOnboardingCompleted({ ...base, options: [{ name: 'onboarding', value: '' }] })).toBe(false)
})

function utcIn(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 19).replace('T', ' ')
}

describe('useAccessState', () => {
  it('derives trial with daysLeft and billingEnabled from config', async () => {
    window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
    server.use(...coreHandlers({ user: { ...fixtureUser, accessLevel: 'full', accessUntil: utcIn(2) } }))
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useAccessState(), { wrapper })
    await waitFor(() => expect(result.current.state).toBe('trial'))
    expect(result.current.daysLeft).toBe(2)
    expect(result.current.billingEnabled).toBe(true)
  })

  it('derives full_access with null daysLeft and disabled billing by default', async () => {
    server.use(...coreHandlers())
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useAccessState(), { wrapper })
    await waitFor(() => expect(result.current.state).toBe('full_access'))
    expect(result.current.daysLeft).toBeNull()
    expect(result.current.billingEnabled).toBe(false)
  })

  it('derives readonly', async () => {
    server.use(...coreHandlers({ user: { ...fixtureUser, accessLevel: 'readonly', accessUntil: '' } }))
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useAccessState(), { wrapper })
    await waitFor(() => expect(result.current.state).toBe('readonly'))
  })
})
