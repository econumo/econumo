import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShareEntryList } from './ShareEntryList'
import type { ShareEntry } from './shared'

const entries: ShareEntry[] = [
  { user: { id: 'u2', name: 'Partner', avatar: 'pets:sky' }, role: 'admin' },
  { user: { id: 'u3', name: 'Newcomer', avatar: 'face:emerald' }, role: null },
]

it('renders role labels and fires onPick for tappable rows', async () => {
  const onPick = vi.fn()
  const user = userEvent.setup()
  render(<ShareEntryList kind="accounts" entries={entries} onPick={onPick} />)
  expect(screen.getByText('Full control')).toBeInTheDocument()
  expect(screen.getByText('No access')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /Partner/ }))
  expect(onPick).toHaveBeenCalledWith(entries[0])
})

it('renders read-only rows when onPick is omitted', () => {
  render(<ShareEntryList kind="accounts" entries={entries} />)
  expect(screen.getByText('Partner')).toBeInTheDocument()
  expect(screen.getByText('Newcomer')).toBeInTheDocument()
  expect(screen.queryByRole('button')).toBeNull()
})
