import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { coreHandlers, fixtureDebtRow, fixtureWireBudget } from '@/test/fixtures'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import { useUiStore } from '@/app/uiStore'
import { wireDebt } from '@/features/debts/testDebt'
import { useBudgetLineMenus } from './useBudgetLineMenus'

function renderMenus(userId = 'u1', adjust?: (b: BudgetDto) => void) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  adjust?.(budget)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  const { result } = renderHook(() => useBudgetLineMenus({ budget, plan: null, userId, onOpenSettings: vi.fn() }), { wrapper })
  const element = (id: string) => budget.structure.elements.find((el) => el.id === id) as BudgetElementDto
  return { result, budget, element }
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
    expect(useUiStore.getState().debtModal).toEqual({ categoryId: 'cat-food', currencyId: 'cur-usd', fromBudget: true })
  })

  it('a category in an envelope takes the envelope\'s currency', () => {
    const { result, element } = renderMenus()
    const envelope = element('env-1')
    const track = result.current.envelopeChildMenu(envelope.children[0], envelope).find((a) => a.label === 'Track as debt')
    act(() => track!.onSelect())
    expect(useUiStore.getState().debtModal).toEqual({ categoryId: 'cat-rent', currencyId: 'cur-eur', fromBudget: true })
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

describe('the debt row menu', () => {
  it('the owner gets Details and Edit', async () => {
    server.use(http.get('*/api/v1/debt/get-debt', () => HttpResponse.json({ success: true, message: '', data: { item: { ...wireDebt, id: 'debt-1' }, snapshots: [], payments: [] } })))
    const { result } = renderMenus()
    const actions = result.current.debtRowMenu(fixtureDebtRow())
    expect(labels(actions)).toEqual(['Details', 'Edit'])
    act(() => actions[0].onSelect())
    expect(useUiStore.getState().debtDetailsId).toBe('debt-1')
    act(() => actions[1].onSelect())
    await waitFor(() => expect(useUiStore.getState().debtModal?.debt?.id).toBe('debt-1'))
  })

  it('a guest sees no Edit in the debt row menu', () => {
    const { result } = renderMenus('u2')
    expect(result.current.debtRowMenu(fixtureDebtRow())).toEqual([])
  })

  it('the sheet offers the owner the edit pencil, and nobody else', () => {
    expect(renderMenus('u1').result.current.editAccess({ kind: 'debt', row: fixtureDebtRow() })).toBe(true)
    expect(renderMenus('u2').result.current.editAccess({ kind: 'debt', row: fixtureDebtRow() })).toBeNull()
  })
})
