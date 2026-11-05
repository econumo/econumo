import { shareUrl } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'

export function shareTripUrl(tagId: string): string {
  const base = shareUrl()
  return base ? `${base}/new?kind=trip&tag=${encodeURIComponent(tagId)}` : ''
}

export function sharedReportsUrl(): string {
  const base = shareUrl()
  return base ? `${base}/app` : ''
}

export function openShareTrip(tagId: string, source: 'tags' | 'budget'): void {
  window.open(shareTripUrl(tagId), '_blank', 'noopener,noreferrer')
  trackEvent(METRICS.SHARE_TRIP_OPEN, { source })
}

export function openSharedReports(): void {
  window.open(sharedReportsUrl(), '_blank', 'noopener,noreferrer')
  trackEvent(METRICS.SHARED_REPORTS_OPEN)
}
