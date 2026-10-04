import { applyArrangement, arrangementFromBuckets, arrangementItem, computeElementMove, moveElementInArrangement, dropIndicatorFor, envelopeOfDrop, placeFromEnvelope, withoutElement } from './elementMove'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget } from '@/test/fixtures'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

const budget = coerceBudgetFixture(fixtureWireBudget)
const buckets = bucketElements(budget, makeBudgetExchange(budget, [usd, eur]))

it('moves an element onto another element in a different folder', () => {
  expect(computeElementMove(buckets, 'env-1', 'cat-food')).toEqual({ id: 'env-1', folderId: 'bf1', position: 0, afterId: null })
})

it('moves onto a folder container id (end of folder)', () => {
  expect(computeElementMove(buckets, 'cat-food', 'bfolder:null')).toEqual({ id: 'cat-food', folderId: null, position: 1, afterId: 'env-1' })
})

it('no-op when dropped onto its own spot', () => {
  expect(computeElementMove(buckets, 'cat-food', 'cat-food')).toBeNull()
})

it('arrangement round-trip: move live across containers and derive the wire item', () => {
  const arrangement = arrangementFromBuckets(buckets)
  expect(arrangement).toEqual([
    { folderId: 'bf1', ids: ['cat-food'] },
    { folderId: null, ids: ['env-1'] },
  ])
  const moved = moveElementInArrangement(arrangement, 'env-1', 'cat-food')
  expect(moved).toEqual([
    { folderId: 'bf1', ids: ['env-1', 'cat-food'] },
    { folderId: null, ids: [] },
  ])
  expect(arrangementItem(moved, 'env-1')).toEqual({ id: 'env-1', folderId: 'bf1', position: 0, afterId: null })
  // dropping onto a container id appends
  const back = moveElementInArrangement(moved, 'env-1', 'bfolder:null')
  expect(arrangementItem(back, 'env-1')).toEqual({ id: 'env-1', folderId: null, position: 0, afterId: null })
})

// An EMPTY folder has no rows, so the only rects under the pointer are the
// section droppable (`bfolder:<id>`) and the folder's own sortable (the bare
// `<id>`, used for folder reordering). Resolving to the bare id must still drop
// the element INTO that folder, not silently no-op.
it('drops into an empty folder addressed by its bare folder id', () => {
  const arrangement = [
    { folderId: 'bf-empty' as string | null, ids: [] as string[] },
    { folderId: 'bf1' as string | null, ids: ['cat-food'] },
    { folderId: null as string | null, ids: ['env-1'] },
  ]
  const moved = moveElementInArrangement(arrangement, 'env-1', 'bf-empty')
  expect(arrangementItem(moved, 'env-1')).toEqual({ id: 'env-1', folderId: 'bf-empty', position: 0, afterId: null })
})

it('applyArrangement patches folderId + order; archived elements untouched', () => {
  const arrangement = moveElementInArrangement(arrangementFromBuckets(buckets), 'env-1', 'cat-food')
  const patched = applyArrangement(budget, arrangement)
  const byId = new Map(patched.structure.elements.map((e) => [e.id, e]))
  expect(byId.get('env-1')!.folderId).toBe('bf1')
  expect(byId.get('env-1')!.position).toBeLessThan(byId.get('cat-food')!.position)
  expect(byId.get('tag-old')!.isArchived).toBe(1)
  // re-bucketing the patched budget shows the new arrangement
  const rebucketed = bucketElements(patched, makeBudgetExchange(patched, [usd, eur]))
  expect(rebucketed.withFolder[0].elements.map((e) => e.id)).toEqual(['env-1', 'cat-food'])
  expect(rebucketed.withoutFolder.elements).toEqual([])
})

it('a category dragged out of an envelope lands where a row dropped there would', () => {
  const base = [
    { folderId: 'f1', ids: ['a', 'b'] },
    { folderId: null, ids: ['c'] },
  ]
  expect(placeFromEnvelope(base, 'x', 'b')).toEqual({ id: 'x', folderId: 'f1', position: 1, afterId: 'a' })
  expect(placeFromEnvelope(base, 'x', 'bfolder:null')).toEqual({ id: 'x', folderId: null, position: 1, afterId: 'c' })
  expect(placeFromEnvelope(base, 'x', 'nowhere')).toBeNull()
})

it('withoutElement drops an element from the top level and from any envelope', () => {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  const envelope = budget.structure.elements.find((el) => el.id === 'env-1')!
  const childId = envelope.children[0].id
  const hidden = withoutElement(withoutElement(budget, childId), 'cat-food')
  expect(hidden.structure.elements.find((el) => el.id === 'env-1')!.children.some((c) => c.id === childId)).toBe(false)
  expect(hidden.structure.elements.some((el) => el.id === 'cat-food')).toBe(false)
})

describe('the insertion line', () => {
  const base = [
    { folderId: 'f1', ids: ['a', 'b', 'c'] },
    { folderId: 'f2', ids: [] as string[] },
    { folderId: null, ids: ['d'] },
  ]
  const open = { fromEnvelope: false, isFolded: () => false }

  it('sits after the row the drop lands behind, or before the first row of its folder', () => {
    // moving down within a folder lands after the row it is dropped on
    expect(dropIndicatorFor(base, 'a', 'c', open)).toEqual({ kind: 'row', id: 'c', edge: 'after' })
    // moving up lands before it
    expect(dropIndicatorFor(base, 'c', 'a', open)).toEqual({ kind: 'row', id: 'a', edge: 'before' })
    // into another folder, onto its first row
    expect(dropIndicatorFor(base, 'b', 'd', open)).toEqual({ kind: 'row', id: 'd', edge: 'before' })
  })

  it('sits under the header of an empty or folded folder, and is gone on a drop onto its own spot', () => {
    expect(dropIndicatorFor(base, 'a', 'bfolder:f2', open)).toEqual({ kind: 'folder', folderId: 'f2' })
    expect(dropIndicatorFor(base, 'd', 'b', { fromEnvelope: false, isFolded: (f) => f === 'f1' })).toEqual({ kind: 'folder', folderId: 'f1' })
    expect(dropIndicatorFor(base, 'b', 'b', open)).toBeNull()
  })

  it('a category from an envelope gets a row-level line; an envelope list gets the category-level one', () => {
    expect(dropIndicatorFor(base, 'x', 'b', { fromEnvelope: true, isFolded: () => false })).toEqual({ kind: 'row', id: 'a', edge: 'after' })
    expect(dropIndicatorFor(base, 'a', 'benv:env-1', open)).toEqual({ kind: 'envelope', envelopeId: 'env-1' })
  })
})

it('both envelope drop zones name their envelope; anything else names none', () => {
  expect(envelopeOfDrop('benv:e1')).toBe('e1')
  expect(envelopeOfDrop('benvh:e1')).toBe('e1')
  expect(envelopeOfDrop('bfolder:null')).toBeNull()
  expect(envelopeOfDrop('e1')).toBeNull()
})
