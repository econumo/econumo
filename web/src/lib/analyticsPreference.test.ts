import { beforeEach, expect, it } from 'vitest'
import { analyticsAllowed, rememberAnalyticsPreference } from './analyticsPreference'

beforeEach(() => localStorage.clear())

it('defaults to allowed', () => {
  expect(analyticsAllowed()).toBe(true)
})

it('remembers an opt-out across reloads', () => {
  rememberAnalyticsPreference(false)
  expect(analyticsAllowed()).toBe(false)
  rememberAnalyticsPreference(true)
  expect(analyticsAllowed()).toBe(true)
})
