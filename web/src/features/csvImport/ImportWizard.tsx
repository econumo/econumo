import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ComponentType } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { importTransactionList } from '@/api/transaction'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { finishImportStep, scheduleAiRefresh } from '@/features/onboarding/finishImport'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage } from '@/lib/apiError'
import { METRICS, trackEvent } from '@/lib/metrics'
import { pluralPick } from '@/lib/plural'
import { profileColumns, type ColumnProfile } from './columns'
import { applyAiGuess, guessAnswers, isEconumoExport, redetectFormats } from './guess'
import { readCsvFile, type ParsedFile } from './parse'
import { buildUpload, prepareRows, type Role, type RowFix, type StepId, type WizardAnswers } from './prepare'
import { StepBar } from './StepBar'
import { ensureAccounts, mergeRetry, runChunks, type ImportResult } from './upload'
import { useColumnGuess } from './useColumnGuess'
import {
  STEP_ORDER,
  defaultAccountName,
  distinctNames,
  missingAnswer,
  nameKey,
  stepWarnings,
  visibleSteps,
  writableAccounts,
  type LoadedWizardState,
  type StepBodyProps,
} from './wizard'
import { AccountStep } from './steps/AccountStep'
import { AmountStep } from './steps/AmountStep'
import { DateStep } from './steps/DateStep'
import { DescriptionStep } from './steps/DescriptionStep'
import { ExtrasStep } from './steps/ExtrasStep'
import { FileStep, type FileError } from './steps/FileStep'
import { PreviewStep } from './steps/PreviewStep'

/** sends the chunks that failed again and answers the whole import's result so far */
export type RetryFailedChunks = () => Promise<ImportResult>

interface ImportWizardProps {
  open: boolean
  onClose: () => void
  // `retry` is given while some chunks failed to send
  onComplete: (result: ImportResult, retry?: RetryFailedChunks) => void
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

type Upload = ReturnType<typeof buildUpload>

const reportRun = (result: ImportResult) => {
  if (result.imported > 0) trackEvent(METRICS.TRANSACTION_IMPORT, { imported: result.imported, failed: result.failed })
  else trackEvent(METRICS.TRANSACTION_IMPORT_FAIL, { failed: result.failed })
}

const amountColumnsChanged = (a: WizardAnswers['amount'], b: WizardAnswers['amount']) =>
  a.mode !== b.mode || a.column !== b.column || a.inColumn !== b.inColumn || a.outColumn !== b.outColumn

export function ImportWizard({ open, onClose, onComplete, initialFile }: ImportWizardProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const online = useOnline()
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
  const [aiRoles, setAiRoles] = useState<Partial<Record<Role, string>> | null>(null)
  const [importing, setImporting] = useState(false)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [importError, setImportError] = useState<string | null>(null)
  // a read that finishes after a newer pick, a reset or a close must not land
  const readToken = useRef(0)
  // accounts made for this file, so pressing Import again never makes them twice
  const created = useRef(new Map<string, string>())
  // what the last import sent and got back: "Try the rest again" re-sends its failed chunks
  const lastRun = useRef<{ upload: Upload; result: ImportResult } | null>(null)

  const reset = () => {
    readToken.current++
    setLoaded(null)
    setFileError(null)
    setAnswersState(null)
    setTouched(new Set())
    setFixes(new Map())
    setStep('file')
    setAiRoles(null)
    setImportError(null)
    setProgress(null)
    created.current = new Map()
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

  const isExport = loaded !== null && isEconumoExport(loaded.file)
  useColumnGuess(open && loaded && !isExport ? loaded.profiles : null, setAiRoles)

  // a reply that lands before the built-in guess waits for it; a step the user answered is kept
  useEffect(() => {
    if (!aiRoles || !answers || !loaded) return
    setAiRoles(null)
    const next = applyAiGuess(answers, aiRoles, loaded.file, navigator.language, touched)
    if (JSON.stringify(next) === JSON.stringify(answers)) return
    setAnswersState(next)
    trackEvent(METRICS.TRANSACTION_IMPORT_AI_GUESS)
  }, [aiRoles, answers, loaded, touched])

  const rows = useMemo(() => (loaded && answers ? prepareRows(loaded.file, answers, fixes) : []), [loaded, answers, fixes])
  const warnings = useMemo(() => (answers ? stepWarnings(answers, rows) : new Set<StepId>()), [answers, rows])
  const steps = visibleSteps(answers, loaded?.profiles ?? [])
  const position = steps.indexOf(step)

  const state: LoadedWizardState | null =
    loaded && answers ? { ...loaded, answers, touched, fixes, rows, isExport } : null

  const goNext = () => {
    if (step === 'file' && isExport) setStep('preview')
    else if (position < steps.length - 1) setStep(steps[position + 1])
  }

  const dismiss = () => {
    if (importing) return
    trackEvent(METRICS.TRANSACTION_IMPORT_CLOSE, { step })
    onClose()
  }

  const importCount = rows.filter((row) => row.status === 'ok').length
  const canImport =
    state !== null && online && !importing && importCount > 0 && !(['account', 'date', 'amount'] as const).some((s) => missingAnswer(s, state.answers))

  // the server asks the AI for icons for what the import created; show them once they land.
  // During onboarding, rows landing settle the import step through finish-import (budget + AI
  // savings); a failure there is retried by the Import step and never fails the import itself.
  const settle = async (result: ImportResult) => {
    scheduleAiRefresh(queryClient)
    if (result.imported > 0) await finishImportStep(queryClient).catch(() => undefined)
    await queryClient.invalidateQueries()
  }

  const retryFailed: RetryFailedChunks = async () => {
    const run = lastRun.current
    if (!run || run.result.failedChunks.length === 0) return run?.result ?? { imported: 0, failed: 0, errors: [], failedChunks: [] }
    const chunks = run.result.failedChunks
    const again = await runChunks(run.upload, importTransactionList, { chunks })
    reportRun(again)
    const result = mergeRetry(run.result, again, run.upload, chunks)
    lastRun.current = { upload: run.upload, result }
    await settle(again).catch(() => undefined)
    return result
  }

  const startImport = async () => {
    if (!state || !canImport) return
    setImporting(true)
    setImportError(null)
    setProgress(null)
    const { answers: current, file } = state
    const existing = new Set(writableAccounts(accountsQuery.data ?? [], user?.id).map((a) => nameKey(a.name)))
    const names =
      current.account?.kind === 'column' ? new Map(distinctNames(file, current.account.column).map((name) => [nameKey(name), name])) : undefined
    let accountId: string | undefined
    try {
      accountId = (await ensureAccounts(current, created.current, existing, names)).accountId
    } catch (error) {
      trackEvent(METRICS.TRANSACTION_IMPORT_FAIL, { failed: importCount })
      setImportError(apiErrorMessage(error))
      setImporting(false)
      return
    }
    // from here rows may land, so nothing may leave the dialog open for a second Import
    try {
      const upload = buildUpload(rows, current)
      if (accountId) upload.fields.accountId = accountId
      const skipped = rows.filter((row) => row.status === 'skipped' && row.reason !== 'user_skipped')
      const result = await runChunks(upload, importTransactionList, { skipped, onProgress: (done, total) => setProgress({ done, total }) })
      reportRun(result)
      lastRun.current = { upload, result }
      await settle(result).catch(() => undefined)
      onClose()
      onComplete(result, result.failedChunks.length > 0 ? retryFailed : undefined)
    } finally {
      setImporting(false)
    }
  }

  const Body = step === 'file' ? null : STEP_BODIES[step]

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && dismiss()}
      dismissible={!importing}
      title={t('transactions.import_wizard.title')}
      size="wide"
      footer={
        <div className={dialogActionsClass}>
          {position > 0 ? (
            <Button type="button" variant="secondary" disabled={importing} onClick={() => setStep(steps[position - 1])}>
              {t('transactions.import_wizard.back')}
            </Button>
          ) : (
            <Button type="button" variant="secondary" disabled={importing} onClick={dismiss}>
              {t('common.button.cancel.label')}
            </Button>
          )}
          {step !== 'preview' ? (
            <Button type="button" disabled={state === null} onClick={goNext}>
              {t('transactions.import_wizard.next')}
            </Button>
          ) : (
            <Button type="button" disabled={!canImport} onClick={() => void startImport()}>
              {pluralPick(t('transactions.import_wizard.preview.import'), importCount, i18n.language)}
            </Button>
          )}
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
          disabled={state === null || importing}
        />
        {Body && state ? (
          // answers must not change between the upload and a retry of its failed chunks
          <fieldset disabled={importing} className="contents">
            <Body state={state} setAnswers={setAnswers} setFixes={setFixes} goTo={setStep} />
          </fieldset>
        ) : (
          <FileStep fileName={loaded?.fileName ?? null} error={fileError} onFile={(f) => void handleFile(f)} />
        )}
        {importing && progress ? (
          <Progress value={(progress.done / progress.total) * 100} aria-label={t('transactions.import_wizard.preview.importing')} />
        ) : null}
        {importError ? (
          <p role="alert" className="text-sm text-destructive">
            {importError}
          </p>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}
