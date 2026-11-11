import { hashPin, isValidPin, verifyPin } from './pin'

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
