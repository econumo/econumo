import { limitAmountFromInput } from './limitAmount'

it('clears on empty, zero and zero-formula input', () => {
  expect(limitAmountFromInput('')).toEqual({ ok: true, amount: null })
  expect(limitAmountFromInput('0')).toEqual({ ok: true, amount: null })
  expect(limitAmountFromInput('5-5')).toEqual({ ok: true, amount: null })
})

it('evaluates formulas and normalizes the amount string', () => {
  expect(limitAmountFromInput('100+50')).toEqual({ ok: true, amount: '150' })
  expect(limitAmountFromInput('1/3')).toEqual({ ok: true, amount: '0.33333333' })
  expect(limitAmountFromInput('200.50')).toEqual({ ok: true, amount: '200.5' })
})

it('rejects garbage', () => {
  expect(limitAmountFromInput('5+')).toEqual({ ok: false })
})
