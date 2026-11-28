import { useId, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'
import { pluralPick } from '@/lib/plural'
import { METRICS, trackEvent } from '@/lib/metrics'
import { cn } from '@/lib/utils'
import { formatDay, formatShortDay, formatSigned, useAccountView, type AccountView } from '../accountView'
import { parseAmountCell, type DecimalSeparator } from '../amounts'
import { summarize, type PreparedRow, type RowFix, type StepId } from '../prepare'
import { existingCodes, writableAccounts, type StepBodyProps } from '../wizard'
import { PerRowAccounts } from './AccountStep'
import { useNameResolution, type NameInFile, type ResolvedNames } from '../ownerLists'

// rows shown before "show more"; rows that will be left out are always shown, so none needs hunting for
const PAGE = 100

type SetFix = (update: (fix: RowFix | undefined) => RowFix | undefined) => void

/** a value in the list that opens the step that shapes it */
function StepLink({ step, goTo, className, label, children }: { step: StepId; goTo: (step: StepId) => void; className?: string; label?: string; children: ReactNode }) {
  return (
    <button type="button" aria-label={label} onClick={() => goTo(step)} className={cn('min-w-0 cursor-pointer text-left underline-offset-2 hover:underline', className)}>
      {children}
    </button>
  )
}

function TransactionRow({ row, names, account, perRow, goTo }: {
  row: PreparedRow
  /** what of the row's names lands: a name the import leaves out is not shown */
  names: ResolvedNames | undefined
  account: AccountView
  perRow: boolean
  goTo: (step: StepId) => void
}) {
  const { t, i18n } = useTranslation()
  const signed = row.amount === null ? null : formatSigned(row.amount, account.currency)
  const extras = names ? [names.payee, names.category, names.tag, ...names.labels].filter((v) => v !== '') : []
  const details = [...(perRow && account.name ? [account.name] : []), ...extras]
  const fullDate = row.date === null ? '' : [formatDay(row.date, i18n.language), row.time].filter(Boolean).join(' ')
  return (
    <li className="flex items-center gap-3 border-b border-border px-3 py-2.5 last:border-0" title={row.raw}>
      <StepLink step="date" goTo={goTo} label={fullDate} className="w-14 shrink-0 text-xs whitespace-nowrap text-muted-foreground">
        {row.date === null ? null : formatShortDay(row.date, i18n.language)}
      </StepLink>
      <div className="flex min-w-0 flex-1 flex-col items-start">
        <StepLink step="description" goTo={goTo} className="max-w-full truncate text-sm">
          {row.description || <span className="text-muted-foreground">{t('transactions.import_wizard.preview.no_description')}</span>}
        </StepLink>
        {details.length > 0 ? (
          <StepLink step={extras.length > 0 ? 'extras' : 'account'} goTo={goTo} className="max-w-full truncate text-xs text-muted-foreground">
            {details.join(' · ')}
          </StepLink>
        ) : null}
      </div>
      {signed ? (
        <StepLink step="amount" goTo={goTo} className={cn('shrink-0 text-sm tabular-nums', signed.income && 'text-income')}>
          {signed.text}
        </StepLink>
      ) : null}
    </li>
  )
}

// read the way the file's amounts are read, so "1.234,50" means the same here as in the file
function AmountFix({ sep, onSave }: { sep: DecimalSeparator; onSave: (amount: string) => void }) {
  const { t } = useTranslation()
  const id = useId()
  const errorId = useId()
  const [value, setValue] = useState('')
  const parsed = parseAmountCell(value, sep)
  const unreadable = value.trim() !== '' && parsed === null
  const save = () => {
    if (parsed !== null) onSave(parsed)
  }
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-xs text-muted-foreground">
        {t('transactions.import_wizard.preview.fix_amount')}
      </label>
      <Input
        id={id}
        inputMode="decimal"
        className="h-9 w-44 bg-background"
        value={value}
        aria-invalid={unreadable || undefined}
        aria-describedby={unreadable ? errorId : undefined}
        onChange={(e) => setValue(e.target.value)}
        onBlur={save}
        onKeyDown={(e) => {
          if (e.key === 'Enter') save()
        }}
      />
      {unreadable ? (
        <span id={errorId} className="text-xs text-destructive">
          {t('transactions.import_wizard.preview.amount_unreadable')}
        </span>
      ) : null}
    </div>
  )
}

function ProblemRow({ row, sep, setFix }: { row: PreparedRow; sep: DecimalSeparator; setFix: SetFix }) {
  const { t } = useTranslation()
  const dateId = useId()
  const typed = (patch: { date?: string; amount?: string }) =>
    setFix((fix) => {
      const next = { ...(fix === 'skip' ? {} : fix), ...patch }
      return next.date || next.amount ? next : undefined
    })
  const canFix = row.reason === 'no_date' || row.reason === 'no_amount'
  return (
    <li className="flex flex-col gap-2 border-b border-border bg-econumo-yellow/15 px-3 py-2.5 last:border-0">
      <div className="flex items-start gap-3">
        <span className="mt-0.5 shrink-0 rounded-md bg-econumo-yellow/40 px-1.5 text-xs text-econumo-yellow-text">
          {t('transactions.import_wizard.preview.left_out')}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm">
            {t('transactions.import_wizard.preview.row', { n: row.fileRow })}
            {row.description ? `: ${row.description}` : null}
          </div>
          <div className="text-xs text-muted-foreground">{t(`transactions.import_wizard.skipped.${row.reason ?? 'user_skipped'}`)}</div>
          <code className="mt-1 block font-mono text-xs break-all text-foreground/80">
            <span className="sr-only">{t('transactions.import_wizard.preview.from_file')} </span>
            {row.raw}
          </code>
        </div>
      </div>
      {canFix || row.reason === 'user_skipped' ? (
        <div className="flex flex-wrap items-end gap-2 pl-0 sm:pl-[4.25rem]">
          {row.reason === 'no_date' ? (
            <div className="flex flex-col gap-1">
              <label htmlFor={dateId} className="text-xs text-muted-foreground">
                {t('transactions.import_wizard.preview.fix_date')}
              </label>
              <Input id={dateId} type="date" className="h-9 w-44 bg-background" onChange={(e) => typed({ date: e.target.value || undefined })} />
            </div>
          ) : null}
          {row.reason === 'no_amount' ? <AmountFix sep={sep} onSave={(amount) => typed({ amount })} /> : null}
          {canFix ? (
            <Button type="button" variant="ghost" className="h-9" onClick={() => setFix(() => 'skip')}>
              {t('transactions.import_wizard.preview.skip_row')}
            </Button>
          ) : (
            <Button type="button" variant="ghost" className="h-9" onClick={() => setFix(() => undefined)}>
              {t('transactions.import_wizard.preview.unskip_row')}
            </Button>
          )}
        </div>
      ) : null}
    </li>
  )
}

export function PreviewStep({ state, setAnswers, setFixes, goTo }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { rows, answers } = state
  const accountView = useAccountView(answers)
  const resolution = useNameResolution(rows, answers)
  const summary = useMemo(() => summarize(rows), [rows])
  const [limit, setLimit] = useState(PAGE)
  const perRow = answers.account?.kind === 'column'
  const visible = useMemo(() => rows.filter((row, i) => i < limit || row.status === 'skipped'), [rows, limit])
  const existingNames = useMemo(() => existingCodes(writableAccounts(accounts, user?.id)), [accounts, user?.id])
  const count = summary.total - summary.skipped

  const setFix = (fileRow: number): SetFix => (update) =>
    setFixes((prev) => {
      const next = new Map(prev)
      const fix = update(prev.get(fileRow))
      if (fix === undefined) next.delete(fileRow)
      else next.set(fileRow, fix)
      return next
    })

  const looksWrong = () => {
    trackEvent(METRICS.TRANSACTION_IMPORT_LOOKS_WRONG)
    goTo(state.isExport ? 'account' : 'date')
  }

  const reasons = Object.entries(summary.byReason)
    .map(([reason, n]) => `${t(`transactions.import_wizard.skipped.${reason}`)}: ${n}`)
    .join(', ')
  // the names the Category and Payee lists show: distinct, among the rows that will be imported
  const nameTile = (label: string, names: NameInFile[]) => {
    const created = names.filter((n) => n.outcome === 'new').length
    let note = '—'
    if (names.length > 0) note = created > 0 ? pluralPick(t('transactions.import_wizard.preview.new_count'), created, i18n.language) : t('transactions.import_wizard.preview.all_existing')
    return { label, value: String(names.length), note }
  }
  const tiles: { label: string; value: string; note?: string }[] = [
    { label: t('transactions.import_wizard.preview.transactions'), value: String(count) },
    nameTile(t('transactions.import_wizard.preview.categories'), resolution.names.category),
    nameTile(t('transactions.import_wizard.preview.payees'), resolution.names.payee),
  ]
  const hiddenCount = rows.length - visible.length

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-3 gap-2">
        {tiles.map((tile) => (
          <div key={tile.label} className="min-w-0 rounded-xl bg-econumo-card px-3 py-2.5">
            <div className="truncate text-xs text-muted-foreground">{tile.label}</div>
            <div className="mt-0.5 truncate text-base tabular-nums sm:text-lg">{tile.value}</div>
            {tile.note ? <div className="truncate text-xs text-muted-foreground">{tile.note}</div> : null}
          </div>
        ))}
      </div>
      <div className="flex flex-col gap-0.5 text-sm text-muted-foreground">
        <p>
          {count > 0 && summary.firstDate && summary.lastDate
            ? t('transactions.import_wizard.preview.range', { first: formatDay(summary.firstDate, i18n.language), last: formatDay(summary.lastDate, i18n.language) })
            : t('transactions.import_wizard.preview.nothing')}
        </p>
        {summary.skipped > 0 ? (
          <p>
            {pluralPick(t('transactions.import_wizard.preview.skipped'), summary.skipped, i18n.language)} ({reasons})
          </p>
        ) : null}
      </div>

      {answers.account?.kind === 'column' ? (
        <PerRowAccounts
          file={state.file}
          account={answers.account}
          existing={existingNames}
          onChange={(account) => setAnswers('account', { account })}
        />
      ) : null}

      {visible.length > 0 ? (
        <ul aria-label={t('transactions.import_wizard.preview.list')} className="overflow-hidden rounded-xl border border-border">
          {visible.map((row) =>
            row.status === 'ok' ? (
              <TransactionRow
                key={row.fileRow}
                row={row}
                names={resolution.byRow.get(row.fileRow)}
                account={accountView(row)}
                perRow={perRow}
                goTo={goTo}
              />
            ) : (
              <ProblemRow key={row.fileRow} row={row} sep={answers.amount.sep} setFix={setFix(row.fileRow)} />
            ),
          )}
        </ul>
      ) : null}
      {hiddenCount > 0 ? (
        <button type="button" className="self-start text-sm text-primary underline-offset-2 hover:underline" onClick={() => setLimit((n) => n + PAGE * 5)}>
          {pluralPick(t('transactions.import_wizard.preview.show_more'), hiddenCount, i18n.language)}
        </button>
      ) : null}

      <button type="button" className="self-start text-sm text-primary underline-offset-2 hover:underline" onClick={looksWrong}>
        {t('transactions.import_wizard.preview.looks_wrong')}
      </button>
    </div>
  )
}
