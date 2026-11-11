import {
  defaultPinIterations,
  deriveJs,
  deriveNative,
  hashPin,
  isValidPin,
  PIN_ITERATIONS,
  PIN_ITERATIONS_JS,
  verifyPin,
} from './pin'
import type { PinHash } from './config'

function hex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

function withoutSubtle(): void {
  const real = globalThis.crypto
  vi.stubGlobal('crypto', { getRandomValues: real.getRandomValues.bind(real) })
}

// RFC 7914 section 11: PBKDF2-HMAC-SHA256, P="passwd", S="salt", c=1 (first 32 bytes).
const RFC_DK = '55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc'
// A fixed vector: PIN 2580, salt 0x00..0x0f, 1000 iterations, made by crypto.subtle.
const FIXED_SALT = new Uint8Array(Array.from({ length: 16 }, (_, i) => i))


afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PIN hashing', () => {
  it('verifies the PIN it hashed and nothing else', async () => {
    const stored = await hashPin('1234', 1000)
    expect(stored).toMatchObject({ iterations: 1000, length: 4 })
    expect(stored.hash).not.toContain('1234')
    await expect(verifyPin('1234', stored)).resolves.toBe(true)
    await expect(verifyPin('1235', stored)).resolves.toBe(false)
    await expect(verifyPin('12345', stored)).resolves.toBe(false)
  })

  it('salts every hash', async () => {
    const a = await hashPin('123456', 1000)
    const b = await hashPin('123456', 1000)
    expect(a.salt).not.toBe(b.salt)
    expect(a.hash).not.toBe(b.hash)
  })

  it('uses the stored iteration count', async () => {
    const stored = await hashPin('1234', 1000)
    await expect(verifyPin('1234', { ...stored, iterations: 1001 })).resolves.toBe(false)
  })

  it('rejects a damaged hash', async () => {
    const stored = await hashPin('1234', 1000)
    await expect(verifyPin('1234', { ...stored, hash: '%%%' })).resolves.toBe(false)
  })

  it('accepts 4 to 8 digits only', async () => {
    expect(isValidPin('1234')).toBe(true)
    expect(isValidPin('12345678')).toBe(true)
    expect(isValidPin('123')).toBe(false)
    expect(isValidPin('123456789')).toBe(false)
    expect(isValidPin('12a4')).toBe(false)
    expect(isValidPin('１２３４')).toBe(false)
    await expect(hashPin('123')).rejects.toThrow()
  })
})

describe('PIN hashing without crypto.subtle (plain http)', () => {
  it('matches the RFC 7914 PBKDF2-HMAC-SHA256 vector in both implementations', async () => {
    const salt = new TextEncoder().encode('salt')
    expect(hex(await deriveNative('passwd', salt, 1))).toBe(RFC_DK)
    expect(hex(await deriveJs('passwd', salt, 1))).toBe(RFC_DK)
  })

  it('derives the same bytes as crypto.subtle for a fixed salt and count', async () => {
    const native = await deriveNative('2580', FIXED_SALT, 1000)
    expect(hex(await deriveJs('2580', FIXED_SALT, 1000))).toBe(hex(native))
  })

  it('round-trips and uses the lower count for a new PIN', async () => {
    withoutSubtle()
    expect(defaultPinIterations()).toBe(PIN_ITERATIONS_JS)
    const stored = await hashPin('2580')
    expect(stored).toMatchObject({ iterations: PIN_ITERATIONS_JS, length: 4 })
    await expect(verifyPin('2580', stored)).resolves.toBe(true)
    await expect(verifyPin('2581', stored)).resolves.toBe(false)
  })

  it('verifies a hash made with crypto.subtle, and the other way round', async () => {
    expect(defaultPinIterations()).toBe(PIN_ITERATIONS)
    const fromNative = await hashPin('13579', 1000)
    const salt = btoa(String.fromCharCode(...FIXED_SALT))

    withoutSubtle()
    await expect(verifyPin('13579', fromNative)).resolves.toBe(true)
    await expect(verifyPin('13570', fromNative)).resolves.toBe(false)
    const fromJs = await hashPin('13579', 1000)
    const fixedJs: PinHash = { salt, hash: btoa(String.fromCharCode(...(await deriveJs('2580', FIXED_SALT, 1000)))), iterations: 1000, length: 4 }
    vi.unstubAllGlobals()

    await expect(verifyPin('13579', fromJs)).resolves.toBe(true)
    await expect(verifyPin('2580', fixedJs)).resolves.toBe(true)
  })
})
