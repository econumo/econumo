import { useCallback, useEffect, useId, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CoinLoader } from '@/components/CoinLoader'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { EntityIcon } from '@/components/EntityIcon'
import { OfflineNotice } from '@/components/OfflineNotice'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate } from '@/lib/datetime'
import { pluralPick } from '@/lib/plural'
import { add, isNegative, sub } from '@/lib/decimal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useUiStore } from '@/app/uiStore'
import type { DebtDetailsTab } from '@/app/uiStore'
import type { DebtDetailsDto, DebtDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { useCurrencies } from '@/features/currencies/queries'
import { debtDay } from './debtDates'
import { DebtBalanceTab } from './DebtBalanceTab'
import { DebtSettingsForm } from './DebtSettingsForm'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import { useDebt, useDebtForecast, useFetchDebtForEdit } from './queries'
import { useDebtDangerActions } from './useDebtDangerActions'
import { useDebtMoney, usePaymentsLeft } from './useDebtMoney'

function SectionHeading({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <h3 id={id} className="mb-2 text-sm uppercase tracking-wide text-muted-foreground">
      {children}
    </h3>
  )
}

function Figure({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1.5">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="text-right text-sm">{children}</dd>
    </div>
  )
}

function DangerAction({ description, children }: { description: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <p className="text-sm text-muted-foreground">{description}</p>
      <div className="shrink-0 [&_button]:h-11 [&_button]:w-full sm:[&_button]:w-auto">{children}</div>
    </div>
  )
}

/** the irreversible actions, on a tab of their own; each asks first */
function DangerZone({
  canMarkPaidOff,
  markingPaidOff,
  onMarkPaidOff,
  onDelete,
}: {
  canMarkPaidOff: boolean
  markingPaidOff: boolean
  onMarkPaidOff: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  // both need the server: offline they stay listed, greyed out
  const online = useOnline()
  // the tab names the zone: the card needs no heading of its own
  return (
    <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-1">
      <div className="divide-y divide-destructive/20">
        {canMarkPaidOff ? (
          <DangerAction description={t('debts.review.danger.paid_off')}>
            <Button type="button" variant="secondary" disabled={markingPaidOff || !online} onClick={onMarkPaidOff}>
              {t('debts.details.mark_paid_off')}
            </Button>
          </DangerAction>
        ) : null}
        <DangerAction description={t('debts.review.danger.delete')}>
          <Button type="button" variant="destructive" disabled={!online} onClick={onDelete}>
            {t('common.button.delete.label')}
          </Button>
        </DangerAction>
      </div>
    </div>
  )
}

// underlined like the Reports tabs: the selected tab carries a 2px rule as
// wide as its label, overlapping the header block's 1px border (-mb-px)
const tabClass =
  '-mb-px h-auto flex-none rounded-none border-0 border-b-2 border-transparent px-0 py-2.5 text-sm font-normal text-muted-foreground shadow-none after:hidden hover:text-foreground focus-visible:ring-0 focus-visible:outline-none focus-visible:underline data-active:border-foreground data-active:bg-transparent data-active:text-foreground dark:data-active:border-foreground dark:data-active:bg-transparent'

// Balance (reconciling with a statement) needs the loan's facts: a debt not
// set up has no such tab
const TABS: { value: DebtDetailsTab; label: string; setUpOnly?: boolean }[] = [
  { value: 'overview', label: 'debts.review.tab.overview' },
  { value: 'balance', label: 'debts.review.tab.balance', setUpOnly: true },
  { value: 'settings', label: 'debts.review.tab.settings' },
  { value: 'danger', label: 'debts.review.danger.title' },
]

function DebtReview({ debtId, budgetId, initialTab, onClose }: { debtId: string; budgetId: Id | null; initialTab: DebtDetailsTab; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const { data, error: loadError } = useDebt(debtId)
  const money = useDebtMoney()
  const fetchDebtForEdit = useFetchDebtForEdit()
  const [tab, setTab] = useState<DebtDetailsTab>(initialTab)
  // the debt the Settings form was built from, loaded afresh: a cached copy
  // could send back facts another device has corrected since
  const [editDebt, setEditDebt] = useState<DebtDto | null>(null)
  const [formKey, setFormKey] = useState(0)
  // a reload on its way replaces the form: Finish setup focuses the new one
  const [reloading, setReloading] = useState(false)
  const dirty = useRef(false)
  const [focusMissing, setFocusMissing] = useState(initialTab === 'settings')
  const [error, setError] = useState<string | null>(null)

  const item = data?.item

  // Each visit to Settings reloads the debt, unless the form holds edits: those
  // stay across a look at the Overview
  const loadSettings = useCallback(() => {
    if (dirty.current) {
      return
    }
    setError(null)
    setReloading(true)
    fetchDebtForEdit(debtId)
      .then((fresh) => {
        if (!dirty.current) {
          setEditDebt(fresh.item)
          setFormKey((k) => k + 1)
        }
      })
      .catch((err: unknown) => setError(apiErrorMessage(err)))
      .finally(() => setReloading(false))
  }, [debtId, fetchDebtForEdit])

  const opened = useRef(false)
  useEffect(() => {
    if (!opened.current && initialTab === 'settings') {
      opened.current = true
      loadSettings()
    }
  }, [initialTab, loadSettings])

  const switchTab = (next: DebtDetailsTab) => {
    setTab(next)
    if (next === 'settings') {
      loadSettings()
    }
  }

  // a write from Settings (a save, Mark as paid off) rebuilds the form from the
  // debt as stored now, so a later edit cannot send the terms from before it,
  // and shows its result on the Overview
  const showSaved = (saved: DebtDto) => {
    dirty.current = false
    setEditDebt(saved)
    setFormKey((k) => k + 1)
    setTab('overview')
  }

  const onDirtyChange = useCallback((value: boolean) => {
    dirty.current = value
  }, [])

  const danger = useDebtDangerActions({ onPaidOff: showSaved, onDeleted: onClose, onError: setError })

  // the tab bar sits in the dialog's header block, outside the scrolling body;
  // the Tabs root wraps the dialog so its context reaches both through the portal
  const tabBar = item ? (
    <div className="w-full">
      <TabsList variant="line" className="h-auto w-full items-end justify-start gap-5 rounded-none bg-transparent p-0">
        {TABS.filter((tab) => !tab.setUpOnly || item.isSetUp === 1).map(({ value, label }) => (
          <TabsTrigger key={value} value={value} className={tabClass}>
            {t(label)}
          </TabsTrigger>
        ))}
      </TabsList>
    </div>
  ) : undefined

  return (
    <Tabs value={tab} onValueChange={(v) => switchTab(v as DebtDetailsTab)} className="contents">
      <ResponsiveDialog
        open
        onOpenChange={(o) => !o && onClose()}
        title={item?.name ?? t('debts.form.header')}
        titleIcon={item ? <EntityIcon name={item.icon} className="text-xl text-muted-foreground" /> : undefined}
        subheader={tabBar}
        fullScreen
        size="column"
      >
        <div className="flex w-full flex-col gap-5 pb-2">
          {item && data ? (
            <>
              <TabsContent value="overview" className="flex flex-col gap-5">
                <ReviewContent
                  details={data}
                  budgetId={budgetId}
                  lang={i18n.language}
                  onFinishSetup={() => {
                    setFocusMissing(true)
                    switchTab('settings')
                  }}
                />
              </TabsContent>
              {item.isSetUp === 1 ? (
                <TabsContent value="balance" className="flex flex-col gap-5">
                  <DebtBalanceTab details={data} amount={(value) => money(value, item)} lang={i18n.language} />
                </TabsContent>
              ) : null}
              {/* kept mounted: edits survive a look at the Overview */}
              <TabsContent value="settings" forceMount hidden={tab !== 'settings'} className="flex flex-col gap-6">
                {editDebt ? (
                  <DebtSettingsForm
                    key={formKey}
                    debt={editDebt}
                    onDirtyChange={onDirtyChange}
                    focusMissing={focusMissing && tab === 'settings' && !reloading}
                    onFocused={() => setFocusMissing(false)}
                    onSaved={showSaved}
                  />
                ) : error ? null : (
                  <div className="flex justify-center py-12">
                    <CoinLoader />
                  </div>
                )}
              </TabsContent>
              <TabsContent value="danger" className="flex flex-col gap-5">
                <OfflineNotice />
                <DangerZone
                  canMarkPaidOff={item.isSetUp === 1 && item.isPaidOff === 0}
                  markingPaidOff={danger.markingPaidOff}
                  onMarkPaidOff={() => {
                    setError(null)
                    danger.askMarkPaidOff(debtId)
                  }}
                  onDelete={() => {
                    setError(null)
                    danger.askDelete(debtId)
                  }}
                />
              </TabsContent>
            </>
          ) : null}
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          {!data && loadError ? (
            <p role="alert" className="text-sm text-destructive">
              {apiErrorMessage(loadError)}
            </p>
          ) : null}
        </div>

        {danger.dialogs}
      </ResponsiveDialog>
    </Tabs>
  )
}

interface PaymentRow {
  key: string
  date: string
  payment: string
  interest: string
  principal: string
  balance: string
}

// a long forecast (a mortgage runs to hundreds of payments) keeps its payoff
// end and the payments nearest today; the middle opens on request
const FORECAST_FOLD_ABOVE = 24
const FORECAST_KEEP_END = 3
const FORECAST_KEEP_NEAR = 12

function PaymentCells({ row, splits, amount, lang, overdue }: { row: PaymentRow; splits: boolean; amount: (value: string) => string; lang: string; overdue?: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <td className="py-1.5 text-left">
        {debtDay(row.date, lang)}
        {overdue ? <span className="ml-1.5 text-xs text-muted-foreground">{t('debts.details.forecast.overdue')}</span> : null}
      </td>
      <td className="py-1.5 text-right">{amount(row.payment)}</td>
      {splits ? (
        <>
          <td className="py-1.5 text-right">{amount(row.interest)}</td>
          <td className="hidden py-1.5 text-right sm:table-cell">{amount(row.principal)}</td>
          <td className="py-1.5 text-right">{row.balance === '' ? '—' : amount(row.balance)}</td>
        </>
      ) : null}
    </>
  )
}

/** One table, newest first: the projected payments (when shown) on top, latest
 *  first down to the next due, muted and italic, then a "Recorded" line and
 *  the booked payments (not "Today": a booked payment may be dated ahead, and
 *  an overdue projected due behind). `splits` adds the interest, principal and balance columns,
 *  which need the terms; Principal (payment less interest) is left out on a
 *  phone, where five amounts do not fit */
function PaymentsTable({ booked, projected, splits, amount, lang, today }: {
  booked: PaymentRow[]
  /** soonest first, as the server sends them; null while not shown */
  projected: PaymentRow[] | null
  splits: boolean
  amount: (value: string) => string
  lang: string
  /** "YYYY-MM-DD": a projected due before it is still unpaid */
  today: string
}) {
  const { t, i18n } = useTranslation()
  const [unfolded, setUnfolded] = useState(false)
  const columns = splits ? 5 : 2
  const head = 'py-1 text-right font-normal'
  const latestFirst = projected ? [...projected].reverse() : []
  const folded = !unfolded && latestFirst.length > FORECAST_FOLD_ABOVE
  const hidden = folded ? latestFirst.length - FORECAST_KEEP_END - FORECAST_KEEP_NEAR : 0
  const projectedRow = (row: PaymentRow) => (
    <tr key={row.key} data-testid="debt-forecast-payment" className="border-t border-border text-muted-foreground italic">
      <PaymentCells row={row} splits={splits} amount={amount} lang={lang} overdue={row.date.slice(0, 10) < today} />
    </tr>
  )
  return (
    <div className="rounded-lg bg-econumo-card px-4 py-1.5">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-xs text-muted-foreground">
            <th scope="col" className="py-1 text-left font-normal">
              {t('debts.details.column.date')}
            </th>
            <th scope="col" className={head}>
              {t('debts.details.column.amount')}
            </th>
            {splits ? (
              <>
                <th scope="col" className={head}>
                  {t('debts.details.column.interest')}
                </th>
                <th scope="col" className={`${head} hidden sm:table-cell`}>
                  {t('debts.details.column.principal')}
                </th>
                <th scope="col" className={head}>
                  {t('debts.details.column.balance')}
                </th>
              </>
            ) : null}
          </tr>
        </thead>
        <tbody>
          {folded ? (
            <>
              {latestFirst.slice(0, FORECAST_KEEP_END).map(projectedRow)}
              <tr className="border-t border-border">
                <td colSpan={columns} className="py-1 text-center">
                  <button type="button" className="text-xs text-econumo-link hover:underline" onClick={() => setUnfolded(true)}>
                    {pluralPick(t('debts.details.forecast.more'), hidden, i18n.language)}
                  </button>
                </td>
              </tr>
              {latestFirst.slice(latestFirst.length - FORECAST_KEEP_NEAR).map(projectedRow)}
            </>
          ) : (
            latestFirst.map(projectedRow)
          )}
          {projected ? (
            <tr data-testid="debt-payments-recorded" className="border-t border-foreground/30">
              <td colSpan={columns} className="pt-1 pb-0.5 text-xs uppercase tracking-wide text-muted-foreground">
                {t('debts.details.forecast.recorded')}
              </td>
            </tr>
          ) : null}
          {booked.map((row) => (
            <tr key={row.key} data-testid="debt-payment" className="border-t border-border">
              <PaymentCells row={row} splits={splits} amount={amount} lang={lang} />
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** The Payments heading with, for a set-up debt that is not paid off, the
 *  future-payments toggle beside it and the note of which plan they follow
 *  under it: the projected rows join the top of the table */
function PaymentsSection({ payments, splits, canForecast, forecast, amount, lang }: {
  payments: DebtDetailsDto['payments']
  splits: boolean
  canForecast: boolean
  /** the Overview's forecast, already loading for its figures: opening the
   *  list fetches nothing more */
  forecast: Pick<ReturnType<typeof useDebtForecast>, 'data' | 'error' | 'isPending'>
  amount: (value: string) => string
  lang: string
}) {
  const { t } = useTranslation()
  const headingId = useId()
  const [open, setOpen] = useState(false)
  const { data, error, isPending } = forecast
  // a view is the user opening the list, not each (background) load of it
  const toggle = () => {
    if (!open) {
      trackEvent(METRICS.DEBT_FORECAST_VIEW)
    }
    setOpen(!open)
  }
  const shown = open && canForecast && data ? data : null
  const booked = payments.map((p) => ({ key: p.transactionId, date: p.date, payment: p.amount, interest: p.interest, principal: p.principal, balance: p.balance }))
  return (
    <section aria-labelledby={headingId}>
      <div className="mb-2 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h3 id={headingId} className="text-sm uppercase tracking-wide text-muted-foreground">
          {t('debts.details.payments')}
        </h3>
        {canForecast ? (
          <button type="button" className="text-sm text-econumo-link hover:underline" aria-expanded={open} onClick={toggle}>
            {open ? t('debts.details.forecast.hide') : t('debts.details.forecast.show')}
          </button>
        ) : null}
      </div>
      {open && canForecast ? (
        isPending ? (
          <div className="flex justify-center py-3">
            <CoinLoader />
          </div>
        ) : error ? (
          <p role="alert" className="mb-2 text-sm text-destructive">
            {apiErrorMessage(error)}
          </p>
        ) : shown ? (
          <p className="mb-2 text-xs text-muted-foreground">
            {shown.budgetName ? t('debts.details.forecast.with_plan', { budget: shown.budgetName }) : t('debts.details.forecast.regular_only')}
          </p>
        ) : null
      ) : null}
      {booked.length === 0 && !shown ? (
        <p className="text-sm text-muted-foreground">{t('debts.details.payments_empty')}</p>
      ) : (
        <PaymentsTable
          // a fresh opening starts folded again
          key={shown ? 'forecast' : 'booked'}
          booked={booked}
          projected={shown ? shown.items.map((p, i) => ({ key: `f${i}-${p.date}`, ...p })) : null}
          splits={splits}
          amount={amount}
          lang={lang}
          today={formatDate(new Date())}
        />
      )}
    </section>
  )
}

function ReviewContent({ details, budgetId, lang, onFinishSetup }: { details: DebtDetailsDto; budgetId: Id | null; lang: string; onFinishSetup: () => void }) {
  const { t } = useTranslation()
  const money = useDebtMoney()
  const paymentsLeft = usePaymentsLeft()
  const { data: currencies } = useCurrencies()
  const loanHeadingId = useId()
  const { item, payments } = details
  const setUp = item.isSetUp === 1
  // the projection needs the terms, and a paid-off debt has nothing to come
  const canForecast = setUp && item.isPaidOff === 0
  // The debt's own figures ignore the budgets' plans; the forecast's follow
  // them (a planned extra pays the loan off sooner). Until it answers, or when
  // it cannot (an error, offline), the debt's own figures stand in.
  const forecast = useDebtForecast(item.id, budgetId, canForecast)
  const summary = canForecast ? forecast.data : undefined
  const left = summary ? summary.paymentsLeft : item.paymentsLeft
  const overpaymentLeft = summary ? summary.overpaymentLeft : item.overpaymentLeft
  const amount = (value: string) => money(value, item)
  const currency = currencies?.find((c) => c.id === item.currencyId)
  const schedule = t(`recurring.schedule.${item.schedule}`)

  // a debt not set up sends its figures empty: the top shows what was paid
  // instead, up to the caller's today (the list also holds future-dated ones)
  const today = formatDate(new Date())
  const paidSoFar = payments.filter((p) => p.date.slice(0, 10) <= today).reduce((sum, p) => add(sum, p.amount), '0')
  const total = item.periodsPaid + left
  const progress = t('debts.review.progress', { paid: item.periodsPaid, total })
  // the bar counts payments; the caption adds the principal repaid of the
  // amount borrowed (a balance above it, from a statement, repaid nothing)
  const repaid = sub(item.amountBorrowed || '0', item.currentBalance || '0')
  const caption =
    item.isPaidOff === 0
      ? t('debts.review.progress_repaid', { paid: item.periodsPaid, total, repaid: amount(isNegative(repaid) ? '0' : repaid), borrowed: amount(item.amountBorrowed) })
      : progress

  return (
    <>
      <section data-testid="debt-review-top" className="flex flex-col gap-3 rounded-lg bg-econumo-card px-4 py-4">
        {setUp ? (
          <>
            <p data-testid="debt-review-payments-left" className="text-3xl leading-tight">
              {item.isPaidOff === 1 ? t('debts.list.paid_off') : paymentsLeft(left)}
            </p>
            {total > 0 ? (
              <div className="flex flex-col gap-1.5">
                {/* the track is the whole loan, the filled part what is paid */}
                <Progress value={(item.periodsPaid / total) * 100} aria-label={progress} className="h-2 bg-foreground/15" />
                <span className="text-xs text-muted-foreground">{caption}</span>
              </div>
            ) : null}
            {item.isPaidOff === 1 ? <p className="text-sm text-muted-foreground">{t('debts.review.owed_now', { amount: amount(item.currentBalance) })}</p> : null}
          </>
        ) : (
          <>
            <div className="flex flex-col gap-0.5">
              <span className="text-sm text-muted-foreground">{t('debts.review.paid_so_far')}</span>
              <span className="text-3xl leading-tight">{amount(paidSoFar)}</span>
            </div>
            <p className="text-sm text-muted-foreground">{t('debts.review.not_set_up')}</p>
            <Button type="button" className="h-11" onClick={onFinishSetup}>
              {t('debts.terms.finish_setup')}
            </Button>
          </>
        )}
      </section>

      <section aria-labelledby={loanHeadingId}>
        <SectionHeading id={loanHeadingId}>{t('debts.review.loan')}</SectionHeading>
        <dl className="divide-y divide-border rounded-lg bg-econumo-card px-4 py-1.5">
          {setUp ? (
            <Figure label={t('debts.review.borrowed')}>{`${amount(item.amountBorrowed)} · ${debtDay(item.startDate, lang)}`}</Figure>
          ) : null}
          <Figure label={t('debts.form.payment.label')}>{`${amount(item.payment)} · ${schedule}`}</Figure>
          {setUp ? (
            <>
              <Figure label={t('debts.review.next_payment')}>{item.nextDueDate ? debtDay(item.nextDueDate, lang) : '—'}</Figure>
              <Figure label={t('debts.review.loan_ends')}>{item.lastPaymentDate ? debtDay(item.lastPaymentDate, lang) : '—'}</Figure>
            </>
          ) : null}
          {/* a set-up debt's start is in Borrowed */}
          {setUp ? null : (
            <Figure label={t('debts.review.start_date')}>{item.startDate ? debtDay(item.startDate, lang) : t('debts.review.from_first_payment')}</Figure>
          )}
          {/* what is owed now is the top card's; a set-up debt's amounts name its currency */}
          {setUp ? (
            <>
              <Figure label={t('debts.details.overpaid_so_far')}>{amount(item.overpaidSoFar)}</Figure>
              <Figure label={t('debts.review.overpayment_to_come')}>{amount(overpaymentLeft)}</Figure>
            </>
          ) : (
            <Figure label={t('debts.form.currency.label')}>{currency ? fullCurrencyLabel(currency) : '—'}</Figure>
          )}
        </dl>
      </section>

      <PaymentsFromEditor
        value={{ categoryId: item.categoryId, payeeId: item.payeeId, tagId: item.tagId, labelId: item.labelId }}
        onChange={() => {}}
        readOnly
      />

      <PaymentsSection payments={payments} splits={setUp} canForecast={canForecast} forecast={forecast} amount={amount} lang={lang} />

    </>
  )
}

/** The owner's review of one debt, opened with `useUiStore().openDebtDetails(id, tab?, budgetId?)`:
 *  a large dialog (full screen on a phone) with Overview, Balance (set-up debts only),
 *  Settings and Danger zone tabs */
export function DebtReviewScreen() {
  const debtId = useUiStore((s) => s.debtDetailsId)
  const tab = useUiStore((s) => s.debtDetailsTab)
  const budgetId = useUiStore((s) => s.debtDetailsBudgetId)
  const close = useUiStore((s) => s.closeDebtDetails)

  if (!debtId) {
    return null
  }

  return <DebtReview key={debtId} debtId={debtId} budgetId={budgetId} initialTab={tab} onClose={close} />
}
