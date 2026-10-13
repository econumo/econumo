import { getSessionUserId, getToken, hasToken, rememberSessionUser, setToken, removeToken, getItem, setItem } from './storage'

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
