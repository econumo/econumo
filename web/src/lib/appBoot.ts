import { isNativeApp, nativePlugin } from './platform'
import { restoreNativeStorage } from './appStorage'
import { fetchServerConfig } from './appConfig'
import { installExternalLinkInterceptor } from './externalLinks'

interface SplashScreenPlugin {
  hide(): Promise<void>
}

interface AppPlugin {
  addListener(ev: 'backButton', cb: () => void): unknown
  minimizeApp(): Promise<void>
}

const ROOT_PATHS = new Set(['/', '/login'])

// Storage restore must finish before the first render (auth state reads the
// token synchronously); the config fetch must NOT block first paint.
export async function bootNativeApp(): Promise<void> {
  if (!isNativeApp()) {
    return
  }
  await restoreNativeStorage()
  installExternalLinkInterceptor()
  installBackHandler()
  void fetchServerConfig()
}

export function installBackHandler(): void {
  const app = nativePlugin<AppPlugin>('App')
  if (!app) {
    return
  }
  app.addListener('backButton', () => {
    if (ROOT_PATHS.has(window.location.pathname)) {
      void app.minimizeApp()
      return
    }
    window.history.back()
  })
}

export function hideSplash(): void {
  void nativePlugin<SplashScreenPlugin>('SplashScreen')?.hide()
}
