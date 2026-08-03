import { isNativeApp, nativePlugin } from './platform'
import { restoreNativeStorage } from './appStorage'
import { fetchServerConfig } from './appConfig'
import { installExternalLinkInterceptor } from './externalLinks'

interface SplashScreenPlugin {
  hide(): Promise<void>
}

// Storage restore must finish before the first render (auth state reads the
// token synchronously); the config fetch must NOT block first paint.
export async function bootNativeApp(): Promise<void> {
  if (!isNativeApp()) {
    return
  }
  await restoreNativeStorage()
  installExternalLinkInterceptor()
  void fetchServerConfig()
}

export function hideSplash(): void {
  void nativePlugin<SplashScreenPlugin>('SplashScreen')?.hide()
}
