// Twillingate capture transport — the only file that knows the collector exists.
// Identified, not anonymous: once set, every batch carries a hashed user id
// ($user_id, opaque — never the raw id or $user_name) and a per-instance group
// ($group_id/$group_name identifying the deployment, not the person). Nothing
// is written to the device — $install_id and the identity fields all live in
// memory only, cleared by resetAnalyticsIdentity on logout (the group survives,
// since it describes the instance rather than the visitor).
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
let installId = uuidv4()
let queue: CapturedEvent[] = []
let timer: ReturnType<typeof setTimeout> | null = null

let userId: string | null = null
let groupId: string | null = null
let groupName: string | null = null

export function setAnalyticsUser(id: string | null): void {
  if (id === userId) {
    return
  }
  // Drain the queue under the outgoing identity first: capture() does not
  // snapshot who was current when an event was queued, so without this a
  // batch still sitting in the queue at an identity change would flush under
  // the new (or cleared) userId, misattributing it across people on a shared
  // device — the same class of leak the install-id re-mint below guards.
  flush()
  userId = id
}

export function setAnalyticsGroup(id: string, name: string): void {
  groupId = id
  groupName = name
}

// Called on logout. Flush first for the same reason as setAnalyticsUser, then
// re-mint the install id so the next person on a shared browser inherits
// nothing; the group is the deployment, not the person, so it stays.
export function resetAnalyticsIdentity(): void {
  flush()
  userId = null
  installId = uuidv4()
}

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
    attributes: {
      $install_id: installId,
      ...(userId ? { $user_id: userId } : {}),
      ...(groupId ? { $group_id: groupId, $group_name: groupName } : {}),
    },
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
