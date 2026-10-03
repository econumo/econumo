import { useMutation, useQuery } from '@tanstack/react-query'
import * as authserverApi from '@/api/authserver'
import type { AuthorizationRequestDto } from '@/api/dto/authserver'
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
