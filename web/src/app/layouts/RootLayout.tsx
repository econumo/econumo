import { ThemeProvider } from '@/app/ThemeProvider'
import { TrackPageViews } from '@/app/TrackPageViews'
import { AppUpdateBlock } from '@/components/AppUpdateBlock'

export function RootLayout() {
  return (
    <ThemeProvider>
      <TrackPageViews />
      {/* The version gate covers every route, including sign-in. */}
      <AppUpdateBlock />
    </ThemeProvider>
  )
}
