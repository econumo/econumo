import { METRICS, trackEvent } from './metrics'

it('pushes the event with context to the dataLayer', () => {
  window.econumoConfig = {}
  window.dataLayer = []
  trackEvent(METRICS.TRANSACTION_CREATE, { a: 1 })
  expect(window.dataLayer).toHaveLength(1)
  const entry = window.dataLayer[0] as Record<string, unknown>
  expect(entry.event).toBe('appTransactionCreate')
  expect(entry.eventData).toEqual({ a: 1 })
  expect(entry.eventContext).toMatchObject({ selfHosted: false, locale: 'en' })
})
