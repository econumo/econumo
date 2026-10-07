import type { Active, ClientRect, DroppableContainer } from '@dnd-kit/core'
import { accountCollisions, bucketsFromAccounts, moveAccount, accountMoveFrom } from './accountOrdering'
import type { FolderBucket } from './accountOrdering'
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

describe('accountCollisions', () => {
  // Layout of a phone list: folder f1 holds a1, a2; folder f2 holds a3; f3 is
  // empty. Every folder section registers two droppables covering the whole
  // section (its sortable, bare id, and its `folder:` container).
  const rect = (top: number, height: number): ClientRect => ({ top, bottom: top + height, left: 0, right: 300, width: 300, height })
  const layout: Record<string, ClientRect> = {
    f1: rect(0, 100), 'folder:f1': rect(0, 100), a1: rect(30, 30), a2: rect(60, 30),
    f2: rect(110, 70), 'folder:f2': rect(110, 70), a3: rect(140, 30),
    f3: rect(190, 40), 'folder:f3': rect(190, 40),
  }
  const buckets: FolderBucket[] = [
    { folderId: 'f1', accountIds: ['a1', 'a2'] },
    { folderId: 'f2', accountIds: ['a3'] },
    { folderId: 'f3', accountIds: [] },
  ]
  const collide = (activeId: string, pointer: { x: number; y: number } | null, current = buckets) =>
    accountCollisions(current)({
      active: { id: activeId } as Active,
      collisionRect: rect((pointer?.y ?? layout[activeId].top) - 15, 30),
      droppableRects: new Map(Object.entries(layout)),
      droppableContainers: Object.keys(layout).map((id) => ({ id }) as DroppableContainer),
      pointerCoordinates: pointer,
    }).map((c) => c.id)

  it('picks the other account row under the pointer, not the folder sections around it', () => {
    expect(collide('a1', { x: 10, y: 150 })).toEqual(['a3'])
  })

  it('never reports a folder that still holds other accounts, so a drop cannot flip to "end of folder"', () => {
    // pointer on f2's header: neither the bare folder nor its container is a target
    expect(collide('a1', { x: 10, y: 120 })).toEqual([])
    // pointer on the dragged row's own slot: stay put
    expect(collide('a1', { x: 10, y: 40 })).toEqual([])
  })

  it('still targets an empty folder through its container', () => {
    expect(collide('a1', { x: 10, y: 210 })).toEqual(['folder:f3'])
  })

  it('treats a folder holding only the dragged account as empty', () => {
    expect(collide('a3', { x: 10, y: 120 })).toEqual(['folder:f2'])
  })

  it('falls back to the nearest row or empty folder without a pointer (keyboard)', () => {
    expect(collide('a1', null)).toEqual(['a2'])
  })
})
