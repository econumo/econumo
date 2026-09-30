import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent, { PointerEventsCheckLevel } from '@testing-library/user-event'
import { CellShell } from './CellShell'

const c = (id: string, text: string, at: string) => ({
  id,
  elementId: 'cat-food',
  period: '2026-07-01',
  comment: text,
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: at,
  updatedAt: at,
})
const three = [
  c('a', 'First note', '2026-07-01 09:00:00'),
  c('b', 'Second note', '2026-07-02 09:00:00'),
  c('d', 'Third note', '2026-07-03 09:00:00'),
]

function mockMatchMedia(matches: (q: string) => boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: matches(q), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}
const desktop = () => mockMatchMedia(() => false)
const tablet = () => mockMatchMedia((q) => q.includes('1023'))

function renderShell(props: Partial<Parameters<typeof CellShell>[0]> = {}, onCellClick = vi.fn()) {
  render(
    <CellShell title="Groceries" comments={three} {...props}>
      <div data-testid="cell" data-comment-anchor="" onClick={onCellClick}>
        700.00
      </div>
    </CellShell>,
  )
  return screen.getByTestId('cell')
}

beforeEach(desktop)

afterEach(() => {
  vi.useRealTimers()
})

it('previews the latest two comments on hover, with a count of the rest', async () => {
  const user = userEvent.setup()
  const cell = renderShell()
  await user.hover(cell)
  const preview = await screen.findByTestId('comment-preview', {}, { timeout: 1500 })
  expect(preview).toHaveTextContent('Second note')
  expect(preview).toHaveTextContent('Third note')
  expect(preview).not.toHaveTextContent('First note')
  expect(preview).toHaveTextContent('+1 more')
})

it('shows no preview when disabled', async () => {
  const user = userEvent.setup()
  const cell = renderShell({ previewDisabled: true })
  await user.hover(cell)
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('closes the preview on press and keeps it closed until the pointer leaves', async () => {
  const user = userEvent.setup()
  const cell = renderShell()
  await user.hover(cell)
  await screen.findByTestId('comment-preview', {}, { timeout: 1500 })
  await user.pointer({ keys: '[MouseLeft]', target: cell })
  await waitFor(() => expect(screen.queryByTestId('comment-preview')).toBeNull())
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('offers set budget, comments and transactions on right-click and runs the picked action with the cell', async () => {
  const user = userEvent.setup()
  const onSetBudget = vi.fn()
  const onOpenComments = vi.fn()
  const onShowTransactions = vi.fn()
  const cell = renderShell({ onSetBudget, onOpenComments, onShowTransactions })
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(await screen.findByRole('menuitem', { name: 'Set budget' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Comments (3)' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Show transactions' })).toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Comments (3)' }))
  await waitFor(() => expect(onOpenComments).toHaveBeenCalledWith(cell))
  expect(onSetBudget).not.toHaveBeenCalled()
})

it('omits the items the caller cannot use, and names an empty thread "Add comment"', async () => {
  const user = userEvent.setup()
  const cell = renderShell({ comments: [], onOpenComments: vi.fn() })
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(await screen.findByRole('menuitem', { name: 'Add comment' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Set budget' })).toBeNull()
  expect(screen.queryByRole('menuitem', { name: 'Show transactions' })).toBeNull()
})

it('opens no menu when disabled', async () => {
  const user = userEvent.setup()
  const cell = renderShell({ menuDisabled: true, onOpenComments: vi.fn() })
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(screen.queryByRole('menu')).toBeNull()
})

it('on a tablet, a long-press opens the actions modal and its release does not also tap the cell', async () => {
  tablet()
  // plain vi.useFakeTimers() deadlocks userEvent.pointer() in this jsdom/vitest
  // combination (reproduced with a bare <div>, no Radix involved): something
  // React's scheduler relies on never fires unless the fake clock also ticks in
  // real time. shouldAdvanceTime keeps that ticking while advanceTimersByTime
  // below still deterministically fires our own long-press timer.
  vi.useFakeTimers({ shouldAdvanceTime: true })
  // The actions modal is a real Radix Dialog, which disables `pointer-events`
  // on the rest of the page (document.body) the instant it opens — including
  // the cell, which is now covered by its overlay. On a real touch screen the
  // release still targets the cell: touch pointers get implicit capture at
  // pointerdown, bypassing hit-testing entirely, so CSS on the covered element
  // is irrelevant. jsdom/user-event don't model that capture and instead
  // re-check the CSS on every call, so the release below would otherwise throw
  // "Unable to perform pointer interaction … pointer-events: none" even though
  // a real device delivers it fine; PointerEventsCheckLevel.Never turns that
  // simulation-only check off for this touch sequence.
  const fakeUser = userEvent.setup({ advanceTimers: vi.advanceTimersByTime, pointerEventsCheck: PointerEventsCheckLevel.Never })
  const onCellClick = vi.fn()
  const onOpenComments = vi.fn()
  const cell = renderShell({ onSetBudget: vi.fn(), onOpenComments, onShowTransactions: vi.fn() }, onCellClick)

  await fakeUser.pointer({ keys: '[TouchA>]', target: cell })
  act(() => {
    vi.advanceTimersByTime(550)
  })
  const modal = screen.getByTestId('cell-actions')
  expect(screen.getByRole('dialog', { name: 'Groceries' })).toBeInTheDocument()
  expect(within(modal).getByRole('button', { name: 'Set budget' })).toBeInTheDocument()
  expect(within(modal).getByRole('button', { name: 'Show transactions' })).toBeInTheDocument()
  await fakeUser.pointer({ keys: '[/TouchA]', target: cell })
  expect(onCellClick).not.toHaveBeenCalled()

  vi.useRealTimers()
  const user = userEvent.setup()
  await user.click(within(modal).getByRole('button', { name: 'Comments (3)' }))
  await waitFor(() => expect(onOpenComments).toHaveBeenCalledWith(cell))
  expect(screen.queryByTestId('cell-actions')).toBeNull()
})

it('on a tablet, a quick tap still taps the cell, and there is no hover preview or context menu', async () => {
  tablet()
  const user = userEvent.setup()
  const onCellClick = vi.fn()
  const cell = renderShell({ onOpenComments: vi.fn() }, onCellClick)
  await user.pointer({ keys: '[TouchA]', target: cell })
  expect(onCellClick).toHaveBeenCalledTimes(1)
  expect(screen.queryByTestId('cell-actions')).toBeNull()
  await user.hover(cell)
  await user.pointer({ keys: '[MouseRight]', target: cell })
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByTestId('comment-preview')).toBeNull()
  expect(screen.queryByRole('menu')).toBeNull()
  // the quick tap's release must not have started (and left pending) a long press
  expect(screen.queryByTestId('cell-actions')).toBeNull()
})

it('never remounts the cell: previewDisabled toggling and comments arriving/leaving keep the same DOM node (an open thread popover is anchored to it)', () => {
  const cellChild = (
    <div data-testid="cell" data-comment-anchor="">
      700.00
    </div>
  )
  const { rerender } = render(
    <CellShell title="Groceries" comments={three} menuDisabled>
      {cellChild}
    </CellShell>,
  )
  const before = screen.getByTestId('cell')

  rerender(
    <CellShell title="Groceries" comments={three} menuDisabled previewDisabled>
      {cellChild}
    </CellShell>,
  )
  expect(screen.getByTestId('cell')).toBe(before)

  rerender(
    <CellShell title="Groceries" comments={[]} menuDisabled previewDisabled>
      {cellChild}
    </CellShell>,
  )
  expect(screen.getByTestId('cell')).toBe(before)

  rerender(
    <CellShell title="Groceries" comments={[three[0]]} menuDisabled previewDisabled={false}>
      {cellChild}
    </CellShell>,
  )
  expect(screen.getByTestId('cell')).toBe(before)
})
