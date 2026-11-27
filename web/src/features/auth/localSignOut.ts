import { clearPendingWrites } from '@/features/pendingWrites/store'
import { resetAnalyticsIdentity } from '@/lib/analytics'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { hasToken, removeToken } from '@/lib/storage'
import { clearConfig } from '@/features/appLock/config'

export async function signOutLocally(opts?: { keepPendingWrites?: boolean; keepAppLock?: boolean }): Promise<void> {
  // A sign-out without a session has no identity left to end: the sign-out or
  // the 401 that removed the token already reset it.
  if (hasToken()) {
    resetAnalyticsIdentity()
  }
  removeToken()
  if (!opts?.keepAppLock) {
    clearConfig()
  }
  await Promise.all([clearPersistedQueryCache(), opts?.keepPendingWrites ? undefined : clearPendingWrites()])
}
