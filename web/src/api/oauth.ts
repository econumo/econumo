import { api, apiUrl } from './client'
import type { UserLoginItemDto } from './dto/user'
import type { IdentityDto, OAuthProviderId, ProviderDto } from './dto/oauth'
import { UserOptions } from './dto/user'
import { deriveAccessState } from '@/lib/access'
import { analyticsUserId } from '@/lib/analyticsId'
import { rememberAnalyticsPreference } from '@/lib/analyticsPreference'
import { setAnalyticsUser } from '@/lib/analytics'
import { setAnalyticsAccessState } from '@/lib/metrics'

interface Envelope<T> {
  data: T
}

export type OAuthClient = 'web' | 'app'

export async function getProviderList(): Promise<ProviderDto[]> {
  const response = await api.get<Envelope<ProviderDto[]>>(apiUrl('/api/v1/oauth/get-provider-list'))
  return response.data.data
}

export async function startLogin(provider: OAuthProviderId, client: OAuthClient): Promise<string> {
  const response = await api.post<Envelope<{ url: string }>>(apiUrl('/api/v1/oauth/start-login'), { provider, client })
  return response.data.data.url
}

export async function startLink(provider: OAuthProviderId, client: OAuthClient): Promise<string> {
  const response = await api.post<Envelope<{ url: string }>>(apiUrl('/api/v1/oauth/start-link'), { provider, client })
  return response.data.data.url
}

// exchange-handoff answers with the bare {token, user} body like login-user,
// and primes the same analytics identity.
export async function exchangeHandoff(code: string): Promise<UserLoginItemDto> {
  const response = await api.post<UserLoginItemDto>(apiUrl('/api/v1/oauth/exchange-handoff'), { code })
  const { user } = response.data
  setAnalyticsAccessState(deriveAccessState(user.accessLevel, user.accessUntil))
  setAnalyticsUser(analyticsUserId(user.id))
  rememberAnalyticsPreference(user.options.find((o) => o.name === UserOptions.ANALYTICS)?.value !== '0')
  return response.data
}

export async function getIdentityList(): Promise<IdentityDto[]> {
  const response = await api.get<Envelope<IdentityDto[]>>(apiUrl('/api/v1/oauth/get-identity-list'))
  return response.data.data
}

export async function unlinkIdentity(provider: OAuthProviderId): Promise<void> {
  await api.post(apiUrl('/api/v1/oauth/unlink-identity'), { provider })
}
