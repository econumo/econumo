import { useTranslation } from 'react-i18next'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerRow } from '../controls'
import { firstDistinct, hasGuess, type StepBodyProps } from '../wizard'

export function DescriptionStep({ state, setAnswers, choosing, change, closeChoices }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { description } = state.answers
  const guessed = hasGuess('description', state)
  const open = choosing || !guessed
  const examples = firstDistinct(state.rows, (row) => row.description)
  return (
    <div className="flex flex-col gap-6">
      {open ? (
        <ColumnChoices
          profiles={state.profiles}
          label={t('transactions.import_wizard.description.choose')}
          selected={description}
          onSelect={(column) => {
            setAnswers('description', { description: column })
            closeChoices()
          }}
          candidates={candidates('description', state.profiles, state.answers)}
          tags={columnTags(state.answers, t, i18n.language, ['description'])}
          noneLabel={t('transactions.import_wizard.description.none')}
        />
      ) : (
        <AnswerRow from={description === null ? null : fromName(state.profiles[description], state.profiles, t)} onChange={change}>
          {examples.values.join(' · ')}
          {examples.more ? ' …' : ''}
        </AnswerRow>
      )}
    </div>
  )
}
