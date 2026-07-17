import { AxiosError, type AxiosResponse } from 'axios'
import { createAppQueryClient } from './queryPersist'

function defaultRetry(): (failureCount: number, error: unknown) => boolean {
  const retry = createAppQueryClient().getDefaultOptions().queries?.retry
  if (typeof retry !== 'function') {
    throw new Error('expected the default query retry to be a function')
  }
  return retry as (failureCount: number, error: unknown) => boolean
}

it('retries without limit while the backend is unreachable (network error, no response)', () => {
  const retry = defaultRetry()
  const networkError = new AxiosError('Network Error', 'ERR_NETWORK')
  expect(retry(1, networkError)).toBe(true)
  expect(retry(50, networkError)).toBe(true)
})

it('keeps the three-attempt default for HTTP errors', () => {
  const retry = defaultRetry()
  const httpError = new AxiosError('Server Error', 'ERR_BAD_RESPONSE', undefined, undefined, {
    status: 500,
  } as AxiosResponse)
  expect(retry(2, httpError)).toBe(true)
  expect(retry(3, httpError)).toBe(false)
})

it('keeps the three-attempt default for non-axios errors', () => {
  const retry = defaultRetry()
  const plainError = new Error('boom')
  expect(retry(2, plainError)).toBe(true)
  expect(retry(3, plainError)).toBe(false)
})
