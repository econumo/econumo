import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as connectionApi from '@/api/connection'
import type { ConnectionDto } from '@/api/dto/connection'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

// poll matches Vue's 5s setInterval on the Connections page; other callers read the cache
export function useConnections(options: { poll?: boolean; enabled?: boolean } = {}) {
  return useQuery({
    queryKey: queryKeys.connections,
    queryFn: connectionApi.getConnectionList,
    staleTime: TEN_MINUTES,
    refetchInterval: options.poll ? 5_000 : undefined,
    enabled: options.enabled ?? true,
  })
}

export function useGenerateInvite() {
  return useMutation({
    mutationFn: connectionApi.generateInvite,
    onSuccess: () => trackEvent(METRICS.CONNECTION_GENERATE_INVITE),
  })
}

export function useAcceptInvite() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (code: string) => connectionApi.acceptInvite(code),
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.connections, items)
      trackEvent(METRICS.CONNECTION_ACCEPT_INVITE)
    },
  })
}

export function useDeleteConnection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (userId: Id) => connectionApi.deleteConnection(userId),
    onSuccess: (_data, userId) => {
      queryClient.setQueryData<ConnectionDto[]>(queryKeys.connections, (old) => old?.filter((c) => c.user.id !== userId))
      // the server revoked shared access both ways; refetch everything (Vue: syncStore.fetchAll)
      void queryClient.invalidateQueries()
      trackEvent(METRICS.CONNECTION_DELETE)
    },
  })
}

