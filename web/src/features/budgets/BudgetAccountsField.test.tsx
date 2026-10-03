import type { ReactElement } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { BudgetAccountsField, BudgetSavingsField } from './BudgetAccountsField'
import { openPicker } from './budgetFormTestUtils'

function renderField(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

beforeEach(() => {
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

const acc = (id: string, name: string, folderId: string | null = null) => ({ id, name, icon: 'wallet', folderId }) as any

describe('BudgetAccountsField', () => {
  const accounts = [acc('a1', 'Cash'), acc('a2', 'Savings')]

  it('is one picker row naming the included accounts, with the counter', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2'])} locked={new Set()} onToggle={vi.fn()} />)
    const row = screen.getByTestId('budget-accounts-field')
    expect(row).toHaveTextContent('Cash, Savings')
    expect(row).toHaveTextContent('2 of 2 included')
    // nothing to toggle until the picker opens
    expect(screen.queryByRole('switch')).toBeNull()
    expect(screen.queryByRole('option')).toBeNull()
  })

  it('reads None with nothing included', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set()} locked={new Set()} onToggle={vi.fn()} />)
    expect(screen.getByTestId('budget-accounts-field')).toHaveTextContent('None')
  })

  it('counts only members that have a row, ignoring deleted ones kept for round-tripping', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2', 'deleted-1'])} locked={new Set()} onToggle={vi.fn()} />)
    expect(screen.getByText('2 of 2 included')).toBeInTheDocument()
    expect(screen.queryByText('3 of 2 included')).toBeNull()
  })

  it('the picker toggles an account, keeps a locked member checked, and explains the lock', async () => {
    const onToggle = vi.fn()
    const user = userEvent.setup()
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1'])} locked={new Set(['a1'])} onToggle={onToggle} />)
    const picker = await openPicker(user, 'accounts')
    const cash = within(picker).getByRole('option', { name: 'include Cash' })
    expect(cash).toHaveAttribute('data-checked', 'true')
    expect(cash).toHaveAttribute('aria-disabled', 'true')
    expect(within(picker).getByText("Accounts with transactions in past months can't be removed")).toBeInTheDocument()
    await user.click(within(picker).getByRole('option', { name: 'include Savings' }))
    expect(onToggle).toHaveBeenCalledWith('a2', true)
    expect(onToggle).not.toHaveBeenCalledWith('a1', expect.anything())
  })

  it('shows no lock hint when nothing is locked', async () => {
    const user = userEvent.setup()
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set()} locked={new Set()} onToggle={vi.fn()} />)
    const picker = await openPicker(user, 'accounts')
    expect(within(picker).queryByText(/can't be removed/)).toBeNull()
  })

  it('searches from six accounts up', async () => {
    const many = ['Cash', 'Bank', 'Card', 'Pension', 'Brokerage', 'Wallet'].map((n, i) => acc(`m${i}`, n))
    const user = userEvent.setup()
    renderField(<BudgetAccountsField accounts={many} selected={new Set()} locked={new Set()} onToggle={vi.fn()} />)
    const picker = await openPicker(user, 'accounts')
    await user.type(within(picker).getByPlaceholderText('Search'), 'pens')
    expect(within(picker).getAllByRole('option').map((o) => o.getAttribute('aria-label'))).toEqual(['include Pension'])
  })

  it('lists accounts in hidden folders after the rest, under their own heading', async () => {
    const user = userEvent.setup()
    renderField(
      <BudgetAccountsField accounts={[acc('h1', 'Mattress', 'f-hidden'), acc('v1', 'Cash', 'f1')]} selected={new Set()} locked={new Set()} onToggle={vi.fn()} />,
    )
    const picker = await openPicker(user, 'accounts')
    expect(await within(picker).findByText('In hidden folders')).toBeInTheDocument()
    expect(within(picker).getAllByRole('option').map((o) => o.getAttribute('aria-label'))).toEqual(['include Cash', 'include Mattress'])
  })
})

describe('BudgetSavingsField', () => {
  const accounts = [acc('a1', 'Cash'), acc('a2', 'Rainy day'), acc('a3', 'Brokerage')]

  it('names the savings accounts; the picker lists included accounts only and toggles', async () => {
    const onToggle = vi.fn()
    const user = userEvent.setup()
    renderField(<BudgetSavingsField accounts={accounts} selected={new Set(['a1', 'a2'])} savings={new Set(['a2'])} onToggle={onToggle} />)
    expect(screen.getByTestId('budget-savings-field')).toHaveTextContent('Rainy day')
    const picker = await openPicker(user, 'savings')
    expect(within(picker).getByText('Money moved into these accounts counts as saved, not spent.')).toBeInTheDocument()
    expect(
      within(picker).getByText('Savings accounts are shown by name, with their saved amounts and balances, to everyone with access to this budget.'),
    ).toBeInTheDocument()
    expect(within(picker).getByRole('option', { name: 'Cash is a savings account' })).toHaveAttribute('data-checked', 'false')
    const rainy = within(picker).getByRole('option', { name: 'Rainy day is a savings account' })
    expect(rainy).toHaveAttribute('data-checked', 'true')
    expect(within(picker).queryByRole('option', { name: 'Brokerage is a savings account' })).toBeNull()
    await user.click(rainy)
    expect(onToggle).toHaveBeenCalledWith('a2', false)
  })

  it('reads None when no included account is savings', () => {
    renderField(<BudgetSavingsField accounts={accounts} selected={new Set(['a1'])} savings={new Set()} onToggle={vi.fn()} />)
    expect(screen.getByTestId('budget-savings-field')).toHaveTextContent('None')
    expect(screen.getByTestId('budget-savings-field')).not.toBeDisabled()
  })

  it('is disabled and asks to include an account first when none is included', () => {
    renderField(<BudgetSavingsField accounts={accounts} selected={new Set()} savings={new Set()} onToggle={vi.fn()} />)
    const row = screen.getByTestId('budget-savings-field')
    expect(row).toBeDisabled()
    expect(row).toHaveTextContent('Include an account above to mark it as savings.')
  })
})
