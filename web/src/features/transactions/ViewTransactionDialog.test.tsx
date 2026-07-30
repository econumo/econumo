import { render, screen } from '@testing-library/react'
import { fixtureOwner } from '@/test/fixtures'
import type { ViewTransaction } from './useAccountTransactions'
import { ViewTransactionDialog } from './ViewTransactionDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const account = { id: 'a1', owner: fixtureOwner, folderId: null, name: 'Cash', position: 0, currency: { id: 'usd', code: 'USD', symbol: '$', fractionDigits: 2 }, balance: '0', type: 1, icon: 'wallet', sharedAccess: [] } as unknown as ViewTransaction['account']

const baseTx = {
  id: 't1',
  author: fixtureOwner,
  type: 'expense',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '9.99',
  amountRecipient: null,
  categoryId: null,
  description: '',
  payeeId: null,
  tagId: null,
  date: '2026-07-01 10:00:00',
  account,
  isInFuture: false,
} as unknown as ViewTransaction

beforeEach(() => {
  mockMatchMedia()
})

it('a categoryless non-transfer transaction shows Uncategorized as the hero name', () => {
  render(
    <ViewTransactionDialog
      transaction={baseTx}
      onClose={() => {}}
      onEdit={() => {}}
      onDelete={() => {}}
      canChange
      isShared={false}
    />,
  )
  expect(screen.getByText('Uncategorized')).toBeInTheDocument()
})
