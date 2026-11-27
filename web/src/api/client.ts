import axios, { type AxiosRequestConfig } from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { toast } from 'sonner'
import { getToken, removeToken } from '@/lib/storage'
import { backendHost, locale } from '@/lib/config'
import i18n from '@/app/i18n'
import { queryClient } from '@/app/queryClient'
import { queryKeys } from '@/app/queryKeys'
import { navigateTo } from '@/app/routerRef'
import { resetAnalyticsIdentity } from '@/lib/analytics'

export const api = axios.create()

export const DELETION_CLEANUP_KEY = 'econumo.deletionCleanup'
// A deletion code lives 10 minutes, so a mark cannot commit later than that
// after its confirm was sent.
const DELETION_CLEANUP_WINDOW_MS = 10 * 60_000

let deletionCleanup: Promise<void> | null = null

// True when a deletion confirm was sent recently enough that this 401 can be
// its committed mark; an older flag is dropped so an ordinary expiry months
// later is not mistaken for a deletion.
function deletionConfirmPending(): boolean {
  try {
    const raw = localStorage.getItem(DELETION_CLEANUP_KEY)
    if (raw === null) {
      return false
    }
    const sentAt = Number(raw)
    if (Number.isFinite(sentAt) && sentAt > 0 && Date.now() - sentAt < DELETION_CLEANUP_WINDOW_MS) {
      return true
    }
    localStorage.removeItem(DELETION_CLEANUP_KEY)
  } catch {
    // storage unavailable: no flag can have been written either
  }
  return false
}

function finishDeletionCleanup(): void {
  deletionCleanup ??= (async () => {
    try {
      // loaded on demand: the sign-out reaches modules that import this client
      const { signOutLocally } = await import('@/features/auth/localSignOut')
      await signOutLocally()
      localStorage.removeItem(DELETION_CLEANUP_KEY)
    } catch {
      // the flag stays for a later 401 to try again
    }
    window.location.assign('/login?reason=deletion-scheduled')
  })().finally(() => {
    deletionCleanup = null
  })
}

export interface WriteOptions {
  /** The same key on every attempt: the queue's retries must read as one operation. */
  idempotencyKey?: string
  timeout?: number
}

export function writeConfig(opts?: WriteOptions): AxiosRequestConfig {
  return {
    timeout: opts?.timeout,
    headers: opts?.idempotencyKey ? { 'Idempotency-Key': opts.idempotencyKey } : undefined,
  }
}

api.interceptors.request.use((config) => {
  config.headers.Accept = 'application/json'
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  config.headers['Accept-Language'] = locale()
  config.headers['X-Timezone'] = Intl.DateTimeFormat().resolvedOptions().timeZone
  if (config.method === 'post' && !config.headers.has('Idempotency-Key')) {
    config.headers.set('Idempotency-Key', uuidv7())
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error.response?.status
    const url: string = error.config?.url ?? ''
    // login-user/exchange-handoff report a 401 as "wrong credentials", not "session expired";
    // logout-user's 401 means the session was already gone before the request — LogoutPage
    // owns the cleanup (removeToken/clearPersistedQueryCache) and the exit itself either way.
    // get-identity-list is a best-effort analytics probe fired without awaiting: it must
    // never be the request that logs someone out, whatever races it.
    const ownsIts401 = url.includes('/api/v1/user/login-user') || url.includes('/api/v1/oauth/exchange-handoff')
      || url.includes('/api/v1/user/logout-user') || url.includes('/api/v1/oauth/get-identity-list')
    if (status === 401 && !ownsIts401 && deletionConfirmPending()) {
      finishDeletionCleanup()
    } else if (status === 401 && !ownsIts401) {
      removeToken()
      // No page reload happens here (client-side navigation only), so the
      // analytics module state would otherwise survive and misattribute the
      // next person's login/register events on a shared browser.
      resetAnalyticsIdentity()
      navigateTo('/login?reason=expired')
    }
    if (status === 402) {
      // The 402 envelope message is deliberately product-neutral and carries
      // no messageCode — render our own localized copy. Fixed id: repeated
      // 402s must not stack toasts.
      toast.error(i18n.t('subscription.toast.readonly'), { id: 'subscription-readonly' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.user })
    }
    return Promise.reject(error)
  },
)

export function apiUrl(path: string): string {
  return `${backendHost()}${path}`
}
