import { create } from 'zustand'
import versions from '../../../compat/versions.json'
import { backendHost } from './config'

// The app bundles a static econumo-config.js; the user's server holds the
// instance truth. Only these keys may cross over — everything else keeps its
// bundled default by design.
const MERGED_KEYS = ['ALLOW_REGISTRATION', 'ANALYTICS'] as const

// Minimum server version the bundled app is compatible with; an older server
// hard-blocks the app. The server's own floor arrives as MIN_APP_VERSION in
// its config (older servers without the key simply never block the app).
// Both floors live in compat/versions.json, shared with the Go backend.
export const MIN_SERVER_VERSION = versions.minServerVersion

export const useServerConfig = create<{
  serverVersion: string | null
  minAppVersion: string | null
}>(() => ({
  serverVersion: null,
  minAppVersion: null,
}))

// The served file is executable JS (`window.econumoConfig = {...}` plus an
// Object.assign suffix the server appends), not JSON — run it against a stub
// window instead of parsing it.
export function evalConfigScript(text: string): Record<string, unknown> | null {
  try {
    const stub: { econumoConfig: Record<string, unknown> } = { econumoConfig: {} }
    // oxlint-disable-next-line no_function_constructor
    new Function('window', text)(stub)
    return stub.econumoConfig
  } catch {
    return null
  }
}

export async function fetchServerConfig(): Promise<void> {
  try {
    const res = await fetch(`${backendHost()}/econumo-config.js`, { cache: 'no-store' })
    if (!res.ok) {
      return
    }
    const cfg = evalConfigScript(await res.text())
    if (!cfg) {
      return
    }
    const target = window.econumoConfig as Record<string, unknown>
    for (const key of MERGED_KEYS) {
      if (key in cfg) {
        target[key] = cfg[key]
      }
    }
    useServerConfig.setState({
      serverVersion: typeof cfg.VERSION === 'string' ? cfg.VERSION : null,
      minAppVersion: typeof cfg.MIN_APP_VERSION === 'string' ? cfg.MIN_APP_VERSION : null,
    })
  } catch {
    // Non-fatal: offline boot keeps bundled defaults and cached data.
  }
}
