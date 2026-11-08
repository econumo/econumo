import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureLabels, fixturePayees, fixtureTags } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import type { PaymentsFrom } from './useDebtForm'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const empty: PaymentsFrom = { categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null }

function renderEditor({ initial = empty, categoryLocked = false, errors = {} }: { initial?: PaymentsFrom; categoryLocked?: boolean; errors?: Record<string, string> } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  queryClient.setQueryData(queryKeys.categories, fixtureCategories)
  queryClient.setQueryData(queryKeys.payees, fixturePayees)
  queryClient.setQueryData(queryKeys.tags, fixtureTags)
  queryClient.setQueryData(queryKeys.labels, [...fixtureLabels, { ...fixtureLabels[0], id: 'label2', name: 'car' }])
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
        categoryLocked={categoryLocked}
        errors={errors}
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

it('reads as a sentence with the explainer under it', () => {
  renderEditor()
  expect(screen.getByText('Payments in')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Category: Food' })).toHaveTextContent('Food')
  expect(screen.getByRole('button', { name: 'Payee (optional): any payee' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Tag (optional): any tag' })).toBeInTheDocument()
  expect(screen.getByText("Econumo finds this loan's payments with these. They move from the category to this debt.")).toBeInTheDocument()
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

it('offers live expense categories only', async () => {
  const user = userEvent.setup()
  renderEditor({ initial: { ...empty, categoryId: null } })

  await user.click(screen.getByRole('button', { name: 'Category: choose a category' }))
  const sheet = await screen.findByRole('dialog', { name: 'Category' })
  expect(within(sheet).getByText('Food')).toBeInTheDocument()
  expect(within(sheet).queryByText('Salary')).toBeNull()
  expect(within(sheet).queryByText('Old')).toBeNull()
  // the category is required: there is no "any" row
  expect(within(sheet).queryByText(/^any/)).toBeNull()
})

it('a locked category cannot be changed', () => {
  renderEditor({ categoryLocked: true })
  expect(screen.getByRole('button', { name: 'Category: Food' })).toBeDisabled()
})

it('shows an error under the sentence', () => {
  renderEditor({ errors: { categoryId: 'Required field' } })
  expect(screen.getByText('Required field')).toBeInTheDocument()
})
