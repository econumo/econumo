import { expect, it } from 'vitest'
import { isIOS, isIPhone, isMac } from './platform'

function stubNavigator(overrides: { userAgent?: string; platform?: string; maxTouchPoints?: number }) {
  Object.defineProperty(window.navigator, 'userAgent', { value: overrides.userAgent ?? 'Mozilla/5.0 (X11; Linux x86_64)', configurable: true })
  Object.defineProperty(window.navigator, 'platform', { value: overrides.platform ?? 'Linux x86_64', configurable: true })
  Object.defineProperty(window.navigator, 'maxTouchPoints', { value: overrides.maxTouchPoints ?? 0, configurable: true })
}

it('isIOS matches an iPhone user agent', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15' })
  expect(isIOS()).toBe(true)
})

it('isIOS treats a touch-capable "MacIntel" as an iPad', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 5 })
  expect(isIOS()).toBe(true)
})

it('isIOS is false on a desktop Mac and on Linux', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 0 })
  expect(isIOS()).toBe(false)
  stubNavigator({})
  expect(isIOS()).toBe(false)
})

it('isIPhone matches an iPhone but neither kind of iPad', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15' })
  expect(isIPhone()).toBe(true)
  stubNavigator({ userAgent: 'Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15' })
  expect(isIPhone()).toBe(false)
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 5 })
  expect(isIPhone()).toBe(false)
})

it('isMac matches a Mac but not an iPad, whichever way it reports itself', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15', platform: 'MacIntel', maxTouchPoints: 0 })
  expect(isMac()).toBe(true)
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 5 })
  expect(isMac()).toBe(false)
  stubNavigator({ userAgent: 'Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15' })
  expect(isMac()).toBe(false)
  stubNavigator({})
  expect(isMac()).toBe(false)
})
