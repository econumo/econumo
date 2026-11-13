import { ThemeProvider } from '@/app/ThemeProvider'
import { TrackPageViews } from '@/app/TrackPageViews'

export function RootLayout() {
  return (
    <ThemeProvider>
      <TrackPageViews />
    </ThemeProvider>
  )
}
