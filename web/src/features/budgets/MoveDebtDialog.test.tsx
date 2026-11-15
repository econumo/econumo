import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { DebtDto } from '@/api/dto/debt'
import { moveDebtCandidates } from './MoveDebtDialog'

const el = (id: string, type: number, extra: Partial<BudgetElementDto> = {}) =>
  ({ id, type, name: id, icon: 'i', isArchived: 0, currencyId: null, children: [], ...extra }) as unknown as BudgetElementDto

const budget = (elements: BudgetElementDto[]) =>
  ({ meta: { id: 'b1', currencyId: 'usd' }, structure: { elements } }) as unknown as BudgetDto

it('offers top-level expense categories and envelopes in the debt currency with no active debt', () => {
  const b = budget([
    el('mortgage', BudgetElementType.CATEGORY),
    el('housing', BudgetElementType.ENVELOPE),
    el('salary', BudgetElementType.INCOME_CATEGORY),
    el('tag', BudgetElementType.TAG),
    el('archived', BudgetElementType.CATEGORY, { isArchived: 1 }),
    el('euro', BudgetElementType.CATEGORY, { currencyId: 'eur' }),
    el('car', BudgetElementType.CATEGORY),
    el('listed-busy', BudgetElementType.CATEGORY),
    el('paid-off', BudgetElementType.CATEGORY, { debt: { isPaidOff: 1 } as BudgetElementDto['debt'] }),
  ])
  const debts = [{ budgetId: 'b1', elementId: 'listed-busy', status: 'active' } as DebtDto, { budgetId: 'b1', elementId: 'car', status: 'paid_off' } as DebtDto]
  const got = moveDebtCandidates(b, { debtId: 'd1', elementId: 'mortgage' }, debts).map((e) => e.id)
  expect(got).toEqual(['car', 'housing', 'paid-off'])
})
