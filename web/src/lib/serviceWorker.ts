const SCRIPT = '/sw.js'
// also named in sw/sw.js, which reads it for offline starts
const CONFIG_CACHE = 'econumo-config'
const CONFIG_URL = '/econumo-config.js'
// A worker stopped mid-refresh never answers; the reload must not wait forever.
const REFRESH_TIMEOUT_MS = 60_000

function supported(): boolean {
  // dev serves no sw.js
  return import.meta.env.PROD && 'serviceWorker' in navigator
}

// Reaching this code is what proves the app runs with this config, so only here
// may it replace the one an offline start will use (see CONFIG in sw/sw.js).
export async function rememberRunningConfig(): Promise<void> {
  if (!supported() || !window.econumoConfig) {
    return
  }
  try {
    const cache = await caches.open(CONFIG_CACHE)
    await cache.put(
      CONFIG_URL,
      new Response(`window.econumoConfig = ${JSON.stringify(window.econumoConfig)};\n`, {
        headers: { 'Content-Type': 'text/javascript; charset=utf-8' },
      }),
    )
  } catch {
    // no Cache API or no quota: an offline start only finds an older config
  }
}

export function registerServiceWorker(): void {
  if (!supported()) {
    return
  }
  void rememberRunningConfig()
  window.addEventListener('load', () => {
    navigator.serviceWorker.register(SCRIPT).catch(() => {
      // no worker only means no offline start; the app itself is unaffected
    })
  })
}

// Downloads every cached app file again and checks for a new worker. The
// worker swaps the cache only once the download is complete, so a refresh
// against an unreachable server keeps the offline copy.
export async function refreshServiceWorkerCache(): Promise<void> {
  if (!supported()) {
    return
  }
  const registration = await navigator.serviceWorker.getRegistration()
  const worker = registration?.active
  if (!registration || !worker) {
    return
  }
  void registration.update().catch(() => {})
  await new Promise<void>((resolve) => {
    const timer = setTimeout(resolve, REFRESH_TIMEOUT_MS)
    const channel = new MessageChannel()
    channel.port1.onmessage = () => {
      clearTimeout(timer)
      resolve()
    }
    worker.postMessage({ type: 'refresh' }, [channel.port2])
  })
}
