import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { CurrentUserDto } from '@/api/dto/user'
import { WhatsNewNotice } from './WhatsNewNotice'

const mocks = vi.hoisted(() => ({
  user: undefined as Partial<CurrentUserDto> | undefined,
  version: 'v1.6.0',
  mutate: vi.fn(),
  trackEvent: vi.fn(),
}))

vi.mock('@/features/user/queries', () => ({
  useUserData: () => ({ data: mocks.user }),
  useUpdateLastSeenVersion: () => ({ mutate: mocks.mutate }),
}))
vi.mock('@/lib/config', () => ({ getVersion: () => mocks.version }))
vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: mocks.trackEvent,
}))

function seen(value: string | null) {
  mocks.user = { options: value === null ? [] : [{ name: 'last_seen_version', value }] } as Partial<CurrentUserDto>
}

beforeEach(() => {
  mocks.version = 'v1.6.0'
  mocks.mutate.mockReset()
  mocks.trackEvent.mockReset()
  seen('v1.5.0')
})

it('links to the release notes in a new window after an upgrade', () => {
  render(<WhatsNewNotice />)
  const link = screen.getByRole('link', { name: /v1\.6\.0/ })
  expect(link).toHaveAttribute('href', 'https://econumo.com/releases/v1.6.0')
  expect(link).toHaveAttribute('target', '_blank')
})

it('shows patch releases', () => {
  mocks.version = 'v1.6.1'
  seen('v1.6.0')
  render(<WhatsNewNotice />)
  expect(screen.getByRole('link', { name: /v1\.6\.1/ })).toBeInTheDocument()
})

it('shows for a user who predates the stored version', () => {
  seen(null)
  render(<WhatsNewNotice />)
  expect(screen.getByRole('link')).toBeInTheDocument()
})

it.each([
  ['the version was already seen', 'v1.6.0', 'v1.6.0'],
  ['the server was downgraded', 'v1.5.0', 'v1.6.0'],
  ['the build is not a release', 'dev', 'v1.5.0'],
])('renders nothing when %s', (_, version, stored) => {
  mocks.version = version
  seen(stored)
  const { container } = render(<WhatsNewNotice />)
  expect(container).toBeEmptyDOMElement()
})

it('renders nothing before the user loads', () => {
  mocks.user = undefined
  const { container } = render(<WhatsNewNotice />)
  expect(container).toBeEmptyDOMElement()
})

it('opening the notes marks the version seen and tracks the open', async () => {
  const user = userEvent.setup()
  render(<WhatsNewNotice />)
  await user.click(screen.getByRole('link'))
  expect(mocks.mutate).toHaveBeenCalledWith('v1.6.0')
  expect(mocks.trackEvent).toHaveBeenCalledWith('appWhatsNewOpen', { version: 'v1.6.0' })
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
})

it('dismissing marks the version seen and tracks the dismissal', async () => {
  const user = userEvent.setup()
  render(<WhatsNewNotice />)
  await user.click(screen.getByRole('button', { name: /dismiss/i }))
  expect(mocks.mutate).toHaveBeenCalledWith('v1.6.0')
  expect(mocks.trackEvent).toHaveBeenCalledWith('appWhatsNewDismiss', { version: 'v1.6.0' })
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
})
