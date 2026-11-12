import {
  activeConfig,
  attemptsExceeded,
  clearConfig,
  lockoutMs,
  lockoutRemainingMs,
  readAttempts,
  readConfig,
  recordFailedAttempt,
  resetAttempts,
  writeConfig,
} from './config'
import { checkSecret, hashSecret } from './secret'
import { signIn, testConfig } from './testUtils'

beforeEach(() => {
  localStorage.clear()
})

describe('config storage', () => {
  it('round-trips a config', () => {
    const config = testConfig()
    writeConfig(config)
    expect(readConfig()).toEqual(config)
  })

  it('reads a malformed config as none', () => {
    for (const raw of [
      'not json',
      '[]',
      JSON.stringify({ ...testConfig(), v: 2 }),
      JSON.stringify({ ...testConfig(), userId: '' }),
      JSON.stringify({ ...testConfig(), lockAfterMs: 1234 }),
      JSON.stringify({ ...testConfig(), secret: { ...testConfig().secret, length: 3 } }),
      JSON.stringify({ ...testConfig(), secret: { ...testConfig().secret, length: 9 } }),
      JSON.stringify({ ...testConfig(), secret: { ...testConfig().secret, kind: 'face' } }),
      JSON.stringify({ ...testConfig(), secret: { ...testConfig().secret, kind: 'pattern', length: 10 } }),
    ]) {
      localStorage.setItem('econumo.appLock', raw)
      expect(readConfig()).toBeNull()
    }
  })

  // Builds that offered biometrics stored a `biometric` field; it is ignored.
  it('reads a config that still holds a biometric field, dropping it', () => {
    for (const biometric of [null, { credentialId: 'abc' }, { credentialId: 5 }]) {
      localStorage.setItem('econumo.appLock', JSON.stringify({ ...testConfig(), biometric }))
      expect(readConfig()).toEqual(testConfig())
    }
  })

  it('clears the config and the attempts', () => {
    writeConfig(testConfig())
    recordFailedAttempt()
    clearConfig()
    expect(localStorage.getItem('econumo.appLock')).toBeNull()
    expect(localStorage.getItem('econumo.appLock.attempts')).toBeNull()
  })
})

describe('activeConfig', () => {
  it('applies to the signed-in owner only', () => {
    writeConfig(testConfig())
    expect(activeConfig()).toBeNull()
    signIn('u1')
    expect(activeConfig()?.userId).toBe('u1')
    signIn('u2')
    expect(activeConfig()).toBeNull()
    expect(readConfig()).not.toBeNull()
  })

  it('fails closed while the session has not recorded its user', () => {
    writeConfig(testConfig())
    signIn(null)
    expect(activeConfig()?.userId).toBe('u1')
  })

})

describe('attempts', () => {
  it('follows the lockout schedule', () => {
    expect([1, 2, 3, 4].map(lockoutMs)).toEqual([0, 0, 0, 0])
    expect([5, 6, 7, 8, 9].map(lockoutMs)).toEqual([30_000, 60_000, 300_000, 900_000, 900_000])
  })

  it('records failures with their lockout and resets', () => {
    const now = 1_000_000
    for (let i = 0; i < 4; i++) {
      expect(recordFailedAttempt(now).lockedUntil).toBeNull()
    }
    expect(recordFailedAttempt(now)).toEqual({ failedAttempts: 5, lockedUntil: now + 30_000 })
    expect(lockoutRemainingMs(now + 10_000)).toBe(20_000)
    resetAttempts()
    expect(readAttempts(now)).toEqual({ failedAttempts: 0, lockedUntil: null })
  })

  it('clamps a lockout set by a clock far ahead', () => {
    localStorage.setItem('econumo.appLock.attempts', JSON.stringify({ failedAttempts: 6, lockedUntil: 10 ** 13 }))
    expect(readAttempts(1000).lockedUntil).toBe(1000 + 900_000)
    expect(readAttempts(61_000).lockedUntil).toBe(1000 + 900_000)
    expect(lockoutRemainingMs(61_000)).toBe(840_000)
  })

  it('counts ten failures as exceeded', () => {
    for (let i = 0; i < 9; i++) {
      recordFailedAttempt()
    }
    expect(attemptsExceeded()).toBe(false)
    recordFailedAttempt()
    expect(attemptsExceeded()).toBe(true)
  })
})

describe('checkSecret', () => {
  beforeEach(async () => {
    writeConfig(testConfig({ secret: await hashSecret('2468', 1000) }))
  })

  it('unlocks with the right PIN and resets the counter', async () => {
    await checkSecret('0000', 0)
    await expect(checkSecret('2468', 0)).resolves.toEqual({ status: 'ok' })
    expect(readAttempts().failedAttempts).toBe(0)
  })

  it('counts the attempt before the PIN is derived', async () => {
    const pending = checkSecret('0000', 0)
    expect(readAttempts(0).failedAttempts).toBe(1)
    await expect(pending).resolves.toMatchObject({ status: 'wrong', failedAttempts: 1, remaining: 9 })
  })

  it('refuses during a lockout without counting, and signs out at the tenth failure', async () => {
    let now = 0
    for (let i = 1; i <= 4; i++) {
      await checkSecret('0000', now)
    }
    await expect(checkSecret('0000', now)).resolves.toMatchObject({ status: 'wrong', lockedUntil: 30_000 })
    await expect(checkSecret('2468', now + 1000)).resolves.toEqual({ status: 'locked-out', lockedUntil: 30_000 })
    expect(readAttempts(now).failedAttempts).toBe(5)
    for (let i = 6; i <= 9; i++) {
      now = readAttempts(now).lockedUntil!
      await checkSecret('0000', now)
    }
    now = readAttempts(now).lockedUntil!
    await expect(checkSecret('0000', now)).resolves.toEqual({ status: 'exceeded' })
    await expect(checkSecret('2468', now + 10 ** 9)).resolves.toEqual({ status: 'exceeded' })
  })

  it('reports a missing config', async () => {
    clearConfig()
    await expect(checkSecret('2468')).resolves.toEqual({ status: 'unconfigured' })
  })
})

describe('checkSecret with a pattern', () => {
  const RIGHT = 'p:0-1-2-5-8'
  const WRONG = 'p:6-7-8-5'

  beforeEach(async () => {
    writeConfig(testConfig({ secret: await hashSecret(RIGHT, 1000) }))
  })

  it('unlocks with the right pattern, counting the try before the derive', async () => {
    const pending = checkSecret(WRONG, 0)
    expect(readAttempts(0).failedAttempts).toBe(1)
    await expect(pending).resolves.toMatchObject({ status: 'wrong', failedAttempts: 1, remaining: 9 })
    await expect(checkSecret(RIGHT, 0)).resolves.toEqual({ status: 'ok' })
    expect(readAttempts().failedAttempts).toBe(0)
  })

  it('a PIN never opens a pattern lock and counts as a failure', async () => {
    await expect(checkSecret('0125', 0)).resolves.toMatchObject({ status: 'wrong', failedAttempts: 1 })
  })

  it('shares the lockout and the ten-try limit', async () => {
    let now = 0
    for (let i = 1; i <= 4; i++) {
      await checkSecret(WRONG, now)
    }
    await expect(checkSecret(WRONG, now)).resolves.toMatchObject({ status: 'wrong', lockedUntil: 30_000 })
    await expect(checkSecret(RIGHT, now + 1000)).resolves.toEqual({ status: 'locked-out', lockedUntil: 30_000 })
    for (let i = 6; i <= 9; i++) {
      now = readAttempts(now).lockedUntil!
      await checkSecret(WRONG, now)
    }
    now = readAttempts(now).lockedUntil!
    await expect(checkSecret(WRONG, now)).resolves.toEqual({ status: 'exceeded' })
  })
})

describe('configs from before patterns', () => {
  it('reads a stored `pin` without a kind as a PIN lock, and unlocks with it', async () => {
    signIn('u1')
    const { kind: _kind, ...pin } = await hashSecret('2468', 1000)
    const { secret: _secret, ...rest } = testConfig()
    localStorage.setItem('econumo.appLock', JSON.stringify({ ...rest, pin }))
    expect(readConfig()?.secret).toEqual({ kind: 'pin', ...pin })
    await expect(checkSecret('2468', 0)).resolves.toEqual({ status: 'ok' })
  })

  it('rewrites the config in the new shape on the next save', () => {
    const { secret, ...rest } = testConfig()
    const { kind: _kind, ...pin } = secret
    localStorage.setItem('econumo.appLock', JSON.stringify({ ...rest, pin }))
    writeConfig({ ...readConfig()!, lockAfterMs: 0 })
    const raw = JSON.parse(localStorage.getItem('econumo.appLock')!)
    expect(raw.pin).toBeUndefined()
    expect(raw.secret).toEqual(secret)
  })
})
