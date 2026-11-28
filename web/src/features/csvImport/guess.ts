import { detectDecimalSeparator } from './amounts'
import { detectDateOrder, localeDateOrder } from './dates'
import { fileCurrencyId, withAccountCurrencies } from './fileCurrency'
import type { ParsedFile } from './parse'
import type { AccountChoice, Role, StepId, WizardAnswers } from './prepare'

// The headers an Econumo export writes: fixed English names in every UI language.
const EXPORT_COLUMNS = {
  account: 'account_name',
  date: 'date',
  amount: 'amount',
  category: 'category',
  description: 'description',
  payee: 'payee',
  tag: 'tag',
  labels: 'labels',
  currency: 'account_currency',
} as const
const EXPORT_HEADER = [...Object.values(EXPORT_COLUMNS), 'transaction_id']

export function isEconumoExport(file: ParsedFile): boolean {
  return EXPORT_HEADER.every((column) => file.header.includes(column))
}

interface GuessContext {
  // with none, the import goes to a new account
  writableAccountIds: readonly string[]
  userCurrencyId: string
  locale: string
  // resolves an Econumo export's account_currency codes (unknown codes get userCurrencyId) and a
  // new account's currency from a file that names one
  currencies?: ReadonlyArray<{ id: string; code: string }>
}

export function columnValues(file: ParsedFile, column: number | null): string[] {
  if (column === null) return []
  return file.rows.map((row) => (row[column] ?? '').trim()).filter((v) => v !== '')
}

function exportAnswers(file: ParsedFile, ctx: GuessContext): WizardAnswers {
  const at = (name: string) => file.header.indexOf(name)
  const account = withAccountCurrencies(
    file,
    { kind: 'column', column: at(EXPORT_COLUMNS.account), currencyColumn: at(EXPORT_COLUMNS.currency), currencyId: ctx.userCurrencyId, overrides: {}, currencies: {} },
    ctx.currencies ?? [],
  )
  return {
    account,
    date: { column: at(EXPORT_COLUMNS.date), order: 'ymd' },
    amount: { mode: 'single', column: at(EXPORT_COLUMNS.amount), inColumn: null, outColumn: null, flip: false, sep: '.' },
    description: at(EXPORT_COLUMNS.description),
    extras: {
      payee: at(EXPORT_COLUMNS.payee),
      category: at(EXPORT_COLUMNS.category),
      tag: at(EXPORT_COLUMNS.tag),
      labels: at(EXPORT_COLUMNS.labels),
      labelsSeparator: ';',
    },
  }
}

// Only an Econumo export, whose header is ours, is mapped by the wizard itself. Any other file
// starts unanswered: a column given a role from its values alone was wrong too often, and the
// user then had to find and undo it. The AI's sure roles are the only other assignment.
export function initialAnswers(file: ParsedFile, ctx: GuessContext): WizardAnswers {
  if (isEconumoExport(file)) return exportAnswers(file, ctx)
  const account: AccountChoice | null =
    ctx.writableAccountIds.length === 0 ? { kind: 'new', name: '', currencyId: fileCurrencyId(file, ctx.currencies ?? []) ?? ctx.userCurrencyId } : null
  return {
    account,
    date: { column: null, order: localeDateOrder(ctx.locale) },
    amount: { mode: 'single', column: null, inColumn: null, outColumn: null, flip: false, sep: '.' },
    description: null,
    extras: { payee: null, category: null, tag: null, labels: null, labelsSeparator: ';' },
  }
}

export function redetectFormats(
  answers: WizardAnswers,
  file: ParsedFile,
  locale: string,
  changed: { date?: boolean; amount?: boolean },
): WizardAnswers {
  let next = answers
  if (changed.date) {
    const { column } = answers.date
    const order = column === null ? localeDateOrder(locale) : detectDateOrder(columnValues(file, column), locale).order
    next = { ...next, date: { ...next.date, order } }
  }
  if (changed.amount) {
    const { mode, column, inColumn, outColumn } = answers.amount
    const columns = mode === 'single' ? [column] : [inColumn, outColumn]
    const sep = detectDecimalSeparator(columns.flatMap((c) => columnValues(file, c))).sep
    next = { ...next, amount: { ...next.amount, sep } }
  }
  return next
}

interface Slot {
  step: StepId
  role: Role
  get: (a: WizardAnswers) => number | null
  set: (a: WizardAnswers, column: number | null) => void
}

const SLOTS: Slot[] = [
  { step: 'account', role: 'account', get: (a) => (a.account?.kind === 'column' ? a.account.column : null), set: () => {} },
  { step: 'account', role: 'currency', get: (a) => (a.account?.kind === 'column' ? a.account.currencyColumn : null), set: () => {} },
  { step: 'date', role: 'date', get: (a) => a.date.column, set: (a, c) => (a.date.column = c) },
  { step: 'amount', role: 'amount', get: (a) => (a.amount.mode === 'single' ? a.amount.column : null), set: (a, c) => (a.amount.column = c) },
  { step: 'amount', role: 'amountIn', get: (a) => (a.amount.mode === 'dual' ? a.amount.inColumn : null), set: (a, c) => (a.amount.inColumn = c) },
  { step: 'amount', role: 'amountOut', get: (a) => (a.amount.mode === 'dual' ? a.amount.outColumn : null), set: (a, c) => (a.amount.outColumn = c) },
  { step: 'description', role: 'description', get: (a) => a.description, set: (a, c) => (a.description = c) },
  { step: 'payee', role: 'payee', get: (a) => a.extras.payee, set: (a, c) => (a.extras.payee = c) },
  { step: 'category', role: 'category', get: (a) => a.extras.category, set: (a, c) => (a.extras.category = c) },
  { step: 'extras', role: 'tag', get: (a) => a.extras.tag, set: (a, c) => (a.extras.tag = c) },
  { step: 'extras', role: 'labels', get: (a) => a.extras.labels, set: (a, c) => (a.extras.labels = c) },
]

// The AI's 'account' and 'currency' roles are not applied: it cannot list the account names a
// per-row account needs currencies for.
export function applyAiGuess(
  answers: WizardAnswers,
  roles: Partial<Record<Role, string>>,
  file: ParsedFile,
  locale: string,
  touched: ReadonlySet<StepId>,
): WizardAnswers {
  const { header } = file
  const next: WizardAnswers = { ...answers, date: { ...answers.date }, amount: { ...answers.amount }, extras: { ...answers.extras } }
  const open = SLOTS.filter((slot) => !touched.has(slot.step) && slot.step !== 'account')
  const taken = new Set(
    SLOTS.filter((slot) => touched.has(slot.step) || slot.step === 'account')
      .map((slot) => slot.get(answers))
      .filter((c) => c !== null),
  )

  const picks = new Map<Role, number>()
  const pick = (role: Role) => {
    const name = roles[role]
    const index = name === undefined ? -1 : header.indexOf(name)
    if (index === -1 || taken.has(index)) return
    picks.set(role, index)
    taken.add(index)
  }
  for (const slot of open) {
    if (slot.role !== 'amountIn' && slot.role !== 'amountOut') pick(slot.role)
  }
  if (!touched.has('amount')) {
    pick('amountIn')
    pick('amountOut')
    if (picks.has('amountIn') && picks.has('amountOut')) {
      picks.delete('amount')
      next.amount.mode = 'dual'
    } else {
      picks.delete('amountIn')
      picks.delete('amountOut')
      if (picks.has('amount')) next.amount.mode = 'single'
    }
  }

  const assigned = new Set(picks.values())
  for (const slot of open) {
    const picked = picks.get(slot.role)
    if (picked !== undefined) slot.set(next, picked)
    else if (assigned.has(slot.get(next) ?? -1)) slot.set(next, null)
  }

  const a = answers.amount
  const b = next.amount
  return redetectFormats(next, file, locale, {
    date: next.date.column !== answers.date.column,
    amount: a.mode !== b.mode || a.column !== b.column || a.inColumn !== b.inColumn || a.outColumn !== b.outColumn,
  })
}
