import { api, apiUrl } from './client'
import type { Id } from './types'
import type {
  AuthorizationDecisionDto,
  AuthorizationRequestDto,
  AuthorizationRequestResultDto,
  ConnectedAppDto,
} from './dto/authserver'

interface Envelope<T> {
  success: boolean
  message: string
  data: T
}

// The OAuth query string uses snake_case; the API takes the same fields in camelCase.
export function authorizationRequestFromSearch(search: URLSearchParams): AuthorizationRequestDto {
  const get = (key: string) => search.get(key) ?? ''
  return {
    clientId: get('client_id'),
    redirectUri: get('redirect_uri'),
    responseType: get('response_type'),
    codeChallenge: get('code_challenge'),
    codeChallengeMethod: get('code_challenge_method'),
    resource: get('resource'),
    scope: get('scope'),
    state: get('state'),
  }
}

export async function getAuthorizationRequest(req: AuthorizationRequestDto): Promise<AuthorizationRequestResultDto> {
  const response = await api.get<Envelope<AuthorizationRequestResultDto>>(
    apiUrl('/api/v1/authserver/get-authorization-request'),
    { params: req },
  )
  return response.data.data
}

export async function approveAuthorization(req: AuthorizationRequestDto): Promise<AuthorizationDecisionDto> {
  const response = await api.post<Envelope<AuthorizationDecisionDto>>(
    apiUrl('/api/v1/authserver/approve-authorization'),
    req,
  )
  return response.data.data
}

export async function declineAuthorization(req: AuthorizationRequestDto): Promise<AuthorizationDecisionDto> {
  const response = await api.post<Envelope<AuthorizationDecisionDto>>(
    apiUrl('/api/v1/authserver/decline-authorization'),
    req,
  )
  return response.data.data
}

export async function getConnectedApps(): Promise<ConnectedAppDto[]> {
  const response = await api.get<Envelope<ConnectedAppDto[]>>(apiUrl('/api/v1/authserver/get-connected-app-list'))
  return response.data.data
}

export async function revokeConnectedApp(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/authserver/revoke-connected-app'), { id })
}
