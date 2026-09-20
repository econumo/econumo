import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { create } from 'zustand'
import * as oauthApi from '@/api/oauth'
import type { OAuthProviderId, ProviderDto } from '@/api/dto/oauth'
import { nativePlugin, isNativeApp } from '@/lib/platform'
import { backendHost } from '@/lib/config'
import type { BrowserPlugin } from '@/lib/externalLinks'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { setToken } from '@/lib/storage'
import { METRICS, trackEvent } from '@/lib/metrics'

export const providersQueryKey = ['oauth', 'providers'] as const

const FLOW_KEY = 'oauthFlow'

// The flow secret must outlive a full-page navigation to the provider and back.
// On the web sessionStorage scopes it to the tab that started the flow; the app
// leaves the WebView for the system browser sheet, which ends the session, so
// there it goes to localStorage.
function flowStore(): Storage {
  return isNativeApp() ? localStorage : sessionStorage
}

export function rememberOAuthFlow(flow: string): void {
  try {
    flowStore().setItem(FLOW_KEY, flow)
  } catch {
    // a storage-less browser fails the exchange instead, with the same error
  }
}

export function takeOAuthFlow(): string {
  useOAuthInFlight.getState().set(false)
  try {
    const store = flowStore()
    const flow = store.getItem(FLOW_KEY) ?? ''
    store.removeItem(FLOW_KEY)
    return flow
  } catch {
    return ''
  }
}

export function oauthClient(): oauthApi.OAuthClient {
  return isNativeApp() ? 'app' : 'web'
}

// On the web the flow returns to the BACKEND's origin (ECONUMO_URL), where a
// tab pointed at a different backend has no flow secret and cannot finish the
// exchange, so it must not offer to start one — for either the login buttons or
// Settings' "Link" buttons; the linked list itself stays visible and unlink
// keeps working, since neither needs a return trip. The app always leaves the
// WebView for the system browser sheet, which returns via a deep link
// regardless of origin, so it can always offer the flow. Origins (not raw
// strings) so a trailing slash on a same-origin host doesn't look off-origin,
// and a malformed stored host fails closed instead of throwing.
export function oauthFlowCanReturnHere(): boolean {
  if (isNativeApp()) {
    return true
  }
  try {
    return new URL(backendHost()).origin === window.location.origin
  } catch {
    return false
  }
}

// The app leaves the WebView for the browser sheet and the start mutation
// resolves as soon as the sheet is asked to open, so `isPending` alone lets a
// second tap mint a second flow whose secret overwrites the first's. Ended by
// whichever comes first: the sheet's own lifecycle (browserFinished), or the
// deep-link return — handleAppUrl (web/src/lib/deepLinks.ts) clears this
// explicitly for EVERY recognised app-return link, success or failure;
// a successful handoff/link additionally takes the flow secret via
// takeOAuthFlow, which also clears it (belt and suspenders with the deep-link
// path, since a web caller has no deep link at all).
export const useOAuthInFlight = create<{ inFlight: boolean; set: (v: boolean) => void }>((set) => ({
  inFlight: false,
  set: (inFlight) => set({ inFlight }),
}))

let browserFinishedInstalled = false

// A top-level navigation on the web (Google and Apple require it); the system
// browser sheet in the app (embedded web views are blocked by Google).
export function openAuthorizationUrl(url: string): void {
  const browser = nativePlugin<BrowserPlugin>('Browser')
  if (browser) {
    if (!browserFinishedInstalled && browser.addListener) {
      browserFinishedInstalled = true
      browser.addListener('browserFinished', () => useOAuthInFlight.getState().set(false))
    }
    useOAuthInFlight.getState().set(true)
    void browser.open({ url })
    return
  }
  window.location.assign(url)
}

export function useProviders() {
  return useQuery({
    queryKey: providersQueryKey,
    queryFn: oauthApi.getProviderList,
    staleTime: Infinity,
  })
}

// The catalogue's "SSO" is a placeholder for the unnamed custom slot — once the
// deployment's provider list carries a configured name (e.g. "Authentik"), that
// takes precedence everywhere the provider is shown to the user.
export function providerDisplayName(id: string, providers: ProviderDto[] | undefined, t: (key: string) => string): string {
  const configured = providers?.find((p) => p.id === id)?.name
  if (configured) {
    return configured
  }
  const key = `auth.oauth.provider_name.${id}`
  const translated = t(key)
  return translated === key ? id : translated
}

export function useStartOAuth() {
  return useMutation({
    mutationFn: async ({ provider, intent }: { provider: OAuthProviderId; intent: 'login' | 'link' }) => {
      const { url, flow } = intent === 'link'
        ? await oauthApi.startLink(provider, oauthClient())
        : await oauthApi.startLogin(provider, oauthClient())
      rememberOAuthFlow(flow)
      openAuthorizationUrl(url)
    },
  })
}

export function useExchangeHandoff() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ code, flow }: { code: string; flow: string }) => oauthApi.exchangeHandoff(code, flow),
    onSuccess: async (data) => {
      // The new session may belong to a different user, and the app can reach
      // this without a page reload (the browser sheet returns to the same SPA
      // instance). Dropping only the persisted snapshot would leave the
      // previous user's profile and accounts live in memory, still inside
      // their staleTime — so cancel what is in flight and empty the cache too.
      await queryClient.cancelQueries()
      clearPersistedQueryCache()
      queryClient.clear()
      setToken(data.token)
      // After the token is stored, for the same reason as the password login
      // (see useLogin): an unawaited probe must not 401 its way into a logout.
      oauthApi.refreshAuthMethodFlags()
      trackEvent(METRICS.OAUTH_LOGIN_COMPLETED)
      if (isFreshAccount(data.user.createdAt)) {
        trackEvent(METRICS.OAUTH_ACCOUNT_CREATED)
      }
    },
  })
}

// The exchange response does not say whether the callback provisioned the
// account; a createdAt within the last two minutes is the proxy (the handoff
// itself lives sixty seconds). createdAt is the frozen UTC "YYYY-MM-DD HH:mm:ss".
export function isFreshAccount(createdAt: string, now: Date = new Date()): boolean {
  if (typeof createdAt !== 'string') {
    return false
  }
  const created = Date.parse(createdAt.replace(' ', 'T') + 'Z')
  return Number.isFinite(created) && now.getTime() - created < 2 * 60 * 1000
}
