import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { evalConfigScript, fetchServerConfig, useServerConfig } from './appConfig'

beforeEach(() => {
  window.econumoConfig = { ALLOW_REGISTRATION: true, INSTANCE_ID: '', BILLING_URL: '' }
  useServerConfig.setState({ serverVersion: null, minAppVersion: null })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// The current server (internal/web/router builds the complete map,
// internal/web/spa writes it verbatim) emits the whole document as a single
// assignment.
const SERVED = `window.econumoConfig = {"ALLOW_REGISTRATION":false,"INSTANCE_ID":"a3f19c02b7d4","VERSION":"v1.4.2","MIN_APP_VERSION":"v1.1.0","BILLING_URL":"https://x"};
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
