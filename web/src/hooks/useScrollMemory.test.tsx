import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render } from '@testing-library/react'
import { useScrollMemory } from './useScrollMemory'

function Scroller({ memoryKey }: { memoryKey: string }) {
  const ref = useScrollMemory(memoryKey)
  return (
    <div data-testid={`scroller-${memoryKey}`} ref={ref}>
      <div style={{ height: 1000 }} />
    </div>
  )
}

const scrollTo = (el: HTMLElement, top: number) => {
  el.scrollTop = top
  el.dispatchEvent(new Event('scroll'))
}

const flushMutations = () => new Promise((resolve) => setTimeout(resolve, 0))

describe('useScrollMemory', () => {
  afterEach(() => {
    cleanup()
    document.body.removeAttribute('data-scroll-locked')
  })

  it('starts at the top when nothing was saved for the key', () => {
    const { getByTestId } = render(<Scroller memoryKey="fresh" />)
    expect(getByTestId('scroller-fresh').scrollTop).toBe(0)
  })

  it('restores the saved position when a container with the same key remounts', () => {
    const first = render(<Scroller memoryKey="remount" />)
    scrollTo(first.getByTestId('scroller-remount'), 400)
    first.unmount()

    const second = render(<Scroller memoryKey="remount" />)
    expect(second.getByTestId('scroller-remount').scrollTop).toBe(400)
  })

  it('keeps positions of different keys independent', () => {
    const first = render(<Scroller memoryKey="a" />)
    scrollTo(first.getByTestId('scroller-a'), 250)
    first.unmount()

    const second = render(<Scroller memoryKey="b" />)
    expect(second.getByTestId('scroller-b').scrollTop).toBe(0)
  })

  it('ignores scrolls while the body is scroll-locked and restores on release', async () => {
    const { getByTestId } = render(<Scroller memoryKey="locked" />)
    const el = getByTestId('scroller-locked')
    scrollTo(el, 300)

    // a modal opens (react-remove-scroll marks the body) and the platform
    // resets the background scroller — that reset must not become the saved spot
    document.body.setAttribute('data-scroll-locked', '1')
    scrollTo(el, 0)

    document.body.removeAttribute('data-scroll-locked')
    await flushMutations()
    expect(el.scrollTop).toBe(300)
  })
})
