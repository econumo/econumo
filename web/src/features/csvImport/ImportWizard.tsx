import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ComponentType } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { profileColumns, type ColumnProfile } from './columns'
import { guessAnswers, isEconumoExport, redetectFormats } from './guess'
import { readCsvFile, type ParsedFile } from './parse'
import { prepareRows, type RowFix, type StepId, type WizardAnswers } from './prepare'
import { StepBar } from './StepBar'
import type { ImportResult } from './upload'
import { STEP_ORDER, defaultAccountName, stepWarnings, visibleSteps, writableAccounts, type LoadedWizardState, type StepBodyProps } from './wizard'
import { AccountStep } from './steps/AccountStep'
import { AmountStep } from './steps/AmountStep'
import { DateStep } from './steps/DateStep'
import { DescriptionStep } from './steps/DescriptionStep'
import { ExtrasStep } from './steps/ExtrasStep'
import { FileStep, type FileError } from './steps/FileStep'
import { PreviewStep } from './steps/PreviewStep'

interface ImportWizardProps {
  open: boolean
  onClose: () => void
  onComplete: (result: ImportResult) => void
  // a file dropped before the dialog opened, read as if picked in it
  initialFile?: File
}

interface LoadedFile {
  file: ParsedFile
  fileName: string
  profiles: ColumnProfile[]
}

const STEP_BODIES: Record<Exclude<StepId, 'file'>, ComponentType<StepBodyProps>> = {
  account: AccountStep,
  date: DateStep,
  amount: AmountStep,
  description: DescriptionStep,
  extras: ExtrasStep,
  preview: PreviewStep,
}

const amountColumnsChanged = (a: WizardAnswers['amount'], b: WizardAnswers['amount']) =>
  a.mode !== b.mode || a.column !== b.column || a.inColumn !== b.inColumn || a.outColumn !== b.outColumn

export function ImportWizard({ open, onClose, initialFile }: ImportWizardProps) {
  const { t } = useTranslation()
  const userQuery = useUserData()
  const accountsQuery = useAccounts()
  const currenciesQuery = useCurrencies()
  const user = userQuery.data

  const [loaded, setLoaded] = useState<LoadedFile | null>(null)
  const [fileError, setFileError] = useState<FileError | null>(null)
  const [answers, setAnswersState] = useState<WizardAnswers | null>(null)
  const [touched, setTouched] = useState<ReadonlySet<StepId>>(() => new Set())
  const [fixes, setFixes] = useState<ReadonlyMap<number, RowFix>>(() => new Map())
  const [step, setStep] = useState<StepId>('file')
  // a read that finishes after a newer pick, a reset or a close must not land
  const readToken = useRef(0)

  const reset = () => {
    readToken.current++
    setLoaded(null)
    setFileError(null)
    setAnswersState(null)
    setTouched(new Set())
    setFixes(new Map())
    setStep('file')
  }

  const handleFile = async (picked: File) => {
    reset()
    const token = readToken.current
    const parsed = await readCsvFile(picked)
    if (token !== readToken.current) return
    if ('error' in parsed) {
      setFileError(parsed.error)
      return
    }
    setLoaded({ file: parsed, fileName: picked.name, profiles: profileColumns(parsed) })
  }

  useEffect(() => {
    if (open) {
      trackEvent(METRICS.TRANSACTION_IMPORT_OPEN, { withFile: initialFile !== undefined })
      reset()
      if (initialFile) void handleFile(initialFile)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialFile])

  // the guess needs to know whether the user has accounts and which currencies
  // exist, so it waits for those reads instead of guessing from empty lists
  const ready = !userQuery.isPending && !accountsQuery.isPending && !currenciesQuery.isPending
  useEffect(() => {
    if (!loaded || answers !== null || !ready) return
    const guessed = guessAnswers(loaded.file, loaded.profiles, {
      hasWritableAccounts: writableAccounts(accountsQuery.data ?? [], user?.id).length > 0,
      userCurrencyId: userCurrencyId(user) ?? '',
      locale: navigator.language,
      defaultAccountName: defaultAccountName(loaded.fileName),
      currencies: currenciesQuery.data ?? [],
    })
    setAnswersState(guessed)
    setStep(isEconumoExport(loaded.file) ? 'preview' : 'account')
  }, [loaded, answers, ready, accountsQuery.data, currenciesQuery.data, user])

  const setAnswers = useCallback(
    (changed: StepId, patch: Partial<WizardAnswers>) => {
      if (!loaded) return
      setAnswersState((prev) => {
        if (prev === null) return prev
        const next = { ...prev, ...patch }
        // a new column gets its formats detected again, unless the same patch chose them
        return redetectFormats(next, loaded.file, navigator.language, {
          date: next.date.column !== prev.date.column && next.date.order === prev.date.order,
          amount: amountColumnsChanged(prev.amount, next.amount) && next.amount.sep === prev.amount.sep,
        })
      })
      setTouched((prev) => (prev.has(changed) ? prev : new Set(prev).add(changed)))
    },
    [loaded],
  )

  const rows = useMemo(() => (loaded && answers ? prepareRows(loaded.file, answers, fixes) : []), [loaded, answers, fixes])
  const warnings = useMemo(() => (answers ? stepWarnings(answers, rows) : new Set<StepId>()), [answers, rows])
  const steps = visibleSteps(answers, loaded?.profiles ?? [])
  const position = steps.indexOf(step)
  const isExport = loaded !== null && isEconumoExport(loaded.file)

  const state: LoadedWizardState | null =
    loaded && answers ? { ...loaded, answers, touched, fixes, rows, isExport } : null

  const goNext = () => {
    if (step === 'file' && isExport) setStep('preview')
    else if (position < steps.length - 1) setStep(steps[position + 1])
  }

  const dismiss = () => {
    trackEvent(METRICS.TRANSACTION_IMPORT_CLOSE, { step })
    onClose()
  }

  const Body = step === 'file' ? null : STEP_BODIES[step]

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && dismiss()}
      title={t('transactions.import_wizard.title')}
      size="wide"
      footer={
        <div className={dialogActionsClass}>
          {position > 0 ? (
            <Button type="button" variant="secondary" onClick={() => setStep(steps[position - 1])}>
              {t('transactions.import_wizard.back')}
            </Button>
          ) : (
            <Button type="button" variant="secondary" onClick={dismiss}>
              {t('common.button.cancel.label')}
            </Button>
          )}
          {step !== 'preview' ? (
            <Button type="button" disabled={state === null} onClick={goNext}>
              {t('transactions.import_wizard.next')}
            </Button>
          ) : null}
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        <StepBar
          steps={STEP_ORDER.map((id) => ({
            id,
            label: t(`transactions.import_wizard.steps.${id}`),
            warning: warnings.has(id),
            hidden: !steps.includes(id),
          }))}
          current={step}
          onSelect={setStep}
          disabled={state === null}
        />
        {Body && state ? (
          <Body state={state} setAnswers={setAnswers} setFixes={setFixes} goTo={setStep} />
        ) : (
          <FileStep fileName={loaded?.fileName ?? null} error={fileError} onFile={(f) => void handleFile(f)} />
        )}
      </div>
    </ResponsiveDialog>
  )
}
