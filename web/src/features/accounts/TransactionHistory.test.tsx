import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { DailyListEntry } from '@/features/transactions/useAccountTransactions'
import { fixtureTransactions } from '@/test/fixtures'
import { TransactionHistory } from './TransactionHistory'

function history(): DailyListEntry[] {
  // Ten years of history: each month has enough transactions that the oldest
  // month is well outside the initial rendered section.
  return Array.from({ length: 120 }, (_, index) => {
    const date = new Date(2026, 9 - index, 15)
    const day = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-15`
    return [
      { kind: 'separator' as const, day, label: 'date' as const },
      ...Array.from({ length: 10 }, (_, row) => ({
        kind: 'transaction' as const,
        transaction: { ...fixtureTransactions[0], type: 'expense' as const, id: `${day}-${row}`, date: `${day} 12:00:00`, isInFuture: false, labelIds: [], recurringId: null, isImported: 0 as const },
      })),
    ]
  }).flat()
}

function renderHistory(entries = history(), anchorIndex = 0) {
  return render(
    <TransactionHistory entries={entries} anchorIndex={anchorIndex} ready>
      {(entry) => entry.kind === 'separator'
        ? <span>{entry.day}</span>
        : <span data-testid={`row-${entry.transaction.id}`}>{entry.transaction.date}</span>}
    </TransactionHistory>,
  )
}

function mockTrackGeometry(marker: HTMLElement, navigation: HTMLElement) {
  vi.spyOn(navigation, 'getBoundingClientRect').mockReturnValue({ left: 0, right: 600, width: 600 } as DOMRect)
  vi.spyOn(marker, 'getBoundingClientRect').mockImplementation(() => {
    const percent = Number(/([\d.]+)%/.exec(marker.style.left)?.[1] ?? 0)
    const left = percent / 100 * 560
    return { left, right: left + 40, width: 40 } as DOMRect
  })
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('jumps across years directly, then returns to today without mounting intervening history', async () => {
  const user = userEvent.setup()
  renderHistory()
  const slider = screen.getByRole('slider', { name: 'Transaction timeline' })
  expect(slider).toHaveAttribute('aria-valuetext', 'October 2026')
  expect(screen.queryByTestId('row-2016-11-15-0')).not.toBeInTheDocument()
  slider.focus()
  await user.keyboard('{End}')
  expect(slider).toHaveAttribute('aria-valuetext', 'November 2016')
  expect(screen.getByTestId('row-2016-11-15-0')).toBeInTheDocument()
  expect(screen.queryByTestId('row-2026-10-15-0')).not.toBeInTheDocument()
  expect(document.querySelectorAll('[data-history-index]').length).toBeLessThanOrEqual(200)

  await user.click(screen.getByRole('button', { name: 'Jump to month' }))
  await user.click(screen.getByRole('button', { name: 'Back to today' }))
  expect(slider).toHaveAttribute('aria-valuetext', 'October 2026')
  expect(screen.getByTestId('row-2026-10-15-0')).toBeInTheDocument()
  expect(screen.queryByTestId('row-2016-11-15-0')).not.toBeInTheDocument()
})

it('selects a year and exact month, with unavailable months disabled', async () => {
  const user = userEvent.setup()
  const entries = history().filter((entry) => entry.kind === 'separator'
    ? entry.day.startsWith('2026-10') || entry.day.startsWith('2020-05')
    : entry.transaction.date.startsWith('2026-10') || entry.transaction.date.startsWith('2020-05'))
  renderHistory(entries)
  await user.click(screen.getByRole('button', { name: 'Jump to month' }))
  const picker = screen.getByRole('dialog', { name: 'Jump to month' })
  await user.selectOptions(within(picker).getByRole('combobox', { name: 'Year' }), '2020')
  expect(within(picker).getByRole('button', { name: 'June 2020' })).toBeDisabled()
  await user.click(within(picker).getByRole('button', { name: 'May 2020' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(screen.getByRole('button', { name: 'Jump to month' })).toHaveTextContent('May 2020')
})

it('opens the picker from the coin marker using the keyboard', async () => {
  const user = userEvent.setup()
  renderHistory()
  screen.getByRole('button', { name: 'Jump to month' }).focus()
  await user.keyboard('{Enter}')
  const picker = screen.getByRole('dialog', { name: 'Jump to month' })
  expect(within(picker).getByRole('combobox', { name: 'Year' })).toHaveValue('2026')
  expect(within(picker).getByRole('button', { name: 'October 2026' })).toHaveAttribute('aria-current', 'date')
})

function mockPhone(phone = true) {
  vi.stubGlobal('matchMedia', vi.fn((query: string) => ({
    matches: phone && query === '(max-width: 639px)', media: query,
    addEventListener: vi.fn(), removeEventListener: vi.fn(),
  })))
}

beforeEach(() => mockPhone(false))

it('on a phone, tapping the coins opens a month/year sheet and selects old history', async () => {
  mockPhone()
  const user = userEvent.setup()
  renderHistory()
  const marker = screen.getByRole('button', { name: 'Jump to month' })
  await user.click(marker)
  const picker = screen.getByRole('dialog', { name: 'Jump to month' })
  expect(picker).toHaveAttribute('data-slot', 'drawer-content')
  expect(marker).toHaveAttribute('aria-expanded', 'true')
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  await user.selectOptions(within(picker).getByRole('combobox', { name: 'Year' }), '2020')
  await user.click(within(picker).getByRole('button', { name: 'May 2020' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(marker).toHaveTextContent('May 2020')
  expect(screen.getByTestId('row-2020-05-15-0')).toBeInTheDocument()
  await waitFor(() => expect(marker).toHaveFocus())
})

it('on a phone, Cancel and Escape leave the timeline unchanged, and Back to today returns to current history', async () => {
  mockPhone()
  const user = userEvent.setup()
  renderHistory()
  const marker = screen.getByRole('button', { name: 'Jump to month' })
  for (const dismiss of ['Cancel', 'Escape']) {
    await user.click(marker)
    const picker = screen.getByRole('dialog', { name: 'Jump to month' })
    await user.selectOptions(within(picker).getByRole('combobox', { name: 'Year' }), '2020')
    if (dismiss === 'Cancel') await user.click(within(picker).getByRole('button', { name: 'Cancel' }))
    else await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(marker).toHaveTextContent('October 2026')
    await waitFor(() => expect(marker).toHaveFocus())
  }
  screen.getByRole('slider').focus()
  await user.keyboard('{End}')
  await user.click(marker)
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Back to today' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(marker).toHaveTextContent('October 2026')
})

it.each([false, true])('shows the month only while dragging the coins, never on hover (phone: %s)', async (phone) => {
  if (phone) mockPhone()
  const user = userEvent.setup()
  renderHistory()
  const marker = screen.getByRole('button', { name: 'Jump to month' })
  const navigation = screen.getByRole('navigation', { name: 'Transaction timeline' })
  await user.hover(marker)
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  mockTrackGeometry(marker, navigation)
  fireEvent.pointerDown(marker, { button: 0, clientX: 20, pointerId: 1 })
  fireEvent.pointerMove(navigation, { clientX: 318.2, pointerId: 1 })
  expect(await screen.findByRole('tooltip')).toHaveTextContent('July 2021')
  fireEvent.pointerUp(navigation, { clientX: 318.2, pointerId: 1 })
  await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument())
})

it('opens the picker only for a press that starts and ends on the coins without dragging', async () => {
  const user = userEvent.setup()
  renderHistory()
  const marker = screen.getByRole('button', { name: 'Jump to month' })
  const navigation = screen.getByRole('navigation', { name: 'Transaction timeline' })
  // A pointer click the press handlers never saw, e.g. one a browser routes
  // to the coins after a drag ended over them.
  fireEvent.click(marker, { detail: 1 })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  mockTrackGeometry(marker, navigation)
  fireEvent.pointerDown(navigation, { button: 0, clientX: 300, pointerId: 1 })
  fireEvent.pointerUp(marker, { clientX: 300, pointerId: 1 })
  fireEvent.click(marker, { detail: 1 })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await user.click(marker)
  expect(screen.getByRole('dialog', { name: 'Jump to month' })).toBeInTheDocument()
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
})

it.each([false, true])('drags the coins without opening the picker on a phone: %s', (phone) => {
  if (phone) mockPhone()
  renderHistory()
  const label = screen.getByRole('button', { name: 'Jump to month' })
  const navigation = screen.getByRole('navigation', { name: 'Transaction timeline' })
  const slider = screen.getByRole('slider')
  expect(slider).toHaveAttribute('aria-orientation', 'horizontal')
  mockTrackGeometry(label, navigation)
  fireEvent.pointerDown(label, { button: 0, clientX: 20, pointerId: 1 })
  fireEvent.pointerMove(navigation, { clientX: 580, pointerId: 1 })
  expect(label).toHaveTextContent('November 2016')
  expect(screen.queryByTestId('row-2016-11-15-0')).not.toBeInTheDocument()
  fireEvent.pointerUp(navigation, { clientX: 580, pointerId: 1 })
  fireEvent.click(label, { detail: 1 })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByTestId('row-2016-11-15-0')).toBeInTheDocument()
  expect(slider).toHaveAttribute('aria-valuetext', 'November 2016')

  fireEvent.pointerDown(label, { button: 0, clientX: 580, pointerId: 2 })
  fireEvent.pointerMove(navigation, { clientX: 20, pointerId: 2 })
  fireEvent.pointerUp(navigation, { clientX: 20, pointerId: 2 })
  expect(screen.getByTestId('row-2026-10-15-0')).toBeInTheDocument()
  expect(slider).toHaveAttribute('aria-valuetext', 'October 2026')
})

it('tracks the visible month during normal scrolling', async () => {
  renderHistory()
  const container = screen.getByTestId('transaction-history')
  Object.defineProperty(container, 'clientHeight', { configurable: true, value: 300 })
  const rows = Array.from(container.querySelectorAll<HTMLElement>('[data-history-index]'))
  rows.forEach((row, index) => {
    vi.spyOn(row, 'getBoundingClientRect').mockImplementation(() => ({
      top: index * 20 - container.scrollTop,
      bottom: (index + 1) * 20 - container.scrollTop,
    } as DOMRect))
  })
  container.scrollTop = 22 * 20 // two months back
  fireEvent.scroll(container)
  await waitFor(() => expect(screen.getByRole('slider')).toHaveAttribute('aria-valuetext', 'August 2026'))
})

it('keeps every pointer position, including two different positions within the same month', () => {
  renderHistory()
  const marker = screen.getByRole('button', { name: 'Jump to month' })
  const navigation = screen.getByRole('navigation', { name: 'Transaction timeline' })
  mockTrackGeometry(marker, navigation)
  fireEvent.pointerDown(marker, { button: 0, clientX: 20, pointerId: 1 })
  fireEvent.pointerMove(navigation, { clientX: 318.2, pointerId: 1 })
  expect(marker).toHaveStyle({ left: 'calc(53.25% - 21.3px)' })
  const previous = marker.style.left
  fireEvent.pointerMove(navigation, { clientX: 316.2, pointerId: 1 })
  expect(marker.style.left).not.toBe(previous)
  expect(marker).toHaveTextContent('July 2021')
  const dropped = marker.style.left
  fireEvent.pointerUp(navigation, { clientX: 316.2, pointerId: 1 })
  expect(marker.style.left).toBe(dropped)
  expect(screen.getByTestId('row-2021-07-15-0')).toBeInTheDocument()
})

it('keeps the whole marker inside the divider at the right edge, including an off-center grab', () => {
  renderHistory()
  const marker = screen.getByRole('button', { name: 'Jump to month' })
  const navigation = screen.getByRole('navigation', { name: 'Transaction timeline' })
  mockTrackGeometry(marker, navigation)
  fireEvent.pointerDown(marker, { button: 0, clientX: 5, pointerId: 1 })
  fireEvent.pointerMove(navigation, { clientX: 700, pointerId: 1 })
  fireEvent.pointerUp(navigation, { clientX: 700, pointerId: 1 })
  expect(marker).toHaveStyle({ left: 'calc(100% - 40px)' })
  expect(marker.getBoundingClientRect().right).toBe(navigation.getBoundingClientRect().right)
  expect(marker).toBeVisible()
  expect(screen.getByRole('slider')).toHaveAttribute('aria-valuenow', '100')
  expect(screen.getByTestId('row-2016-11-15-0')).toBeInTheDocument()
})

it('aligns a sparse oldest month at the top instead of letting the browser clamp the jump', async () => {
  const user = userEvent.setup()
  const entries = history().slice(0, 221)
  entries.push(
    { kind: 'separator', day: '2010-01-15', label: 'date' },
    { kind: 'transaction', transaction: { ...fixtureTransactions[0], id: 'oldest', type: 'expense', date: '2010-01-15 12:00:00', isInFuture: false, labelIds: [], recurringId: null, isImported: 0 } },
  )
  renderHistory(entries)
  const container = screen.getByTestId('transaction-history')
  Object.defineProperty(container, 'clientHeight', { configurable: true, value: 300 })
  let scrollTop = 0
  Object.defineProperty(container, 'scrollTop', {
    configurable: true,
    get: () => scrollTop,
    set: (value: number) => {
      const padding = container.querySelector<HTMLElement>('[data-history-trailing-space]')
      const height = container.querySelectorAll('[data-history-index]').length * 20 + Number.parseFloat(padding?.style.height || '0') + 16
      scrollTop = Math.max(0, Math.min(value, height - 300))
    },
  })
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this === container) return { top: 0, bottom: 300 } as DOMRect
    const index = Array.from(container.querySelectorAll('[data-history-index]')).indexOf(this)
    return { top: index * 20 - scrollTop, bottom: (index + 1) * 20 - scrollTop } as DOMRect
  })
  screen.getByRole('slider').focus()
  await user.keyboard('{End}')
  const heading = container.querySelector<HTMLElement>('[data-history-index="221"]')!
  expect(heading.getBoundingClientRect().top).toBe(0)
  expect(screen.getByRole('slider')).toHaveAttribute('aria-valuetext', 'January 2010')
})

it('loads in both directions after a jump and preserves the visible row when prepending', async () => {
  const observers: { callback: IntersectionObserverCallback; targets: Element[] }[] = []
  vi.stubGlobal('IntersectionObserver', class {
    targets: Element[] = []
    constructor(callback: IntersectionObserverCallback) { observers.push({ callback, targets: this.targets }) }
    observe(target: Element) { this.targets.push(target) }
    disconnect() {}
  })
  const user = userEvent.setup()
  renderHistory()
  const container = screen.getByTestId('transaction-history')
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this === container) return { top: 0, bottom: 300 } as DOMRect
    const rows = Array.from(container.querySelectorAll('[data-history-index]'))
    const index = rows.indexOf(this)
    return { top: index * 20 - container.scrollTop, bottom: (index + 1) * 20 - container.scrollTop } as DOMRect
  })
  const slider = screen.getByRole('slider')
  slider.focus()
  await user.keyboard('{PageDown}{PageDown}')
  expect(slider).toHaveAttribute('aria-valuetext', 'October 2024')
  const before = container.firstElementChild!
  expect(before).toHaveAttribute('data-history-boundary', 'before')
  const heldRow = container.querySelector<HTMLElement>('[data-history-index="264"]')!
  const top = heldRow.getBoundingClientRect().top
  const observer = observers.at(-1)!
  act(() => observer.callback([{ isIntersecting: true, target: before }] as IntersectionObserverEntry[], {} as IntersectionObserver))
  expect(heldRow.getBoundingClientRect().top).toBe(top)
  expect(container.querySelector('[data-history-index="64"]')).toBeInTheDocument()
  const after = container.lastElementChild!
  act(() => observers.at(-1)!.callback([{ isIntersecting: true, target: after }] as IntersectionObserverEntry[], {} as IntersectionObserver))
  expect(container.querySelector('[data-history-index="463"]')).toBeInTheDocument()
})

it('starts near today even with a large future block and returns there from older history', async () => {
  const user = userEvent.setup()
  renderHistory(history(), 500)
  expect(screen.getByTestId('future-anchor')).toBeInTheDocument()
  expect(document.querySelector('[data-history-index="0"]')).not.toBeInTheDocument()
  expect(document.querySelectorAll('[data-history-index]').length).toBeLessThanOrEqual(200)
  const month = screen.getByRole('slider').getAttribute('aria-valuetext')
  screen.getByRole('slider').focus()
  await user.keyboard('{End}')
  await user.click(screen.getByRole('button', { name: 'Jump to month' }))
  await user.click(screen.getByRole('button', { name: 'Back to today' }))
  expect(screen.getByRole('slider')).toHaveAttribute('aria-valuetext', month)
  expect(document.querySelector('[data-history-index="500"]')).toBeInTheDocument()
})

it('shows an empty state without a navigator when there are no matches', () => {
  renderHistory([])
  expect(screen.getByText('No transactions found')).toBeInTheDocument()
  expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
})
