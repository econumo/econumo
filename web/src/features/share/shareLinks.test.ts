import { openShareTrip, openSharedReports, shareTripUrl, sharedReportsUrl } from './shareLinks'
import { METRICS, trackEvent } from '@/lib/metrics'
import { backendHost, selfHosted } from '@/lib/config'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  window.econumoConfig = { SHARE_URL: 'https://share.example.test' }
})

describe('share links', () => {
  it('builds the trip and reports urls', () => {
    expect(shareTripUrl('a b')).toBe('https://share.example.test/new?kind=trip&tag=a%20b')
    expect(sharedReportsUrl()).toBe('https://share.example.test/app')
  })

  it('opens the trip page and tracks it', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    openShareTrip('t1', 'budget')
    expect(open).toHaveBeenCalledWith('https://share.example.test/new?kind=trip&tag=t1', '_blank', 'noopener,noreferrer')
    expect(trackEvent).toHaveBeenCalledWith(METRICS.SHARE_TRIP_OPEN, { source: 'budget' })
    open.mockRestore()
  })

  it('opens shared reports and tracks it', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    openSharedReports()
    expect(open).toHaveBeenCalledWith('https://share.example.test/app', '_blank', 'noopener,noreferrer')
    expect(trackEvent).toHaveBeenCalledWith(METRICS.SHARED_REPORTS_OPEN)
    open.mockRestore()
  })

  it('is empty without a share url', () => {
    window.econumoConfig = { SHARE_URL: '' }
    expect(shareTripUrl('t1')).toBe('')
    expect(sharedReportsUrl()).toBe('')
  })

  it('is empty when the page talks to a backend on another origin', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: true, SHARE_URL: 'https://share.example.test' }
    selfHosted(true)
    backendHost('https://other.example.test')
    expect(shareTripUrl('t1')).toBe('')
  })
})
