import type { AccountDto } from '@/api/dto/account'
import type { FolderDto } from '@/api/dto/folder'
import type { ColumnProfile } from './columns'
import type { ParsedFile } from './parse'
import type { PreparedRow, RowFix, StepId, WizardAnswers } from './prepare'

export const STEP_ORDER: StepId[] = ['file', 'account', 'date', 'amount', 'description', 'extras', 'preview']

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
  /** the step's list of choices was opened ("Not right? Choose differently") */
  choosing: boolean
  /** opens the step's list of choices; it answers nothing, so the step stays open to later guesses */
  choose: () => void
  /** "Looks right": the next step, answering nothing */
  next: () => void
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

/** columns the Account to Description steps read */
export function usedColumns(answers: WizardAnswers): Set<number> {
  const { account, date, amount, description } = answers
  const used = [
    account?.kind === 'column' ? account.column : null,
    date.column,
    ...(amount.mode === 'single' ? [amount.column] : [amount.inColumn, amount.outColumn]),
    description,
  ]
  return new Set(used.filter((c) => c !== null))
}

/** columns the Account to Description steps left free, the ones Extra details offers */
export function unusedColumns(answers: WizardAnswers, profiles: ColumnProfile[]): ColumnProfile[] {
  const used = usedColumns(answers)
  return profiles.filter((p) => !used.has(p.index) && p.type !== 'empty')
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
    values. Without one the step opens on its list of choices. */
export function hasGuess(step: StepId, state: Pick<LoadedWizardState, 'answers' | 'rows'>): boolean {
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
    case 'extras':
      return (['payee', 'category', 'tag', 'labels'] as const).some((role) => answers.extras[role] !== null)
    default:
      return false
  }
}

/** a column one of the main steps reads is never also uploaded as an extra detail */
export function releaseTakenExtras(answers: WizardAnswers): WizardAnswers {
  const used = usedColumns(answers)
  const extras = { ...answers.extras }
  let changed = false
  for (const role of ['payee', 'category', 'tag', 'labels'] as const) {
    const column = extras[role]
    if (column !== null && used.has(column)) {
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
