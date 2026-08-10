import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ServerVersionNotice } from './ServerVersionNotice'
import { useServerConfig } from '@/lib/appConfig'

beforeEach(() => {
  // getVersion() reads this as the app build's own version.
  window.econumoConfig = { VERSION: 'v1.5.0' }
  vi.stubGlobal('fetch', vi.fn(async () => {
    throw new Error('offline')
  }))
})

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  useServerConfig.setState({ serverVersion: null, minAppVersion: null })
  vi.unstubAllGlobals()
})

it('renders nothing on the web or when the server is not older than the app', () => {
  useServerConfig.setState({ serverVersion: 'v0.9.0' })
  const { container } = render(<ServerVersionNotice />)
  expect(container).toBeEmptyDOMElement()

  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v99.0.0' })
  const { container: newer } = render(<ServerVersionNotice />)
  expect(newer).toBeEmptyDOMElement()

  useServerConfig.setState({ serverVersion: 'v1.5.0' })
  const { container: equal } = render(<ServerVersionNotice />)
  expect(equal).toBeEmptyDOMElement()
})

it('nudges when the server is older than the app', () => {
  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v1.4.0' })
  render(<ServerVersionNotice />)
  expect(screen.getByText(/v1\.4\.0/)).toBeInTheDocument()
})
