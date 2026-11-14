import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixturePayees } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { ImportSuggestionDto } from '@/api/dto/imports'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useImportSuggestions } from './queries'
import { SuggestionsSection } from './SuggestionsSection'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/lib/metrics', async (original) => ({ ...await original<typeof import('@/lib/metrics')>(), trackEvent: vi.fn() }))

const category = fixtureCategories.find((c) => c.type === 'expense')!
const payee = fixturePayees[0]

const suggestion = (over: Partial<ImportSuggestionDto> = {}): ImportSuggestionDto => ({
  id: 'sg1', matchField: 'external_payee', matchText: 'BLUE BOTTLE', type: 'expense', categoryId: category.id, payeeId: payee.id,
  transactions: [
    { transactionId: 't1', accountId: 'a1', amount: '4.75', currency: 'USD', date: '2026-09-03 10:00:00', externalPayee: 'BLUE BOTTLE #1' },
    { transactionId: 't2', accountId: 'a1', amount: '5.25', currency: 'USD', date: '2026-09-01 10:00:00', externalPayee: 'BLUE BOTTLE #2' },
    { transactionId: 't3', accountId: 'a1', amount: '3.00', currency: 'USD', date: '2026-08-28 10:00:00', externalPayee: 'BLUE BOTTLE #3' },
  ],
  ...over,
})

function Section() {
  const { data = [] } = useImportSuggestions()
  return <SuggestionsSection suggestions={data} />
}

function renderSection(items: ImportSuggestionDto[]) {
  server.use(...coreHandlers({ importSuggestions: items }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(<QueryClientProvider client={queryClient}><Section /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  useUiStore.setState({ rulePrompt: null })
  vi.mocked(trackEvent).mockClear()
})

it('renders one row per suggestion with names and count', async () => {
  renderSection([suggestion(), suggestion({ id: 'sg2', matchText: 'WHOLE FOODS', payeeId: '', transactions: suggestion().transactions.slice(0, 1) })])
  expect(await screen.findByText('BLUE BOTTLE')).toBeInTheDocument()
  expect(screen.getByText('WHOLE FOODS')).toBeInTheDocument()
  expect(await screen.findByText(`→ ${category.name} · ${payee.name}`)).toBeInTheDocument()
  expect(screen.getByText(`→ ${category.name}`)).toBeInTheDocument()
  expect(screen.getByText(/3 transactions/)).toBeInTheDocument()
  expect(screen.getByText(/1 transaction ·/)).toBeInTheDocument()
  expect(screen.getAllByText(/4\.75 USD/)).toHaveLength(2)
})

it('dismiss removes the row and fires the metric', async () => {
  const dismissed: unknown[] = []
  renderSection([suggestion(), suggestion({ id: 'sg2', matchText: 'WHOLE FOODS' })])
  const rest = [suggestion({ id: 'sg2', matchText: 'WHOLE FOODS' })]
  server.use(
    http.post('*/api/v1/import/dismiss-suggestion', async ({ request }) => {
      dismissed.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: { items: rest } })
    }),
    http.get('*/api/v1/import/get-suggestion-list', () => HttpResponse.json({ success: true, message: '', data: { items: dismissed.length ? rest : [suggestion(), ...rest] } })),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Dismiss BLUE BOTTLE' }))
  await waitFor(() => expect(screen.queryByText('BLUE BOTTLE')).not.toBeInTheDocument())
  expect(screen.getByText('WHOLE FOODS')).toBeInTheDocument()
  expect(dismissed).toEqual([{ id: 'sg1' }])
  expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_DISMISS)
})

it('dismiss is disabled offline', async () => {
  renderSection([suggestion()])
  await screen.findByText('BLUE BOTTLE')
  onlineManager.setOnline(false)
  onTestFinished(() => onlineManager.setOnline(true))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Dismiss BLUE BOTTLE' })).toBeDisabled())
})

it('tapping a row opens the rule prompt in suggestion mode', async () => {
  renderSection([suggestion()])
  const user = userEvent.setup()
  await user.click(await screen.findByText('BLUE BOTTLE'))
  expect(useUiStore.getState().rulePrompt).toEqual({ kind: 'suggestion', suggestion: suggestion() })
})
