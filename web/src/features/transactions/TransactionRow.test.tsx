import { cleanup, render, screen } from '@testing-library/react'
import type { AccountDto } from '@/api/dto/account'
import { fixtureOwner } from '@/test/fixtures'
import { TransactionRow } from './TransactionRow'
import type { ViewTransaction } from './useAccountTransactions'

const pageAccount = {
  id: 'a1',
  owner: fixtureOwner,
  folderId: null,
  name: 'Cash',
  position: 0,
  currency: { id: 'usd', code: 'USD', symbol: '$', fractionDigits: 2 },
  balance: '100',
  type: 1,
  icon: 'wallet',
  sharedAccess: [],
} as unknown as AccountDto

const baseTx = {
  id: 't1',
  author: fixtureOwner,
  type: 'expense',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '9.99',
  amountRecipient: '9.99',
  categoryId: 'cat1',
  description: '',
  payeeId: null,
  tagId: null,
  date: '2026-07-02 09:30:00',
  account: pageAccount,
  category: { id: 'cat1', name: 'Groceries', icon: 'fastfood' },
  isInFuture: false,
  recurringId: null,
} as unknown as ViewTransaction

function renderRow(overrides: Partial<ViewTransaction> = {}) {
  render(<TransactionRow transaction={{ ...baseTx, ...overrides } as ViewTransaction} pageAccount={pageAccount} />)
  return document.querySelector('[data-testid="tx-t1"]') as HTMLElement
}

const recurringIcon = () => screen.queryByLabelText('Recurring transaction')

it('shows no recurring icon on a hand-entered transaction', () => {
  renderRow()
  expect(recurringIcon()).toBeNull()
})

it('shows the recurring icon on a transaction posted from a template', () => {
  renderRow({ recurringId: 'r1' })
  expect(recurringIcon()).toBeInTheDocument()
})

it('shows the recurring icon on an unposted virtual row', () => {
  renderRow({ recurring: { id: 'r1' } as ViewTransaction['recurring'] })
  expect(recurringIcon()).toBeInTheDocument()
})

it('does NOT dim a posted transaction — it is settled money', () => {
  const row = renderRow({ recurringId: 'r1' })
  expect(row.className).not.toContain('opacity-50')
})

it('dims the unposted virtual row', () => {
  const row = renderRow({ recurring: { id: 'r1' } as ViewTransaction['recurring'] })
  expect(row.className).toContain('opacity-50')
})

it('renders the title and amount notes when provided, and neither by default', () => {
  render(
    <TransactionRow
      transaction={baseTx}
      pageAccount={pageAccount}
      titleNote="Monthly"
      amountNote={<span>2026-09-17</span>}
    />,
  )
  expect(screen.getByText('Monthly')).toBeInTheDocument()
  expect(screen.getByText('2026-09-17')).toBeInTheDocument()

  cleanup()
  renderRow()
  expect(screen.queryByText('Monthly')).toBeNull()
  expect(screen.queryByText('2026-09-17')).toBeNull()
})

it('renders the icon after the name, and the name still truncates', () => {
  renderRow({ recurringId: 'r1', category: { id: 'cat1', name: 'Groceries', icon: 'fastfood' } as ViewTransaction['category'] })
  const name = screen.getByTitle('Groceries')
  expect(name.className).toContain('truncate')
  const icon = recurringIcon()
  // DOCUMENT_POSITION_FOLLOWING: the icon comes after the name in document order
  expect(name.compareDocumentPosition(icon!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('shows reporting tags beside the budget tag', () => {
  const row = renderRow({
    tag: { id: 'tg1', name: 'Italy 2026', icon: 'tag' },
    labels: [
      { id: 'lb1', name: 'Kitty', icon: 'label' },
      { id: 'lb2', name: 'Doggo', icon: 'label' },
    ],
  } as Partial<ViewTransaction>)
  expect(row).toHaveTextContent('Italy 2026')
  expect(row).toHaveTextContent('Kitty')
  expect(row).toHaveTextContent('Doggo')
})

it('shows reporting tags even when the transaction has no budget tag', () => {
  const row = renderRow({ labels: [{ id: 'lb1', name: 'Kitty', icon: 'label' }] } as Partial<ViewTransaction>)
  expect(row).toHaveTextContent('Kitty')
})

it('marks an imported transaction with the import glyph', () => {
  renderRow({ isImported: 1 })
  expect(screen.getByLabelText('Imported')).toBeInTheDocument()
})

it('shows no import glyph on a hand-entered transaction', () => {
  renderRow({ isImported: 0 })
  expect(screen.queryByLabelText('Imported')).toBeNull()
})

it('never promotes a reporting tag to the row title', () => {
  // the title chain is category -> description -> budget tag -> payee; a
  // reporting tag stays a badge, so a title-less row reads "Uncategorized"
  const row = renderRow({
    category: undefined,
    labels: [{ id: 'lb1', name: 'Kitty', icon: 'label' }],
  } as Partial<ViewTransaction>)
  expect(row).toHaveTextContent('Uncategorized')
  expect(row.textContent?.match(/Kitty/g) ?? []).toHaveLength(1)
})
