import { api, apiUrl } from './client'
import { backendHost } from '@/lib/config'
import type { UserLoginItemDto } from './dto/user'
import type { IdentityDto, OAuthProviderId, ProviderDto } from './dto/oauth'
import { UserOptions } from './dto/user'
import { deriveAccessState } from '@/lib/access'
import { analyticsUserId } from '@/lib/analyticsId'
import { rememberAnalyticsPreference } from '@/lib/analyticsPreference'
import { setAnalyticsUser } from '@/lib/analytics'
import { setAnalyticsAccessState } from '@/lib/metrics'
import { rememberHasPassword, rememberLinkedProviders } from '@/lib/analyticsAuthMethods'

interface Envelope<T> {
  data: T
}

export type OAuthClient = 'web' | 'app'

export async function getProviderList(host: string = backendHost()): Promise<ProviderDto[]> {
  const response = await api.get<Envelope<ProviderDto[]>>(`${host}/api/v1/oauth/get-provider-list`)
  return response.data.data
}

// The flow secret binds the sign-in to this client: the callback carries no
// credential, so only whoever started the flow can redeem its handoff.
export interface StartOAuthResult {
  url: string
  flow: string
}

export async function startLogin(provider: OAuthProviderId, client: OAuthClient): Promise<StartOAuthResult> {
  const response = await api.post<Envelope<StartOAuthResult>>(apiUrl('/api/v1/oauth/start-login'), { provider, client })
  return response.data.data
}

export async function startLink(provider: OAuthProviderId, client: OAuthClient): Promise<StartOAuthResult> {
  const response = await api.post<Envelope<StartOAuthResult>>(apiUrl('/api/v1/oauth/start-link'), { provider, client })
  return response.data.data
}

// exchange-handoff answers with the bare {token, user} body like login-user,
// and primes the same analytics identity.
export async function exchangeHandoff(code: string, flow: string): Promise<UserLoginItemDto> {
  const response = await api.post<UserLoginItemDto>(apiUrl('/api/v1/oauth/exchange-handoff'), { code, flow })
  const { user } = response.data
  setAnalyticsAccessState(deriveAccessState(user.accessLevel, user.accessUntil))
  setAnalyticsUser(analyticsUserId(user.id))
  rememberHasPassword(user.hasPassword !== false)
  rememberAnalyticsPreference(user.options.find((o) => o.name === UserOptions.ANALYTICS)?.value !== '0')
  return response.data
}

// The link callback parks the resolved identity behind a one-shot code instead
// of writing it: only this call — authenticated, and carrying the flow secret
// of the client that started the link — completes it.
export async function completeLink(code: string, flow: string): Promise<{ provider: OAuthProviderId }> {
  const response = await api.post<Envelope<{ provider: OAuthProviderId }>>(apiUrl('/api/v1/oauth/complete-link'), { code, flow })
  return response.data.data
}

export async function getIdentityList(): Promise<IdentityDto[]> {
  const response = await api.get<Envelope<IdentityDto[]>>(apiUrl('/api/v1/oauth/get-identity-list'))
  const identities = response.data.data
  // The one place the linked set is known for certain; every caller (the
  // Settings page and the post-link/unlink refetches) funnels through here,
  // so the analytics flags refresh without each call site remembering to.
  rememberLinkedProviders(identities.map((identity) => identity.provider))
  return identities
}

// Refreshes the analytics auth-method flags for a user who may never open
// Settings. Deliberately NOT a react-query query: nothing on screen consumes
// it, and a query in the cache would drag the shell's sync indicator amber on
// failure. Failure is silent for the same reason — the flags simply stay as
// they were rather than the app reporting a problem the user cannot act on.
export function refreshAuthMethodFlags(): void {
  void getIdentityList().catch(() => {})
}

export async function unlinkIdentity(provider: OAuthProviderId): Promise<void> {
  await api.post(apiUrl('/api/v1/oauth/unlink-identity'), { provider })
}
