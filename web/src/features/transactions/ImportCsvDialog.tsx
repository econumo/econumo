import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeftRight, Split, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { importTransactionList } from '@/api/transaction'
import { METRICS, trackEvent } from '@/lib/metrics'
import { pluralPick } from '@/lib/plural'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import type { AggregatedImportResult, CsvAnalysis, FieldKey, ImportSelection } from './importCsv'
import { analyzeCsv, autoDetect, CHUNK_SIZE, countNewLabels, defaultSelection, runImport, selectionValid } from './importCsv'

const MAX_FILE_SIZE = 10485760

const SEPARATOR_PRESETS: { value: string; display: string }[] = [
  { value: ';', display: ';' },
  { value: ',', display: ',' },
  { value: '|', display: '|' },
]
const PRESET_SEPARATOR_VALUES = [';', ',', '|', '\t', '\n']

interface ImportCsvDialogProps {
  open: boolean
  onClose: () => void
  onComplete: (result: AggregatedImportResult) => void
}

export function ImportCsvDialog({ open, onClose, onComplete }: ImportCsvDialogProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()

  const [file, setFile] = useState<File | null>(null)
  const [analysis, setAnalysis] = useState<CsvAnalysis | null>(null)
  const [selection, setSelection] = useState<ImportSelection>(defaultSelection)
  const [submitting, setSubmitting] = useState(false)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [separatorDialogOpen, setSeparatorDialogOpen] = useState(false)

  const reset = () => {
    setFile(null)
    setAnalysis(null)
    setSelection(defaultSelection())
    setSubmitting(false)
    setProgress(null)
    setSeparatorDialogOpen(false)
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
      prev.fixed.categoryId || prev.fixed.payeeId || prev.fixed.tagId || prev.fixed.labelIds.length > 0
        ? { ...prev, fixed: { ...prev.fixed, categoryId: null, payeeId: null, tagId: null, labelIds: [] } }
        : prev,
    )
  }, [targetUserId])

  const ownerLabels = useMemo(
    () => labels.filter((label) => label.ownerUserId === targetUserId && label.isArchived === 0),
    [labels, targetUserId],
  )

  // The "new labels" count matches names against ALL of the owner's labels,
  // archived included: the import resolves names server-side with no archived
  // filter, so a CSV naming an archived label attaches it rather than
  // creating a new one. ownerLabels stays archived-free for the visible list.
  const existingLabelNames = useMemo(
    () => labels.filter((label) => label.ownerUserId === targetUserId).map((label) => label.name),
    [labels, targetUserId],
  )
  // narrow deps: only the pieces countNewLabels actually reads, so typing in
  // the description/date inputs doesn't rescan every row on a 10 MB import;
  // a broad `[selection]` dep would invalidate on every unrelated keystroke,
  // same as no memo at all
  const newLabelCount = useMemo(
    // eslint-disable-next-line react-hooks/exhaustive-deps
    () => (analysis ? countNewLabels(analysis, selection, existingLabelNames) : 0),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [analysis, selection.modes.labels, selection.columns.labels, selection.labelsSeparator, existingLabelNames],
  )

  const fieldLabels: Record<FieldKey, string> = {
    account: t('transactions.import_csv.fields.account'),
    date: t('transactions.import_csv.fields.date'),
    amount: t('transactions.import_csv.fields.amount'),
    amountInflow: t('transactions.import_csv.fields.amount_inflow'),
    amountOutflow: t('transactions.import_csv.fields.amount_outflow'),
    category: t('transactions.import_csv.fields.category'),
    description: t('transactions.import_csv.fields.description'),
    payee: t('transactions.import_csv.fields.payee'),
    tag: t('transactions.import_csv.fields.tag'),
    labels: t('transactions.import_csv.fields.labels'),
  }

  const handleFile = async (picked: File | undefined) => {
    if (!picked) return
    if (picked.size > MAX_FILE_SIZE) {
      reset()
      return
    }
    const text = await picked.text()
    const parsed = analyzeCsv(text)
    const detected = autoDetect(parsed.header, fieldLabels)
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
      aria-label={fieldLabels[field]}
      className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
      value={value ?? ''}
      onChange={(e) => patchColumns({ [field]: e.target.value === '' ? null : e.target.value } as Partial<ImportSelection['columns']>)}
    >
      <option value="">{t('transactions.import_csv.none')}</option>
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
      <option value="">{t('transactions.import_csv.none')}</option>
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
      aria-label={`toggle ${fieldLabels[field as FieldKey]} mode`}
      title={selection.modes[field] === 'csv_column' ? t('transactions.import_csv.switch_to_manual') : t('transactions.import_csv.switch_to_csv')}
      onClick={() => toggleMode(field, next)}
    >
      <ArrowLeftRight className="size-4" />
    </Button>
  )

  const toggleLabelId = (id: string) =>
    patchFixed({ labelIds: selection.fixed.labelIds.includes(id) ? selection.fixed.labelIds.filter((x) => x !== id) : [...selection.fixed.labelIds, id] })

  const labelMultiSelect = (
    <div className="flex min-h-9 flex-wrap items-center gap-1.5 rounded-md border p-1.5" role="group" aria-label={fieldLabels.labels}>
      {ownerLabels.length === 0 ? <span className="px-1 text-xs text-muted-foreground">{t('transactions.import_csv.none')}</span> : null}
      {ownerLabels.map((label) => {
        const checked = selection.fixed.labelIds.includes(label.id)
        return (
          <Badge
            key={label.id}
            role="checkbox"
            aria-checked={checked}
            aria-label={label.name}
            tabIndex={0}
            variant={checked ? 'default' : 'secondary'}
            className="cursor-pointer"
            onClick={() => toggleLabelId(label.id)}
            onKeyDown={(e) => {
              if (!e.repeat && (e.key === 'Enter' || e.key === ' ')) {
                e.preventDefault()
                toggleLabelId(label.id)
              }
            }}
          >
            {label.name}
          </Badge>
        )
      })}
    </div>
  )

  const labelsSeparatorControl = (
    <div className="flex items-center gap-2">
      <div className="min-w-0 flex-1">{columnSelect('labels', selection.columns.labels)}</div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={t('transactions.import_csv.labels_separator.button')}
        title={t('transactions.import_csv.labels_separator.button')}
        onClick={() => setSeparatorDialogOpen(true)}
      >
        <SlidersHorizontal className="size-4" />
      </Button>
    </div>
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
      trackEvent(METRICS.TRANSACTION_IMPORT)
      await queryClient.invalidateQueries()
      onClose()
      onComplete(result)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && !submitting && onClose()} title={t('transactions.import_csv.header')}>
      <div className="flex flex-col gap-3">
        {!file ? (
          <div className="flex flex-col gap-1.5">
            <Input type="file" accept=".csv" aria-label={t('transactions.import_csv.file.label')} onChange={(e) => void handleFile(e.target.files?.[0])} />
            <p className="text-xs text-muted-foreground">{t('transactions.import_csv.file.hint')}</p>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-sm">{file.name}</span>
            <Button type="button" variant="secondary" size="sm" onClick={reset}>
              {t('common.button.change.label')}
            </Button>
          </div>
        )}

        {analysis && analysis.header.length > 0 ? (
          <div className="flex flex-col gap-2">
            <p className="text-xs text-muted-foreground">{t('transactions.import_csv.mapping.description')}</p>

            {fieldRow(
              fieldLabels.account,
              true,
              selection.modes.account === 'csv_column' ? (
                columnSelect('account', selection.columns.account)
              ) : (
                <select
                  aria-label={fieldLabels.account}
                  className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                  value={selection.fixed.accountId ?? ''}
                  onChange={(e) => patchFixed({ accountId: e.target.value === '' ? null : e.target.value })}
                >
                  <option value="">{t('transactions.import_csv.none')}</option>
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
              fieldLabels.date,
              true,
              selection.modes.date === 'csv_column' ? (
                columnSelect('date', selection.columns.date)
              ) : (
                <Input
                  aria-label={fieldLabels.date}
                  placeholder="YYYY-MM-DD"
                  value={selection.fixed.date}
                  onChange={(e) => patchFixed({ date: e.target.value })}
                />
              ),
              modeToggle('date', 'manual'),
            )}

            {selection.amountMode === 'single' ? (
              fieldRow(
                fieldLabels.amount,
                true,
                columnSelect('amount', selection.columns.amount),
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="toggle amount mode"
                  title={t('transactions.import_csv.amount_mode.switch_to_dual')}
                  onClick={() => setSelection((prev) => ({ ...prev, amountMode: 'dual' }))}
                >
                  <Split className="size-4" />
                </Button>,
              )
            ) : (
              <>
                {fieldRow(
                  fieldLabels.amountInflow,
                  true,
                  columnSelect('amountInflow', selection.columns.amountInflow),
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label="toggle amount mode"
                    title={t('transactions.import_csv.amount_mode.switch_to_single')}
                    onClick={() => setSelection((prev) => ({ ...prev, amountMode: 'single' }))}
                  >
                    <Split className="size-4" />
                  </Button>,
                )}
                {fieldRow(fieldLabels.amountOutflow, true, columnSelect('amountOutflow', selection.columns.amountOutflow))}
              </>
            )}

            {fieldRow(
              fieldLabels.category,
              false,
              selection.modes.category === 'csv_column'
                ? columnSelect('category', selection.columns.category)
                : entitySelect('categoryId', fieldLabels.category, categories),
              modeToggle('category', 'existing'),
            )}

            {fieldRow(
              fieldLabels.description,
              false,
              selection.modes.description === 'csv_column' ? (
                columnSelect('description', selection.columns.description)
              ) : (
                <Input
                  aria-label={fieldLabels.description}
                  placeholder={t('transactions.import_csv.fields.description_placeholder')}
                  value={selection.fixed.description}
                  onChange={(e) => patchFixed({ description: e.target.value })}
                />
              ),
              modeToggle('description', 'manual'),
            )}

            {fieldRow(
              fieldLabels.payee,
              false,
              selection.modes.payee === 'csv_column'
                ? columnSelect('payee', selection.columns.payee)
                : entitySelect('payeeId', fieldLabels.payee, payees),
              modeToggle('payee', 'existing'),
            )}

            {fieldRow(
              fieldLabels.tag,
              false,
              selection.modes.tag === 'csv_column'
                ? columnSelect('tag', selection.columns.tag)
                : entitySelect('tagId', fieldLabels.tag, tags),
              modeToggle('tag', 'existing'),
            )}

            {fieldRow(
              fieldLabels.labels,
              false,
              selection.modes.labels === 'csv_column' ? labelsSeparatorControl : labelMultiSelect,
              modeToggle('labels', 'existing'),
            )}

            {newLabelCount > 0 ? (
              <p role="status" className="text-xs text-muted-foreground">
                {pluralPick(t('transactions.import_csv.new_labels_count'), newLabelCount, i18n.language)}
              </p>
            ) : null}
          </div>
        ) : null}

        {submitting && progress && chunkCount > 1 ? (
          <Progress value={(progress.done / progress.total) * 100} aria-label="import progress" />
        ) : null}

        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" disabled={submitting} onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="button" disabled={!analysis || !selectionValid(selection) || submitting} onClick={() => void handleSubmit()}>
            {t('common.button.import.label')}
          </Button>
        </div>
      </div>

      <ResponsiveDialog
        open={separatorDialogOpen}
        onOpenChange={(o) => !o && setSeparatorDialogOpen(false)}
        title={t('transactions.import_csv.labels_separator.title')}
      >
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap gap-2" role="radiogroup" aria-label={t('transactions.import_csv.labels_separator.title')}>
            {SEPARATOR_PRESETS.map((preset) => (
              <Button
                key={preset.value}
                type="button"
                variant={selection.labelsSeparator === preset.value ? 'default' : 'secondary'}
                onClick={() => setSelection((prev) => ({ ...prev, labelsSeparator: preset.value }))}
              >
                {preset.display}
              </Button>
            ))}
            <Button
              type="button"
              variant={selection.labelsSeparator === '\t' ? 'default' : 'secondary'}
              onClick={() => setSelection((prev) => ({ ...prev, labelsSeparator: '\t' }))}
            >
              {t('transactions.import_csv.labels_separator.tab')}
            </Button>
            <Button
              type="button"
              variant={selection.labelsSeparator === '\n' ? 'default' : 'secondary'}
              onClick={() => setSelection((prev) => ({ ...prev, labelsSeparator: '\n' }))}
            >
              {t('transactions.import_csv.labels_separator.newline')}
            </Button>
          </div>
          <Input
            aria-label={t('transactions.import_csv.labels_separator.custom_label')}
            placeholder={t('transactions.import_csv.labels_separator.custom_label')}
            value={PRESET_SEPARATOR_VALUES.includes(selection.labelsSeparator) ? '' : selection.labelsSeparator}
            onChange={(e) => setSelection((prev) => ({ ...prev, labelsSeparator: e.target.value }))}
          />
          <Button type="button" className="h-11" onClick={() => setSeparatorDialogOpen(false)}>
            {t('common.button.save.label')}
          </Button>
        </div>
      </ResponsiveDialog>
    </ResponsiveDialog>
  )
}
