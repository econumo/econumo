import { render, screen } from '@testing-library/react'
import { OfflineUnavailable } from './OfflineUnavailable'

it('says the page is not on this device and how to get it', () => {
  render(<OfflineUnavailable />)
  const state = screen.getByTestId('offline-unavailable')
  expect(state).toHaveTextContent('Not saved on this device')
  expect(state).toHaveTextContent("This data isn't saved on this device. Connect to the internet to load it.")
})

it('compact form fits inside a card: one line, no heading', () => {
  render(<OfflineUnavailable compact />)
  const state = screen.getByTestId('offline-unavailable')
  expect(state).toHaveTextContent("This data isn't saved on this device.")
  expect(state).not.toHaveTextContent('Not saved on this device')
})
