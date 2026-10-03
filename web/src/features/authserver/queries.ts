import { useMutation, useQuery } from '@tanstack/react-query'
import * as authserverApi from '@/api/authserver'
import type { AuthorizationRequestDto } from '@/api/dto/authserver'
import { METRICS, trackEvent } from '@/lib/metrics'
import { clearPostLoginRedirect } from './postLoginRedirect'

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
    mutationFn: (req: AuthorizationRequestDto) => authserverApi.approveAuthorization(req),
    onSuccess: ({ redirectUrl }) => {
      trackEvent(METRICS.CONNECTED_APP_APPROVE, {})
      clearPostLoginRedirect()
      window.location.assign(redirectUrl)
    },
  })
}

export function useDeclineAuthorization() {
  return useMutation({
    mutationFn: (req: AuthorizationRequestDto) => authserverApi.declineAuthorization(req),
    onSuccess: ({ redirectUrl }) => {
      clearPostLoginRedirect()
      window.location.assign(redirectUrl)
    },
  })
}
