import type { TFunction } from 'i18next'
import { formatDay } from './accountView'
import { detectDecimalSeparator, parseAmountCell } from './amounts'
import { headerHasValues, type ColumnProfile } from './columns'
import { detectDateOrder, parseDateCell } from './dates'
import type { Role, WizardAnswers } from './prepare'

const ROLE_KEYS: Record<Role, string> = {
  account: 'transactions.import_wizard.steps.account',
  date: 'transactions.import_wizard.steps.date',
  amount: 'transactions.import_wizard.steps.amount',
  amountIn: 'transactions.import_wizard.amount.money_in',
  amountOut: 'transactions.import_wizard.amount.money_out',
  description: 'transactions.import_wizard.steps.description',
  category: 'transactions.import_wizard.steps.category',
  payee: 'transactions.import_wizard.steps.payee',
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

/** The file's own name for a part of it, or null when it has none worth showing: a blank
    header (parse.ts names it "Column 3" in English) or a first row that is a transaction. */
export function sourceName(profile: ColumnProfile, profiles: ColumnProfile[]): string | null {
  const name = profile.name.trim()
  if (name === '' || name === `Column ${profile.index + 1}` || headerHasValues(profiles)) return null
  return name
}

/** "from “Buchungstag”", or null when the file gave it no name */
export function fromName(profile: ColumnProfile | undefined, profiles: ColumnProfile[], t: TFunction): string | null {
  const name = profile ? sourceName(profile, profiles) : null
  return name === null ? null : t('transactions.import_wizard.from', { name })
}

// A sample as a person reads it: a date in the UI language, an amount with its decimals.
// A date whose day and month could swap, or a separator the samples do not settle, stays as written.
export function displaySamples(profile: ColumnProfile, language: string): string[] {
  const { samples, type } = profile
  if (type === 'date') {
    const { order, ambiguous } = detectDateOrder(samples, navigator.language)
    if (ambiguous) return samples
    return samples.map((v) => {
      const parsed = parseDateCell(v, order)
      return parsed ? formatDay(parsed.date, language) : v
    })
  }
  if (type === 'number' && !profile.internal) {
    const { sep, ambiguous } = detectDecimalSeparator(samples)
    if (ambiguous) return samples
    const format = new Intl.NumberFormat(language, { minimumFractionDigits: 2, maximumFractionDigits: 20 })
    return samples.map((v) => {
      const parsed = parseAmountCell(v, sep)
      return parsed === null ? v : format.format(Number(parsed))
    })
  }
  return samples.map(splitList)
}

const HASHTAGS = /^#?[^\s,;#]+(?:\s*,\s*#[^\s,;#]+)+$/

// labels are kept as one cell joined by ";" (or hashtags by commas); shown apart, uploaded untouched
function splitList(value: string): string {
  if (value.includes(';')) {
    const parts = value.split(';').map((v) => v.trim()).filter((v) => v !== '')
    return parts.length > 1 ? parts.join(' · ') : value
  }
  return HASHTAGS.test(value) ? value.split(',').map((v) => v.trim()).join(' · ') : value
}

export function roleLabel(role: Role, t: TFunction): string {
  return t(ROLE_KEYS[role])
}

// a role named inside a phrase ("used for payee", "with payee, category"); German nouns keep their capital
export const inPhrase = (text: string, language: string) => (language.startsWith('de') ? text : text.toLocaleLowerCase(language))

/** the small "used for payee" tag of each part of the file another role already reads */
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
