import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { recordPathname, resetNavTracking } from '@/lib/navigation'
import { SettingsShell } from './SettingsShell'

function mockCompactViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderShell() {
  const router = createMemoryRouter(
    [
      { path: '/', element: <div>MAIN SCREEN</div> },
      { path: '/somewhere-odd', element: <div>ODD PAGE</div> },
      {
        path: '/settings/profile',
        element: (
          <SettingsShell title="Profile" backTo="/settings">
            <div>PROFILE</div>
          </SettingsShell>
        ),
      },
      { path: '/settings', element: <div>SETTINGS HUB</div> },
    ],
    { initialEntries: ['/settings/profile'] },
  )
  render(<RouterProvider router={router} />)
}

beforeEach(() => {
  mockCompactViewport()
  resetNavTracking()
})

it('mobile back returns to the parent settings page when the user came from it', async () => {
  const user = userEvent.setup()
  recordPathname('/settings')
  recordPathname('/settings/profile')
  renderShell()
  await user.click(screen.getByRole('button', { name: 'back' }))
  expect(await screen.findByText('SETTINGS HUB')).toBeInTheDocument()
})

it('mobile back returns to the main screen when the user came from it', async () => {
  const user = userEvent.setup()
  recordPathname('/')
  recordPathname('/settings/profile')
  renderShell()
  await user.click(screen.getByRole('button', { name: 'back' }))
  expect(await screen.findByText('MAIN SCREEN')).toBeInTheDocument()
})

it('mobile back falls back to the main screen when the origin is any other page', async () => {
  const user = userEvent.setup()
  recordPathname('/somewhere-odd')
  recordPathname('/settings/profile')
  renderShell()
  await user.click(screen.getByRole('button', { name: 'back' }))
  expect(await screen.findByText('MAIN SCREEN')).toBeInTheDocument()
  expect(screen.queryByText('ODD PAGE')).not.toBeInTheDocument()
})

it('mobile back falls back to the main screen on a deep link (no origin)', async () => {
  const user = userEvent.setup()
  recordPathname('/settings/profile')
  renderShell()
  await user.click(screen.getByRole('button', { name: 'back' }))
  expect(await screen.findByText('MAIN SCREEN')).toBeInTheDocument()
})
