import { navigateTo } from '@/app/routerRef'
import { RouterPage } from '@/app/router-pages'
import { useOAuthInFlight } from '@/features/auth/oauthQueries'
import type { BrowserPlugin } from './externalLinks'
import { nativePlugin } from './platform'

interface AppUrlPlugin {
  addListener(ev: 'appUrlOpen', cb: (data: { url: string }) => void): unknown
}

// Two shapes reach the app (spec §6.3): the verified https app link the backend
// prefers, and the reverse-domain private scheme it falls back to when app links
// are not configured. The host of the https shape is whatever backend the user
// picked, so only the path identifies it.
function isAppReturn(url: URL): boolean {
  if (url.protocol === 'com.econumo.app:' && url.host === 'oauth') {
    return true
  }
  return url.protocol === 'https:' && url.pathname === RouterPage.OAUTH_APP_RETURN
}

// The browser sheet is closed first; the SPA route then does the same work as on
// the web. Anything else reaching the handler is ignored.
export function handleAppUrl(raw: string): void {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return
  }
  if (!isAppReturn(url)) {
    return
  }
  void nativePlugin<BrowserPlugin>('Browser')?.close().catch(() => {})
  // The deep link is itself proof the flow ended (success or failure) — clear
  // it explicitly here rather than relying on the sheet's `browserFinished`
  // event, which races this navigation and is not guaranteed to fire first.
  useOAuthInFlight.getState().set(false)
  // Handoffs ride in the fragment so they never reach a server log or a
  // Referer header; error codes are plain query parameters.
  const fragment = new URLSearchParams(url.hash.slice(1))
  const q = url.searchParams
  const handoff = fragment.get('handoff')
  const linkHandoff = fragment.get('linkHandoff')
  const linkError = q.get('linkError')
  const error = q.get('error')
  if (handoff) {
    navigateTo(`${RouterPage.OAUTH_CALLBACK}#handoff=${encodeURIComponent(handoff)}`)
  } else if (linkHandoff) {
    navigateTo(`${RouterPage.SETTINGS_LINKED_ACCOUNTS}#linkHandoff=${encodeURIComponent(linkHandoff)}`)
  } else if (linkError) {
    navigateTo(`${RouterPage.SETTINGS_LINKED_ACCOUNTS}?oauthError=${encodeURIComponent(linkError)}`)
  } else if (error) {
    navigateTo(`${RouterPage.LOGIN}?oauthError=${encodeURIComponent(error)}`)
  }
}

export function installDeepLinkHandler(): void {
  nativePlugin<AppUrlPlugin>('App')?.addListener('appUrlOpen', ({ url }) => handleAppUrl(url))
}
