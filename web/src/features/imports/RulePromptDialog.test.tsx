import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixturePayees, fixtureUser } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { ImportSuggestionDto, TransactionImportLinkDto } from '@/api/dto/imports'
import { toast } from 'sonner'
import { METRICS, trackEvent } from '@/lib/metrics'
import { RulePromptDialog } from './RulePromptDialog'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/lib/metrics', async (original) => ({ ...await original<typeof import('@/lib/metrics')>(), trackEvent: vi.fn() }))

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
  server.use(...coreHandlers({ categories: fixtureCategories, importSuggestions: [suggestion], ...data }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(<QueryClientProvider client={queryClient}><RulePromptDialog /></QueryClientProvider>)
  return queryClient
}

const posted: Record<string, unknown[]> = {}
// what create-rule stored, by id: like the server, a stored id is answered with the rule it stored
const storedRules = new Map<string, Record<string, unknown>>()
beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  useUiStore.setState({ rulePrompt: null })
  storedRules.clear()
  for (const k of Object.keys(posted)) delete posted[k]
  server.use(
    http.post('*/api/v1/import/preview-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.preview ??= []).push(body)
      const matched = body.scope === 'all' ? 9 : 5
      return HttpResponse.json({ success: true, message: '', data: { matched, alreadyEdited: 2 } })
    }),
    http.post('*/api/v1/import/create-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.create ??= []).push(body)
      const id = body.id as string
      if (!storedRules.has(id)) storedRules.set(id, { ...rule, ...body, id: 'rule-new' })
      return HttpResponse.json({ success: true, message: '', data: storedRules.get(id) })
    }),
    http.post('*/api/v1/import/update-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.update ??= []).push(body)
      for (const [key, stored] of storedRules) {
        if (stored.id === body.id) storedRules.set(key, { ...stored, ...body })
      }
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

it('prefills a suggested match value, previews live, creates the rule, then applies to this import and offers every import', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
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

  // clearly labelled second step: every import, whatever its provider, with its own count
  expect(await screen.findByText('Also apply to all your imports? 9 transactions match.')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Apply to all imports' }))
  await waitFor(() => expect(posted.apply![1]).toEqual({ ruleId: 'rule-new', scope: 'all', runId: '', scopeSourceId: '', includeEdited: false }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(useUiStore.getState().rulePrompt).toBeNull()
})

it('the include-edited override is opt-in and travels on the apply call', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await user.click(await screen.findByRole('checkbox', { name: 'Also update the 2 transactions you edited' }))
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(posted.apply![0]).toMatchObject({ includeEdited: true }))
})

it('"Not now" closes without creating anything', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Not now' }))
  await waitFor(() => expect(useUiStore.getState().rulePrompt).toBeNull())
  expect(posted.create).toBeUndefined()
})

it('a transaction classified by a rule offers to update that rule instead, keeping its match', async () => {
  renderPrompt({ importRules: [rule] })
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link({ appliedRuleId: 'rule1', appliedCategoryId: 'cat-salary' }), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  expect(await screen.findByText('This transaction was classified by the rule “Payee contains BLUE BOTTLE”.')).toBeInTheDocument()
  expect(screen.queryByLabelText('Payee contains')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Update rule' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food' }))
  expect(posted.create).toBeUndefined()
  expect(await screen.findByText('Apply this rule to 5 matching transactions in this import?')).toBeInTheDocument()
})

it('a preview refresh failure after a successful create still advances to apply, without risking a duplicate create on retry', async () => {
  renderPrompt()
  server.use(http.post('*/api/v1/import/preview-rule', () =>
    HttpResponse.json({ success: false, message: 'boom', code: 0, errors: {} }, { status: 500 })))
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  // the rule was created despite the refresh failing; the apply step shows a
  // stale (zero) count rather than getting stuck on "define" and inviting a
  // second, duplicate create-rule click
  expect(await screen.findByText('Apply this rule to 0 matching transactions in this import?')).toBeInTheDocument()
  expect(posted.create).toHaveLength(1)
})

 it('a second tap on Create rule while the counts refresh creates nothing more', async () => {
  renderPrompt()
  let release: () => void = () => {}
  const held = new Promise<void>((resolve) => { release = resolve })
  server.use(http.post('*/api/v1/import/preview-rule', async () => {
    if (posted.create) await held
    return HttpResponse.json({ success: true, message: '', data: { matched: 5, alreadyEdited: 0 } })
  }))
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  const button = await screen.findByRole('button', { name: 'Create rule' })
  await user.click(button)
  await waitFor(() => expect(posted.create).toHaveLength(1))
  await user.click(button)
  release()
  expect(await screen.findByRole('button', { name: 'Apply' })).toBeInTheDocument()
  expect(posted.create).toHaveLength(1)
})

it('a link with no run applies to every import at once and skips the second step', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link({ runId: '' }), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  expect(await screen.findByText('Apply this rule to 9 matching transactions across your imports?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(posted.apply![0]).toEqual({ ruleId: 'rule-new', scope: 'all', runId: '', scopeSourceId: '', includeEdited: false }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(useUiStore.getState().rulePrompt).toBeNull()
})

it('the include-edited choice does not carry over from the run-scoped apply to the apply on every import', async () => {
  renderPrompt()
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await user.click(await screen.findByRole('checkbox', { name: 'Also update the 2 transactions you edited' }))
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(posted.apply![0]).toMatchObject({ scope: 'run', includeEdited: true }))

  await screen.findByText('Also apply to all your imports? 9 transactions match.')
  expect(screen.getByRole('checkbox', { name: 'Also update the 2 transactions you edited' })).not.toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Apply to all imports' }))
  await waitFor(() => expect(posted.apply![1]).toMatchObject({ scope: 'all', includeEdited: false }))
})

// update-rule REPLACES the rule's whole label set while the prompt's diff
// carries only the labels the user ADDED, so assigning the diff would
// silently strip every label the rule already sets from every future import.
it('updating a rule unions the added labels with the ones it already sets', async () => {
  const labelled = { ...rule, labelIds: ['label-work', 'label-reimbursable'] }
  renderPrompt({ importRules: [labelled] })
  useUiStore.getState().setRulePrompt({
    kind: 'edit',
    link: link({ appliedRuleId: 'rule1', appliedCategoryId: 'cat-salary', appliedLabelIds: ['label-work', 'label-reimbursable'] }),
    diff: { labelIds: ['label1'] },
  })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Update rule' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1' }))
  expect((posted.update![0] as { labelIds: string[] }).labelIds).toEqual(['label-work', 'label-reimbursable', 'label1'])
})

it('the unioned label set is capped at the server maximum', async () => {
  const existingIds = Array.from({ length: 10 }, (_, i) => `label-${i}`)
  renderPrompt({ importRules: [{ ...rule, labelIds: existingIds }] })
  useUiStore.getState().setRulePrompt({
    kind: 'edit',
    link: link({ appliedRuleId: 'rule1', appliedCategoryId: 'cat-salary', appliedLabelIds: existingIds }),
    diff: { labelIds: ['label1'] },
  })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Update rule' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1' }))
  expect((posted.update![0] as { labelIds: string[] }).labelIds).toEqual(existingIds)
})

function previewCounts(run: number, all: number, allEdited = 0) {
  server.use(http.post('*/api/v1/import/preview-rule', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>
    ;(posted.preview ??= []).push(body)
    return HttpResponse.json({ success: true, message: '', data: { matched: body.scope === 'all' ? all : run, alreadyEdited: body.scope === 'all' ? allEdited : 0 } })
  }))
}

it('skips the run step when the preview matches nothing', async () => {
  renderPrompt()
  previewCounts(0, 3)
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  expect(await screen.findByText('Also apply to all your imports? 3 transactions match.')).toBeInTheDocument()
  expect(screen.queryByText(/Apply this rule to/)).not.toBeInTheDocument()
})

it('closes after saving when neither scope has anything to apply', async () => {
  renderPrompt()
  previewCounts(0, 0)
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(posted.create).toHaveLength(1)
  expect(posted.apply).toBeUndefined()
})

it('a link with no run closes after saving when nothing would change', async () => {
  renderPrompt()
  previewCounts(0, 0)
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link({ runId: '' }), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(posted.apply).toBeUndefined()
})

it('closes after applying to the run when the import-wide count fails, instead of waiting forever', async () => {
  renderPrompt()
  server.use(http.post('*/api/v1/import/preview-rule', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>
    if (body.scope === 'all') return HttpResponse.json({ success: false, message: 'boom', code: 0, errors: {} }, { status: 500 })
    return HttpResponse.json({ success: true, message: '', data: { matched: 5, alreadyEdited: 0 } })
  }))
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await user.click(await screen.findByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(posted.apply).toHaveLength(1)
})

it('closes after applying to the run when no other import would change', async () => {
  renderPrompt()
  previewCounts(5, 0)
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await user.click(await screen.findByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(posted.apply).toHaveLength(1)
})

type Counts = { matched: number; changes: number; alreadyEdited: number }
function previewFull(run: Counts, all: Counts) {
  server.use(http.post('*/api/v1/import/preview-rule', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>
    ;(posted.preview ??= []).push(body)
    return HttpResponse.json({ success: true, message: '', data: body.scope === 'all' ? all : run })
  }))
}

it('the live count reads every match, the apply question only the rows that would change', async () => {
  renderPrompt()
  previewFull({ matched: 5, changes: 3, alreadyEdited: 0 }, { matched: 9, changes: 4, alreadyEdited: 0 })
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  expect(await screen.findByText('Matches 5 transactions in this import')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Create rule' }))
  expect(await screen.findByText('Apply this rule to 3 matching transactions in this import?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  expect(await screen.findByText('Also apply to all your imports? 4 transactions match.')).toBeInTheDocument()
})

it('a rule already applied still shows its matches, and closes after saving with nothing to apply', async () => {
  renderPrompt()
  previewFull({ matched: 5, changes: 0, alreadyEdited: 0 }, { matched: 9, changes: 0, alreadyEdited: 0 })
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  expect(await screen.findByText('Matches 5 transactions in this import')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Create rule' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(posted.apply).toBeUndefined()
})

it('skips both apply steps when every row that would change is one the user edited', async () => {
  renderPrompt()
  previewFull({ matched: 5, changes: 2, alreadyEdited: 2 }, { matched: 9, changes: 3, alreadyEdited: 3 })
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(posted.preview!.some((p) => (p as { scope?: string }).scope === 'all')).toBe(true)
  expect(posted.apply).toBeUndefined()
})

it('offers every import after the run only when an unedited row there would change', async () => {
  renderPrompt()
  previewFull({ matched: 5, changes: 3, alreadyEdited: 0 }, { matched: 9, changes: 2, alreadyEdited: 2 })
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Create rule' }))
  await user.click(await screen.findByRole('button', { name: 'Apply' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(posted.apply).toHaveLength(1)
})

it('a failed live count shows no count instead of loading forever', async () => {
  renderPrompt()
  server.use(http.post('*/api/v1/import/preview-rule', () => HttpResponse.json({ success: false, message: 'boom', code: 0, errors: {} }, { status: 500 })))
  useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
  await screen.findByText('You set the category to Food. Create a rule for similar imports?')
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByText('Loading your data')).not.toBeInTheDocument()
  expect(screen.queryByText(/Matches \d+ transactions/)).not.toBeInTheDocument()
})

const suggestion: ImportSuggestionDto = {
  id: 'sg1', matchField: 'external_payee', matchText: 'BLUE BOTTLE', type: 'expense', categoryId: 'cat-food', payeeId: 'p1', payeeName: '',
  transactions: [
    { transactionId: 't1', accountId: 'a1', amount: '4.75', currency: 'USD', date: '2026-09-03 10:00:00', externalPayee: 'BLUE BOTTLE #1' },
    { transactionId: 't2', accountId: 'a1', amount: '5.25', currency: 'USD', date: '2026-09-01 10:00:00', externalPayee: 'BLUE BOTTLE #2' },
  ],
}

function acceptHandler(updated = 2) {
  server.use(http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
    ;(posted.accept ??= []).push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { updated } })
  }))
}

describe('suggestion mode', () => {
  beforeEach(() => {
    vi.mocked(trackEvent).mockClear()
    vi.mocked(toast.success).mockClear()
  })

  it('suggestion mode prefills match text, category and payee', async () => {
    renderPrompt()
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    expect(await screen.findByText('Suggested for 2 transactions')).toBeInTheDocument()
    expect((screen.getByLabelText('Payee contains') as HTMLInputElement).value).toBe('BLUE BOTTLE')
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Category' })).toHaveValue('Food'))
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Grocer'))
    await waitFor(() => expect(posted.preview?.[0]).toMatchObject({ matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food', payeeId: 'p1', scope: 'all' }))
  })

  it('saving creates the rule, accepts the suggestion and closes when nothing else matches', async () => {
    renderPrompt()
    previewCounts(0, 0)
    acceptHandler(2)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(posted.create![0]).toMatchObject({ action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food', payeeId: 'p1' })
    expect(posted.accept).toEqual([{ id: 'sg1', ruleId: 'rule-new' }])
    expect(posted.apply).toBeUndefined()
    expect(vi.mocked(toast.success)).toHaveBeenCalledWith('Updated 2 transactions')
  })

  // the AI job usually filled the suggestion's rows already: "Updated 0 transactions" would read as
  // a failure
  it('saving closes without a toast when the accept updated nothing', async () => {
    renderPrompt()
    previewCounts(0, 0)
    acceptHandler(0)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(posted.accept).toEqual([{ id: 'sg1', ruleId: 'rule-new' }])
    expect(vi.mocked(toast.success)).not.toHaveBeenCalled()
  })

  it('saving asks to apply to others when the all preview matches 2', async () => {
    renderPrompt()
    previewCounts(0, 2)
    acceptHandler(2)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Also apply to all your imports? 2 transactions match.')).toBeInTheDocument()
    expect(posted.accept).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Apply to all imports' }))
    await waitFor(() => expect(posted.apply![0]).toEqual({ ruleId: 'rule-new', scope: 'all', runId: '', scopeSourceId: '', includeEdited: false }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  // a suggestion row whose payee the user set by hand still matches, but only as an edited row:
  // offering it would update nothing, or overwrite the hand-set payee
  it('saving closes with the accept toast when every other match is an edited row', async () => {
    renderPrompt()
    previewCounts(0, 1, 1)
    acceptHandler(2)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(posted.apply).toBeUndefined()
    expect(vi.mocked(toast.success)).toHaveBeenCalledWith('Updated 2 transactions')
  })

  it('offers every import only when the rule would change a row the user did not edit', async () => {
    renderPrompt()
    previewFull({ matched: 0, changes: 0, alreadyEdited: 0 }, { matched: 4, changes: 1, alreadyEdited: 1 })
    acceptHandler(2)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(posted.apply).toBeUndefined()
  })

  it('a failed live count shows no count instead of loading forever', async () => {
    renderPrompt()
    server.use(http.post('*/api/v1/import/preview-rule', () => HttpResponse.json({ success: false, message: 'boom', code: 0, errors: {} }, { status: 500 })))
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    await screen.findByRole('button', { name: 'Save' })
    await new Promise((r) => setTimeout(r, 500))
    expect(screen.queryByText('Loading your data')).not.toBeInTheDocument()
  })

  it('the accept metric reports changed fields', async () => {
    renderPrompt()
    previewCounts(0, 0)
    acceptHandler(1)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    const value = await screen.findByLabelText('Payee contains')
    await user.clear(value)
    await user.type(value, 'BLUE')
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Grocer'))
    await user.click(screen.getByRole('combobox', { name: 'Payee' }))
    await user.click(await screen.findByRole('option', { name: '—' }))
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue(''))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(posted.accept).toHaveLength(1))
    expect(posted.create![0]).toMatchObject({ matchValue: 'BLUE', categoryId: 'cat-food', payeeId: '' })
    await waitFor(() => expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_ACCEPT, { changedMatch: true, changedCategory: false, changedPayee: true }))
  })

  it('the category picker offers only categories of the suggestion\'s type', async () => {
    renderPrompt()
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Category' })).toHaveValue('Food'))
    await user.click(screen.getByRole('combobox', { name: 'Category' }))
    expect(await screen.findByRole('option', { name: 'Food' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Salary' })).not.toBeInTheDocument()
  })

  it('a failed accept, Cancel and a reopen reuse the created rule instead of creating another', async () => {
    renderPrompt({ importRules: [{ ...rule, id: 'rule-new' }] })
    previewCounts(0, 0)
    let fail = true
    server.use(http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
      ;(posted.accept ??= []).push(await request.json())
      if (fail) return HttpResponse.json({ success: false, message: 'boom', code: 0, errors: {} }, { status: 400 })
      return HttpResponse.json({ success: true, message: '', data: { updated: 2 } })
    }))
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(posted.accept).toHaveLength(1))
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(useUiStore.getState().rulePrompt).toBeNull())
    fail = false
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    const ids = (posted.create as Record<string, unknown>[]).map((c) => c.id)
    expect(ids).toEqual(['sg1', 'sg1'])
    expect(storedRules.size).toBe(1)
    expect(posted.update).toBeUndefined()
    expect(posted.accept).toEqual([{ id: 'sg1', ruleId: 'rule-new' }, { id: 'sg1', ruleId: 'rule-new' }])
  })

  it('a retry after an edit updates the created rule to the new spec', async () => {
    renderPrompt({ importRules: [{ ...rule, id: 'rule-new' }] })
    previewCounts(0, 0)
    let fail = true
    server.use(http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
      ;(posted.accept ??= []).push(await request.json())
      if (fail) return HttpResponse.json({ success: false, message: 'boom', code: 0, errors: {} }, { status: 400 })
      return HttpResponse.json({ success: true, message: '', data: { updated: 2 } })
    }))
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(posted.accept).toHaveLength(1))
    fail = false
    const value = screen.getByLabelText('Payee contains')
    await user.clear(value)
    await user.type(value, 'BLUE B')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(storedRules.size).toBe(1)
    expect(posted.update).toHaveLength(1)
    expect(posted.update![0]).toMatchObject({ id: 'rule-new', matchValue: 'BLUE B' })
    expect(posted.accept![1]).toEqual({ id: 'sg1', ruleId: 'rule-new' })
  })

  // a failed accept, then a reload: nothing of the first Save is left on the device
  const reloadAfterCommittedCreate = async () => {
    previewCounts(0, 0)
    let fail = true
    server.use(http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
      ;(posted.accept ??= []).push(await request.json())
      if (fail) return HttpResponse.error()
      return HttpResponse.json({ success: true, message: '', data: { updated: 2 } })
    }))
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(posted.accept).toHaveLength(1))
    cleanup()
    useUiStore.setState({ rulePrompt: null })
    fail = false
    renderPrompt()
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    return user
  }

  it('a Save after a reload replays the rule the first Save created instead of making another', async () => {
    renderPrompt()
    const user = await reloadAfterCommittedCreate()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect((posted.create as Record<string, unknown>[]).map((c) => c.id)).toEqual(['sg1', 'sg1'])
    expect(storedRules.size).toBe(1)
    expect(posted.update).toBeUndefined()
    expect(posted.accept!.at(-1)).toEqual({ id: 'sg1', ruleId: 'rule-new' })
  })

  it('a Save after a reload with a changed spec updates the stored rule', async () => {
    renderPrompt()
    const user = await reloadAfterCommittedCreate()
    const value = await screen.findByLabelText('Payee contains')
    await user.clear(value)
    await user.type(value, 'BLUE B')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(storedRules.size).toBe(1)
    expect(posted.update).toHaveLength(1)
    expect(posted.update![0]).toMatchObject({ id: 'rule-new', matchValue: 'BLUE B' })
    expect(posted.accept!.at(-1)).toEqual({ id: 'sg1', ruleId: 'rule-new' })
  })

  it('a stored rule edited on the rules page keeps its tag, labels, priority and source when the dialog saves again', async () => {
    renderPrompt()
    storedRules.set('sg1', {
      ...rule, id: 'rule-new', matchValue: 'BLUE', categoryId: 'cat-food', payeeId: '',
      tagId: 'tag-1', labelIds: ['lab-1'], priority: 7, sourceId: 's1', isCaseSensitive: true,
    })
    previewCounts(0, 0)
    acceptHandler(1)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(posted.update).toHaveLength(1)
    expect(posted.update![0]).toMatchObject({
      id: 'rule-new', matchValue: suggestion.matchText, categoryId: suggestion.categoryId, payeeId: suggestion.payeeId,
      tagId: 'tag-1', labelIds: ['lab-1'], priority: 7, sourceId: 's1', isCaseSensitive: true, action: 'classify',
    })
  })

  it('the apply-to-all preview describes the stored rule, labels and tag included', async () => {
    renderPrompt()
    storedRules.set('sg1', {
      ...rule, id: 'rule-new', matchValue: 'BLUE BOTTLE', categoryId: 'cat-food', payeeId: 'p1', matchField: 'external_payee',
      tagId: 'tag-1', labelIds: ['lab-1'], priority: 7,
    })
    previewCounts(0, 3)
    acceptHandler(1)
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await screen.findByRole('button', { name: /apply/i })
    const all = (posted.preview as Record<string, unknown>[]).filter((b) => b.scope === 'all')
    expect(all.at(-1)).toMatchObject({ labelIds: ['lab-1'], tagId: 'tag-1', priority: 7 })
  })

  it('a rule deleted since is created again under the same id', async () => {
    renderPrompt()
    const user = await reloadAfterCommittedCreate()
    storedRules.clear()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(storedRules.size).toBe(1)
    expect([...storedRules.keys()]).toEqual(['sg1'])
    expect(posted.update).toBeUndefined()
  })

  it('a create whose answer was lost is sent again under the same id, and no second rule is made', async () => {
    renderPrompt()
    previewCounts(0, 0)
    acceptHandler(2)
    const stored = new Map<string, Record<string, unknown>>()
    let lose = true
    server.use(http.post('*/api/v1/import/create-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.create ??= []).push(body)
      // the server answers a stored id with the rule it stored
      const id = body.id as string
      if (!stored.has(id)) stored.set(id, { ...rule, ...body })
      if (lose) {
        lose = false
        return HttpResponse.error()
      }
      return HttpResponse.json({ success: true, message: '', data: stored.get(id) })
    }))
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(posted.create).toHaveLength(1))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
    expect(posted.accept).toBeUndefined()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    const ids = (posted.create as Record<string, unknown>[]).map((c) => c.id)
    expect(ids).toHaveLength(2)
    expect(ids[1]).toBe(ids[0])
    expect(stored.size).toBe(1)
    expect(posted.accept).toEqual([{ id: 'sg1', ruleId: ids[0] }])
  })

  it('an accept refused because its first attempt already landed closes as done', async () => {
    renderPrompt({ importRules: [{ ...rule, id: 'rule-new' }] })
    previewCounts(0, 0)
    let listed = true
    server.use(
      http.get('*/api/v1/import/get-suggestion-list', () => HttpResponse.json({ success: true, message: '', data: { items: listed ? [suggestion] : [] } })),
      http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
        ;(posted.accept ??= []).push(await request.json())
        if (posted.accept!.length === 1) {
          listed = false
          return HttpResponse.error()
        }
        return HttpResponse.json({ success: false, message: 'Import suggestion not found', code: 0, errors: {} }, { status: 400 })
      }),
    )
    vi.mocked(toast.error).mockClear()
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(posted.accept).toHaveLength(1))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
    vi.mocked(toast.error).mockClear()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(posted.accept).toHaveLength(2)
    expect(storedRules.size).toBe(1)
    expect(vi.mocked(toast.error)).not.toHaveBeenCalled()
  })

  it('a first accept refused because the suggestion was settled elsewhere is an error, and the dialog stays', async () => {
    renderPrompt()
    previewCounts(0, 0)
    server.use(
      http.get('*/api/v1/import/get-suggestion-list', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })),
      http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
        ;(posted.accept ??= []).push(await request.json())
        return HttpResponse.json({ success: false, message: 'Import suggestion not found', code: 0, errors: {} }, { status: 400 })
      }),
    )
    vi.mocked(toast.error).mockClear()
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Import suggestion not found'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(posted.accept).toHaveLength(1)
  })

  describe('a suggested new payee', () => {
    let seq = 0
    // each test its own suggestion: the operation id a Save mints is kept per suggestion
    const proposing = (over: Partial<ImportSuggestionDto> = {}): ImportSuggestionDto =>
      ({ ...suggestion, id: `sg-new-${++seq}`, payeeId: '', payeeName: 'Blue Bottle', ...over })
    const refused = (message: string) => HttpResponse.json({ success: false, message, code: 0, errors: {} }, { status: 400 })

    // like the server: the request id is the operation id and the payee gets its own; a repeated
    // operation id and a name already taken are refused
    function payeeServer(initial: Record<string, unknown>[] = fixturePayees, { loseFirst = false } = {}) {
      const payees = [...initial]
      const ops = new Set<string>()
      let lose = loseFirst
      server.use(
        http.get('*/api/v1/payee/get-payee-list', () => HttpResponse.json({ success: true, message: '', data: { items: payees } })),
        http.post('*/api/v1/payee/create-payee', async ({ request }) => {
          const body = await request.json() as { id: string; name: string }
          ;(posted.createPayee ??= []).push(body)
          if (ops.has(body.id)) return refused('Operation is locked')
          if (payees.some((p) => p.name === body.name)) return refused('Payee already exists.')
          ops.add(body.id)
          const item = { id: `payee-${payees.length + 1}`, ownerUserId: 'u1', name: body.name, position: payees.length, isArchived: 0, createdAt: '', updatedAt: '' }
          payees.push(item)
          if (lose) {
            lose = false
            return HttpResponse.error()
          }
          return HttpResponse.json({ success: true, message: '', data: { item } })
        }),
      )
      return payees
    }

    const open = async (s: ImportSuggestionDto) => {
      useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion: s })
      await screen.findByRole('button', { name: 'Save' })
      return userEvent.setup()
    }

    it('shows as new, and Save creates it, then the rule with its id, then accepts', async () => {
      const queryClient = renderPrompt()
      const payees = payeeServer()
      previewCounts(0, 0)
      acceptHandler(2)
      const s = proposing()
      const user = await open(s)
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Blue Bottle (new)'))
      expect(posted.createPayee).toBeUndefined()
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toEqual([{ id: expect.any(String), name: 'Blue Bottle' }])
      expect(payees).toHaveLength(2)
      expect(posted.create).toHaveLength(1)
      expect(posted.create![0]).toMatchObject({ categoryId: 'cat-food', payeeId: 'payee-2' })
      expect(posted.accept).toEqual([{ id: s.id, ruleId: 'rule-new' }])
      expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.payees)?.map((p) => p.id)).toContain('payee-2')
      expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_ACCEPT,
        { changedMatch: false, changedCategory: false, changedPayee: false, newPayee: true })
    })

    it('a Save retried after a lost create-payee answer finds the payee it made and makes no second one', async () => {
      renderPrompt()
      const payees = payeeServer(fixturePayees, { loseFirst: true })
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(posted.createPayee).toHaveLength(1))
      await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
      expect(posted.create).toBeUndefined()
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toHaveLength(1)
      expect(payees).toHaveLength(2)
      expect(posted.create![0]).toMatchObject({ payeeId: 'payee-2' })
      expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_ACCEPT, expect.objectContaining({ newPayee: true }))
    })

    it('a Save retried after a failed accept, Cancel and a reopen uses the payee it created', async () => {
      renderPrompt()
      const payees = payeeServer()
      previewCounts(0, 0)
      let fail = true
      server.use(http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
        ;(posted.accept ??= []).push(await request.json())
        if (fail) return refused('boom')
        return HttpResponse.json({ success: true, message: '', data: { updated: 2 } })
      }))
      const s = proposing()
      let user = await open(s)
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(posted.accept).toHaveLength(1))
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(useUiStore.getState().rulePrompt).toBeNull())
      fail = false
      user = await open(s)
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toHaveLength(1)
      expect(payees).toHaveLength(2)
      expect((posted.create as Record<string, unknown>[]).map((c) => c.payeeId)).toEqual(['payee-2', 'payee-2'])
      expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_ACCEPT, expect.objectContaining({ newPayee: true }))
    })

    it('a payee of the same name already there is used instead of creating one', async () => {
      renderPrompt()
      payeeServer([...fixturePayees, { id: 'p-bb', ownerUserId: 'u1', name: 'blue-bottle', position: 1, isArchived: 0, createdAt: '', updatedAt: '' }])
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('blue-bottle'))
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toBeUndefined()
      expect(posted.create![0]).toMatchObject({ payeeId: 'p-bb' })
      expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_ACCEPT,
        { changedMatch: false, changedCategory: false, changedPayee: false })
    })

    it('a Save before the payee list has loaded still reuses a payee of that name', async () => {
      renderPrompt()
      payeeServer([...fixturePayees, { id: 'p-bb', ownerUserId: 'u1', name: 'blue-bottle', position: 1, isArchived: 0, createdAt: '', updatedAt: '' }])
      const listed = [...fixturePayees, { id: 'p-bb', ownerUserId: 'u1', name: 'blue-bottle', position: 1, isArchived: 0, createdAt: '', updatedAt: '' }]
      server.use(http.get('*/api/v1/payee/get-payee-list', async () => {
        await new Promise((r) => setTimeout(r, 300))
        return HttpResponse.json({ success: true, message: '', data: { items: listed } })
      }))
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toBeUndefined()
      expect(posted.create![0]).toMatchObject({ payeeId: 'p-bb' })
    })

    it('a payee of that name made elsewhere after the dialog opened is used without creating one', async () => {
      renderPrompt()
      const payees = payeeServer()
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Blue Bottle (new)'))
      // only case and punctuation differ: the server's exact-name check would take a second one
      payees.push({ id: 'p-other', ownerUserId: 'u1', name: 'BLUE-BOTTLE', position: 1, isArchived: 0, createdAt: '', updatedAt: '' })
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toBeUndefined()
      expect(payees).toHaveLength(2)
      expect(posted.create![0]).toMatchObject({ payeeId: 'p-other' })
    })

    it('a payee of that name made between the fresh read and the post is used when create-payee refuses it', async () => {
      renderPrompt()
      const payees = payeeServer()
      server.use(http.post('*/api/v1/payee/create-payee', async ({ request }) => {
        ;(posted.createPayee ??= []).push(await request.json())
        payees.push({ id: 'p-other', ownerUserId: 'u1', name: 'Blue Bottle', position: 1, isArchived: 0, createdAt: '', updatedAt: '' })
        return refused('Payee already exists.')
      }))
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Blue Bottle (new)'))
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toHaveLength(1)
      expect(payees).toHaveLength(2)
      expect(posted.create![0]).toMatchObject({ payeeId: 'p-other' })
      expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_ACCEPT,
        { changedMatch: false, changedCategory: false, changedPayee: false })
    })

    it('an archived payee of that name means no payee: nothing is created and the rule has none', async () => {
      renderPrompt()
      const payees = payeeServer([...fixturePayees, { id: 'p-arch', ownerUserId: 'u1', name: 'BLUE BOTTLE', position: 1, isArchived: 1, createdAt: '', updatedAt: '' }])
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).not.toHaveValue('Blue Bottle (new)'))
      expect(screen.getByRole('combobox', { name: 'Payee' })).not.toHaveValue('BLUE BOTTLE')
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toBeUndefined()
      expect(payees).toHaveLength(2)
      expect(posted.create![0]).toMatchObject({ categoryId: 'cat-food', payeeId: '' })
    })

    it('a payee archived after the dialog opened is not created again on Save', async () => {
      renderPrompt()
      const payees = payeeServer()
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Blue Bottle (new)'))
      payees.push({ id: 'p-arch', ownerUserId: 'u1', name: 'Blue Bottle', position: 1, isArchived: 1, createdAt: '', updatedAt: '' })
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toBeUndefined()
      expect(posted.create![0]).toMatchObject({ categoryId: 'cat-food', payeeId: '' })
    })

    it('picking another payee drops the new one', async () => {
      renderPrompt()
      const payees = payeeServer()
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing())
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Blue Bottle (new)'))
      await user.click(screen.getByRole('combobox', { name: 'Payee' }))
      await user.click(await screen.findByRole('option', { name: 'Grocer' }))
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Grocer'))
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.createPayee).toBeUndefined()
      expect(payees).toHaveLength(1)
      expect(posted.create![0]).toMatchObject({ payeeId: 'p1' })
      expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_SUGGESTION_ACCEPT,
        { changedMatch: false, changedCategory: false, changedPayee: true })
    })

    it('with no category, Save is offered without a live count and saves a payee-only rule', async () => {
      renderPrompt()
      payeeServer()
      previewCounts(0, 0)
      acceptHandler(2)
      const user = await open(proposing({ categoryId: '' }))
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Payee' })).toHaveValue('Blue Bottle (new)'))
      await new Promise((r) => setTimeout(r, 400))
      expect(posted.preview).toBeUndefined()
      expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(posted.create![0]).toMatchObject({ categoryId: '', payeeId: 'payee-2' })
    })

    it('a Save retried after the payee it made was deleted elsewhere creates it again under a new id', async () => {
      renderPrompt()
      const payees = payeeServer()
      previewCounts(0, 0)
      let fail = true
      server.use(http.post('*/api/v1/import/accept-suggestion', async ({ request }) => {
        ;(posted.accept ??= []).push(await request.json())
        if (fail) return refused('boom')
        return HttpResponse.json({ success: true, message: '', data: { updated: 2 } })
      }))
      const user = await open(proposing())
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(posted.accept).toHaveLength(1))
      await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
      payees.splice(payees.findIndex((p) => p.id === 'payee-2'), 1)
      fail = false
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      const ids = (posted.createPayee as { id: string }[]).map((b) => b.id)
      expect(ids).toHaveLength(3)
      expect(ids[1]).toBe(ids[0])
      expect(ids[2]).not.toBe(ids[0])
      expect((posted.create as Record<string, unknown>[]).at(-1)).toMatchObject({ payeeId: 'payee-2' })
    })

    it('Cancel creates no payee', async () => {
      renderPrompt()
      payeeServer()
      const user = await open(proposing())
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(useUiStore.getState().rulePrompt).toBeNull())
      expect(posted.createPayee).toBeUndefined()
      expect(posted.create).toBeUndefined()
    })
  })

  it('cancel leaves the suggestion', async () => {
    renderPrompt()
    useUiStore.getState().setRulePrompt({ kind: 'suggestion', suggestion })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(useUiStore.getState().rulePrompt).toBeNull())
    expect(posted.create).toBeUndefined()
    expect(posted.accept).toBeUndefined()
  })
})

describe('AI match text', () => {
  beforeEach(() => { vi.mocked(trackEvent).mockClear() })
  const aiHandler = (matchText: string, matchField = 'description', gate?: Promise<void>) => server.use(
    http.post('*/api/v1/import/suggest-match-text', async () => {
      ;(posted.suggest ??= []).push(1)
      await gate
      return HttpResponse.json({ success: true, message: '', data: { matchField, matchText } })
    }),
  )
  const deferred = () => {
    let release: () => void = () => {}
    const promise = new Promise<void>((r) => { release = r })
    return { promise, release }
  }
  const matchTextKey = ['importMatchText', 'l1']
  // the AI answer is in the cache and every effect it triggers has run
  const answerLanded = async (qc: QueryClient) => {
    await waitFor(() => expect(qc.getQueryState(matchTextKey)?.status).toBe('success'))
    await act(async () => {})
  }
  // everything the dialog waits on before asking is loaded and rendered, so a call would have started
  const askingSettled = async (qc: QueryClient, rules = true) => {
    await waitFor(() => {
      expect(qc.getQueryState(queryKeys.user)?.status).toBe('success')
      if (rules) expect(qc.getQueryState(queryKeys.importRules)?.status).toBe('success')
    })
    await act(async () => {})
  }
  const notAsked = (qc: QueryClient) => {
    expect(qc.getQueryState(matchTextKey)?.fetchStatus ?? 'idle').toBe('idle')
    expect(qc.getQueryState(matchTextKey)?.status ?? 'pending').toBe('pending')
    expect(posted.suggest).toBeUndefined()
  }

  it('uses the AI match text when it arrives', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    renderPrompt()
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    await waitFor(() => expect((screen.getByLabelText('Description contains') as HTMLInputElement).value).toBe('BLUE BOTTLE'))
  })

  it('keeps what the user typed when the AI answer arrives later', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    const qc = renderPrompt()
    const ai = deferred()
    aiHandler('BLUE BOTTLE', 'description', ai.promise)
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    const user = userEvent.setup()
    const value = await screen.findByLabelText('Payee contains')
    await user.clear(value)
    await user.type(value, 'MINE')
    await waitFor(() => expect(posted.suggest).toHaveLength(1))
    ai.release()
    await answerLanded(qc)
    expect((screen.getByLabelText('Payee contains') as HTMLInputElement).value).toBe('MINE')
  })

  it('never calls suggest-match-text when AI is off', async () => {
    const qc = renderPrompt()
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    expect(await screen.findByLabelText('Payee contains')).toBeInTheDocument()
    await askingSettled(qc)
    notAsked(qc)
  })

  it('fires the AI-match metric when saved unchanged', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    renderPrompt()
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    const user = userEvent.setup()
    await waitFor(() => expect((screen.getByLabelText('Description contains') as HTMLInputElement).value).toBe('BLUE BOTTLE'))
    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    await waitFor(() => expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_RULE_AI_MATCH_USED))
  })

  it('does not call suggest-match-text offline', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    const { onlineManager } = await import('@tanstack/react-query')
    onlineManager.setOnline(false)
    try {
      const qc = renderPrompt()
      aiHandler('BLUE BOTTLE')
      useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
      expect(await screen.findByLabelText('Payee contains')).toBeInTheDocument()
      // offline nothing loads, so there is nothing to wait for: the field is up and no call started
      await act(async () => {})
      notAsked(qc)
    } finally {
      onlineManager.setOnline(true)
    }
  })

  it('does not call suggest-match-text for a readonly user', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    const qc = renderPrompt({ user: { ...fixtureUser, accessLevel: 'readonly' } })
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    expect(await screen.findByLabelText('Payee contains')).toBeInTheDocument()
    await askingSettled(qc)
    notAsked(qc)
  })

  it('does not call suggest-match-text for a user who opted out of AI', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    const qc = renderPrompt({ user: { ...fixtureUser, options: [...fixtureUser.options, { name: 'ai', value: '0' }] } })
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    expect(await screen.findByLabelText('Payee contains')).toBeInTheDocument()
    await askingSettled(qc)
    notAsked(qc)
  })

  it('never calls suggest-match-text for a link a rule classified', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    const qc = renderPrompt({ importRules: [rule] })
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link({ appliedRuleId: 'rule1', appliedCategoryId: 'cat-salary' }), diff: { categoryId: 'cat-food' } })
    expect(await screen.findByRole('button', { name: 'Update rule' })).toBeInTheDocument()
    await askingSettled(qc)
    notAsked(qc)
  })

  it('waits for the rule list before calling suggest-match-text', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    const qc = renderPrompt()
    let releaseRules: () => void = () => {}
    const rulesReady = new Promise<void>((r) => { releaseRules = r })
    server.use(http.get('*/api/v1/import/get-rule-list', async () => {
      await rulesReady
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }))
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    expect(await screen.findByLabelText('Payee contains')).toBeInTheDocument()
    await askingSettled(qc, false)
    notAsked(qc)
    releaseRules()
    await waitFor(() => expect(posted.suggest).toHaveLength(1))
  })

  it('ignores an AI answer that arrives once the save has started', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    const qc = renderPrompt()
    const ai = deferred()
    const created = deferred()
    aiHandler('BLUE BOTTLE', 'description', ai.promise)
    server.use(http.post('*/api/v1/import/create-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.create ??= []).push(body)
      await created.promise
      return HttpResponse.json({ success: true, message: '', data: { ...rule, ...body, id: 'rule-new' } })
    }))
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    const user = userEvent.setup()
    await screen.findByLabelText('Payee contains')
    await waitFor(() => expect(posted.suggest).toHaveLength(1))
    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    // the save is in flight; only now does the AI answer arrive
    await waitFor(() => expect(posted.create).toHaveLength(1))
    ai.release()
    await answerLanded(qc)
    created.release()
    expect(await screen.findByText('Apply this rule to 5 matching transactions in this import?')).toBeInTheDocument()
    const saved = posted.create![0] as Record<string, unknown>
    expect(saved.matchField).toBe('external_payee')
    // the answer landed while the create was in flight; it must not have moved the form
    const previews = posted.preview as Record<string, unknown>[]
    expect(previews.every((p) => p.matchField === 'external_payee' && p.matchValue === saved.matchValue)).toBe(true)
    expect(vi.mocked(trackEvent)).not.toHaveBeenCalledWith(METRICS.IMPORT_RULE_AI_MATCH_USED)
  })

  it('does not fire the AI-match metric when the user typed over the AI text', async () => {
    window.econumoConfig = { AI_ENABLED: true }
    renderPrompt()
    aiHandler('BLUE BOTTLE')
    useUiStore.getState().setRulePrompt({ kind: 'edit', link: link(), diff: { categoryId: 'cat-food' } })
    const user = userEvent.setup()
    const value = await screen.findByLabelText('Description contains')
    await waitFor(() => expect((value as HTMLInputElement).value).toBe('BLUE BOTTLE'))
    await user.type(value, 'X')
    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    await waitFor(() => expect(posted.create).toHaveLength(1))
    expect(vi.mocked(trackEvent)).not.toHaveBeenCalledWith(METRICS.IMPORT_RULE_AI_MATCH_USED)
  })
})
