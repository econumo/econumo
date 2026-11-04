import { act, renderHook } from '@testing-library/react'
import { initialDebtFormState, projectForm, useDebtForm } from './useDebtForm'
import { wireDebt } from './testDebt'

const debt = { ...wireDebt, periodRate: '0.01', currentBalance: '900' }

it('projects nothing under another schedule: the stored rate and next due belong to the old one', () => {
  const form = { ...initialDebtFormState({ debt }, [], null), schedule: 'weekly' as const, balance: '600' }
  expect(projectForm(form, debt)).toEqual({ status: 'unknown' })
})

it('an amount edited after the schedule changed leaves the last payment date alone', () => {
  const { result } = renderHook(() => useDebtForm({ debt }, [], null))
  act(() => result.current.patch({ schedule: 'weekly' }))
  act(() => result.current.setBalance('600'))
  expect(result.current.form.lastPaymentDate).toBe(debt.lastPaymentDate)
  expect(result.current.projection).toEqual({ status: 'unknown' })
})
