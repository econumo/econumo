import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerLines, AnswerRow } from '../controls'
import { NAMES_SHOWN, NamesList } from '../NamesList'
import { useNameResolution } from '../ownerLists'
import { firstDistinct, hasGuess, type StepBodyProps } from '../wizard'

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
          // with nothing in the file that fits, "none" is the answer the step already gave
          unanswered={!state.touched.has(role) && column === null && likely.length > 0}
        />
      ) : column === null ? (
        <AnswerRow onChange={change}>{k(likely.length === 0 ? 'none_found' : 'left_out')}</AnswerRow>
      ) : (
        <AnswerRow from={fromName(state.profiles[column], state.profiles, t)} onChange={change}>
          <AnswerLines lines={examples.values} more={examples.rest} />
        </AnswerRow>
      )}
      {list}
    </div>
  )
}

function DetailNames({ role, state }: { role: Detail; state: StepBodyProps['state'] }) {
  const { t } = useTranslation()
  const { names } = useNameResolution(state.rows, state.answers)
  const k = (key: string) => t(`transactions.import_wizard.${role}.${key}`)
  return (
    <NamesList
      heading={k('found')}
      limit={NAMES_SHOWN}
      items={names[role].map((entry) => {
        const base = entry.outcome === 'existing' ? k('status_existing') : entry.outcome === 'new' ? k('status_new') : t('transactions.import_wizard.status_left_out')
        return {
          key: entry.key,
          name: entry.name,
          count: entry.count,
          status: entry.newFor ? `${base} · ${t(`transactions.import_wizard.category.kind_${entry.newFor}`)}` : base,
        }
      })}
    />
  )
}

export function CategoryStep(props: StepBodyProps) {
  const list = props.state.answers.extras.category !== null ? <DetailNames role="category" state={props.state} /> : null
  return <DetailStep role="category" list={list} {...props} />
}

export function PayeeStep(props: StepBodyProps) {
  const list = props.state.answers.extras.payee !== null ? <DetailNames role="payee" state={props.state} /> : null
  return <DetailStep role="payee" list={list} {...props} />
}
