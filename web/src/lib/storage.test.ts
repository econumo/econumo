import { getSessionUserId, getToken, hasToken, rememberSessionUser, setSignInListener, setToken, removeToken, getItem, setItem } from './storage'

beforeEach(() => localStorage.clear())

describe('token storage', () => {
  it('round-trips the token through localStorage', () => {
    expect(hasToken()).toBe(false)
    setToken('abc')
    expect(getToken()).toBe('abc')
    expect(hasToken()).toBe(true)
    removeToken()
    expect(getToken()).toBeNull()
  })
})

describe('session user', () => {
  it('is stored with the token and removed with it', () => {
    setToken('abc', 'u1')
    expect(getSessionUserId()).toBe('u1')
    removeToken()
    expect(getSessionUserId()).toBeNull()
  })

  it('a token stored without a user forgets the previous session\'s user', () => {
    setToken('abc', 'u1')
    setToken('def')
    expect(getSessionUserId()).toBeNull()
  })

  it('is filled in for a session that has none, never replaced, never without a token', () => {
    rememberSessionUser('u1')
    expect(getSessionUserId()).toBeNull()
    setToken('abc')
    rememberSessionUser('u1')
    expect(getSessionUserId()).toBe('u1')
    rememberSessionUser('u2')
    expect(getSessionUserId()).toBe('u1')
  })
})

describe('JSON item storage', () => {
  it('serializes values and parses them back', () => {
    setItem('k', { a: 1 })
    expect(getItem('k')).toEqual({ a: 1 })
  })

  it('returns null for missing keys and unparseable values', () => {
    expect(getItem('missing')).toBeNull()
    localStorage.setItem('bad', '{oops')
    expect(getItem('bad')).toBeNull()
  })
})

describe('app lock at sign-in', () => {
  const lockConfig = (userId: string) => JSON.stringify({ v: 1, userId })

  beforeEach(() => {
    sessionStorage.clear()
    setSignInListener(null)
  })

  it('deletes another user\'s lock and keeps the signed-in user\'s', () => {
    localStorage.setItem('econumo.appLock', lockConfig('u1'))
    localStorage.setItem('econumo.appLock.attempts', '{"failedAttempts":3,"lockedUntil":null}')
    setToken('abc', 'u1')
    expect(localStorage.getItem('econumo.appLock')).toBe(lockConfig('u1'))
    expect(localStorage.getItem('econumo.appLock.attempts')).toBeNull()
    setToken('def', 'u2')
    expect(localStorage.getItem('econumo.appLock')).toBeNull()
  })

  it('keeps the lock when the new session\'s user is unknown', () => {
    localStorage.setItem('econumo.appLock', lockConfig('u1'))
    setToken('abc')
    expect(localStorage.getItem('econumo.appLock')).toBe(lockConfig('u1'))
  })

  it('marks the tab as freshly signed in and tells the lock', () => {
    const listener = vi.fn()
    setSignInListener(listener)
    setToken('abc', 'u1')
    expect(sessionStorage.getItem('econumo.appLock.fresh')).not.toBeNull()
    expect(listener).toHaveBeenCalledOnce()
  })
})
