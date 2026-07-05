import { render, screen } from '@testing-library/react'
import { ResponsiveDialog } from './ResponsiveDialog'

function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

it('renders title and children as a dialog on desktop', () => {
  mockMatchMedia(false)
  render(
    <ResponsiveDialog open onOpenChange={() => {}} title="My title">
      <p>body text</p>
    </ResponsiveDialog>,
  )
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(screen.getByText('My title')).toBeInTheDocument()
  expect(screen.getByText('body text')).toBeInTheDocument()
})

it('renders as a drawer on mobile', () => {
  mockMatchMedia(true)
  render(
    <ResponsiveDialog open onOpenChange={() => {}} title="My title">
      <p>body text</p>
    </ResponsiveDialog>,
  )
  expect(screen.getByText('body text')).toBeInTheDocument()
})

it('fullScreen on mobile renders a full-viewport fading page, not a bottom sheet', () => {
  mockMatchMedia(true)
  const { baseElement } = render(
    <ResponsiveDialog open fullScreen onOpenChange={() => {}} title="My title">
      <p>body text</p>
    </ResponsiveDialog>,
  )
  expect(baseElement.querySelector('[data-slot="drawer-content"]')).toBeNull()
  const content = baseElement.querySelector('[data-slot="dialog-content"]')
  expect(content?.className).toContain('h-dvh')
  expect(content?.className).toContain('rounded-none')
})
