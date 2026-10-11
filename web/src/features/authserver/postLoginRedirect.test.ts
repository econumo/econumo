import { beforeEach, expect, test, vi } from 'vitest'
import { clearPostLoginRedirect, rememberPostLoginRedirect, takePostLoginRedirect } from './postLoginRedirect'

beforeEach(() => {
  sessionStorage.clear()
  vi.useRealTimers()
})

test('returns the consent URL once', () => {
  rememberPostLoginRedirect('/oauth/authorize?client_id=a&state=b')
  expect(takePostLoginRedirect()).toBe('/oauth/authorize?client_id=a&state=b')
  expect(takePostLoginRedirect()).toBe('/')
})

test('ignores anything but the consent route', () => {
  for (const p of ['//evil.test/oauth/authorize', 'https://evil.test', '/settings', '/oauth/authorizex']) {
    rememberPostLoginRedirect(p)
    expect(takePostLoginRedirect()).toBe('/')
  }
})

test('expires after 10 minutes', () => {
  vi.useFakeTimers()
  rememberPostLoginRedirect('/oauth/authorize?x=1')
  vi.advanceTimersByTime(10 * 60 * 1000 + 1)
  expect(takePostLoginRedirect()).toBe('/')
})

test('clearing forgets the remembered URL', () => {
  rememberPostLoginRedirect('/oauth/authorize?x=1')
  clearPostLoginRedirect()
  expect(takePostLoginRedirect()).toBe('/')
})

test('survives corrupt storage', () => {
  sessionStorage.setItem('econumo.postLoginRedirect', '{not json')
  expect(takePostLoginRedirect()).toBe('/')
  sessionStorage.setItem('econumo.postLoginRedirect', JSON.stringify({ path: 'https://evil.test', at: Date.now() }))
  expect(takePostLoginRedirect()).toBe('/')
})
