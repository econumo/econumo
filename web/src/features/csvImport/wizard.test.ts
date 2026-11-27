import type { PreparedRow, WizardAnswers } from './prepare'
import type { AccountDto } from '@/api/dto/account'
import type { ColumnProfile } from './columns'
import { claimPicked, firstDistinct, hasGuess, missingAnswer, newAccountFolderId, releaseTakenExtras, unusedColumns, visibleSteps, writableAccounts } from './wizard'

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
  const before = answers({ account: { kind: 'column', column: 4, currencyColumn: null, currencyId: '', overrides: {}, currencies: {} }, extras: { payee: null, category: null, tag: null, labels: 4, labelsSeparator: ';' } })
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
  expect(hasGuess('account', { answers: answers({ account: { kind: 'column', column: 3, currencyColumn: null, currencyId: '', overrides: {}, currencies: {} } }), rows: [row({ account: 'Cash' })], profiles: [], touched: new Set() })).toBe(true)
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

const pick = (before: WizardAnswers, patch: Partial<WizardAnswers>) => claimPicked(before, { ...before, ...patch })
const withExtras = (before: WizardAnswers, patch: Partial<WizardAnswers['extras']>) => pick(before, { extras: { ...before.extras, ...patch } })

it('one column, one role: a Category or Payee pick takes its column from Description and the other details', () => {
  const before = answers({ extras: { payee: 3, category: null, tag: 4, labels: null, labelsSeparator: ';' } })
  expect(withExtras(before, { category: 3 })).toMatchObject({ description: 2, extras: { payee: null, category: 3, tag: 4 } })
  expect(withExtras(before, { payee: 2 })).toMatchObject({ description: null, extras: { payee: 2, category: null, tag: 4 } })
  expect(withExtras(before, { payee: null }).extras).toEqual({ payee: null, category: null, tag: 4, labels: null, labelsSeparator: ';' })
  const picked = withExtras(before, { payee: 2 })
  expect(releaseTakenExtras(picked)).toBe(picked)
})

it('one column, one role: a pick takes the Amount, Date or per-row Account column, and that step needs an answer again', () => {
  const before = answers({ account: { kind: 'column', column: 5, currencyColumn: null, currencyId: '', overrides: {}, currencies: {} } })
  const amount = withExtras(before, { category: 1 })
  expect(amount.amount.column).toBeNull()
  expect(amount.extras.category).toBe(1)
  expect(missingAnswer('amount', amount)).toBe(true)
  expect(releaseTakenExtras(amount)).toBe(amount)
  const date = withExtras(before, { payee: 0 })
  expect(date.date.column).toBeNull()
  expect(missingAnswer('date', date)).toBe(true)
  const account = withExtras(before, { tag: 5 })
  expect(account.account).toBeNull()
  expect(missingAnswer('account', account)).toBe(true)
  const dual = answers({ amount: { mode: 'dual', column: null, inColumn: 1, outColumn: 6, flip: false, sep: '.' } })
  expect(withExtras(dual, { payee: 6 }).amount).toMatchObject({ inColumn: 1, outColumn: null })
  expect(pick(dual, { amount: { ...dual.amount, outColumn: 1 } }).amount).toMatchObject({ inColumn: null, outColumn: 1 })
})

it('one column, one role: the currency column is a role of the Account step', () => {
  const perRow = { kind: 'column' as const, column: 5, currencyColumn: 3, currencyId: 'cur-usd', overrides: {}, currencies: {} }
  const before = answers({ account: perRow, extras: { payee: null, category: 3, tag: null, labels: null, labelsSeparator: ';' } })
  // a currency column is never also a category
  expect(releaseTakenExtras(before).extras.category).toBeNull()
  // a later pick takes it: the account stays, the file no longer says the currency
  const taken = withExtras(answers({ account: perRow }), { category: 3 })
  expect(taken.account).toMatchObject({ kind: 'column', column: 5, currencyColumn: null })
  expect(taken.extras.category).toBe(3)
  expect(missingAnswer('account', taken)).toBe(false)
})

it('one column, one role: Description takes the Date column, and Date takes the Description column', () => {
  const before = answers({})
  const described = pick(before, { description: 0 })
  expect(described).toMatchObject({ description: 0, date: { column: null } })
  expect(missingAnswer('date', described)).toBe(true)
  const dated = pick(before, { date: { ...before.date, column: 2 } })
  expect(dated).toMatchObject({ description: null, date: { column: 2 } })
  expect(missingAnswer('date', dated)).toBe(false)
})

it('a pick that changes nothing takes nothing', () => {
  const before = answers({ extras: { payee: 3, category: null, tag: null, labels: null, labelsSeparator: ';' } })
  expect(pick(before, { description: 2 })).toEqual(before)
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
