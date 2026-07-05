import { render, screen } from '@testing-library/react'
import { LoadingDialog } from './LoadingDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

beforeEach(mockMatchMedia)

it('shows three coins with staggered delays and a screen-reader-only label', () => {
  render(<LoadingDialog open label="Loading details" />)
  expect(screen.getByRole('status', { name: 'Loading details' })).toBeInTheDocument()
  // the header is hidden visually but the title stays in the a11y tree
  expect(screen.getByText('Loading details').closest('[data-slot="dialog-header"]')).toHaveClass('sr-only')
  const coins = document.querySelectorAll('.coin-loader-coin')
  expect(coins).toHaveLength(3)
  const delays = Array.from(document.querySelectorAll('.coin-loader-unit')).map(
    (el) => (el as HTMLElement).style.getPropertyValue('--coin-delay'),
  )
  expect(delays).toEqual(['0s', '0.16s', '0.32s'])
})

it('renders nothing when closed', () => {
  render(<LoadingDialog open={false} label="Loading details" />)
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})
