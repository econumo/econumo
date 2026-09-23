import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import type { BudgetCommentDto } from '@/api/dto/budget'
import { CommentThread, type CommentThreadProps } from './CommentThread'

const ada = { id: 'u1', avatar: 'face:emerald', name: 'Ada' }
const bob = { id: 'u2', avatar: 'pets:sky', name: 'Bob' }

const commentByAda: BudgetCommentDto = {
  id: 'c1',
  elementId: 'e1',
  period: '2026-01-01',
  comment: 'First note',
  author: ada,
  createdAt: '2026-01-01 10:00:00',
  updatedAt: '2026-01-01 10:00:00',
}

const commentByBob: BudgetCommentDto = {
  id: 'c2',
  elementId: 'e1',
  period: '2026-01-01',
  comment: 'Second note',
  author: bob,
  createdAt: '2026-01-02 10:00:00',
  updatedAt: '2026-01-02 11:00:00',
}

function renderThread(overrides: Partial<CommentThreadProps> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const props: CommentThreadProps = {
    budgetId: 'b1',
    elementId: 'e1',
    period: '2026-01-01',
    comments: [],
    currentUserId: 'u1',
    canModerate: false,
    readOnly: false,
    ...overrides,
  }
  const view = render(
    <QueryClientProvider client={queryClient}>
      <CommentThread {...props} />
    </QueryClientProvider>,
  )
  const rerenderWith = (next: Partial<CommentThreadProps>) => {
    const merged = { ...props, ...next }
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <CommentThread {...merged} />
      </QueryClientProvider>,
    )
  }
  return { props, rerender: rerenderWith }
}

beforeEach(() => {
  localStorage.clear()
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('renders every comment with its author name and local date, oldest first', () => {
  renderThread({ comments: [commentByBob, commentByAda] })
  expect(screen.getByRole('list', { name: '2 comments' })).toBeInTheDocument()
  const items = screen.getAllByRole('listitem')
  expect(items).toHaveLength(2)
  expect(within(items[0]).getByText('Ada')).toBeInTheDocument()
  expect(within(items[0]).getByText('First note')).toBeInTheDocument()
  expect(within(items[1]).getByText('Bob')).toBeInTheDocument()
  expect(within(items[1]).getByText('Second note')).toBeInTheDocument()
})

it('shows (edited) only when updatedAt !== createdAt', () => {
  renderThread({ comments: [commentByAda, commentByBob] })
  const items = screen.getAllByRole('listitem')
  expect(within(items[0]).queryByText('(edited)')).toBeNull()
  expect(within(items[1]).getByText('(edited)')).toBeInTheDocument()
})

it('own comment offers Edit and Delete; another author\'s offers neither when canModerate is false', () => {
  renderThread({ comments: [commentByAda, commentByBob], currentUserId: 'u1', canModerate: false })
  const items = screen.getAllByRole('listitem')
  expect(within(items[0]).getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(within(items[0]).getByRole('button', { name: 'Delete' })).toBeInTheDocument()
  expect(within(items[1]).queryByRole('button', { name: 'Edit' })).toBeNull()
  expect(within(items[1]).queryByRole('button', { name: 'Delete' })).toBeNull()
})

it('another author\'s comment offers Delete but not Edit when canModerate is true', () => {
  renderThread({ comments: [commentByAda, commentByBob], currentUserId: 'u1', canModerate: true })
  const items = screen.getAllByRole('listitem')
  expect(within(items[1]).queryByRole('button', { name: 'Edit' })).toBeNull()
  expect(within(items[1]).getByRole('button', { name: 'Delete' })).toBeInTheDocument()
})

it('the composer posts on click and on Cmd/Ctrl+Enter, and is disabled while blank', async () => {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/budget/create-comment', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({
        success: true,
        message: '',
        data: { item: { ...commentByAda, id: `posted-${bodies.length}`, comment: (bodies.at(-1)!.comment as string) } },
      })
    }),
  )
  const user = userEvent.setup()
  renderThread({ comments: [] })
  const composer = screen.getByPlaceholderText('Add a note for this month')
  const post = screen.getByRole('button', { name: 'Post' })
  expect(post).toBeDisabled()

  await user.type(composer, 'Via click')
  expect(post).not.toBeDisabled()
  await user.click(post)
  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0].comment).toBe('Via click')
  expect(bodies[0].elementId).toBe('e1')
  expect(bodies[0].period).toBe('2026-01-01')

  await user.type(composer, 'Via keyboard')
  await user.keyboard('{Control>}{Enter}{/Control}')
  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[1].comment).toBe('Via keyboard')
})

it('the counter shows n/500 and the post button disables past 500 characters', async () => {
  const user = userEvent.setup()
  renderThread({ comments: [] })
  const composer = screen.getByPlaceholderText('Add a note for this month')
  const post = screen.getByRole('button', { name: 'Post' })

  await user.type(composer, 'hello')
  expect(screen.getByText('5/500')).toBeInTheDocument()
  expect(post).not.toBeDisabled()

  // '😀' is 2 UTF-16 code units but 1 rune: .length and [...value].length
  // disagree here, so this pair is what actually pins rune counting — a
  // regression to .length would still pass with an ASCII fixture.
  const emoji = '😀'
  await user.clear(composer)
  await user.paste(emoji.repeat(500))
  expect(screen.getByText('500/500')).toBeInTheDocument()
  expect(post).not.toBeDisabled()

  await user.clear(composer)
  await user.paste(emoji.repeat(501))
  expect(screen.getByText('501/500')).toBeInTheDocument()
  expect(post).toBeDisabled()
})

it('readOnly hides the composer and every menu, and shows the read-only hint', () => {
  renderThread({ comments: [commentByAda, commentByBob], currentUserId: 'u1', canModerate: true, readOnly: true })
  expect(screen.queryByPlaceholderText('Add a note for this month')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
  expect(screen.getByText("This budget is archived — comments can be read but not changed.")).toBeInTheDocument()
})

it('drops an in-flight edit box and delete confirm when readOnly turns on mid-mount', async () => {
  const user = userEvent.setup()
  const { rerender } = renderThread({ comments: [commentByAda, commentByBob], currentUserId: 'u1', canModerate: true, readOnly: false })
  const items = screen.getAllByRole('listitem')

  await user.click(within(items[0]).getByRole('button', { name: 'Edit' }))
  expect(within(items[0]).getByRole('textbox', { name: 'Comment' })).toBeInTheDocument()

  await user.click(within(items[1]).getByRole('button', { name: 'Delete' }))
  expect(screen.getByText('Delete this comment?')).toBeInTheDocument()

  rerender({ readOnly: true })

  expect(within(items[0]).queryByRole('textbox', { name: 'Comment' })).toBeNull()
  expect(within(items[0]).queryByRole('button', { name: 'Save' })).toBeNull()
  expect(screen.queryByText('Delete this comment?')).toBeNull()
})
