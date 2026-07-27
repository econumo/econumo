import { useQuery } from '@tanstack/react-query'
import { getUpdateInfo } from '@/api/system'
import { ONE_DAY, queryKeys } from '@/app/queryKeys'
import { getVersion } from '@/lib/config'
import { isNewerVersion } from '@/lib/version'

export interface AvailableUpdate {
  version: string
  url: string
}

// The latest published release when it is strictly newer than this build;
// null otherwise (current, dev build, check disabled, or feed unavailable).
export function useAvailableUpdate(): AvailableUpdate | null {
  const { data } = useQuery({
    queryKey: queryKeys.updateInfo,
    queryFn: getUpdateInfo,
    staleTime: ONE_DAY,
    refetchOnWindowFocus: false,
  })
  if (!data || !isNewerVersion(data.version, getVersion())) {
    return null
  }
  return { version: data.version, url: data.url }
}
