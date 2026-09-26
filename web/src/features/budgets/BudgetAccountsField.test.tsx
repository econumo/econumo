import type { ReactElement } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2'])} locked={new Set(['a1'])} onToggle={vi.fn()} savings={new Set()} onToggleSavings={vi.fn()} />)
    const cash = screen.getByRole('switch', { name: 'include Cash' })
    expect(cash).toBeChecked()
    expect(cash).toBeDisabled()
    expect(screen.getByRole('switch', { name: 'include Savings' })).not.toBeDisabled()
    expect(screen.getByText("Accounts with transactions in past months can't be removed")).toBeInTheDocument()
    expect(screen.getByText('2 of 2 included')).toBeInTheDocument()
  })
  it('counts only members that have a row, ignoring deleted ones kept for round-tripping', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2', 'deleted-1'])} locked={new Set()} onToggle={vi.fn()} savings={new Set()} onToggleSavings={vi.fn()} />)
    expect(screen.getByText('2 of 2 included')).toBeInTheDocument()
    expect(screen.queryByText('3 of 2 included')).toBeNull()
  })
  it('shows no hint when nothing is locked', () => {
    renderField(<BudgetAccountsField accounts={accounts} selected={new Set()} locked={new Set()} onToggle={vi.fn()} savings={new Set()} onToggleSavings={vi.fn()} />)
    expect(screen.queryByText(/can't be removed/)).toBeNull()
  })

  it('shows a savings switch on selected rows only, enabled even for locked members', async () => {
    const onToggleSavings = vi.fn()
    renderField(
      <BudgetAccountsField
        accounts={accounts}
        selected={new Set(['a1'])}
        locked={new Set(['a1'])}
        onToggle={vi.fn()}
        savings={new Set(['a1'])}
        onToggleSavings={onToggleSavings}
      />,
    )
    const cashSavings = screen.getByRole('switch', { name: 'Cash is a savings account' })
    expect(cashSavings).toBeChecked()
    expect(cashSavings).not.toBeDisabled()
    expect(screen.queryByRole('switch', { name: 'Savings is a savings account' })).toBeNull()
    await userEvent.setup().click(cashSavings)
    expect(onToggleSavings).toHaveBeenCalledWith('a1', false)
  })
  it('keeps at least 24px between the savings switch and the include switch, so their tap zones do not overlap', () => {
    // each Switch widens its own tap zone by 12px a side (after:-inset-x-3): with the
    // two switches only 18px apart (the old mr-2 8px + the row's gap-2.5 10px), the tap
    // zones overlapped on phones. The row's flex gap-2.5 (10px) is unchanged, so the
    // savings wrapper's own right margin must contribute at least 14px on its own.
    renderField(
      <BudgetAccountsField
        accounts={accounts}
        selected={new Set(['a1'])}
        locked={new Set()}
        onToggle={vi.fn()}
        savings={new Set(['a1'])}
        onToggleSavings={vi.fn()}
      />,
    )
    const savingsSwitch = screen.getByRole('switch', { name: 'Cash is a savings account' })
    const wrapper = savingsSwitch.closest('span')
    expect(wrapper).not.toBeNull()
    expect(wrapper!.className).not.toMatch(/(?:^|\s)mr-2(?:\s|$)/)
    expect(wrapper!.className).toMatch(/(?:^|\s)mr-(?:3\.5|4|5|6|7|8|9|10)(?:\s|$)/)
  })

  it('stacks a row in a narrow list: include switch on the name line, savings switch on a second line under the name', () => {
    renderField(
      <BudgetAccountsField accounts={accounts} selected={new Set(['a1'])} locked={new Set()} onToggle={vi.fn()} savings={new Set()} onToggleSavings={vi.fn()} />,
    )
    const include = screen.getByRole('switch', { name: 'include Cash' })
    const row = include.closest('li')!
    expect(row.className).toMatch(/(?:^|\s)grid(?:\s|$)/)
    expect(row.className).toMatch(/(?:^|\s)@md:flex(?:\s|$)/)
    expect(row.parentElement!.className).toMatch(/(?:^|\s)@container(?:\s|$)/)
    expect(include.className).toMatch(/(?:^|\s)row-start-1(?:\s|$)/)
    const savingsWrapper = screen.getByRole('switch', { name: 'Cash is a savings account' }).closest('span')!
    expect(savingsWrapper.className).toMatch(/(?:^|\s)col-start-2 row-start-2(?:\s|$)/)
    expect(within(row).getByText('Cash').className).toMatch(/(?:^|\s)col-start-2 row-start-1(?:\s|$)/)
    // an unselected account has no second line at all
    const other = screen.getByRole('switch', { name: 'include Savings' }).closest('li')!
    expect(within(other).queryAllByRole('switch')).toHaveLength(1)
  })

  it('shows the savings visibility note whenever the list is shown', () => {
    const note = 'Savings accounts are shown by name, with their saved amounts and balances, to everyone with access to this budget.'
    const { unmount } = renderField(
      <BudgetAccountsField accounts={accounts} selected={new Set()} locked={new Set()} onToggle={vi.fn()} savings={new Set()} onToggleSavings={vi.fn()} />,
    )
    expect(screen.getByText(note)).toBeInTheDocument()
    unmount()
    renderField(
      <BudgetAccountsField accounts={accounts} selected={new Set(['a2'])} locked={new Set()} onToggle={vi.fn()} savings={new Set(['a2'])} onToggleSavings={vi.fn()} />,
    )
    expect(screen.getByText(note)).toBeInTheDocument()
  })
})
