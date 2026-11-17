import { useEffect, useId, useMemo, useRef, useState } from 'react'
import type { ReactNode, TouchEvent } from 'react'
import { AlertTriangle, ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { abs, isNegative } from '@/lib/decimal'
import { moneyFormat, type CurrencyLike } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import { METRICS, trackEvent } from '@/lib/metrics'
import { summarize, type PreparedRow, type RowFix, type StepId, type WizardAnswers } from '../prepare'
import { nameKey, writableAccounts, type StepBodyProps } from '../wizard'
import { PerRowAccounts } from './AccountStep'
import { StepTitle } from './StepTitle'

interface AccountView {
  name: string | null
  icon: string
  currency: CurrencyLike | null
}

const SWIPE_MIN_PX = 50

// arrow keys belong to the control that has focus when it uses them itself
const ownsArrowKeys = (target: EventTarget | null) =>
  target instanceof HTMLElement &&
  (target.isContentEditable || target.matches('input, textarea, select, [role="combobox"], [role="radio"], [role="slider"], [role="listbox"], [role="option"]'))

function useAccountView(answers: WizardAnswers): (row: PreparedRow) => AccountView {
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: currencies = [] } = useCurrencies()
  const writable = useMemo(() => writableAccounts(accounts, user?.id), [accounts, user?.id])
  const { account } = answers

  return (row) => {
    if (account?.kind === 'existing') {
      const found = accounts.find((a) => a.id === account.accountId)
      return { name: found?.name ?? null, icon: found?.icon || 'account_balance_wallet', currency: found?.currency ?? null }
    }
    if (account?.kind === 'new') {
      return { name: account.name, icon: 'account_balance_wallet', currency: currencies.find((c) => c.id === account.currencyId) ?? null }
    }
    if (account?.kind === 'column' && row.account !== null) {
      const key = nameKey(row.account)
      const found = writable.find((a) => nameKey(a.name) === key)
      if (found) return { name: found.name, icon: found.icon || 'account_balance_wallet', currency: found.currency }
      const currencyId = Object.hasOwn(account.currencies, key) ? account.currencies[key] : userCurrencyId(user)
      return { name: row.account, icon: 'account_balance_wallet', currency: currencies.find((c) => c.id === currencyId) ?? null }
    }
    return { name: null, icon: 'account_balance_wallet', currency: null }
  }
}

function formatAmount(amount: string, currency: CurrencyLike | null): { sign: string; text: string } {
  return {
    sign: isNegative(amount) ? '−' : '+',
    text: moneyFormat(abs(amount), currency, { showCurrency: false, useNativePrecision: false }),
  }
}

/** a value on the card that opens the step that shapes it */
function StepLink({ step, goTo, className = '', children }: { step: StepId; goTo: (step: StepId) => void; className?: string; children: ReactNode }) {
  return (
    <button type="button" onClick={() => goTo(step)} className={`min-w-0 cursor-pointer text-left underline-offset-2 hover:underline ${className}`}>
      {children}
    </button>
  )
}

function TransactionCard({ row, account, labelsSeparator, goTo }: { row: PreparedRow; account: AccountView; labelsSeparator: string; goTo: (step: StepId) => void }) {
  const { t } = useTranslation()
  const amount = row.amount === null ? null : formatAmount(row.amount, account.currency)
  const labels = row.labels === '' ? [] : row.labels.split(labelsSeparator).map((l) => l.trim()).filter((l) => l !== '')
  return (
    <div className="flex w-full items-start gap-4">
      <span className="grid size-10 shrink-0 place-items-center rounded-full bg-background">
        <EntityIcon name="question_mark" className="text-xl text-entity-icon" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col items-start gap-1">
        <StepLink step="description" goTo={goTo} className="break-words text-base leading-6">
          {row.description || <span className="text-muted-foreground">{t('transactions.import_wizard.preview.no_description')}</span>}
        </StepLink>
        {row.category ? (
          <StepLink step="extras" goTo={goTo} className="flex items-center gap-1 text-[13px] text-muted-foreground">
            <EntityIcon name="category" className="shrink-0 text-sm" />
            <span className="truncate">{row.category}</span>
          </StepLink>
        ) : null}
        {row.payee ? (
          <StepLink step="extras" goTo={goTo} className="truncate text-[13px] text-muted-foreground">
            {row.payee}
          </StepLink>
        ) : null}
        {row.tag || labels.length > 0 ? (
          <StepLink step="extras" goTo={goTo} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] text-muted-foreground">
            {row.tag ? (
              <span className="flex min-w-0 items-center gap-1">
                <EntityIcon name="tag" className="shrink-0 text-sm" />
                <span className="truncate">{row.tag}</span>
              </span>
            ) : null}
            {labels.map((label, i) => (
              <span key={i} className="flex min-w-0 items-center gap-1">
                <EntityIcon name="label" className="shrink-0 text-sm" />
                <span className="truncate">{label}</span>
              </span>
            ))}
          </StepLink>
        ) : null}
      </span>
      <span className="flex max-w-[45%] shrink-0 flex-col items-end gap-0.5 text-right">
        {amount ? (
          <StepLink step="amount" goTo={goTo} className={`text-sm leading-6 tabular-nums ${amount.sign === '+' ? 'text-income' : 'text-expense'}`}>
            {amount.sign}
            {amount.text}
            {account.currency ? <span className="ml-1 text-muted-foreground">{account.currency.symbol}</span> : null}
          </StepLink>
        ) : null}
        <StepLink step="account" goTo={goTo} className="flex max-w-full items-center gap-1 text-[13px] text-muted-foreground">
          <EntityIcon name={account.icon} className="shrink-0 text-sm" />
          <span className="truncate">{account.name ?? t('transactions.import_wizard.preview.no_account')}</span>
        </StepLink>
        <StepLink step="date" goTo={goTo} className="text-[13px] tabular-nums text-muted-foreground">
          {row.time ? `${row.date} ${row.time}` : row.date}
        </StepLink>
      </span>
    </div>
  )
}

function AmountFix({ onSave }: { onSave: (amount: string) => void }) {
  const { t } = useTranslation()
  const id = useId()
  const [value, setValue] = useState('')
  const save = () => {
    const normalized = value.replace(/\s/g, '').replace('−', '-').replace(',', '.')
    if (normalized !== '') onSave(normalized)
  }
  return (
    <CardField label={t('transactions.import_wizard.preview.fix_amount')} htmlFor={id}>
      <Input
        id={id}
        inputMode="decimal"
        className={cardFieldControlClass}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={save}
        onKeyDown={(e) => {
          if (e.key === 'Enter') save()
        }}
      />
    </CardField>
  )
}

function ProblemCard({ row, setFix }: { row: PreparedRow; setFix: (update: (fix: RowFix | undefined) => RowFix | undefined) => void }) {
  const { t } = useTranslation()
  const dateId = useId()
  const typed = (patch: { date?: string; amount?: string }) =>
    setFix((fix) => {
      const next = { ...(fix === 'skip' ? {} : fix), ...patch }
      return next.date || next.amount ? next : undefined
    })
  const canFix = row.reason === 'no_date' || row.reason === 'no_amount'
  return (
    <div className="flex flex-col gap-3">
      <p className="flex items-center gap-2 text-sm">
        <AlertTriangle className="size-4 shrink-0 text-amber-600 dark:text-amber-400" aria-hidden />
        {t(`transactions.import_wizard.skipped.${row.reason ?? 'user_skipped'}`)}
      </p>
      {row.reason === 'no_date' ? (
        <CardField label={t('transactions.import_wizard.preview.fix_date')} htmlFor={dateId}>
          <Input id={dateId} type="date" className={cardFieldControlClass} onChange={(e) => typed({ date: e.target.value || undefined })} />
        </CardField>
      ) : null}
      {row.reason === 'no_amount' ? <AmountFix onSave={(amount) => typed({ amount })} /> : null}
      {canFix ? (
        <Button type="button" variant="secondary" className="self-start" onClick={() => setFix(() => 'skip')}>
          {t('transactions.import_wizard.preview.skip_row')}
        </Button>
      ) : null}
      {row.reason === 'user_skipped' ? (
        <Button type="button" variant="secondary" className="self-start" onClick={() => setFix(() => undefined)}>
          {t('transactions.import_wizard.preview.unskip_row')}
        </Button>
      ) : null}
    </div>
  )
}

export function PreviewStep({ state, setAnswers, setFixes, goTo }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { rows, answers } = state
  const accountView = useAccountView(answers)
  const summary = useMemo(() => summarize(rows), [rows])
  const [index, setIndex] = useState(0)
  const current = Math.min(index, Math.max(rows.length - 1, 0))
  const row = rows[current] as PreparedRow | undefined
  const rootRef = useRef<HTMLDivElement>(null)
  const touchStart = useRef<{ x: number; y: number } | null>(null)
  const positionId = useId()

  const existingNames = useMemo(() => new Set(writableAccounts(accounts, user?.id).map((a) => nameKey(a.name))), [accounts, user?.id])
  const hasProblems = summary.skipped > 0
  const count = summary.total - summary.skipped

  const move = (by: number) => setIndex(Math.min(Math.max(current + by, 0), rows.length - 1))
  const nextProblem = () => {
    for (let step = 1; step <= rows.length; step++) {
      const at = (current + step) % rows.length
      if (rows[at].status === 'skipped') {
        setIndex(at)
        return
      }
    }
  }

  const moveRef = useRef(move)
  useEffect(() => {
    moveRef.current = move
  })
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
      const dialog = rootRef.current?.closest('[role="dialog"]')
      if (!dialog || !(e.target instanceof Node) || !dialog.contains(e.target) || ownsArrowKeys(e.target)) return
      e.preventDefault()
      moveRef.current(e.key === 'ArrowRight' ? 1 : -1)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  const onTouchStart = (e: TouchEvent) => {
    const touch = e.touches[0]
    touchStart.current = touch ? { x: touch.clientX, y: touch.clientY } : null
  }
  const onTouchEnd = (e: TouchEvent) => {
    const start = touchStart.current
    const touch = e.changedTouches[0]
    touchStart.current = null
    if (!start || !touch) return
    const dx = touch.clientX - start.x
    if (Math.abs(dx) >= SWIPE_MIN_PX && Math.abs(dx) > Math.abs(touch.clientY - start.y)) move(dx < 0 ? 1 : -1)
  }

  const setFix = (fileRow: number) => (update: (fix: RowFix | undefined) => RowFix | undefined) =>
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

  // one currency for the totals only when the whole file goes to one account
  const totalsCurrency = answers.account?.kind === 'column' || !row ? null : accountView(row).currency
  const money = (value: string) => moneyFormat(value, totalsCurrency, { showCurrency: totalsCurrency !== null, useNativePrecision: false })
  const reasons = Object.entries(summary.byReason)
    .map(([reason, n]) => `${t(`transactions.import_wizard.skipped.${reason}`)}: ${n}`)
    .join(', ')

  return (
    <div ref={rootRef} className="flex flex-col gap-3">
      <StepTitle>{t('transactions.import_wizard.preview.title')}</StepTitle>
      <div className="flex flex-col gap-1 text-sm">
        <p>
          {count > 0 && summary.firstDate && summary.lastDate
            ? pluralPick(
                t('transactions.import_wizard.preview.totals', {
                  first: summary.firstDate,
                  last: summary.lastDate,
                  in: money(summary.moneyIn),
                  out: money(summary.moneyOut),
                }),
                count,
                i18n.language,
              )
            : t('transactions.import_wizard.preview.nothing')}
        </p>
        {hasProblems ? (
          <p className="text-muted-foreground">
            {pluralPick(t('transactions.import_wizard.preview.skipped'), summary.skipped, i18n.language)} ({reasons})
          </p>
        ) : null}
      </div>

      {answers.account?.kind === 'column' ? (
        <PerRowAccounts
          file={state.file}
          account={answers.account}
          existing={existingNames}
          fallbackCurrency={userCurrencyId(user) ?? ''}
          onChange={(account) => setAnswers('account', { account })}
        />
      ) : null}

      {row ? (
        <section aria-labelledby={positionId} className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h4 id={positionId} className="text-sm text-muted-foreground" aria-live="polite">
              {t('transactions.import_wizard.preview.position', { n: current + 1, total: rows.length })}
            </h4>
            <div className="flex items-center gap-2">
              {hasProblems ? (
                <Button type="button" variant="ghost" size="sm" onClick={nextProblem}>
                  {t('transactions.import_wizard.preview.next_problem')}
                </Button>
              ) : null}
              <Button type="button" variant="secondary" size="sm" disabled={current === 0} onClick={() => move(-1)}>
                <ChevronLeft className="size-4" aria-hidden />
                {t('transactions.import_wizard.preview.previous')}
              </Button>
              <Button type="button" variant="secondary" size="sm" disabled={current >= rows.length - 1} onClick={() => move(1)}>
                {t('transactions.import_wizard.preview.next')}
                <ChevronRight className="size-4" aria-hidden />
              </Button>
            </div>
          </div>
          <div className="touch-pan-y rounded-lg bg-econumo-card p-3" onTouchStart={onTouchStart} onTouchEnd={onTouchEnd}>
            {row.status === 'ok' ? (
              <TransactionCard row={row} account={accountView(row)} labelsSeparator={answers.extras.labelsSeparator || ';'} goTo={goTo} />
            ) : (
              <ProblemCard key={row.fileRow} row={row} setFix={setFix(row.fileRow)} />
            )}
          </div>
          <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
            <span>{t('transactions.import_wizard.preview.from_file')}</span>
            <code className="break-all font-mono text-foreground">{row.raw}</code>
          </div>
        </section>
      ) : null}

      <button type="button" className="self-start text-sm text-primary underline-offset-2 hover:underline" onClick={looksWrong}>
        {t('transactions.import_wizard.preview.looks_wrong')}
      </button>
    </div>
  )
}
