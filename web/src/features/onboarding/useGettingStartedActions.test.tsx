import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { listenForInstallPrompt, resetInstallStateForTests } from '@/features/install/installPrompt'
import { HOME_SCREEN_KEY, useGettingStarted } from './useGettingStarted'
import { useGettingStartedActions } from './useGettingStartedActions'

const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'
const ANDROID_CHROME = 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36'

function Harness() {
  const { steps } = useGettingStarted()
  const { open, dialogs } = useGettingStartedActions('budget')
  const home = steps.find((s) => s.key === 'home_screen')
  return (
    <>
      <p data-testid="home-state">{home ? (home.done ? 'done' : home.dismissed ? 'dismissed' : 'open') : 'absent'}</p>
      <button type="button" onClick={() => open('home_screen')}>
        open home screen
      </button>
      {dialogs}
    </>
  )
}

function renderHarness(ua: string) {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(ua)
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('max-width'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers({ user: { ...fixtureUser, options: [...fixtureUser.options, { name: 'getting_started', value: '' }] } }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <Harness /> }])
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeAll(() => listenForInstallPrompt())

beforeEach(() => {
  resetInstallStateForTests()
  localStorage.clear()
  window.econumoConfig = {}
})

afterEach(() => vi.restoreAllMocks())

it('closing the iPhone install steps opened from the card finishes the home screen step', async () => {
  const user = userEvent.setup()
  renderHarness(IPHONE)
  await waitFor(() => expect(screen.getByTestId('home-state')).toHaveTextContent('open'))
  await user.click(screen.getByRole('button', { name: 'open home screen' }))
  const dialog = await screen.findByRole('dialog')
  expect(screen.getByTestId('home-state')).toHaveTextContent('open')
  await user.click(within(dialog).getByRole('button', { name: 'Got it' }))
  await waitFor(() => expect(screen.getByTestId('home-state')).toHaveTextContent('done'))
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBe('done')
})

it('an accepted install prompt finishes the home screen step; a dismissed one does not', async () => {
  const user = userEvent.setup()
  renderHarness(ANDROID_CHROME)
  const fire = (outcome: 'accepted' | 'dismissed') =>
    act(() => {
      window.dispatchEvent(
        Object.assign(new Event('beforeinstallprompt', { cancelable: true }), {
          prompt: vi.fn().mockResolvedValue(undefined),
          userChoice: Promise.resolve({ outcome }),
        }),
      )
    })
  fire('dismissed')
  await waitFor(() => expect(screen.getByTestId('home-state')).toHaveTextContent('open'))
  await user.click(screen.getByRole('button', { name: 'open home screen' }))
  await new Promise((r) => setTimeout(r, 20))
  // a used prompt is gone, so the card leaves with it until the browser offers another
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBeNull()

  fire('accepted')
  await waitFor(() => expect(screen.getByTestId('home-state')).toHaveTextContent('open'))
  await user.click(screen.getByRole('button', { name: 'open home screen' }))
  await waitFor(() => expect(screen.getByTestId('home-state')).toHaveTextContent('done'))
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBe('done')
})
