import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { ImportCredentialKeyDto } from '@/api/dto/imports'
import { queryKeys } from '@/app/queryKeys'
import { loadStoredKey } from '@/lib/importCrypto'
import { useImportCredentialKey } from './queries'

export type ImportKeyState =
  | { status: 'loading' }
  | { status: 'none' }
  | { status: 'locked'; wrapped: ImportCredentialKeyDto }
  | { status: 'unlocked'; wrapped: ImportCredentialKeyDto }

// Server half: the passphrase-wrapped key (shared by every device). Local
// half: whether THIS device holds the unwrapped key in IndexedDB. Both are
// queries so a successful unlock/forget only has to invalidate the local one.
export function useImportKey(): { state: ImportKeyState; refresh: () => void } {
  const queryClient = useQueryClient()
  const remote = useImportCredentialKey()
  const local = useQuery({ queryKey: queryKeys.importLocalKey, queryFn: loadStoredKey, staleTime: Infinity, gcTime: 0 })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: queryKeys.importLocalKey })
  if (remote.isPending || local.isPending) {
    return { state: { status: 'loading' }, refresh }
  }
  if (!remote.data) {
    return { state: { status: 'none' }, refresh }
  }
  return { state: { status: local.data ? 'unlocked' : 'locked', wrapped: remote.data }, refresh }
}
