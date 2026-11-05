import type { NetWorthReportDto } from '@/api/dto/report'
import { netWorthGroupOf, netWorthSeries } from './reportMath'

describe('netWorthSeries', () => {
  const acc = (id: string, currencyId: string, balances: string[]) => ({ id, name: id, icon: '', currencyId, ownerUserId: 'u', balances })
  it('sums balances by group and in total', () => {
    const report: NetWorthReportDto = {
      currencyId: 'c',
      months: ['2026-01-01', '2026-02-01'],
      accounts: [acc('a', 'usd', ['10', '20']), acc('b', 'eur', ['1', '2']), acc('c', 'usd', ['5', '5'])],
    }
    const r = netWorthSeries(report, (a) => ({ id: a.currencyId, name: a.currencyId.toUpperCase() }))
    expect(r.groups).toEqual([
      { id: 'usd', name: 'USD', balances: ['15', '25'] },
      { id: 'eur', name: 'EUR', balances: ['1', '2'] },
    ])
    expect(r.total).toEqual(['16', '27'])
  })
  it('empty report gives empty arrays', () => {
    expect(netWorthSeries({ currencyId: 'c', months: [], accounts: [] }, () => ({ id: 'x', name: 'x' }))).toEqual({ groups: [], total: [] })
  })
})

describe('netWorthGroupOf', () => {
  const acc = (id: string, ownerUserId: string) => ({ id, name: id, icon: '', currencyId: 'c', ownerUserId, balances: [] })
  const groupOf = netWorthGroupOf({
    me: 'me',
    folderOf: { cash: 'f1', card: 'f2', loose: null },
    folders: [{ id: 'f1', name: 'Cash' }, { id: 'f2', name: 'Credit cards' }],
    owners: { bob: 'Bob' },
    names: { noFolder: 'No folder', otherOwner: 'Participant' },
  })
  it('puts my accounts in my folders', () => {
    expect(groupOf(acc('cash', 'me'))).toEqual({ id: 'folder:f1', name: 'Cash' })
    expect(groupOf(acc('card', 'me'))).toEqual({ id: 'folder:f2', name: 'Credit cards' })
  })
  it('my accounts without a folder get their own group', () => {
    expect(groupOf(acc('loose', 'me'))).toEqual({ id: 'nofolder', name: 'No folder' })
  })
  it("a partner's accounts go under the partner's name, whatever folder they sit in", () => {
    expect(groupOf(acc('cash', 'bob'))).toEqual({ id: 'owner:bob', name: 'Bob' })
    expect(groupOf(acc('x', 'eve'))).toEqual({ id: 'owner:eve', name: 'Participant' })
  })
})
