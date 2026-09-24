import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { evalConfigScript, fetchServerConfig, useServerConfig } from './appConfig'

beforeEach(() => {
  window.econumoConfig = { ALLOW_REGISTRATION: true, INSTANCE_ID: '', BILLING_URL: '' }
  useServerConfig.setState({ serverVersion: null, minAppVersion: null, configHost: null })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// The current server (internal/web/router builds the complete map,
// internal/web/spa writes it verbatim) emits the whole document as a single
// assignment.
const SERVED = `window.econumoConfig = {"ALLOW_REGISTRATION":false,"PASSWORD_LOGIN":false,"INSTANCE_ID":"a3f19c02b7d4","VERSION":"v1.4.2","MIN_APP_VERSION":"v1.1.0","BILLING_URL":"https://x"};
`

// The app can be pointed at any user-chosen backend, including one running an
// Econumo version older than this change, which still serves the dist file
// plus an Object.assign merge suffix (two statements). evalConfigScript must
// keep parsing this shape too.
const SERVED_LEGACY_TWO_STATEMENT = `window.econumoConfig = {
  ALLOW_REGISTRATION: true,
  INSTANCE_ID: '',
  VERSION: null,
};
Object.assign(window.econumoConfig, {"ALLOW_REGISTRATION":false,"INSTANCE_ID":"a3f19c02b7d4","VERSION":"v1.4.2","MIN_APP_VERSION":"v1.1.0","BILLING_URL":"https://x"});
`

it('evaluates the served config script (current single-assignment format)', () => {
  expect(evalConfigScript(SERVED)).toMatchObject({
    ALLOW_REGISTRATION: false,
    INSTANCE_ID: 'a3f19c02b7d4',
    VERSION: 'v1.4.2',
    MIN_APP_VERSION: 'v1.1.0',
  })
  expect(evalConfigScript('not js {')).toBeNull()
})

it('evaluates a legacy two-statement response from a pre-change server', () => {
  expect(evalConfigScript(SERVED_LEGACY_TWO_STATEMENT)).toMatchObject({
    ALLOW_REGISTRATION: false,
    INSTANCE_ID: 'a3f19c02b7d4',
    VERSION: 'v1.4.2',
    MIN_APP_VERSION: 'v1.1.0',
  })
})

it('merges only the allowlist and stores the version handshake separately', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(SERVED, { status: 200 })))
  await fetchServerConfig()
  expect(window.econumoConfig.ALLOW_REGISTRATION).toBe(false)
  expect(window.econumoConfig.PASSWORD_LOGIN).toBe(false)
  expect(window.econumoConfig.INSTANCE_ID).toBe('a3f19c02b7d4')
  expect(window.econumoConfig.BILLING_URL).toBe('') // not on the allowlist
  expect(window.econumoConfig.VERSION).toBeUndefined() // never merged
  expect('MIN_APP_VERSION' in window.econumoConfig).toBe(false) // never merged
  expect(useServerConfig.getState().serverVersion).toBe('v1.4.2')
  expect(useServerConfig.getState().minAppVersion).toBe('v1.1.0')
})

it('is non-fatal on network failure', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => {
    throw new Error('offline')
  }))
  await fetchServerConfig()
  expect(window.econumoConfig.ALLOW_REGISTRATION).toBe(true)
  expect(useServerConfig.getState().serverVersion).toBeNull()
  expect(useServerConfig.getState().minAppVersion).toBeNull()
})

function servedConfig(values: Record<string, unknown>) {
  return new Response(`window.econumoConfig = ${JSON.stringify(values)};\n`, { status: 200 })
}

// A server switch must never leave the previous server's settings in force:
// they describe a different instance.
it('drops the previous server config when switching servers', async () => {
  window.econumoConfig = { ALLOW_REGISTRATION: true, PASSWORD_LOGIN: true, INSTANCE_ID: '' }
  localStorage.setItem('selfHosted', 'true')
  window.Capacitor = { isNativePlatform: () => true }
  try {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (url.startsWith('https://a.example.test')) {
        return servedConfig({ PASSWORD_LOGIN: false, ALLOW_REGISTRATION: false, INSTANCE_ID: 'aaa', VERSION: 'v1.0.0' })
      }
      throw new Error('unreachable')
    }))
    localStorage.setItem('backendHost', JSON.stringify('https://a.example.test'))
    await fetchServerConfig()
    expect(window.econumoConfig.PASSWORD_LOGIN).toBe(false)
    const revision = useServerConfig.getState().revision

    localStorage.setItem('backendHost', JSON.stringify('https://b.example.test'))
    await fetchServerConfig()
    expect(window.econumoConfig.PASSWORD_LOGIN).toBe(true)
    expect(window.econumoConfig.ALLOW_REGISTRATION).toBe(true)
    expect(window.econumoConfig.INSTANCE_ID).toBe('')
    expect(useServerConfig.getState().serverVersion).toBeNull()
    expect(useServerConfig.getState().revision).toBeGreaterThan(revision)
  } finally {
    delete (window as { Capacitor?: unknown }).Capacitor
    localStorage.clear()
  }
})

it('ignores a response for a server that is no longer selected', async () => {
  window.econumoConfig = { PASSWORD_LOGIN: true }
  localStorage.setItem('selfHosted', 'true')
  window.Capacitor = { isNativePlatform: () => true }
  try {
    let release: () => void = () => {}
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    vi.stubGlobal('fetch', vi.fn(async () => {
      await gate
      return servedConfig({ PASSWORD_LOGIN: false })
    }))
    localStorage.setItem('backendHost', JSON.stringify('https://a.example.test'))
    const pending = fetchServerConfig()
    localStorage.setItem('backendHost', JSON.stringify('https://b.example.test'))
    release()
    await pending
    expect(window.econumoConfig.PASSWORD_LOGIN).toBe(true)
  } finally {
    delete (window as { Capacitor?: unknown }).Capacitor
    localStorage.clear()
  }
})
