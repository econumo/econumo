import { add, abs, isNegative, toFixedString } from '@/lib/decimal'
import { signedAmount, type DecimalSeparator } from './amounts'
import { parseDateCell, type DateOrder, type ParsedDate } from './dates'
import type { ParsedFile } from './parse'

export type Role =
  | 'date'
  | 'amount'
  | 'amountIn'
  | 'amountOut'
  | 'description'
  | 'payee'
  | 'category'
  | 'tag'
  | 'labels'
  | 'account'

export type AccountChoice =
  | { kind: 'new'; name: string; currencyId: string }
  | { kind: 'existing'; accountId: string }
  | { kind: 'column'; column: number; currencies: Record<string, string> /* lowercased new name -> currencyId */ }

export interface WizardAnswers {
  account: AccountChoice | null
  date: { column: number | null; order: DateOrder }
  amount: {
    mode: 'single' | 'dual'
    column: number | null
    inColumn: number | null
    outColumn: number | null
    flip: boolean
    sep: DecimalSeparator
  }
  description: number | null
  extras: { payee: number | null; category: number | null; tag: number | null; labels: number | null; labelsSeparator: string }
}

export type RowFix = { date?: string; amount?: string } | 'skip'

export type StepId = 'file' | 'account' | 'date' | 'amount' | 'description' | 'extras' | 'preview'

export interface PreparedRow {
  // the row number counting the header as row 1 (first data row = 2), as the server numbers rows
  fileRow: number
  raw: string
  status: 'ok' | 'skipped'
  reason: 'no_date' | 'no_amount' | 'no_account' | 'user_skipped' | null
  date: string | null
  time: string | null
  amount: string | null
  account: string | null
  description: string
  payee: string
  category: string
  tag: string
  labels: string
}

const PLAIN_DECIMAL = /^-?\d+(\.\d+)?$/

function cell(row: string[], column: number | null): string {
  return column === null ? '' : (row[column] ?? '').trim()
}

// A typed amount is what the user wants stored: plain decimal, its own sign,
// never flipped or read with the file's separator.
function fixedAmount(value: string | undefined): string | null {
  const s = value?.trim() ?? ''
  return PLAIN_DECIMAL.test(s) ? s : null
}

function fixedDate(value: string | undefined): ParsedDate | null {
  return value ? parseDateCell(value, 'ymd') : null
}

// What the row's own cells give under the current answers, before any typed fix.
function readRow(row: string[], answers: WizardAnswers): { date: ParsedDate | null; amount: string | null; account: string | null } {
  const { amount: am, account } = answers
  const date = answers.date.column === null ? null : parseDateCell(cell(row, answers.date.column), answers.date.order)
  let amount: string | null = null
  if (am.mode === 'single' ? am.column !== null : am.inColumn !== null || am.outColumn !== null) {
    amount = signedAmount(
      { single: cell(row, am.column), inflow: cell(row, am.inColumn), outflow: cell(row, am.outColumn) },
      am.mode,
      am.flip,
      am.sep,
    )
  }
  const accountName = account?.kind === 'column' ? cell(row, account.column) || null : null
  return { date, amount, account: accountName }
}

export function prepareRows(file: ParsedFile, answers: WizardAnswers, fixes: ReadonlyMap<number, RowFix>): PreparedRow[] {
  const { account } = answers
  const result: PreparedRow[] = []

  file.rows.forEach((row, i) => {
    if (row.every((c) => c.trim() === '')) return
    const fileRow = i + 2
    const fix = fixes.get(fileRow)
    const typed = fix === 'skip' ? undefined : fix

    const own = readRow(row, answers)
    const date = own.date ?? fixedDate(typed?.date)
    const amount = own.amount ?? fixedAmount(typed?.amount)
    const accountName = own.account

    let reason: PreparedRow['reason'] = null
    if (date === null) reason = 'no_date'
    else if (amount === null) reason = 'no_amount'
    else if (account?.kind === 'column' && accountName === null) reason = 'no_account'
    if (reason !== null && fix === 'skip') reason = 'user_skipped'

    result.push({
      fileRow,
      raw: file.rawLines[i] ?? '',
      status: reason === null ? 'ok' : 'skipped',
      reason,
      date: date?.date ?? null,
      time: date?.time ?? null,
      amount,
      account: accountName,
      description: cell(row, answers.description),
      payee: cell(row, answers.extras.payee),
      category: cell(row, answers.extras.category),
      tag: cell(row, answers.extras.tag),
      labels: cell(row, answers.extras.labels),
    })
  })
  return result
}

/** The fixes without what the rows now read on their own: a typed value the file's cell
    replaced, or a skip of a row that is readable again. Dropped rather than ignored, so a
    later answer change that breaks the row again does not bring the stale value back.
    Returns `fixes` itself when nothing is stale. */
export function pruneFixes(file: ParsedFile, answers: WizardAnswers, fixes: ReadonlyMap<number, RowFix>): ReadonlyMap<number, RowFix> {
  let pruned: Map<number, RowFix> | null = null
  for (const [fileRow, fix] of fixes) {
    const row = file.rows[fileRow - 2]
    if (!row) continue
    const own = readRow(row, answers)
    let kept: RowFix | null = fix
    if (fix === 'skip') {
      if (own.date !== null && own.amount !== null && (answers.account?.kind !== 'column' || own.account !== null)) kept = null
    } else {
      const next = { ...fix }
      if (own.date !== null) delete next.date
      if (own.amount !== null) delete next.amount
      if (next.date === undefined && next.amount === undefined) kept = null
      else if (next.date !== fix.date || next.amount !== fix.amount) kept = next
    }
    if (kept === fix) continue
    pruned ??= new Map(fixes)
    if (kept === null) pruned.delete(fileRow)
    else pruned.set(fileRow, kept)
  }
  return pruned ?? fixes
}

const fractionDigits = (amount: string) => amount.split('.')[1]?.length ?? 0

export function summarize(rows: PreparedRow[]): {
  total: number
  skipped: number
  byReason: Record<string, number>
  firstDate: string | null
  lastDate: string | null
  moneyIn: string
  moneyOut: string
} {
  const byReason: Record<string, number> = {}
  let skipped = 0
  let firstDate: string | null = null
  let lastDate: string | null = null
  let moneyIn = '0'
  let moneyOut = '0'
  let digits = 0

  for (const row of rows) {
    if (row.status === 'skipped') {
      skipped++
      if (row.reason) byReason[row.reason] = (byReason[row.reason] ?? 0) + 1
      continue
    }
    if (row.date !== null) {
      if (firstDate === null || row.date < firstDate) firstDate = row.date
      if (lastDate === null || row.date > lastDate) lastDate = row.date
    }
    if (row.amount === null) continue
    digits = Math.max(digits, fractionDigits(row.amount))
    if (isNegative(row.amount)) moneyOut = add(moneyOut, abs(row.amount))
    else moneyIn = add(moneyIn, row.amount)
  }

  return {
    total: rows.length,
    skipped,
    byReason,
    firstDate,
    lastDate,
    moneyIn: toFixedString(moneyIn, digits),
    moneyOut: toFixedString(moneyOut, digits),
  }
}

type UploadColumn = 'date' | 'amount' | 'account' | 'description' | 'payee' | 'category' | 'tag' | 'labels'

export function buildUpload(
  rows: PreparedRow[],
  answers: WizardAnswers,
): { header: string[]; rows: string[][]; fileRows: number[]; mapping: Record<string, string | null>; fields: Record<string, string> } {
  const { extras } = answers
  const mapped: Record<UploadColumn, boolean> = {
    date: true,
    amount: true,
    account: answers.account?.kind === 'column',
    description: answers.description !== null,
    payee: extras.payee !== null,
    category: extras.category !== null,
    tag: extras.tag !== null,
    labels: extras.labels !== null,
  }
  const header = (Object.keys(mapped) as UploadColumn[]).filter((column) => mapped[column])
  const ok = rows.filter((row) => row.status === 'ok')

  const value = (row: PreparedRow, column: UploadColumn): string => {
    if (column === 'date') return row.time ? `${row.date} ${row.time}` : (row.date ?? '')
    return row[column] ?? ''
  }

  const mappedOrNull = (column: UploadColumn) => (mapped[column] ? column : null)
  const mapping: Record<string, string | null> = {
    account: mappedOrNull('account'),
    date: 'date',
    amount: 'amount',
    amountInflow: null,
    amountOutflow: null,
    category: mappedOrNull('category'),
    description: mappedOrNull('description'),
    payee: mappedOrNull('payee'),
    tag: mappedOrNull('tag'),
    labels: mappedOrNull('labels'),
  }

  const fields: Record<string, string> = {}
  if (answers.account?.kind === 'existing') fields.accountId = answers.account.accountId
  fields.labelsSeparator = extras.labelsSeparator || ';'

  return {
    header,
    rows: ok.map((row) => header.map((column) => value(row, column))),
    fileRows: ok.map((row) => row.fileRow),
    mapping,
    fields,
  }
}
