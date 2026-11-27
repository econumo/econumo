import type { PreparedRow, WizardAnswers } from './prepare'
import type { AccountDto } from '@/api/dto/account'
import type { ColumnProfile } from './columns'
import { firstDistinct, hasGuess, missingAnswer, newAccountFolderId, pickDetail, releaseTakenExtras, unusedColumns, visibleSteps, writableAccounts } from './wizard'

const answers = (patch: Partial<WizardAnswers>): WizardAnswers => ({
  account: { kind: 'existing', accountId: 'a1' },
  date: { column: 0, order: 'ymd' },
  amount: { mode: 'single', column: 1, inColumn: null, outColumn: null, flip: false, sep: '.' },
  description: 2,
  extras: { payee: null, category: null, tag: null, labels: null, labelsSeparator: ';' },
  ...patch,
})

it('an extra detail on a column a main step reads is dropped; others stay', () => {
  const before = answers({ extras: { payee: 2, category: 3, tag: 1, labels: null, labelsSeparator: ';' } })
  expect(releaseTakenExtras(before).extras).toEqual({ payee: null, category: 3, tag: null, labels: null, labelsSeparator: ';' })
})

it('answers with nothing to release come back as they were', () => {
  const before = answers({ extras: { payee: 3, category: null, tag: null, labels: null, labelsSeparator: ';' } })
  expect(releaseTakenExtras(before)).toBe(before)
})

it('a per-row account column is released from extras too', () => {
  const before = answers({ account: { kind: 'column', column: 4, currencies: {} }, extras: { payee: null, category: null, tag: null, labels: 4, labelsSeparator: ';' } })
  expect(releaseTakenExtras(before).extras.labels).toBeNull()
})

it('a new account goes to the first visible folder, a hidden one only when all are hidden', () => {
  expect(newAccountFolderId([{ id: 'h', name: 'H', position: 0, isVisible: 0 }, { id: 'v', name: 'V', position: 1, isVisible: 1 }])).toBe('v')
  expect(newAccountFolderId([{ id: 'b', name: 'B', position: 1, isVisible: 1 }, { id: 'a', name: 'A', position: 0, isVisible: 1 }])).toBe('a')
  expect(newAccountFolderId([{ id: 'h', name: 'H', position: 0, isVisible: 0 }])).toBe('h')
  expect(newAccountFolderId([])).toBeNull()
})

it('Extra details stays listed while it is open, even with no free column left', () => {
  const profiles = [0, 1, 2].map((index) => ({ index, name: `c${index}`, type: 'text' as const, negatives: false, samples: [], internal: false, filled: 1, distinct: 1, averageLength: 1, codes: false }))
  expect(visibleSteps(answers({}), profiles)).not.toContain('extras')
  expect(visibleSteps(answers({}), profiles, 'extras')).toEqual(['file', 'account', 'date', 'amount', 'category', 'description', 'payee', 'extras', 'preview'])
})

it('offers the accounts the import accepts: mine, or shared with an accepted admin or user grant', () => {
  const account = (id: string, owner: string, access: { role: 'admin' | 'user' | 'guest'; isAccepted: 0 | 1 }[] = []) =>
    ({ id, owner: { id: owner }, sharedAccess: access.map((a) => ({ user: { id: 'me' }, ...a })) }) as unknown as AccountDto
  const accounts = [
    account('mine', 'me'),
    account('user', 'o', [{ role: 'user', isAccepted: 1 }]),
    account('admin', 'o', [{ role: 'admin', isAccepted: 1 }]),
    account('guest', 'o', [{ role: 'guest', isAccepted: 1 }]),
    account('pending', 'o', [{ role: 'user', isAccepted: 0 }]),
    account('other', 'o'),
  ]
  expect(writableAccounts(accounts, 'me').map((a) => a.id)).toEqual(['mine', 'user', 'admin'])
})

const row = (patch: Partial<PreparedRow>): PreparedRow => ({
  fileRow: 2, raw: '', status: 'ok', reason: null, date: '2026-01-02', time: null, amount: '-5', account: null,
  description: 'Coffee', payee: '', category: '', tag: '', labels: '', ...patch,
})

it('a step shows its answer first only when the answer reads real values', () => {
  const rows = [row({})]
  expect(hasGuess('date', { answers: answers({}), rows, profiles: [], touched: new Set() })).toBe(true)
  expect(hasGuess('date', { answers: answers({ date: { column: null, order: 'ymd' } }), rows, profiles: [], touched: new Set() })).toBe(false)
  expect(hasGuess('date', { answers: answers({}), rows: [row({ date: null, status: 'skipped', reason: 'no_date' })], profiles: [], touched: new Set() })).toBe(false)
  expect(hasGuess('description', { answers: answers({ description: null }), rows, profiles: [], touched: new Set() })).toBe(false)
  expect(hasGuess('account', { answers: answers({}), rows, profiles: [], touched: new Set() })).toBe(false)
  expect(hasGuess('account', { answers: answers({ account: { kind: 'column', column: 3, currencies: {} } }), rows: [row({ account: 'Cash' })], profiles: [], touched: new Set() })).toBe(true)
  expect(hasGuess('extras', { answers: answers({}), rows, profiles: [], touched: new Set() })).toBe(false)
  expect(hasGuess('extras', { answers: answers({ extras: { payee: 3, category: null, tag: null, labels: null, labelsSeparator: ';' } }), rows, profiles: [], touched: new Set() })).toBe(false)
  expect(hasGuess('extras', { answers: answers({ extras: { payee: null, category: null, tag: 3, labels: null, labelsSeparator: ';' } }), rows, profiles: [], touched: new Set() })).toBe(true)
})

it('firstDistinct takes the first few different values and says whether there are more', () => {
  expect(firstDistinct(['a', '', 'a', 'b', 'c'], (v) => v)).toEqual({ values: ['a', 'b', 'c'], more: false })
  expect(firstDistinct(['a', 'b', 'c', 'd'], (v) => v)).toEqual({ values: ['a', 'b', 'c'], more: true })
})

const text = (index: number, over: Partial<ColumnProfile> = {}): ColumnProfile => ({
  index, name: `c${index}`, type: 'text', negatives: false, samples: ['x'], internal: false, filled: 9, distinct: 2, averageLength: 6, codes: false, ...over,
})

it('Category and Payee answer "none" when nothing in the file fits or None was picked, and ask while something fits', () => {
  const rows = [row({})]
  const fitting = [text(0), text(1), text(2), text(3)]
  expect(hasGuess('category', { answers: answers({}), rows, profiles: [text(0), text(1), text(2)], touched: new Set() })).toBe(true)
  expect(hasGuess('category', { answers: answers({}), rows, profiles: fitting, touched: new Set() })).toBe(false)
  expect(hasGuess('category', { answers: answers({}), rows, profiles: fitting, touched: new Set(['category']) })).toBe(true)
  expect(hasGuess('payee', { answers: answers({}), rows, profiles: fitting, touched: new Set() })).toBe(false)
  const withCategory = answers({ extras: { payee: null, category: 3, tag: null, labels: null, labelsSeparator: ';' } })
  expect(hasGuess('category', { answers: withCategory, rows: [row({ category: 'Food' })], profiles: fitting, touched: new Set() })).toBe(true)
  expect(hasGuess('category', { answers: withCategory, rows: [row({ category: '' })], profiles: fitting, touched: new Set() })).toBe(false)
})

it('a Category or Payee pick takes its column from Description and the other details: the last pick wins', () => {
  const before = answers({ extras: { payee: 3, category: null, tag: 4, labels: null, labelsSeparator: ';' } })
  expect(pickDetail(before, 'category', 3)).toMatchObject({ description: 2, extras: { payee: null, category: 3, tag: 4, labels: null, labelsSeparator: ';' } })
  expect(pickDetail(before, 'payee', 2)).toMatchObject({ description: null, extras: { payee: 2, category: null, tag: 4, labels: null, labelsSeparator: ';' } })
  expect(pickDetail(before, 'payee', null)).toEqual({ extras: { payee: null, category: null, tag: 4, labels: null, labelsSeparator: ';' } })
  const picked = { ...before, ...pickDetail(before, 'payee', 2) }
  expect(releaseTakenExtras(picked)).toBe(picked)
})

it('one column, one role: a Category pick takes the Amount, Date or per-row Account column, and that step needs an answer again', () => {
  const before = answers({ account: { kind: 'column', column: 5, currencies: {} } })
  const amount = { ...before, ...pickDetail(before, 'category', 1) }
  expect(amount.amount.column).toBeNull()
  expect(amount.extras.category).toBe(1)
  expect(missingAnswer('amount', amount)).toBe(true)
  expect(releaseTakenExtras(amount)).toBe(amount)
  const date = { ...before, ...pickDetail(before, 'payee', 0) }
  expect(date.date.column).toBeNull()
  expect(missingAnswer('date', date)).toBe(true)
  const account = { ...before, ...pickDetail(before, 'category', 5) }
  expect(account.account).toBeNull()
  expect(missingAnswer('account', account)).toBe(true)
  const dual = answers({ amount: { mode: 'dual', column: null, inColumn: 1, outColumn: 6, flip: false, sep: '.' } })
  expect(pickDetail(dual, 'payee', 6).amount).toMatchObject({ inColumn: 1, outColumn: null })
})

it('a guessed category or payee on an Account, Date, Amount or Description column is released', () => {
  const before = answers({ extras: { payee: 1, category: 0, tag: null, labels: null, labelsSeparator: ';' } })
  expect(releaseTakenExtras(before).extras).toMatchObject({ payee: null, category: null })
})

it('Extra details offers neither the category nor the payee column, and a tag on either is released', () => {
  const profiles = [0, 1, 2, 3, 4, 5].map((i) => text(i))
  const chosen = answers({ extras: { payee: 3, category: 4, tag: 3, labels: 4, labelsSeparator: ';' } })
  expect(unusedColumns(chosen, profiles).map((p) => p.index)).toEqual([5])
  expect(releaseTakenExtras(chosen).extras).toEqual({ payee: 3, category: 4, tag: null, labels: null, labelsSeparator: ';' })
})
