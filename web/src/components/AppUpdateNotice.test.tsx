import { afterEach, beforeEach, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AppUpdateNotice } from './AppUpdateNotice'
import { useServerConfig } from '@/lib/appConfig'

beforeEach(() => {
  // getVersion() reads this as the app build's own version.
  window.econumoConfig = { VERSION: 'v1.5.0' }
  localStorage.clear()
})

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  useServerConfig.setState({ serverVersion: null, minAppVersion: null })
})

it('renders nothing on the web or when the app is current', () => {
  useServerConfig.setState({ serverVersion: 'v99.0.0' })
  const { container } = render(<AppUpdateNotice />)
  expect(container).toBeEmptyDOMElement()

  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v1.5.0' })
  const { container: current } = render(<AppUpdateNotice />)
  expect(current).toBeEmptyDOMElement()
})

it('nudges when the server runs a newer version, dismissable per server version', async () => {
  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v2.0.0' })
  render(<AppUpdateNotice />)
  expect(screen.getByText(/v2\.0\.0/)).toBeInTheDocument()

  await userEvent.click(screen.getByRole('button'))
  expect(screen.queryByText(/v2\.0\.0/)).not.toBeInTheDocument()

  // The dismissal persists for this server version across remounts.
  const { container } = render(<AppUpdateNotice />)
  expect(container).toBeEmptyDOMElement()
})
