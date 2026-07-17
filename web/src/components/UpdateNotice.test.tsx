import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AvailableUpdate } from '@/hooks/useAvailableUpdate'
import { UpdateNotice } from './UpdateNotice'

const mockUpdate = vi.hoisted(() => ({ value: null as AvailableUpdate | null }))
vi.mock('@/hooks/useAvailableUpdate', () => ({
  useAvailableUpdate: () => mockUpdate.value,
}))

beforeEach(() => {
  localStorage.removeItem('econumo.dismissed-update-version')
  mockUpdate.value = { version: 'v9.9.9', url: 'https://econumo.com/releases/v9.9.9/' }
})

it('renders the release link when an update is available', () => {
  render(<UpdateNotice />)
  const link = screen.getByRole('link', { name: /v9\.9\.9/ })
  expect(link).toHaveAttribute('href', 'https://econumo.com/releases/v9.9.9/')
  expect(link).toHaveAttribute('target', '_blank')
})

it('renders nothing when no update is available', () => {
  mockUpdate.value = null
  const { container } = render(<UpdateNotice />)
  expect(container).toBeEmptyDOMElement()
})

it('dismisses per version and stays hidden', async () => {
  const user = userEvent.setup()
  render(<UpdateNotice />)
  await user.click(screen.getByRole('button', { name: /dismiss/i }))
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
  expect(localStorage.getItem('econumo.dismissed-update-version')).toBe('v9.9.9')
})

it('reappears for a newer version after a dismissal', () => {
  localStorage.setItem('econumo.dismissed-update-version', 'v9.9.8')
  render(<UpdateNotice />)
  expect(screen.getByRole('link', { name: /v9\.9\.9/ })).toBeInTheDocument()
})
