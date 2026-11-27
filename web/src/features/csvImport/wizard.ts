import type { AccountDto } from '@/api/dto/account'
import type { FolderDto } from '@/api/dto/folder'
import { candidates } from './candidates'
import { columnRoles } from './columnLabels'
import type { ColumnProfile } from './columns'
import type { ParsedFile } from './parse'
import type { PreparedRow, Role, RowFix, StepId, WizardAnswers } from './prepare'

export const STEP_ORDER: StepId[] = ['file', 'account', 'date', 'amount', 'category', 'description', 'payee', 'extras', 'preview']

export interface LoadedWizardState {
  file: ParsedFile
  fileName: string
  profiles: ColumnProfile[]
  answers: WizardAnswers
  touched: ReadonlySet<StepId>
  fixes: ReadonlyMap<number, RowFix>
  rows: PreparedRow[]
  isExport: boolean
}

export interface StepBodyProps {
  state: LoadedWizardState
  /** merges `patch` into the answers and marks `step` as answered by the user */
  setAnswers: (step: StepId, patch: Partial<WizardAnswers>) => void
  setFixes: (update: (prev: ReadonlyMap<number, RowFix>) => ReadonlyMap<number, RowFix>) => void
  goTo: (step: StepId) => void
  /** the step's list of choices was opened with Change */
  choosing: boolean
  /** opens the step's list of choices; it answers nothing, so the step stays open to later guesses */
  change: () => void
  /** a pick was made: back to the answer */
  closeChoices: () => void
}

// accounts I can write transactions into, as the import checks it: mine, or shared to me with
// an accepted admin or user grant (a pending invitation rides the account list too)
export function writableAccounts(accounts: AccountDto[], meId: string | undefined): AccountDto[] {
  return accounts.filter((account) => {
    if (account.owner.id === meId) return true
    const grant = account.sharedAccess.find((a) => a.user.id === meId)
    return grant !== undefined && grant.isAccepted === 1 && grant.role !== 'guest'
  })
}

/** the user's accounts by lowercased name, with their currency code */
export function existingCodes(accounts: AccountDto[]): Map<string, string> {
  return new Map(accounts.map((a) => [nameKey(a.name), a.currency.code]))
}

/** columns the Account (its currency too), Date, Amount and Description steps read */
function mainColumns(answers: WizardAnswers): Set<number> {
  const { account, date, amount, description } = answers
  const used = [
    account?.kind === 'column' ? account.column : null,
    account?.kind === 'column' ? account.currencyColumn : null,
    date.column,
    ...(amount.mode === 'single' ? [amount.column] : [amount.inColumn, amount.outColumn]),
    description,
  ]
  return new Set(used.filter((c) => c !== null))
}

/** columns every step before Extra details reads */
export function usedColumns(answers: WizardAnswers): Set<number> {
  const used = mainColumns(answers)
  for (const column of [answers.extras.category, answers.extras.payee]) if (column !== null) used.add(column)
  return used
}

/** columns the steps before Extra details left free, the ones Extra details offers */
export function unusedColumns(answers: WizardAnswers, profiles: ColumnProfile[]): ColumnProfile[] {
  const used = usedColumns(answers)
  return profiles.filter((p) => !used.has(p.index) && p.type !== 'empty')
}

function clearRole(answers: WizardAnswers, role: Role): WizardAnswers {
  switch (role) {
    case 'account':
      return { ...answers, account: null }
    case 'currency':
      return answers.account?.kind === 'column' ? { ...answers, account: { ...answers.account, currencyColumn: null } } : answers
    case 'date':
      return { ...answers, date: { ...answers.date, column: null } }
    case 'amount':
      return { ...answers, amount: { ...answers.amount, column: null } }
    case 'amountIn':
      return { ...answers, amount: { ...answers.amount, inColumn: null } }
    case 'amountOut':
      return { ...answers, amount: { ...answers.amount, outColumn: null } }
    case 'description':
      return { ...answers, description: null }
    default:
      return { ...answers, extras: { ...answers.extras, [role]: null } }
  }
}

/** One column, one role: a column a pick gives a role is taken from every other role that read
    it, the last pick wins. A required step that loses its column asks again. */
export function claimPicked(prev: WizardAnswers, next: WizardAnswers): WizardAnswers {
  const before = columnRoles(prev)
  const picked = columnRoles(next).filter(([role, column]) => !before.some(([r, c]) => r === role && c === column))
  let result = next
  for (const [role, column] of picked) {
    for (const [other, at] of columnRoles(result)) if (other !== role && at === column) result = clearRole(result, other)
  }
  return result
}

export function missingAnswer(step: StepId, answers: WizardAnswers): boolean {
  switch (step) {
    case 'account':
      return answers.account === null || (answers.account.kind === 'new' && answers.account.name.trim() === '')
    case 'date':
      return answers.date.column === null
    case 'amount':
      return answers.amount.mode === 'single'
        ? answers.amount.column === null
        : answers.amount.inColumn === null && answers.amount.outColumn === null
    default:
      return false
  }
}

const ANSWER_EXAMPLES = 3

/** the first few different values `pick` reads from the rows, and whether there are more */
export function firstDistinct<T>(items: T[], pick: (item: T) => string | null, count = ANSWER_EXAMPLES): { values: string[]; more: boolean } {
  const found = new Set<string>()
  for (const item of items) {
    const value = pick(item)?.trim()
    if (!value || found.has(value)) continue
    if (found.size === count) return { values: [...found], more: true }
    found.add(value)
  }
  return { values: [...found], more: false }
}

/** The step has an answer worth showing as a sentence: a guess, or a choice, that reads real
    values. Without one the step opens on its list of choices. Category and Payee are optional:
    with nothing in the file that fits, or None picked, "none" is their answer. */
export function hasGuess(step: StepId, state: Pick<LoadedWizardState, 'answers' | 'rows' | 'profiles' | 'touched'>): boolean {
  const { answers, rows } = state
  switch (step) {
    case 'account':
      return answers.account?.kind === 'column' && rows.some((row) => row.account !== null)
    case 'date':
      return answers.date.column !== null && rows.some((row) => row.date !== null)
    case 'amount':
      return !missingAnswer('amount', answers) && rows.some((row) => row.amount !== null)
    case 'description':
      return answers.description !== null && rows.some((row) => row.description !== '')
    case 'category':
    case 'payee':
      if (answers.extras[step] !== null) return rows.some((row) => row[step] !== '')
      return state.touched.has(step) || candidates(step, state.profiles, answers).length === 0
    case 'extras':
      return answers.extras.tag !== null || answers.extras.labels !== null
    default:
      return false
  }
}

/** a column Account, Date, Amount or Description reads is never also a category or payee, and
    one any earlier step reads is never also a tag */
export function releaseTakenExtras(answers: WizardAnswers): WizardAnswers {
  const main = mainColumns(answers)
  const used = usedColumns(answers)
  const extras = { ...answers.extras }
  let changed = false
  for (const role of ['payee', 'category', 'tag', 'labels'] as const) {
    const column = extras[role]
    if (column === null) continue
    if (role === 'tag' || role === 'labels' ? used.has(column) : main.has(column)) {
      extras[role] = null
      changed = true
    }
  }
  return changed ? { ...answers, extras } : answers
}

// the folder the accounts list shows first; a hidden one only when every folder is hidden
export function newAccountFolderId(folders: FolderDto[] | undefined): string | null {
  if (!folders || folders.length === 0) return null
  const ordered = [...folders].sort((a, b) => a.position - b.position)
  return (ordered.find((f) => f.isVisible === 1) ?? ordered[0]).id
}

// The open step stays listed: Extra details losing its last free column while it is open
// (an AI reply landing) must not leave the wizard on a step that has no place in the list.
export function visibleSteps(answers: WizardAnswers | null, profiles: ColumnProfile[], open?: StepId): StepId[] {
  if (answers === null) return STEP_ORDER
  const hasExtras = unusedColumns(answers, profiles).length > 0
  return STEP_ORDER.filter((step) => step !== 'extras' || hasExtras || open === 'extras')
}

export function defaultAccountName(fileName: string): string {
  return fileName.replace(/\.[^.]+$/, '') || fileName
}

// names are matched trimmed and case-insensitively, as the server matches them
export const nameKey = (name: string) => name.trim().toLowerCase()

export function distinctNames(file: ParsedFile, column: number): string[] {
  const names = new Map<string, string>()
  for (const row of file.rows) {
    const name = (row[column] ?? '').trim()
    if (name !== '' && !names.has(nameKey(name))) names.set(nameKey(name), name)
  }
  return [...names.values()]
}
