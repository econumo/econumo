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

/** a column's header, or "Column 3" when the file left it blank */
export function columnName(profile: { index: number; name: string }, t: TFunction): string {
  return profile.name.trim() || t('transactions.import_wizard.column_n', { n: profile.index + 1 })
}

export function roleLabel(role: Role, t: TFunction): string {
  return t(ROLE_KEYS[role])
}

// a role named inside a phrase ("now payee", "with payee, category"); German nouns keep their capital
export const inPhrase = (text: string, language: string) => (language.startsWith('de') ? text : text.toLocaleLowerCase(language))

/** the small "now payee" tag of each column another role already reads */
export function columnTags(answers: WizardAnswers, t: TFunction, language: string, omit: Role[] = []): Record<number, string> {
  const names: Record<number, string[]> = {}
  for (const [role, column] of columnRoles(answers)) {
    if (omit.includes(role)) continue
    ;(names[column] ??= []).push(inPhrase(roleLabel(role, t), language))
  }
  return Object.fromEntries(
    Object.entries(names).map(([column, roles]) => [column, t('transactions.import_wizard.now_used', { role: roles.join(', ') })]),
  )
}
