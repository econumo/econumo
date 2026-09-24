// Twillingate SDK wrapper — the only file that knows the collector exists.
// The SDK is injected from the collector (the served file carries the
// collector's origin, so it cannot be bundled) the first time an event is
// captured, which only happens for a signed-in user who has analytics on.
//
// Identified, never anonymous: nothing reaches the SDK until a hashed user id
// is set ($user_id, opaque — never the raw id or $user_name), alongside a
// per-instance group ($group_id/$group_name identifying the deployment, not
// the person). Calls made before the user resolves (the boot page view) or
// before the script loads wait here; resetAnalyticsIdentity discards whatever
// is still unattributed. The SDK runs without consent, so it keeps nothing on
// the device, sends $consent 0 and no $install_id.

import { forgetAuthMethods } from './analyticsAuthMethods'

const SDK_URL = 'https://t.econumo.com/js/twillingate.js'
// Named, so the SDK copy the cloud's liltag config loads for its own web
// analytics project keeps the default instance and the two never collide.
const INSTANCE = 'econumo'
// An ingest key is public by design (it can only ingest events). It belongs
// to the collector's product-analytics project — this wrapper is that
// project's only source.
const INGEST_KEY = 'ak_f81498e32742bc1ec94d35f60bcf7e0b'
// Same bound as the SDK's own pre-init hold: oldest dropped first.
const MAX_HELD = 500

interface TwillingateInstance {
  init(opts: Record<string, unknown>): unknown
  identify(user: string): void
  group(id: string, name?: string): void
  track(name: string, attrs?: Record<string, unknown>): void
  page(path: string, attrs?: Record<string, unknown>): void
  flush(): void
  reset(): void
}

interface TwillingateGlobal {
  get(name: string): TwillingateInstance | undefined
  create(name: string): TwillingateInstance
}

declare global {
  interface Window {
    twillingate?: TwillingateGlobal
  }
}

type Call = (sdk: TwillingateInstance) => void

let sdk: TwillingateInstance | null = null
let injected = false
let failed = false
let held: Call[] = []

let userId: string | null = null
let groupId: string | null = null
let groupName: string | null = null
// Session-wide attributes, set once by the caller rather than recomputed on
// every capture(), and stamped onto each event when it is captured.
let context: Record<string, unknown> = {}

export function setAnalyticsContext(attrs: Record<string, unknown>): void {
  context = attrs
}

export function setAnalyticsUser(id: string | null): void {
  if (id === userId) {
    return
  }
  // The SDK reads identity when it flushes, not when an event is captured:
  // send what is queued under the outgoing identity first, or a batch still
  // waiting at an identity change would go out under the new (or cleared)
  // user, misattributing it across people on a shared device.
  sdk?.flush()
  userId = id
  if (sdk) {
    applyIdentity(sdk)
  }
  release()
}

export function setAnalyticsGroup(id: string, name: string): void {
  groupId = id
  groupName = name
  sdk?.group(id, name)
}

// Called on logout and on a dead session (401). Flush first for the same
// reason as setAnalyticsUser, then drop what could not go out (events held
// for a user who never resolved) so the next person on a shared browser
// inherits nothing; the group is the deployment, not the person, so it stays.
export function resetAnalyticsIdentity(): void {
  sdk?.flush()
  userId = null
  held = []
  if (sdk) {
    applyIdentity(sdk)
  }
  // Describes the person whose session just ended, so it goes with it —
  // unlike the analytics opt-out, which is a device-level fail-safe and stays.
  forgetAuthMethods()
}

export function capture(event: string, properties: Record<string, unknown> = {}): void {
  const attrs = { ...context, ...properties }
  run((s) => s.track(event, attrs))
}

// path is the raw location, which is what the SDK dedupes on (a masked one
// would collapse /account/1 -> /account/2 into one view); properties must
// carry the masked $host and $path that are actually stored.
export function capturePageView(path: string, properties: Record<string, unknown>): void {
  const attrs = { ...context, ...properties }
  run((s) => s.page(path, attrs))
}

function run(call: Call): void {
  if (failed) {
    return
  }
  inject()
  if (sdk && userId) {
    call(sdk)
    return
  }
  if (held.length >= MAX_HELD) {
    held.shift()
  }
  held.push(call)
}

function release(): void {
  if (!sdk || !userId) {
    return
  }
  const calls = held
  held = []
  for (const call of calls) {
    call(sdk)
  }
}

function applyIdentity(s: TwillingateInstance): void {
  if (userId) {
    s.identify(userId)
    return
  }
  // reset() also clears the group, which describes the instance, not the person.
  s.reset()
  if (groupId) {
    s.group(groupId, groupName ?? undefined)
  }
}

function inject(): void {
  if (injected) {
    return
  }
  injected = true
  const script = document.createElement('script')
  script.src = SDK_URL
  script.async = true
  // No data-key: the tag loads dormant and is initialised in start().
  // data-instance registers the named instance whichever SDK copy got to the
  // page first.
  script.dataset.instance = INSTANCE
  script.referrerPolicy = 'no-referrer'
  script.addEventListener('load', () => {
    const global = window.twillingate
    if (!global) {
      fail()
      return
    }
    start(global.get(INSTANCE) ?? global.create(INSTANCE))
  })
  // Blocked or offline: analytics must never break or noisy-log the app.
  script.addEventListener('error', fail)
  document.head.appendChild(script)
}

function start(instance: TwillingateInstance): void {
  instance.init({
    key: INGEST_KEY,
    identity: 'identified',
    // Page views are sent explicitly (capturePageView) with the masked host
    // and path; nothing in the markup is tracked.
    autoPageviews: false,
    taggedEvents: false,
  })
  sdk = instance
  if (groupId) {
    instance.group(groupId, groupName ?? undefined)
  }
  if (userId) {
    instance.identify(userId)
  }
  release()
}

function fail(): void {
  failed = true
  held = []
}
