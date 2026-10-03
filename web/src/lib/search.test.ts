import { matchRank, matchesTerms, rankByName } from './search'

describe('matchRank', () => {
  it('ranks exact < prefix < substring < subsequence and rejects non-matches', () => {
    expect(matchRank('Food', 'food')).toBe(0)
    expect(matchRank('Groceries', 'gro')).toBe(1)
    expect(matchRank('Groceries', 'cer')).toBe(2)
    expect(matchRank('Groceries', 'grcries')).toBe(3)
    expect(matchRank('Groceries', 'xyz')).toBeNull()
  })
  it('matches everything on an empty or blank query', () => {
    expect(matchRank('Anything', '')).toBe(0)
    expect(matchRank('Anything', '   ')).toBe(0)
  })
})

describe('rankByName', () => {
  const items = [
    { name: 'Tagged groceries' }, // substring "gro"
    { name: 'Gym' },              // no match for "gro"
    { name: 'Groceries' },        // prefix
    { name: 'gRo' },              // exact (case-insensitive)
    { name: 'Garage repairs o' }, // subsequence g..r..o
  ]
  it('filters and orders by rank, keeping input order on ties', () => {
    expect(rankByName(items, (i) => i.name, 'gro').map((i) => i.name)).toEqual([
      'gRo', 'Groceries', 'Tagged groceries', 'Garage repairs o',
    ])
  })
  it('returns the input unchanged for an empty query', () => {
    expect(rankByName(items, (i) => i.name, '')).toEqual(items)
  })
})

describe('rankByName with several names per item', () => {
  const currencies = [
    { name: 'Euro', code: 'EUR', symbol: '€' },
    { name: 'US Dollar', code: 'USD', symbol: '$' },
    { name: 'Australian Dollar', code: 'AUD', symbol: 'A$' },
  ]
  const names = (c: (typeof currencies)[number]) => [c.name, c.code, c.symbol]
  it('ranks by the best-matching name', () => {
    // "Australian Dollar" holds u..s..d too, so it trails as a loose match
    expect(rankByName(currencies, names, 'usd').map((c) => c.code)).toEqual(['USD', 'AUD'])
    expect(rankByName(currencies, names, 'dolar').map((c) => c.code)).toEqual(['USD', 'AUD'])
    expect(rankByName(currencies, names, '$').map((c) => c.code)).toEqual(['USD', 'AUD'])
  })
})

describe('matchesTerms', () => {
  const fields = { text: ['Morning latte at Starbucks', 'Visa Gold', 'Coffee'], exact: ['12.50', '2026-10-01 08:00:00', '-'] }
  it('requires every term (AND)', () => {
    expect(matchesTerms(fields, 'visa coffee')).toBe(true)
    expect(matchesTerms(fields, 'visa rent')).toBe(false)
  })
  it('lets a term skip characters within one word', () => {
    expect(matchesTerms(fields, 'stbcks')).toBe(true)
    expect(matchesTerms(fields, 'cofee')).toBe(true)
  })
  it('never matches a subsequence that spans words', () => {
    expect(matchesTerms({ text: ['come for elections'], exact: [] }, 'cofe')).toBe(false)
  })
  it('matches exact fields by substring only', () => {
    expect(matchesTerms(fields, '12.5')).toBe(true)
    expect(matchesTerms({ text: [], exact: ['1250'] }, '12.50')).toBe(false)
    expect(matchesTerms({ text: [], exact: ['12.50'] }, '1250')).toBe(false)
  })
  it('matches everything for an empty query', () => {
    expect(matchesTerms(fields, '  ')).toBe(true)
  })
})
