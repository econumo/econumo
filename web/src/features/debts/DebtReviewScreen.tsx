import { useCallback, useEffect, useId, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/datetime'
import { add } from '@/lib/decimal'
import { useUiStore } from '@/app/uiStore'
import type { DebtDetailsTab } from '@/app/uiStore'
import type { DebtDetailsDto, DebtDto } from '@/api/dto/debt'
import { useCurrencies } from '@/features/currencies/queries'
import { debtDay, debtMonth } from './debtDates'
import { DebtSettingsForm } from './DebtSettingsForm'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import { useDeleteDebt, useDebt, useFetchDebtForEdit, useMarkDebtPaidOff } from './queries'
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

type Confirm = 'paid-off' | 'delete' | null

const tabClass =
  '-mb-px h-auto flex-none rounded-none border-0 border-b-2 border-transparent px-0 py-2.5 text-sm font-normal text-muted-foreground after:hidden hover:text-foreground data-active:border-foreground data-active:text-foreground dark:data-active:border-foreground'

function DebtReview({ debtId, initialTab, onClose }: { debtId: string; initialTab: DebtDetailsTab; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const { data, error: loadError } = useDebt(debtId)
  const fetchDebtForEdit = useFetchDebtForEdit()
  const markPaidOff = useMarkDebtPaidOff()
  const deleteDebt = useDeleteDebt()
  const [tab, setTab] = useState<DebtDetailsTab>(initialTab)
  // the debt the Settings form was built from, loaded afresh: the form starts
  // from what is owed now, and a cached balance from before a payment would
  // store a snapshot that loses that payment's principal
  const [editDebt, setEditDebt] = useState<DebtDto | null>(null)
  const [formKey, setFormKey] = useState(0)
  // a reload on its way replaces the form: Finish setup focuses the new one
  const [reloading, setReloading] = useState(false)
  const dirty = useRef(false)
  const [focusMissing, setFocusMissing] = useState(initialTab === 'settings')
  const [confirm, setConfirm] = useState<Confirm>(null)
  // the server's own wording of what deleting drops, once it has refused
  const [removalMessage, setRemovalMessage] = useState<string | null>(null)
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

  const onDirtyChange = useCallback((value: boolean) => {
    dirty.current = value
  }, [])

  const sendDelete = (confirmDataRemoval: boolean) => {
    setError(null)
    deleteDebt.mutate(
      { id: debtId, ...(confirmDataRemoval ? { confirmDataRemoval: true } : {}) },
      {
        onSuccess: onClose,
        onError: (err) => {
          const refusal = apiFieldErrors(err, 'confirmDataRemoval')?.[0]
          if (!confirmDataRemoval && refusal) {
            setRemovalMessage(refusal)
            return
          }
          setRemovalMessage(null)
          setError(apiErrorMessage(err))
        },
      },
    )
  }

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={item?.name ?? t('debts.form.header')}
      titleIcon={item ? <EntityIcon name={item.icon} className="text-xl text-muted-foreground" /> : undefined}
      fullScreen
      size="large"
    >
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5 pb-2">
        {item && data ? (
          <Tabs value={tab} onValueChange={(v) => switchTab(v as DebtDetailsTab)} className="gap-4">
            {/* stays in view while the tab scrolls */}
            <div className="sticky top-0 z-10 -mx-1 bg-popover px-1 pt-1">
              {/* underlined like the Reports tabs: the selected tab carries the rule */}
              <TabsList variant="line" className="h-auto w-full justify-start gap-5 rounded-none border-b p-0">
                <TabsTrigger value="overview" className={tabClass}>
                  {t('debts.review.tab.overview')}
                </TabsTrigger>
                <TabsTrigger value="settings" className={tabClass}>
                  {t('debts.review.tab.settings')}
                </TabsTrigger>
              </TabsList>
            </div>
            <TabsContent value="overview" className="flex flex-col gap-5">
              <ReviewContent
                details={data}
                lang={i18n.language}
                onFinishSetup={() => {
                  setFocusMissing(true)
                  switchTab('settings')
                }}
              />
            </TabsContent>
            {/* kept mounted: edits survive a look at the Overview */}
            <TabsContent value="settings" forceMount hidden={tab !== 'settings'} className="flex flex-col gap-6">
              {editDebt ? (
                <DebtSettingsForm
                  key={formKey}
                  debt={editDebt}
                  onDirtyChange={onDirtyChange}
                  focusMissing={focusMissing && tab === 'settings' && !reloading}
                  onFocused={() => setFocusMissing(false)}
                  onSaved={(saved) => {
                    dirty.current = false
                    setEditDebt(saved)
                    setFormKey((k) => k + 1)
                    setTab('overview')
                  }}
                />
              ) : null}
              <div className="flex flex-col gap-3 border-t pt-5 sm:flex-row [&_button]:h-11 sm:[&_button]:flex-1">
                {item.isSetUp === 1 && item.isPaidOff === 0 ? (
                  <Button type="button" variant="secondary" disabled={markPaidOff.isPending} onClick={() => setConfirm('paid-off')}>
                    {t('debts.details.mark_paid_off')}
                  </Button>
                ) : null}
                <Button type="button" variant="destructive" onClick={() => setConfirm('delete')}>
                  {t('common.button.delete.label')}
                </Button>
              </div>
            </TabsContent>
          </Tabs>
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

      <ConfirmDialog
        open={confirm === 'paid-off'}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          setConfirm(null)
          setError(null)
          markPaidOff.mutate(debtId, { onError: (err) => setError(apiErrorMessage(err)) })
        }}
        title={t('debts.details.paid_off_confirm.title')}
        question={t('debts.details.paid_off_confirm.question')}
        confirmLabel={t('debts.details.mark_paid_off')}
        cancelLabel={t('common.button.cancel.label')}
      />
      <ConfirmDialog
        open={confirm === 'delete'}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          setConfirm(null)
          sendDelete(false)
        }}
        title={t('debts.details.delete_confirm.title')}
        question={t('debts.details.delete_confirm.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
      <ConfirmDialog
        open={removalMessage !== null}
        onClose={() => setRemovalMessage(null)}
        onConfirm={() => {
          setRemovalMessage(null)
          sendDelete(true)
        }}
        title={t('debts.details.removal_confirm.title')}
        question={removalMessage ?? ''}
        confirmLabel={t('debts.details.removal_confirm.action')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </ResponsiveDialog>
  )
}

function ReviewContent({ details, lang, onFinishSetup }: { details: DebtDetailsDto; lang: string; onFinishSetup: () => void }) {
  const { t } = useTranslation()
  const money = useDebtMoney()
  const paymentsLeft = usePaymentsLeft()
  const { data: currencies } = useCurrencies()
  const loanHeadingId = useId()
  const [historyOpen, setHistoryOpen] = useState(false)
  const { item, snapshots, payments } = details
  const setUp = item.isSetUp === 1
  const amount = (value: string) => money(value, item)
  const currency = currencies?.find((c) => c.id === item.currencyId)
  const schedule = t(`recurring.schedule.${item.schedule}`)

  // a debt not set up sends its figures empty: the top shows what was paid
  // instead, up to the caller's today (the list also holds future-dated ones)
  const today = formatDate(new Date())
  const paidSoFar = payments.filter((p) => p.date.slice(0, 10) <= today).reduce((sum, p) => add(sum, p.amount), '0')
  const total = item.periodsPaid + item.paymentsLeft
  const owed = t('debts.review.owed_now', { amount: amount(item.currentBalance) })
  const payoff = item.isPaidOff === 0 && item.payoffDate !== '' ? t('budgets.page.debts.payoff_by', { date: debtMonth(item.payoffDate, lang) }) : null

  return (
    <>
      <section data-testid="debt-review-top" className="flex flex-col gap-3 rounded-lg bg-econumo-card px-4 py-4">
        {setUp ? (
          <>
            <p data-testid="debt-review-payments-left" className="text-3xl leading-tight">
              {item.isPaidOff === 1 ? t('debts.list.paid_off') : paymentsLeft(item.paymentsLeft)}
            </p>
            {total > 0 ? (
              <div className="flex flex-col gap-1.5">
                {/* the track is the whole loan, the filled part what is paid */}
                <Progress value={(item.periodsPaid / total) * 100} aria-label={t('debts.review.progress', { paid: item.periodsPaid, total })} className="h-2 bg-foreground/15" />
                <span className="text-xs text-muted-foreground">{t('debts.review.progress', { paid: item.periodsPaid, total })}</span>
              </div>
            ) : null}
            <p className="text-sm text-muted-foreground">{payoff ? `${owed} · ${payoff}` : owed}</p>
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
          {setUp ? <Figure label={t('debts.review.owed_now_label')}>{amount(item.currentBalance)}</Figure> : null}
          <Figure label={t('debts.form.payment.label')}>{`${amount(item.payment)} · ${schedule}`}</Figure>
          {setUp ? (
            <>
              <Figure label={t('debts.review.next_payment')}>{item.nextDueDate ? debtDay(item.nextDueDate, lang) : '—'}</Figure>
              <Figure label={t('debts.review.loan_ends')}>{item.lastPaymentDate ? debtDay(item.lastPaymentDate, lang) : '—'}</Figure>
            </>
          ) : null}
          <Figure label={t('debts.review.start_date')}>{item.startDate ? debtDay(item.startDate, lang) : t('debts.review.from_first_payment')}</Figure>
          {setUp ? (
            <>
              <Figure label={t('debts.details.money_left')}>{amount(item.moneyLeft)}</Figure>
              <Figure label={t('debts.details.overpaid_so_far')}>{amount(item.overpaidSoFar)}</Figure>
              <Figure label={t('debts.review.overpayment_to_come')}>{amount(item.overpaymentLeft)}</Figure>
            </>
          ) : null}
          <Figure label={t('debts.form.currency.label')}>{currency ? fullCurrencyLabel(currency) : '—'}</Figure>
        </dl>
      </section>

      <PaymentsFromEditor
        value={{ categoryId: item.categoryId, payeeId: item.payeeId, tagId: item.tagId, labelId: item.labelId }}
        onChange={() => {}}
        readOnly
      />

      <section>
        <SectionHeading>{t('debts.details.payments')}</SectionHeading>
        {payments.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('debts.details.payments_empty')}</p>
        ) : (
          <div className="rounded-lg bg-econumo-card px-4 py-1.5">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-xs text-muted-foreground">
                  <th scope="col" className="py-1 text-left font-normal">
                    {t('debts.details.column.date')}
                  </th>
                  <th scope="col" className="py-1 text-right font-normal">
                    {t('debts.details.column.amount')}
                  </th>
                  {/* the interest split needs the terms */}
                  {setUp ? (
                    <>
                      <th scope="col" className="py-1 text-right font-normal">
                        {t('debts.details.column.interest')}
                      </th>
                      <th scope="col" className="py-1 text-right font-normal">
                        {t('debts.details.column.principal')}
                      </th>
                    </>
                  ) : null}
                </tr>
              </thead>
              <tbody>
                {payments.map((payment) => (
                  <tr key={payment.transactionId} data-testid="debt-payment" className="border-t border-border">
                    <td className="py-1.5 text-left">{debtDay(payment.date, lang)}</td>
                    <td className="py-1.5 text-right">{amount(payment.amount)}</td>
                    {setUp ? (
                      <>
                        <td className="py-1.5 text-right">{amount(payment.interest)}</td>
                        <td className="py-1.5 text-right">{amount(payment.principal)}</td>
                      </>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {snapshots.length > 0 ? (
        <section>
          <button
            type="button"
            className="mb-2 flex items-center gap-1 text-sm uppercase tracking-wide text-muted-foreground"
            aria-expanded={historyOpen}
            onClick={() => setHistoryOpen((o) => !o)}
          >
            {historyOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
            {t('debts.details.snapshots')}
          </button>
          {historyOpen ? (
            <ul className="flex flex-col gap-1.5" aria-label={t('debts.details.snapshots')}>
              {snapshots.map((snapshot, index) => (
                <li key={`${snapshot.asOf}-${index}`} className="rounded-lg bg-econumo-card px-4 py-2 text-sm" data-testid="debt-snapshot">
                  <div className="flex items-baseline justify-between gap-3">
                    <span className="text-muted-foreground">{debtDay(snapshot.asOf, lang)}</span>
                    <span>{amount(snapshot.balance)}</span>
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {t('debts.details.snapshot_terms', {
                      payment: amount(snapshot.payment),
                      schedule: t(`recurring.schedule.${snapshot.schedule}`),
                      date: debtDay(snapshot.lastPaymentDate, lang),
                    })}
                  </div>
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      ) : null}
    </>
  )
}

/** The owner's review of one debt, opened with `useUiStore().openDebtDetails(id, tab?)`:
 *  a large dialog (full screen on a phone) with an Overview and a Settings tab */
export function DebtReviewScreen() {
  const debtId = useUiStore((s) => s.debtDetailsId)
  const tab = useUiStore((s) => s.debtDetailsTab)
  const close = useUiStore((s) => s.closeDebtDetails)

  if (!debtId) {
    return null
  }

  return <DebtReview key={debtId} debtId={debtId} initialTab={tab} onClose={close} />
}
