import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as userApi from '@/api/user'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useSessions() {
  return useQuery({ queryKey: queryKeys.sessions, queryFn: userApi.getSessionList })
}

export function useRevokeSession() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => userApi.revokeSession(id),
    onSuccess: () => {
      trackEvent(METRICS.SESSION_REVOKE)
      return queryClient.invalidateQueries({ queryKey: queryKeys.sessions })
    },
  })
}

export function useRevokeOtherSessions() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => userApi.revokeOtherSessions(),
    onSuccess: () => {
      trackEvent(METRICS.SESSION_REVOKE_OTHERS)
      return queryClient.invalidateQueries({ queryKey: queryKeys.sessions })
    },
  })
}

export function usePersonalTokens() {
  return useQuery({ queryKey: queryKeys.personalTokens, queryFn: userApi.getPersonalTokenList })
}

export function useCreatePersonalToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, expiresAt }: { name: string; expiresAt: string | null }) =>
      userApi.createPersonalToken(name, expiresAt),
    onSuccess: () => {
      trackEvent(METRICS.PERSONAL_TOKEN_CREATE)
      return queryClient.invalidateQueries({ queryKey: queryKeys.personalTokens })
    },
  })
}

export function useRevokePersonalToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => userApi.revokePersonalToken(id),
    onSuccess: () => {
      trackEvent(METRICS.PERSONAL_TOKEN_REVOKE)
      return queryClient.invalidateQueries({ queryKey: queryKeys.personalTokens })
    },
  })
}
