import { beforeEach, expect, it, vi } from 'vitest'
import { act, StrictMode } from 'react'
import { createBrowserRouter, Navigate, RouterProvider } from 'react-router'
import { render } from '@testing-library/react'
import { TrackPageViews } from './TrackPageViews'
import { METRICS, trackEvent } from '@/lib/metrics'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

beforeEach(() => {
  vi.clearAllMocks()
})

// A browser (not memory) router: TrackPageViews compares the route location
// against window.location to drop commits superseded by a redirect.
function makeRouter(initialPath: string, home = <div>home</div>) {
  window.history.replaceState({}, '', initialPath)
  return createBrowserRouter([
    {
      element: <TrackPageViews />,
      children: [
        { path: '/', element: home },
        { path: '/budget', element: <div>budget</div> },
      ],
    },
  ])
}

it('tracks a page view on initial load and on every route change', async () => {
  const router = makeRouter('/')
  render(<RouterProvider router={router} />)
  expect(trackEvent).toHaveBeenCalledTimes(1)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.PAGE_VIEW)
  await act(() => router.navigate('/budget'))
  expect(trackEvent).toHaveBeenCalledTimes(2)
})

it('does not double-track under StrictMode or same-path navigation', async () => {
  const router = makeRouter('/')
  render(
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>,
  )
  expect(trackEvent).toHaveBeenCalledTimes(1)
  await act(() => router.navigate('/'))
  expect(trackEvent).toHaveBeenCalledTimes(1)
})

it('counts a redirected landing once, for the destination only', async () => {
  const router = makeRouter('/', <Navigate to="/budget" replace />)
  await act(() => render(<RouterProvider router={router} />))
  expect(trackEvent).toHaveBeenCalledTimes(1)
})
