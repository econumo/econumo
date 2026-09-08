import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { isFreshAccount, oauthClient, openAuthorizationUrl, useExchangeHandoff, useStartOAuth } from './oauthQueries'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('oauthClient reports web outside the app and app inside it', () => {
  expect(oauthClient()).toBe('web')
  window.Capacitor = { isNativePlatform: () => true }
  expect(oauthClient()).toBe('app')
})

it('openAuthorizationUrl assigns location on the web and opens the Browser plugin in the app', () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  openAuthorizationUrl('https://idp/a')
  expect(assign).toHaveBeenCalledWith('https://idp/a')
  const open = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { open } } }
  openAuthorizationUrl('https://idp/b')
  expect(open).toHaveBeenCalledWith({ url: 'https://idp/b' })
})

it('useStartOAuth posts to start-login for login and start-link for link, then navigates', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  const hits: string[] = []
  server.use(
    http.post('*/api/v1/oauth/start-login', () => { hits.push('login'); return HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/1' } }) }),
    http.post('*/api/v1/oauth/start-link', () => { hits.push('link'); return HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/2' } }) }),
  )
  const { result } = renderHook(() => useStartOAuth(), { wrapper })
  await result.current.mutateAsync({ provider: 'google', intent: 'login' })
  await result.current.mutateAsync({ provider: 'google', intent: 'link' })
  await waitFor(() => expect(hits).toEqual(['login', 'link']))
  expect(assign).toHaveBeenNthCalledWith(1, 'https://idp/1')
  expect(assign).toHaveBeenNthCalledWith(2, 'https://idp/2')
})

it('useExchangeHandoff stores the token and clears the persisted cache', async () => {
  localStorage.setItem('econumo.query-cache', '{"stale":true}')
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ token: 'eco_ses_new', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '' } })))
  const { result } = renderHook(() => useExchangeHandoff(), { wrapper })
  await result.current.mutateAsync('code')
  expect(localStorage.getItem('token')).toBe('eco_ses_new')
  expect(localStorage.getItem('econumo.query-cache')).toBeNull()
})

describe('isFreshAccount', () => {
  const now = new Date('2026-09-07T12:00:00Z')

  it('is fresh when createdAt is within the last two minutes', () => {
    expect(isFreshAccount('2026-09-07 11:59:00', now)).toBe(true)
  })

  it('is not fresh when createdAt is older than two minutes', () => {
    expect(isFreshAccount('2026-09-07 11:00:00', now)).toBe(false)
  })

  it('is not fresh when createdAt is malformed', () => {
    expect(isFreshAccount('not-a-date', now)).toBe(false)
  })
})
