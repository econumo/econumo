import { isNativeApp } from '@/lib/platform'
import { APP_LOCK_DEVICE_KEY } from './keys'

export type BiometricResult = 'ok' | 'cancelled' | 'failed'

export type RegisterResult = { status: 'ok'; credentialId: string } | { status: 'cancelled' } | { status: 'failed' }

const TIMEOUT_MS = 60_000
// The OS prompt can background the page (Android shows it as a separate
// activity) and the visibility events trail it, so the grace outlasts it.
const CEREMONY_GRACE_MS = 2_000

let pendingCeremonies = 0
let ceremonyEndedAt = Number.NEGATIVE_INFINITY

export function ceremonyInFlight(now: number = Date.now()): boolean {
  const elapsed = now - ceremonyEndedAt
  // A clock set back must not hold the grace open.
  return pendingCeremonies > 0 || (elapsed >= 0 && elapsed < CEREMONY_GRACE_MS)
}

function track<T>(promise: Promise<T>): Promise<T> {
  pendingCeremonies++
  return promise.finally(() => {
    pendingCeremonies--
    ceremonyEndedAt = Date.now()
  })
}

export function resetBiometricForTests(): void {
  pendingCeremonies = 0
  ceremonyEndedAt = Number.NEGATIVE_INFINITY
}

export function toBase64Url(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function fromBase64Url(value: string): Uint8Array<ArrayBuffer> | null {
  try {
    const padded = value.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (value.length % 4)) % 4)
    const binary = atob(padded)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i)
    }
    return bytes
  } catch {
    return null
  }
}

export function isIpAddress(hostname: string): boolean {
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(hostname)) {
    return true
  }
  return hostname.includes(':') || /^\[.*\]$/.test(hostname)
}

// Plain http has no WebAuthn; the PIN lock still works there.
export function biometricNeedsHttps(): boolean {
  return !isNativeApp() && window.isSecureContext !== true
}

// WebAuthn needs a registrable domain: an IP-addressed self-hosted instance
// would let the user turn biometrics on and then never get a passkey.
export async function biometricAvailable(): Promise<boolean> {
  if (isNativeApp() || window.isSecureContext !== true || isIpAddress(location.hostname)) {
    return false
  }
  const pkc = (window as { PublicKeyCredential?: typeof PublicKeyCredential }).PublicKeyCredential
  if (!pkc || typeof pkc.isUserVerifyingPlatformAuthenticatorAvailable !== 'function') {
    return false
  }
  try {
    return (await pkc.isUserVerifyingPlatformAuthenticatorAvailable()) === true
  } catch {
    return false
  }
}

function randomBytes(length: number): Uint8Array<ArrayBuffer> {
  return crypto.getRandomValues(new Uint8Array(length))
}

function deviceId(): string {
  let id = localStorage.getItem(APP_LOCK_DEVICE_KEY)
  if (!id) {
    id = toBase64Url(randomBytes(16))
    localStorage.setItem(APP_LOCK_DEVICE_KEY, id)
  }
  return id
}

function isCancel(error: unknown): boolean {
  const name = (error as { name?: unknown } | null)?.name
  return name === 'NotAllowedError' || name === 'AbortError'
}

export interface BiometricUser {
  userId: string
  email: string
}

// Hashing the user id needs an await, and Safari only shows the passkey sheet
// when create() runs inside the click: prepare on mount, start on click.
export async function prepareBiometricRegistration(
  user: BiometricUser,
  existingCredentialId: string | null = null,
): Promise<() => Promise<RegisterResult>> {
  // Stable per user and device: re-enabling here replaces this device's
  // keychain entry instead of piling up passkeys, while the passkey synced from
  // another device has a different handle and is left alone.
  const userHandle = new Uint8Array(
    await crypto.subtle.digest('SHA-256', new TextEncoder().encode(`${user.userId}:${deviceId()}`)),
  )
  const existing = existingCredentialId ? fromBase64Url(existingCredentialId) : null
  return () => {
    const options: CredentialCreationOptions = {
      publicKey: {
        rp: { name: 'Econumo' },
        user: { id: userHandle, name: `${user.email} (app lock)`, displayName: 'Econumo app lock' },
        challenge: randomBytes(32),
        pubKeyCredParams: [
          { type: 'public-key', alg: -7 },
          { type: 'public-key', alg: -257 },
        ],
        authenticatorSelection: {
          authenticatorAttachment: 'platform',
          userVerification: 'required',
          residentKey: 'discouraged',
        },
        excludeCredentials: existing ? [{ type: 'public-key', id: existing }] : [],
        attestation: 'none',
        timeout: TIMEOUT_MS,
      },
    }
    let call: Promise<Credential | null>
    try {
      call = navigator.credentials.create(options)
    } catch {
      return Promise.resolve({ status: 'failed' })
    }
    return track(call).then(
      (credential): RegisterResult => {
        const rawId = (credential as PublicKeyCredential | null)?.rawId
        if (!rawId) {
          return { status: 'failed' }
        }
        return { status: 'ok', credentialId: toBase64Url(new Uint8Array(rawId)) }
      },
      (error: unknown): RegisterResult => {
        // excludeCredentials matched: this device already holds the stored passkey.
        if (existingCredentialId && (error as { name?: unknown } | null)?.name === 'InvalidStateError') {
          return { status: 'ok', credentialId: existingCredentialId }
        }
        return isCancel(error) ? { status: 'cancelled' } : { status: 'failed' }
      },
    )
  }
}

function decodeClientData(buffer: ArrayBuffer): Record<string, unknown> | null {
  try {
    const value: unknown = JSON.parse(new TextDecoder().decode(buffer))
    return typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : null
  } catch {
    return null
  }
}

// No signature check: there is no server to hold the public key, and anyone
// who can tamper with this code is outside the threat model. What is checked
// is that a real ceremony answered THIS request with user verification.
function accepted(credential: Credential | null, challenge: Uint8Array): boolean {
  const response = (credential as PublicKeyCredential | null)?.response as AuthenticatorAssertionResponse | undefined
  if (!response?.clientDataJSON || !response.authenticatorData) {
    return false
  }
  const clientData = decodeClientData(response.clientDataJSON)
  if (
    clientData === null ||
    clientData.type !== 'webauthn.get' ||
    clientData.challenge !== toBase64Url(challenge) ||
    clientData.origin !== location.origin
  ) {
    return false
  }
  const authenticatorData = new Uint8Array(response.authenticatorData)
  return authenticatorData.length > 32 && (authenticatorData[32] & 0x04) !== 0
}

// get() is called before anything is awaited, so this may run straight from
// a click handler.
export function verifyBiometric(credentialId: string, signal?: AbortSignal): Promise<BiometricResult> {
  const id = fromBase64Url(credentialId)
  if (id === null || id.length === 0) {
    return Promise.resolve('failed')
  }
  const challenge = randomBytes(32)
  let call: Promise<Credential | null>
  try {
    call = navigator.credentials.get({
      publicKey: {
        challenge,
        allowCredentials: [{ type: 'public-key', id, transports: ['internal'] }],
        userVerification: 'required',
        timeout: TIMEOUT_MS,
      },
      signal,
    })
  } catch {
    return Promise.resolve('failed')
  }
  return track(call).then(
    (credential): BiometricResult => (accepted(credential, challenge) ? 'ok' : 'failed'),
    (error: unknown): BiometricResult => (isCancel(error) ? 'cancelled' : 'failed'),
  )
}
