import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { CommentsPanel } from './CommentsPanel'

const comment = {
  id: 'cm1',
  elementId: 'cat-food',
  period: '2026-07-01',
  comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-07-17 09:00:00',
  updatedAt: '2026-07-17 09:00:00',
}

function mockMatchMedia(matches: (q: string) => boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: matches(q), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPanel(anchor: HTMLElement | null) {
  server.use(...coreHandlers())
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CommentsPanel
        open
        onClose={() => {}}
        title="Groceries"
        anchor={anchor}
        budgetId="b1"
        elementId="cat-food"
        period="2026-07-01"
        comments={[comment]}
        currentUserId="u1"
        canModerate={false}
        readOnly={false}
        truncated={false}
      />
    </QueryClientProvider>,
  )
}

function makeAnchor() {
  const el = document.createElement('div')
  document.body.appendChild(el)
  return el
}

it('anchors the thread to the cell on desktop', async () => {
  mockMatchMedia(() => false)
  renderPanel(makeAnchor())
  expect(await screen.findByTestId('comments-popover')).toBeInTheDocument()
  expect(screen.getByText('Trip to Lisbon')).toBeInTheDocument()
  expect(screen.queryByTestId('comments-sheet')).toBeNull()
})

it('anchors the thread on a tablet too', async () => {
  mockMatchMedia((q) => q.includes('1023'))
  renderPanel(makeAnchor())
  expect(await screen.findByTestId('comments-popover')).toBeInTheDocument()
})

it('falls back to the sheet without an anchor', async () => {
  mockMatchMedia(() => false)
  renderPanel(null)
  expect(await screen.findByTestId('comments-sheet')).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Groceries' })).toBeInTheDocument()
})

it('ignores the anchor on a phone', async () => {
  mockMatchMedia(() => true)
  renderPanel(makeAnchor())
  expect(await screen.findByTestId('comments-sheet')).toBeInTheDocument()
  expect(screen.queryByTestId('comments-popover')).toBeNull()
})
