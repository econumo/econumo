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
    // never inherits Radix's pointer-events: none on <body>, or iOS's ^ / v keys
    // skip every field below the one focused as the page opens
    expect(content()?.style.pointerEvents).toBe('auto')
    act(() => {
      vv.height = 450
      vv.offsetTop = 120
      vv.dispatchEvent(new Event('resize'))
    })
    // iOS panned the layout viewport: the page follows it instead of sliding off the top
    expect(content()?.style.top).toBe('120px')
    expect(content()?.style.height).toBe('450px')
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

it('a fullScreen page holds still while a sheet is open over it', () => {
  mockMatchMedia(true)
  const vv = Object.assign(new EventTarget(), { height: 450, offsetTop: 0 })
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  const page = (sheetOpen: boolean) => (
    <ResponsiveDialog open fullScreen onOpenChange={() => {}} title="Page">
      <ResponsiveDialog open={sheetOpen} onOpenChange={() => {}} title="Sheet">
        <p>sheet</p>
      </ResponsiveDialog>
    </ResponsiveDialog>
  )
  try {
    const { baseElement, rerender } = render(page(false))
    const content = () => baseElement.querySelector<HTMLElement>('[data-slot="dialog-content"]')
    expect(content()?.style.height).toBe('450px')

    rerender(page(true))
    // the sheet's own keyboard (and iOS's pan for it) moves the sheet, not the page
    act(() => {
      vv.height = 400
      vv.offsetTop = 300
      vv.dispatchEvent(new Event('resize'))
    })
    expect(content()?.style.height).toBe('450px')
    expect(content()?.style.top).toBe('0px')

    // the keyboard hidden inside the sheet: the page is full height again
    // already, so closing the sheet later changes nothing
    act(() => {
      vv.height = 800
      vv.offsetTop = 0
      vv.dispatchEvent(new Event('resize'))
    })
    expect(content()?.style.height).toBe('')
  } finally {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  }
})

it('fullScreen on mobile scrolls the focused field back into view when the page shrinks for the keyboard', async () => {
  mockMatchMedia(true)
  const vv = Object.assign(new EventTarget(), { height: 800, offsetTop: 0 })
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  try {
    const { baseElement } = render(
      <ResponsiveDialog open fullScreen onOpenChange={() => {}} title="Page">
        <textarea aria-label="Notes" />
      </ResponsiveDialog>,
    )
    const body = baseElement.querySelector<HTMLElement>('[data-slot="dialog-body"]')!
    const notes = screen.getByRole('textbox', { name: 'Notes' })
    notes.focus()
    // tapped on the full-height page it was in view; above the keyboard the form ends at 300
    body.getBoundingClientRect = () => ({ top: 100, bottom: 300 }) as DOMRect
    notes.getBoundingClientRect = () => ({ top: 400, bottom: 460 }) as DOMRect
    act(() => {
      vv.height = 450
      vv.dispatchEvent(new Event('resize'))
    })
    await act(() => new Promise((resolve) => setTimeout(resolve, 150)))
    // 460 - 300 plus the 8px gap
    expect(body.scrollTop).toBe(168)
  } finally {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  }
})

it('a fullScreen page stays held after a sheet closes until its keyboard is down', () => {
  mockMatchMedia(true)
  const vv = Object.assign(new EventTarget(), { height: 800, offsetTop: 0 })
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  const page = (sheetOpen: boolean) => (
    <ResponsiveDialog open fullScreen onOpenChange={() => {}} title="Page">
      <ResponsiveDialog open={sheetOpen} onOpenChange={() => {}} title="Sheet">
        <p>sheet</p>
      </ResponsiveDialog>
    </ResponsiveDialog>
  )
  const keyboard = (height: number, offsetTop = 0) =>
    act(() => {
      vv.height = height
      vv.offsetTop = offsetTop
      vv.dispatchEvent(new Event('resize'))
    })
  try {
    const { baseElement, rerender } = render(page(true))
    const content = () => baseElement.querySelector<HTMLElement>('[data-slot="dialog-content"]')
    keyboard(400, 300)
    rerender(page(false))
    // the closed sheet's keyboard is still going away, panned for its search
    keyboard(420, 200)
    expect(content()?.style.height).toBe('')
    keyboard(800)
    expect(content()?.style.height).toBe('')
  } finally {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  }
})

it('a sheet closed with its keyboard already down leaves the page free to fit the next keyboard', () => {
  mockMatchMedia(true)
  const vv = Object.assign(new EventTarget(), { height: 450, offsetTop: 0 })
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  const page = (sheetOpen: boolean) => (
    <ResponsiveDialog open fullScreen onOpenChange={() => {}} title="Page">
      <ResponsiveDialog open={sheetOpen} onOpenChange={() => {}} title="Sheet">
        <p>sheet</p>
      </ResponsiveDialog>
    </ResponsiveDialog>
  )
  const keyboard = (height: number) =>
    act(() => {
      vv.height = height
      vv.dispatchEvent(new Event('resize'))
    })
  try {
    // fitted to a page field's keyboard, then a sheet opens and its keyboard is hidden
    const { baseElement, rerender } = render(page(false))
    const content = () => baseElement.querySelector<HTMLElement>('[data-slot="dialog-content"]')
    expect(content()?.style.height).toBe('450px')
    rerender(page(true))
    keyboard(800)
    rerender(page(false))
    // a page field raises a keyboard of another height: the page fits it, not the stale 450
    keyboard(380)
    expect(content()?.style.height).toBe('380px')
  } finally {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  }
})

it('focus a closing sheet hands back to a button keeps the page held; a text field releases it', () => {
  mockMatchMedia(true)
  const vv = Object.assign(new EventTarget(), { height: 800, offsetTop: 0 })
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  const page = (sheetOpen: boolean) => (
    <ResponsiveDialog open fullScreen onOpenChange={() => {}} title="Page">
      <button type="button">Currency</button>
      <textarea aria-label="Notes" />
      <ResponsiveDialog open={sheetOpen} onOpenChange={() => {}} title="Sheet">
        <p>sheet</p>
      </ResponsiveDialog>
    </ResponsiveDialog>
  )
  const keyboard = (height: number, offsetTop = 0) =>
    act(() => {
      vv.height = height
      vv.offsetTop = offsetTop
      vv.dispatchEvent(new Event('resize'))
    })
  try {
    const { baseElement, rerender } = render(page(true))
    const content = () => baseElement.querySelector<HTMLElement>('[data-slot="dialog-content"]')
    keyboard(400, 300)
    rerender(page(false))
    act(() => screen.getByRole('button', { name: 'Currency' }).focus())
    keyboard(420, 200)
    expect(content()?.style.height).toBe('')

    act(() => screen.getByRole('textbox', { name: 'Notes' }).focus())
    keyboard(430)
    expect(content()?.style.height).toBe('430px')
  } finally {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  }
})
