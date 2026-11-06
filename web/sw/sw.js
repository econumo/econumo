/* The build prepends `self.__ECONUMO_SW__ = { buildHash, files }` (see
 * vite.config.ts), naming the build and the shell files to precache. */
const { buildHash, files } = self.__ECONUMO_SW__
// The shell is cached under "/": Go's file server redirects /index.html to it,
// and Safari refuses a redirected response for a navigation.
const SHELL_URL = '/'
const CONFIG_URL = '/econumo-config.js'
const PREFIX = 'econumo-'
const SHELL = `${PREFIX}shell-${buildHash}`
// Stored by the page that booted with it (rememberRunningConfig in
// serviceWorker.ts), in a cache that outlives every worker, and only ever read
// here. It must stay the config the app last ran with: its VERSION_REF keys the
// data on disk (getCacheBuster), and an offline launch under a config the app
// never ran with would discard that data with nothing able to download it again.
const CONFIG = `${PREFIX}config`
const RUNTIME = `${PREFIX}runtime`
// A launch on a dead connection (a tunnel, captive wifi) would otherwise hang
// on the network until the browser gives up, and econumo-config.js blocks the
// parser, so past this every shell file opens from the cache.
const NETWORK_TIMEOUT_MS = 4000
// A page opened from the cached shell runs the cached bundle, so its config
// must be the cached one too, even if the server answers that request: a newer
// build's config would have the old bundle discard the stored data as foreign.
const CACHED_SHELL_WINDOW_MS = 10_000
let cachedShellOpenedAt = -Infinity

// Never intercepted: the API and server-owned routes must hit the server, and
// a navigation to one of them must never be answered with the SPA shell.
const BYPASS = [/^\/api(\/|$)/, /^\/mcp(\/|$)/, /^\/oauth\/(register|token)$/, /^\/\.well-known\//, /^\/health$/, /^\/_(\/|$)/, /^\/liltag/]

self.addEventListener('install', (event) => {
  event.waitUntil(precache().then(() => self.skipWaiting()))
})

self.addEventListener('activate', (event) => {
  event.waitUntil(activate())
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
    // Not kept: the offline shell is the precached one, whose bundle is known
    // to be cached. A newer build's page kept here would open blank offline.
    event.respondWith(networkFirst(event, SHELL_URL))
    return
  }
  if (url.pathname.startsWith('/assets/')) {
    event.respondWith(cacheFirst(event))
    return
  }
  if (url.pathname === CONFIG_URL && Date.now() - cachedShellOpenedAt < CACHED_SHELL_WINDOW_MS) {
    event.respondWith(caches.match(CONFIG_URL).then((cached) => cached ?? fetch(request)))
    return
  }
  // the runtime config, manifest, icons: keep their names across deploys, so
  // the network wins and the cache only covers being offline
  event.respondWith(networkFirst(event, request))
})

async function activate() {
  const keys = await caches.keys()
  await Promise.all(keys.filter((k) => k.startsWith(PREFIX) && k !== SHELL && k !== CONFIG).map((k) => caches.delete(k)))
  await self.clients.claim()
}

function isPage(response) {
  return Boolean(response.headers.get('content-type')?.includes('text/html'))
}

// A proxy's page kept under a file's name would keep the app broken once the
// server is back, as nothing replaces a cached asset.
function keepable(response) {
  return response.ok && !response.redirected && !isPage(response)
}

// Every shell file, fresh from the server. Rejects unless all of them arrived
// and belong to this build: a redirect is a sign-in gateway, a page naming no
// asset a proxy's answer to every path, and one naming other assets a server
// already on another build, whose own worker brings it.
async function downloadShell() {
  const downloads = await Promise.all(
    files.map(async (f) => {
      const response = await fetch(new Request(f, { cache: 'reload' }))
      // "/" is the one file that is a page; its content is checked below
      if (!response.ok || response.redirected || (f !== SHELL_URL && isPage(response))) {
        throw new Error(`shell: ${f} ${response.status}`)
      }
      return [f, response]
    }),
  )
  const [, shell] = downloads.find(([f]) => f === SHELL_URL)
  const assets = (await shell.clone().text()).match(/\/assets\/[^"'\s>]+/g) ?? []
  if (assets.length === 0 || !assets.every((a) => files.includes(a))) {
    throw new Error('shell: not the page of this build')
  }
  return downloads
}

async function storeShell(downloads) {
  const cache = await caches.open(SHELL)
  await Promise.all(downloads.map(([f, response]) => cache.put(f, response)))
}

async function precache() {
  await storeShell(await downloadShell())
}

// "Clear cache and reload": download this build's shell again. Nothing is
// touched until every file has arrived, so a failed refresh (server
// unreachable) leaves the app still able to start offline. Other workers'
// caches are not ours to drop: a newer one may be installing right now.
async function refresh() {
  await storeShell(await downloadShell())
  await caches.delete(RUNTIME)
}

function keep(event, cacheName, key, response) {
  const copy = response.clone()
  event.waitUntil(caches.open(cacheName).then((cache) => cache.put(key, copy)))
}

async function cacheFirst(event) {
  const request = event.request
  const cached = await caches.match(request)
  if (cached) {
    return cached
  }
  const response = await fetch(request)
  if (keepable(response)) {
    keep(event, RUNTIME, request, response)
  }
  return response
}

// Undefined when the promise rejects or takes longer than the limit.
function within(promise, ms) {
  return new Promise((resolve) => {
    const timer = setTimeout(() => resolve(undefined), ms)
    promise.then(
      (value) => {
        clearTimeout(timer)
        resolve(value)
      },
      () => {
        clearTimeout(timer)
        resolve(undefined)
      },
    )
  })
}

async function networkFirst(event, cacheKey) {
  const request = event.request
  const navigation = request.mode === 'navigate'
  // no RequestInit: a navigation Request cannot be re-constructed with one,
  // and the server's Cache-Control: no-cache already forces revalidation
  // The whole body counts: headers followed by a stall would otherwise block
  // the parser on econumo-config.js just the same.
  const network = fetch(request).then(async (response) => {
    await response.clone().arrayBuffer()
    return response
  })
  const response = await within(network, NETWORK_TIMEOUT_MS)
  // A 5xx is a server that is down behind its proxy, which is being offline as
  // far as the app can tell. So is a 404 for anything cached: the server has
  // these files and answers client routes with the shell, so it is a proxy's,
  // and some say 404 for a stopped upstream.
  if (!response || response.status >= 500 || response.status === 404) {
    const cached = await caches.match(cacheKey)
    if (cached) {
      if (navigation) {
        cachedShellOpenedAt = Date.now()
      }
      return cached
    }
    return network
  }
  if (navigation) {
    cachedShellOpenedAt = -Infinity
  }
  // A late response is never kept: the page has moved on with the cached copy.
  const pathname = new URL(request.url).pathname
  if (!navigation && pathname !== CONFIG_URL && keepable(response)) {
    keep(event, files.includes(pathname) ? SHELL : RUNTIME, cacheKey, response)
  }
  return response
}
