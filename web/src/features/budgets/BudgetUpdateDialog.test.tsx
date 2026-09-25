import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner } from '@/test/fixtures'
import type { BudgetDto } from '@/api/dto/budget'
import { BudgetUpdateDialog } from './BudgetUpdateDialog'

const baseBudget: BudgetDto = {
  meta: {
    id: 'b1', ownerUserId: 'u1', name: 'Main budget', startedAt: '2026-01-01 00:00:00', endedAt: '', currencyId: 'cur-usd',
    isArchived: 0, access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
  },
  filters: {
    periodStart: '2026-07-01 00:00:00',
    periodEnd: '2026-08-01 00:00:00',
    accounts: [
      { id: 'a1', removable: false },
      { id: 'a2', removable: true },
    ],
  },
  balances: [],
  currencyRates: [],
  structure: { folders: [], elements: [] },
}

function renderDialog(budget: BudgetDto, onClose = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <BudgetUpdateDialog open budget={budget} onClose={onClose} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('renders the locked member disabled and submits the unchanged membership', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: baseBudget.meta } })
    }),
  )
  const user = userEvent.setup()
  renderDialog(baseBudget)
  const cash = await screen.findByRole('switch', { name: 'include Cash' })
  expect(cash).toBeChecked()
  expect(cash).toBeDisabled()
  const bank = screen.getByRole('switch', { name: 'include Bank' })
  expect(bank).toBeChecked()
  expect(bank).not.toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountIds).toEqual(['a1', 'a2'])
})

it('toggling an unlocked member off drops it from the submitted accountIds', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: baseBudget.meta } })
    }),
  )
  const user = userEvent.setup()
  renderDialog(baseBudget)
  await screen.findByRole('switch', { name: 'include Cash' })
  await user.click(screen.getByRole('switch', { name: 'include Bank' }))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountIds).toEqual(['a1'])
})

it('a deleted member absent from the live account list still round-trips in accountIds', async () => {
  const budgetWithDeletedMember: BudgetDto = {
    ...baseBudget,
    filters: {
      ...baseBudget.filters,
      accounts: [
        { id: 'a1', removable: false },
        { id: 'a-deleted', removable: false },
      ],
    },
  }
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: baseBudget.meta } })
    }),
  )
  const user = userEvent.setup()
  renderDialog(budgetWithDeletedMember)
  await screen.findByRole('switch', { name: 'include Cash' })
  // no row is rendered for the deleted member — it has no matching live account
  expect(screen.queryByText('a-deleted')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountIds).toEqual(['a1', 'a-deleted'])
})

// A server older than the membership release omits filters.accounts (and older
// still, filters entirely). The dialog must degrade to "no locked rows", not
// crash the whole page with a render error.
it('renders when the server omits filters.accounts', async () => {
  const legacy = { ...baseBudget, filters: { ...baseBudget.filters, accounts: undefined } } as unknown as BudgetDto
  renderDialog(legacy)
  await waitFor(() => expect(screen.getByDisplayValue('Main budget')).toBeInTheDocument())
})

it('renders when the server omits filters entirely', async () => {
  const legacy = { ...baseBudget, filters: undefined } as unknown as BudgetDto
  renderDialog(legacy)
  await waitFor(() => expect(screen.getByDisplayValue('Main budget')).toBeInTheDocument())
})

describe('savings toggles', () => {
  const savingsBudget: BudgetDto = {
    ...baseBudget,
    filters: {
      ...baseBudget.filters,
      accounts: [
        { id: 'a1', removable: false, isSavings: true },
        { id: 'a2', removable: true, isSavings: false },
      ],
    },
  }
  const confirmQuestion =
    'Planned amounts and comments of the savings accounts you turned off or removed will be deleted from this budget. Saved amounts and transactions are not affected.'

  function captureUpdate(respond: (call: number) => Response | undefined = () => undefined) {
    const bodies: Record<string, unknown>[] = []
    server.use(
      http.post('*/api/v1/budget/update-budget', async ({ request }) => {
        bodies.push((await request.json()) as Record<string, unknown>)
        return respond(bodies.length) ?? HttpResponse.json({ success: true, message: '', data: { item: baseBudget.meta } })
      }),
    )
    return bodies
  }

  const refusal = (field: string, message: string) =>
    HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { [field]: [message] } }, { status: 400 })

  it('initialises the toggles from filters.accounts and sends the full own savings set', async () => {
    const bodies = captureUpdate()
    const user = userEvent.setup()
    renderDialog(savingsBudget)
    const cashSavings = await screen.findByRole('switch', { name: 'Cash is a savings account' })
    expect(cashSavings).toBeChecked()
    // a locked member can still change its savings role
    expect(cashSavings).not.toBeDisabled()
    expect(screen.getByRole('switch', { name: 'Bank is a savings account' })).not.toBeChecked()
    await user.click(screen.getByRole('switch', { name: 'Bank is a savings account' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].accountIds).toEqual(['a1', 'a2'])
    expect(bodies[0].savingsAccountIds).toEqual(['a1', 'a2'])
    expect(bodies[0]).not.toHaveProperty('confirmSavingsRemoval')
  })

  it('deselecting an account drops it from savingsAccountIds', async () => {
    const bodies = captureUpdate()
    const user = userEvent.setup()
    renderDialog({
      ...savingsBudget,
      filters: { ...savingsBudget.filters, accounts: [{ id: 'a1', removable: false, isSavings: false }, { id: 'a2', removable: true, isSavings: true }] },
    })
    await screen.findByRole('switch', { name: 'Bank is a savings account' })
    await user.click(screen.getByRole('switch', { name: 'include Bank' }))
    expect(screen.queryByRole('switch', { name: 'Bank is a savings account' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Update' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].accountIds).toEqual(['a1'])
    expect(bodies[0].savingsAccountIds).toEqual([])
  })

  it('a refused savings removal asks; cancel sends nothing more and keeps the edits', async () => {
    const bodies = captureUpdate((n) => (n === 1 ? refusal('confirmSavingsRemoval', 'confirm to continue') : undefined))
    const onClose = vi.fn()
    const user = userEvent.setup()
    renderDialog(savingsBudget, onClose)
    await user.click(await screen.findByRole('switch', { name: 'Cash is a savings account' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))
    expect(await screen.findByText(confirmQuestion)).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: 'Cancel' }).at(-1)!)
    await waitFor(() => expect(screen.queryByText(confirmQuestion)).toBeNull())
    expect(bodies).toHaveLength(1)
    expect(onClose).not.toHaveBeenCalled()
    expect(screen.getByRole('switch', { name: 'Cash is a savings account' })).not.toBeChecked()
    expect(screen.queryByText('confirm to continue')).toBeNull()
  })

  it('confirming resends the same payload with confirmSavingsRemoval and closes on success', async () => {
    const bodies = captureUpdate((n) => (n === 1 ? refusal('confirmSavingsRemoval', 'confirm to continue') : undefined))
    const onClose = vi.fn()
    const user = userEvent.setup()
    renderDialog(savingsBudget, onClose)
    await user.click(await screen.findByRole('switch', { name: 'Cash is a savings account' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))
    await user.click(await screen.findByRole('button', { name: 'Delete plans' }))
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[0].savingsAccountIds).toEqual([])
    expect(bodies[1]).toEqual({ ...bodies[0], confirmSavingsRemoval: true })
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('another 400 surfaces as the dialog error, not the confirmation', async () => {
    const bodies = captureUpdate(() => refusal('savingsAccountIds', 'Savings accounts must be your own accounts in this budget'))
    const onClose = vi.fn()
    const user = userEvent.setup()
    renderDialog(savingsBudget, onClose)
    await screen.findByRole('switch', { name: 'Cash is a savings account' })
    await user.click(screen.getByRole('button', { name: 'Update' }))
    expect(await screen.findByText('Savings accounts must be your own accounts in this budget')).toBeInTheDocument()
    expect(screen.queryByText(confirmQuestion)).toBeNull()
    expect(bodies).toHaveLength(1)
    expect(onClose).not.toHaveBeenCalled()
  })
})
