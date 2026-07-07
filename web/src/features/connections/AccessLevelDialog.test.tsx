import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AccessLevelDialog } from './AccessLevelDialog'
import { DeclineAccessDialog } from './DeclineAccessDialog'

const partner = { id: 'u2', avatar: 'https://avatars.test/partner', name: 'Partner' }

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('accounts kind renders the role options and hint, no revoke without a role', () => {
  render(
    <AccessLevelDialog open kind="accounts" user={partner} role={null} onSelect={() => {}} onRevoke={() => {}} onClose={() => {}} />,
  )
  expect(screen.getByText('Choose an access level to grant')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'View only' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Manage transactions' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Full control' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Revoke access' })).not.toBeInTheDocument()
})

it('existing role highlights, revoke row fires onRevoke, options fire onSelect', async () => {
  const onSelect = vi.fn()
  const onRevoke = vi.fn()
  const user = userEvent.setup()
  render(
    <AccessLevelDialog open kind="accounts" user={partner} role="user" onSelect={onSelect} onRevoke={onRevoke} onClose={() => {}} />,
  )
  expect(screen.getByRole('button', { name: 'Manage transactions' })).toHaveAttribute('aria-pressed', 'true')
  await user.click(screen.getByRole('button', { name: 'Full control' }))
  expect(onSelect).toHaveBeenCalledWith('admin')
  await user.click(screen.getByRole('button', { name: 'Revoke access' }))
  // revoking asks for confirmation first
  expect(onRevoke).not.toHaveBeenCalled()
  const confirm = await screen.findByRole('dialog', { name: 'Revoke access?' })
  await user.click(within(confirm).getByRole('button', { name: 'Revoke access' }))
  expect(onRevoke).toHaveBeenCalled()
})

it('budgets kind uses budget role labels and hides revoke for owner', () => {
  render(
    <AccessLevelDialog open kind="budgets" user={partner} role="owner" onSelect={() => {}} onRevoke={() => {}} onClose={() => {}} />,
  )
  expect(screen.getByRole('button', { name: 'Manage budget' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Revoke access' })).not.toBeInTheDocument()
})

it('decline dialog shows owner + item and fires onDecline', async () => {
  const onDecline = vi.fn()
  const user = userEvent.setup()
  render(<DeclineAccessDialog open owner={partner} itemName="Their cash" onDecline={onDecline} onClose={() => {}} />)
  expect(screen.getAllByText('Partner').length).toBeGreaterThan(0)
  expect(screen.getByText('Their cash')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Decline access' }))
  expect(onDecline).toHaveBeenCalled()
})
