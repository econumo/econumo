import { api, apiUrl } from './client'
import type { UpdateInfoDto } from './dto/system'

interface Envelope<T> {
  data: T
}

// The update check is decorative and its failure must be silence (same
// contract as the backend poller): a rejection would mark the shared query
// cache as sync-failing (amber icon) and re-run retry cycles on every
// invalidate — observed live behind a reverse proxy that 403s this path.
export async function getUpdateInfo(): Promise<UpdateInfoDto | null> {
  try {
    const response = await api.get<Envelope<UpdateInfoDto>>(apiUrl('/api/v1/system/get-update-info'))
    return response.data.data ?? null
  } catch {
    return null
  }
}
