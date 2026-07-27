export const SEMVER = /^v(\d+)\.(\d+)\.(\d+)$/

// True only when BOTH strings are strict vX.Y.Z and latest > current — so a
// `dev` image, an empty feed, or a custom build tag can never trigger the
// update surfaces.
export function isNewerVersion(latest: string, current: string): boolean {
  const l = SEMVER.exec(latest)
  const c = SEMVER.exec(current)
  if (!l || !c) {
    return false
  }
  for (let i = 1; i <= 3; i++) {
    const d = Number(l[i]) - Number(c[i])
    if (d !== 0) {
      return d > 0
    }
  }
  return false
}

const DISMISSED_KEY = 'econumo.dismissed-update-version'

export function getDismissedUpdateVersion(): string | null {
  return localStorage.getItem(DISMISSED_KEY)
}

export function setDismissedUpdateVersion(version: string): void {
  localStorage.setItem(DISMISSED_KEY, version)
}
