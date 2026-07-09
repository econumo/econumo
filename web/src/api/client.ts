import axios from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { getToken, removeToken } from '@/lib/storage'
import { backendHost, locale } from '@/lib/config'

export const api = axios.create()

api.interceptors.request.use((config) => {
  config.headers.Accept = 'application/json'
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  config.headers['Accept-Language'] = locale()
  config.headers['X-Timezone'] = Intl.DateTimeFormat().resolvedOptions().timeZone
  config.headers['X-Request-Id'] = uuidv7()
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error.response?.status
    const url: string = error.config?.url ?? ''
    if (status === 401 && !url.includes('/api/v1/user/login-user')) {
      removeToken()
      window.location.assign('/login?reason=expired')
    }
    return Promise.reject(error)
  },
)

export function apiUrl(path: string): string {
  return `${backendHost()}${path}`
}
