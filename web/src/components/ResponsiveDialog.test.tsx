import { act, render, screen } from '@testing-library/react'
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
  // edge-to-edge viewport: the header clears the status bar
  expect(content?.querySelector('[data-slot="dialog-header"]')?.className).toContain('safe-area-inset-top')
})

it('bottom-reaching content clears the home indicator: body inset without a footer, footer inset with one', () => {
  mockMatchMedia(true)
  const { baseElement, rerender } = render(
    <ResponsiveDialog open onOpenChange={() => {}} title="My title">
      <p>body text</p>
    </ResponsiveDialog>,
  )
  const body = () => screen.getByText('body text').parentElement
  expect(body()?.className).toContain('safe-area-inset-bottom')
  rerender(
    <ResponsiveDialog open onOpenChange={() => {}} title="My title" footer={<button>ok</button>}>
      <p>body text</p>
    </ResponsiveDialog>,
  )
  expect(body()?.className).not.toContain('safe-area-inset-bottom')
  expect(baseElement.textContent).toContain('ok')
  expect(screen.getByRole('button', { name: 'ok' }).parentElement?.className).toContain('safe-area-inset-bottom')
})

it('size="wide" widens the desktop dialog instead of the default narrow width', () => {
  mockMatchMedia(false)
  const { baseElement } = render(
    <ResponsiveDialog open size="wide" onOpenChange={() => {}} title="My title">
      <p>body text</p>
    </ResponsiveDialog>,
  )
  const content = baseElement.querySelector('[data-slot="dialog-content"]')
  expect(content?.className).toContain('sm:max-w-2xl')
  expect(content?.className).not.toContain('sm:max-w-sm')
})

it('fullScreen on mobile fits the visible area above the on-screen keyboard', () => {
  mockMatchMedia(true)
  const vv = Object.assign(new EventTarget(), { height: 800, offsetTop: 0 })
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  try {
    const { baseElement } = render(
      <ResponsiveDialog open fullScreen onOpenChange={() => {}} title="My title" footer={<button>ok</button>}>
        <p>body text</p>
      </ResponsiveDialog>,
    )
    const content = () => baseElement.querySelector<HTMLElement>('[data-slot="dialog-content"]')
    expect(content()?.style.height).toBe('')
    act(() => {
      vv.height = 450
      vv.offsetTop = 120
      vv.dispatchEvent(new Event('resize'))
    })
    // iOS panned the layout viewport: the page follows it instead of sliding off the top
    expect(content()?.style.top).toBe('120px')
    expect(content()?.style.height).toBe('450px')
    // trailing room so any field can scroll to the top, its picker listing below it
    const body = baseElement.querySelector<HTMLElement>('[data-slot="dialog-body"]')
    expect(body?.style.paddingBottom).toBe('')
    expect((body?.lastElementChild as HTMLElement | null)?.style.height).toBe('450px')
    // the keyboard hides the home indicator, so the footer drops its inset
    expect(screen.getByText('ok').parentElement?.className).not.toContain('safe-area-inset-bottom')
  } finally {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  }
})
