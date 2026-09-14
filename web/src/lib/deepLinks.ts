import { navigateTo } from '@/app/routerRef'
import { RouterPage } from '@/app/router-pages'
import { useOAuthInFlight } from '@/features/auth/oauthQueries'
import type { BrowserPlugin } from './externalLinks'
import { nativePlugin } from './platform'

interface AppUrlPlugin {
  addListener(ev: 'appUrlOpen', cb: (data: { url: string }) => void): unknown
}

// The backend redirects app flows to econumo://oauth?… (spec §6.3). The
// browser sheet is closed first; the SPA route then does the same work as on
// the web. Anything else on the scheme is ignored.
export function handleAppUrl(raw: string): void {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return
  }
  if (url.protocol !== 'econumo:' || url.host !== 'oauth') {
    return
  }
  void nativePlugin<BrowserPlugin>('Browser')?.close().catch(() => {})
  // The deep link is itself proof the flow ended (success or failure) — clear
  // it explicitly here rather than relying on the sheet's `browserFinished`
  // event, which races this navigation and is not guaranteed to fire first.
  useOAuthInFlight.getState().set(false)
  const q = url.searchParams
  const handoff = q.get('handoff')
  const linkHandoff = q.get('linkHandoff')
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
