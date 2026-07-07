import { getToken, hasToken, setToken, removeToken, isTokenExpired, getItem, setItem } from './storage'

function fakeJwt(payload: object): string {
  const b64 = (o: object) => btoa(JSON.stringify(o)).replace(/=+$/, '')
  return `${b64({ alg: 'RS256', typ: 'JWT' })}.${b64(payload)}.sig`
}

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

  it('detects an expired token by the exp claim', () => {
    const past = Math.floor(Date.now() / 1000) - 60
    const future = Math.floor(Date.now() / 1000) + 3600
    expect(isTokenExpired(fakeJwt({ exp: past }))).toBe(true)
    expect(isTokenExpired(fakeJwt({ exp: future }))).toBe(false)
  })

  it('treats a token without exp as not expired, and garbage as expired', () => {
    expect(isTokenExpired(fakeJwt({ id: 'u1' }))).toBe(false)
    expect(isTokenExpired('not-a-jwt')).toBe(true)
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
