import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import type { PaymentsFromChoices } from './PaymentsFromEditor'
import type { PaymentsFrom } from './rules'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const empty: PaymentsFrom = { categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null }

const choices: PaymentsFromChoices = {
  categories: [{ options: [{ id: 'cat-food', name: 'Food', icon: 'restaurant' }] }],
  payees: [{ id: 'p1', name: 'Grocer' }],
  tags: [{ id: 'tag1', name: 'vacation', icon: 'tag' }],
  labels: [{ id: 'label1', name: 'health', icon: 'sell' }, { id: 'label2', name: 'car', icon: 'sell' }],
}

function renderEditor({ initial = empty, categoryLocked = false, onRemove }: { initial?: PaymentsFrom; categoryLocked?: boolean; onRemove?: () => void } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const changes: PaymentsFrom[] = []
  function Host() {
    const [value, setValue] = useState(initial)
    return (
      <PaymentsFromEditor
        value={value}
        onChange={(next) => {
          changes.push(next)
          setValue(next)
        }}
        choices={choices}
        categoryLocked={categoryLocked}
        onRemove={onRemove}
      />
    )
  }
  render(
    <QueryClientProvider client={queryClient}>
      <Host />
    </QueryClientProvider>,
  )
  return changes
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
})

it('reads as a sentence with no explainer under it', () => {
  renderEditor()
  expect(screen.getByText('Payments in')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Category: Food' })).toHaveTextContent('Food')
  expect(screen.getByRole('button', { name: 'Payee (optional): any payee' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Tag (optional): any tag' })).toBeInTheDocument()
  expect(screen.queryByText(/Econumo finds this loan's payments/)).toBeNull()
})

it('one tag slot lists tags and reporting tags in two groups', async () => {
  const user = userEvent.setup()
  renderEditor()
  expect(screen.queryByRole('button', { name: /^Reporting tag/ })).toBeNull()

  await user.click(screen.getByRole('button', { name: 'Tag (optional): any tag' }))
  const sheet = await screen.findByRole('dialog', { name: 'Tag (optional)' })
  expect(within(sheet).getByText('Tags')).toBeInTheDocument()
  expect(within(sheet).getByText('Reporting tags')).toBeInTheDocument()
  expect(within(sheet).getByText('vacation')).toBeInTheDocument()
  expect(within(sheet).getByText('health')).toBeInTheDocument()
})

it('the search narrows both groups by name', async () => {
  const user = userEvent.setup()
  renderEditor()
  await user.click(screen.getByRole('button', { name: 'Tag (optional): any tag' }))
  const sheet = await screen.findByRole('dialog', { name: 'Tag (optional)' })
  await user.type(within(sheet).getByRole('combobox'), 'car')
  expect(within(sheet).queryByText('health')).toBeNull()
  expect(within(sheet).queryByText('vacation')).toBeNull()
  expect(within(sheet).queryByText('Tags')).toBeNull()
  expect(within(sheet).getByText('car')).toBeInTheDocument()
})

it('picking a reporting tag sets the label and clears the tag', async () => {
  const user = userEvent.setup()
  const changes = renderEditor({ initial: { ...empty, tagId: 'tag1' } })

  await user.click(screen.getByRole('button', { name: 'Tag (optional): vacation' }))
  const sheet = await screen.findByRole('dialog', { name: 'Tag (optional)' })
  await user.click(within(sheet).getByText('car'))

  expect(changes.at(-1)).toEqual({ ...empty, labelId: 'label2' })
  expect(screen.getByRole('button', { name: 'Tag (optional): car' })).toBeInTheDocument()
})

it('picking a tag sets the tag and clears the label', async () => {
  const user = userEvent.setup()
  const changes = renderEditor({ initial: { ...empty, labelId: 'label1' } })

  await user.click(screen.getByRole('button', { name: 'Tag (optional): health' }))
  const sheet = await screen.findByRole('dialog', { name: 'Tag (optional)' })
  await user.click(within(sheet).getByText('vacation'))

  expect(changes.at(-1)).toEqual({ ...empty, tagId: 'tag1' })
})

it('any tag clears both', async () => {
  const user = userEvent.setup()
  const changes = renderEditor({ initial: { ...empty, labelId: 'label1' } })

  await user.click(screen.getByRole('button', { name: 'Tag (optional): health' }))
  const sheet = await screen.findByRole('dialog', { name: 'Tag (optional)' })
  await user.click(within(sheet).getByText('any tag'))

  expect(changes.at(-1)).toEqual(empty)
})

it('an optional filter can go back to any', async () => {
  const user = userEvent.setup()
  const changes = renderEditor({ initial: { ...empty, payeeId: 'p1' } })

  await user.click(screen.getByRole('button', { name: 'Payee (optional): Grocer' }))
  const sheet = await screen.findByRole('dialog', { name: 'Payee (optional)' })
  await user.click(within(sheet).getByText('any payee'))

  expect(changes.at(-1)).toEqual(empty)
})

it('the category is required: its picker has no "any" row', async () => {
  const user = userEvent.setup()
  renderEditor({ initial: { ...empty, categoryId: null } })

  await user.click(screen.getByRole('button', { name: 'Category: choose a category' }))
  const sheet = await screen.findByRole('dialog', { name: 'Category' })
  expect(within(sheet).getByText('Food')).toBeInTheDocument()
  expect(within(sheet).queryByText(/^any/)).toBeNull()
})

it('a locked category cannot be changed', () => {
  renderEditor({ categoryLocked: true })
  expect(screen.getByRole('button', { name: 'Category: Food' })).toBeDisabled()
})

it('a removable sentence has a remove button', async () => {
  const user = userEvent.setup()
  const onRemove = vi.fn()
  renderEditor({ onRemove })
  await user.click(screen.getByRole('button', { name: 'Remove this category' }))
  expect(onRemove).toHaveBeenCalledOnce()
})
