import { bucketsFromAccounts, moveAccount, accountMoveFrom } from './accountOrdering'
import type { AccountDto } from '@/api/dto/account'

const owner = { id: 'u1', avatar: '', name: 'Ada' }
const usd = { id: 'usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const account = (id: string, folderId: string | null, position: number): AccountDto => ({
  id, owner, folderId, name: id, position, currency: usd, balance: '0', type: 1, icon: 'wallet', sharedAccess: [],
})

const accounts = [account('a1', 'f1', 0), account('a2', 'f1', 1), account('a3', 'f2', 2)]

it('buckets accounts into folders by position', () => {
  expect(bucketsFromAccounts(accounts, ['f1', 'f2'])).toEqual([
    { folderId: 'f1', accountIds: ['a1', 'a2'] },
    { folderId: 'f2', accountIds: ['a3'] },
  ])
})

it('moves within a folder and reports the relative move', () => {
  const buckets = bucketsFromAccounts(accounts, ['f1', 'f2'])
  const moved = moveAccount(buckets, 'a1', 'a2')
  expect(moved[0].accountIds).toEqual(['a2', 'a1'])
  expect(accountMoveFrom(moved, 'a1')).toEqual({ id: 'a1', folderId: 'f1', afterId: 'a2' })
})

it('moves across folders onto another account', () => {
  const buckets = bucketsFromAccounts(accounts, ['f1', 'f2'])
  const moved = moveAccount(buckets, 'a1', 'a3')
  expect(moved[0].accountIds).toEqual(['a2'])
  expect(moved[1].accountIds).toEqual(['a1', 'a3'])
  expect(accountMoveFrom(moved, 'a1')).toEqual({ id: 'a1', folderId: 'f2', afterId: null })
})

it('drops into an empty folder via the container id', () => {
  const withEmpty = [...bucketsFromAccounts(accounts, ['f1', 'f2']), { folderId: 'f3', accountIds: [] }]
  const moved = moveAccount(withEmpty, 'a3', 'folder:f3')
  expect(moved[1].accountIds).toEqual([])
  expect(moved[2].accountIds).toEqual(['a3'])
  expect(accountMoveFrom(moved, 'a3')).toEqual({ id: 'a3', folderId: 'f3', afterId: null })
})

it('drops onto a bare folder id (the folder sortable is a droppable too)', () => {
  const buckets = bucketsFromAccounts(accounts, ['f1', 'f2'])
  const moved = moveAccount(buckets, 'a1', 'f2')
  expect(moved[0].accountIds).toEqual(['a2'])
  expect(moved[1].accountIds).toEqual(['a3', 'a1'])
})

it('reports null for an account that is in no bucket', () => {
  const buckets = bucketsFromAccounts(accounts, ['f1', 'f2'])
  expect(accountMoveFrom(buckets, 'ghost')).toBeNull()
})
