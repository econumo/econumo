import { getChangedPositions } from './ordering'

const items = [
  { id: 'a', position: 0 },
  { id: 'b', position: 1 },
  { id: 'c', position: 2 },
]

it('returns an empty array when nothing moved', () => {
  expect(getChangedPositions(items, ['a', 'b', 'c'])).toEqual([])
})

it('reports only the items whose index changed', () => {
  expect(getChangedPositions(items, ['b', 'a', 'c'])).toEqual([
    { id: 'b', position: 0 },
    { id: 'a', position: 1 },
  ])
})

it('handles a move to the end', () => {
  expect(getChangedPositions(items, ['b', 'c', 'a'])).toEqual([
    { id: 'b', position: 0 },
    { id: 'c', position: 1 },
    { id: 'a', position: 2 },
  ])
})

it('ignores unknown ids', () => {
  expect(getChangedPositions(items, ['ghost', 'a', 'b', 'c'])).toEqual([
    { id: 'a', position: 1 },
    { id: 'b', position: 2 },
    { id: 'c', position: 3 },
  ])
})
