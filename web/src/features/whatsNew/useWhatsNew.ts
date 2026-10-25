import { UserOptions } from '@/api/dto/user'
import { useUserData } from '@/features/user/queries'
import { getVersion } from '@/lib/config'
import { isNativeApp } from '@/lib/platform'
import { isNewerVersion, SEMVER } from '@/lib/version'

const RELEASE_NOTES_URL = 'https://econumo.com/releases/'

export interface WhatsNew {
  // The running release, stored as seen on open/dismiss.
  version: string
  // The release whose notes the card links to (see notesVersion).
  notesVersion: string
  url: string
}

// An upgrade into a new minor line links to its .0 notes, where the big
// changes are announced (v1.6.x -> v1.7.2 shows v1.7.0); a patch upgrade
// within the line links to the running patch.
function notesVersion(version: string, seen: string | null | undefined): string {
  const cur = SEMVER.exec(version)!
  const prev = seen ? SEMVER.exec(seen) : null
  if (prev && prev[1] === cur[1] && prev[2] === cur[2]) {
    return version
  }
  return `v${cur[1]}.${cur[2]}.0`
}

// The running release when the user has not seen its notes yet; null for a
// non-release build (dev, a custom label) or when they already have. A user
// with no stored version predates the option, so they did upgrade: new
// accounts get the running version seeded server-side and never see this.
// Web only: the app compares its bundled version, not the server's that the
// seed uses, and may talk to a server without update-last-seen-version.
export function useWhatsNew(): WhatsNew | null {
  const { data: user } = useUserData()
  const version = getVersion()
  if (!user || isNativeApp() || !SEMVER.test(version)) {
    return null
  }
  const seen = user.options.find((o) => o.name === UserOptions.LAST_SEEN_VERSION)?.value
  if (seen && SEMVER.test(seen) && !isNewerVersion(version, seen)) {
    return null
  }
  const notes = notesVersion(version, seen)
  return { version, notesVersion: notes, url: RELEASE_NOTES_URL + notes }
}
