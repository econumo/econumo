import { useTranslation } from 'react-i18next'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { Answer, AnswerActions } from '../controls'
import { firstDistinct, hasGuess, type StepBodyProps } from '../wizard'

export function DescriptionStep({ state, setAnswers, choosing, choose, next }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { description } = state.answers
  const guessed = hasGuess('description', state)
  const open = choosing || !guessed
  const examples = firstDistinct(state.rows, (row) => row.description)
  return (
    <div className="flex flex-col gap-6">
      {guessed ? (
        <Answer from={description === null ? null : fromName(state.profiles[description], state.profiles, t)}>
          {examples.values.join(' · ')}
          {examples.more ? ' …' : ''}
        </Answer>
      ) : null}
      {open ? (
        <ColumnChoices
          profiles={state.profiles}
          label={t('transactions.import_wizard.description.choose')}
          selected={description}
          onSelect={(column) => setAnswers('description', { description: column })}
          fits={(type) => type === 'text'}
          tags={columnTags(state.answers, t, i18n.language, ['description'])}
          noneLabel={t('transactions.import_wizard.description.none')}
        />
      ) : (
        <AnswerActions onConfirm={next} onChoose={choose} />
      )}
    </div>
  )
}
