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

const access = vi.hoisted(() => ({ state: 'full_access' as 'full_access' | 'trial' | 'readonly' }))
vi.mock('@/features/user/queries', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/user/queries')>()),
  useAccessState: () => ({ state: access.state, accessUntil: '', daysLeft: null, billingEnabled: true }),
}))

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
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/import-rules', element: <ImportRulesPage /> }], { initialEntries: ['/settings/import-rules'] })
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  access.state = 'full_access'
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  for (const k of Object.keys(posted)) delete posted[k]
})

it('lists rules without bulk suggestions even with AI enabled', async () => {
  window.econumoConfig = { AI_ENABLED: true }
  renderPage()
  const rows = await screen.findAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getByText('Payee contains BLUE BOTTLE')).toBeInTheDocument()
  expect(within(rows[0]).getByText('Food · health')).toBeInTheDocument()
  expect(within(rows[1]).getByText('Description starts with PAYMENT THANK YOU')).toBeInTheDocument()
  expect(within(rows[1]).getByText('Matching transactions are not imported')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Suggest rules' })).not.toBeInTheDocument()
  // rule order carries meaning: no A-Z sort, and no archive switch
  expect(screen.queryByRole('button', { name: 'Reorder list' })).not.toBeInTheDocument()
  expect(screen.queryByRole('switch')).not.toBeInTheDocument()
})

it('creates a rule from the editor with a live match count', async () => {
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(within(dialog).getByLabelText('Match value'), 'IKEA')
  // a classify rule with nothing to set is not previewable yet
  await new Promise((r) => setTimeout(r, 400))
  expect(posted.preview).toBeUndefined()
  await user.click(within(dialog).getByRole('combobox', { name: 'Category' }))
  await user.click(await screen.findByRole('option', { name: 'Food' }))
  expect(within(dialog).getByText('Only expense transactions get this category')).toBeInTheDocument()
  await user.click(within(dialog).getByRole('checkbox', { name: /health/ }))
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
  await user.click(within(dialog).getByRole('radio', { name: 'Skip' }))
  expect(within(dialog).queryByRole('combobox', { name: 'Category' })).not.toBeInTheDocument()
  await user.type(within(dialog).getByLabelText('Match value'), 'TRANSFER')
  await user.click(within(dialog).getByRole('button', { name: 'Case sensitive' }))
  expect(within(dialog).getByRole('button', { name: 'Case sensitive' })).toHaveAttribute('aria-pressed', 'true')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ action: 'skip', matchValue: 'TRANSFER', isCaseSensitive: true, categoryId: '', payeeId: '', tagId: '', labelIds: [] }))
})

it('edits and deletes a rule', async () => {
  renderPage()
  const user = userEvent.setup()
  const rows = await screen.findAllByRole('listitem')
  await user.click(within(rows[0]).getByRole('button', { name: 'actions Payee contains BLUE BOTTLE' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog')
  const value = within(dialog).getByLabelText('Match value') as HTMLInputElement
  expect(value.value).toBe('BLUE BOTTLE')
  await user.clear(value)
  await user.type(value, 'BLUE BOTTLE COFFEE')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.update![0]).toMatchObject({ id: 'rule1', matchValue: 'BLUE BOTTLE COFFEE', priority: 0 }))

  await user.click(within((await screen.findAllByRole('listitem'))[1]).getByRole('button', { name: 'actions Description starts with PAYMENT THANK YOU' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Delete this rule?')).toBeInTheDocument()
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(posted.delete![0]).toEqual({ id: 'rule2' }))
  await waitFor(() => expect(screen.getAllByRole('listitem')).toHaveLength(1))
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

it('creates a category and a payee from the editor, the category of the chosen kind', async () => {
  const created: Record<string, Record<string, unknown>> = {}
  renderPage()
  server.use(
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      created.category = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'cat-new', ownerUserId: 'u1', name: 'Bonus', position: 9, type: 'income', icon: '', isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' } } })
    }),
    http.post('*/api/v1/payee/create-payee', async ({ request }) => {
      created.payee = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'payee-new', ownerUserId: 'u1', name: 'ACME', position: 9, isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' } } })
    }),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(within(dialog).getByLabelText('Match value'), 'ACME PAYROLL')

  // the picker shows one kind at a time; switching it offers the other kind's rows
  await user.click(within(dialog).getByRole('combobox', { name: 'Category' }))
  expect(await screen.findByRole('option', { name: 'Food' })).toBeInTheDocument()
  expect(screen.queryByRole('option', { name: 'Salary' })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')
  await user.click(within(dialog).getByRole('radio', { name: 'Income' }))
  await user.click(within(dialog).getByRole('combobox', { name: 'Category' }))
  expect(await screen.findByRole('option', { name: 'Salary' })).toBeInTheDocument()
  await user.keyboard('Bonus')
  await user.click(await screen.findByText(/Add «Bonus»/))
  await waitFor(() => expect(created.category).toMatchObject({ name: 'Bonus', type: 'income' }))
  await waitFor(() => expect(within(dialog).getByRole('combobox', { name: 'Category' })).toHaveValue('Bonus'))
  expect(within(dialog).getByText('Only income transactions get this category')).toBeInTheDocument()

  await user.click(within(dialog).getByRole('combobox', { name: 'Payee' }))
  await user.keyboard('ACME')
  await user.click(await screen.findByText(/Add «ACME»/))
  await waitFor(() => expect(created.payee).toMatchObject({ name: 'ACME' }))
  await waitFor(() => expect(within(dialog).getByRole('combobox', { name: 'Payee' })).toHaveValue('ACME'))

  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(posted.create![0]).toMatchObject({ matchValue: 'ACME PAYROLL', categoryId: 'cat-new', payeeId: 'payee-new' }))
})

it('a read-only user cannot create classifications from the editor', async () => {
  access.state = 'readonly'
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('combobox', { name: 'Category' }))
  await user.keyboard('Bonus')
  expect(screen.queryByText(/Add «Bonus»/)).not.toBeInTheDocument()
  expect(within(dialog).queryByRole('button', { name: 'Add tag' })).not.toBeInTheDocument()
})

it('switching the category type clears the picked category', async () => {
  renderPage()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Add rule' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('combobox', { name: 'Category' }))
  await user.click(await screen.findByRole('option', { name: 'Food' }))
  expect(within(dialog).getByRole('combobox', { name: 'Category' })).toHaveValue('Food')
  await user.click(within(dialog).getByRole('radio', { name: 'Income' }))
  expect(within(dialog).getByRole('combobox', { name: 'Category' })).toHaveValue('')
  expect(within(dialog).queryByText('Only expense transactions get this category')).not.toBeInTheDocument()
  // re-selecting the current type keeps a pick
  await user.click(within(dialog).getByRole('combobox', { name: 'Category' }))
  await user.click(await screen.findByRole('option', { name: 'Salary' }))
  await user.click(within(dialog).getByRole('radio', { name: 'Income' }))
  expect(within(dialog).getByRole('combobox', { name: 'Category' })).toHaveValue('Salary')
})
