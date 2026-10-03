import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as authserverApi from '@/api/authserver'
import type { AuthorizationRequestDto } from '@/api/dto/authserver'
import type { Id } from '@/api/types'
import { METRICS, trackEvent } from '@/lib/metrics'
import { clearPostLoginRedirect } from './postLoginRedirect'

export class InvalidRedirectError extends Error {}

// The redirect comes from the server's own checks, but it ends up in
// location.assign, so a javascript:/data: URL must never get there.
export function isSafeRedirectUrl(url: string): boolean {
  try {
    const { protocol } = new URL(url)
    return protocol === 'https:' || protocol === 'http:'
  } catch {
    return false
  }
}

async function safeDecision(decision: Promise<{ redirectUrl: string }>) {
  const result = await decision
  if (!isSafeRedirectUrl(result.redirectUrl)) {
    throw new InvalidRedirectError()
  }
  return result
}

export function useAuthorizationRequest(req: AuthorizationRequestDto) {
  return useQuery({
    queryKey: ['authserver', 'authorization-request', req],
    queryFn: () => authserverApi.getAuthorizationRequest(req),
    retry: false,
    refetchOnWindowFocus: false,
  })
}

export function useApproveAuthorization() {
  return useMutation({
    mutationFn: (req: AuthorizationRequestDto) => safeDecision(authserverApi.approveAuthorization(req)),
    onSuccess: ({ redirectUrl }) => {
      trackEvent(METRICS.CONNECTED_APP_APPROVE, {})
      clearPostLoginRedirect()
      window.location.assign(redirectUrl)
    },
  })
}

export function useDeclineAuthorization() {
  return useMutation({
    mutationFn: (req: AuthorizationRequestDto) => safeDecision(authserverApi.declineAuthorization(req)),
    onSuccess: ({ redirectUrl }) => {
      clearPostLoginRedirect()
      window.location.assign(redirectUrl)
    },
  })
}

// Under the 'authserver' root so queryPersist keeps it out of the stored cache
// along with the rest of this feature.
const connectedAppsKey = ['authserver', 'connected-apps'] as const

export function useConnectedApps() {
  return useQuery({ queryKey: connectedAppsKey, queryFn: authserverApi.getConnectedApps })
}

export function useRevokeConnectedApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => authserverApi.revokeConnectedApp(id),
    onSuccess: () => {
      trackEvent(METRICS.CONNECTED_APP_REVOKE, {})
      return queryClient.invalidateQueries({ queryKey: connectedAppsKey })
    },
  })
}
