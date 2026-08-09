import { afterIdFromDrop, afterIdInScope, applyMove } from './ordering'

const items = [
  { id: 'a', position: 0 },
  { id: 'b', position: 1 },
  { id: 'c', position: 2 },
]

it('reports null when the item landed first', () => {
  expect(afterIdFromDrop(['b', 'a', 'c'], 'b')).toBeNull()
})

it('reports the preceding id', () => {
  expect(afterIdFromDrop(['b', 'a', 'c'], 'a')).toBe('b')
  expect(afterIdFromDrop(['b', 'a', 'c'], 'c')).toBe('a')
})

it('reports null for an id that is not in the list', () => {
  expect(afterIdFromDrop(['a', 'b'], 'ghost')).toBeNull()
})

it('applyMove moves to the front and re-stamps dense positions', () => {
  expect(applyMove(items, 'c', null)).toEqual([
    { id: 'c', position: 0 },
    { id: 'a', position: 1 },
    { id: 'b', position: 2 },
  ])
})

it('applyMove places directly after the anchor', () => {
  expect(applyMove(items, 'a', 'b')).toEqual([
    { id: 'b', position: 0 },
    { id: 'a', position: 1 },
    { id: 'c', position: 2 },
  ])
})

it('applyMove appends when the anchor is unknown, mirroring the server', () => {
  expect(applyMove(items, 'a', 'ghost')).toEqual([
    { id: 'b', position: 0 },
    { id: 'c', position: 1 },
    { id: 'a', position: 2 },
  ])
})

it('applyMove leaves the list alone when the moved id is absent', () => {
  expect(applyMove(items, 'ghost', null)).toBe(items)
})

// A downward drag is where inferring the moved row from the reordered list goes
// wrong: [A,B,C] -> [B,C,A] first differs at index 0, which is B, the DISPLACED
// neighbour, not A. The dragged id must come from the drag event instead.
it('afterIdFromDrop reports the right anchor for a downward drag', () => {
  const after = ['B', 'C', 'A']
  expect(afterIdFromDrop(after, 'A')).toBe('C')
  // and the neighbour a naive diff would have blamed is genuinely first
  expect(afterIdFromDrop(after, 'B')).toBeNull()
})

it('afterIdFromDrop reports the right anchor for an upward drag', () => {
  expect(afterIdFromDrop(['C', 'A', 'B'], 'C')).toBeNull()
  expect(afterIdFromDrop(['A', 'C', 'B'], 'C')).toBe('A')
})

// Tags and labels are rendered as one list but hold independent sort-key
// sequences, so a label dropped at the top of the label section must report
// "front" (null), not the tag that happens to sit above it -- the label
// endpoint does not own that tag and would silently append instead.
describe('afterIdInScope', () => {
  const scope = (id: string) => (id.startsWith('tag') ? 'tag' : 'label')

  it('ignores rows of another kind above the moved row', () => {
    expect(afterIdInScope(['tag1', 'tag2', 'label1', 'label2'], 'label1', scope)).toBeNull()
  })

  it('reports the preceding row of the same kind', () => {
    expect(afterIdInScope(['tag1', 'tag2', 'label1', 'label2'], 'label2', scope)).toBe('label1')
    expect(afterIdInScope(['tag1', 'tag2', 'label1'], 'tag2', scope)).toBe('tag1')
  })

  it('reports null for the first row of the first kind', () => {
    expect(afterIdInScope(['tag1', 'label1'], 'tag1', scope)).toBeNull()
  })
})
