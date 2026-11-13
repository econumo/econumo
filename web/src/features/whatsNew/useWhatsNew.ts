import { UserOptions } from '@/api/dto/user'
import { isOnboardingCompleted, useUserData } from '@/features/user/queries'
import { getVersion } from '@/lib/config'
import { isNewerVersion, SEMVER } from '@/lib/version'

const RELEASE_NOTES_URL = 'https://econumo.com/releases/'

export interface WhatsNew {
  // The running release, stored as seen on open/dismiss.
  version: string
  // What the card names, e.g. v1.7.
  line: string
  // The release whose notes the card links to, e.g. v1.7.0.
  notesVersion: string
  url: string
}

// The running release when the user has not seen its major or minor line yet;
// null for a non-release build (dev, a custom label), after a patch upgrade
// within a seen line, or when they already have. A user with no stored version
// predates the option, so they did upgrade: new accounts get the running
// version seeded server-side and never see this.
export function useWhatsNew(): WhatsNew | null {
  const { data: user } = useUserData()
  const version = getVersion()
  if (!user || !SEMVER.test(version)) {
    return null
  }
  // Not during onboarding: the first session is busy enough. The card waits
  // until it is completed, since the version is stored only on open/dismiss.
  if (!isOnboardingCompleted(user)) {
    return null
  }
  const seen = user.options.find((o) => o.name === UserOptions.LAST_SEEN_VERSION)?.value
  // The card announces major and minor releases only: it names the running
  // line and links its .0 notes, where the big changes are written up
  // (v1.6.x -> v1.7.2 shows v1.7 and links v1.7.0).
  const [, major, minor] = SEMVER.exec(version)!
  const line = `v${major}.${minor}`
  const notes = `${line}.0`
  if (seen && SEMVER.test(seen) && !isNewerVersion(notes, seen)) {
    return null
  }
  return { version, line, notesVersion: notes, url: RELEASE_NOTES_URL + notes }
}
