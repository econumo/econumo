import type { ReactNode } from 'react'
import { act, render, renderHook, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { coreHandlers, fixtureDebtRow, fixtureWireBudget } from '@/test/fixtures'
import type { BudgetDto, BudgetElementDto, BudgetPlanDto, PlanElementDto } from '@/api/dto/budget'
import { useUiStore } from '@/app/uiStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { wireDebt } from '@/features/debts/testDebt'
import { useBudgetLineMenus } from './useBudgetLineMenus'
import type { BudgetLineMenus } from './useBudgetLineMenus'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

function renderMenus(userId = 'u1', adjust?: (b: BudgetDto) => void) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  adjust?.(budget)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  const { result } = renderHook(() => useBudgetLineMenus({ budget, plan: null, userId, onOpenSettings: vi.fn() }), { wrapper })
  const element = (id: string) => budget.structure.elements.find((el) => el.id === id) as BudgetElementDto
  return { result, budget, element, queryClient }
}

const labels = (actions: { label: string }[]) => actions.map((a) => a.label)

beforeEach(() => {
  server.use(...coreHandlers())
  useUiStore.setState({ debtModal: null, debtDetailsId: null })
})

describe('Track as debt', () => {
  it('Track as debt opens the form prefilled', () => {
    const { result, element } = renderMenus()
    const track = result.current.expenseRowMenu(element('cat-food')).find((a) => a.label === 'Track as debt')
    expect(track).toBeDefined()
    act(() => track!.onSelect())
    // Food holds no currency of its own: the budget's
    expect(useUiStore.getState().debtModal).toEqual({ categoryId: 'cat-food', currencyId: 'cur-usd', fromBudget: true, budgetId: 'b1' })
  })

  it('a category in an envelope takes the envelope\'s currency', () => {
    const { result, element } = renderMenus()
    const envelope = element('env-1')
    const track = result.current.envelopeChildMenu(envelope.children[0], envelope).find((a) => a.label === 'Track as debt')
    act(() => track!.onSelect())
    expect(useUiStore.getState().debtModal).toEqual({ categoryId: 'cat-rent', currencyId: 'cur-eur', fromBudget: true, budgetId: 'b1' })
  })

  it('is offered only on the caller\'s own live expense category', () => {
    const { result, element } = renderMenus('u1', (b) => {
      b.structure.elements.push(
        { ...b.structure.elements[0], id: 'cat-other', name: 'Other', ownerUserId: 'u9' },
        { ...b.structure.elements[0], id: 'cat-old', name: 'Old', isArchived: 1 },
      )
    })
    expect(labels(result.current.expenseRowMenu(element('cat-other')))).not.toContain('Track as debt')
    expect(labels(result.current.expenseRowMenu(element('cat-old')))).not.toContain('Track as debt')
    // an envelope and a tag are not categories
    expect(labels(result.current.expenseRowMenu(element('env-1')))).not.toContain('Track as debt')
    expect(labels(result.current.expenseRowMenu(element('tag-old')))).not.toContain('Track as debt')
  })
})

it('the Plan grid\'s expense category row offers it too', () => {
  const { result } = renderMenus()
  const food = { id: 'cat-food', type: 1, name: 'Food', icon: 'restaurant', currencyId: 'cur-usd', isArchived: 0, folderId: 'bf1', position: 0, ownerUserId: 'u1', cells: [], children: [] }
  expect(labels(result.current.planRowMenu(food as never, 0))).toContain('Track as debt')
})

it('an archived budget offers no Track as debt', () => {
  const { result, element } = renderMenus('u1', (b) => {
    b.meta.isArchived = 1
  })
  expect(labels(result.current.expenseRowMenu(element('cat-food')))).not.toContain('Track as debt')
  const envelope = element('env-1')
  expect(labels(result.current.envelopeChildMenu(envelope.children[0], envelope))).not.toContain('Track as debt')
})

describe('the debt row menu', () => {
  it('the owner of a set-up debt gets Details, Mark as paid off and Delete; Details opens the review screen', () => {
    const { result } = renderMenus()
    const actions = result.current.debtRowMenu(fixtureDebtRow())
    expect(labels(actions)).toEqual(['Details', 'Mark as paid off', 'Delete'])
    expect(actions[2].destructive).toBe(true)
    act(() => actions[0].onSelect())
    expect(useUiStore.getState().debtDetailsId).toBe('debt-1')
    expect(useUiStore.getState().debtDetailsBudgetId).toBe('b1')
    expect(useUiStore.getState().debtModal).toBeNull()
  })

  it('no Mark as paid off while the debt is not set up, or once it is paid off', () => {
    const { result } = renderMenus()
    expect(labels(result.current.debtRowMenu(fixtureDebtRow({ isSetUp: 0 })))).toEqual(['Details', 'Delete'])
    expect(labels(result.current.debtRowMenu(fixtureDebtRow({ isPaidOff: 1 })))).toEqual(['Details', 'Delete'])
  })

  it('the Plan grid\'s debt row reads the debt\'s state from the plan', () => {
    const debt = { id: 'debt-1', isSetUp: 1, isPaidOff: 0 }
    const plan = (over: object) => ({ structure: { folders: [], elements: [], debts: [{ ...debt, ...over }] } }) as unknown as BudgetPlanDto
    const row = { id: 'debt-1', type: 6, name: 'Mortgage', icon: 'home', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 0, ownerUserId: 'u1', cells: [], children: [] } as PlanElementDto
    const menusWith = (p: BudgetPlanDto) => renderHook(() => useBudgetLineMenus({ budget: coerceBudgetFixture(fixtureWireBudget), plan: p, userId: 'u1', onOpenSettings: vi.fn() }), { wrapper }).result.current
    expect(labels(menusWith(plan({})).planRowMenu(row, 0))).toEqual(['Details', 'Mark as paid off', 'Delete'])
    expect(labels(menusWith(plan({ isPaidOff: 1 })).planRowMenu(row, 0))).toEqual(['Details', 'Delete'])
    expect(labels(menusWith(plan({ isSetUp: 0 })).planRowMenu(row, 0))).toEqual(['Details', 'Delete'])
  })

  it('offline every entry stays listed, greyed out', () => {
    act(() => onlineManager.setOnline(false))
    onTestFinished(() => act(() => onlineManager.setOnline(true)))
    const { result } = renderMenus()
    const actions = result.current.debtRowMenu(fixtureDebtRow())
    expect(labels(actions)).toEqual(['Details', 'Mark as paid off', 'Delete'])
    expect(actions.every((a) => a.disabled)).toBe(true)
  })

  it('a guest sees no Details in the debt row menu', () => {
    const { result } = renderMenus('u2')
    expect(result.current.debtRowMenu(fixtureDebtRow())).toEqual([])
  })

  it('the sheet\'s access check (which gates Details) passes the owner, and nobody else', () => {
    expect(renderMenus('u1').result.current.editAccess({ kind: 'debt', row: fixtureDebtRow() })).toBe(true)
    expect(renderMenus('u2').result.current.editAccess({ kind: 'debt', row: fixtureDebtRow() })).toBeNull()
  })
})

describe('the debt row menu\'s actions', () => {
  // the menus with their dialogs on screen, as the Budget view and the Plan grid render them
  function renderWithDialogs() {
    const budget = coerceBudgetFixture(fixtureWireBudget)
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const menus: { current: BudgetLineMenus | null } = { current: null }
    function Harness() {
      const m = useBudgetLineMenus({ budget, plan: null, userId: 'u1', onOpenSettings: vi.fn() })
      menus.current = m
      return <>{m.dialogs}</>
    }
    render(
      <QueryClientProvider client={queryClient}>
        <Harness />
      </QueryClientProvider>,
    )
    const select = (label: string) => act(() => menus.current!.debtRowMenu(fixtureDebtRow()).find((a) => a.label === label)!.onSelect())
    return { select }
  }

  beforeEach(() => {
    vi.mocked(trackEvent).mockClear()
    window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  })

  it('Mark as paid off asks first, then posts', async () => {
    const ids: unknown[] = []
    server.use(
      http.post('*/api/v1/debt/mark-debt-paid-off', async ({ request }) => {
        ids.push(((await request.json()) as { id: string }).id)
        return ok({ ...wireDebt, id: 'debt-1', isPaidOff: 1 })
      }),
    )
    const user = userEvent.setup()
    const { select } = renderWithDialogs()
    select('Mark as paid off')
    const dialog = await screen.findByRole('dialog', { name: 'Mark as paid off?' })
    expect(ids).toEqual([])
    await user.click(within(dialog).getByRole('button', { name: 'Mark as paid off' }))
    await waitFor(() => expect(ids).toEqual(['debt-1']))
    await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_MARK_PAID_OFF))
  })

  it('Delete asks, and a removal_unconfirmed answer asks again and resends with confirmDataRemoval', async () => {
    const bodies: Record<string, unknown>[] = []
    const refusal = 'This deletes the planned amounts and comments of this debt in your budgets; confirm to continue.'
    server.use(
      http.post('*/api/v1/debt/delete-debt', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        bodies.push(body)
        if (!body.confirmDataRemoval) {
          return HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { confirmDataRemoval: [refusal] } }, { status: 400 })
        }
        return ok({})
      }),
    )
    const user = userEvent.setup()
    const { select } = renderWithDialogs()
    select('Delete')
    const first = await screen.findByRole('dialog', { name: 'Delete this debt?' })
    expect(bodies).toEqual([])
    await user.click(within(first).getByRole('button', { name: 'Delete' }))

    const second = await screen.findByRole('dialog', { name: 'Delete plans and comments?' })
    expect(within(second).getByText(refusal)).toBeInTheDocument()
    expect(bodies).toEqual([{ id: 'debt-1' }])
    await user.click(within(second).getByRole('button', { name: 'Delete debt' }))
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[1]).toEqual({ id: 'debt-1', confirmDataRemoval: true })
    await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_DELETE))
  })

  it('a refusal shows up as a toast', async () => {
    const spy = vi.spyOn(toast, 'error')
    onTestFinished(() => spy.mockRestore())
    server.use(
      http.post('*/api/v1/debt/delete-debt', () =>
        HttpResponse.json({ success: false, message: 'Debt not found', code: 400, errors: {} }, { status: 400 }),
      ),
    )
    const user = userEvent.setup()
    const { select } = renderWithDialogs()
    select('Delete')
    await user.click(within(await screen.findByRole('dialog', { name: 'Delete this debt?' })).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('Debt not found', expect.anything()))
    expect(screen.queryByRole('dialog', { name: 'Delete plans and comments?' })).toBeNull()
  })
})
