import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useCategories, usePayees } from '@/features/classifications/queries'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerRow } from '../controls'
import { NAMES_SHOWN, NamesList } from '../NamesList'
import { categoryKey, namesInFile, useOwnerLists } from '../ownerLists'
import type { PreparedRow } from '../prepare'
import { firstDistinct, hasGuess, nameKey, type StepBodyProps } from '../wizard'

type Detail = 'category' | 'payee'

function DetailStep({ role, state, setAnswers, choosing, change, closeChoices, list }: StepBodyProps & { role: Detail; list: ReactNode }) {
  const { t, i18n } = useTranslation()
  const k = (key: string) => t(`transactions.import_wizard.${role}.${key}`)
  const column = state.answers.extras[role]
  const likely = candidates(role, state.profiles, state.answers)
  const open = choosing || !hasGuess(role, state)
  const examples = firstDistinct(state.rows, (row) => row[role])
  return (
    <div className="flex flex-col gap-5">
      {open ? (
        <ColumnChoices
          profiles={state.profiles}
          label={k('choose')}
          selected={column}
          onSelect={(picked) => {
            setAnswers(role, { extras: { ...state.answers.extras, [role]: picked } })
            closeChoices()
          }}
          candidates={likely}
          nothingFitsNote={t(`transactions.import_wizard.nothing_fits.${role}`)}
          onlyOneNote={t(`transactions.import_wizard.only_one.${role}`)}
          tags={columnTags(state.answers, t, i18n.language, [role])}
          noneLabel={k('none')}
        />
      ) : column === null ? (
        <AnswerRow onChange={change}>{k(likely.length === 0 ? 'none_found' : 'left_out')}</AnswerRow>
      ) : (
        <AnswerRow from={fromName(state.profiles[column], state.profiles, t)} onChange={change}>
          {examples.values.join(' · ')}
          {examples.more ? ' …' : ''}
        </AnswerRow>
      )}
      {list}
    </div>
  )
}

function DetailNames({ role, rows, existing, readOnlyLists }: { role: Detail; rows: PreparedRow[]; existing: Set<string>; readOnlyLists: boolean }) {
  const { t } = useTranslation()
  const names = useMemo(() => namesInFile(rows, role, null, existing), [rows, role, existing])
  const k = (key: string) => t(`transactions.import_wizard.${role}.${key}`)
  return (
    <NamesList
      heading={k('found')}
      limit={NAMES_SHOWN}
      items={names.map((entry) => {
        const base = !entry.isNew ? k('status_existing') : readOnlyLists ? t('transactions.import_wizard.status_left_out') : k('status_new')
        return {
          key: entry.key,
          name: entry.name,
          count: entry.count,
          status: entry.newKind ? `${base} · ${t(`transactions.import_wizard.category.kind_${entry.newKind}`)}` : base,
        }
      })}
    />
  )
}

export function CategoryStep(props: StepBodyProps) {
  const { data: categories = [] } = useCategories()
  const { ownerId, readOnlyLists } = useOwnerLists(props.state.answers.account)
  // Archived ones count too: the server finds them and does not create a second one.
  const existing = useMemo(() => new Set(categories.filter((c) => c.ownerUserId === ownerId).map((c) => categoryKey(c.type, c.name))), [categories, ownerId])
  const list = props.state.answers.extras.category !== null ? <DetailNames role="category" rows={props.state.rows} existing={existing} readOnlyLists={readOnlyLists} /> : null
  return <DetailStep role="category" list={list} {...props} />
}

export function PayeeStep(props: StepBodyProps) {
  const { data: payees = [] } = usePayees()
  const { ownerId, readOnlyLists } = useOwnerLists(props.state.answers.account)
  const existing = useMemo(() => new Set(payees.filter((p) => p.ownerUserId === ownerId).map((p) => nameKey(p.name))), [payees, ownerId])
  const list = props.state.answers.extras.payee !== null ? <DetailNames role="payee" rows={props.state.rows} existing={existing} readOnlyLists={readOnlyLists} /> : null
  return <DetailStep role="payee" list={list} {...props} />
}
