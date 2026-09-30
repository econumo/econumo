import { createRef } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

// The preview's open/close delays are CellShell's own timers: a fake clock that
// still ticks in real time (see the tablet long-press test below for why plain
// fake timers are not an option) lets the tests step over them deterministically.
function previewClock() {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
}
const tick = (ms: number) =>
  act(() => {
    vi.advanceTimersByTime(ms)
  })

it('previews the latest two comments on hover, with a count of the rest', async () => {
  const user = previewClock()
  const cell = renderShell()
  await user.hover(cell)
  tick(200)
  expect(screen.queryByTestId('comment-preview')).toBeNull()
  tick(100)
  const preview = screen.getByTestId('comment-preview')
  expect(preview).toHaveTextContent('Second note')
  expect(preview).toHaveTextContent('Third note')
  expect(preview).not.toHaveTextContent('First note')
  expect(preview).toHaveTextContent('+1 more')
  // a read-only card: the cell keeps whatever focus it had
  expect(preview).not.toContainElement(document.activeElement as HTMLElement)
})

it('keeps the preview open while the pointer is on the card, and closes it once the pointer leaves both', async () => {
  const user = previewClock()
  const cell = renderShell()
  await user.hover(cell)
  tick(300)
  const preview = screen.getByTestId('comment-preview')
  await user.hover(preview)
  tick(500)
  expect(screen.getByTestId('comment-preview')).toBeInTheDocument()
  await user.unhover(preview)
  tick(100)
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('shows no preview when disabled', async () => {
  const user = previewClock()
  const cell = renderShell({ previewDisabled: true })
  await user.hover(cell)
  tick(500)
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('closes the preview on press and keeps it closed until the pointer leaves', async () => {
  const user = previewClock()
  const cell = renderShell()
  await user.hover(cell)
  tick(300)
  expect(screen.getByTestId('comment-preview')).toBeInTheDocument()
  await user.pointer({ keys: '[MouseLeft]', target: cell })
  expect(screen.queryByTestId('comment-preview')).toBeNull()
  tick(500)
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('shows no preview over an open amount editor, even after the pointer left and came back', async () => {
  const user = previewClock()
  render(
    <CellShell title="Groceries" comments={three}>
      <div data-testid="cell" data-comment-anchor="">
        <button type="button" data-limit-trigger="" data-state="open">
          700.00
        </button>
      </div>
    </CellShell>,
  )
  const cell = screen.getByTestId('cell')
  await user.hover(cell)
  tick(500)
  expect(screen.queryByTestId('comment-preview')).toBeNull()
  await user.unhover(cell)
  await user.hover(cell)
  tick(500)
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('never previews from keyboard focus', () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  render(
    <CellShell title="Groceries" comments={three}>
      <div data-testid="cell" data-comment-anchor="">
        <button type="button">700.00</button>
      </div>
    </CellShell>,
  )
  act(() => screen.getByRole('button', { name: '700.00' }).focus())
  tick(500)
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

it('opens the menu from the keyboard context-menu key too', async () => {
  const cell = renderShell({ onOpenComments: vi.fn() })
  // Shift+F10 / the ContextMenu key fire `contextmenu` with no pointer position
  fireEvent.contextMenu(cell, { clientX: 0, clientY: 0 })
  expect(await screen.findByRole('menuitem', { name: 'Comments (3)' })).toBeInTheDocument()
})

it('hands focus back to what had it when the menu is dismissed without an action', async () => {
  const user = userEvent.setup()
  const onOpenComments = vi.fn()
  render(
    <CellShell title="Groceries" comments={three} onOpenComments={onOpenComments}>
      <div data-testid="cell" data-comment-anchor="">
        <button type="button">700.00</button>
      </div>
    </CellShell>,
  )
  const amount = screen.getByRole('button', { name: '700.00' })
  act(() => amount.focus())
  await user.pointer({ keys: '[MouseRight]', target: amount })
  await screen.findByRole('menu')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())
  await waitFor(() => expect(amount).toHaveFocus())
  expect(onOpenComments).not.toHaveBeenCalled()
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

it('mounts nothing but the cell itself while closed, and keeps the cell node through every open state', async () => {
  const user = previewClock()
  const { container } = render(
    <CellShell title="Groceries" comments={three} onOpenComments={vi.fn()} onShowTransactions={vi.fn()}>
      <div data-testid="cell" data-comment-anchor="">
        700.00
      </div>
    </CellShell>,
  )
  const cell = screen.getByTestId('cell')
  expect(container.childNodes).toHaveLength(1)
  expect(container.firstChild).toBe(cell)
  expect(document.body.childNodes).toHaveLength(1)

  await user.hover(cell)
  tick(300)
  expect(screen.getByTestId('comment-preview')).toBeInTheDocument()
  expect(screen.getByTestId('cell')).toBe(cell)

  await user.pointer({ keys: '[MouseRight]', target: cell })
  await screen.findByRole('menu')
  expect(screen.getByTestId('cell')).toBe(cell)
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())
  await user.unhover(cell)
  tick(100)
  await waitFor(() => expect(document.body.childNodes).toHaveLength(1))
  expect(container.firstChild).toBe(cell)
})

it('keeps the cell\'s own handlers and ref', async () => {
  const user = userEvent.setup()
  const ref = createRef<HTMLDivElement>()
  const onPointerDown = vi.fn()
  const onContextMenu = vi.fn()
  const onKeyDown = vi.fn()
  render(
    <CellShell title="Groceries" comments={three} onOpenComments={vi.fn()}>
      <div ref={ref} data-testid="cell" data-comment-anchor="" onPointerDown={onPointerDown} onContextMenu={onContextMenu} onKeyDown={onKeyDown}>
        <button type="button">700.00</button>
      </div>
    </CellShell>,
  )
  const cell = screen.getByTestId('cell')
  expect(ref.current).toBe(cell)
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(onPointerDown).toHaveBeenCalled()
  expect(onContextMenu).toHaveBeenCalled()
  expect(await screen.findByRole('menu')).toBeInTheDocument()
  fireEvent.keyDown(screen.getByRole('button', { name: '700.00', hidden: true }), { key: 'a' })
  expect(onKeyDown).toHaveBeenCalled()
})

describe('Shift+F2', () => {
  function renderKeyboardShell(props: Partial<Parameters<typeof CellShell>[0]>, onParentKeyDown = vi.fn()) {
    render(
      <div onKeyDown={onParentKeyDown}>
        <CellShell title="Groceries" comments={[]} {...props}>
          <div data-testid="cell" data-comment-anchor="">
            <button type="button">700.00</button>
          </div>
        </CellShell>
      </div>,
    )
    const amount = screen.getByRole('button', { name: '700.00' })
    act(() => amount.focus())
    return screen.getByTestId('cell')
  }

  it('opens the thread from a focused control inside the cell, and keeps the key from an enclosing grid', async () => {
    const user = userEvent.setup()
    const onOpenComments = vi.fn()
    const onParentKeyDown = vi.fn()
    const cell = renderKeyboardShell({ onOpenComments }, onParentKeyDown)
    await user.keyboard('{Shift>}{F2}{/Shift}')
    expect(onOpenComments).toHaveBeenCalledWith(cell)
    expect(onParentKeyDown.mock.calls.filter(([e]) => e.key === 'F2')).toHaveLength(0)
  })

  it('leaves the key alone where the cell has no thread or no menu', async () => {
    const user = userEvent.setup()
    const onParentKeyDown = vi.fn()
    renderKeyboardShell({}, onParentKeyDown)
    await user.keyboard('{Shift>}{F2}{/Shift}')
    expect(onParentKeyDown.mock.calls.filter(([e]) => e.key === 'F2')).toHaveLength(1)
  })

  it('does nothing when the menu is disabled (phones, edit mode)', async () => {
    const user = userEvent.setup()
    const onOpenComments = vi.fn()
    renderKeyboardShell({ onOpenComments, menuDisabled: true })
    await user.keyboard('{Shift>}{F2}{/Shift}')
    expect(onOpenComments).not.toHaveBeenCalled()
  })
})
