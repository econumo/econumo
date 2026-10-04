import type { ReactNode } from 'react'
import { act, render, screen, within } from '@testing-library/react'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { useBudgetPeriodStore } from './budgetStore'
import { MonthFlows } from './MonthFlows'
import { planMonthFigures } from './phoneMonth'

// dnd-kit stand-in: each section's onDragStart/onDragEnd is captured and fired
// directly, in render order (income first, then savings)
let captured: { onDragStart?: (e: never) => void; onDragEnd: (e: never) => void }[] = []
vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  return {
    ...actual,
    DndContext: ({ onDragStart, onDragEnd, children }: { onDragStart?: (e: never) => void; onDragEnd: (e: never) => void; children: ReactNode }) => {
      captured.push({ onDragStart, onDragEnd })
      return children
    },
  }
})

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

// Freelance in a "Side gigs" income folder; Salaries folder-less
function plan(): BudgetPlanDto {
  const p = JSON.parse(JSON.stringify(fixtureWirePlan)) as BudgetPlanDto
  p.structure.folders = [...p.structure.folders, { id: 'bf-inc', name: 'Side gigs', position: 1 }]
  p.structure.elements = p.structure.elements.map((el) => (el.id === 'cat-freelance' ? { ...el, folderId: 'bf-inc' } : el))
  return p
}

function renderFlows() {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  budget.structure.savings = [
    { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '20', available: '80' },
    { id: 'acc-s2', type: 5, name: 'Holiday', icon: 'beach_access', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 1, budgeted: '50', spent: '0', available: '50' },
  ] as never
  const drag = { onMoveIncome: vi.fn(), onMoveIncomeIntoEnvelope: vi.fn(), onMoveIncomeFolder: vi.fn(), onMoveSavings: vi.fn() }
  render(
    <MonthFlows
      budget={budget}
      currencies={[usd, eur]}
      planMonth={planMonthFigures(plan(), [usd, eur], '2026-07-01', new Date(2026, 11, 1))}
      future={false}
      actionsColumn={false}
      renderPlanned={(_t, text) => text}
      drag={drag}
    />,
  )
  return drag
}

beforeEach(() => {
  captured = []
  useBudgetPeriodStore.setState({ unfoldedElements: {}, planFolds: {} })
})

it('income rows and folders carry hover grips; Uncategorized and savings history do not move', () => {
  renderFlows()
  const salaries = screen.getByRole('button', { name: 'move ie1' })
  expect(salaries.className).toContain('opacity-0')
  expect(salaries.className).toContain('group-hover/drag:opacity-100')
  expect(screen.getByRole('button', { name: 'move folder Side gigs' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'move acc-s1' })).toBeInTheDocument()
})

it('dropping an income row on another folder\'s row moves it there, after nothing', () => {
  const drag = renderFlows()
  const income = captured[captured.length - 2]
  act(() => income.onDragEnd({ active: { id: 'ie1' }, over: { id: 'cat-freelance' } } as never))
  expect(drag.onMoveIncome).toHaveBeenCalledWith({ id: 'ie1', folderId: 'bf-inc', position: 0, afterId: null })
  // the dropped order shows at once: Salaries now sits in the folder
  expect(within(screen.getByTestId('month-income-folder-bf-inc')).getByTestId('month-income-row-ie1')).toBeInTheDocument()
})

it('dropping an income row on a folder appends it to that folder', () => {
  const drag = renderFlows()
  const income = captured[captured.length - 2]
  act(() => income.onDragEnd({ active: { id: 'ie1' }, over: { id: 'bfolder:bf-inc' } } as never))
  expect(drag.onMoveIncome).toHaveBeenCalledWith({ id: 'ie1', folderId: 'bf-inc', position: 1, afterId: 'cat-freelance' })
})

it('savings rows reorder among themselves only', () => {
  const drag = renderFlows()
  const savings = captured[captured.length - 1]
  act(() => savings.onDragEnd({ active: { id: 'acc-s2' }, over: { id: 'acc-s1' } } as never))
  expect(drag.onMoveSavings).toHaveBeenCalledWith('acc-s2', null)
  // not a savings row: nothing happens
  act(() => savings.onDragEnd({ active: { id: 'acc-s1' }, over: { id: 'bfolder:bf-inc' } } as never))
  expect(drag.onMoveSavings).toHaveBeenCalledTimes(1)
})

describe('income categories and envelopes', () => {
  beforeEach(() => {
    useBudgetPeriodStore.setState({ unfoldedElements: { ie1: true }, planFolds: {} })
  })

  it('an unfolded income envelope lists its categories with grips, as a drop zone', () => {
    renderFlows()
    const drop = screen.getByTestId('envelope-drop-ie1')
    expect(within(drop).getByTestId('month-income-child-cat-salary')).toBeInTheDocument()
    expect(within(drop).getByRole('button', { name: 'move cat-salary' })).toBeInTheDocument()
  })

  it('a category dragged out of its envelope goes where it is dropped, and hides until the refetch', () => {
    const drag = renderFlows()
    const income = captured[captured.length - 2]
    act(() => income.onDragEnd({ active: { id: 'cat-salary' }, over: { id: 'bfolder:bf-inc' } } as never))
    expect(drag.onMoveIncome).toHaveBeenCalledWith({ id: 'cat-salary', folderId: 'bf-inc', position: 1, afterId: 'cat-freelance' }, expect.any(Function))
    expect(screen.queryByTestId('month-income-child-cat-salary')).toBeNull()
  })

  it('a category dropped on an envelope\'s list joins it, and hides until the refetch', () => {
    const drag = renderFlows()
    const income = captured[captured.length - 2]
    act(() => income.onDragEnd({ active: { id: 'cat-freelance' }, over: { id: 'benv:ie1' } } as never))
    expect(drag.onMoveIncomeIntoEnvelope).toHaveBeenCalledWith('cat-freelance', 'ie1', expect.any(Function))
    expect(screen.queryByTestId('month-income-row-cat-freelance')).toBeNull()
  })

  it('dropped back on its own envelope, or an envelope dropped on a list: nothing moves', () => {
    const drag = renderFlows()
    const income = captured[captured.length - 2]
    act(() => income.onDragEnd({ active: { id: 'cat-salary' }, over: { id: 'benv:ie1' } } as never))
    act(() => income.onDragEnd({ active: { id: 'cat-salary' }, over: { id: 'ie1' } } as never))
    act(() => income.onDragEnd({ active: { id: 'ie1' }, over: { id: 'benv:ie1' } } as never))
    expect(drag.onMoveIncome).not.toHaveBeenCalled()
    expect(drag.onMoveIncomeIntoEnvelope).not.toHaveBeenCalled()
    expect(screen.getByTestId('month-income-child-cat-salary')).toBeInTheDocument()
  })
})
