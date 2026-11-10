import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { ImportCredentialKeyDto } from '@/api/dto/imports'
import { queryKeys } from '@/app/queryKeys'
import { loadStoredKey } from '@/lib/importCrypto'
import { useImportCredentialKey } from './queries'

export type ImportKeyState =
  | { status: 'loading' }
  | { status: 'none' }
  // stale: this device holds a key, but not the one the server's wrapped
  // form describes — the passphrase was reset on another device.
  | { status: 'locked'; wrapped: ImportCredentialKeyDto; stale: boolean }
  | { status: 'unlocked'; wrapped: ImportCredentialKeyDto }

// Server half: the passphrase-wrapped key (shared by every device). Local
// half: whether THIS device holds the unwrapped key in IndexedDB. Both are
// queries so a successful unlock/forget only has to invalidate the local one.
// remote is exposed so a page can tell, offline, a server key never loaded on
// this device (it waits for the network) from the local IndexedDB read, which
// is pending for a moment on every mount.
export function useImportKey(): { state: ImportKeyState; refresh: () => void; remote: ReturnType<typeof useImportCredentialKey> } {
  const queryClient = useQueryClient()
  const remote = useImportCredentialKey()
  const local = useQuery({ queryKey: queryKeys.importLocalKey, queryFn: loadStoredKey, staleTime: Infinity, gcTime: 0, networkMode: 'always' })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: queryKeys.importLocalKey })
  if (remote.isPending || local.isPending) {
    return { state: { status: 'loading' }, refresh, remote }
  }
  if (!remote.data) {
    return { state: { status: 'none' }, refresh, remote }
  }
  if (local.data?.tag === remote.data.wrappedDataKey) {
    return { state: { status: 'unlocked', wrapped: remote.data }, refresh, remote }
  }
  return { state: { status: 'locked', wrapped: remote.data, stale: local.data !== null }, refresh, remote }
}
