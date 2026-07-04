import { computeElementMove } from './elementMove'
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
