import { act, render, screen } from '@testing-library/react'
import { onlineManager } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { OfflineNotice } from './OfflineNotice'

afterEach(() => act(() => onlineManager.setOnline(true)))

it('shows only while offline and goes away when the connection returns', () => {
  render(<OfflineNotice />)
  expect(screen.queryByTestId('offline-notice')).not.toBeInTheDocument()
  act(() => onlineManager.setOnline(false))
  expect(screen.getByTestId('offline-notice')).toHaveTextContent("No internet connection. Data is view-only.")
  act(() => onlineManager.setOnline(true))
  expect(screen.queryByTestId('offline-notice')).not.toBeInTheDocument()
})

function renderShell(editableOffline: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const router = createMemoryRouter(
    [{ path: '/', element: <SettingsShell title="Page" backTo="/settings" editableOffline={editableOffline}><div>BODY</div></SettingsShell> }],
    { initialEntries: ['/'] },
  )
  render(<RouterProvider router={router} />)
}

it('SettingsShell shows it offline unless the page stays editable offline', () => {
  act(() => onlineManager.setOnline(false))
  renderShell(false)
  expect(screen.getByTestId('offline-notice')).toBeInTheDocument()
})

it('SettingsShell leaves it out for a page that stays editable offline', () => {
  act(() => onlineManager.setOnline(false))
  renderShell(true)
  expect(screen.getByText('BODY')).toBeInTheDocument()
  expect(screen.queryByTestId('offline-notice')).not.toBeInTheDocument()
})
