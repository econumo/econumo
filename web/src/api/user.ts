import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CreatedPersonalTokenDto, CurrentUserDto, CurrentUserResponseDto, PersonalTokenDto, SessionDto, UserLoginItemDto } from './dto/user'

// login-user is the one endpoint that responds with a bare {token, user}
// body instead of the standard {success, message, data} envelope.
export async function login(username: string, password: string): Promise<UserLoginItemDto> {
  const response = await api.post<UserLoginItemDto>(apiUrl('/api/v1/user/login-user'), { username, password })
  return response.data
}

export async function logout(): Promise<void> {
  await api.post(apiUrl('/api/v1/user/logout-user'))
}

export async function register(email: string, password: string, name: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/register-user'), { email, password, name })
}

// update-name/currency/budget echo the refreshed user (incl. the synthetic currency_id option)
export async function updateName(name: string): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/update-name'), { name })
  return response.data.data.user
}

export async function updateAvatar(icon: string, color: string): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/update-avatar'), { icon, color })
  return response.data.data.user
}

export async function updatePassword(oldPassword: string, newPassword: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-password'), { oldPassword, newPassword })
}

export async function updateCurrency(currency: string): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/update-currency'), { currency })
  return response.data.data.user
}

export async function updateDefaultBudget(budgetId: Id): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/update-budget'), { value: budgetId })
  return response.data.data.user
}

export async function getUserData(): Promise<CurrentUserDto> {
  const response = await api.get<CurrentUserResponseDto>(apiUrl('/api/v1/user/get-user-data'))
  return response.data.data.user
}

export async function remindPassword(username: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/remind-password'), { username })
}

export async function resetPassword(username: string, code: string, password: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/reset-password'), { username, code, password })
}

export async function updateLanguage(language: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-language'), { language })
}

export async function completeOnboarding(): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/complete-onboarding'))
  return response.data.data.user
}

interface Envelope<T> {
  data: T
}

export async function getSessionList(): Promise<SessionDto[]> {
  const response = await api.get<Envelope<SessionDto[]>>(apiUrl('/api/v1/user/get-session-list'))
  return response.data.data
}

export async function revokeSession(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/user/revoke-session'), { id })
}

export async function revokeOtherSessions(): Promise<void> {
  await api.post(apiUrl('/api/v1/user/revoke-other-sessions'))
}

export async function getPersonalTokenList(): Promise<PersonalTokenDto[]> {
  const response = await api.get<Envelope<PersonalTokenDto[]>>(apiUrl('/api/v1/user/get-personal-token-list'))
  return response.data.data
}

// expiresAt: a "YYYY-MM-DD HH:mm:ss" datetime, or null for a token that never expires.
export async function createPersonalToken(name: string, expiresAt: string | null): Promise<CreatedPersonalTokenDto> {
  const response = await api.post<Envelope<CreatedPersonalTokenDto>>(
    apiUrl('/api/v1/user/create-personal-token'),
    { name, expiresAt: expiresAt ?? '' },
  )
  return response.data.data
}

export async function revokePersonalToken(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/user/revoke-personal-token'), { id })
}
