import { useEffect } from 'react'
import { create } from 'zustand'
import versions from '../../../compat/versions.json'
import { backendHost } from './config'
import { isNativeApp } from './platform'

// The app bundles a static econumo-config.js; the user's server holds the
// instance truth. Only these keys may cross over — everything else keeps its
// bundled default by design. INSTANCE_ID is server truth too: an app pointed
// at a self-hosted backend must report that backend's instance, not none.
// PASSWORD_LOGIN decides whether the login screen offers a password form at all.
const MERGED_KEYS = ['ALLOW_REGISTRATION', 'PASSWORD_LOGIN', 'INSTANCE_ID'] as const

// Minimum server version the bundled app is compatible with; an older server
// hard-blocks the app. The server's own floor arrives as MIN_APP_VERSION in
// its config (older servers without the key simply never block the app).
// Both floors live in compat/versions.json, shared with the Go backend.
export const MIN_SERVER_VERSION = versions.minServerVersion

// revision bumps whenever the merged keys change: screens that read them
// through window.econumoConfig subscribe to it to re-render. configHost is the
// server whose config was last requested (null = none yet); until that request
// succeeds the merged keys hold the bundled defaults.
export const useServerConfig = create<{
  serverVersion: string | null
  minAppVersion: string | null
  configHost: string | null
  revision: number
}>(() => ({
  serverVersion: null,
  minAppVersion: null,
  configHost: null,
  revision: 0,
}))

// Snapshot of the bundled values, taken before the first merge overwrites them.
const bundledDefaults = new WeakMap<object, Record<string, unknown>>()

// A server switch must not leave the previous server's settings in force while
// the new ones load (or when they never arrive): fall back to the bundled
// defaults, which keep every sign-in method visible and let the server decide.
function restoreBundledDefaults(host: string): void {
  const target = window.econumoConfig as Record<string, unknown>
  let defaults = bundledDefaults.get(target)
  if (!defaults) {
    defaults = Object.fromEntries(MERGED_KEYS.map((key) => [key, target[key]]))
    bundledDefaults.set(target, defaults)
  }
  for (const key of MERGED_KEYS) {
    target[key] = defaults[key]
  }
  useServerConfig.setState((s) => ({ serverVersion: null, minAppVersion: null, configHost: host, revision: s.revision + 1 }))
}

// The served file is executable JS (`window.econumoConfig = {...};`, generated
// whole by the Go server — see internal/web/router — or the static
// public/econumo-config.js fallback), not JSON — run it against a stub window
// instead of parsing it.
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
  const host = backendHost()
  if (useServerConfig.getState().configHost !== host) {
    restoreBundledDefaults(host)
  }
  try {
    const res = await fetch(`${host}/econumo-config.js`, { cache: 'no-store' })
    if (!res.ok) {
      return
    }
    const cfg = evalConfigScript(await res.text())
    // The user may have picked another server while this one answered.
    if (!cfg || backendHost() !== host) {
      return
    }
    const target = window.econumoConfig as Record<string, unknown>
    for (const key of MERGED_KEYS) {
      if (key in cfg) {
        target[key] = cfg[key]
      }
    }
    useServerConfig.setState((s) => ({
      serverVersion: typeof cfg.VERSION === 'string' ? cfg.VERSION : null,
      minAppVersion: typeof cfg.MIN_APP_VERSION === 'string' ? cfg.MIN_APP_VERSION : null,
      revision: s.revision + 1,
    }))
  } catch {
    // Non-fatal: offline boot keeps bundled defaults and cached data.
  }
}

// Auth screens let the user type a server address; in the app each address
// gets its own config, fetched once typing pauses. Returns the revision so the
// caller re-renders when the merged keys change.
export function useServerConfigFor(host: string): number {
  const revision = useServerConfig((s) => s.revision)
  const configHost = useServerConfig((s) => s.configHost)
  useEffect(() => {
    if (!isNativeApp() || configHost === host) {
      return
    }
    const timer = setTimeout(() => void fetchServerConfig(), 400)
    return () => clearTimeout(timer)
  }, [host, configHost])
  return revision
}
