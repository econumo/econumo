import type { TFunction } from 'i18next'
import { pluralPick } from '@/lib/plural'
import { formatDay } from './accountView'
import { columnName, inPhrase, roleLabel } from './columnLabels'
import type { ColumnProfile } from './columns'
import type { PreparedRow, StepId, WizardAnswers } from './prepare'

export interface SummaryContext {
  answers: WizardAnswers
  profiles: ColumnProfile[]
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
  const { answers, profiles, rows, t, language } = ctx
  const k = (key: string, vars?: Record<string, unknown>) => t(`transactions.import_wizard.summary.${key}`, vars)
  const header = (column: number | null) => {
    const profile = column === null ? undefined : profiles[column]
    return profile ? columnName(profile, t) : null
  }
  switch (step) {
    case 'account': {
      const { account } = answers
      if (account?.kind === 'new') return k('new_account', { name: account.name, currency: ctx.currencyCode(account.currencyId) ?? '' })
      if (account?.kind === 'existing') {
        const found = ctx.account(account.accountId)
        return found ? `${found.name} · ${found.code}` : k('not_chosen')
      }
      if (account?.kind === 'column') return k('each_row', { column: header(account.column) ?? '' })
      return k('not_chosen')
    }
    case 'date': {
      const name = header(answers.date.column)
      if (name === null) return k('not_chosen')
      const first = rows.find((row) => row.date !== null)
      return first?.date ? `${name} · ${formatDay(first.date, language)}` : name
    }
    case 'amount': {
      const { amount } = answers
      if (amount.mode === 'dual') {
        if (amount.inColumn === null && amount.outColumn === null) return k('not_chosen')
        return k('in_out', { in: header(amount.inColumn) ?? '—', out: header(amount.outColumn) ?? '—' })
      }
      const name = header(amount.column)
      if (name === null) return k('not_chosen')
      const parts = [name, amount.sep === ',' ? '1.234,56' : '1,234.56']
      if (amount.flip) parts.push(k('flipped'))
      return parts.join(' · ')
    }
    case 'description':
      return header(answers.description) ?? k('none')
    case 'extras': {
      const roles = (['payee', 'category', 'tag', 'labels'] as const).filter((role) => answers.extras[role] !== null)
      return roles.length > 0 ? k('with', { list: roles.map((role) => inPhrase(roleLabel(role, t), language)).join(', ') }) : k('nothing_extra')
    }
    case 'preview': {
      const skipped = rows.filter((row) => row.status === 'skipped').length
      return pluralPick(k('check', { skipped }), rows.length - skipped, language)
    }
  }
}
