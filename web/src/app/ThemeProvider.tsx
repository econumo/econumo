import { useEffect, useLayoutEffect, type ReactNode } from 'react'
import { ThemeProvider as AppearanceProvider, useTheme } from 'next-themes'
import { THEME_STORAGE_KEY, themePreference } from '@/lib/theme'

function DeviceTheme() {
  const { theme, setTheme, resolvedTheme } = useTheme()

  useLayoutEffect(() => {
    if (theme !== undefined && theme !== themePreference(theme)) {
      setTheme('system')
    }
  }, [theme, setTheme])

  useEffect(() => {
    // Also follows live OS changes in System mode, including the PWA chrome.
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
