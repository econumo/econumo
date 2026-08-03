import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isNativeApp } from '@/lib/platform'
import { useServerConfig } from '@/lib/appConfig'
import { getVersion } from '@/lib/config'
import {
  getDismissedAppUpdateVersion,
  isNewerVersion,
  setDismissedAppUpdateVersion,
} from '@/lib/version'

// App-mode soft tier of the version handshake, app side: the server runs a
// newer Econumo than this app build but still accepts it — nudge toward a
// store update, dismissable once per server version.
export function AppUpdateNotice() {
  const { t } = useTranslation()
  const serverVersion = useServerConfig((s) => s.serverVersion)
  const [dismissed, setDismissed] = useState<string | null>(getDismissedAppUpdateVersion)
  if (!isNativeApp() || !serverVersion || dismissed === serverVersion) {
    return null
  }
  if (!isNewerVersion(serverVersion, getVersion())) {
    return null
  }
  return (
    <div className="flex items-center gap-2 bg-accent px-3 py-2 text-xs text-muted-foreground">
      <span className="min-w-0 flex-1">
        {t('common.appUpdate.notice', { version: serverVersion })}
      </span>
      <button
        type="button"
        aria-label={t('common.update.dismiss')}
        title={t('common.update.dismiss')}
        className="shrink-0 hover:text-foreground"
        onClick={() => {
          setDismissedAppUpdateVersion(serverVersion)
          setDismissed(serverVersion)
        }}
      >
        <X className="size-3.5" />
      </button>
    </div>
  )
}
