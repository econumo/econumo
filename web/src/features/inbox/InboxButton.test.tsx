import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import type { Inbox } from './useInbox'
import { METRICS, trackEvent } from '@/lib/metrics'
import { InboxButton } from './InboxButton'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const mockInbox = vi.hoisted(() => ({ value: null as unknown as Inbox }))
vi.mock('./useInbox', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./useInbox')>()),
  useInbox: () => mockInbox.value,
}))

function makeInbox(over: Partial<Inbox> = {}): Inbox {
  return {
    dueRecurring: [],
    invites: [],
    syncProblems: [],
    failed: [],
    queued: [],
    skipped: [],
    count: 0,
    isLoaded: true,
    importsError: false,
    retryImports: vi.fn(),
    ...over,
  }
}

function renderButton() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/', element: <InboxButton variant="row" /> },
      { path: '/inbox', element: <div>INBOX PAGE</div> },
    ],
    { initialEntries: ['/'] },
  )
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.mocked(trackEvent).mockClear()
})

it('shows the Inbox link with no badge when nothing is pending', () => {
  mockInbox.value = makeInbox()
  renderButton()
  expect(screen.getByRole('link', { name: 'Inbox' })).toBeInTheDocument()
  expect(screen.queryByTestId('inbox-badge')).toBeNull()
})

it('shows a badge with the total count and a countable accessible name', () => {
  mockInbox.value = makeInbox({ queued: [{} as never, {} as never], failed: [{} as never], count: 3 })
  renderButton()
  expect(screen.getByTestId('inbox-badge')).toHaveTextContent('3')
  expect(screen.getByRole('link', { name: 'Inbox, 3 waiting' })).toBeInTheDocument()
})

it('caps the badge at 99+', () => {
  mockInbox.value = makeInbox({ queued: new Array(120).fill({}), count: 120 })
  renderButton()
  expect(screen.getByTestId('inbox-badge')).toHaveTextContent('99+')
})

it('navigates to the Inbox page and tracks INBOX_OPEN on click', async () => {
  mockInbox.value = makeInbox({ dueRecurring: [{} as never], queued: [{} as never, {} as never], failed: [{} as never], count: 4 })
  renderButton()
  const user = userEvent.setup()
  await user.click(screen.getByRole('link', { name: 'Inbox, 4 waiting' }))
  expect(await screen.findByText('INBOX PAGE')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.INBOX_OPEN, { dueRecurring: 1, invites: 0, syncProblems: 0, failed: 1, queued: 2 })
})
