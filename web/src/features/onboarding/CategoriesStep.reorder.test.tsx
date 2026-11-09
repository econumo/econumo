import type { ReactNode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories } from '@/test/fixtures'
import { CategoriesStep, CategoriesStepBody, type ExistingCategory } from './CategoriesStep'
import { emptyDraft, saveDraft, type CategoryRow } from './draft'

// jsdom has no layout, so dnd-kit cannot complete a drag; this stands in for the drop
// with a "move up" button per row reporting the order a finished drag would.
vi.mock('@/components/SortableList', () => ({
  SortableList: <T extends { id: string }>({
    items,
    onReorder,
    renderItem,
    disabled,
  }: {
    items: T[]
    onReorder: (orderedIds: string[], movedId: string) => void
    renderItem: (item: T, handle: { attributes: Record<string, never>; listeners: undefined }) => ReactNode
    disabled?: boolean
  }) => (
    <ul>
      {items.map((item, i) => (
        <li key={item.id}>
          {renderItem(item, { attributes: {}, listeners: undefined })}
          {!disabled && i > 0 && (
            <button
              type="button"
              aria-label={`move-up ${item.id}`}
              onClick={() => {
                const ids = items.map((x) => x.id)
                ;[ids[i - 1], ids[i]] = [ids[i], ids[i - 1]]
                onReorder(ids, item.id)
              }}
            />
          )}
        </li>
      ))}
    </ul>
  ),
}))

function renderStep(props: Partial<React.ComponentProps<typeof CategoriesStep>> = {}) {
  const onContinue = vi.fn()
  render(<CategoriesStep existing={[]} onContinue={onContinue} onSkip={vi.fn()} pending={false} {...props} />)
  return { onContinue }
}

const food: ExistingCategory = { id: 'cat-1', name: 'Groceries', type: 'expense', icon: 'cake' }
const rowsOf = (type: 'expense' | 'income') => within(screen.getByTestId(`group-${type}`)).getAllByTestId('category-row')
const nameOf = (row: HTMLElement) => within(row).getByRole('checkbox').getAttribute('aria-label')
const names = (type: 'expense' | 'income') => rowsOf(type).map(nameOf)
const keyOf = (rows: CategoryRow[], name: string) => rows.find((r) => r.name === name)!.key
const moveUp = (user: ReturnType<typeof userEvent.setup>, rowName: string) => {
  const row = rowsOf('expense').find((r) => nameOf(r) === rowName)!
  return user.click(within(row.parentElement!).getByRole('button', { name: /^move-up / }))
}

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('moves a row within its group and submits rows and the full order in that order', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await moveUp(user, 'Pets')
  expect(names('expense').slice(7, 10)).toEqual(['Entertainment', 'Pets', 'Kids'])
  expect(names('income')).toEqual(['Salary', 'Bonus', 'Other income'])
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [rows, order] = onContinue.mock.calls[0] as [CategoryRow[], string[]]
  expect(rows.filter((r) => r.type === 'expense').map((r) => r.name).slice(7, 10)).toEqual(['Entertainment', 'Pets', 'Kids'])
  expect(order).toEqual(rows.map((r) => r.key))
  expect(order.indexOf(keyOf(rows, 'Pets'))).toBeLessThan(order.indexOf(keyOf(rows, 'Kids')))
})

it('moves a new row above an existing locked category', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [food] })
  expect(names('expense')[0]).toBe('Groceries')
  await moveUp(user, 'Housing')
  expect(names('expense').slice(0, 2)).toEqual(['Housing', 'Groceries'])
  await user.click(screen.getByRole('checkbox', { name: 'Housing' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [rows, order] = onContinue.mock.calls[0] as [CategoryRow[], string[]]
  expect(order.slice(0, 2)).toEqual([keyOf(rows, 'Housing'), 'cat-1'])
})

it('passes no order when nothing was moved', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][1]).toBeUndefined()
})

it('offers no reordering on a phone or tablet width', () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  renderStep()
  expect(screen.queryByRole('button', { name: /^move-up / })).not.toBeInTheDocument()
})

it('restores a stored order and submits it again', async () => {
  const user = userEvent.setup()
  const initialRows: CategoryRow[] = [{ key: 'k1', name: 'Pets', type: 'expense', icon: 'pets', checked: true }]
  const { onContinue } = renderStep({ existing: [food], initialRows, initialOrder: ['k1', 'cat-1'] })
  expect(names('expense')).toEqual(['Pets', 'Groceries'])
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][1]).toEqual(['k1', 'cat-1'])
})

it('maps a created row of the stored order to its category, so a revisit keeps the order', async () => {
  saveDraft('u1', {
    ...emptyDraft(),
    categoryRows: [
      { key: 'k1', name: 'Food', type: 'expense', icon: 'restaurant', checked: true },
      { key: 'k2', name: 'Fun', type: 'expense', icon: 'movie', checked: true },
    ],
    createdCategories: { k1: 'cat-food' },
    categoryOrder: ['k2', 'k1'],
  })
  server.use(...coreHandlers({ categories: fixtureCategories }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <CategoriesStepBody onDone={vi.fn(() => Promise.resolve())} onSkip={vi.fn(() => Promise.resolve())} />
    </QueryClientProvider>,
  )
  await screen.findByRole('heading', { name: 'What do you spend on?' })
  await waitFor(() => expect(names('expense')).toEqual(['Fun', 'Food']))
  localStorage.clear()
})

it('keeps the order once created rows come back as existing categories, and a retry submits it', async () => {
  const user = userEvent.setup()
  const onContinue = vi.fn()
  const initialRows: CategoryRow[] = [
    { key: 'k1', name: 'Pets', type: 'expense', icon: 'pets', checked: true },
    { key: 'k2', name: 'Kids', type: 'expense', icon: 'child_care', checked: true },
  ]
  const props = { onContinue, onSkip: vi.fn(), pending: false, initialRows }
  const { rerender } = render(<CategoriesStep existing={[food]} {...props} />)
  await moveUp(user, 'Kids')
  await moveUp(user, 'Kids')
  expect(names('expense')).toEqual(['Kids', 'Groceries', 'Pets'])
  rerender(
    <CategoriesStep
      {...props}
      existing={[food, { id: 'new-1', name: 'Pets', type: 'expense', icon: 'pets' }, { id: 'new-2', name: 'Kids', type: 'expense', icon: 'child_care' }]}
      createdKeys={new Set(['k1', 'k2'])}
      createdIds={{ k1: 'new-1', k2: 'new-2' }}
    />,
  )
  expect(names('expense')).toEqual(['Kids', 'Groceries', 'Pets'])
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][1]).toEqual(['new-2', 'cat-1', 'new-1'])
})
