import { useEffect, useEffectEvent, useLayoutEffect, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ThemeProvider as AppearanceProvider, useTheme } from 'next-themes'
import { queryKeys } from './queryKeys'
import type { CurrentUserDto } from '@/api/dto/user'
import { UserOptions } from '@/api/dto/user'
import { themePreference } from '@/lib/theme'
import { nativePlugin } from '@/lib/platform'

function DatabaseTheme({ system }: { system: boolean }) {
  // Observe login, hydration, refetches and preference writes without issuing
  // a user request on public routes. Authenticated screens own that fetch.
  const { data: user } = useQuery<CurrentUserDto>({ queryKey: queryKeys.user, enabled: false })
  const { setTheme, resolvedTheme } = useTheme()
  const applyPreference = useEffectEvent(setTheme)
  const preference = system ? 'system' : user ? themePreference(user.options.find((o) => o.name === UserOptions.THEME)?.value) : undefined

  useLayoutEffect(() => {
    if (preference !== undefined) {
      applyPreference(preference)
    }
    // A storage event updates the appearance without changing this tab's
    // cached user. Reapply only when the database preference actually changes.
  }, [preference, user?.id])

  useEffect(() => {
    // Also follows live OS changes in System mode, including the PWA chrome.
    const statusBar = nativePlugin<{ setStyle(options: { style: 'LIGHT' | 'DARK' }): Promise<void> }>('StatusBar')
    void statusBar?.setStyle({ style: resolvedTheme === 'dark' ? 'DARK' : 'LIGHT' }).catch(() => {})
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', resolvedTheme === 'dark' ? '#27282d' : '#ffffff')
  }, [resolvedTheme])

  return null
}

export function ThemeProvider({ children, system = false }: { children: ReactNode; system?: boolean }) {
  return (
    <AppearanceProvider
      key={system ? 'public' : 'user'}
      attribute="class"
      defaultTheme="system"
      storageKey={system ? 'econumo.public-theme' : 'econumo.theme'}
      enableSystem
      disableTransitionOnChange
    >
      <DatabaseTheme system={system} />
      {children}
    </AppearanceProvider>
  )
}
