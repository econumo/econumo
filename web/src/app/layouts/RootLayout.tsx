import { useLocation } from 'react-router'
import { ThemeProvider } from '@/app/ThemeProvider'
import { TrackPageViews } from '@/app/TrackPageViews'
import { AppUpdateBlock } from '@/components/AppUpdateBlock'

export function RootLayout() {
  const { pathname } = useLocation()
  const system = /^\/(?:login|register)\/?$/.test(pathname)
  return (
    <ThemeProvider system={system}>
      <TrackPageViews />
      {/* The version gate covers every route, including sign-in. */}
      <AppUpdateBlock />
    </ThemeProvider>
  )
}
