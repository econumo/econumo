import { pbkdf2Async } from '@noble/hashes/pbkdf2.js'
import { sha256 } from '@noble/hashes/sha2.js'
import {
  MAX_FAILED_ATTEMPTS,
  readAttempts,
  readConfig,
  recordFailedAttempt,
  resetAttempts,
  type PinHash,
} from './config'

export const PIN_ITERATIONS = 150_000
// Plain http has no crypto.subtle, and the pure-JS derive is roughly 7x slower
// than the native one: 150k rounds would hold a phone's unlock for seconds.
export const PIN_ITERATIONS_JS = 25_000
const PIN_PATTERN = /^\d{4,8}$/

export function isValidPin(pin: string): boolean {
  return PIN_PATTERN.test(pin)
}

function toBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary)
}

function fromBase64(value: string): Uint8Array<ArrayBuffer> | null {
  try {
    const binary = atob(value)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i)
    }
    return bytes
  } catch {
    return null
  }
}

export function nativeCryptoAvailable(): boolean {
  return typeof globalThis.crypto?.subtle?.deriveBits === 'function'
}

export function defaultPinIterations(): number {
  return nativeCryptoAvailable() ? PIN_ITERATIONS : PIN_ITERATIONS_JS
}

export async function deriveJs(pin: string, salt: Uint8Array, iterations: number): Promise<Uint8Array> {
  return pbkdf2Async(sha256, new TextEncoder().encode(pin), salt, { c: iterations, dkLen: 32 })
}

export async function deriveNative(pin: string, salt: Uint8Array<ArrayBuffer>, iterations: number): Promise<Uint8Array> {
  const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(pin), 'PBKDF2', false, ['deriveBits'])
  const bits = await crypto.subtle.deriveBits({ name: 'PBKDF2', hash: 'SHA-256', salt, iterations }, key, 256)
  return new Uint8Array(bits)
}

// Both produce the same PBKDF2-HMAC-SHA256 bytes, so a stored hash verifies
// with either; the stored iteration count always wins.
function derive(pin: string, salt: Uint8Array<ArrayBuffer>, iterations: number): Promise<Uint8Array> {
  return nativeCryptoAvailable() ? deriveNative(pin, salt, iterations) : deriveJs(pin, salt, iterations)
}

export async function hashPin(pin: string, iterations: number = defaultPinIterations()): Promise<PinHash> {
  if (!isValidPin(pin)) {
    throw new Error('invalid PIN')
  }
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const hash = await derive(pin, salt, iterations)
  return { salt: toBase64(salt), hash: toBase64(hash), iterations, length: pin.length }
}

export async function verifyPin(pin: string, stored: PinHash): Promise<boolean> {
  const salt = fromBase64(stored.salt)
  const expected = fromBase64(stored.hash)
  if (!salt || !expected || !isValidPin(pin) || pin.length !== stored.length) {
    return false
  }
  const actual = await derive(pin, salt, stored.iterations)
  if (actual.length !== expected.length) {
    return false
  }
  let diff = 0
  for (let i = 0; i < actual.length; i++) {
    diff |= actual[i] ^ expected[i]
  }
  return diff === 0
}

export type PinCheck =
  | { status: 'ok' }
  | { status: 'wrong'; failedAttempts: number; remaining: number; lockedUntil: number | null }
  | { status: 'locked-out'; lockedUntil: number }
  | { status: 'exceeded' }
  | { status: 'unconfigured' }

// The one PIN check for the lock screen and every settings prompt: they share
// the attempt counter, and the failure is stored before the slow derive runs.
export async function checkPin(pin: string, now: number = Date.now()): Promise<PinCheck> {
  const config = readConfig()
  if (config === null) {
    return { status: 'unconfigured' }
  }
  const before = readAttempts(now)
  if (before.failedAttempts >= MAX_FAILED_ATTEMPTS) {
    return { status: 'exceeded' }
  }
  if (before.lockedUntil !== null && before.lockedUntil > now) {
    return { status: 'locked-out', lockedUntil: before.lockedUntil }
  }
  const counted = recordFailedAttempt(now)
  if (await verifyPin(pin, config.pin)) {
    resetAttempts()
    return { status: 'ok' }
  }
  if (counted.failedAttempts >= MAX_FAILED_ATTEMPTS) {
    return { status: 'exceeded' }
  }
  return {
    status: 'wrong',
    failedAttempts: counted.failedAttempts,
    remaining: MAX_FAILED_ATTEMPTS - counted.failedAttempts,
    lockedUntil: counted.lockedUntil,
  }
}
