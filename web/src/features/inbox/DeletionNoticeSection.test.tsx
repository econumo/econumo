import { render, screen } from '@testing-library/react'
import type { ConnectionDto } from '@/api/dto/connection'
import { DeletionNoticeSection } from './DeletionNoticeSection'

const connection = (over: Partial<ConnectionDto> & { name: string; id: string }): ConnectionDto => ({
  user: { id: over.id, avatar: 'pets:sky', name: over.name },
  accessLevel: 'full', accessUntil: '', createdAt: '', sharedAccounts: [], deletionMarkedAt: '',
  ...over,
})

it('renders one row per connection marked for deletion, with the name and the notice', () => {
  render(
    <DeletionNoticeSection
      connections={[
        connection({ id: 'u2', name: 'Partner', deletionMarkedAt: '2026-11-20 09:30:15' }),
        connection({ id: 'u3', name: 'Friend' }),
        connection({ id: 'u4', name: 'Sibling', deletionMarkedAt: '2026-11-21 10:00:00' }),
      ]}
    />,
  )
  expect(screen.getAllByTestId('deletion-notice')).toHaveLength(2)
  expect(screen.getByText(/^Partner's data is scheduled for deletion\. The accounts and budgets they share with you will disappear, and transfers with their accounts will become corrections\.$/)).toBeInTheDocument()
  expect(screen.getByText(/^Sibling's data is scheduled for deletion\./)).toBeInTheDocument()
  expect(screen.queryByText(/Friend/)).toBeNull()
  expect(screen.queryByRole('button')).toBeNull()
})

it('renders nothing for an empty list', () => {
  const { container } = render(<DeletionNoticeSection connections={[]} />)
  expect(container).toBeEmptyDOMElement()
})
