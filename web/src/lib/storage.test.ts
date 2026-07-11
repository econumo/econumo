import { getToken, hasToken, setToken, removeToken, getItem, setItem } from './storage'

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
