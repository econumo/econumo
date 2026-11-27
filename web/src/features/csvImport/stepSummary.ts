import type { TFunction } from 'i18next'
import { pluralPick } from '@/lib/plural'
import { abs, isNegative } from '@/lib/decimal'
import { formatDay } from './accountView'
import { inPhrase, roleLabel } from './columnLabels'
import type { PreparedRow, StepId, WizardAnswers } from './prepare'
import { firstDistinct, missingAnswer, nameKey } from './wizard'

const SUMMARY_AMOUNTS = 2
const SUMMARY_NAMES = 2

export interface SummaryContext {
  answers: WizardAnswers
  rows: PreparedRow[]
  /** an existing account's name and currency code */
  account: (id: string) => { name: string; code: string } | null
  /** a currency's code */
  currencyCode: (id: string) => string | null
  t: TFunction
  language: string
}

/** a step's answer in plain words, shown under its name in the step list */
export function stepSummary(step: Exclude<StepId, 'file'>, ctx: SummaryContext): string {
  const { answers, rows, t, language } = ctx
  const k = (key: string, vars?: Record<string, unknown>) => t(`transactions.import_wizard.summary.${key}`, vars)
  switch (step) {
    case 'account': {
      const { account } = answers
      if (account?.kind === 'new') return k('new_account', { name: account.name, currency: ctx.currencyCode(account.currencyId) ?? '' })
      if (account?.kind === 'existing') {
        const found = ctx.account(account.accountId)
        return found ? `${found.name} · ${found.code}` : k('not_chosen')
      }
      if (account?.kind === 'column') {
        const names = new Map<string, string>()
        for (const row of rows) if (row.account !== null && !names.has(nameKey(row.account))) names.set(nameKey(row.account), row.account.trim())
        const [first] = names.values()
        if (first === undefined) return k('not_chosen')
        return names.size === 1 ? k('from_account', { name: first }) : pluralPick(k('from_accounts', { name: first }), names.size - 1, language)
      }
      return k('not_chosen')
    }
    case 'date': {
      if (answers.date.column === null) return k('not_chosen')
      const first = rows.find((row) => row.date !== null)
      return first?.date ? formatDay(first.date, language) : k('not_chosen')
    }
    case 'amount': {
      if (missingAnswer('amount', answers)) return k('not_chosen')
      const format = new Intl.NumberFormat(language, { minimumFractionDigits: 2, maximumFractionDigits: 20 })
      const amounts = rows
        .filter((row) => row.amount !== null)
        .slice(0, SUMMARY_AMOUNTS)
        .map((row) => `${isNegative(row.amount as string) ? '−' : '+'}${format.format(Number(abs(row.amount as string)))}`)
      return amounts.length > 0 ? amounts.join(' · ') : k('not_chosen')
    }
    case 'description': {
      if (answers.description === null) return k('none')
      return firstDistinct(rows, (row) => row.description, 1).values[0] ?? k('none')
    }
    case 'category':
    case 'payee': {
      if (answers.extras[step] === null) return k('nothing')
      const { values } = firstDistinct(rows, (row) => row[step], SUMMARY_NAMES)
      return values.length > 0 ? values.join(' · ') : k('nothing')
    }
    case 'extras': {
      const roles = (['tag', 'labels'] as const).filter((role) => answers.extras[role] !== null)
      return roles.length > 0 ? k('with', { list: roles.map((role) => inPhrase(roleLabel(role, t), language)).join(', ') }) : k('nothing_extra')
    }
    case 'preview': {
      const skipped = rows.filter((row) => row.status === 'skipped').length
      return pluralPick(k('check', { skipped }), rows.length - skipped, language)
    }
  }
}
