import { applyArrangement, arrangementFromBuckets, arrangementItem, computeElementMove, moveElementInArrangement } from './elementMove'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget } from '@/test/fixtures'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

const budget = coerceBudgetFixture(fixtureWireBudget)
const buckets = bucketElements(budget, makeBudgetExchange(budget, [usd, eur]))

it('moves an element onto another element in a different folder', () => {
  expect(computeElementMove(buckets, 'env-1', 'cat-food')).toEqual({ id: 'env-1', folderId: 'bf1', position: 0 })
})

it('moves onto a folder container id (end of folder)', () => {
  expect(computeElementMove(buckets, 'cat-food', 'bfolder:null')).toEqual({ id: 'cat-food', folderId: null, position: 1 })
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
  expect(arrangementItem(moved, 'env-1')).toEqual({ id: 'env-1', folderId: 'bf1', position: 0 })
  // dropping onto a container id appends
  const back = moveElementInArrangement(moved, 'env-1', 'bfolder:null')
  expect(arrangementItem(back, 'env-1')).toEqual({ id: 'env-1', folderId: null, position: 0 })
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
