import {
  biometricAvailable,
  ceremonyInFlight,
  fromBase64Url,
  isIpAddress,
  prepareBiometricRegistration,
  resetBiometricForTests,
  toBase64Url,
  verifyBiometric,
} from './biometric'
import { setSecureContext } from './testUtils'

const credentials = { create: vi.fn(), get: vi.fn() }

function buffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.slice().buffer
}

function assertion(options: CredentialRequestOptions, overrides: { type?: string; challenge?: string; origin?: string; flags?: number } = {}) {
  const sent = new Uint8Array(options.publicKey!.challenge as ArrayBuffer)
  const clientData = {
    type: overrides.type ?? 'webauthn.get',
    challenge: overrides.challenge ?? toBase64Url(sent),
    origin: overrides.origin ?? location.origin,
  }
  const authenticatorData = new Uint8Array(37)
  authenticatorData[32] = overrides.flags ?? 0x05
  return {
    rawId: buffer(new Uint8Array([1, 2, 3])),
    response: {
      clientDataJSON: buffer(new TextEncoder().encode(JSON.stringify(clientData))),
      authenticatorData: buffer(authenticatorData),
    },
  }
}

function named(name: string): Error {
  const error = new Error(name)
  error.name = name
  return error
}

beforeEach(() => {
  credentials.create.mockReset()
  credentials.get.mockReset()
  Object.defineProperty(navigator, 'credentials', { value: credentials, configurable: true })
  setSecureContext(true)
  resetBiometricForTests()
})

afterEach(() => {
  setSecureContext(false)
  vi.unstubAllGlobals()
  delete window.Capacitor
})

describe('biometricAvailable', () => {
  const probe = vi.fn()

  beforeEach(() => {
    probe.mockReset().mockResolvedValue(true)
    vi.stubGlobal('PublicKeyCredential', { isUserVerifyingPlatformAuthenticatorAvailable: probe })
  })

  it('is true when the platform authenticator is there', async () => {
    await expect(biometricAvailable()).resolves.toBe(true)
  })

  it('is false when the probe says no or throws', async () => {
    probe.mockResolvedValue(false)
    await expect(biometricAvailable()).resolves.toBe(false)
    probe.mockRejectedValue(new Error('x'))
    await expect(biometricAvailable()).resolves.toBe(false)
  })

  it('is false in the native app and without a secure context', async () => {
    window.Capacitor = { isNativePlatform: () => true }
    await expect(biometricAvailable()).resolves.toBe(false)
    delete window.Capacitor
    setSecureContext(false)
    await expect(biometricAvailable()).resolves.toBe(false)
  })

  it('treats IP hosts as unsupported', () => {
    expect(isIpAddress('192.168.1.10')).toBe(true)
    expect(isIpAddress('[::1]')).toBe(true)
    expect(isIpAddress('localhost')).toBe(false)
    expect(isIpAddress('app.econumo.com')).toBe(false)
  })
})

describe('verifyBiometric', () => {
  it('accepts a verified answer to its own challenge', async () => {
    credentials.get.mockImplementation(async (options: CredentialRequestOptions) => assertion(options))
    await expect(verifyBiometric('AQID')).resolves.toBe('ok')
    const options = credentials.get.mock.calls[0][0] as CredentialRequestOptions
    expect(options.publicKey!.userVerification).toBe('required')
    expect(options.publicKey!.allowCredentials![0].transports).toEqual(['internal'])
    expect(new Uint8Array(options.publicKey!.allowCredentials![0].id as ArrayBuffer)).toEqual(new Uint8Array([1, 2, 3]))
  })

  it('calls get() synchronously', () => {
    credentials.get.mockReturnValue(new Promise(() => {}))
    void verifyBiometric('AQID')
    expect(credentials.get).toHaveBeenCalledOnce()
  })

  it('rejects another challenge, origin, type, or a missing UV flag', async () => {
    for (const overrides of [{ challenge: 'AAAA' }, { origin: 'https://evil.test' }, { type: 'webauthn.create' }, { flags: 0x01 }]) {
      credentials.get.mockImplementation(async (options: CredentialRequestOptions) => assertion(options, overrides))
      await expect(verifyBiometric('AQID')).resolves.toBe('failed')
    }
  })

  it('reports a cancel apart from a failure', async () => {
    credentials.get.mockRejectedValue(named('NotAllowedError'))
    await expect(verifyBiometric('AQID')).resolves.toBe('cancelled')
    credentials.get.mockRejectedValue(named('AbortError'))
    await expect(verifyBiometric('AQID')).resolves.toBe('cancelled')
    credentials.get.mockRejectedValue(named('SecurityError'))
    await expect(verifyBiometric('AQID')).resolves.toBe('failed')
  })

  it('marks the ceremony in flight and for two seconds after', async () => {
    let finish: (value: unknown) => void = () => {}
    credentials.get.mockReturnValue(new Promise((resolve) => (finish = resolve)))
    expect(ceremonyInFlight()).toBe(false)
    const pending = verifyBiometric('AQID')
    expect(ceremonyInFlight()).toBe(true)
    finish(null)
    await pending
    expect(ceremonyInFlight()).toBe(true)
    expect(ceremonyInFlight(Date.now() + 2_001)).toBe(false)
    expect(ceremonyInFlight(Date.now() - 60_000)).toBe(false)
  })
})

describe('prepareBiometricRegistration', () => {
  const user = { userId: 'u1', email: 'a@example.test' }

  it('registers a platform passkey with a per-device user handle', async () => {
    localStorage.setItem('econumo.appLock.device', 'dev-1')
    credentials.create.mockResolvedValue({ rawId: buffer(new Uint8Array([9, 8, 7])) })
    const start = await prepareBiometricRegistration(user)
    await expect(start()).resolves.toEqual({ status: 'ok', credentialId: toBase64Url(new Uint8Array([9, 8, 7])) })
    const options = (credentials.create.mock.calls[0][0] as CredentialCreationOptions).publicKey!
    const handle = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode('u1:dev-1')))
    expect(new Uint8Array(options.user.id as ArrayBuffer)).toEqual(handle)
    expect(options.user.name).toBe('a@example.test (app lock)')
    expect(options.authenticatorSelection).toMatchObject({ authenticatorAttachment: 'platform', userVerification: 'required' })
    expect(options.excludeCredentials).toEqual([])
  })

  it('keeps one handle per device and gives another device its own', async () => {
    credentials.create.mockResolvedValue({ rawId: buffer(new Uint8Array([1])) })
    localStorage.removeItem('econumo.appLock.device')
    await (await prepareBiometricRegistration(user))()
    await (await prepareBiometricRegistration(user))()
    localStorage.setItem('econumo.appLock.device', 'other-device')
    await (await prepareBiometricRegistration(user))()
    const ids = credentials.create.mock.calls.map((c) =>
      Array.from(new Uint8Array((c[0] as CredentialCreationOptions).publicKey!.user.id as ArrayBuffer)).join(','),
    )
    expect(ids[0]).toBe(ids[1])
    expect(ids[2]).not.toBe(ids[0])
  })

  it('calls create() synchronously from the prepared start', async () => {
    credentials.create.mockReturnValue(new Promise(() => {}))
    const start = await prepareBiometricRegistration(user)
    void start()
    expect(credentials.create).toHaveBeenCalledOnce()
  })

  it('keeps the stored passkey when the device already holds it', async () => {
    credentials.create.mockRejectedValue(named('InvalidStateError'))
    const start = await prepareBiometricRegistration(user, 'AQID')
    await expect(start()).resolves.toEqual({ status: 'ok', credentialId: 'AQID' })
    const options = (credentials.create.mock.calls[0][0] as CredentialCreationOptions).publicKey!
    expect(new Uint8Array(options.excludeCredentials![0].id as ArrayBuffer)).toEqual(fromBase64Url('AQID'))
  })

  it('reports a cancel or a failure', async () => {
    credentials.create.mockRejectedValue(named('NotAllowedError'))
    await expect((await prepareBiometricRegistration(user))()).resolves.toEqual({ status: 'cancelled' })
    credentials.create.mockRejectedValue(named('InvalidStateError'))
    await expect((await prepareBiometricRegistration(user))()).resolves.toEqual({ status: 'failed' })
  })
})
