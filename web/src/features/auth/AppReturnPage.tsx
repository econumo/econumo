import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router'
import { Button } from '@/components/ui/button'
import { isNativeApp } from '@/lib/platform'

const APP_SCHEME = 'com.econumo.app://oauth'

function schemeURL(search: string, hash: string): string {
  const fragment = new URLSearchParams(hash.replace(/^#/, ''))
  const query = new URLSearchParams(search.replace(/^\?/, ''))
  // Each parameter keeps the position the backend gave it: handoffs in the
  // fragment (never logged, never a Referer), error codes in the query.
  for (const key of ['handoff', 'linkHandoff'] as const) {
    const value = fragment.get(key)
    if (value) {
      return `${APP_SCHEME}#${key}=${encodeURIComponent(value)}`
    }
  }
  for (const key of ['error', 'linkError'] as const) {
    const value = query.get(key)
    if (value) {
      return `${APP_SCHEME}?${key}=${encodeURIComponent(value)}`
    }
  }
  return APP_SCHEME
}

// Only the https app link lands here (spec §6.3), and only in a browser that
// could not hand it to the app: not installed, or the association unverified.
// The private scheme is the one lever left, and opening it needs a user gesture.
export function AppReturnPage() {
  const { t } = useTranslation()
  const { search, hash } = useLocation()
  // In the app the deep-link handler already dispatched this URL; showing a
  // button that re-opens the app on top of itself would only confuse.
  if (isNativeApp()) {
    return null
  }
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-lg font-medium">{t('auth.oauth.app_return.title')}</h1>
      <Button asChild>
        <a href={schemeURL(search, hash)}>{t('auth.oauth.app_return.open_app')}</a>
      </Button>
    </div>
  )
}
