import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PATTERN_FLASH_MS, PatternPad } from './PatternPad'
import { dotCentre, drawPattern, layOutGrid } from './testUtils'

const grid = () => screen.getByTestId('pattern-grid')
const drawn = () => grid().dataset.nodes

it('hands over a drawn pattern, adding a dot the line passes straight over', async () => {
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(<PatternPad onSubmit={onSubmit} />)
  // 0 -> 2 crosses 1; 2 -> 8 crosses 5.
  drawPattern(fireEvent, grid(), [0, 2, 8, 7])
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('p:0-1-2-5-8-7'))
  await waitFor(() => expect(drawn()).toBe(''))
})

it('does not add a crossed dot that is already part of the pattern', async () => {
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(<PatternPad onSubmit={onSubmit} />)
  drawPattern(fireEvent, grid(), [4, 0, 8, 7])
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('p:4-0-8-7'))
})

it('picks up a dot a fast swipe jumps over between two moves', async () => {
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(<PatternPad onSubmit={onSubmit} />)
  layOutGrid(grid())
  const init = { pointerId: 1, pointerType: 'touch' }
  fireEvent.pointerDown(grid(), { ...init, ...dotCentre(0) })
  // one event from dot 1 to an empty spot: the way passes over dot 5, which
  // is not in line with anything, so only sampling the path finds it
  fireEvent.pointerMove(grid(), { ...init, clientX: 270, clientY: 200 })
  fireEvent.pointerMove(grid(), { ...init, ...dotCentre(5) })
  fireEvent.pointerMove(grid(), { ...init, ...dotCentre(2) })
  fireEvent.pointerUp(grid(), { ...init, ...dotCentre(2) })
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('p:0-4-5-2'))
})

it('asks for at least 4 dots without submitting a shorter pattern', async () => {
  const onSubmit = vi.fn()
  render(<PatternPad onSubmit={onSubmit} hint="Draw it" />)
  expect(screen.getByText('Draw it')).toBeInTheDocument()
  drawPattern(fireEvent, grid(), [0, 1, 4])
  const message = await screen.findByText('Connect at least 4 dots')
  expect(message).toHaveAttribute('aria-live', 'assertive')
  expect(onSubmit).not.toHaveBeenCalled()
  await waitFor(() => expect(drawn()).toBe(''), { timeout: PATTERN_FLASH_MS + 500 })
})

it('flashes a rejected pattern red, then clears it', async () => {
  render(<PatternPad onSubmit={() => false} error="Wrong pattern" />)
  drawPattern(fireEvent, grid(), [0, 1, 2, 5])
  await waitFor(() => expect(grid()).toHaveClass('pin-shake'))
  expect(drawn()).toBe('0-1-2-5')
  expect(screen.getByText('Wrong pattern')).toHaveAttribute('aria-live', 'assertive')
  await waitFor(() => expect(drawn()).toBe(''), { timeout: PATTERN_FLASH_MS + 500 })
})

it('takes the dots one by one from the keyboard and submits on Done', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(<PatternPad onSubmit={onSubmit} />)
  expect(screen.queryByRole('button', { name: 'Done' })).not.toBeInTheDocument()
  screen.getByRole('button', { name: 'Dot 1' }).focus()
  await user.keyboard('{Enter}')
  expect(screen.getByRole('button', { name: 'Dot 1' })).toHaveAttribute('aria-pressed', 'true')
  expect(screen.getByRole('button', { name: 'Done' })).toBeDisabled()
  // Dot 1 -> Dot 9 crosses the centre, Dot 5.
  screen.getByRole('button', { name: 'Dot 9' }).focus()
  await user.keyboard('{Enter}')
  screen.getByRole('button', { name: 'Dot 6' }).focus()
  await user.keyboard(' ')
  expect(drawn()).toBe('0-4-8-5')
  await user.click(screen.getByRole('button', { name: 'Done' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('p:0-4-8-5'))
})

it('Clear drops a keyboard selection', async () => {
  const user = userEvent.setup()
  render(<PatternPad onSubmit={vi.fn()} />)
  screen.getByRole('button', { name: 'Dot 3' }).focus()
  await user.keyboard('{Enter}')
  await user.click(screen.getByRole('button', { name: 'Clear' }))
  expect(drawn()).toBe('')
  expect(screen.queryByRole('button', { name: 'Done' })).not.toBeInTheDocument()
})

it('single taps add up like keyboard picks', async () => {
  const onSubmit = vi.fn().mockResolvedValue(true)
  render(<PatternPad onSubmit={onSubmit} />)
  layOutGrid(grid())
  for (const node of [6, 7, 8, 5]) {
    const at = { pointerId: 1, pointerType: 'touch', ...dotCentre(node) }
    fireEvent.pointerDown(grid(), at)
    fireEvent.pointerUp(grid(), at)
  }
  expect(drawn()).toBe('6-7-8-5')
  expect(onSubmit).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Done' }))
  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('p:6-7-8-5'))
})

it('keeps the drawer from taking the stroke', () => {
  const onDrawerDown = vi.fn()
  render(
    <div onPointerDown={onDrawerDown}>
      <PatternPad onSubmit={vi.fn()} />
    </div>,
  )
  expect(grid()).toHaveAttribute('data-vaul-no-drag')
  expect(grid()).toHaveClass('touch-none')
  layOutGrid(grid())
  fireEvent.pointerDown(grid(), { pointerId: 1, ...dotCentre(0) })
  expect(onDrawerDown).not.toHaveBeenCalled()
})

it('ignores input while disabled', () => {
  const onSubmit = vi.fn()
  render(<PatternPad onSubmit={onSubmit} disabled />)
  drawPattern(fireEvent, grid(), [0, 1, 2, 5])
  expect(drawn()).toBe('')
  expect(screen.getByRole('button', { name: 'Dot 1' })).toBeDisabled()
  act(() => {})
  expect(onSubmit).not.toHaveBeenCalled()
})

it('renders the biometric button under the grid', () => {
  render(<PatternPad onSubmit={vi.fn()} biometric={<button type="button">Bio</button>} />)
  expect(screen.getByRole('button', { name: 'Bio' })).toBeInTheDocument()
})
