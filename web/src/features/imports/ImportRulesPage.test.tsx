import type { ReactNode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import type { ImportRuleDto } from '@/api/dto/imports'
import { ImportRulesPage } from './ImportRulesPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

// jsdom cannot drive a real dnd-kit drag (no layout for collision detection),
// so this stands in for the drop: it renders the real row (via renderItem,
// exercising the real ImportRulesPage reorder wiring) plus a "move down"
// trigger that fires the exact onReorder(orderedIds, movedId) shape a
// completed keyboard drag one slot down would report.
vi.mock('@/components/SortableList', () => ({
  SortableList: <T extends { id: string }>({
    items,
    onReorder,
    renderItem,
  }: {
    items: T[]
    onReorder: (orderedIds: string[], movedId: string) => void
    renderItem: (item: T, handle: { attributes: Record<string, never>; listeners: undefined }) => ReactNode
  }) => (
    <ul>
      {items.map((item, index) => (
        <li key={item.id}>
          {renderItem(item, { attributes: {}, listeners: undefined })}
          {index < items.length - 1 ? (
            <button
              type="button"
              aria-label={`move-down ${item.id}`}
              onClick={() => {
                const ids = items.map((i) => i.id)
                ;[ids[index], ids[index + 1]] = [ids[index + 1], ids[index]]
                onReorder(ids, item.id)
              }}
            />
          ) : null}
        </li>
      ))}
    </ul>
  ),
}))

const rule = (over: Partial<ImportRuleDto> = {}): ImportRuleDto => ({
  id: 'rule1', sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE',
  isCaseSensitive: false, categoryId: 'cat-food', payeeId: '', tagId: '', labelIds: ['label1'], priority: 0,
  createdAt: '2026-09-01 00:00:00', updatedAt: '2026-09-01 00:00:00', ...over,
})
const skipRule = rule({ id: 'rule2', action: 'skip', matchField: 'description', matchType: 'prefix', matchValue: 'PAYMENT THANK YOU', categoryId: '', labelIds: [], priority: 1 })

const posted: Record<string, unknown[]> = {}
function renderPage(data: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ importRules: [rule(), skipRule], ...data }))
  server.use(
    http.post('*/api/v1/import/preview-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.preview ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: { matched: 7, alreadyEdited: 1 } })
    }),
    http.post('*/api/v1/import/create-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.create ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: rule({ ...(body as Partial<ImportRuleDto>), id: `rule-${posted.create!.length}` }) })
    }),
    http.post('*/api/v1/import/update-rule', async ({ request }) => {
      const body = await request.json() as Record<string, unknown>
      ;(posted.update ??= []).push(body)
      return HttpResponse.json({ success: true, message: '', data: rule(body as Partial<ImportRuleDto>) })
    }),
    http.post('*/api/v1/import/delete-rule', async ({ request }) => {
      ;(posted.delete ??= []).push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: null })
    }),
    http.post('*/api/v1/import/suggest-rules', async ({ request }) => {
      ;(posted.suggest ??= []).push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: { items: [
        { sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'UBER', isCaseSensitive: false, categoryId: 'cat-food', payeeId: '', tagId: '', labelIds: [], priority: 0, reason: '4 rides last month' },
      ] } })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/import-rules', element: <ImportRulesPage /> }], { initialEntries: ['/settings/import-rules'] })
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  for (const k of Object.keys(posted)) delete posted[k]
})

it('lists rules in priority order with skip rules marked and targets named', async () => {
  renderPage()
  const rows = await screen.findAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getByText('Payee contains BLUE BOTTLE')).toBeInTheDocument()
  expect(within(rows[0]).getByText('Food · health')).toBeInTheDocument()
  expect(within(rows[1]).getByText('Description starts with PAYMENT THANK YOU')).toBeInTheDocument()
  expect(within(rows[1]).getByText('Skip')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Suggest rules' })).not.toBeInTheDocument()
})

it('creates a rule from the editor with a live match count', async () => {
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(within(dialog).getByLabelText('Match value'), 'IKEA')
  await user.selectOptions(within(dialog).getByLabelText('Category'), 'cat-food')
  await user.click(within(dialog).getByRole('checkbox', { name: 'health' }))
  expect(await within(dialog).findByText('Matches 7 imported transactions')).toBeInTheDocument()
  expect(posted.preview!.at(-1)).toMatchObject({ matchValue: 'IKEA', scope: 'all', runId: '', scopeSourceId: '' })
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'IKEA', categoryId: 'cat-food', labelIds: ['label1'], priority: 2 }))
  expect(await screen.findByText('Payee contains IKEA')).toBeInTheDocument()
})

it('a skip rule hides the targets and posts none', async () => {
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.selectOptions(within(dialog).getByLabelText('Action'), 'skip')
  expect(within(dialog).queryByLabelText('Category')).not.toBeInTheDocument()
  await user.type(within(dialog).getByLabelText('Match value'), 'TRANSFER')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ action: 'skip', matchValue: 'TRANSFER', categoryId: '', payeeId: '', tagId: '', labelIds: [] }))
})

it('edits and deletes a rule', async () => {
  renderPage()
  const user = userEvent.setup()
  const rows = await screen.findAllByRole('listitem')
  await user.click(within(rows[0]).getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog')
  const value = within(dialog).getByLabelText('Match value') as HTMLInputElement
  expect(value.value).toBe('BLUE BOTTLE')
  await user.clear(value)
  await user.type(value, 'BLUE BOTTLE COFFEE')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1', matchValue: 'BLUE BOTTLE COFFEE', priority: 0 }))

  await user.click(within((await screen.findAllByRole('listitem'))[1]).getByRole('button', { name: 'Delete' }))
  await user.click(await screen.findByRole('button', { name: 'Delete rule' }))
  await waitFor(() => expect(posted.delete![0]).toEqual({ id: 'rule2' }))
  await waitFor(() => expect(screen.getAllByRole('listitem')).toHaveLength(1))
})

it('with AI enabled, suggestions can be accepted or discarded', async () => {
  window.econumoConfig = { AI_ENABLED: true }
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Suggest rules' }))
  await waitFor(() => expect(posted.suggest![0]).toEqual({ scope: 'all', runId: '', scopeSourceId: '' }))
  const suggestion = await screen.findByRole('region', { name: 'Suggested rules' })
  expect(within(suggestion).getByText('Payee contains UBER')).toBeInTheDocument()
  expect(within(suggestion).getByText('4 rides last month')).toBeInTheDocument()
  expect(await within(suggestion).findByText('Matches 7 imported transactions')).toBeInTheDocument()
  await user.click(within(suggestion).getByRole('button', { name: 'Accept' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ matchValue: 'UBER', categoryId: 'cat-food', priority: 2 }))
  await waitFor(() => expect(screen.queryByRole('region', { name: 'Suggested rules' })).not.toBeInTheDocument())
})

it('reordering persists the new priorities', async () => {
  renderPage()
  const user = userEvent.setup()
  const rows = await screen.findAllByRole('listitem')
  await user.click(within(rows[0]).getByRole('button', { name: 'move-down rule1' }))
  await waitFor(() => expect(posted.update).toHaveLength(2))
  expect(posted.update).toEqual(expect.arrayContaining([
    expect.objectContaining({ id: 'rule2', priority: 0 }),
    expect.objectContaining({ id: 'rule1', priority: 1 }),
  ]))
})
