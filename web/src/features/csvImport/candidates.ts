import { columnRoles } from './columnLabels'
import type { ColumnProfile } from './columns'
import type { Role, StepId, WizardAnswers } from './prepare'

const STEP_OF: Record<Role, StepId> = {
  account: 'account',
  date: 'date',
  amount: 'amount',
  amountIn: 'amount',
  amountOut: 'amount',
  description: 'description',
  category: 'category',
  payee: 'payee',
  tag: 'extras',
  labels: 'extras',
}

// An account or category name is short and the same on many rows; a sentence or a value on
// nearly every row on its own is something else. A small file repeats little, so up to three
// names pass.
const NAME_MAX_LENGTH = 40
const NAME_MIN_DISTINCT = 3
const NAME_ROWS_PER_NAME = 3

const words = (p: ColumnProfile) => p.type === 'text' && !p.internal && !p.codes
const repeatedNames = (p: ColumnProfile) =>
  words(p) && p.averageLength <= NAME_MAX_LENGTH && p.distinct <= Math.max(NAME_MIN_DISTINCT, Math.floor(p.filled / NAME_ROWS_PER_NAME))

const fits: Record<Role, (p: ColumnProfile) => boolean> = {
  account: repeatedNames,
  date: (p) => p.type === 'date',
  amount: (p) => p.type === 'number' && !p.internal,
  amountIn: (p) => p.type === 'number' && !p.internal,
  amountOut: (p) => p.type === 'number' && !p.internal,
  description: words,
  category: repeatedNames,
  payee: words,
  tag: words,
  labels: words,
}

const rank: Partial<Record<Role, (a: ColumnProfile, b: ColumnProfile) => number>> = {
  account: (a, b) => a.distinct - b.distinct,
  category: (a, b) => a.distinct - b.distinct,
  description: (a, b) => b.averageLength - a.averageLength,
}

/** The parts of the file that could answer `role`, most likely first. A part another step
    already reads is not one of them: it stays reachable behind "Show all", with its tag. */
export function candidates(role: Role, profiles: ColumnProfile[], answers: WizardAnswers): number[] {
  const step = STEP_OF[role]
  const elsewhere = new Set(columnRoles(answers).filter(([r]) => STEP_OF[r] !== step).map(([, column]) => column))
  const found = profiles.filter((p) => !elsewhere.has(p.index) && fits[role](p))
  const order = rank[role]
  return (order ? found.sort((a, b) => order(a, b) || a.index - b.index) : found).map((p) => p.index)
}
