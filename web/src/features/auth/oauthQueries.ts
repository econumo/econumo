import { useMutation, useQuery } from '@tanstack/react-query'
import * as oauthApi from '@/api/oauth'
import type { OAuthProviderId, ProviderDto } from '@/api/dto/oauth'
import { nativePlugin, isNativeApp } from '@/lib/platform'
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

// A top-level navigation on the web (Google and Apple require it); the system
// browser sheet in the app (embedded web views are blocked by Google).
export function openAuthorizationUrl(url: string): void {
  const browser = nativePlugin<BrowserPlugin>('Browser')
  if (browser) {
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
  return useMutation({
    mutationFn: ({ code, flow }: { code: string; flow: string }) => oauthApi.exchangeHandoff(code, flow),
    onSuccess: (data) => {
      // the new session may belong to a different user — never restore the
      // previous user's persisted finances
      clearPersistedQueryCache()
      setToken(data.token)
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
