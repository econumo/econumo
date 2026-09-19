import { cleanup, render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { AppReturnPage } from './AppReturnPage'

function renderAt(entry: string) {
  const router = createMemoryRouter([{ path: '/oauth/app-return', element: <AppReturnPage /> }], {
    initialEntries: [entry],
  })
  return render(<RouterProvider router={router} />)
}

beforeEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('offers one link back into the app carrying the handoff in the fragment', () => {
  renderAt('/oauth/app-return#handoff=abc')
  expect(screen.getByText('Sign-in finished — return to the Econumo app.')).toBeInTheDocument()
  const link = screen.getByRole('link', { name: 'Open the app' })
  expect(link).toHaveAttribute('href', 'com.econumo.app://oauth#handoff=abc')
})

it('keeps the handoff out of the rendered text', () => {
  const { container } = renderAt('/oauth/app-return#handoff=abc')
  expect(container.textContent).not.toContain('abc')
})

it('carries a link handoff, an error and a link error the way the backend sent them', () => {
  for (const [entry, href] of [
    ['/oauth/app-return#linkHandoff=xyz', 'com.econumo.app://oauth#linkHandoff=xyz'],
    ['/oauth/app-return?error=denied', 'com.econumo.app://oauth?error=denied'],
    ['/oauth/app-return?linkError=identity_taken', 'com.econumo.app://oauth?linkError=identity_taken'],
  ]) {
    renderAt(entry)
    expect(screen.getByRole('link')).toHaveAttribute('href', href)
    cleanup()
  }
})

it('renders nothing inside the app, where the deep-link handler already dispatched', () => {
  window.Capacitor = { isNativePlatform: () => true, Plugins: {} }
  const { container } = renderAt('/oauth/app-return#handoff=abc')
  expect(container.textContent).toBe('')
})
