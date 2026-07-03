import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CurrentUserDto, CurrentUserResponseDto, UserLoginItemDto, UserLoginResponseDto } from './dto/user'

export async function login(username: string, password: string): Promise<UserLoginItemDto> {
  const response = await api.post<UserLoginResponseDto>(apiUrl('/api/v1/user/login-user'), { username, password })
  return response.data.data
}

export async function logout(): Promise<void> {
  await api.post(apiUrl('/api/v1/user/logout-user'))
}

export async function register(email: string, password: string, name: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/register-user'), { email, password, name })
}

export async function updateName(name: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-name'), { name })
}

export async function updatePassword(oldPassword: string, newPassword: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-password'), { oldPassword, newPassword })
}

export async function updateCurrency(currency: string): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-currency'), { currency })
}

export async function updateDefaultBudget(budgetId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/user/update-budget'), { value: budgetId })
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

export async function completeOnboarding(): Promise<void> {
  await api.post(apiUrl('/api/v1/user/complete-onboarding'))
}
