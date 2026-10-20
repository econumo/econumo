/* The build prepends `self.__ECONUMO_SW__ = { version, files }` (see
 * vite.config.ts), naming the build and the shell files to precache. */
const { version, files } = self.__ECONUMO_SW__
// The shell is cached under "/": Go's file server redirects /index.html to it,
// and Safari refuses a redirected response for a navigation.
const SHELL_URL = '/'
const PREFIX = 'econumo-'
const SHELL = `${PREFIX}shell-${version}`
const RUNTIME = `${PREFIX}runtime`
// A launch on a dead connection (a tunnel, captive wifi) would otherwise hang
// on the network until the browser gives up; past this the cached shell opens.
const NAVIGATION_TIMEOUT_MS = 4000

// Never intercepted: the API and server-owned routes must hit the server, and
// a navigation to one of them must never be answered with the SPA shell.
const BYPASS = [/^\/api(\/|$)/, /^\/mcp(\/|$)/, /^\/oauth\/(register|token)$/, /^\/\.well-known\//, /^\/health$/, /^\/_(\/|$)/, /^\/liltag/]

self.addEventListener('install', (event) => {
  event.waitUntil(precache().then(() => self.skipWaiting()))
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith(PREFIX) && k !== SHELL && k !== RUNTIME).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('message', (event) => {
  if (event.data?.type === 'refresh') {
    const reply = event.ports?.[0]
    event.waitUntil(
      refresh().then(
        () => reply?.postMessage({ ok: true }),
        () => reply?.postMessage({ ok: false }),
      ),
    )
  }
})

self.addEventListener('fetch', (event) => {
  const request = event.request
  if (request.method !== 'GET') {
    return
  }
  const url = new URL(request.url)
  if (url.origin !== self.location.origin || BYPASS.some((re) => re.test(url.pathname))) {
    return
  }
  if (request.mode === 'navigate') {
    // client routes are extensionless; a navigation to a file goes to the network
    if (/\.[^/]+$/.test(url.pathname)) {
      return
    }
    event.respondWith(networkFirst(request, SHELL_URL, NAVIGATION_TIMEOUT_MS))
    return
  }
  if (url.pathname.startsWith('/assets/')) {
    event.respondWith(cacheFirst(request))
    return
  }
  // index.html, the runtime config, manifest, icons: keep their names across
  // deploys, so the network wins and the cache only covers being offline
  event.respondWith(networkFirst(request, request))
})

async function precache() {
  const cache = await caches.open(SHELL)
  await cache.addAll(files.map((f) => new Request(f, { cache: 'reload' })))
}

// "Clear cache and reload": download the whole shell again, and only once
// every file has arrived drop what was cached, so a failed refresh (server
// unreachable) leaves the app still able to start offline.
async function refresh() {
  const responses = await Promise.all(
    files.map(async (f) => {
      const response = await fetch(new Request(f, { cache: 'reload' }))
      if (!response.ok) {
        throw new Error(`refresh: ${f} ${response.status}`)
      }
      return [f, response]
    }),
  )
  const keys = await caches.keys()
  await Promise.all(keys.filter((k) => k.startsWith(PREFIX)).map((k) => caches.delete(k)))
  const cache = await caches.open(SHELL)
  await Promise.all(responses.map(([f, response]) => cache.put(f, response)))
}

async function cacheFirst(request) {
  const cached = await caches.match(request)
  if (cached) {
    return cached
  }
  const response = await fetch(request)
  if (response.ok) {
    const copy = response.clone()
    caches.open(RUNTIME).then((cache) => cache.put(request, copy))
  }
  return response
}

async function networkFirst(request, cacheKey, timeoutMs) {
  // no RequestInit: a navigation Request cannot be re-constructed with one,
  // and the server's Cache-Control: no-cache already forces revalidation
  const network = fetch(request).then((response) => {
    if (response.ok && !response.redirected) {
      const copy = response.clone()
      caches.open(SHELL).then((cache) => cache.put(cacheKey, copy))
    }
    return response
  })
  const fallback = async () => {
    const cached = await caches.match(cacheKey)
    if (cached) {
      return cached
    }
    return network
  }
  if (!timeoutMs) {
    return network.catch(fallback)
  }
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => fallback().then(resolve, reject), timeoutMs)
    network.then(
      (response) => {
        clearTimeout(timer)
        resolve(response)
      },
      () => {
        clearTimeout(timer)
        fallback().then(resolve, reject)
      },
    )
  })
}
