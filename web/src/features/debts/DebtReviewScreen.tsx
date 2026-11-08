import { useId, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight, MoreVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Progress } from '@/components/ui/progress'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { EntityIcon } from '@/components/EntityIcon'
import { PromptDialog } from '@/components/PromptDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { add } from '@/lib/decimal'
import { isNotEmpty } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import type { DebtDetailsDto, DebtDto, UpdateDebtDto } from '@/api/dto/debt'
import { useCurrencies } from '@/features/currencies/queries'
import { debtDay, debtMonth } from './debtDates'
import { DebtTermsForm } from './DebtTermsForm'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import type { PaymentsFromErrors } from './PaymentsFromEditor'
import { useDeleteDebt, useDebt, useFetchDebtForEdit, useMarkDebtPaidOff, useUpdateDebt } from './queries'
import { buildTermsPayload, initialTermsState } from './useDebtForm'
import type { PaymentsFrom } from './useDebtForm'
import { useDebtMoney, usePaymentsLeft } from './useDebtMoney'

const FILTER_KEYS = ['categoryId', 'payeeId', 'tagId', 'labelId'] as const

function SectionHeading({ id, children, action }: { id?: string; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="mb-2 flex items-center justify-between gap-3">
      <h3 id={id} className="text-sm uppercase tracking-wide text-muted-foreground">
        {children}
      </h3>
      {action}
    </div>
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

const filtersOf = (debt: DebtDto): PaymentsFrom => ({ categoryId: debt.categoryId, payeeId: debt.payeeId, tagId: debt.tagId, labelId: debt.labelId })

/** the debt as stored, ready to send back: for a set-up debt the stored snapshot, so
 *  a name or filter change writes no new one */
const storedPayload = (debt: DebtDto): UpdateDebtDto => buildTermsPayload(initialTermsState(debt), initialTermsState(debt), debt)

type Confirm = 'paid-off' | 'delete' | 'rename' | null
type Terms = { mode: 'setup' | 'edit'; debt: DebtDto }

function DebtReview({ debtId, onClose }: { debtId: string; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const { data, error: loadError } = useDebt(debtId)
  const fetchDebtForEdit = useFetchDebtForEdit()
  const markPaidOff = useMarkDebtPaidOff()
  const deleteDebt = useDeleteDebt()
  const updateDebt = useUpdateDebt()
  const [terms, setTerms] = useState<Terms | null>(null)
  const [confirm, setConfirm] = useState<Confirm>(null)
  // the server's own wording of what deleting drops, once it has refused
  const [removalMessage, setRemovalMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const item = data?.item

  const openTerms = (mode: Terms['mode']) => {
    setError(null)
    // afresh: the form starts from what is owed now, and a cached balance from
    // before a payment would store a snapshot that loses that payment's principal
    fetchDebtForEdit(debtId)
      .then((fresh) => setTerms({ mode, debt: fresh.item }))
      .catch((err: unknown) => setError(apiErrorMessage(err)))
  }

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

  const menuLabel = t('debts.review.menu')
  const menu =
    item && !terms ? (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="ghost" size="icon-sm" aria-label={menuLabel} title={menuLabel}>
            <MoreVertical className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-44">
          <DropdownMenuItem onSelect={() => setConfirm('rename')}>{t('debts.review.rename')}</DropdownMenuItem>
          {item.isSetUp === 1 && item.isPaidOff === 0 ? (
            <DropdownMenuItem disabled={markPaidOff.isPending} onSelect={() => setConfirm('paid-off')}>
              {t('debts.details.mark_paid_off')}
            </DropdownMenuItem>
          ) : null}
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirm('delete')}>
            {t('common.button.delete.label')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    ) : undefined

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={item?.name ?? t('debts.form.header')}
      titleIcon={item ? <EntityIcon name={item.icon} className="text-xl text-muted-foreground" /> : undefined}
      headerAction={menu}
      fullScreen="always"
    >
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-5 pb-2">
        {item && data ? (
          terms ? (
            <DebtTermsForm
              key={`${terms.debt.id}:${terms.debt.isSetUp}`}
              debt={terms.debt}
              mode={terms.mode}
              onSaved={() => setTerms(null)}
              onCancel={() => setTerms(null)}
            />
          ) : (
            <ReviewContent details={data} lang={i18n.language} onFinishSetup={() => openTerms('setup')} onEdit={() => openTerms('edit')} />
          )
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

      {item ? (
        <PromptDialog
          open={confirm === 'rename'}
          onClose={() => setConfirm(null)}
          title={t('debts.review.rename')}
          inputLabel={t('debts.form.name.label')}
          initialValue={item.name}
          validate={(value) => (isNotEmpty(value.trim()) ? null : t('common.validation.required_field'))}
          onSubmit={(value) => {
            setConfirm(null)
            setError(null)
            updateDebt.mutate({ ...storedPayload(item), name: value.trim() }, { onError: (err) => setError(apiErrorMessage(err)) })
          }}
          submitLabel={t('common.button.save.label')}
          cancelLabel={t('common.button.cancel.label')}
        />
      ) : null}
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

function ReviewContent({ details, lang, onFinishSetup, onEdit }: { details: DebtDetailsDto; lang: string; onFinishSetup: () => void; onEdit: () => void }) {
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

  // a debt not set up sends its figures empty: the top shows what was paid instead
  const paidSoFar = payments.reduce((sum, p) => add(sum, p.amount), '0')
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
                <Progress value={(item.periodsPaid / total) * 100} aria-label={t('debts.review.progress', { paid: item.periodsPaid, total })} className="h-2" />
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
        <SectionHeading
          id={loanHeadingId}
          action={
            setUp ? (
              <Button type="button" variant="ghost" size="sm" className="text-econumo-link" onClick={onEdit}>
                {t('common.button.edit.label')}
              </Button>
            ) : null
          }
        >
          {t('debts.review.loan')}
        </SectionHeading>
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

      <FiltersEditor debt={item} />

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

/** Which payments count, edited in place: a Save shows once a pick differs from the debt */
function FiltersEditor({ debt }: { debt: DebtDto }) {
  const { t } = useTranslation()
  const updateDebt = useUpdateDebt()
  const stored = filtersOf(debt)
  const [draft, setDraft] = useState<PaymentsFrom | null>(null)
  const [errors, setErrors] = useState<PaymentsFromErrors>({})
  const [serverError, setServerError] = useState<string | null>(null)
  const value = draft ?? stored
  const changed = FILTER_KEYS.some((key) => value[key] !== stored[key])

  const save = () => {
    setServerError(null)
    updateDebt.mutate(
      // the category is required: its picker has no "any" row
      { ...storedPayload(debt), ...value, categoryId: value.categoryId ?? debt.categoryId },
      {
        onSuccess: () => {
          setDraft(null)
          setErrors({})
        },
        onError: (err) => {
          const named = Object.fromEntries(FILTER_KEYS.map((key) => [key, apiFieldErrors(err, key)?.[0]])) as PaymentsFromErrors
          setErrors(named)
          if (!Object.values(named).some(Boolean)) {
            setServerError(apiErrorMessage(err))
          }
        },
      },
    )
  }

  return (
    <section className="flex flex-col gap-2">
      <PaymentsFromEditor
        value={value}
        onChange={(next) => {
          setErrors({})
          setServerError(null)
          setDraft(next)
        }}
        errors={errors}
      />
      {serverError ? (
        <p role="alert" className="text-sm text-destructive">
          {serverError}
        </p>
      ) : null}
      {changed ? (
        <div className="grid grid-cols-2 gap-3 [&_button]:h-11">
          <Button
            type="button"
            variant="secondary"
            onClick={() => {
              setDraft(null)
              setErrors({})
              setServerError(null)
            }}
          >
            {t('common.button.cancel.label')}
          </Button>
          <Button type="button" disabled={updateDebt.isPending} onClick={save}>
            {t('common.button.save.label')}
          </Button>
        </div>
      ) : null}
    </section>
  )
}

/** The owner's full-screen review of one debt, opened with `useUiStore().openDebtDetails(id)` */
export function DebtReviewScreen() {
  const debtId = useUiStore((s) => s.debtDetailsId)
  const close = useUiStore((s) => s.closeDebtDetails)

  if (!debtId) {
    return null
  }

  return <DebtReview key={debtId} debtId={debtId} onClose={close} />
}
