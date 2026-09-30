import { createRef } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
    <CellShell comments={three} {...props}>
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
// still ticks in real time (see the tablet press test below for why plain
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
    <CellShell comments={three}>
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
    <CellShell comments={three}>
      <div data-testid="cell" data-comment-anchor="">
        <button type="button">700.00</button>
      </div>
    </CellShell>,
  )
  act(() => screen.getByRole('button', { name: '700.00' }).focus())
  tick(500)
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('leaves a mouse right-click to the browser: no app menu, native menu not suppressed', async () => {
  const cell = renderShell({ onOpenComments: vi.fn() })
  // fireEvent returns false when a handler called preventDefault
  expect(fireEvent.contextMenu(cell)).toBe(true)
  await new Promise((r) => setTimeout(r, 100))
  expect(screen.queryByRole('menu')).toBeNull()
  expect(screen.queryByRole('dialog')).toBeNull()
})

it('on a tablet, a press held past half a second opens nothing and the tap still reaches the cell', async () => {
  tablet()
  // plain vi.useFakeTimers() deadlocks userEvent.pointer() in this jsdom/vitest
  // combination: something React's scheduler relies on never fires unless the
  // fake clock also ticks in real time
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    const onClick = vi.fn()
    render(
      <CellShell comments={three} onOpenComments={vi.fn()}>
        <button type="button" onClick={onClick}>cell</button>
      </CellShell>,
    )
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const cell = screen.getByRole('button', { name: 'cell' })
    await user.pointer({ keys: '[TouchA>]', target: cell })
    await vi.advanceTimersByTimeAsync(800)
    await user.pointer({ keys: '[/TouchA]', target: cell })
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByTestId('comment-preview')).toBeNull()
    expect(onClick).toHaveBeenCalledTimes(1)
  } finally {
    vi.useRealTimers()
  }
})

it('never remounts the cell: previewDisabled toggling and comments arriving/leaving keep the same DOM node (an open thread popover is anchored to it)', () => {
  const cellChild = (
    <div data-testid="cell" data-comment-anchor="">
      700.00
    </div>
  )
  const { rerender } = render(
    <CellShell comments={three} shortcutDisabled>
      {cellChild}
    </CellShell>,
  )
  const before = screen.getByTestId('cell')

  rerender(
    <CellShell comments={three} shortcutDisabled previewDisabled>
      {cellChild}
    </CellShell>,
  )
  expect(screen.getByTestId('cell')).toBe(before)

  rerender(
    <CellShell comments={[]} shortcutDisabled previewDisabled>
      {cellChild}
    </CellShell>,
  )
  expect(screen.getByTestId('cell')).toBe(before)

  rerender(
    <CellShell comments={[three[0]]} shortcutDisabled previewDisabled={false}>
      {cellChild}
    </CellShell>,
  )
  expect(screen.getByTestId('cell')).toBe(before)
})

it('mounts nothing but the cell itself while closed, and keeps the cell node through every open state', async () => {
  const user = previewClock()
  const { container } = render(
    <CellShell comments={three} onOpenComments={vi.fn()}>
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
    <CellShell comments={three} onOpenComments={vi.fn()}>
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
  fireEvent.keyDown(screen.getByRole('button', { name: '700.00' }), { key: 'a' })
  expect(onKeyDown).toHaveBeenCalled()
})

describe('Shift+F2', () => {
  function renderKeyboardShell(props: Partial<Parameters<typeof CellShell>[0]>, onParentKeyDown = vi.fn()) {
    render(
      <div onKeyDown={onParentKeyDown}>
        <CellShell comments={[]} {...props}>
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

  it('leaves the key alone where the cell has no thread', async () => {
    const user = userEvent.setup()
    const onParentKeyDown = vi.fn()
    renderKeyboardShell({}, onParentKeyDown)
    await user.keyboard('{Shift>}{F2}{/Shift}')
    expect(onParentKeyDown.mock.calls.filter(([e]) => e.key === 'F2')).toHaveLength(1)
  })

  it('does nothing when the shortcut is disabled (edit mode)', async () => {
    const user = userEvent.setup()
    const onOpenComments = vi.fn()
    renderKeyboardShell({ onOpenComments, shortcutDisabled: true })
    await user.keyboard('{Shift>}{F2}{/Shift}')
    expect(onOpenComments).not.toHaveBeenCalled()
  })
})
