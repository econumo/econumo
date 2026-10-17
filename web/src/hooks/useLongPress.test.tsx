import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useLongPress } from './useLongPress'

function Probe({ onClick, onLongPress }: { onClick: () => void; onLongPress: () => void }) {
  const press = useLongPress({ onClick, onLongPress })
  return <button {...press}>hold</button>
}

describe('useLongPress', () => {
  const onClick = vi.fn()
  const onLongPress = vi.fn()

  beforeEach(() => {
    vi.useFakeTimers()
    onClick.mockClear()
    onLongPress.mockClear()
    render(<Probe onClick={onClick} onLongPress={onLongPress} />)
  })

  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('a press released before the delay is a plain click', () => {
    const button = screen.getByRole('button')
    fireEvent.pointerDown(button)
    vi.advanceTimersByTime(300)
    fireEvent.pointerUp(button)
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
    expect(onLongPress).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1000)
    expect(onLongPress).not.toHaveBeenCalled()
  })

  it('a press held past the delay fires the long press and swallows the click that follows', () => {
    const button = screen.getByRole('button')
    fireEvent.pointerDown(button)
    vi.advanceTimersByTime(599)
    expect(onLongPress).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(onLongPress).toHaveBeenCalledTimes(1)
    fireEvent.pointerUp(button)
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
    expect(onLongPress).toHaveBeenCalledTimes(1)
  })

  it('a click that never follows a long press does not swallow the next tap', () => {
    const button = screen.getByRole('button')
    fireEvent.pointerDown(button)
    vi.advanceTimersByTime(600)
    fireEvent.pointerCancel(button)
    fireEvent.pointerDown(button)
    fireEvent.pointerUp(button)
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('leaving the button, or a cancelled pointer, abandons the press', () => {
    const button = screen.getByRole('button')
    fireEvent.pointerDown(button)
    vi.advanceTimersByTime(300)
    fireEvent.pointerLeave(button)
    vi.advanceTimersByTime(1000)
    fireEvent.pointerDown(button)
    vi.advanceTimersByTime(300)
    fireEvent.pointerCancel(button)
    vi.advanceTimersByTime(1000)
    expect(onClick).not.toHaveBeenCalled()
    expect(onLongPress).not.toHaveBeenCalled()
  })

  it('Shift+click is a long press', () => {
    fireEvent.click(screen.getByRole('button'), { shiftKey: true })
    expect(onLongPress).toHaveBeenCalledTimes(1)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('a keyboard click, with no pointer, is a plain click', () => {
    fireEvent.click(screen.getByRole('button'))
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('suppresses the context menu only while a press is active', () => {
    const button = screen.getByRole('button')
    expect(fireEvent.contextMenu(button)).toBe(true)
    fireEvent.pointerDown(button)
    expect(fireEvent.contextMenu(button)).toBe(false)
    fireEvent.pointerUp(button)
    expect(fireEvent.contextMenu(button)).toBe(true)
  })
})
