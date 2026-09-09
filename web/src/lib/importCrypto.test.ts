import { IDBFactory } from 'fake-indexeddb'
import {
  KDF_ITERATIONS, KeyLockedError, WrongPassphraseError,
  changePassphrase, createKey, decryptCredential, encryptCredential, forgetKey, loadStoredKey, unlockKey,
} from './importCrypto'

beforeEach(() => {
  // a fresh IndexedDB per test; the module opens the db lazily on every call
  globalThis.indexedDB = new IDBFactory()
})

it('createKey stores a non-extractable key and returns a wrapped key with the documented kdf', async () => {
  const wrapped = await createKey('correct horse')
  expect(wrapped.wrappedDataKey).toMatch(/^v1:[A-Za-z0-9+/=]+:[A-Za-z0-9+/=]+$/)
  const kdf = JSON.parse(wrapped.kdf)
  expect(kdf.alg).toBe('PBKDF2-SHA256')
  expect(kdf.iterations).toBe(KDF_ITERATIONS)
  expect(atob(kdf.salt)).toHaveLength(16)
  const key = await loadStoredKey()
  expect(key?.extractable).toBe(false)
  expect(key?.usages.sort()).toEqual(['decrypt', 'encrypt'])
})

it('round-trips a credential and uses a fresh iv per call', async () => {
  await createKey('pw')
  const a = await encryptCredential('https://u:p@bridge.example/simplefin')
  const b = await encryptCredential('https://u:p@bridge.example/simplefin')
  expect(a).not.toBe(b)
  expect(await decryptCredential(a)).toBe('https://u:p@bridge.example/simplefin')
  expect(await decryptCredential(b)).toBe('https://u:p@bridge.example/simplefin')
})

it('unlockKey restores the same data key on a new device; the wrong passphrase is rejected', async () => {
  const wrapped = await createKey('pw')
  const ct = await encryptCredential('secret')
  await forgetKey()
  expect(await loadStoredKey()).toBeNull()
  await expect(encryptCredential('x')).rejects.toBeInstanceOf(KeyLockedError)
  await expect(unlockKey('nope', wrapped)).rejects.toBeInstanceOf(WrongPassphraseError)
  await unlockKey('pw', wrapped)
  expect(await decryptCredential(ct)).toBe('secret')
})

it('changePassphrase re-wraps the SAME data key, so old ciphertexts still decrypt', async () => {
  const wrapped = await createKey('old')
  const ct = await encryptCredential('secret')
  const rewrapped = await changePassphrase('old', wrapped, 'new')
  expect(rewrapped.wrappedDataKey).not.toBe(wrapped.wrappedDataKey)
  await forgetKey()
  await expect(unlockKey('old', rewrapped)).rejects.toBeInstanceOf(WrongPassphraseError)
  await unlockKey('new', rewrapped)
  expect(await decryptCredential(ct)).toBe('secret')
})

it('rejects a malformed ciphertext without touching the key', async () => {
  await createKey('pw')
  await expect(decryptCredential('garbage')).rejects.toThrow(/ciphertext/)
  await expect(decryptCredential('v2:a:b')).rejects.toThrow(/ciphertext/)
})

it('rejects a tampered iv and tampered key parameters', async () => {
  const wrapped = await createKey('pw')
  const ct = await encryptCredential('secret')
  // the 12-byte iv is part of the frozen format; anything else is tampering
  await expect(decryptCredential(`v1:${btoa('shortiv')}:${ct.split(':')[2]}`)).rejects.toThrow(/ciphertext/)

  const tampered = (over: Record<string, unknown>) =>
    ({ ...wrapped, kdf: JSON.stringify({ ...JSON.parse(wrapped.kdf), ...over }) })
  for (const over of [{ alg: 'PBKDF2-SHA1' }, { iterations: 1000 }, { salt: btoa('short') }]) {
    const err: unknown = await unlockKey('pw', tampered(over)).then(() => null, (e: unknown) => e)
    expect(err).toBeInstanceOf(Error)
    expect(err).not.toBeInstanceOf(WrongPassphraseError)
  }
  await expect(unlockKey('pw', { ...wrapped, kdf: 'not json' })).rejects.toThrow(/key parameters/)
})
