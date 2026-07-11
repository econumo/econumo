import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { SettingsShell } from './SettingsShell'

function mockCompactViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

it('mobile back always goes up to backTo, never through history', async () => {
  // A broken/odd history stack (restored tab, redirect chains) must not trap
  // the user: on compact viewports the chevron is the only way out, so it
  // navigates the hierarchy deterministically instead of replaying history.
  mockCompactViewport()
  const user = userEvent.setup()
  const router = createMemoryRouter(
    [
      { path: '/somewhere-odd', element: <div>ODD HISTORY ENTRY</div> },
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
    { initialEntries: ['/somewhere-odd', '/settings/profile'], initialIndex: 1 },
  )
  render(<RouterProvider router={router} />)

  await user.click(screen.getByRole('button', { name: 'back' }))
  expect(await screen.findByText('SETTINGS HUB')).toBeInTheDocument()
  expect(screen.queryByText('ODD HISTORY ENTRY')).not.toBeInTheDocument()
})
