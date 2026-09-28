import type { ReactElement } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { BudgetAccountsField, BudgetSavingsField } from './BudgetAccountsField'

function renderField(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

beforeEach(() => {
  server.use(...coreHandlers())
})

describe('BudgetAccountsField', () => {
  const accounts = [
    { id: 'a1', name: 'Cash', icon: 'wallet', folderId: null } as any,
    { id: 'a2', name: 'Savings', icon: 'savings', folderId: null } as any,
  ]
  it('renders selected state and disables locked rows with a hint', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2'])} locked={new Set(['a1'])} onToggle={vi.fn()} />)
    const cash = screen.getByRole('switch', { name: 'include Cash' })
    expect(cash).toBeChecked()
    expect(cash).toBeDisabled()
    expect(screen.getByRole('switch', { name: 'include Savings' })).not.toBeDisabled()
    expect(screen.getByText("Accounts with transactions in past months can't be removed")).toBeInTheDocument()
    expect(screen.getByText('2 of 2 included')).toBeInTheDocument()
  })
  it('counts only members that have a row, ignoring deleted ones kept for round-tripping', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2', 'deleted-1'])} locked={new Set()} onToggle={vi.fn()} />)
    expect(screen.getByText('2 of 2 included')).toBeInTheDocument()
    expect(screen.queryByText('3 of 2 included')).toBeNull()
  })
  it('shows no hint when nothing is locked', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set()} locked={new Set()} onToggle={vi.fn()} />)
    expect(screen.queryByText(/can't be removed/)).toBeNull()
  })

  it('is one switch per row: the savings role lives in its own field', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2'])} locked={new Set()} onToggle={vi.fn()} />)
    for (const name of ['Cash', 'Savings']) {
      const row = screen.getByRole('switch', { name: `include ${name}` }).closest('li')!
      expect(within(row).getAllByRole('switch')).toHaveLength(1)
    }
  })
})

describe('BudgetSavingsField', () => {
  const accounts = [
    { id: 'a1', name: 'Cash', icon: 'wallet', folderId: null } as any,
    { id: 'a2', name: 'Rainy day', icon: 'savings', folderId: null } as any,
    { id: 'a3', name: 'Brokerage', icon: 'trending_up', folderId: null } as any,
  ]

  it('offers a pressed/unpressed chip per included account only, and toggles it', async () => {
    const onToggle = vi.fn()
    renderField(<BudgetSavingsField accounts={accounts} selected={new Set(['a1', 'a2'])} savings={new Set(['a2'])} onToggle={onToggle} />)
    const cash = screen.getByRole('button', { name: 'Cash is a savings account' })
    const rainy = screen.getByRole('button', { name: 'Rainy day is a savings account' })
    expect(cash).toHaveAttribute('aria-pressed', 'false')
    expect(rainy).toHaveAttribute('aria-pressed', 'true')
    expect(screen.queryByRole('button', { name: 'Brokerage is a savings account' })).toBeNull()
    const user = userEvent.setup()
    await user.click(cash)
    expect(onToggle).toHaveBeenCalledWith('a1', true)
    await user.click(rainy)
    expect(onToggle).toHaveBeenCalledWith('a2', false)
  })

  it('asks to include an account first when none is included; the visibility note shows either way', () => {
    const note = 'Savings accounts are shown by name, with their saved amounts and balances, to everyone with access to this budget.'
    const { unmount } = renderField(<BudgetSavingsField accounts={accounts} selected={new Set()} savings={new Set()} onToggle={vi.fn()} />)
    expect(screen.getByText('Include an account above to mark it as savings.')).toBeInTheDocument()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
    expect(screen.getByText(note)).toBeInTheDocument()
    unmount()
    renderField(<BudgetSavingsField accounts={accounts} selected={new Set(['a2'])} savings={new Set(['a2'])} onToggle={vi.fn()} />)
    expect(screen.queryByText('Include an account above to mark it as savings.')).toBeNull()
    expect(screen.getByText(note)).toBeInTheDocument()
  })
})
