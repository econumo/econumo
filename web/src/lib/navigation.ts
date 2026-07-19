// In-app navigation trail, independent of the browser history stack (which can
// hold restored or redirect-chain entries the UI must never send the user back
// to). ApplicationLayout records each pathname; consumers read the previous one
// to decide where a "back" control may return.

let current: string | null = null
let previous: string | null = null

export function recordPathname(pathname: string): void {
  if (pathname === current) {
    return
  }
  previous = current
  current = pathname
}

export function previousPathname(): string | null {
  return previous
}

export function resetNavTracking(): void {
  current = null
  previous = null
}
