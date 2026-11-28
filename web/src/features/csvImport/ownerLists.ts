import { useMemo } from 'react'
import type { CategoryType } from '@/api/dto/category'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import { isNegative } from '@/lib/decimal'
import { splitLabels, type PreparedRow, type WizardAnswers } from './prepare'
import { nameKey, writableAccounts } from './wizard'

// A category is found within its kind, as the server does: an income row (an amount that is
// not negative) naming the expense category "Food" gets an income "Food" of its own.
export const categoryKey = (type: CategoryType, name: string) => `${type}:${nameKey(name)}`
const rowKind = (row: PreparedRow): CategoryType => (row.amount !== null && isNegative(row.amount) ? 'expense' : 'income')

export type NameRole = 'category' | 'payee' | 'tag' | 'labels'

/** what the import does with a name: finds it, creates it, or leaves it out of the row
    (`left_out`: a shared account's owner lacks it and the importer may not add it;
    `taken`: a budget tag and a reporting tag cannot share a name) */
export type NameOutcome = 'existing' | 'new' | 'left_out' | 'taken'

/** one owner's names, by `nameKey` (categories by `categoryKey`), archived ones included:
    the server finds them and does not create a second one */
export interface OwnerNames {
  categories: Set<string>
  payees: Set<string>
  tags: Set<string>
  labels: Set<string>
}

/** whose lists a row's names resolve in: the owner of the account it lands in */
export interface RowOwner {
  ownerId: string
  /** a member with a user grant uses the owner's names but cannot add any */
  readOnly: boolean
}

export interface NameInFile {
  key: string
  /** the first spelling in the file */
  name: string
  /** transactions that name it */
  count: number
  outcome: NameOutcome
  /** a new category named by both kinds of row: the kind it is new for, or both */
  newFor: CategoryType | 'both' | null
}

/** what lands on the row once its names are resolved; a name left out is '' (or missing from labels) */
export interface ResolvedNames {
  category: string
  payee: string
  tag: string
  labels: string[]
}

export interface NameResolution {
  byRow: Map<number, ResolvedNames>
  names: Record<NameRole, NameInFile[]>
}

const PRIORITY: NameOutcome[] = ['new', 'existing', 'taken', 'left_out']

/**
 * The server's name resolution, replayed: rows top-down, each row's category, payee, budget
 * tag, then reporting tags, in the lists of the row's account owner, adding what it creates
 * as it goes. Only rows that will upload count: a name found only on a skipped row is never
 * created.
 */
export function resolveNames(
  rows: PreparedRow[],
  labelsSeparator: string,
  ownerOf: (row: PreparedRow) => RowOwner,
  listsOf: (ownerId: string) => OwnerNames,
): NameResolution {
  const working = new Map<string, OwnerNames>()
  const found: Record<NameRole, Map<string, { name: string; count: number; outcomes: Set<NameOutcome>; kinds: Set<CategoryType>; newKinds: Set<CategoryType> }>> = {
    category: new Map(),
    payee: new Map(),
    tag: new Map(),
    labels: new Map(),
  }
  const byRow = new Map<number, ResolvedNames>()

  for (const row of rows) {
    if (row.status !== 'ok') continue
    const { ownerId, readOnly } = ownerOf(row)
    let lists = working.get(ownerId)
    if (!lists) {
      const base = listsOf(ownerId)
      lists = { categories: new Set(base.categories), payees: new Set(base.payees), tags: new Set(base.tags), labels: new Set(base.labels) }
      working.set(ownerId, lists)
    }
    const kind = rowKind(row)
    const resolve = (role: NameRole, name: string, own: Set<string>, ownKey: string, other: Set<string> | null): boolean => {
      const key = nameKey(name)
      let outcome: NameOutcome
      if (own.has(ownKey)) outcome = 'existing'
      else if (other?.has(key)) outcome = 'taken'
      else if (readOnly) outcome = 'left_out'
      else {
        own.add(ownKey)
        outcome = 'new'
      }
      const entry = found[role].get(key) ?? { name: name.trim(), count: 0, outcomes: new Set(), kinds: new Set(), newKinds: new Set() }
      entry.count++
      entry.outcomes.add(outcome)
      entry.kinds.add(kind)
      if (outcome === 'new') entry.newKinds.add(kind)
      found[role].set(key, entry)
      return outcome === 'existing' || outcome === 'new'
    }

    const resolved: ResolvedNames = { category: '', payee: '', tag: '', labels: [] }
    if (nameKey(row.category) !== '' && resolve('category', row.category, lists.categories, categoryKey(kind, row.category), null)) resolved.category = row.category
    if (nameKey(row.payee) !== '' && resolve('payee', row.payee, lists.payees, nameKey(row.payee), null)) resolved.payee = row.payee
    if (nameKey(row.tag) !== '' && resolve('tag', row.tag, lists.tags, nameKey(row.tag), lists.labels)) resolved.tag = row.tag
    if (row.labels !== '') {
      for (const label of splitLabels(row.labels, labelsSeparator)) {
        if (resolve('labels', label, lists.labels, nameKey(label), lists.tags)) resolved.labels.push(label)
      }
    }
    byRow.set(row.fileRow, resolved)
  }

  // most used first, ties in file order
  const listOf = (role: NameRole): NameInFile[] =>
    [...found[role].entries()]
      .map(([key, { name, count, outcomes, kinds, newKinds }]) => {
        const outcome = PRIORITY.find((o) => outcomes.has(o)) ?? 'left_out'
        let newFor: NameInFile['newFor'] = null
        if (role === 'category' && outcome === 'new' && kinds.size === 2) newFor = newKinds.size === 2 ? 'both' : [...newKinds][0]
        return { key, name, count, outcome, newFor }
      })
      .sort((a, b) => b.count - a.count)

  return { byRow, names: { category: listOf('category'), payee: listOf('payee'), tag: listOf('tag'), labels: listOf('labels') } }
}

/** `resolveNames` under the current answers, with the lists the app already holds */
export function useNameResolution(rows: PreparedRow[], answers: WizardAnswers): NameResolution {
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const { account, extras } = answers
  const meId = user?.id ?? ''

  return useMemo(() => {
    const writable = writableAccounts(accounts, user?.id)
    const ownerOfAccount = (target: (typeof accounts)[number] | undefined): RowOwner =>
      target === undefined
        ? { ownerId: meId, readOnly: false }
        : {
            ownerId: target.owner.id,
            readOnly: target.owner.id !== meId && target.sharedAccess.find((a) => a.user.id === meId)?.role !== 'admin',
          }
    const fixed = account?.kind === 'existing' ? ownerOfAccount(accounts.find((a) => a.id === account.accountId)) : ownerOfAccount(undefined)
    const ownerOf = (row: PreparedRow): RowOwner => {
      if (account?.kind !== 'column' || row.account === null) return fixed
      const key = nameKey(row.account)
      return ownerOfAccount(writable.find((a) => nameKey(a.name) === key))
    }
    const listsOf = (ownerId: string): OwnerNames => ({
      categories: new Set(categories.filter((c) => c.ownerUserId === ownerId).map((c) => categoryKey(c.type, c.name))),
      payees: new Set(payees.filter((p) => p.ownerUserId === ownerId).map((p) => nameKey(p.name))),
      tags: new Set(tags.filter((t) => t.ownerUserId === ownerId).map((t) => nameKey(t.name))),
      labels: new Set(labels.filter((l) => l.ownerUserId === ownerId).map((l) => nameKey(l.name))),
    })
    return resolveNames(rows, extras.labelsSeparator, ownerOf, listsOf)
  }, [rows, account, extras.labelsSeparator, accounts, categories, payees, tags, labels, user?.id, meId])
}
