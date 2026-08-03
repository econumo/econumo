import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ServerVersionNotice } from './ServerVersionNotice'
import { useServerConfig } from '@/lib/appConfig'

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => {
    throw new Error('offline')
  }))
})

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  useServerConfig.setState({ serverVersion: null })
  vi.unstubAllGlobals()
})

it('renders nothing on the web or when the server is current', () => {
  useServerConfig.setState({ serverVersion: 'v0.9.0' })
  const { container } = render(<ServerVersionNotice />)
  expect(container).toBeEmptyDOMElement()

  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v99.0.0' })
  const { container: current } = render(<ServerVersionNotice />)
  expect(current).toBeEmptyDOMElement()
})

it('warns when the app runs against an outdated server', () => {
  window.Capacitor = { isNativePlatform: () => true }
  useServerConfig.setState({ serverVersion: 'v0.9.0' })
  render(<ServerVersionNotice />)
  expect(screen.getByText(/v0\.9\.0/)).toBeInTheDocument()
})
