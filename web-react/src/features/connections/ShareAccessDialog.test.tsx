import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShareAccessDialog } from './ShareAccessDialog'
import { buildShareEntries } from './shared'
import type { ConnectionDto } from '@/api/dto/connection'

const me = { id: 'u1', avatar: 'https://a/me', name: 'Me' }
const partner = { id: 'u2', avatar: 'https://a/partner', name: 'Partner' }
const third = { id: 'u3', avatar: 'https://a/third', name: 'Third' }

const connections: ConnectionDto[] = [
  { user: partner, sharedAccounts: [] },
  { user: third, sharedAccounts: [] },
]

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('buildShareEntries seeds connections, overlays access, excludes me, marks the owner', () => {
  const entries = buildShareEntries(connections, [{ user: partner, role: 'user', isAccepted: 0 }], 'u1', 'u1')
  expect(entries).toEqual([
    { user: partner, role: 'user', isAccepted: false },
    { user: third, role: null, isAccepted: undefined },
  ])
  // the owner connection gets role 'owner'; I never appear in my own list
  const owned = buildShareEntries(connections, [], 'u1', 'u2')
  expect(owned.find((e) => e.user.id === 'u2')!.role).toBe('owner')
  expect(buildShareEntries([{ user: me, sharedAccounts: [] }, ...connections], [], 'u1', 'u1').some((e) => e.user.id === 'u1')).toBe(false)
})

it('renders role labels, no-access rows, and the budgets not-accepted suffix', () => {
  render(
    <ShareAccessDialog
      open
      title="Household"
      kind="budgets"
      entries={[
        { user: partner, role: 'user', isAccepted: false },
        { user: third, role: null, isAccepted: undefined },
      ]}
      onPick={() => {}}
      onClose={() => {}}
    />,
  )
  expect(screen.getByText('Select a user to share access with')).toBeInTheDocument()
  expect(screen.getByText(/Manage budget – invitation pending/)).toBeInTheDocument()
  expect(screen.getByText('No access')).toBeInTheDocument()
})

it('accounts kind has no not-accepted suffix and clicking a row fires onPick', async () => {
  const onPick = vi.fn()
  const user = userEvent.setup()
  render(
    <ShareAccessDialog
      open
      title="Wallet"
      kind="accounts"
      entries={[{ user: partner, role: 'user', isAccepted: undefined }]}
      onPick={onPick}
      onClose={() => {}}
    />,
  )
  expect(screen.getByText('Manage transactions')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /Partner/ }))
  expect(onPick).toHaveBeenCalledWith({ user: partner, role: 'user', isAccepted: undefined })
})

it('shows the empty note without entries', () => {
  render(<ShareAccessDialog open title="Wallet" kind="accounts" entries={[]} onPick={() => {}} onClose={() => {}} />)
  expect(screen.getByText('No connections found')).toBeInTheDocument()
})
