import { isNativeApp } from './platform'

const SCRIPT = '/sw.js'
// A worker stopped mid-refresh never answers; the reload must not wait forever.
const REFRESH_TIMEOUT_MS = 60_000

function supported(): boolean {
  // dev serves no sw.js, and the native app ships its files inside the bundle
  return import.meta.env.PROD && !isNativeApp() && 'serviceWorker' in navigator
}

export function registerServiceWorker(): void {
  if (!supported()) {
    return
  }
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
