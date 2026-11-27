import { describe, expect, it } from 'vitest'
import { resolveNames, categoryKey, type OwnerNames, type RowOwner } from './ownerLists'
import type { PreparedRow } from './prepare'

let line = 1
function row(cells: Partial<PreparedRow>): PreparedRow {
  line++
  return {
    fileRow: line, raw: '', status: 'ok', reason: null, date: '2026-01-02', time: null, amount: '-1', account: null,
    description: '', payee: '', category: '', tag: '', labels: '', ...cells,
  }
}

const empty = (): OwnerNames => ({ categories: new Set(), payees: new Set(), tags: new Set(), labels: new Set() })
const me: RowOwner = { ownerId: 'me', readOnly: false }

describe('resolveNames', () => {
  it('leaves out a budget tag named like a reporting tag and the reverse, in the lists or made earlier in the file', () => {
    const lists = { ...empty(), labels: new Set(['holiday']) }
    const rows = [row({ tag: 'Holiday' }), row({ tag: 'Trip', labels: 'trip;Food' }), row({ labels: 'Car' }), row({ tag: 'car' })]
    const { byRow, names } = resolveNames(rows, ';', () => me, () => lists)
    expect(rows.map((r) => byRow.get(r.fileRow))).toEqual([
      { category: '', payee: '', tag: '', labels: [] },
      { category: '', payee: '', tag: 'Trip', labels: ['Food'] },
      { category: '', payee: '', tag: '', labels: ['Car'] },
      { category: '', payee: '', tag: '', labels: [] },
    ])
    expect(names.tag.map((n) => [n.name, n.outcome])).toEqual([['Holiday', 'taken'], ['Trip', 'new'], ['car', 'taken']])
    expect(names.labels.map((n) => [n.name, n.outcome])).toEqual([['trip', 'taken'], ['Food', 'new'], ['Car', 'new']])
  })

  it('a user grant on a shared account uses the owner’s names and leaves new ones out of the row', () => {
    const owner = { ...empty(), categories: new Set([categoryKey('expense', 'Food')]), payees: new Set(['market']) }
    const rows = [row({ account: 'Shared', category: 'food', payee: 'Shop' }), row({ account: 'Mine', category: 'food', payee: 'Shop' })]
    const ownerOf = (r: PreparedRow): RowOwner => (r.account === 'Shared' ? { ownerId: 'bo', readOnly: true } : me)
    const { byRow, names } = resolveNames(rows, ';', ownerOf, (id) => (id === 'bo' ? owner : empty()))
    expect(byRow.get(rows[0].fileRow)).toMatchObject({ category: 'food', payee: '' })
    expect(byRow.get(rows[1].fileRow)).toMatchObject({ category: 'food', payee: 'Shop' })
    expect(names.payee.map((n) => n.outcome)).toEqual(['new'])
  })

  it('a category new for both kinds of row says so; one existing for one kind names the other', () => {
    const rows = [row({ category: 'Gifts', amount: '-5' }), row({ category: 'gifts', amount: '5' }), row({ category: 'Food', amount: '-5' }), row({ category: 'Food', amount: '5' }), row({ category: 'Rent', amount: '-5' })]
    const lists = { ...empty(), categories: new Set([categoryKey('expense', 'Food')]) }
    const { names } = resolveNames(rows, ';', () => me, () => lists)
    expect(names.category.map((n) => [n.name, n.outcome, n.newFor])).toEqual([
      ['Gifts', 'new', 'both'],
      ['Food', 'new', 'income'],
      ['Rent', 'new', null],
    ])
  })

  it('counts only the rows that will upload', () => {
    const rows = [row({ payee: 'Cafe', status: 'skipped', reason: 'no_date' })]
    expect(resolveNames(rows, ';', () => me, empty).names.payee).toEqual([])
  })
})
