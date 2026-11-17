import type { TFunction } from 'i18next'
import type { Role, WizardAnswers } from './prepare'

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

/** what each column is used for, leaving out the roles the current step asks about itself */
export function columnLabels(answers: WizardAnswers, t: TFunction, omit: Role[] = []): Record<number, string> {
  const labels: Record<number, string> = {}
  for (const [role, column] of columnRoles(answers)) {
    if (omit.includes(role)) continue
    const text = t(ROLE_KEYS[role])
    labels[column] = labels[column] ? `${labels[column]}, ${text}` : text
  }
  return labels
}
