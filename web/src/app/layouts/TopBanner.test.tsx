import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { TopBanner } from './TopBanner'

// No sale running: billing answers null.
vi.mock('@/lib/billingBanner', () => ({ getSaleBanner: vi.fn(async () => null) }))

vi.mock('@/features/install/installPrompt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/install/installPrompt')>()),
  useInstallMode: () => 'ios',
}))

function utcIn(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 19).replace('T', ' ')
}

function renderTopBanner() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return render(<TopBanner />, { wrapper })
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
})

it('shows one banner at a time: the subscription first, the install nudge once it is dismissed', async () => {
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(2) } }))
  const user = userEvent.setup()
  renderTopBanner()
  expect(await screen.findByText('Your subscription ends in 2 days')).toBeInTheDocument()
  expect(screen.queryByText('Econumo works best as an app')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Dismiss' }))
  expect(screen.queryByText('Your subscription ends in 2 days')).not.toBeInTheDocument()
  expect(screen.getByText('Econumo works best as an app')).toBeInTheDocument()
})

it('shows the install nudge when nothing more urgent is up', async () => {
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(30) } }))
  renderTopBanner()
  expect(await screen.findByText('Econumo works best as an app')).toBeInTheDocument()
})
