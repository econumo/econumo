import type { TFunction } from 'i18next'
import type { Role, StepId, WizardAnswers } from './prepare'

const ROLE_KEYS: Record<Role, string> = {
  account: 'transactions.import_wizard.steps.account',
  date: 'transactions.import_wizard.steps.date',
  amount: 'transactions.import_wizard.steps.amount',
  amountIn: 'transactions.import_wizard.amount.money_in',
  amountOut: 'transactions.import_wizard.amount.money_out',
  description: 'transactions.import_wizard.steps.description',
  category: 'transactions.import_wizard.extras.role.category',
  payee: 'transactions.import_wizard.extras.role.payee',
  tag: 'transactions.import_wizard.extras.role.tag',
  labels: 'transactions.import_wizard.extras.role.labels',
}

export function columnRoles(answers: WizardAnswers): [Role, number][] {
  const { account, date, amount, description, extras } = answers
  const roles: [Role, number | null][] = [
    ['account', account?.kind === 'column' ? account.column : null],
    ['date', date.column],
    ...(amount.mode === 'single'
      ? ([['amount', amount.column]] as [Role, number | null][])
      : ([
          ['amountIn', amount.inColumn],
          ['amountOut', amount.outColumn],
        ] as [Role, number | null][])),
    ['description', description],
    ['category', extras.category],
    ['payee', extras.payee],
    ['tag', extras.tag],
    ['labels', extras.labels],
  ]
  return roles.filter((entry): entry is [Role, number] => entry[1] !== null)
}

const ROLE_STEP: Record<Role, StepId> = {
  account: 'account',
  date: 'date',
  amount: 'amount',
  amountIn: 'amount',
  amountOut: 'amount',
  description: 'description',
  category: 'extras',
  payee: 'extras',
  tag: 'extras',
  labels: 'extras',
}

export interface CollapsedColumn {
  label: string
  step: StepId
}

/** how a step's table marks columns: its own other roles as labels, other steps' columns collapsed to a link back */
export function columnMarks(
  answers: WizardAnswers,
  t: TFunction,
  step: StepId,
  omit: Role[] = [],
): { labels: Record<number, string>; collapsed: Record<number, CollapsedColumn> } {
  const labels: Record<number, string> = {}
  const collapsed: Record<number, CollapsedColumn> = {}
  for (const [role, column] of columnRoles(answers)) {
    if (omit.includes(role)) continue
    const text = t(ROLE_KEYS[role])
    if (ROLE_STEP[role] === step) labels[column] = labels[column] ? `${labels[column]}, ${text}` : text
    else if (collapsed[column]) collapsed[column] = { ...collapsed[column], label: `${collapsed[column].label}, ${text}` }
    else collapsed[column] = { label: text, step: ROLE_STEP[role] }
  }
  return { labels, collapsed }
}
