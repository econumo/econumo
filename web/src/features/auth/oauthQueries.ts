import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as oauthApi from '@/api/oauth'
import type { OAuthProviderId, ProviderDto } from '@/api/dto/oauth'
import { backendHost, isPasswordLoginAllowed } from '@/lib/config'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { setToken } from '@/lib/storage'
import { METRICS, trackEvent } from '@/lib/metrics'

export const PROVIDERS_QUERY_KEY = ['oauth', 'providers'] as const

const FLOW_KEY = 'oauthFlow'

// The flow secret must outlive a full-page navigation to the provider and back;
// sessionStorage scopes it to the tab that started the flow.
function flowStore(): Storage {
  return sessionStorage
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

// The flow returns to the BACKEND's origin (ECONUMO_URL), where a tab pointed
// at a different backend has no flow secret and cannot finish the exchange, so
// it must not offer to start one — for either the login buttons or Settings'
// "Link" buttons; the linked list itself stays visible and unlink keeps
// working, since neither needs a return trip. Origins (not raw strings) so a
// trailing slash on a same-origin host doesn't look off-origin, and a malformed
// stored host fails closed instead of throwing.
export function oauthFlowCanReturnHere(): boolean {
  try {
    return new URL(backendHost()).origin === window.location.origin
  } catch {
    return false
  }
}

// PASSWORD_LOGIN describes the instance serving this page. A custom backend on
// another origin cannot take the provider round trip back here, so there the
// password form stays and that backend decides whether it accepts passwords.
export function passwordLoginAvailable(): boolean {
  return isPasswordLoginAllowed() || !oauthFlowCanReturnHere()
}

export function usePasswordLoginAvailable(): boolean {
  return passwordLoginAvailable()
}

// A top-level navigation (Google and Apple require it).
export function openAuthorizationUrl(url: string): void {
  window.location.assign(url)
}

export function useProviders() {
  return useQuery({
    queryKey: PROVIDERS_QUERY_KEY,
    queryFn: () => oauthApi.getProviderList(),
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
        ? await oauthApi.startLink(provider)
        : await oauthApi.startLogin(provider)
      rememberOAuthFlow(flow)
      openAuthorizationUrl(url)
    },
  })
}

export function useExchangeHandoff() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: ({ code, flow }: { code: string; flow: string }) => oauthApi.exchangeHandoff(code, flow),
    onSuccess: async (data) => {
      // The new session may belong to a different user. Dropping only the
      // persisted snapshot would leave the previous user's profile and
      // accounts live in memory, still inside their staleTime — so cancel what
      // is in flight and empty the cache too.
      await queryClient.cancelQueries()
      await clearPersistedQueryCache()
      queryClient.clear()
      setToken(data.token, data.user.id)
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
