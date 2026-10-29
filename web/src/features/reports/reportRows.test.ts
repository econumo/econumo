import type { ReportRowDto } from '@/api/dto/report'
import { findRow, rowDisplayName } from './reportRows'

const t = (key: string) => key

it('the no-classification bucket is named for the grouping', () => {
  expect(rowDisplayName({ id: 'uncategorized', name: 'Uncategorized', kind: 'payee' }, t)).toBe('reports.spending.no_payee')
  expect(rowDisplayName({ id: 'uncategorized', name: 'Uncategorized', kind: 'tag' }, t)).toBe('reports.spending.no_tag')
  expect(rowDisplayName({ id: 'uncategorized', name: 'Uncategorized', kind: 'category' }, t)).toBe('common.uncategorized')
  expect(rowDisplayName({ id: 'c1', name: 'Food', kind: 'category' }, t)).toBe('Food')
})

it('finds a child row with its parent, and nothing for an unknown id', () => {
  const env: ReportRowDto = {
    id: 'e1', kind: 'envelope', name: 'Home', icon: '', isArchived: 0, ownerUserId: null, amounts: ['1'], planned: ['2'],
    children: [{ id: 'c1', kind: 'category', name: 'Rent', icon: '', isArchived: 0, amounts: ['1'] }],
  }
  expect(findRow([env], 'c1')).toMatchObject({ row: { id: 'c1', planned: [] }, parent: { id: 'e1' } })
  expect(findRow([env], 'nope')).toBeNull()
  expect(findRow([env], null)).toBeNull()
})
