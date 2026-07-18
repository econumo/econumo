// PostHog capture transport — the only file that knows PostHog exists.
// Anonymous by construction: the distinct_id is a random per-page-load value
// held in memory (never persisted anywhere), and only the whitelisted
// properties built by the caller ever leave the browser.
// See docs/superpowers/specs/2026-07-17-posthog-analytics-design.md.

const POSTHOG_HOST = 'https://us.i.posthog.com'
// A PostHog project API key is public by design (it can only ingest events).
const POSTHOG_KEY = 'phc_nsMAM8nZ2N9Xh4PmCTuUpihHC2tHJnkMKKPdHxSHwDEk'
const FLUSH_AT = 10
const FLUSH_INTERVAL_MS = 10_000

interface CapturedEvent {
  event: string
  distinct_id: string
  timestamp: string
  properties: Record<string, unknown>
}

const distinctId = crypto.randomUUID()
let queue: CapturedEvent[] = []
let timer: ReturnType<typeof setTimeout> | null = null

export function analyticsDomain(hostname: string = window.location.hostname): string {
  if (hostname === 'econumo.com' || hostname.endsWith('.econumo.com')) {
    return hostname
  }
  return 'self-hosted'
}

export function capture(event: string, properties: Record<string, unknown> = {}): void {
  if (import.meta.env.MODE === 'development') {
    return
  }
  queue.push({
    event,
    distinct_id: distinctId,
    timestamp: new Date().toISOString(),
    properties: { ...properties, $process_person_profile: false },
  })
  if (queue.length >= FLUSH_AT) {
    flush()
    return
  }
  timer ??= setTimeout(flush, FLUSH_INTERVAL_MS)
}

function takeBatch(): string | null {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
  if (queue.length === 0) {
    return null
  }
  const body = JSON.stringify({ api_key: POSTHOG_KEY, batch: queue })
  queue = []
  return body
}

function flush(): void {
  const body = takeBatch()
  if (!body) {
    return
  }
  void fetch(`${POSTHOG_HOST}/batch/`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    keepalive: true,
    body,
  }).catch(() => {
    // Dropped on purpose: analytics must never break or noisy-log the app.
  })
}

// Flush the tail when the tab hides; sendBeacon survives page teardown.
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState !== 'hidden') {
    return
  }
  const body = takeBatch()
  if (!body) {
    return
  }
  try {
    navigator.sendBeacon(`${POSTHOG_HOST}/batch/`, body)
  } catch {
    // Same policy as flush: drop silently.
  }
})
