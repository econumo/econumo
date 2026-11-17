import type { AccountDto } from '@/api/dto/account'
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
}

// accounts I can write transactions into: mine, or shared to me above view-only
export function writableAccounts(accounts: AccountDto[], meId: string | undefined): AccountDto[] {
  return accounts.filter((account) => {
    if (account.owner.id === meId) return true
    const grant = account.sharedAccess.find((a) => a.user.id === meId)
    return grant !== undefined && grant.role !== 'guest'
  })
}

function usedColumns(answers: WizardAnswers): Set<number> {
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

const SKIP_STEP: Partial<Record<NonNullable<PreparedRow['reason']>, StepId>> = {
  no_account: 'account',
  no_date: 'date',
  no_amount: 'amount',
}

export function stepWarnings(answers: WizardAnswers, rows: PreparedRow[]): Set<StepId> {
  const warned = new Set<StepId>((['account', 'date', 'amount'] as const).filter((step) => missingAnswer(step, answers)))
  for (const row of rows) {
    const step = row.reason ? SKIP_STEP[row.reason] : undefined
    if (step) warned.add(step)
  }
  return warned
}

export function visibleSteps(answers: WizardAnswers | null, profiles: ColumnProfile[]): StepId[] {
  if (answers === null) return STEP_ORDER
  const hasExtras = unusedColumns(answers, profiles).length > 0
  return STEP_ORDER.filter((step) => step !== 'extras' || hasExtras)
}

export function defaultAccountName(fileName: string): string {
  return fileName.replace(/\.[^.]+$/, '') || fileName
}
