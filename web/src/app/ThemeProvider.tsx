import { useEffect, useLayoutEffect, type ReactNode } from 'react'
import { ThemeProvider as AppearanceProvider, useTheme } from 'next-themes'
import { THEME_STORAGE_KEY, themePreference } from '@/lib/theme'
import { nativePlugin } from '@/lib/platform'
import { mirrorWrite } from '@/lib/appStorage'

function DeviceTheme() {
  const { theme, setTheme, resolvedTheme } = useTheme()

  useLayoutEffect(() => {
    if (theme !== undefined && theme !== themePreference(theme)) {
      setTheme('system')
    }
  }, [theme, setTheme])

  useEffect(() => {
    // Keep the native copy through WebView storage eviction. This is local
    // to the app installation, independent of account or selected server.
    mirrorWrite(THEME_STORAGE_KEY, themePreference(theme))
  }, [theme])

  useEffect(() => {
    // Also follows live OS changes in System mode, including the PWA chrome.
    const statusBar = nativePlugin<{ setStyle(options: { style: 'LIGHT' | 'DARK' }): Promise<void> }>('StatusBar')
    void statusBar?.setStyle({ style: resolvedTheme === 'dark' ? 'DARK' : 'LIGHT' }).catch(() => {})
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', resolvedTheme === 'dark' ? '#27282d' : '#ffffff')
  }, [resolvedTheme])

  return null
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <AppearanceProvider
      attribute="class"
      defaultTheme="system"
      storageKey={THEME_STORAGE_KEY}
      enableSystem
      disableTransitionOnChange
    >
      <DeviceTheme />
      {children}
    </AppearanceProvider>
  )
}
