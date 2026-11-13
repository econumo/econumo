import { act, renderHook } from '@testing-library/react'
import { buildCreatePayload, buildTermsPayload, initialCreateState, initialTermsState, plainAmount, useDebtTermsForm } from './useDebtForm'
import { wireDebt, wireDebtNotSetUp } from './testDebt'

it('a set-up debt starts from the loan facts', () => {
  const { result } = renderHook(() => useDebtTermsForm({ ...wireDebt, currentBalance: '900' }))
  expect(result.current.form).toEqual({
    name: 'Car loan', icon: 'directions_car', rules: [{ id: 'r1', categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null }], currencyId: 'cur-usd',
    amountBorrowed: '1200', startDate: '2026-11-05', lastPaymentDate: '2027-11-05', payment: '100', schedule: 'monthly',
  })
})

it('a debt not set up starts with the loan facts empty', () => {
  const { result } = renderHook(() => useDebtTermsForm(wireDebtNotSetUp))
  expect(result.current.form).toMatchObject({ amountBorrowed: '', startDate: '', lastPaymentDate: '', payment: '100' })
  act(() => result.current.patch({ amountBorrowed: '600' }))
  expect(result.current.dirty).toBe(true)
})

it('setup without the loan facts sends payment, repeats and start date only', () => {
  const initial = initialTermsState(wireDebtNotSetUp)
  const payload = buildTermsPayload({ ...initial, payment: '120', startDate: '2026-01-01' }, initial, wireDebtNotSetUp)
  // the currency as loaded goes as none: the server keeps the stored one
  expect(payload).toEqual({
    id: 'd1', name: 'Car loan', currencyId: '',
    startDate: '2026-01-01', payment: '120', schedule: 'monthly',
  })
})

it('setup with the loan facts sends them', () => {
  const initial = initialTermsState(wireDebtNotSetUp)
  const facts = { ...initial, amountBorrowed: '2 000', startDate: '2025-08-05', lastPaymentDate: '2027-08-05' }
  const payload = buildTermsPayload(facts, initial, wireDebtNotSetUp)
  expect(payload).toMatchObject({ amountBorrowed: '2000', startDate: '2025-08-05', lastPaymentDate: '2027-08-05', payment: '100', schedule: 'monthly' })
  expect(payload).not.toHaveProperty('balance')
})

it('a set-up debt sends no loan facts and no start date while none was touched', () => {
  const initial = initialTermsState(wireDebt)
  const payload = buildTermsPayload({ ...initial, name: 'Renamed' }, initial, wireDebt)
  expect(payload).toEqual({
    id: 'd1', name: 'Renamed', currencyId: '',
  })
  // a changed start goes, alone
  expect(buildTermsPayload({ ...initial, startDate: '2026-11-01' }, initial, wireDebt)).toEqual({
    id: 'd1', name: 'Car loan', currencyId: '', startDate: '2026-11-01',
  })
  // "1 200" typed over a stored 1200 is the same amount
  expect(buildTermsPayload({ ...initial, amountBorrowed: '1 200' }, initial, wireDebt)).not.toHaveProperty('amountBorrowed')
})

it('a touched loan fact sends every fact, so the server reads one correction', () => {
  const initial = initialTermsState(wireDebt)
  expect(buildTermsPayload({ ...initial, payment: '110' }, initial, wireDebt)).toMatchObject({
    amountBorrowed: '1200', lastPaymentDate: '2027-11-05', payment: '110', schedule: 'monthly',
  })
})

it('a rule edited sends every rule, with the kept ids', () => {
  const initial = initialTermsState(wireDebt)
  const rules = [{ ...initial.rules[0], payeeId: 'p1' }, { categoryId: 'cat-rent', payeeId: null, tagId: null, labelId: null }]
  expect(buildTermsPayload({ ...initial, rules }, initial, wireDebt).rules).toEqual([
    { id: 'r1', categoryId: 'cat-food', payeeId: 'p1', tagId: null, labelId: null },
    { categoryId: 'cat-rent', payeeId: null, tagId: null, labelId: null },
  ])
})

it('a new debt is created in its budget without terms, every rule kept', () => {
  const state = initialCreateState({ categoryId: 'cat-food', budgetId: 'b1', currencyId: 'cur-usd' }, [])
  const rules = [{ ...state.rules[0], labelId: 'label1' }, { categoryId: 'cat-rent', payeeId: null, tagId: null, labelId: null }]
  const payload = buildCreatePayload({ ...state, name: ' Car ', payment: '50', rules }, 'b1')
  expect(payload).toEqual({
    budgetId: 'b1', name: 'Car', currencyId: 'cur-usd',
    rules: [
      { categoryId: 'cat-food', payeeId: null, tagId: null, labelId: 'label1' },
      { categoryId: 'cat-rent', payeeId: null, tagId: null, labelId: null },
    ],
    startDate: null, payment: '50', schedule: 'monthly',
  })
  expect(payload).not.toHaveProperty('balance')
  expect(payload).not.toHaveProperty('amountBorrowed')
  expect(payload).not.toHaveProperty('lastPaymentDate')
})

it('a new debt without a currency from its row sends none: the budget\'s is taken', () => {
  const state = { ...initialCreateState({ budgetId: 'b2' }, []), name: 'Car', payment: '50' }
  expect(buildCreatePayload(state, 'b2')).not.toHaveProperty('currencyId')
})

it.each([
  ['2000', '2000'],
  ['2 000', '2000'],
  ['2 000,50', '2000.5'],
  ['2,000.50', '2000.5'],
  ['1,000,000', '1000000'],
  ['2.000,50', '2000.5'],
  ['1,5', '1.5'],
  ['1,50', '1.5'],
  ['1,200', '1200'],
  ['0,125', '0.125'],
  ['1.500', '1.5'],
  ['0', '0'],
  ['-5', '-5'],
])('reads the typed amount %s as %s', (typed, read) => {
  expect(plainAmount(typed)).toBe(read)
})

it.each(['', 'abc', '5+5', '2,00,0', '1.2.3', '2.'])('reads %s as no amount', (typed) => {
  expect(plainAmount(typed)).toBeNull()
})

it('the icon goes only once changed', () => {
  const initial = initialTermsState(wireDebt)
  expect(buildTermsPayload(initial, initial, wireDebt)).not.toHaveProperty('icon')
  expect(buildTermsPayload({ ...initial, icon: 'home' }, initial, wireDebt)).toMatchObject({ icon: 'home' })
})
