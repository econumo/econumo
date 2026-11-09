import { UserOptions } from '@/api/dto/user'
import { isOnboardingCompleted, useUserData } from '@/features/user/queries'
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

// The card announces major and minor releases only, so it names the .0 of the
// running line, where the big changes are written up (v1.6.x -> v1.7.2 shows
// v1.7.0).
function lineRelease(version: string): string {
  const [, major, minor] = SEMVER.exec(version)!
  return `v${major}.${minor}.0`
}

// The running release when the user has not seen its major or minor line yet;
// null for a non-release build (dev, a custom label), after a patch upgrade
// within a seen line, or when they already have. A user with no stored version
// predates the option, so they did upgrade: new accounts get the running
// version seeded server-side and never see this.
// Web only: the app compares its bundled version, not the server's that the
// seed uses, and may talk to a server without update-last-seen-version.
export function useWhatsNew(): WhatsNew | null {
  const { data: user } = useUserData()
  const version = getVersion()
  if (!user || isNativeApp() || !SEMVER.test(version)) {
    return null
  }
  // Not during onboarding: the first session is busy enough. The card waits
  // until it is completed, since the version is stored only on open/dismiss.
  if (!isOnboardingCompleted(user)) {
    return null
  }
  const seen = user.options.find((o) => o.name === UserOptions.LAST_SEEN_VERSION)?.value
  const notes = lineRelease(version)
  if (seen && SEMVER.test(seen) && !isNewerVersion(notes, seen)) {
    return null
  }
  return { version, notesVersion: notes, url: RELEASE_NOTES_URL + notes }
}
