import { afterEach, beforeEach, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AppUpdateBlock } from './AppUpdateBlock'
import { useServerConfig } from '@/lib/appConfig'

beforeEach(() => {
  // getVersion() reads this as the app build's own version.
  window.econumoConfig = { VERSION: 'v1.5.0' }
})

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  useServerConfig.setState({ serverVersion: null, minAppVersion: null })
})

it('renders nothing on the web or when both floors are satisfied', () => {
  useServerConfig.setState({ serverVersion: 'v0.1.0', minAppVersion: 'v99.0.0' })
  const { container } = render(<AppUpdateBlock />)
  expect(container).toBeEmptyDOMElement()

  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v99.0.0', minAppVersion: 'v1.0.0' })
  const { container: compatible } = render(<AppUpdateBlock />)
  expect(compatible).toBeEmptyDOMElement()
})

it('blocks when the app is older than the server accepts', () => {
  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v99.0.0', minAppVersion: 'v2.0.0' })
  render(<AppUpdateBlock />)
  expect(screen.getByText(/v2\.0\.0/)).toBeInTheDocument()
})

it('blocks when the server is older than the app supports', () => {
  window.Capacitor = { isNativePlatform: () => true }
  // A server predating the MIN_APP_VERSION key: only the app-side floor fires.
  useServerConfig.setState({ serverVersion: 'v0.9.0', minAppVersion: null })
  render(<AppUpdateBlock />)
  expect(screen.getByText(/v0\.9\.0/)).toBeInTheDocument()
})
