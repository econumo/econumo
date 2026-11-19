import { useTranslation } from 'react-i18next'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags } from '../columnLabels'
import type { StepBodyProps } from '../wizard'

export function DescriptionStep({ state, setAnswers }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  return (
    <ColumnChoices
      profiles={state.profiles}
      label={t('transactions.import_wizard.description.title')}
      selected={state.answers.description}
      onSelect={(description) => setAnswers('description', { description })}
      fits={(type) => type === 'text'}
      tags={columnTags(state.answers, t, i18n.language, ['description'])}
      noneLabel={t('transactions.import_wizard.description.none')}
    />
  )
}
