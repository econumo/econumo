import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import i18n from '@/app/i18n'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories } from '@/test/fixtures'
import { CategoriesStep, CategoriesStepBody, type ExistingCategory } from './CategoriesStep'
import { METRICS, trackEvent } from '@/lib/metrics'
import { emptyDraft, loadDraft, saveDraft, type CategoryRow } from './draft'
import { stepRoutes, userWith } from './onboardingRoutes.test-utils'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

function renderStep(props: Partial<React.ComponentProps<typeof CategoriesStep>> = {}) {
  const onContinue = vi.fn()
  const onSkip = vi.fn()
  render(<CategoriesStep existing={[]} onContinue={onContinue} onSkip={onSkip} pending={false} {...props} />)
  return { onContinue, onSkip }
}

const submitted = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[0][0] as CategoryRow[]
const removeButtons = () => screen.queryAllByRole('button', { name: /^Remove / })

const food: ExistingCategory = { id: 'cat-1', name: 'Groceries', type: 'expense', icon: 'cake' }
const pay: ExistingCategory = { id: 'cat-2', name: 'Paycheck', type: 'income', icon: 'work' }

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('lists all 14 defaults with a remove button after the name, in one card per type', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  expect(removeButtons()).toHaveLength(14)
  const first = screen.getAllByTestId('category-row')[0]
  const name = within(first).getByLabelText('Category name')
  expect(name.compareDocumentPosition(within(first).getByRole('button', { name: 'Remove Groceries' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(within(screen.getByTestId('group-expense')).getAllByTestId('category-row')).toHaveLength(11)
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const rows = submitted(onContinue)
  expect(rows).toHaveLength(14)
  expect(rows.filter((r) => r.type === 'expense').map((r) => r.name).slice(8, 10)).toEqual(['Kids', 'Pets'])
  expect(rows.filter((r) => r.type === 'income').map((r) => r.name)).toEqual(['Salary', 'Bonus', 'Other income'])
})

it('a removed category leaves the list and is not submitted; the rest go out in display order', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.click(screen.getByRole('button', { name: 'Remove Kids' }))
  expect(screen.queryByDisplayValue('Kids')).not.toBeInTheDocument()
  expect(screen.getAllByTestId('category-row')).toHaveLength(13)
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const names = submitted(onContinue).map((r) => r.name)
  expect(names).toHaveLength(13)
  expect(names).not.toContain('Kids')
  expect(names.slice(7, 9)).toEqual(['Entertainment', 'Pets'])
})

it('a removed category typed again under Add your own comes back as a normal row', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.click(screen.getByRole('button', { name: 'Remove Kids' }))
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'Kids{Enter}')
  expect(screen.getByRole('button', { name: 'Remove Kids' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).filter((r) => r.name === 'Kids')).toHaveLength(1)
})

it('passes a rename and an icon change through', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  const first = screen.getAllByTestId('category-row')[0]
  const input = within(first).getByLabelText('Category name')
  await user.clear(input)
  await user.type(input, '  Food ')
  await user.click(within(first).getByRole('button', { name: 'Change icon' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('option', { name: 'account_balance_wallet' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue)[0]).toMatchObject({ name: 'Food', icon: 'account_balance_wallet', type: 'expense' })
})

it('adds your own expense and income rows on Enter and blur', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'Pets{Enter}')
  await user.type(within(screen.getByTestId('group-income')).getByPlaceholderText('Add your own'), 'Gifts')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const rows = submitted(onContinue)
  expect(rows.find((r) => r.name === 'Pets')).toMatchObject({ type: 'expense' })
  expect(rows.find((r) => r.name === 'Gifts')).toMatchObject({ type: 'income' })
})

it('merges duplicate names within a type, case-insensitively', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'groceries{Enter}')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).filter((r) => r.name.toLowerCase() === 'groceries')).toHaveLength(1)
})

it('disables Continue only once every category is removed on a fresh account', async () => {
  const user = userEvent.setup()
  renderStep()
  const buttons = removeButtons()
  for (const b of buttons.slice(0, -1)) await user.click(b)
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  await user.click(buttons.at(-1)!)
  expect(screen.queryAllByTestId('category-row')).toHaveLength(0)
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('keeps Continue enabled with every new row removed while the user has categories', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [food], initialRows: [{ key: 'k1', name: 'Fun', type: 'expense', icon: 'movie' }] })
  await user.click(screen.getByRole('button', { name: 'Remove Fun' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue)).toEqual([])
})

it('lists existing categories read-only, with no remove button, and offers no defaults on top of them', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [food, pay] })
  const expense = within(screen.getByTestId('group-expense')).getAllByTestId('category-row')
  expect(expense).toHaveLength(1)
  expect(expense[0]).toHaveTextContent('Groceries')
  expect(within(expense[0]).queryByLabelText('Category name')).not.toBeInTheDocument()
  expect(within(expense[0]).queryByRole('checkbox')).not.toBeInTheDocument()
  expect(within(expense[0]).queryByRole('button', { name: /^Remove / })).not.toBeInTheDocument()
  expect(within(expense[0]).queryByRole('button', { name: 'Change icon' })).not.toBeInTheDocument()
  expect(within(screen.getByTestId('group-income')).getAllByTestId('category-row')[0]).toHaveTextContent('Paycheck')
  expect(screen.queryByDisplayValue('Housing')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue)).toEqual([])
})

it('does not offer a default named like an archived category, nor list the archived one', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ archived: [{ id: 'cat-9', name: 'groceries', type: 'expense', icon: 'cake' }] })
  expect(screen.queryByDisplayValue('Groceries')).not.toBeInTheDocument()
  expect(screen.getByDisplayValue('Housing')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).map((r) => r.name)).not.toContain('Groceries')
})

it('never submits a new row named like an existing category', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [food] })
  await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'GROCERIES{Enter}')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue)).toEqual([])
})

it('reseeds from a stored draft keeping keys, showing created ones as existing only', async () => {
  const user = userEvent.setup()
  const rows = [
    { key: 'k1', name: 'Groceries', type: 'expense', icon: 'cake' },
    { key: 'k2', name: 'Fun', type: 'expense', icon: 'movie' },
    { key: 'k4', name: 'Books', type: 'expense', icon: 'book' },
  ] as CategoryRow[]
  const { onContinue } = renderStep({ initialRows: rows, existing: [food] })
  expect(screen.getAllByTestId('category-row')).toHaveLength(3)
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).map((r) => r.key)).toEqual(['k2', 'k4'])
})

it('treats an unticked row of an older draft as removed', async () => {
  const user = userEvent.setup()
  const rows = [
    { key: 'k2', name: 'Fun', type: 'expense', icon: 'movie', checked: true },
    { key: 'k3', name: 'Kids', type: 'expense', icon: 'child_care', checked: false },
  ] as unknown as CategoryRow[]
  const { onContinue } = renderStep({ initialRows: rows })
  expect(screen.queryByDisplayValue('Kids')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).map((r) => r.key)).toEqual(['k2'])
})

it('Skip calls onSkip', async () => {
  const user = userEvent.setup()
  const { onSkip } = renderStep()
  await user.click(screen.getByRole('button', { name: 'Skip' }))
  expect(onSkip).toHaveBeenCalled()
})

it('is inert while pending', () => {
  renderStep({ pending: true })
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Skip' })).toBeDisabled()
})

it('flags a renamed or added category outside 3-64 characters and blocks Continue until it is fixed or removed', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  const first = screen.getAllByTestId('category-row')[0]
  const input = within(first).getByLabelText('Category name')
  await user.clear(input)
  await user.type(input, 'Fo')
  expect(screen.queryByText('Category name must be 3-64 characters')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.tab()
  expect(within(first).getByText('Category name must be 3-64 characters')).toBeInTheDocument()
  expect(input).toHaveAttribute('aria-invalid', 'true')

  await user.type(input, 'od')
  expect(screen.queryByText('Category name must be 3-64 characters')).not.toBeInTheDocument()

  await user.type(within(screen.getByTestId('group-income')).getByPlaceholderText('Add your own'), 'Tx{Enter}')
  const added = screen.getAllByTestId('category-row').at(-1)!
  expect(within(added).getByText('Category name must be 3-64 characters')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.click(within(added).getByRole('button', { name: 'Remove Tx' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(submitted(onContinue).map((r) => r.name)).not.toContain('Tx')
})

it('gives every row a keyboard-reachable drag handle on a desktop width, none on a phone', () => {
  renderStep({ existing: [food], initialRows: [{ key: 'k1', name: 'Pets', type: 'expense', icon: 'pets' }] })
  expect(screen.getByRole('button', { name: 'Reorder Groceries' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Reorder Pets' })).toHaveAttribute('tabindex', '0')
})

it('shows no drag handles on a phone or tablet width', () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  renderStep({ existing: [food] })
  expect(screen.queryByRole('button', { name: /^Reorder / })).not.toBeInTheDocument()
})

describe('in the UI language', () => {
  afterEach(async () => {
    await act(() => i18n.changeLanguage('en'))
  })

  it('re-labels untouched defaults on a language switch, keeps a renamed row, and submits the current labels', async () => {
    const user = userEvent.setup()
    const { onContinue } = renderStep()
    const names = () => screen.getAllByLabelText(/Category name|Kategoriename/).map((el) => (el as HTMLInputElement).value)
    await user.clear(screen.getAllByLabelText('Category name')[1])
    await user.type(screen.getAllByLabelText('Category name')[1], 'Rent')
    await act(() => i18n.changeLanguage('de'))
    expect(names().slice(0, 3)).toEqual(['Lebensmittel', 'Rent', 'Nebenkosten'])
    expect(names()).toContain('Haustiere')
    await user.click(screen.getByRole('button', { name: 'Weiter' }))
    const rows = submitted(onContinue)
    expect(rows[0]).toMatchObject({ name: 'Lebensmittel', labelKey: 'onboarding.categories.groceries' })
    expect(rows[1].name).toBe('Rent')
    expect(rows[1].labelKey).toBeUndefined()
  })

  it('a stored draft with label keys reloads in the current language', async () => {
    await act(() => i18n.changeLanguage('de'))
    renderStep({
      initialRows: [
        { key: 'k1', name: 'Pets', labelKey: 'onboarding.categories.pets', type: 'expense', icon: 'pets' },
        { key: 'k2', name: 'Rent', type: 'expense', icon: 'home' },
      ],
    })
    expect(screen.getAllByLabelText('Kategoriename').map((el) => (el as HTMLInputElement).value)).toEqual(['Haustiere', 'Rent'])
  })
})

describe('CategoriesStepBody', () => {
  const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })
  let routes: ReturnType<typeof stepRoutes>

  function renderBody(categories: unknown[], first: Parameters<typeof server.use> = []) {
    routes = stepRoutes()
    server.use(...first, ...coreHandlers({ categories, user: userWith('') }), ...routes.handlers)
    const onDone = vi.fn(() => Promise.resolve())
    const onSkip = vi.fn(() => Promise.resolve())
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <CategoriesStepBody onDone={onDone} onSkip={onSkip} />
      </QueryClientProvider>,
    )
    return { onDone, onSkip }
  }

  beforeEach(() => {
    localStorage.clear()
    vi.mocked(trackEvent).mockClear()
  })

  it('reports removed, renamed and custom categories by catalogue key once the save lands', async () => {
    const user = userEvent.setup()
    const { onDone } = renderBody([])
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    await user.click(screen.getByRole('button', { name: 'Remove Kids' }))
    await user.click(screen.getByRole('button', { name: 'Remove Pets' }))
    const groceries = screen.getByDisplayValue('Groceries')
    await user.clear(groceries)
    await user.type(groceries, 'Food')
    await user.type(within(screen.getByTestId('group-expense')).getByPlaceholderText('Add your own'), 'Gym{Enter}')
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    const calls = vi.mocked(trackEvent).mock.calls.filter(([name]) => name === METRICS.ONBOARDING_CATEGORIES_COMPLETE)
    expect(calls).toEqual([[METRICS.ONBOARDING_CATEGORIES_COMPLETE, { renamed: 'groceries', custom: 1, offered: 14 }]])
    const removed = vi.mocked(trackEvent).mock.calls.filter(([name]) => name === METRICS.ONBOARDING_CATEGORY_REMOVED)
    expect(removed).toEqual([
      [METRICS.ONBOARDING_CATEGORY_REMOVED, { category: 'kids' }],
      [METRICS.ONBOARDING_CATEGORY_REMOVED, { category: 'pets' }],
    ])
  })

  it('sends no removal events when nothing was changed', async () => {
    const user = userEvent.setup()
    const { onDone } = renderBody([])
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_CATEGORIES_COMPLETE, { renamed: '', custom: 0, offered: 14 })
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.ONBOARDING_CATEGORY_REMOVED, expect.anything())
  })

  it('Continue posts one save-categories with every row in display order, then moves on', async () => {
    const user = userEvent.setup()
    const { onDone } = renderBody([])
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls).toHaveLength(1)
    const body = routes.srv.calls[0].body
    expect(body.order).toBeUndefined()
    expect(body.categories).toHaveLength(14)
    expect(body.categories[0]).toEqual({ key: expect.any(String), name: 'Groceries', type: 'expense', icon: expect.any(String) })
    expect(loadDraft('u1')!.ops.categories).toBeUndefined()
  })

  it('a user who already has categories continues with no new rows', async () => {
    const user = userEvent.setup()
    const { onDone } = renderBody(fixtureCategories)
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls.map((c) => c.body.categories)).toEqual([[]])
  })

  it('a stored draft from a failed submit reseeds the form and replays its operation id', async () => {
    const rows: CategoryRow[] = [
      { key: 'k1', name: 'Gifts', type: 'expense', icon: 'redeem' },
      { key: 'k2', name: 'Fun', type: 'expense', icon: 'movie' },
    ]
    saveDraft('u1', { ...emptyDraft(), ops: { categories: 'op-cats' }, categoryRows: rows })
    const user = userEvent.setup()
    const { onDone } = renderBody(fixtureCategories)
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    expect(screen.getByDisplayValue('Gifts')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Fun')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls.map((c) => [c.body.id, c.body.categories.map((r: CategoryRow) => r.key)])).toEqual([['op-cats', ['k1', 'k2']]])
    expect(loadDraft('u1')!.ops.categories).toBeUndefined()
  })

  it('an archived category of the user keeps its default name out of the list', async () => {
    // get-category-list as the server sends it: archived categories are included, isArchived 1
    renderBody([{ id: 'cat-a', ownerUserId: 'u1', name: 'Groceries', position: 0, type: 'expense', icon: 'shopping_cart', isArchived: 1, createdAt: '2026-11-08 21:37:17', updatedAt: '2026-11-08 21:37:17' }])
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    expect(screen.queryByDisplayValue('Groceries')).not.toBeInTheDocument()
    expect(screen.getByDisplayValue('Housing')).toBeInTheDocument()
  })

  it('a failed save shows the error and stays on the step', async () => {
    const user = userEvent.setup()
    const { onDone } = renderBody([])
    server.use(http.post('*/api/v1/onboarding/save-categories', () => HttpResponse.json({ success: false, message: 'Nope', code: 400, errors: {} }, { status: 400 })))
    await screen.findByRole('heading', { name: 'What do you spend on?' })
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Nope')
    expect(onDone).not.toHaveBeenCalled()
  })

  it('a failed category list shows the error with a retry instead of the defaults', async () => {
    let fail = true
    renderBody([], [
      http.get('*/api/v1/category/get-category-list', () =>
        fail ? HttpResponse.json({ success: false, message: 'Server down' }, { status: 500 }) : ok({ items: fixtureCategories }),
      ),
    ])
    const user = userEvent.setup()
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Remove / })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Continue' })).not.toBeInTheDocument()
    fail = false
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Food')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Remove Food' })).not.toBeInTheDocument()
  })
})
