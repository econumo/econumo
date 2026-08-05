import { useTranslation } from 'react-i18next'
import { TriangleAlert } from 'lucide-react'
import { isNativeApp } from '@/lib/platform'
import { MIN_SERVER_VERSION, useServerConfig } from '@/lib/appConfig'
import { getVersion } from '@/lib/config'
import { isNewerVersion } from '@/lib/version'

// App-mode hard tier of the version handshake. Each side owns a floor: the
// server publishes MIN_APP_VERSION (the oldest app build it accepts) in its
// config, and the app carries MIN_SERVER_VERSION (the oldest server it can
// talk to). Crossing either floor means the pair is incompatible, so the app
// blocks itself until one side is updated; merely-outdated-but-compatible
// pairs get the dismissable layout banners instead. Servers predating the
// MIN_APP_VERSION key never block the app — the app-side floor governs them.
export function AppUpdateBlock() {
  const { t } = useTranslation()
  const { serverVersion, minAppVersion } = useServerConfig()
  if (!isNativeApp()) {
    return null
  }
  const appVersion = getVersion()
  const appTooOld = minAppVersion !== null && isNewerVersion(minAppVersion, appVersion)
  const serverTooOld = serverVersion !== null && isNewerVersion(MIN_SERVER_VERSION, serverVersion)
  if (!appTooOld && !serverTooOld) {
    return null
  }
  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-background p-6">
      <div className="flex max-w-sm flex-col items-center gap-3 text-center">
        <TriangleAlert className="size-8 text-amber-500" />
        <h1 className="text-lg font-semibold">{t('common.appUpdate.title')}</h1>
        <p className="text-sm text-muted-foreground">
          {appTooOld
            ? t('common.appUpdate.blockedApp', { version: minAppVersion })
            : t('common.appUpdate.blockedServer', { min: MIN_SERVER_VERSION, version: serverVersion })}
        </p>
      </div>
    </div>
  )
}
