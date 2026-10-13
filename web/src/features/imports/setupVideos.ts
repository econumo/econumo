// One walkthrough per iOS version, newest first: the Shortcuts app looks
// different enough between versions that one recording cannot serve both.
// The videos live on YouTube, never in the binary (every self-hosted download
// would carry them), and the player is only loaded once the user opens one,
// so an unclicked page makes no request to YouTube. An empty list hides the
// link.
export interface SetupVideo {
  /** shown as is, e.g. "iOS 26" */
  label: string
  /** the YouTube link as shared: youtu.be/…, youtube.com/watch?v=… or /shorts/… */
  url: string
}

export const SETUP_VIDEOS: SetupVideo[] = []

const ID = /^[A-Za-z0-9_-]{11}$/

export function youtubeId(url: string): string | null {
  let u: URL
  try {
    u = new URL(url)
  } catch {
    return null
  }
  const host = u.hostname.replace(/^(www|m)\./, '')
  const path = u.pathname.split('/').filter(Boolean)
  const id =
    host === 'youtu.be' ? path[0]
      : host === 'youtube.com' && path[0] === 'watch' ? u.searchParams.get('v')
        : host === 'youtube.com' && (path[0] === 'shorts' || path[0] === 'embed' || path[0] === 'live') ? path[1]
          : null
  return id && ID.test(id) ? id : null
}

// youtube-nocookie keeps the player from setting cookies until play;
// playsinline stops iOS from jumping to fullscreen, so the dialog stays the
// frame around it.
export function embedUrl(id: string): string {
  return `https://www.youtube-nocookie.com/embed/${id}?playsinline=1&rel=0&modestbranding=1`
}
