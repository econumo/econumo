import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlert } from 'lucide-react'
import { isNativeApp } from '@/lib/platform'
import { fetchServerConfig, useServerConfig } from '@/lib/appConfig'
import { getVersion } from '@/lib/config'
import { isNewerVersion } from '@/lib/version'

// App-mode soft tier of the version handshake: the server is older than this
// app but still within the app's supported floor (an incompatible server is
// hard-blocked by AppUpdateBlock instead). Refetches on mount so a post-login
// server switch is picked up without an app restart.
export function ServerVersionNotice() {
  const { t } = useTranslation()
  const serverVersion = useServerConfig((s) => s.serverVersion)
  const native = isNativeApp()

  useEffect(() => {
    if (native) {
      void fetchServerConfig()
    }
  }, [native])

  if (!native || !serverVersion || !isNewerVersion(getVersion(), serverVersion)) {
    return null
  }
  return (
    <div className="flex items-center gap-2 bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
      <TriangleAlert className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1">
        {t('common.serverUpdate.notice', { version: serverVersion })}
      </span>
    </div>
  )
}
