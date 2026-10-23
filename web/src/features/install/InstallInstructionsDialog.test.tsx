import { render, screen } from '@testing-library/react'
import { InstallInstructionsDialog } from './InstallInstructionsDialog'

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it.each([
  ['ios' as const, /Add to Home Screen”/, /from its Home Screen icon/],
  ['macos_safari' as const, /Add to Dock”/, /from the Dock instead of Safari/],
  ['android_firefox' as const, /Choose “Install”/, /from its Home Screen icon/],
])('shows the %s steps', (mode, step, hint) => {
  render(<InstallInstructionsDialog mode={mode} open onClose={vi.fn()} />)
  expect(screen.getByText(step)).toBeInTheDocument()
  expect(screen.getByText(hint)).toBeInTheDocument()
})
