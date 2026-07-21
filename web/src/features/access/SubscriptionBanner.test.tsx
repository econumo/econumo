import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { formatDate } from '@/lib/datetime'
import { SubscriptionBanner } from './SubscriptionBanner'

function utcIn(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 19).replace('T', ' ')
}

function renderBanner() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return render(<SubscriptionBanner />, { wrapper })
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.dataLayer = []
})

it('renders nothing for full access', async () => {
  server.use(...coreHandlers())
  renderBanner()
  await waitFor(() => expect(window.dataLayer).toEqual([]))
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})

it('renders nothing for a trial outside the 3-day window', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(30) } }))
  renderBanner()
  await waitFor(() => expect(window.dataLayer).toEqual([]))
})

it('shows the dismissible trial variant inside 3 days, with the CTA, and fires the metric', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(2) } }))
  const user = userEvent.setup()
  renderBanner()
  expect(await screen.findByText('Your subscription ends in 2 days')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Manage subscription' })).toBeInTheDocument()
  expect(window.dataLayer).toContainEqual(expect.objectContaining({ event: 'appSubscriptionBannerShow' }))
  await user.click(screen.getByRole('button', { name: 'Dismiss' }))
  expect(screen.queryByText('Your subscription ends in 2 days')).not.toBeInTheDocument()
})

it('keeps the trial variant dismissed for the rest of the day across mounts', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(2) } }))
  const user = userEvent.setup()
  const { unmount } = renderBanner()
  await user.click(await screen.findByRole('button', { name: 'Dismiss' }))
  unmount()
  window.dataLayer = []
  renderBanner()
  await new Promise((r) => setTimeout(r, 50))
  expect(screen.queryByText('Your subscription ends in 2 days')).not.toBeInTheDocument()
  // No show metric for a banner suppressed by a persisted dismissal
  expect(window.dataLayer).toEqual([])
})

it('shows the trial variant again when the dismissal is from a previous day', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  localStorage.setItem('subscriptionBannerDismissedDay', formatDate(new Date(Date.now() - 86_400_000)))
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(2) } }))
  renderBanner()
  expect(await screen.findByText('Your subscription ends in 2 days')).toBeInTheDocument()
})

it('hides the trial variant entirely when billing is disabled', async () => {
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(2) } }))
  renderBanner()
  await waitFor(() => expect(window.dataLayer).toEqual([]))
})

it('shows the permanent readonly variant even without billing, minus the CTA', async () => {
  server.use(...coreHandlers({ user: { ...fixtureUser, accessLevel: 'readonly', accessUntil: '' } }))
  renderBanner()
  expect(await screen.findByText(/read-only/)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Manage subscription' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Dismiss' })).not.toBeInTheDocument()
})

it('shows the readonly CTA when billing is enabled', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessLevel: 'readonly', accessUntil: '' } }))
  renderBanner()
  expect(await screen.findByRole('button', { name: 'Manage subscription' })).toBeInTheDocument()
})

function partnerConn(accessLevel: 'full' | 'readonly', accessUntil: string) {
  return [{ user: { id: 'u2', avatar: 'pets:sky', name: 'Megan' }, accessLevel, accessUntil, sharedAccounts: [] }]
}

it('warns when a connection trial ends within 3 days, with the partner name', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ connections: partnerConn('full', utcIn(2)) }))
  renderBanner()
  expect(await screen.findByText("Megan's subscription ends in 2 days")).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Manage subscription' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Dismiss' })).toBeInTheDocument()
  expect(window.dataLayer).toContainEqual(expect.objectContaining({ event: 'appSubscriptionBannerShow' }))
})

it('shows nothing for a connection trial more than 3 days out', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ connections: partnerConn('full', utcIn(30)) }))
  renderBanner()
  await waitFor(() => expect(window.dataLayer).toEqual([]))
})

it('warns dismissibly when a connection is read-only', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ connections: partnerConn('readonly', '') }))
  const user = userEvent.setup()
  renderBanner()
  expect(await screen.findByText(/Megan's subscription has ended/)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Dismiss' }))
  expect(screen.queryByText(/Megan's subscription has ended/)).not.toBeInTheDocument()
  expect(localStorage.getItem('subscriptionBannerDismissedDay')).not.toBeNull()
})

it('own trial outranks a read-only connection', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(
    ...coreHandlers({
      user: { ...fixtureUser, accessUntil: utcIn(2) },
      connections: partnerConn('readonly', ''),
    }),
  )
  renderBanner()
  expect(await screen.findByText('Your subscription ends in 2 days')).toBeInTheDocument()
  expect(screen.queryByText(/Megan's subscription/)).not.toBeInTheDocument()
})

it('shows no partner variants when billing is disabled', async () => {
  server.use(...coreHandlers({ connections: partnerConn('readonly', '') }))
  renderBanner()
  await waitFor(() => expect(window.dataLayer).toEqual([]))
})
