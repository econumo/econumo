import { fuzzyMatch } from './fuzzy'

it('fuzzy-matches subsequences like the Vue component', () => {
  expect(fuzzyMatch('US Dollar', 'usd')).toBe(true)
  expect(fuzzyMatch('Euro', 'eo')).toBe(true)
  expect(fuzzyMatch('Euro', 'x')).toBe(false)
})

it('is case-insensitive and matches everything on an empty pattern', () => {
  expect(fuzzyMatch('Groceries', 'GROC')).toBe(true)
  expect(fuzzyMatch('anything', '')).toBe(true)
})
