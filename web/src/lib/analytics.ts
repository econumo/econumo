// Twillingate capture transport — the only file that knows the collector exists.
// Anonymous by construction: $install_id is a random per-page-load value held in
// memory (never persisted anywhere), and only the whitelisted attributes built by
// the caller ever leave the browser. The collector salts and daily-rotates the
// id on its side, so nothing links a visitor across days.
// Wire format: POST /api/events, one batch per flush.

import { v4 as uuidv4, v7 as uuidv7 } from 'uuid'

const COLLECTOR_URL = 'https://t.kuznetsov.dev/api/events'
// An ingest key is public by design (it can only ingest events).
const INGEST_KEY = 'ak_be90b906b92397705359981ebced78ef'
const FLUSH_AT = 10
const FLUSH_INTERVAL_MS = 10_000

interface CapturedEvent {
  id: string
  ts: string
  name: string
  attributes: Record<string, unknown>
}

// Not crypto.randomUUID: that one is secure-context-only, so it is missing on a
// self-hosted instance served over plain http://<lan-ip>, and this module-level
// call would take the whole SPA down there. uuid falls back to getRandomValues.
const installId = uuidv4()
let queue: CapturedEvent[] = []
let timer: ReturnType<typeof setTimeout> | null = null

export function analyticsDomain(hostname: string = window.location.hostname): string {
  if (hostname === 'econumo.com' || hostname.endsWith('.econumo.com')) {
    return hostname
  }
  return 'self-hosted'
}

export function capture(event: string, properties: Record<string, unknown> = {}): void {
  queue.push({
    // v7 so ids sort by time; supplying one at all is what makes a batch that
    // was retried after a timeout land as a no-op instead of double-counting.
    id: uuidv7(),
    ts: new Date().toISOString(),
    name: event,
    attributes: properties,
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
  // The key travels in the body, not a header: sendBeacon cannot set headers,
  // and beacons are the only transport that survives page teardown.
  const body = JSON.stringify({
    key: INGEST_KEY,
    attributes: { $install_id: installId },
    events: queue,
  })
  queue = []
  return body
}

function flush(): void {
  const body = takeBatch()
  if (!body) {
    return
  }
  void fetch(COLLECTOR_URL, {
    method: 'POST',
    // text/plain keeps the request CORS-simple, so no preflight round trip.
    headers: { 'Content-Type': 'text/plain' },
    keepalive: true,
    body,
  }).catch(() => {
    // Dropped on purpose: analytics must never break or noisy-log the app.
    // Dropping is also the correct retry policy — the collector treats every
    // 4xx as a poison batch, and a failed flush is not worth a second one.
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
    navigator.sendBeacon(COLLECTOR_URL, body)
  } catch {
    // Same policy as flush: drop silently.
  }
})
