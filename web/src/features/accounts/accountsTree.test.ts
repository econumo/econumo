import { buildAccountsTree, SYNTHETIC_FOLDER_ID } from './accountsTree'
import type { AccountDto } from '@/api/dto/account'
import type { FolderDto } from '@/api/dto/folder'

const usd = { id: 'usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2, scope: 'global' as const, isArchived: 0 as const, isHidden: 0 as const }
const eur = { id: 'eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2, scope: 'global' as const, isArchived: 0 as const, isHidden: 0 as const }
const owner = { id: 'u1', avatar: '', name: 'Ada' }

const account = (over: Partial<AccountDto>): AccountDto => ({
  id: 'a', owner, folderId: 'f1', name: 'Acc', position: 0, currency: usd,
  balance: 0, type: 1, icon: 'wallet', sharedAccess: [], ...over,
})
const folder = (over: Partial<FolderDto>): FolderDto => ({ id: 'f1', name: 'General', position: 0, isVisible: 1, ...over })

// converts 1 EUR = 2 USD for easy math
const exch = (from: string, to: string, amount: number) => (from === to ? amount : from === 'eur' ? amount * 2 : amount / 2)

it('groups position-sorted accounts into visible position-sorted folders', () => {
  const tree = buildAccountsTree(
    [
      account({ id: 'a2', folderId: 'f2', position: 1, balance: 5 }),
      account({ id: 'a1', folderId: 'f1', position: 0, balance: 3 }),
    ],
    [folder({ id: 'f2', name: 'Second', position: 1 }), folder({ id: 'f1', position: 0 })],
    usd,
    exch,
    'All accounts',
  )
  expect(tree.map((i) => i.folder.id)).toEqual(['f1', 'f2'])
  expect(tree[0].accounts.map((a) => a.id)).toEqual(['a1'])
})

it('excludes hidden folders and their accounts entirely', () => {
  const tree = buildAccountsTree(
    [account({ id: 'a1', folderId: 'fh', balance: 5 })],
    [folder({ id: 'fh', isVisible: 0 })],
    usd,
    exch,
    'All accounts',
  )
  expect(tree).toEqual([])
})

it('drops folders with no accounts', () => {
  const tree = buildAccountsTree([account({ id: 'a1', folderId: 'f1' })], [folder({}), folder({ id: 'f-empty', position: 1 })], usd, exch, 'All accounts')
  expect(tree.map((i) => i.folder.id)).toEqual(['f1'])
})

it('puts folderless accounts into a trailing synthetic translated folder', () => {
  const tree = buildAccountsTree(
    [account({ id: 'a1', folderId: 'f1' }), account({ id: 'a2', folderId: null, position: 1 })],
    [folder({})],
    usd,
    exch,
    'All accounts',
  )
  expect(tree).toHaveLength(2)
  expect(tree[1].folder.id).toBe(SYNTHETIC_FOLDER_ID)
  expect(tree[1].folder.name).toBe('All accounts')
  expect(tree[1].accounts.map((a) => a.id)).toEqual(['a2'])
})

it('uses the native currency total when a folder has one currency', () => {
  const tree = buildAccountsTree(
    [account({ id: 'a1', balance: 10, currency: eur }), account({ id: 'a2', position: 1, balance: 5, currency: eur })],
    [folder({})],
    usd,
    exch,
    'All accounts',
  )
  expect(tree[0].currency).toEqual(eur)
  expect(tree[0].total).toBe(15)
})

it('converts to the user currency when the folder mixes currencies', () => {
  const tree = buildAccountsTree(
    [account({ id: 'a1', balance: 10, currency: eur }), account({ id: 'a2', position: 1, balance: 5, currency: usd })],
    [folder({})],
    usd,
    exch,
    'All accounts',
  )
  expect(tree[0].currency).toEqual(usd)
  // 10 EUR * 2 + 5 USD = 25 USD
  expect(tree[0].total).toBe(25)
})
