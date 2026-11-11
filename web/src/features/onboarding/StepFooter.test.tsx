import { act, render, screen } from '@testing-library/react'
import { StepFooter } from './StepFooter'

it('pins the buttons to the bottom and shows its border only while content runs under it', () => {
  let report: (entries: { isIntersecting: boolean }[]) => void = () => undefined
  const observe = vi.fn()
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(cb: typeof report) {
        report = cb
      }
      observe = observe
      disconnect = vi.fn()
    },
  )
  render(
    <StepFooter>
      <button type="button">Skip</button>
      <button type="button">Save and continue</button>
    </StepFooter>,
  )
  const footer = screen.getByTestId('step-footer')
  expect(footer).toHaveClass('sticky', '-bottom-4')
  expect(footer).toContainElement(screen.getByRole('button', { name: 'Save and continue' }))
  expect(observe).toHaveBeenCalledTimes(1)
  expect(footer).not.toHaveAttribute('data-stuck')
  act(() => report([{ isIntersecting: false }]))
  expect(footer).toHaveAttribute('data-stuck', 'true')
  act(() => report([{ isIntersecting: true }]))
  expect(footer).not.toHaveAttribute('data-stuck')
  vi.unstubAllGlobals()
})
