import { BudgetElementType } from '@/api/dto/budget'
import { segmentTagGroups, tagGroupKey, withTagGroupsAdjacent } from './tagGroups'

const el = (id: string, name: string, type: BudgetElementType = BudgetElementType.TAG, isArchived: 0 | 1 = 0) => ({ id, name, type, isArchived })
const ids = (items: { id: string }[]) => items.map((i) => i.id)

it('gathers live same-name tags at the first one, case and spaces aside', () => {
  const items = [el('a', 'Travel'), el('food', 'Food', BudgetElementType.CATEGORY), el('b', ' travel '), el('c', 'Gifts')]
  const segments = segmentTagGroups(items, (i) => i, 'bf1')
  expect(segments.map((s) => (s.kind === 'row' ? s.item.id : `group:${ids(s.items).join(',')}`))).toEqual(['group:a,b', 'food', 'c'])
  const group = segments[0]
  expect(group.kind === 'group' && group.name).toBe('Travel')
  expect(group.kind === 'group' && group.key).toBe(tagGroupKey('bf1', 'TRAVEL'))
  expect(ids(withTagGroupsAdjacent(items, (i) => i))).toEqual(['a', 'b', 'food', 'c'])
})

it('a lone tag, a live tag next to an archived one and a same-name category stay rows of their own', () => {
  const items = [el('a', 'Travel'), el('b', 'Travel', BudgetElementType.TAG, 1), el('cat', 'Travel', BudgetElementType.CATEGORY)]
  expect(segmentTagGroups(items, (i) => i, null).every((s) => s.kind === 'row')).toBe(true)
})

it('archived same-name tags group with each other', () => {
  const items = [el('a', 'Hawaii', BudgetElementType.TAG, 1), el('b', 'Hawaii', BudgetElementType.TAG, 1), el('c', 'Rome', BudgetElementType.TAG, 1)]
  const segments = segmentTagGroups(items, (i) => i, 'archived')
  expect(segments.map((s) => (s.kind === 'row' ? s.item.id : `group:${ids(s.items).join(',')}`))).toEqual(['group:a,b', 'c'])
})

it('the same name in two folders folds apart', () => {
  expect(tagGroupKey('bf1', 'Travel')).not.toBe(tagGroupKey(null, 'Travel'))
})
