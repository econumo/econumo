// Credential encryption for pull-import sources (SimpleFIN access URLs).
//
// Credentials are encrypted at rest under a passphrase the server never
// learns: the server stores only ciphertext and a passphrase-wrapped data
// key, and the unwrapped data key is kept non-extractable in IndexedDB, so a
// script on the page can use it but not read its bytes. The server still
// sees the plaintext access URL for the duration of a sync request (it does
// the bank fetch).
//
// Formats are frozen: another client must be able to unlock what this one
// wrote.
//   kdf:            {"alg":"PBKDF2-SHA256","salt":<b64 16B>,"iterations":600000}
//   wrappedDataKey: v1:<b64 iv 12B>:<b64 AES-GCM(raw 32B data key)>
//   credential:     v1:<b64 iv 12B>:<b64 AES-GCM(utf8 plaintext)>

export interface WrappedKey {
  wrappedDataKey: string
  kdf: string
}

export const KDF_ITERATIONS = 600_000
// Floors for the kdf the SERVER hands back: it is untrusted input here, and a
// tampered row must fail loudly instead of deriving a weak key.
const MIN_KDF_ITERATIONS = 100_000
const SALT_BYTES = 16
const IV_BYTES = 12
const KDF_ALG = 'PBKDF2-SHA256'
const DB_NAME = 'econumo-import'
const STORE = 'keys'
const RECORD = 'dataKey'
const VERSION_TAG = 'v1'

export class WrongPassphraseError extends Error {
  constructor() {
    super('Wrong passphrase')
    this.name = 'WrongPassphraseError'
  }
}

export class KeyLockedError extends Error {
  constructor() {
    super('Credential key is locked')
    this.name = 'KeyLockedError'
  }
}

interface KdfParams {
  alg: string
  salt: string
  iterations: number
}

const b64 = (bytes: ArrayBuffer | Uint8Array): string => btoa(String.fromCharCode(...new Uint8Array(bytes)))
const unb64 = (s: string): Uint8Array => Uint8Array.from(atob(s), (c) => c.charCodeAt(0))

function encode(iv: Uint8Array, ct: ArrayBuffer): string {
  return `${VERSION_TAG}:${b64(iv)}:${b64(ct)}`
}

function decode(value: string, what: string): { iv: Uint8Array; ct: Uint8Array } {
  const parts = value.split(':')
  if (parts.length !== 3 || parts[0] !== VERSION_TAG) {
    throw new Error(`Malformed ${what}`)
  }
  let iv: Uint8Array
  let ct: Uint8Array
  try {
    iv = unb64(parts[1])
    ct = unb64(parts[2])
  } catch {
    throw new Error(`Malformed ${what}`)
  }
  if (iv.length !== IV_BYTES) {
    throw new Error(`Malformed ${what}`)
  }
  return { iv, ct }
}

function kdfParams(raw: string): KdfParams {
  let params: KdfParams
  try {
    params = JSON.parse(raw) as KdfParams
  } catch {
    throw new Error('Malformed key parameters')
  }
  if (params?.alg !== KDF_ALG) {
    throw new Error(`Unsupported kdf ${params?.alg}`)
  }
  let salt: Uint8Array
  try {
    salt = unb64(params.salt)
  } catch {
    throw new Error('Malformed key salt')
  }
  if (salt.length !== SALT_BYTES) {
    throw new Error('Malformed key salt')
  }
  if (!Number.isInteger(params.iterations) || params.iterations < MIN_KDF_ITERATIONS) {
    throw new Error('Key iterations below the minimum')
  }
  return params
}

async function wrappingKey(passphrase: string, params: KdfParams): Promise<CryptoKey> {
  if (params.alg !== KDF_ALG) {
    throw new Error(`Unsupported kdf ${params.alg}`)
  }
  const material = await crypto.subtle.importKey('raw', new TextEncoder().encode(passphrase), 'PBKDF2', false, ['deriveKey'])
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', hash: 'SHA-256', salt: unb64(params.salt) as BufferSource, iterations: params.iterations },
    material,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt'],
  )
}

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1)
    req.onupgradeneeded = () => req.result.createObjectStore(STORE)
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

function tx<T>(mode: IDBTransactionMode, run: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return openDb().then((db) => new Promise<T>((resolve, reject) => {
    const t = db.transaction(STORE, mode)
    const req = run(t.objectStore(STORE))
    t.oncomplete = () => { db.close(); resolve(req.result) }
    t.onerror = () => { db.close(); reject(t.error) }
    t.onabort = () => { db.close(); reject(t.error) }
  }))
}

// The unwrapped key plus the wrapped form it was unlocked from. The tag is
// how a device notices its key was superseded elsewhere: a passphrase reset
// mints a new data key, and this device's stale copy would otherwise report
// itself as unlocked and fail on the first decrypt.
export interface StoredKey {
  key: CryptoKey
  tag: string
}

export async function loadStoredKey(): Promise<StoredKey | null> {
  const value = await tx<StoredKey | CryptoKey | undefined>('readonly', (s) => s.get(RECORD) as IDBRequest<StoredKey | CryptoKey | undefined>)
  if (value instanceof CryptoKey) {
    return { key: value, tag: '' }  // untagged record from before tags existed: forces one re-unlock
  }
  return value ?? null
}

async function storeKey(key: CryptoKey, tag: string): Promise<void> {
  const record: StoredKey = { key, tag }
  await tx('readwrite', (s) => s.put(record, RECORD))
}

export async function forgetKey(): Promise<void> {
  await tx('readwrite', (s) => s.delete(RECORD))
}

async function wrap(rawDataKey: ArrayBuffer, passphrase: string, params: KdfParams): Promise<WrappedKey> {
  const wk = await wrappingKey(passphrase, params)
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: iv as BufferSource }, wk, rawDataKey)
  return { wrappedDataKey: encode(iv, ct), kdf: JSON.stringify(params) }
}

async function unwrapRaw(passphrase: string, wrapped: WrappedKey): Promise<ArrayBuffer> {
  const params = kdfParams(wrapped.kdf)
  const wk = await wrappingKey(passphrase, params)
  const { iv, ct } = decode(wrapped.wrappedDataKey, 'wrapped key')
  try {
    return await crypto.subtle.decrypt({ name: 'AES-GCM', iv: iv as BufferSource }, wk, ct as BufferSource)
  } catch {
    // AES-GCM authentication failure is the only way a wrong passphrase shows up
    throw new WrongPassphraseError()
  }
}

function importDataKey(raw: ArrayBuffer): Promise<CryptoKey> {
  return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
}

function freshParams(): KdfParams {
  return { alg: KDF_ALG, salt: b64(crypto.getRandomValues(new Uint8Array(16))), iterations: KDF_ITERATIONS }
}

export async function createKey(passphrase: string): Promise<WrappedKey> {
  const raw = crypto.getRandomValues(new Uint8Array(32)).buffer
  const wrapped = await wrap(raw, passphrase, freshParams())
  await storeKey(await importDataKey(raw), wrapped.wrappedDataKey)
  return wrapped
}

export async function unlockKey(passphrase: string, wrapped: WrappedKey): Promise<void> {
  const raw = await unwrapRaw(passphrase, wrapped)
  await storeKey(await importDataKey(raw), wrapped.wrappedDataKey)
}

// The data key itself never changes (existing ciphertexts stay valid); only
// the passphrase that wraps it does. Requires the old passphrase because the
// stored key is non-extractable — the raw bytes only exist unwrapped here.
export async function changePassphrase(oldPassphrase: string, wrapped: WrappedKey, newPassphrase: string): Promise<WrappedKey> {
  const raw = await unwrapRaw(oldPassphrase, wrapped)
  const rewrapped = await wrap(raw, newPassphrase, freshParams())
  await storeKey(await importDataKey(raw), rewrapped.wrappedDataKey)
  return rewrapped
}

async function requireKey(): Promise<CryptoKey> {
  const stored = await loadStoredKey()
  if (!stored) {
    throw new KeyLockedError()
  }
  return stored.key
}

export async function encryptCredential(plaintext: string): Promise<string> {
  const key = await requireKey()
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: iv as BufferSource }, key, new TextEncoder().encode(plaintext))
  return encode(iv, ct)
}

export async function decryptCredential(ciphertext: string): Promise<string> {
  const { iv, ct } = decode(ciphertext, 'ciphertext')
  const key = await requireKey()
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: iv as BufferSource }, key, ct as BufferSource)
  return new TextDecoder().decode(pt)
}
