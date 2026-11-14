import { suggestedPayment } from './suggestedPayment'

const base = { planned: '', actual: '0', previous: ['0', '0', '0'], fractionDigits: 2 }

describe('suggestedPayment', () => {
  it('takes the plan when it is above the spending', () => {
    expect(suggestedPayment({ ...base, planned: '2647.82', actual: '100' })).toBe('2647.82')
  })
  it('takes the spending when it is above the plan', () => {
    expect(suggestedPayment({ ...base, planned: '100', actual: '320.5' })).toBe('320.50')
    expect(suggestedPayment({ ...base, planned: '', actual: '50' })).toBe('50.00')
  })
  it('averages the three previous periods when this one has neither', () => {
    expect(suggestedPayment({ ...base, previous: ['300', '330', '0'] })).toBe('210.00')
  })
  it('is empty when everything is zero', () => {
    expect(suggestedPayment(base)).toBe('')
    expect(suggestedPayment({ ...base, previous: [] })).toBe('')
  })
  it('rounds to the currency digits', () => {
    expect(suggestedPayment({ ...base, previous: ['100', '100', '100.01'], fractionDigits: 2 })).toBe('100.00')
    expect(suggestedPayment({ ...base, previous: ['100', '100', '101'], fractionDigits: 0 })).toBe('100')
    expect(suggestedPayment({ ...base, planned: '10.126', fractionDigits: 2 })).toBe('10.13')
  })
})
