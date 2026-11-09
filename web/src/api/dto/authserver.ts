import type { Id } from '../types'

export interface AuthorizationRequestDto {
  clientId: string
  redirectUri: string
  responseType: string
  codeChallenge: string
  codeChallengeMethod: string
  resource: string
  scope: string
  state: string
}

export interface AuthorizationRequestResultDto {
  clientName: string
  redirectHost: string
  isLoopback: boolean
  isOfficial: boolean
  scope: 'mcp' | 'read'
  errorRedirectUrl: string
}

export interface AuthorizationDecisionDto {
  redirectUrl: string
}

export interface ConnectedAppDto {
  id: Id
  clientName: string
  redirectHost: string
  isLoopback: boolean
  isOfficial: boolean
  createdAt: string
  lastUsedAt: string
}
