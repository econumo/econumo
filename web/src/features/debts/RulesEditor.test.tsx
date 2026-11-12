import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import type { DebtRuleDto } from '@/api/dto/debt'
import { RulesEditor } from './RulesEditor'
import type { RuleDraft } from './rules'
import { wireRule, wireRuleOptions } from './testDebt'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const food: RuleDraft = { categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null }

function renderEditor({ initial = [food], initialRules, readOnly = false, lockedCategoryId, error }: {
  initial?: RuleDraft[]
  initialRules?: DebtRuleDto[]
  readOnly?: boolean
  lockedCategoryId?: string
  error?: string
} = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  const changes: RuleDraft[][] = []
  function Host() {
    const [value, setValue] = useState(initial)
    return (
      <RulesEditor
        value={value}
        onChange={(next) => {
          changes.push(next)
          setValue(next)
        }}
        options={wireRuleOptions}
        initialRules={initialRules}
        readOnly={readOnly}
        lockedCategoryId={lockedCategoryId}
        error={error}
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

it('adds a second rule, removes the first, and drops the remove button at one rule', async () => {
  const user = userEvent.setup()
  const changes = renderEditor()
  // one rule: nothing to remove
  expect(screen.queryByRole('button', { name: 'Remove this category' })).toBeNull()

  await user.click(screen.getByRole('button', { name: 'Add a category' }))
  expect(screen.getAllByRole('button', { name: /^Category:/ })).toHaveLength(2)
  expect(screen.getByRole('button', { name: 'Category: choose a category' })).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Category: choose a category' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Category' })).getByText('Mortgage'))
  expect(screen.getAllByRole('button', { name: 'Remove this category' })).toHaveLength(2)

  await user.click(screen.getAllByRole('button', { name: 'Remove this category' })[0])
  expect(changes.at(-1)).toEqual([{ categoryId: 'cat-mortgage', payeeId: null, tagId: null, labelId: null }])
  expect(screen.getAllByRole('button', { name: /^Category:/ })).toHaveLength(1)
  expect(screen.queryByRole('button', { name: 'Remove this category' })).toBeNull()
})

it('the category picker lists the caller\'s categories first, then the partner\'s under their name', async () => {
  const user = userEvent.setup()
  renderEditor()

  await user.click(screen.getByRole('button', { name: 'Category: Food' }))
  const sheet = await screen.findByRole('dialog', { name: 'Category' })
  const ada = within(sheet).getByText('Ada')
  const anna = within(sheet).getByText('Anna')
  expect(ada.compareDocumentPosition(anna) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(within(sheet).getByText('Mortgage').compareDocumentPosition(anna) & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy()
})

it('the payee picker lists the chosen category owner\'s payees', async () => {
  const user = userEvent.setup()
  const changes = renderEditor({ initial: [{ ...food, payeeId: 'p1' }] })

  await user.click(screen.getByRole('button', { name: 'Category: Food' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Category' })).getByText('Mortgage'))
  // the caller's payee does not narrow the partner's category
  expect(changes.at(-1)).toEqual([{ categoryId: 'cat-mortgage', payeeId: null, tagId: null, labelId: null }])

  await user.click(screen.getByRole('button', { name: 'Payee (optional): any payee' }))
  const sheet = await screen.findByRole('dialog', { name: 'Payee (optional)' })
  expect(within(sheet).getByText('Bank')).toBeInTheDocument()
  expect(within(sheet).queryByText('Grocer')).toBeNull()
  await user.click(within(sheet).getByText('Bank'))
  expect(changes.at(-1)).toEqual([{ categoryId: 'cat-mortgage', payeeId: 'p-bank', tagId: null, labelId: null }])
})

it('offers no more categories at ten rules', () => {
  renderEditor({ initial: Array.from({ length: 10 }, (_, i) => ({ ...food, labelId: `l${i}` })) })
  expect(screen.getAllByRole('button', { name: /^Category:/ })).toHaveLength(10)
  expect(screen.queryByRole('button', { name: 'Add a category' })).toBeNull()
})

it('a rule naming classifications missing from the options shows them by name and keeps them', async () => {
  const user = userEvent.setup()
  // an archived category of a former participant, and their payee
  const gone: DebtRuleDto = {
    ...wireRule, id: 'r-gone', categoryId: 'cat-old', categoryName: 'Old loan', categoryIcon: 'history',
    ownerUserId: 'u9', ownerName: 'Max', payeeId: 'p-gone', payeeName: 'Old bank',
  }
  const kept: RuleDraft = { id: 'r-gone', categoryId: 'cat-old', payeeId: 'p-gone', tagId: null, labelId: null }
  const changes = renderEditor({ initial: [kept], initialRules: [gone] })

  expect(screen.getByRole('button', { name: 'Category: Old loan' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Payee (optional): Old bank' })).toBeInTheDocument()

  // another rule added leaves this one as it was
  await user.click(screen.getByRole('button', { name: 'Add a category' }))
  expect(changes.at(-1)?.[0]).toEqual(kept)
})

it('the locked category cannot be changed or removed', async () => {
  const user = userEvent.setup()
  renderEditor({ lockedCategoryId: 'cat-food' })
  expect(screen.getByRole('button', { name: 'Category: Food' })).toBeDisabled()

  await user.click(screen.getByRole('button', { name: 'Add a category' }))
  // only the added line can go
  expect(screen.getAllByRole('button', { name: 'Remove this category' })).toHaveLength(1)
})

it('shows its error under the rules', () => {
  renderEditor({ error: 'Another debt already counts these payments.' })
  expect(screen.getByText('Another debt already counts these payments.')).toBeInTheDocument()
})

it('read-only, it names each rule and the owner of a partner\'s category', () => {
  const anna: DebtRuleDto = { ...wireRule, id: 'r2', categoryId: 'cat-mortgage', categoryName: 'Mortgage', categoryIcon: 'home', ownerUserId: 'u2', ownerName: 'Anna', labelId: 'l-house', labelName: 'house' }
  renderEditor({ readOnly: true, initialRules: [wireRule, anna] })
  const group = screen.getByRole('group', { name: 'Which payments count' })
  // icons are ligature text
  expect(group).toHaveTextContent(/Payments in \w*Food · from any payee · tagged any tag/)
  expect(group).toHaveTextContent(/Payments in \w*Anna's Mortgage · from any payee · tagged \w*house/)
  expect(within(group).queryAllByRole('button')).toHaveLength(0)
})
