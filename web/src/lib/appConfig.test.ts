import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { evalConfigScript, fetchServerConfig, useServerConfig } from './appConfig'

beforeEach(() => {
  window.econumoConfig = { ALLOW_REGISTRATION: true, ANALYTICS: true, BILLING_URL: '' }
  useServerConfig.setState({ serverVersion: null })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const SERVED = `window.econumoConfig = {
  ALLOW_REGISTRATION: true,
  ANALYTICS: true,
  VERSION: null,
};
Object.assign(window.econumoConfig, {"ALLOW_REGISTRATION":false,"ANALYTICS":false,"VERSION":"v1.4.2","BILLING_URL":"https://x"});
`

it('evaluates the served config script including the server suffix', () => {
  expect(evalConfigScript(SERVED)).toMatchObject({
    ALLOW_REGISTRATION: false,
    ANALYTICS: false,
    VERSION: 'v1.4.2',
  })
  expect(evalConfigScript('not js {')).toBeNull()
})

it('merges only the allowlist and stores the server version separately', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(SERVED, { status: 200 })))
  await fetchServerConfig()
  expect(window.econumoConfig.ALLOW_REGISTRATION).toBe(false)
  expect(window.econumoConfig.ANALYTICS).toBe(false)
  expect(window.econumoConfig.BILLING_URL).toBe('') // not on the allowlist
  expect(window.econumoConfig.VERSION).toBeUndefined() // never merged
  expect(useServerConfig.getState().serverVersion).toBe('v1.4.2')
})

it('is non-fatal on network failure', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => {
    throw new Error('offline')
  }))
  await fetchServerConfig()
  expect(window.econumoConfig.ALLOW_REGISTRATION).toBe(true)
  expect(useServerConfig.getState().serverVersion).toBeNull()
})
