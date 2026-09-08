import { useMutation, useQuery } from '@tanstack/react-query'
import * as oauthApi from '@/api/oauth'
import type { OAuthProviderId } from '@/api/dto/oauth'
import { nativePlugin, isNativeApp } from '@/lib/platform'
import type { BrowserPlugin } from '@/lib/externalLinks'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { setToken } from '@/lib/storage'
import { METRICS, trackEvent } from '@/lib/metrics'

export const providersQueryKey = ['oauth', 'providers'] as const

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

export function useStartOAuth() {
  return useMutation({
    mutationFn: async ({ provider, intent }: { provider: OAuthProviderId; intent: 'login' | 'link' }) => {
      const url = intent === 'link'
        ? await oauthApi.startLink(provider, oauthClient())
        : await oauthApi.startLogin(provider, oauthClient())
      openAuthorizationUrl(url)
    },
  })
}

export function useExchangeHandoff() {
  return useMutation({
    mutationFn: (code: string) => oauthApi.exchangeHandoff(code),
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
