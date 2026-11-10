import { render, screen } from '@testing-library/react'
import { OfflineUnavailable } from './OfflineUnavailable'

it('says the page is not available offline', () => {
  render(<OfflineUnavailable />)
  const state = screen.getByTestId('offline-unavailable')
  expect(state).toHaveTextContent('Not available offline')
  expect(state).toHaveTextContent("This data isn't available without an internet connection.")
})

it('compact form fits inside a card: one line, no heading', () => {
  render(<OfflineUnavailable compact />)
  const state = screen.getByTestId('offline-unavailable')
  expect(state).toHaveTextContent("This data isn't available without an internet connection.")
  expect(state).not.toHaveTextContent('Not available offline')
})
