import type { AppLockConfig } from './config'

// jsdom reports no secure context; the lock is off without one.
export function setSecureContext(value: boolean): void {
  Object.defineProperty(window, 'isSecureContext', { value, configurable: true })
}

export function testConfig(overrides: Partial<AppLockConfig> = {}): AppLockConfig {
  return {
    v: 1,
    userId: 'u1',
    pin: { salt: 'c2FsdA==', hash: 'aGFzaA==', iterations: 1000, length: 4 },
    lockAfterMs: 60_000,
    biometric: null,
    ...overrides,
  }
}

export function signIn(userId: string | null = 'u1'): void {
  localStorage.setItem('token', 'tok')
  if (userId) {
    localStorage.setItem('sessionUserId', userId)
  }
}
