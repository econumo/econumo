import { UserOptions } from '@/api/dto/user'
import { useUserData } from '@/features/user/queries'
import { getVersion } from '@/lib/config'
import { isNewerVersion, SEMVER } from '@/lib/version'

const RELEASE_NOTES_URL = 'https://econumo.com/releases/'

export interface WhatsNew {
  version: string
  url: string
}

// The running release when the user has not seen its notes yet; null for a
// non-release build (dev, a custom label) or when they already have. A user
// with no stored version predates the option, so they did upgrade: new
// accounts get the running version seeded server-side and never see this.
export function useWhatsNew(): WhatsNew | null {
  const { data: user } = useUserData()
  const version = getVersion()
  if (!user || !SEMVER.test(version)) {
    return null
  }
  const seen = user.options.find((o) => o.name === UserOptions.LAST_SEEN_VERSION)?.value
  if (seen && SEMVER.test(seen) && !isNewerVersion(version, seen)) {
    return null
  }
  return { version, url: RELEASE_NOTES_URL + version }
}
