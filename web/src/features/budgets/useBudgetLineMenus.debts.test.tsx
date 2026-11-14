import type { ReactNode } from 'react'
import { act, render, renderHook, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { coreHandlers, fixtureElementDebt, fixtureWireBudget } from '@/test/fixtures'
import type { BudgetDto, BudgetElementDto, PlanElementDto } from '@/api/dto/budget'
import { useUiStore } from '@/app/uiStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { wireDebt } from '@/features/debts/testDebt'
import type { DebtDto } from '@/api/dto/debt'
import { queryKeys } from '@/app/queryKeys'
import { useBudgetLineMenus } from './useBudgetLineMenus'
import type { BudgetLineMenus } from './useBudgetLineMenus'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

// Food (cat-food) carries the active car loan d1; it paid off two older loans
const pastDebt = (id: string, startDate: string, endedAt: string, amountBorrowed: string): DebtDto => ({
  ...wireDebt,
  id,
  status: 'paid_off',
  endedAt,
  startDate,
  amountBorrowed,
  isPaidOff: 1,
})
const history = [pastDebt('d-old', '2020-03-01', '2022-05-31', '9000'), pastDebt('d-older', '2018-01-10', '2019-12-31', '4000')]
// another budget's debt on the same category id must not count
const elsewhere: DebtDto = { ...pastDebt('d-x', '2019-01-01', '2019-06-30', '100'), budgetId: 'b2' }

// debts: the cached debt list; null leaves it unloaded
function renderMenus(userId = 'u1', adjust?: (b: BudgetDto) => void, debts: DebtDto[] | null = [wireDebt, ...history, elsewhere]) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  budget.structure.elements[0].debt = fixtureElementDebt({ id: 'd1' })
  adjust?.(budget)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  if (debts) {
    queryClient.setQueryData(queryKeys.debts, debts)
  }
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  const { result } = renderHook(() => useBudgetLineMenus({ budget, plan: null, userId, onOpenSettings: vi.fn() }), { wrapper })
  const element = (id: string) => budget.structure.elements.find((el) => el.id === id) as BudgetElementDto
  return { result, budget, element, queryClient }
}

const labels = (actions: { label: string }[]) => actions.map((a) => a.label)
const DEBT_LABELS = ['Track as debt', 'Debt details', 'Mark as paid off', 'Delete debt', 'Past debts (2)']
const debtLabels = (actions: { label: string }[]) => labels(actions).filter((l) => DEBT_LABELS.includes(l) || l.startsWith('Past debts'))

// the caller joins the fixture budget (owned by u1) with a role
const joinAs = (userId: string, role: 'admin' | 'user' | 'guest') => (b: BudgetDto) => {
  b.meta.access.push({ user: { id: userId, name: 'Anna', avatar: 'face:fuchsia' }, role, isAccepted: 1 })
}

beforeEach(() => {
  server.use(...coreHandlers())
  useUiStore.setState({ debtModal: null, debtDetailsId: null, debtDetailsTab: 'overview' })
})

describe('an element with no active debt', () => {
  it('a top-level expense category offers Track as debt, opening the form on the element', () => {
    const { result, element } = renderMenus('u1', undefined, [])
    const food = { ...element('cat-food'), debt: undefined }
    const actions = result.current.expenseRowMenu(food)
    expect(debtLabels(actions)).toEqual(['Track as debt'])
    act(() => actions.find((a) => a.label === 'Track as debt')!.onSelect())
    expect(useUiStore.getState().debtModal).toEqual({ elementId: 'cat-food', budgetId: 'b1' })
  })

  it('an expense envelope offers it too', () => {
    const { result, element } = renderMenus()
    expect(debtLabels(result.current.expenseRowMenu(element('env-1')))).toEqual(['Track as debt'])
  })

  it('a debt paid off earlier leaves the element free: Track as debt and its history', () => {
    const { result, element } = renderMenus('u1', undefined, history)
    // the period shows the paid-off debt that was in force then
    const food = { ...element('cat-food'), debt: fixtureElementDebt({ id: 'd-old', isPaidOff: 1 }) }
    expect(debtLabels(result.current.expenseRowMenu(food))).toEqual(['Track as debt', 'Past debts (2)'])
  })

  it('a tag, an archived category, a category inside an envelope, income and savings offer nothing', () => {
    const { result, element } = renderMenus('u1', (b) => {
      b.structure.elements.push({ ...b.structure.elements[0], id: 'cat-old', name: 'Old', isArchived: 1, debt: undefined })
    })
    expect(debtLabels(result.current.expenseRowMenu(element('tag-old')))).toEqual([])
    expect(debtLabels(result.current.expenseRowMenu(element('cat-old')))).toEqual([])
    expect(debtLabels(result.current.envelopeChildMenu(element('env-1').children[0]))).toEqual([])
    const salary = { id: 'cat-salary', type: 3, name: 'Salary', icon: 'payments', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 0, ownerUserId: 'u1', cells: [], children: [] } as PlanElementDto
    expect(debtLabels(result.current.planRowMenu(salary, 0))).toEqual([])
    const incomeEnvelope = { ...salary, id: 'ie1', type: 4, ownerUserId: null } as PlanElementDto
    expect(debtLabels(result.current.planRowMenu(incomeEnvelope, 0))).toEqual([])
    const savings = { ...salary, id: 'acc-s1', type: 5 } as PlanElementDto
    expect(debtLabels(result.current.planRowMenu(savings, 0))).toEqual([])
  })

  it('a guest gets none, nor does an archived budget', () => {
    expect(debtLabels(renderMenus('u2', joinAs('u2', 'guest'), []).result.current.expenseRowMenu(coerceBudgetFixture(fixtureWireBudget).structure.elements[1]))).toEqual([])
    const archived = renderMenus('u1', (b) => {
      b.meta.isArchived = 1
    }, [])
    expect(debtLabels(archived.result.current.expenseRowMenu(archived.element('env-1')))).toEqual([])
  })
})

describe('an element carrying its active debt', () => {
  it('offers Debt details, Mark as paid off, Delete debt and the past debts', () => {
    const { result, element } = renderMenus()
    const actions = result.current.expenseRowMenu(element('cat-food'))
    expect(debtLabels(actions)).toEqual(['Debt details', 'Mark as paid off', 'Delete debt', 'Past debts (2)'])
    expect(actions.find((a) => a.label === 'Delete debt')!.destructive).toBe(true)
    act(() => actions.find((a) => a.label === 'Debt details')!.onSelect())
    expect(useUiStore.getState().debtDetailsId).toBe('d1')
    expect(useUiStore.getState().debtDetailsTab).toBe('overview')
  })

  it('no Mark as paid off while the debt is not set up', () => {
    const { result, element } = renderMenus('u1', undefined, [{ ...wireDebt, isSetUp: 0 }])
    expect(debtLabels(result.current.expenseRowMenu(element('cat-food')))).toEqual(['Debt details', 'Delete debt'])
  })

  it('before the debt list loads, the debt in force stands in', () => {
    const { result, element } = renderMenus('u1', undefined, null)
    expect(debtLabels(result.current.expenseRowMenu(element('cat-food')))).toEqual(['Debt details', 'Mark as paid off', 'Delete debt'])
  })

  it('the Plan grid\'s row reads its active debt', () => {
    const { result } = renderMenus('u1', undefined, null)
    const food = { id: 'cat-food', type: 1, name: 'Food', icon: 'restaurant', currencyId: 'cur-usd', isArchived: 0, folderId: 'bf1', position: 0, ownerUserId: 'u1', cells: [], children: [], debt: fixtureElementDebt({ id: 'd1' }) } as PlanElementDto
    expect(debtLabels(result.current.planRowMenu(food, 0))).toEqual(['Debt details', 'Mark as paid off', 'Delete debt'])
  })

  it('a guest gets Debt details only, which opens the read-only review', () => {
    const { result, element } = renderMenus('u2', joinAs('u2', 'guest'))
    const actions = result.current.expenseRowMenu(element('cat-food'))
    expect(debtLabels(actions)).toEqual(['Debt details'])
    act(() => actions.find((a) => a.label === 'Debt details')!.onSelect())
    expect(useUiStore.getState().debtDetailsId).toBe('d1')
  })

  it('a participant with the user role gets every action', () => {
    const { result, element } = renderMenus('u2', joinAs('u2', 'user'))
    expect(debtLabels(result.current.expenseRowMenu(element('cat-food')))).toEqual(['Debt details', 'Mark as paid off', 'Delete debt', 'Past debts (2)'])
  })

  it('offline the debt entries stay listed, greyed out', () => {
    act(() => onlineManager.setOnline(false))
    onTestFinished(() => act(() => onlineManager.setOnline(true)))
    const { result, element } = renderMenus()
    const actions = result.current.expenseRowMenu(element('cat-food')).filter((a) => debtLabels([a]).length > 0)
    expect(labels(actions)).toEqual(['Debt details', 'Mark as paid off', 'Delete debt', 'Past debts (2)'])
    expect(actions.every((a) => a.disabled)).toBe(true)
  })
})

describe('the menus\' dialogs', () => {
  // the menus with their dialogs on screen, as the Budget view and the Plan grid render them
  function renderWithDialogs(debts: DebtDto[] = [wireDebt, ...history]) {
    const budget = coerceBudgetFixture(fixtureWireBudget)
    budget.structure.elements[0].debt = fixtureElementDebt({ id: 'd1' })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    queryClient.setQueryData(queryKeys.debts, debts)
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
    const select = (label: string) =>
      act(() => menus.current!.expenseRowMenu(budget.structure.elements[0]).find((a) => a.label === label)!.onSelect())
    return { select }
  }

  beforeEach(() => {
    vi.mocked(trackEvent).mockClear()
    window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  })

  it('Past debts lists the history, newest first, each opening its read-only review', async () => {
    const user = userEvent.setup()
    const { select } = renderWithDialogs()
    select('Past debts (2)')
    const dialog = await screen.findByRole('dialog', { name: 'Past debts' })
    const items = within(dialog).getAllByRole('button', { name: /–/ })
    expect(items).toHaveLength(2)
    expect(items[0]).toHaveTextContent('Mar 1, 2020 – May 31, 2022')
    expect(items[0]).toHaveTextContent('9,000.00')
    expect(items[1]).toHaveTextContent('Jan 10, 2018 – Dec 31, 2019')
    await user.click(items[0])
    expect(useUiStore.getState().debtDetailsId).toBe('d-old')
    expect(useUiStore.getState().debtDetailsTab).toBe('overview')
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Past debts' })).toBeNull())
  })

  it('Mark as paid off asks first, then posts', async () => {
    const ids: unknown[] = []
    server.use(
      http.post('*/api/v1/debt/mark-debt-paid-off', async ({ request }) => {
        ids.push(((await request.json()) as { id: string }).id)
        return ok({ ...wireDebt, status: 'paid_off', isPaidOff: 1 })
      }),
    )
    const user = userEvent.setup()
    const { select } = renderWithDialogs()
    select('Mark as paid off')
    const dialog = await screen.findByRole('dialog', { name: 'Mark as paid off?' })
    expect(ids).toEqual([])
    await user.click(within(dialog).getByRole('button', { name: 'Mark as paid off' }))
    await waitFor(() => expect(ids).toEqual(['d1']))
    await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_MARK_PAID_OFF))
  })

  it('a refused delete shows up as a toast', async () => {
    const spy = vi.spyOn(toast, 'error')
    onTestFinished(() => spy.mockRestore())
    server.use(
      http.post('*/api/v1/debt/delete-debt', () =>
        HttpResponse.json({ success: false, message: 'Debt not found', code: 400, errors: {} }, { status: 400 }),
      ),
    )
    const user = userEvent.setup()
    const { select } = renderWithDialogs()
    select('Delete debt')
    await user.click(within(await screen.findByRole('dialog', { name: 'Delete this debt?' })).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('Debt not found', expect.anything()))
  })
})
