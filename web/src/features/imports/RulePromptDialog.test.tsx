import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { TransactionImportLinkDto } from '@/api/dto/imports'
import { RulePromptDialog } from './RulePromptDialog'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const link = (over: Partial<TransactionImportLinkDto> = {}): TransactionImportLinkDto => ({
  id: 'l1', sourceId: 's1', runId: 'r1', provider: 'apple-wallet', sourceName: 'iPhone', externalAccountId: 'wallet',
  externalTransactionId: 'tap-1', externalPayee: 'BLUE BOTTLE COFFEE #142 SAN FRANCISCO CA', externalDescription: '', externalAmount: '4.75',
  externalCurrency: 'USD', externalPostedAt: '2026-09-01 10:00:00', status: 'created', importedAt: '2026-09-01 10:00:05',
  appliedCategoryId: '', appliedPayeeId: '', appliedTagId: '', appliedLabelIds: [], appliedRuleId: '', ...over,
})

const rule = {
  id: 'rule1', sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE',
  isCaseSensitive: false, categoryId: 'cat-salary', payeeId: '', tagId: '', labelIds: [], priority: 0,
  createdAt: '2026-09-01 00:00:00', updatedAt: '2026-09-01 00:00:00',
}

function renderPrompt(data: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ categories: fixtureCategories, ...data }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(<QueryClientProvider client={queryClient}><RulePromptDialog /></QueryClientProvider>)
}

const posted: Record<string, unknown[]> = {}
beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  useUiStore.setState({ rulePrompt: null })
  for (const k of Object.keys(posted)) delete posted[k]
  server.use(
    http.post('*/api/v1/import/preview-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.preview ??= []).push(body)
      const matched = body.scope === 'source' ? 9 : 5
      return HttpResponse.json({ success: true, message: '', data: { matched, alreadyEdited: 2 } })
    }),
    http.post('*/api/v1/import/create-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.create ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { ...rule, ...body, id: 'rule-new' } })
    }),
    http.post('*/api/v1/import/update-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.update ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { ...rule, ...body } })
    }),
    http.post('*/api/v1/import/apply-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.apply ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { updated: 3, skipped: 2 } })
    }),
  )
})

it('renders nothing without a pending prompt', () => {
  renderPrompt()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('prefills a suggested match value, previews live, creates the rule, then applies to this import and offers the whole source', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  expect(await screen.findByText('You set the category to Food. Create a rule for similar imports?')).toBeInTheDocument()
  const value = screen.getByLabelText('Payee contains') as HTMLInputElement
  expect(value.value).toBe('BLUE BOTTLE COFFEE')
  expect(await screen.findByText('Matches 5 transactions in this import')).toBeInTheDocument()
  expect(posted.preview![0]).toMatchObject({ matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE COFFEE', categoryId: 'cat-food', scope: 'run', runId: 'r1', scopeSourceId: 's1' })

  await user.clear(value)
  await user.type(value, 'BLUE BOTTLE')
  await waitFor(() => expect(posted.preview!.at(-1)).toMatchObject({ matchValue: 'BLUE BOTTLE' }))

  await user.click(screen.getByRole('button', { name: 'Create rule' }))
  expect(posted.create![0]).toMatchObject({ action: 'classify', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food', sourceId: '' })
  expect(await screen.findByText('Apply this rule to 5 matching transactions in this import?')).toBeInTheDocument()
  expect(screen.getByText("2 skipped (you've edited these)")).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(posted.apply![0]).toEqual({ ruleId: 'rule-new', scope: 'run', runId: 'r1', scopeSourceId: 's1', includeEdited: false }))

  // clearly labelled second step: the whole source, with its own count
  expect(await screen.findByText('Also apply to all imports from iPhone? 9 transactions match.')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Apply to all imports' }))
  await waitFor(() => expect(posted.apply![1]).toEqual({ ruleId: 'rule-new', scope: 'source', runId: '', scopeSourceId: 's1', includeEdited: false }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(useUiStore.getState().rulePrompt).toBeNull()
})

it('the include-edited override is opt-in and travels on the apply call', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await user.click(await screen.findByRole('checkbox', { name: 'Also update the 2 transactions you edited' }))
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(posted.apply![0]).toMatchObject({ includeEdited: true }))
})

it('"Not now" closes without creating anything', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Not now' }))
  await waitFor(() => expect(useUiStore.getState().rulePrompt).toBeNull())
  expect(posted.create).toBeUndefined()
})

it('a transaction classified by a rule offers to update that rule instead, keeping its match', async () => {
  renderPrompt({ importRules: [rule] })
  useUiStore.getState().setRulePrompt({ link: link({ appliedRuleId: 'rule1', appliedCategoryId: 'cat-salary' }), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  expect(await screen.findByText('This transaction was classified by the rule “Payee contains BLUE BOTTLE”.')).toBeInTheDocument()
  expect(screen.queryByLabelText('Payee contains')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Update rule' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food' }))
  expect(posted.create).toBeUndefined()
  expect(await screen.findByText('Apply this rule to 5 matching transactions in this import?')).toBeInTheDocument()
})
