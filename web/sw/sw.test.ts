import fs from 'node:fs'
import path from 'node:path'

const ORIGIN = 'https://econumo.test'
const FILES = ['/', '/assets/index-abc.js', '/econumo-config.js']
const source = fs.readFileSync(path.resolve(import.meta.dirname, 'sw.js'), 'utf8')

type Req = { url: string; method?: string; mode?: string }
type Handler = (event: unknown) => void

function key(x: string | Req): string {
  return new URL(typeof x === 'string' ? x : x.url, ORIGIN).href
}

function makeCaches() {
  const store = new Map<string, Map<string, Response>>()
  const open = async (name: string) => {
    if (!store.has(name)) store.set(name, new Map())
    const c = store.get(name)!
    return {
      put: async (k: string | Req, r: Response) => void c.set(key(k), r),
      match: async (k: string | Req) => c.get(key(k))?.clone(),
      addAll: async (reqs: Req[]) => {
        for (const r of reqs) {
          const res = await fetchMock(r)
          if (!res.ok) throw new Error('addAll')
          c.set(key(r), res)
        }
      },
    }
  }
  return {
    store,
    api: {
      open,
      keys: async () => [...store.keys()],
      delete: async (name: string) => store.delete(name),
      match: async (k: string | Req) => {
        for (const c of store.values()) {
          const hit = c.get(key(k))
          if (hit) return hit.clone()
        }
        return undefined
      },
    },
  }
}

let fetchMock: ReturnType<typeof vi.fn>
let caches: ReturnType<typeof makeCaches>
let handlers: Record<string, Handler>
let immediateTimers = false

function boot(buildHash = 'v1', files = FILES) {
  handlers = {}
  caches = caches ?? makeCaches()
  const self = {
    __ECONUMO_SW__: { buildHash, files },
    location: { origin: ORIGIN },
    addEventListener: (type: string, fn: Handler) => (handlers[type] = fn),
    skipWaiting: vi.fn(async () => {}),
    clients: { claim: vi.fn(async () => {}) },
  }
  const timer = (fn: () => void, ms: number) => setTimeout(fn, immediateTimers ? 0 : ms)
  // the worker builds relative Requests, which Node's Request cannot resolve
  const RequestShim = function (url: string, init?: object) {
    return { url: new URL(url, ORIGIN).href, method: 'GET', mode: 'cors', ...init }
  }
  new Function('self', 'caches', 'fetch', 'setTimeout', 'clearTimeout', 'Request', source)(self, caches.api, (r: Req) => fetchMock(r), timer, clearTimeout, RequestShim)
}

async function lifecycle(type: 'install' | 'activate') {
  let done: Promise<unknown> = Promise.resolve()
  handlers[type]({ waitUntil: (p: Promise<unknown>) => (done = p) })
  await done
}

function fetchEvent(pathname: string, mode = 'cors', method = 'GET') {
  const event = {
    request: { url: ORIGIN + pathname, method, mode },
    response: undefined as Promise<Response> | undefined,
    respondWith(p: Promise<Response>) {
      this.response = p
    },
    waitUntil() {},
  }
  handlers.fetch(event)
  return event
}

const online = (body = 'net') => vi.fn(async () => new Response(body, { status: 200 }))
const offline = () => vi.fn(async () => Promise.reject(new TypeError('Failed to fetch')))

beforeEach(async () => {
  immediateTimers = false
  caches = undefined as unknown as ReturnType<typeof makeCaches>
  fetchMock = vi.fn(async (r: Req) => new Response(`v1:${new URL(r.url, ORIGIN).pathname}`))
  boot()
  await lifecycle('install')
})

it('precaches every shell file on install', async () => {
  expect([...caches.store.get('econumo-shell-v1')!.keys()]).toEqual(FILES.map((f) => ORIGIN + f))
})

it('opens the network copy of the shell when online, leaving the offline copy to the precache', async () => {
  fetchMock = online('newer build')
  const event = fetchEvent('/budget/123', 'navigate')
  expect(await (await event.response!).text()).toBe('newer build')
  await new Promise((r) => setTimeout(r, 0))
  // its bundle is not cached yet: kept, the next offline launch would be blank
  expect(await (await caches.api.match('/'))!.text()).toBe('v1:/')
})

it('opens the cached shell offline', async () => {
  fetchMock = offline()
  const event = fetchEvent('/account/1', 'navigate')
  expect(await (await event.response!).text()).toBe('v1:/')
})

it('opens the cached shell when the network hangs', async () => {
  immediateTimers = true
  boot()
  await lifecycle('install')
  fetchMock = vi.fn(() => new Promise<Response>(() => {}))
  const event = fetchEvent('/', 'navigate')
  expect(await (await event.response!).text()).toBe('v1:/')
})

it('opens the cached shell when the server answers a 5xx', async () => {
  fetchMock = vi.fn(async () => new Response('bad gateway', { status: 502 }))
  const event = fetchEvent('/account/1', 'navigate')
  expect(await (await event.response!).text()).toBe('v1:/')
})

it('passes a 404 through instead of hiding it behind the cache', async () => {
  fetchMock = vi.fn(async () => new Response('gone', { status: 404 }))
  expect((await fetchEvent('/econumo-config.js').response!).status).toBe(404)
})

it('passes a 5xx through when nothing is cached for the request', async () => {
  fetchMock = vi.fn(async () => new Response('bad gateway', { status: 502 }))
  expect((await fetchEvent('/not-cached.png').response!).status).toBe(502)
})

it('answers the runtime config from the cache when the network hangs', async () => {
  immediateTimers = true
  boot()
  fetchMock = vi.fn(() => new Promise<Response>(() => {}))
  expect(await (await fetchEvent('/econumo-config.js').response!).text()).toBe('v1:/econumo-config.js')
})

it('answers the runtime config from the cache on a 5xx', async () => {
  fetchMock = vi.fn(async () => new Response('bad gateway', { status: 502 }))
  expect(await (await fetchEvent('/econumo-config.js').response!).text()).toBe('v1:/econumo-config.js')
})

it('does not keep a runtime config that arrived after the page moved on without it', async () => {
  immediateTimers = true
  boot()
  let arrive: (r: Response) => void = () => {}
  fetchMock = vi.fn(() => new Promise<Response>((resolve) => (arrive = resolve)))
  expect(await (await fetchEvent('/econumo-config.js').response!).text()).toBe('v1:/econumo-config.js')
  arrive(new Response('late'))
  await new Promise((r) => setTimeout(r, 0))
  expect(await (await caches.api.match('/econumo-config.js'))!.text()).toBe('v1:/econumo-config.js')
})

it.each([
  ['/api/v1/user/get-user-data', 'cors'],
  ['/api/doc', 'navigate'],
  ['/mcp', 'cors'],
  ['/oauth/token', 'cors'],
  ['/.well-known/oauth-authorization-server', 'navigate'],
  ['/health', 'cors'],
  ['/liltag-config.json', 'cors'],
  ['/shortcuts/econumo.shortcut', 'navigate'],
])('leaves %s to the network', (pathname, mode) => {
  expect(fetchEvent(pathname, mode).response).toBeUndefined()
})

it('leaves writes and other origins to the network', () => {
  expect(fetchEvent('/econumo-config.js', 'cors', 'POST').response).toBeUndefined()
  const event = { request: { url: 'https://t.econumo.com/js/twillingate.js', method: 'GET', mode: 'no-cors' }, respondWith: vi.fn() }
  handlers.fetch(event)
  expect(event.respondWith).not.toHaveBeenCalled()
})

it('serves fingerprinted assets from the cache without the network', async () => {
  fetchMock = vi.fn()
  const event = fetchEvent('/assets/index-abc.js')
  expect(await (await event.response!).text()).toBe('v1:/assets/index-abc.js')
  expect(fetchMock).not.toHaveBeenCalled()
})

it('prefers the network for the runtime config and falls back to the cache offline', async () => {
  fetchMock = online('window.econumoConfig = {new}')
  expect(await (await fetchEvent('/econumo-config.js').response!).text()).toBe('window.econumoConfig = {new}')
  fetchMock = offline()
  await new Promise((r) => setTimeout(r, 0))
  expect(await (await fetchEvent('/econumo-config.js').response!).text()).toBe('window.econumoConfig = {new}')
})

it('drops the caches of older builds on activate, and nothing else', async () => {
  caches.store.set('econumo-shell-v0', new Map())
  caches.store.set('someone-else', new Map())
  await lifecycle('activate')
  expect([...caches.store.keys()].sort()).toEqual(['econumo-shell-v1', 'someone-else'])
})

it('drops the runtime cache on activate', async () => {
  await (await caches.api.open('econumo-runtime')).put('/assets/old.js', new Response('old'))
  await lifecycle('activate')
  expect([...caches.store.keys()]).toEqual(['econumo-shell-v1'])
})

it('a new worker takes the new shell but keeps the runtime config the app last ran with', async () => {
  // the transactions on disk are keyed by that config's revision; the new one
  // would discard them on an offline launch, before anything can be downloaded
  await new Promise((r) => setTimeout(r, 0))
  fetchMock = vi.fn(async (r: Req) => new Response(`v2:${new URL(r.url, ORIGIN).pathname}`))
  boot('v2')
  await lifecycle('install')
  await lifecycle('activate')
  expect([...caches.store.keys()]).toEqual(['econumo-shell-v2'])
  expect(await (await caches.api.match('/'))!.text()).toBe('v2:/')
  expect(await (await caches.api.match('/econumo-config.js'))!.text()).toBe('v1:/econumo-config.js')
  expect(fetchMock).not.toHaveBeenCalledWith(expect.objectContaining({ url: `${ORIGIN}/econumo-config.js` }))
})

async function refresh(): Promise<{ ok: boolean }> {
  let done: Promise<unknown> = Promise.resolve()
  let reply: { ok: boolean } | undefined
  handlers.message({
    data: { type: 'refresh' },
    ports: [{ postMessage: (m: { ok: boolean }) => (reply = m) }],
    waitUntil: (p: Promise<unknown>) => (done = p),
  })
  await done
  return reply!
}

it('refresh downloads the shell again and drops every other cached file', async () => {
  await (await caches.api.open('econumo-runtime')).put('/assets/old.js', new Response('old'))
  fetchMock = vi.fn(async (r: Req) => new Response(`v2:${new URL(r.url, ORIGIN).pathname}`))
  expect(await refresh()).toEqual({ ok: true })
  expect([...caches.store.keys()]).toEqual(['econumo-shell-v1'])
  expect(await (await caches.api.match('/'))!.text()).toBe('v2:/')
})

it('refresh keeps the runtime config the app is running with', async () => {
  fetchMock = vi.fn(async (r: Req) => new Response(`v2:${new URL(r.url, ORIGIN).pathname}`))
  expect(await refresh()).toEqual({ ok: true })
  expect(await (await caches.api.match('/econumo-config.js'))!.text()).toBe('v1:/econumo-config.js')
})

it('refresh leaves the cache of a newer worker installing beside it', async () => {
  caches.store.set('econumo-shell-v2', new Map())
  expect(await refresh()).toEqual({ ok: true })
  expect([...caches.store.keys()].sort()).toEqual(['econumo-shell-v1', 'econumo-shell-v2'])
})

it('refuses a refresh once the server has moved to another build', async () => {
  fetchMock = vi.fn(async (r: Req) =>
    new URL(r.url).pathname === '/'
      ? new Response('<script src="/assets/index-new.js"></script>')
      : new Response(`v2:${new URL(r.url).pathname}`),
  )
  expect(await refresh()).toEqual({ ok: false })
  expect(await (await caches.api.match('/'))!.text()).toBe('v1:/')
})

it('accepts a refresh whose shell names only files of this build', async () => {
  fetchMock = vi.fn(async (r: Req) =>
    new URL(r.url).pathname === '/'
      ? new Response('<script src="/assets/index-abc.js"></script>')
      : new Response(`v2:${new URL(r.url).pathname}`),
  )
  expect(await refresh()).toEqual({ ok: true })
  expect(await (await caches.api.match('/'))!.text()).toContain('index-abc.js')
})

it('a failed refresh keeps the offline copy', async () => {
  fetchMock = vi.fn(async (r: Req) => (new URL(r.url).pathname === '/' ? new Response('down', { status: 502 }) : new Response('v2')))
  expect(await refresh()).toEqual({ ok: false })
  expect(await (await caches.api.match('/'))!.text()).toBe('v1:/')
})
