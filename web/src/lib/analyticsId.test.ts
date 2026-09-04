import { describe, expect, it } from 'vitest'
import { analyticsUserId, sha256Hex } from './analyticsId'

async function webcrypto(input: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(input))
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
}

describe('sha256Hex', () => {
  it('matches the NIST vectors', () => {
    expect(sha256Hex('')).toBe('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
    expect(sha256Hex('abc')).toBe('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  })

  // Lengths either side of the 64-byte block and the 55/56-byte padding edge.
  it.each([0, 1, 55, 56, 63, 64, 65, 119, 120, 200])('matches webcrypto at %i bytes', async (n) => {
    const input = 'x'.repeat(n)
    expect(sha256Hex(input)).toBe(await webcrypto(input))
  })

  it('hashes multi-byte characters as UTF-8', async () => {
    expect(sha256Hex('日本語 café')).toBe(await webcrypto('日本語 café'))
  })
})

describe('analyticsUserId', () => {
  it('is the first 32 chars of the domain-separated digest', () => {
    const id = '018f1e5c-0000-7000-8000-000000000001'
    expect(analyticsUserId(id)).toBe(sha256Hex(`econumo:analytics:v1:${id}`).slice(0, 32))
    expect(analyticsUserId(id)).toHaveLength(32)
  })

  it('is stable and distinct per user', () => {
    expect(analyticsUserId('a')).toBe(analyticsUserId('a'))
    expect(analyticsUserId('a')).not.toBe(analyticsUserId('b'))
  })
})
