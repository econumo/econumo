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

it('PaymentsFromEditor offers label and shows "any reporting tag" when empty', async () => {
  const user = userEvent.setup()
  const changes = renderEditor()
  const label = screen.getByRole('button', { name: 'Reporting tag (optional): any reporting tag' })
  expect(label).toHaveTextContent('any reporting tag')

  await user.click(label)
  const sheet = await screen.findByRole('dialog', { name: 'Reporting tag (optional)' })
  // the search narrows the list by name
  await user.type(within(sheet).getByRole('combobox'), 'car')
  expect(within(sheet).queryByText('health')).toBeNull()
  await user.click(within(sheet).getByText('car'))

  expect(changes.at(-1)).toEqual({ ...empty, labelId: 'label2' })
  expect(screen.getByRole('button', { name: 'Reporting tag (optional): car' })).toBeInTheDocument()
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
