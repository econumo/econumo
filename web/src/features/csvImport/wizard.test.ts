import type { PreparedRow, WizardAnswers } from './prepare'
import type { AccountDto } from '@/api/dto/account'
import { firstDistinct, hasGuess, newAccountFolderId, releaseTakenExtras, visibleSteps, writableAccounts } from './wizard'

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
  const profiles = [0, 1, 2].map((index) => ({ index, name: `c${index}`, type: 'text' as const, negatives: false, samples: [], internal: false }))
  expect(visibleSteps(answers({}), profiles)).not.toContain('extras')
  expect(visibleSteps(answers({}), profiles, 'extras')).toEqual(['file', 'account', 'date', 'amount', 'description', 'extras', 'preview'])
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
  expect(hasGuess('date', { answers: answers({}), rows })).toBe(true)
  expect(hasGuess('date', { answers: answers({ date: { column: null, order: 'ymd' } }), rows })).toBe(false)
  expect(hasGuess('date', { answers: answers({}), rows: [row({ date: null, status: 'skipped', reason: 'no_date' })] })).toBe(false)
  expect(hasGuess('description', { answers: answers({ description: null }), rows })).toBe(false)
  expect(hasGuess('account', { answers: answers({}), rows })).toBe(false)
  expect(hasGuess('account', { answers: answers({ account: { kind: 'column', column: 3, currencies: {} } }), rows: [row({ account: 'Cash' })] })).toBe(true)
  expect(hasGuess('extras', { answers: answers({}), rows })).toBe(false)
  expect(hasGuess('extras', { answers: answers({ extras: { payee: 3, category: null, tag: null, labels: null, labelsSeparator: ';' } }), rows })).toBe(true)
})

it('firstDistinct takes the first few different values and says whether there are more', () => {
  expect(firstDistinct(['a', '', 'a', 'b', 'c'], (v) => v)).toEqual({ values: ['a', 'b', 'c'], more: false })
  expect(firstDistinct(['a', 'b', 'c', 'd'], (v) => v)).toEqual({ values: ['a', 'b', 'c'], more: true })
})
