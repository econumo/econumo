import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeftRight, Split } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { importTransactionList } from '@/api/transaction'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import type { AggregatedImportResult, CsvAnalysis, FieldKey, ImportSelection } from './importCsv'
import { analyzeCsv, autoDetect, CHUNK_SIZE, defaultSelection, runImport, selectionValid } from './importCsv'

const MAX_FILE_SIZE = 10485760

interface ImportCsvDialogProps {
  open: boolean
  onClose: () => void
  onComplete: (result: AggregatedImportResult) => void
}

export function ImportCsvDialog({ open, onClose, onComplete }: ImportCsvDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()

  const [file, setFile] = useState<File | null>(null)
  const [analysis, setAnalysis] = useState<CsvAnalysis | null>(null)
  const [selection, setSelection] = useState<ImportSelection>(defaultSelection)
  const [submitting, setSubmitting] = useState(false)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)

  const reset = () => {
    setFile(null)
    setAnalysis(null)
    setSelection(defaultSelection())
    setSubmitting(false)
    setProgress(null)
  }

  useEffect(() => {
    if (open) {
      reset()
    }
  }, [open])

  const meId = user?.id
  // accounts I can write transactions into: mine, or shared to me above view-only
  const writableAccounts = useMemo(
    () =>
      accounts.filter((account) => {
        if (account.owner.id === meId) return true
        const grant = account.sharedAccess.find((a) => a.user.id === meId)
        return grant !== undefined && grant.role !== 'guest'
      }),
    [accounts, meId],
  )

  const targetUserId = useMemo(() => {
    if (selection.modes.account === 'existing' && selection.fixed.accountId) {
      return accounts.find((a) => a.id === selection.fixed.accountId)?.owner.id ?? meId
    }
    return meId
  }, [selection.modes.account, selection.fixed.accountId, accounts, meId])

  // switching the target account owner invalidates fixed entity picks (Vue parity)
  useEffect(() => {
    setSelection((prev) =>
      prev.fixed.categoryId || prev.fixed.payeeId || prev.fixed.tagId
        ? { ...prev, fixed: { ...prev.fixed, categoryId: null, payeeId: null, tagId: null } }
        : prev,
    )
  }, [targetUserId])

  const labels: Record<FieldKey, string> = {
    account: t('modals.import_csv.fields.account'),
    date: t('modals.import_csv.fields.date'),
    amount: t('modals.import_csv.fields.amount'),
    amountInflow: t('modals.import_csv.fields.amount_inflow'),
    amountOutflow: t('modals.import_csv.fields.amount_outflow'),
    category: t('modals.import_csv.fields.category'),
    description: t('modals.import_csv.fields.description'),
    payee: t('modals.import_csv.fields.payee'),
    tag: t('modals.import_csv.fields.tag'),
  }

  const handleFile = async (picked: File | undefined) => {
    if (!picked) return
    if (picked.size > MAX_FILE_SIZE) {
      reset()
      return
    }
    const text = await picked.text()
    const parsed = analyzeCsv(text)
    const detected = autoDetect(parsed.header, labels)
    setFile(picked)
    setAnalysis(parsed)
    setSelection({ ...defaultSelection(), columns: detected.columns, amountMode: detected.amountMode })
  }

  const patchColumns = (patch: Partial<ImportSelection['columns']>) =>
    setSelection((prev) => ({ ...prev, columns: { ...prev.columns, ...patch } }))
  const patchFixed = (patch: Partial<ImportSelection['fixed']>) =>
    setSelection((prev) => ({ ...prev, fixed: { ...prev.fixed, ...patch } }))
  const toggleMode = (field: keyof ImportSelection['modes'], next: ImportSelection['modes'][typeof field]) =>
    setSelection((prev) => ({
      ...prev,
      modes: { ...prev.modes, [field]: prev.modes[field] === 'csv_column' ? next : 'csv_column' },
    }))

  const columnSelect = (field: FieldKey, value: string | null) => (
    <select
      aria-label={labels[field]}
      className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
      value={value ?? ''}
      onChange={(e) => patchColumns({ [field]: e.target.value === '' ? null : e.target.value } as Partial<ImportSelection['columns']>)}
    >
      <option value="">{t('modals.import_csv.none')}</option>
      {(analysis?.header ?? []).map((column) => (
        <option key={column} value={column}>
          {analysis?.samples[column] ? `${column} ("${analysis.samples[column]}")` : column}
        </option>
      ))}
    </select>
  )

  const entitySelect = (
    field: 'categoryId' | 'payeeId' | 'tagId',
    label: string,
    items: { id: string; name: string; ownerUserId: string }[],
  ) => (
    <select
      aria-label={label}
      className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
      value={selection.fixed[field] ?? ''}
      onChange={(e) => patchFixed({ [field]: e.target.value === '' ? null : e.target.value })}
    >
      <option value="">{t('modals.import_csv.none')}</option>
      {items
        .filter((item) => item.ownerUserId === targetUserId)
        .map((item) => (
          <option key={item.id} value={item.id}>
            {item.name}
          </option>
        ))}
    </select>
  )

  const modeToggle = (field: keyof ImportSelection['modes'], next: 'existing' | 'manual') => (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={`toggle ${labels[field as FieldKey]} mode`}
      title={selection.modes[field] === 'csv_column' ? t('modals.import_csv.switch_to_manual') : t('modals.import_csv.switch_to_csv')}
      onClick={() => toggleMode(field, next)}
    >
      <ArrowLeftRight className="size-4" />
    </Button>
  )

  const fieldRow = (label: string, required: boolean, control: React.ReactNode, toggle?: React.ReactNode) => (
    <div className="flex items-center gap-2">
      <span className="w-32 shrink-0 text-sm">
        {label}
        {required ? ' *' : ''}
      </span>
      <div className="min-w-0 flex-1">{control}</div>
      {toggle}
    </div>
  )

  const chunkCount = analysis ? Math.max(1, Math.ceil(analysis.rows.length / CHUNK_SIZE)) : 0

  const handleSubmit = async () => {
    if (!analysis || !selectionValid(selection)) return
    setSubmitting(true)
    try {
      const result = await runImport(analysis, selection, importTransactionList, (done, total) => setProgress({ done, total }))
      await queryClient.invalidateQueries()
      onClose()
      onComplete(result)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && !submitting && onClose()} title={t('modals.import_csv.header')}>
      <div className="flex flex-col gap-3">
        {!file ? (
          <div className="flex flex-col gap-1.5">
            <Input type="file" accept=".csv" aria-label={t('modals.import_csv.file.label')} onChange={(e) => void handleFile(e.target.files?.[0])} />
            <p className="text-xs text-muted-foreground">{t('modals.import_csv.file.hint')}</p>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-sm">{file.name}</span>
            <Button type="button" variant="secondary" size="sm" onClick={reset}>
              {t('elements.button.change.label')}
            </Button>
          </div>
        )}

        {analysis && analysis.header.length > 0 ? (
          <div className="flex flex-col gap-2">
            <p className="text-xs text-muted-foreground">{t('modals.import_csv.mapping.description')}</p>

            {fieldRow(
              labels.account,
              true,
              selection.modes.account === 'csv_column' ? (
                columnSelect('account', selection.columns.account)
              ) : (
                <select
                  aria-label={labels.account}
                  className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                  value={selection.fixed.accountId ?? ''}
                  onChange={(e) => patchFixed({ accountId: e.target.value === '' ? null : e.target.value })}
                >
                  <option value="">{t('modals.import_csv.none')}</option>
                  {writableAccounts.map((account) => (
                    <option key={account.id} value={account.id}>
                      {`${account.name} (${account.balance} ${account.currency.code})`}
                    </option>
                  ))}
                </select>
              ),
              modeToggle('account', 'existing'),
            )}

            {fieldRow(
              labels.date,
              true,
              selection.modes.date === 'csv_column' ? (
                columnSelect('date', selection.columns.date)
              ) : (
                <Input
                  aria-label={labels.date}
                  placeholder="YYYY-MM-DD"
                  value={selection.fixed.date}
                  onChange={(e) => patchFixed({ date: e.target.value })}
                />
              ),
              modeToggle('date', 'manual'),
            )}

            {selection.amountMode === 'single' ? (
              fieldRow(
                labels.amount,
                true,
                columnSelect('amount', selection.columns.amount),
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="toggle amount mode"
                  title={t('modals.import_csv.amount_mode.switch_to_dual')}
                  onClick={() => setSelection((prev) => ({ ...prev, amountMode: 'dual' }))}
                >
                  <Split className="size-4" />
                </Button>,
              )
            ) : (
              <>
                {fieldRow(
                  labels.amountInflow,
                  true,
                  columnSelect('amountInflow', selection.columns.amountInflow),
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label="toggle amount mode"
                    title={t('modals.import_csv.amount_mode.switch_to_single')}
                    onClick={() => setSelection((prev) => ({ ...prev, amountMode: 'single' }))}
                  >
                    <Split className="size-4" />
                  </Button>,
                )}
                {fieldRow(labels.amountOutflow, true, columnSelect('amountOutflow', selection.columns.amountOutflow))}
              </>
            )}

            {fieldRow(
              labels.category,
              false,
              selection.modes.category === 'csv_column'
                ? columnSelect('category', selection.columns.category)
                : entitySelect('categoryId', labels.category, categories),
              modeToggle('category', 'existing'),
            )}

            {fieldRow(
              labels.description,
              false,
              selection.modes.description === 'csv_column' ? (
                columnSelect('description', selection.columns.description)
              ) : (
                <Input
                  aria-label={labels.description}
                  placeholder={t('modals.import_csv.fields.description_placeholder')}
                  value={selection.fixed.description}
                  onChange={(e) => patchFixed({ description: e.target.value })}
                />
              ),
              modeToggle('description', 'manual'),
            )}

            {fieldRow(
              labels.payee,
              false,
              selection.modes.payee === 'csv_column'
                ? columnSelect('payee', selection.columns.payee)
                : entitySelect('payeeId', labels.payee, payees),
              modeToggle('payee', 'existing'),
            )}

            {fieldRow(
              labels.tag,
              false,
              selection.modes.tag === 'csv_column'
                ? columnSelect('tag', selection.columns.tag)
                : entitySelect('tagId', labels.tag, tags),
              modeToggle('tag', 'existing'),
            )}
          </div>
        ) : null}

        {submitting && progress && chunkCount > 1 ? (
          <Progress value={(progress.done / progress.total) * 100} aria-label="import progress" />
        ) : null}

        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" disabled={submitting} onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="button" disabled={!analysis || !selectionValid(selection) || submitting} onClick={() => void handleSubmit()}>
            {t('elements.button.import.label')}
          </Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
