export type OAuthProviderId = 'google' | 'apple' | 'oidc'

export interface ProviderDto {
  id: OAuthProviderId
  name: string
}

export interface IdentityDto {
  provider: OAuthProviderId
  email: string
  /** frozen wire format "YYYY-MM-DD HH:mm:ss" UTC */
  createdAt: string
}
