import type { CapacitorConfig } from '@capacitor/cli'

const config: CapacitorConfig = {
  appId: 'com.econumo.app',
  appName: 'Econumo',
  webDir: '../web/dist',
  plugins: {
    // Route fetch/XHR through the native HTTP stack: requests carry no browser
    // origin, so CORS never applies and self-hosted servers need zero config.
    CapacitorHttp: { enabled: true },
    // appBoot.hideSplash() dismisses it after the first render.
    SplashScreen: { launchAutoHide: false },
  },
}

export default config
