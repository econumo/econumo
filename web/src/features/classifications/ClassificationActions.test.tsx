import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import type { ClassificationType } from '@/lib/search'
import type { CategoryDto } from '@/api/dto/category'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { LabelDto } from '@/api/dto/label'
import { ClassificationActionsMenu } from './ClassificationActions'

const stamp = { createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }
const categories: CategoryDto[] = [
  { id: 'c1', ownerUserId: 'u1', name: 'Food', position: 0, type: 'expense', icon: 'restaurant', isArchived: 0, ...stamp },
  { id: 'c2', ownerUserId: 'u1', name: 'Groceries', position: 1, type: 'expense', icon: 'shopping_cart', isArchived: 0, ...stamp },
  { id: 'c3', ownerUserId: 'u1', name: 'Salary', position: 2, type: 'income', icon: 'payments', isArchived: 0, ...stamp },
  { id: 'c4', ownerUserId: 'u2', name: 'Partner food', position: 3, type: 'expense', icon: 'restaurant', isArchived: 0, ...stamp },
  { id: 'c5', ownerUserId: 'u1', name: 'Old', position: 4, type: 'expense', icon: 'delete', isArchived: 1, ...stamp },
]
const payees: PayeeDto[] = [
  { id: 'p1', ownerUserId: 'u1', name: 'Grocer', position: 0, isArchived: 0, ...stamp },
  { id: 'p2', ownerUserId: 'u1', name: 'Butcher', position: 1, isArchived: 0, ...stamp },
  { id: 'p3', ownerUserId: 'u2', name: 'Partner shop', position: 2, isArchived: 0, ...stamp },
]
const tags: TagDto[] = [
  { id: 't1', ownerUserId: 'u1', name: 'vacation', icon: 'tag', position: 0, isArchived: 0, ...stamp },
  { id: 't2', ownerUserId: 'u1', name: 'work', icon: 'tag', position: 1, isArchived: 0, ...stamp },
  { id: 't3', ownerUserId: 'u2', name: 'partner tag', icon: 'tag', position: 2, isArchived: 0, ...stamp },
]
const labels: LabelDto[] = [
  { id: 'l1', ownerUserId: 'u1', name: 'health', icon: 'sell', position: 0, isArchived: 0, ...stamp },
  { id: 'l2', ownerUserId: 'u1', name: 'sport', icon: 'sell', position: 1, isArchived: 0, ...stamp },
  { id: 'l3', ownerUserId: 'u2', name: 'partner label', icon: 'sell', position: 2, isArchived: 0, ...stamp },
]

const rowClick = vi.fn()

function renderMenu(type: ClassificationType, item: CategoryDto | PayeeDto | TagDto | LabelDto) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <div onClick={rowClick}>
          <ClassificationActionsMenu type={type} item={item} />
        </div>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const ok = () => HttpResponse.json({ success: true, message: '', data: {} })

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers({ categories, payees, tags, labels }))
  rowClick.mockClear()
})

const cases: {
  type: ClassificationType
  item: CategoryDto | PayeeDto | TagDto | LabelDto
  module: string
  deleteTitle: string
  deleteBody: unknown
  // own items of the same kind, minus the source
  candidates: string[]
  excluded: string[]
}[] = [
  {
    type: 'category', item: categories[0], module: 'category', deleteTitle: 'Delete category?',
    deleteBody: { id: 'c1', mode: 'delete' }, candidates: ['Groceries', 'Old'], excluded: ['Food', 'Salary', 'Partner food'],
  },
  {
    type: 'payee', item: payees[0], module: 'payee', deleteTitle: 'Delete payee?',
    deleteBody: { id: 'p1' }, candidates: ['Butcher'], excluded: ['Grocer', 'Partner shop'],
  },
  {
    type: 'tag', item: tags[0], module: 'tag', deleteTitle: 'Delete tag?',
    deleteBody: { id: 't1' }, candidates: ['work'], excluded: ['vacation', 'partner tag', 'health'],
  },
  {
    type: 'label', item: labels[0], module: 'label', deleteTitle: 'Delete tag?',
    deleteBody: { id: 'l1' }, candidates: ['sport'], excluded: ['health', 'partner label', 'vacation'],
  },
]

describe.each(cases)('$type', ({ type, item, module, deleteTitle, deleteBody, candidates, excluded }) => {
  it('offers Edit, Archive, Merge and Delete; Archive posts the archive endpoint', async () => {
    let archived: unknown
    server.use(
      http.post(`*/api/v1/${module}/archive-${module}`, async ({ request }) => {
        archived = await request.json()
        return ok()
      }),
    )
    const user = userEvent.setup()
    renderMenu(type, item)
    await user.click(screen.getByRole('button', { name: `actions ${item.name}` }))
    expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Merge into…' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Unarchive' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('menuitem', { name: 'Archive' }))
    await waitFor(() => expect(archived).toEqual({ id: item.id }))
    expect(rowClick).not.toHaveBeenCalled()
  })

  it('an archived item offers Unarchive, which posts the unarchive endpoint', async () => {
    let unarchived: unknown
    server.use(
      http.post(`*/api/v1/${module}/unarchive-${module}`, async ({ request }) => {
        unarchived = await request.json()
        return ok()
      }),
    )
    const user = userEvent.setup()
    renderMenu(type, { ...item, isArchived: 1 })
    await user.click(screen.getByRole('button', { name: `actions ${item.name}` }))
    expect(screen.queryByRole('menuitem', { name: 'Archive' })).not.toBeInTheDocument()
    await user.click(await screen.findByRole('menuitem', { name: 'Unarchive' }))
    await waitFor(() => expect(unarchived).toEqual({ id: item.id }))
  })

  it('Delete confirms, then posts the delete endpoint', async () => {
    let deleted: unknown
    server.use(
      http.post(`*/api/v1/${module}/delete-${module}`, async ({ request }) => {
        deleted = await request.json()
        return ok()
      }),
    )
    const user = userEvent.setup()
    renderMenu(type, item)
    await user.click(screen.getByRole('button', { name: `actions ${item.name}` }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    expect(await screen.findByText(deleteTitle)).toBeInTheDocument()
    expect(deleted).toBeUndefined()
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(deleted).toEqual(deleteBody))
    expect(rowClick).not.toHaveBeenCalled()
  })

  it('Merge offers own items of the same kind, minus the source, and posts the merge', async () => {
    let merged: unknown
    server.use(
      http.post(`*/api/v1/${module}/merge-${module}`, async ({ request }) => {
        merged = await request.json()
        return ok()
      }),
    )
    const user = userEvent.setup()
    renderMenu(type, item)
    await user.click(screen.getByRole('button', { name: `actions ${item.name}` }))
    await user.click(await screen.findByRole('menuitem', { name: 'Merge into…' }))
    await screen.findByRole('option', { name: candidates[0] })
    expect(screen.getAllByRole('option')).toHaveLength(candidates.length)
    for (const name of candidates) {
      expect(screen.getByRole('option', { name })).toBeInTheDocument()
    }
    for (const name of excluded) {
      expect(screen.queryByRole('option', { name })).not.toBeInTheDocument()
    }
    await user.click(screen.getByRole('option', { name: candidates[0] }))
    await user.click(screen.getByRole('button', { name: 'Merge' }))
    const target = [...categories, ...payees, ...tags, ...labels].find((i) => i.name === candidates[0])!
    await waitFor(() => expect(merged).toEqual({ sourceId: item.id, targetId: target.id }))
    expect(rowClick).not.toHaveBeenCalled()
  })
})

it('category Edit opens the category dialog and posts id/name/icon', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/category/update-category', async ({ request }) => {
      body = await request.json()
      return ok()
    }),
  )
  const user = userEvent.setup()
  renderMenu('category', categories[0])
  await user.click(screen.getByRole('button', { name: 'actions Food' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const input = await screen.findByLabelText('Name')
  await waitFor(() => expect(input).toHaveValue('Food'))
  await user.clear(input)
  await user.type(input, 'Eating out')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toEqual({ id: 'c1', name: 'Eating out', icon: 'restaurant' }))
  expect(rowClick).not.toHaveBeenCalled()
})

it('payee Edit validates the name like the payees page, then posts id/name', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/payee/update-payee', async ({ request }) => {
      body = await request.json()
      return ok()
    }),
  )
  const user = userEvent.setup()
  renderMenu('payee', payees[0])
  await user.click(screen.getByRole('button', { name: 'actions Grocer' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(await screen.findByText('Edit payee')).toBeInTheDocument()
  const input = screen.getByLabelText('Name')
  expect(input).toHaveValue('Grocer')
  await user.clear(input)
  await user.type(input, 'a')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  expect(await screen.findByText('Payee name must be 3-64 characters')).toBeInTheDocument()
  await user.clear(input)
  await user.type(input, 'Greengrocer')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toEqual({ id: 'p1', name: 'Greengrocer' }))
})

it.each([
  ['tag', tags[0], 'tag'],
  ['label', labels[0], 'label'],
] as const)('%s Edit opens the tag dialog and posts to its own update endpoint', async (type, item, module) => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post(`*/api/v1/${module}/update-${module}`, async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return ok()
    }),
  )
  const user = userEvent.setup()
  renderMenu(type, item)
  await user.click(screen.getByRole('button', { name: `actions ${item.name}` }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const input = await screen.findByLabelText('Name')
  await waitFor(() => expect(input).toHaveValue(item.name))
  await user.clear(input)
  await user.type(input, 'renamed')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toMatchObject({ id: item.id, name: 'renamed' }))
})
