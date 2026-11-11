import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { ApplicationLayout } from './ApplicationLayout'

vi.mock('@/features/install/installPrompt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/install/installPrompt')>()),
  useInstallMode: () => 'ios',
}))

const withOnboarding = (value: string) => ({
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value } : o)),
})

function renderShell() {
  const router = createMemoryRouter(
    [{ element: <ApplicationLayout />, children: [{ path: '/', element: <div>HOME CONTENT</div> }] }],
    { initialEntries: ['/'] },
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('shows neither the install banner nor the install popup while onboarding is incomplete', async () => {
  server.use(...coreHandlers({ user: withOnboarding('') }))
  renderShell()
  expect(await screen.findByText('HOME CONTENT')).toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 50))
  expect(screen.queryByText('Econumo works best as an app')).not.toBeInTheDocument()
  expect(screen.queryByText('Install Econumo')).not.toBeInTheDocument()
  // the once-per-device popup is still owed for after onboarding
  expect(localStorage.getItem('econumo.install-popup-shown')).toBeNull()
})

it('shows both once onboarding is complete', async () => {
  server.use(...coreHandlers({ user: withOnboarding('completed') }))
  renderShell()
  expect(await screen.findByText('Install Econumo')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByText('Econumo works best as an app')).toBeInTheDocument())
})
