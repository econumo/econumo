import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ComponentType } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { importTransactionList } from '@/api/transaction'
import { queryKeys } from '@/app/queryKeys'
import { useAccounts, useFolders } from '@/features/accounts/queries'
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
import { buildUpload, prepareRows, pruneFixes, type Role, type RowFix, type StepId, type WizardAnswers } from './prepare'
import { ensureAccounts, mergeRetry, runChunks, type ImportResult } from './upload'
import { stepSummary } from './stepSummary'
import { useColumnGuess } from './useColumnGuess'
import { WizardShell, type ShellStep } from './WizardShell'
import {
  defaultAccountName,
  distinctNames,
  hasGuess,
  missingAnswer,
  nameKey,
  newAccountFolderId,
  releaseTakenExtras,
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
  const foldersQuery = useFolders()
  const currenciesQuery = useCurrencies()
  const user = userQuery.data

  const [loaded, setLoaded] = useState<LoadedFile | null>(null)
  const [fileError, setFileError] = useState<FileError | null>(null)
  const [answers, setAnswersState] = useState<WizardAnswers | null>(null)
  const [touched, setTouched] = useState<ReadonlySet<StepId>>(() => new Set())
  // steps whose list of choices was opened; not "touched": opening it answers nothing
  const [choosing, setChoosing] = useState<ReadonlySet<StepId>>(() => new Set())
  const [fixes, setFixes] = useState<ReadonlyMap<number, RowFix>>(() => new Map())
  const [step, setStep] = useState<StepId>('file')
  // a phone shows the step list first; picking a step opens it over the list
  const [phoneList, setPhoneList] = useState(false)
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
    setChoosing(new Set())
    setFixes(new Map())
    setStep('file')
    setPhoneList(false)
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
      writableAccountIds: writableAccounts(accountsQuery.data ?? [], user?.id).map((a) => a.id),
      userCurrencyId: userCurrencyId(user) ?? '',
      locale: navigator.language,
      defaultAccountName: defaultAccountName(loaded.fileName),
      currencies: currenciesQuery.data ?? [],
    })
    setAnswersState(releaseTakenExtras(guessed))
    const exported = isEconumoExport(loaded.file)
    setStep(exported ? 'preview' : 'account')
    setPhoneList(!exported)
  }, [loaded, answers, ready, accountsQuery.data, currenciesQuery.data, user])

  const setAnswers = useCallback(
    (changed: StepId, patch: Partial<WizardAnswers>) => {
      if (!loaded) return
      setAnswersState((prev) => {
        if (prev === null) return prev
        const next = { ...prev, ...patch }
        // a new column gets its formats detected again, unless the same patch chose them
        const redetected = redetectFormats(next, loaded.file, navigator.language, {
          date: next.date.column !== prev.date.column && next.date.order === prev.date.order,
          amount: amountColumnsChanged(prev.amount, next.amount) && next.amount.sep === prev.amount.sep,
        })
        return releaseTakenExtras(redetected)
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
    const next = releaseTakenExtras(applyAiGuess(answers, aiRoles, loaded.file, navigator.language, touched))
    if (JSON.stringify(next) === JSON.stringify(answers)) return
    setAnswersState(next)
    trackEvent(METRICS.TRANSACTION_IMPORT_AI_GUESS)
  }, [aiRoles, answers, loaded, touched])

  // a typed value or a skip is dropped once a changed answer makes the row read on its own
  useEffect(() => {
    if (loaded && answers) setFixes((prev) => pruneFixes(loaded.file, answers, prev))
  }, [loaded, answers])

  const rows = useMemo(() => (loaded && answers ? prepareRows(loaded.file, answers, fixes) : []), [loaded, answers, fixes])
  const steps = visibleSteps(answers, loaded?.profiles ?? [], step)
  const position = steps.indexOf(step)
  const listed = steps.filter((s): s is Exclude<StepId, 'file'> => s !== 'file')

  const state: LoadedWizardState | null =
    loaded && answers ? { ...loaded, answers, touched, fixes, rows, isExport } : null

  const openStep = (next: StepId) => {
    setStep(next)
    setPhoneList(false)
  }

  const goNext = () => {
    if (step === 'file' && isExport) openStep('preview')
    else if (position < steps.length - 1) openStep(steps[position + 1])
  }

  const dismiss = () => {
    if (importing) return
    trackEvent(METRICS.TRANSACTION_IMPORT_CLOSE, { step })
    onClose()
  }

  const importCount = rows.filter((row) => row.status === 'ok').length
  // a new account needs the user's folder: the server refuses a blank one once the user has any
  const canImport =
    state !== null &&
    online &&
    !importing &&
    !foldersQuery.isPending &&
    importCount > 0 &&
    !(['account', 'date', 'amount'] as const).some((s) => missingAnswer(s, state.answers))

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
    const importedNames = new Set(rows.flatMap((row) => (row.status === 'ok' && row.account ? [nameKey(row.account)] : [])))
    // only accounts the imported rows name: a name found only on left-out rows makes none
    const names =
      current.account?.kind === 'column'
        ? new Map(distinctNames(file, current.account.column).filter((name) => importedNames.has(nameKey(name))).map((name) => [nameKey(name), name]))
        : undefined
    let accountId: string | undefined
    try {
      accountId = (await ensureAccounts(current, created.current, existing, names, newAccountFolderId(foldersQuery.data))).accountId
    } catch (error) {
      trackEvent(METRICS.TRANSACTION_IMPORT_FAIL, { failed: importCount })
      setImportError(apiErrorMessage(error))
      // accounts (and the folder the server made for the first) may have landed before the
      // failure; a new file or a reopened dialog forgets `created`, so they must be in the lists
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.accounts }),
        queryClient.invalidateQueries({ queryKey: queryKeys.folders }),
      ]).catch(() => undefined)
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

  const chooseStep = () => {
    if (choosing.has(step)) return
    setChoosing((prev) => new Set(prev).add(step))
    trackEvent(METRICS.TRANSACTION_IMPORT_CHOOSE_DIFFERENTLY, { step })
  }
  // A step that opens on its list of choices keeps it open: picking an answer there must not
  // fold the list away under the pointer. Set during render, so the list never flickers shut.
  if (state !== null && step !== 'file' && step !== 'preview' && !choosing.has(step) && !hasGuess(step, state)) {
    setChoosing((prev) => new Set(prev).add(step))
  }
  // a guessed answer has its own "Looks right" under it, so the footer has no second Continue
  const guessShown = state !== null && !choosing.has(step) && hasGuess(step, state)

  const Body = step === 'file' ? null : STEP_BODIES[step]
  const okCount = importCount
  const leftOut = rows.length - okCount
  const currencies = currenciesQuery.data ?? []
  const allAccounts = accountsQuery.data ?? []
  const shellSteps: ShellStep[] = state
    ? listed.map((id) => ({
        id,
        title: t(`transactions.import_wizard.steps.${id}`),
        summary: stepSummary(id, {
          answers: state.answers,
          rows,
          account: (accountId) => {
            const found = allAccounts.find((a) => a.id === accountId)
            return found ? { name: found.name, code: found.currency.code } : null
          },
          currencyCode: (currencyId) => currencies.find((c) => c.id === currencyId)?.code ?? null,
          t,
          language: i18n.language,
        }),
        answered: id !== 'preview' && !missingAnswer(id, state.answers),
        warning: missingAnswer(id, state.answers),
      }))
    : listed.map((id) => ({ id, title: t(`transactions.import_wizard.steps.${id}`), summary: '', answered: false, warning: false }))
  const question = t(step === 'preview' ? 'transactions.import_wizard.preview.title' : `transactions.import_wizard.${step}.title`)
  const at = listed.indexOf(step as Exclude<StepId, 'file'>)

  return (
    <WizardShell
      open={open}
      onClose={dismiss}
      closable={!importing}
      file={loaded ? { name: loaded.fileName, rows: rows.length || loaded.file.rows.length } : null}
      steps={shellSteps}
      current={step}
      onSelect={openStep}
      locked={importing}
      position={
        at < 0
          ? null
          : t('transactions.import_wizard.step_of', { n: at + 1, total: listed.length, title: t(`transactions.import_wizard.steps.${step}`) })
      }
      question={question}
      ready={state ? pluralPick(t('transactions.import_wizard.ready', { skipped: leftOut }), okCount, i18n.language) : null}
      phoneList={phoneList}
      onPhoneBack={() => setPhoneList(true)}
      phoneCheck={state ? { label: pluralPick(t('transactions.import_wizard.check_button'), okCount, i18n.language), onClick: () => openStep('preview') } : null}
      status={
        <>
          {importing && progress ? (
            <Progress value={(progress.done / progress.total) * 100} aria-label={t('transactions.import_wizard.preview.importing')} />
          ) : null}
          {importError ? (
            <p role="alert" className="text-sm text-destructive">
              {importError}
            </p>
          ) : null}
        </>
      }
      actions={
        <>
          {position > 0 && steps[position - 1] !== 'file' ? (
            <Button type="button" variant="ghost" className="h-10" disabled={importing} onClick={() => openStep(steps[position - 1])}>
              {t('transactions.import_wizard.back')}
            </Button>
          ) : null}
          <div className="ml-auto" />
          {guessShown ? null : step !== 'preview' ? (
            <Button type="button" className="h-10 px-5" disabled={state === null} onClick={goNext}>
              {t('transactions.import_wizard.continue')}
            </Button>
          ) : (
            // a long count or language wraps to two lines instead of running out of the button
            <Button type="button" className="h-auto min-h-10 px-5 leading-tight whitespace-normal text-balance" disabled={!canImport} onClick={() => void startImport()}>
              {pluralPick(t('transactions.import_wizard.preview.import'), importCount, i18n.language)}
            </Button>
          )}
        </>
      }
    >
      {Body && state ? (
        // answers must not change between the upload and a retry of its failed chunks
        <fieldset disabled={importing} className="contents">
          <Body
            state={state}
            setAnswers={setAnswers}
            setFixes={setFixes}
            goTo={openStep}
            choosing={choosing.has(step)}
            choose={chooseStep}
            next={goNext}
          />
        </fieldset>
      ) : (
        <FileStep fileName={loaded?.fileName ?? null} error={fileError} onFile={(f) => void handleFile(f)} />
      )}
    </WizardShell>
  )
}
