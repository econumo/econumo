import { isUnavailableOffline } from './offlineQuery'

const base = { data: undefined, isPlaceholderData: false, fetchStatus: 'idle' as const, isError: false }

it('a query paused with no data of its own is unavailable', () => {
  expect(isUnavailableOffline({ ...base, fetchStatus: 'paused' }, false)).toBe(true)
})

it('placeholder data from another key does not count as its own', () => {
  expect(isUnavailableOffline({ ...base, data: { x: 1 }, isPlaceholderData: true, fetchStatus: 'paused' }, false)).toBe(true)
})

it('a paused query that holds its own cached data is available', () => {
  expect(isUnavailableOffline({ ...base, data: { x: 1 }, fetchStatus: 'paused' }, false)).toBe(false)
})

it('an error while offline with nothing cached is unavailable, not a server error', () => {
  expect(isUnavailableOffline({ ...base, isError: true }, false)).toBe(true)
})

it('an error while online stays an error', () => {
  expect(isUnavailableOffline({ ...base, isError: true }, true)).toBe(false)
})

it('a query still fetching online is not unavailable', () => {
  expect(isUnavailableOffline({ ...base, fetchStatus: 'fetching' }, true)).toBe(false)
})

it('null data (an answered "nothing") is data of its own', () => {
  expect(isUnavailableOffline({ ...base, data: null, fetchStatus: 'paused' }, false)).toBe(false)
})
