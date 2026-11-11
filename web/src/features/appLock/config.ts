import { mirrorWrite } from '@/lib/appStorage'
import { getSessionUserId, getToken } from '@/lib/storage'
import { APP_LOCK_ATTEMPTS_KEY, APP_LOCK_CONFIG_KEY } from './keys'

export interface PinHash {
  salt: string
  hash: string
  iterations: number
  length: number
}

export interface AppLockConfig {
  v: 1
  userId: string
  pin: PinHash
  lockAfterMs: number
  biometric: { credentialId: string } | null
}

export interface AppLockAttempts {
  failedAttempts: number
  lockedUntil: number | null
}

export const LOCK_AFTER_OPTIONS = [0, 60_000, 300_000, 900_000] as const
export const DEFAULT_LOCK_AFTER_MS = 60_000
export const MAX_FAILED_ATTEMPTS = 10
export const MAX_LOCKOUT_MS = 15 * 60_000

const NO_ATTEMPTS: AppLockAttempts = { failedAttempts: 0, lockedUntil: null }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function nonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value !== ''
}

function parse(key: string): unknown {
  const raw = localStorage.getItem(key)
  if (raw === null) {
    return null
  }
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function isPinHash(value: unknown): value is PinHash {
  return (
    isRecord(value) &&
    nonEmptyString(value.salt) &&
    nonEmptyString(value.hash) &&
    Number.isInteger(value.iterations) &&
    (value.iterations as number) > 0 &&
    Number.isInteger(value.length) &&
    (value.length as number) >= 4 &&
    (value.length as number) <= 8
  )
}

export function readConfig(): AppLockConfig | null {
  const value = parse(APP_LOCK_CONFIG_KEY)
  if (!isRecord(value) || value.v !== 1 || !nonEmptyString(value.userId) || !isPinHash(value.pin)) {
    return null
  }
  if (!(LOCK_AFTER_OPTIONS as readonly unknown[]).includes(value.lockAfterMs)) {
    return null
  }
  const biometric = value.biometric
  if (biometric !== null && !(isRecord(biometric) && nonEmptyString(biometric.credentialId))) {
    return null
  }
  return {
    v: 1,
    userId: value.userId,
    pin: { salt: value.pin.salt, hash: value.pin.hash, iterations: value.pin.iterations, length: value.pin.length },
    lockAfterMs: value.lockAfterMs as number,
    biometric: biometric === null ? null : { credentialId: (biometric as { credentialId: string }).credentialId },
  }
}

function store(key: string, value: unknown): void {
  const encoded = JSON.stringify(value)
  localStorage.setItem(key, encoded)
  mirrorWrite(key, encoded)
}

function drop(key: string): void {
  localStorage.removeItem(key)
  mirrorWrite(key, null)
}

export function writeConfig(config: AppLockConfig): void {
  store(APP_LOCK_CONFIG_KEY, config)
}

export function clearConfig(): void {
  drop(APP_LOCK_CONFIG_KEY)
  drop(APP_LOCK_ATTEMPTS_KEY)
}

// Fails closed: a session that has not recorded its user yet still locks. Only
// setToken, which knows both ids, ever deletes another user's config.
export function activeConfig(): AppLockConfig | null {
  if (getToken() === null) {
    return null
  }
  const config = readConfig()
  if (config === null) {
    return null
  }
  const sessionUserId = getSessionUserId()
  if (sessionUserId !== null && sessionUserId !== config.userId) {
    return null
  }
  return config
}

// The delay before the next try once `failures` attempts have failed.
export function lockoutMs(failures: number): number {
  if (failures < 5) {
    return 0
  }
  return [30_000, 60_000, 300_000, MAX_LOCKOUT_MS, MAX_LOCKOUT_MS][Math.min(failures, 9) - 5]
}

export function readAttempts(now: number = Date.now()): AppLockAttempts {
  const value = parse(APP_LOCK_ATTEMPTS_KEY)
  if (!isRecord(value)) {
    return { ...NO_ATTEMPTS }
  }
  const failed = Number.isInteger(value.failedAttempts) && (value.failedAttempts as number) > 0 ? (value.failedAttempts as number) : 0
  let lockedUntil = typeof value.lockedUntil === 'number' && Number.isFinite(value.lockedUntil) ? value.lockedUntil : null
  // A clock set back must not stretch a lockout beyond the longest one.
  // Stored back, so later reads count down from the cap instead of re-capping.
  if (lockedUntil !== null && lockedUntil > now + MAX_LOCKOUT_MS) {
    lockedUntil = now + MAX_LOCKOUT_MS
    store(APP_LOCK_ATTEMPTS_KEY, { failedAttempts: failed, lockedUntil })
  }
  return { failedAttempts: failed, lockedUntil }
}

export function lockoutRemainingMs(now: number = Date.now()): number {
  const { lockedUntil } = readAttempts(now)
  return lockedUntil === null ? 0 : Math.max(0, lockedUntil - now)
}

export function attemptsExceeded(): boolean {
  return readAttempts().failedAttempts >= MAX_FAILED_ATTEMPTS
}

// Written BEFORE the PIN is checked, so a reload during the check cannot buy
// a free attempt; a correct PIN then resets it.
export function recordFailedAttempt(now: number = Date.now()): AppLockAttempts {
  const failedAttempts = readAttempts(now).failedAttempts + 1
  const delay = lockoutMs(failedAttempts)
  const next: AppLockAttempts = { failedAttempts, lockedUntil: delay > 0 ? now + delay : null }
  store(APP_LOCK_ATTEMPTS_KEY, next)
  return next
}

export function resetAttempts(): void {
  if (localStorage.getItem(APP_LOCK_ATTEMPTS_KEY) !== null) {
    drop(APP_LOCK_ATTEMPTS_KEY)
  }
}
