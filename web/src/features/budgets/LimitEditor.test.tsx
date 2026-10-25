import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureOwner, fixtureUsd } from '@/test/fixtures'
import { BudgetElementType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { LimitEditor } from './LimitEditor'
import type { UpcomingQuery } from './UpcomingPaymentsPanel'

function recordTransactionLists() {
  const seen: URL[] = []
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      seen.push(new URL(request.url))
      const upcoming = [
        {
          id: 'rt1', author: fixtureOwner, currencyId: 'cur-usd', amount: '250', description: 'Groceries box',
          category: null, payee: null, tag: null, spentAt: '2026-07-20 00:00:00',
          templateId: 'rt1', schedule: 'monthly', accountId: 'a1', accountName: 'Cash',
          labelNames: [], isNext: true, isOverdue: false, canEdit: true,
        },
      ]
      return HttpResponse.json({ success: true, message: '', data: { items: [], upcoming } })
    }),
  )
  return seen
}

const upcoming: UpcomingQuery = {
  budget: { meta: { id: 'b1' } },
  target: { id: 'cat-food', type: BudgetElementType.CATEGORY, name: 'Food', icon: 'restaurant', currencyId: null },
  periodStart: '2026-07-01',
  currency: fixtureUsd as CurrencyDto,
  exchange: (_from, _to, amount) => amount,
  spent: '300',
  upcoming: '250',
}

function renderEditor(query: UpcomingQuery | null) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <LimitEditor id="cat-food" name="Food" value="0" currency={fixtureUsd as CurrencyDto} onCommit={vi.fn()} upcoming={query} />
    </QueryClientProvider>,
  )
}

it('lists the upcoming payments under the amount once opened', async () => {
  const seen = recordTransactionLists()
  renderEditor(upcoming)
  // closed: nothing is fetched yet
  await new Promise((r) => setTimeout(r, 20))
  expect(seen).toEqual([])
  fireEvent.click(screen.getByRole('button', { name: 'limit Food' }))
  expect(await screen.findByText('Groceries box')).toBeInTheDocument()
  expect(screen.getByTestId('upcoming-spent')).toHaveTextContent('Already spent300.00 $')
  expect(screen.getByTestId('upcoming-total')).toHaveTextContent('Total550.00')
})

it('a press on the list puts spent + still due into the input and keeps it focused', async () => {
  recordTransactionLists()
  renderEditor(upcoming)
  fireEvent.click(screen.getByRole('button', { name: 'limit Food' }))
  expect(await screen.findByRole('button', { name: /^Total 550\.00/ })).toBeInTheDocument()
  fireEvent.click(screen.getByText('Groceries box'))
  const input = screen.getByRole('textbox')
  expect(input).toHaveValue('550')
  expect(input).toHaveFocus()
})

it('fetches nothing for a cell without upcoming money', async () => {
  const seen = recordTransactionLists()
  renderEditor(null)
  fireEvent.click(screen.getByRole('button', { name: 'limit Food' }))
  expect(await screen.findByRole('textbox')).toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 20))
  expect(seen).toEqual([])
})
