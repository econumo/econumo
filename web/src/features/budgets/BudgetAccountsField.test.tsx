import type { ReactElement } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { BudgetAccountsField } from './BudgetAccountsField'

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
  it('shows no hint when nothing is locked', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set()} locked={new Set()} onToggle={vi.fn()} />)
    expect(screen.queryByText(/can't be removed/)).toBeNull()
  })
})
