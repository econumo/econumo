import { detectDecimalSeparator, parseAmountCell } from './amounts'
import type { ColumnProfile } from './columns'
import { detectDateOrder, localeDateOrder, parseDateCell } from './dates'
import { currencyColumn, fileCurrencyId, withAccountCurrencies } from './fileCurrency'
import type { ParsedFile } from './parse'
import type { AccountChoice, Role, StepId, WizardAnswers } from './prepare'
import { nameKey } from './wizard'

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

type GuessedRole = 'account' | 'date' | 'amount' | 'amountIn' | 'amountOut' | 'description' | 'payee' | 'category'

export function isEconumoExport(file: ParsedFile): boolean {
  return EXPORT_HEADER.every((column) => file.header.includes(column))
}

interface GuessContext {
  writableAccountIds: readonly string[]
  // the names of those accounts: a column holding them is the file's account column
  writableAccountNames?: readonly string[]
  userCurrencyId: string
  locale: string
  // resolves an Econumo export's account_currency codes; unknown codes get userCurrencyId
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

// A name column (category, account) is short and repeats; a description or payee mostly differs
// from row to row.
const NAME_MAX_LENGTH = 40
const NAME_ROWS_PER_NAME = 3
const BALANCE_SHARE = 0.8
// a description differs on (nearly) every row; a payee or account repeats more
const DESCRIPTION_SHARE = 0.9

const isWords = (p: ColumnProfile) => p.type === 'text' && !p.internal && !p.codes
const mostlyDistinct = (p: ColumnProfile) => p.distinct >= 2 && p.distinct * 2 >= p.filled
const repeatsFew = (p: ColumnProfile) =>
  p.averageLength <= NAME_MAX_LENGTH && p.distinct >= 2 && p.distinct < p.filled && p.distinct <= Math.max(3, Math.floor(p.filled / NAME_ROWS_PER_NAME))

const parsedDates = (file: ParsedFile, column: number) =>
  columnValues(file, column).filter((v) => (['ymd', 'dmy', 'mdy'] as const).some((order) => parseDateCell(v, order) !== null)).length

function amountsOf(file: ParsedFile, column: number): (number | null)[] {
  const { sep } = detectDecimalSeparator(columnValues(file, column))
  return file.rows.map((row) => {
    const parsed = parseAmountCell((row[column] ?? '').trim(), sep)
    return parsed === null ? null : Number(parsed)
  })
}

// A running balance moves by each row's amount, whichever way the file is sorted.
function isBalanceOf(balance: (number | null)[], amount: (number | null)[]): boolean {
  let pairs = 0
  let forward = 0
  let backward = 0
  for (let i = 1; i < balance.length; i++) {
    const [before, after] = [balance[i - 1], balance[i]]
    if (before === null || after === null) continue
    pairs++
    if (amount[i] !== null && Math.abs(after - before - amount[i]!) < 0.005) forward++
    if (amount[i - 1] !== null && Math.abs(before - after - amount[i - 1]!) < 0.005) backward++
  }
  return pairs >= 2 && Math.max(forward, backward) >= BALANCE_SHARE * pairs
}

// Each row with money has it in exactly one of the two columns.
function splitsRows(a: (number | null)[], b: (number | null)[]): boolean {
  let inA = 0
  let inB = 0
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== null && b[i] !== null) return false
    if (a[i] !== null) inA++
    if (b[i] !== null) inB++
  }
  return inA > 0 && inB > 0
}

// Built from the values alone, never from header words: the file's headers can be in any
// language. The AI, when it answers, refines what the user has not touched yet.
function guessRoles(file: ParsedFile, profiles: ColumnProfile[], ctx: GuessContext): { found: Partial<Record<GuessedRole, number>>; dual: boolean } {
  const found: Partial<Record<GuessedRole, number>> = {}
  let dual = false

  const dates = profiles.filter((p) => p.type === 'date')
  if (dates.length > 0) {
    const counts = dates.map((p) => parsedDates(file, p.index))
    found.date = dates[counts.indexOf(Math.max(...counts))].index
  }

  const numbers = profiles.filter((p) => p.type === 'number' && !p.internal)
  const amounts = new Map(numbers.map((p) => [p.index, amountsOf(file, p.index)]))
  const balances = new Set(
    numbers.filter((b) => numbers.some((a) => a !== b && isBalanceOf(amounts.get(b.index)!, amounts.get(a.index)!))).map((p) => p.index),
  )
  const money = numbers.filter((p) => !balances.has(p.index))
  const pair = money.flatMap((a, i) => money.slice(i + 1).map((b) => [a, b] as const)).find(([a, b]) => splitsRows(amounts.get(a.index)!, amounts.get(b.index)!))
  if (pair && !money.some((p) => p.negatives && !pair.includes(p))) {
    const [a, b] = pair
    dual = true
    // Spending shows as negatives, or else outnumbers income; when neither tells, the step asks.
    const out = a.negatives !== b.negatives ? (a.negatives ? a : b) : a.filled !== b.filled ? (a.filled > b.filled ? a : b) : null
    if (out !== null) {
      found.amountOut = out.index
      found.amountIn = (out === a ? b : a).index
    }
  } else if (money.length > 0) {
    found.amount = (money.find((p) => p.negatives) ?? money[0]).index
  }

  const text = profiles.filter(isWords)
  const free = (p: ColumnProfile) => !Object.values(found).includes(p.index)
  const longest = (list: ColumnProfile[]) => list.reduce<ColumnProfile | undefined>((best, p) => (best === undefined || p.averageLength > best.averageLength ? p : best), undefined)
  // A column holding the names of the user's own accounts is the account column.
  const accountNames = new Set((ctx.writableAccountNames ?? []).map(nameKey))
  const accountHits = (p: ColumnProfile) => columnValues(file, p.index).filter((v) => accountNames.has(nameKey(v))).length
  const byName = text.filter(free).map((p) => ({ p, hits: accountHits(p) })).filter((c) => c.hits > 0).sort((a, b) => b.hits - a.hits || a.p.index - b.p.index)[0]
  if (byName) found.account = byName.p.index

  const varied = text.filter((p) => free(p) && mostlyDistinct(p))
  const mostDistinct = Math.max(0, ...varied.map((p) => p.distinct))
  const description = longest(varied.filter((p) => p.distinct >= DESCRIPTION_SHARE * mostDistinct))
  if (description) found.description = description.index

  // Of two repeating columns, the one with fewer names is taken for the account, which the
  // Account step asks about on its own.
  const named = text.filter((p) => free(p) && repeatsFew(p)).sort((a, b) => a.distinct - b.distinct || a.index - b.index)
  // With no account of the user's own to tell by, of two or more repeating columns the one with
  // the fewest names is the account (unless the user has exactly one, which the file is then
  // taken to be for); a lone one stays a category.
  if (found.account === undefined && ctx.writableAccountIds.length !== 1 && named.length >= 2) found.account = named.shift()!.index
  const category = found.account !== undefined || named.length < 2 ? named[0] : named[1]
  if (category) found.category = category.index

  const payee = text
    .filter((p) => free(p) && mostlyDistinct(p) && p.averageLength <= NAME_MAX_LENGTH)
    .sort((a, b) => b.distinct - a.distinct || a.index - b.index)[0]
  if (payee) found.payee = payee.index
  return { found, dual }
}

export function guessAnswers(file: ParsedFile, profiles: ColumnProfile[], ctx: GuessContext): WizardAnswers {
  if (isEconumoExport(file)) return exportAnswers(file, ctx)

  const { found: roles, dual } = guessRoles(file, profiles, ctx)
  const writable = ctx.writableAccountIds
  const taken = new Set(Object.values(roles))
  const account: AccountChoice | null =
    roles.account !== undefined
      ? withAccountCurrencies(
          file,
          {
            kind: 'column',
            column: roles.account,
            currencyColumn: currencyColumn(file, ctx.currencies ?? [], taken),
            currencyId: ctx.userCurrencyId,
            overrides: {},
            currencies: {},
          },
          ctx.currencies ?? [],
        )
      : writable.length === 0
      ? { kind: 'new', name: '', currencyId: fileCurrencyId(file, ctx.currencies ?? []) ?? ctx.userCurrencyId }
      : writable.length === 1
        ? { kind: 'existing', accountId: writable[0] }
        : null

  const answers: WizardAnswers = {
    account,
    date: { column: roles.date ?? null, order: 'ymd' },
    amount: {
      mode: dual ? 'dual' : 'single',
      column: roles.amount ?? null,
      inColumn: roles.amountIn ?? null,
      outColumn: roles.amountOut ?? null,
      flip: false,
      sep: '.',
    },
    description: roles.description ?? null,
    extras: {
      payee: roles.payee ?? null,
      category: roles.category ?? null,
      tag: null,
      labels: null,
      labelsSeparator: ';',
    },
  }
  return redetectFormats(answers, file, ctx.locale, { date: true, amount: true })
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
