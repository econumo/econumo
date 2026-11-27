import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useCategories } from '@/features/classifications/queries'
import { pluralPick } from '@/lib/plural'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerRow } from '../controls'
import { categoryKey, distinctNew, useOwnerLists } from '../ownerLists'
import { firstDistinct, hasGuess, type StepBodyProps } from '../wizard'

type Detail = 'category' | 'payee'

function DetailStep({ role, state, setAnswers, choosing, change, closeChoices, notes }: StepBodyProps & { role: Detail; notes: string[] }) {
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
      {notes.map((note) => (
        <p key={note} role="status" className="text-xs text-muted-foreground">
          {note}
        </p>
      ))}
    </div>
  )
}

export function CategoryStep(props: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { data: categories = [] } = useCategories()
  const { ownerId, readOnlyLists } = useOwnerLists(props.state.answers.account)
  const chosen = props.state.answers.extras.category !== null
  // Archived ones count too: the server finds them and does not create a second one.
  const existing = useMemo(() => new Set(categories.filter((c) => c.ownerUserId === ownerId).map((c) => categoryKey(c.type, c.name))), [categories, ownerId])
  const created = useMemo(() => distinctNew(props.state.rows, 'category', null, existing), [props.state.rows, existing])
  const notes =
    chosen && readOnlyLists
      ? [t('transactions.import_wizard.category.shared_owner_lists')]
      : chosen && created > 0
        ? [pluralPick(t('transactions.import_wizard.category.new'), created, i18n.language)]
        : []
  return <DetailStep role="category" notes={notes} {...props} />
}

export function PayeeStep(props: StepBodyProps) {
  const { t } = useTranslation()
  const { readOnlyLists } = useOwnerLists(props.state.answers.account)
  const notes = props.state.answers.extras.payee !== null && readOnlyLists ? [t('transactions.import_wizard.payee.shared_owner_lists')] : []
  return <DetailStep role="payee" notes={notes} {...props} />
}
