import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureOwner, fixtureUsd } from '@/test/fixtures'
import { BudgetElementType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { SetLimitDialog } from './SetLimitDialog'

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('lists the upcoming payments under the amount', async () => {
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: {
          items: [],
          upcoming: [
            {
              id: 'rt1', author: fixtureOwner, currencyId: 'cur-usd', amount: '70', description: 'Phone plan',
              category: null, payee: null, tag: null, spentAt: '2026-07-20 00:00:00',
              templateId: 'rt1', schedule: 'monthly', accountId: 'a1', accountName: 'Cash',
              labelNames: [], isNext: true, isOverdue: false, canEdit: true,
            },
          ],
        },
      }),
    ),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <SetLimitDialog
        target={{ id: 'cat-food', name: 'Food', value: '0' }}
        onClose={vi.fn()}
        onCommit={vi.fn()}
        upcoming={{
          budget: { meta: { id: 'b1' } },
          target: { id: 'cat-food', type: BudgetElementType.CATEGORY, name: 'Food', icon: 'restaurant', currencyId: null },
          periodStart: '2026-07-01',
          currency: fixtureUsd as CurrencyDto,
          exchange: (_from, _to, amount) => amount,
          spent: '0',
          upcoming: '70',
        }}
      />
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Phone plan')).toBeInTheDocument()
  expect(screen.queryByText(/^Spent /)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Use 70.00' }))
  expect(document.getElementById('set-limit-amount')).toHaveValue('70')
})
