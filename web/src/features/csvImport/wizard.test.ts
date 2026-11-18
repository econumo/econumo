import type { WizardAnswers } from './prepare'
import { newAccountFolderId, releaseTakenExtras } from './wizard'

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
