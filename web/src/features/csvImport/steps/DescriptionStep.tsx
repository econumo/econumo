import { useTranslation } from 'react-i18next'
import { columnLabels } from '../columnLabels'
import { FileTable } from '../FileTable'
import type { StepBodyProps } from '../wizard'
import { StepTitle } from './StepTitle'

export function DescriptionStep({ state, setAnswers }: StepBodyProps) {
  const { t } = useTranslation()
  const title = t('transactions.import_wizard.description.title')
  return (
    <div className="flex flex-col gap-3">
      <StepTitle>{title}</StepTitle>
      <FileTable
        file={state.file}
        label={title}
        selected={state.answers.description}
        onSelect={(description) => setAnswers('description', { description })}
        labels={columnLabels(state.answers, t, ['description'])}
        noneLabel={t('transactions.import_wizard.description.none')}
      />
    </div>
  )
}
