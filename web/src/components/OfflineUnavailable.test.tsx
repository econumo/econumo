import { render, screen } from '@testing-library/react'
import { OfflineUnavailable } from './OfflineUnavailable'

it('says the page is not on this device and how to get it', () => {
  render(<OfflineUnavailable />)
  const state = screen.getByTestId('offline-unavailable')
  expect(state).toHaveTextContent('Not saved on this device')
  expect(state).toHaveTextContent('Connect to the internet to load it.')
})

it('names a budget month', () => {
  render(<OfflineUnavailable subject="month" />)
  expect(screen.getByTestId('offline-unavailable')).toHaveTextContent("This month isn't saved on this device.")
})

it('compact form fits inside a card: one line, no heading', () => {
  render(<OfflineUnavailable subject="report" compact />)
  const state = screen.getByTestId('offline-unavailable')
  expect(state).toHaveTextContent("This report isn't saved on this device.")
  expect(state).not.toHaveTextContent('Not saved on this device')
})
