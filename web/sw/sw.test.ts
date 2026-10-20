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

function boot() {
  handlers = {}
  caches = makeCaches()
  const self = {
    __ECONUMO_SW__: { version: 'v1', files: FILES },
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
  }
  handlers.fetch(event)
  return event
}

const online = (body = 'net') => vi.fn(async () => new Response(body, { status: 200 }))
const offline = () => vi.fn(async () => Promise.reject(new TypeError('Failed to fetch')))

beforeEach(async () => {
  immediateTimers = false
  fetchMock = vi.fn(async (r: Req) => new Response(`v1:${new URL(r.url, ORIGIN).pathname}`))
  boot()
  await lifecycle('install')
})

it('precaches every shell file on install', async () => {
  expect([...caches.store.get('econumo-shell-v1')!.keys()]).toEqual(FILES.map((f) => ORIGIN + f))
})

it('opens the network copy of the shell when online and keeps it for later', async () => {
  fetchMock = online('fresh shell')
  const event = fetchEvent('/budget/123', 'navigate')
  expect(await (await event.response!).text()).toBe('fresh shell')
  await new Promise((r) => setTimeout(r, 0))
  expect(await (await caches.api.match('/'))!.text()).toBe('fresh shell')
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

it('a failed refresh keeps the offline copy', async () => {
  fetchMock = vi.fn(async (r: Req) => (new URL(r.url).pathname === '/' ? new Response('down', { status: 502 }) : new Response('v2')))
  expect(await refresh()).toEqual({ ok: false })
  expect(await (await caches.api.match('/'))!.text()).toBe('v1:/')
})
