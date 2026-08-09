import { useEffect, useState } from 'react'
import { ArrowUpDown, ChevronLeft, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Textarea } from '@/components/ui/textarea'
import { CalculatorInput } from '@/components/CalculatorInput'
import { amountCardInputClass, CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { calendarLocale } from '@/lib/calendarLocale'
import { formatDate, parseDateTime, formatDateTime, dayKey } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { isNotEmpty, isValidDecimalNumber, isValidFormula, isValidNumber, isValidCategoryName, isValidPayeeName } from '@/lib/validation'
import { tryNormalize } from '@/lib/decimal'
import { useUiStore } from '@/app/uiStore'
import type { OpenTransactionParams } from '@/app/uiStore'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import {
  useCategories,
  useLabels,
  usePayees,
  useTags,
  useCreateCategory,
  useCreatePayee,
} from '@/features/classifications/queries'
import { canWriteToAccount } from '@/features/connections/shared'
import { useExchange } from '@/features/currencies/useExchange'
import { usePostRecurring } from '@/features/recurring/queries'
import { useUserData } from '@/features/user/queries'
import { useCreateTransaction, useUpdateTransaction } from './queries'
import {
  useTransactionForm,
  buildPayload,
  scrubForeignClassifications,
  accountOptions,
  categoryOptions,
  canChangeAccountData,
  classificationChips,
  evaluatedAmount,
  toggleClassification,
} from './useTransactionForm'
import { ClassificationChips } from './ClassificationChips'
import { EntitySelect } from './EntitySelect'
import { SelectCard } from './SelectCard'
import { TagDialog } from '@/features/classifications/TagDialog'
import type { TransactionType } from '@/api/dto/transaction'

const TYPE_ORDER: TransactionType[] = ['income', 'transfer', 'expense']

function TransactionForm({ params, onDone }: { params: OpenTransactionParams; onDone: () => void }) {
  const { t, i18n } = useTranslation()
  const { id: routeAccountId } = useParams()
  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const { data: user } = useUserData()
  const exchangeFn = useExchange()
  const setSwitchAccountPrompt = useUiStore((s) => s.setSwitchAccountPrompt)

  const createTransaction = useCreateTransaction()
  const updateTransaction = useUpdateTransaction()
  const postRecurring = usePostRecurring()
  const createCategory = useCreateCategory()
  const createPayee = useCreatePayee()

  const { form, patch, setType, account, accountRecipient, recomputeRecipientAmount, swapAccounts } = useTransactionForm(
    params,
    accounts,
    routeAccountId ?? null,
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [addTagOpen, setAddTagOpen] = useState(false)
  const [dateOpen, setDateOpen] = useState(false)

  const isTransfer = form.type === 'transfer'
  const isExpense = form.type === 'expense'
  const ownerId = account?.owner.id
  const canEditData = canChangeAccountData(account, user?.id)
  const crossCurrency = isTransfer && account && accountRecipient && account.currency.id !== accountRecipient.currency.id

  // posting a recurring template starts with an empty recipient amount
  // (Task 15's initialFormState); prefill it once from the current rates,
  // same as a fresh cross-currency transfer would compute on entry
  useEffect(() => {
    if (params.postRecurring && crossCurrency && form.amountRecipient === '') {
      patch({ amountRecipient: recomputeRecipientAmount(form.amount, account, accountRecipient, exchangeFn) })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // A transaction stored under an older, laxer server rule can reference the
  // CALLER's classifications instead of the account owner's; the server now
  // rejects those, so every save would fail. Clear them once the lists load so
  // the form shows — and submits — only what the owner actually owns.
  useEffect(() => {
    if (!ownerId) {
      return
    }
    const scrubbed = scrubForeignClassifications(form, { categories, payees, tags, labels }, ownerId)
    if (
      scrubbed.categoryId !== form.categoryId ||
      scrubbed.payeeId !== form.payeeId ||
      scrubbed.tagId !== form.tagId ||
      scrubbed.labelIds.length !== form.labelIds.length
    ) {
      patch(scrubbed)
    }
  }, [ownerId, categories, payees, tags, labels, form, patch])

  const selectableAccounts = accountOptions(accounts, folders, form.isNew)
  const currentCategories = categoryOptions(categories, form.type, ownerId)
  const currentPayees = payees.filter((p) => p.isArchived === 0 && (!ownerId || p.ownerUserId === ownerId))
  const chips = classificationChips(tags, labels, form, ownerId)

  const setAmount = (amount: string) => {
    if (isTransfer) {
      // also when editing: a stale recipient amount would silently keep the
      // recipient account's balance unchanged (Vue recomputed unconditionally)
      patch({ amount, amountRecipient: recomputeRecipientAmount(amount, account, accountRecipient, exchangeFn) })
    } else {
      patch({ amount })
    }
  }

  const setRecipientAccount = (id: string | null) => {
    const recipient = accounts.find((a) => a.id === id)
    patch({
      accountRecipientId: id,
      // also when editing: the saved recipient amount is for the OLD
      // destination (and possibly its currency), so re-derive it
      amountRecipient: recomputeRecipientAmount(form.amount, account, recipient, exchangeFn),
    })
  }

  const setSenderAccount = (id: string | null) => {
    const sender = accounts.find((a) => a.id === id)
    patch({
      accountId: id,
      // the recipient amount is derived from the SENDER's currency too — a
      // stale value would silently mis-fund the recipient on a currency change
      amountRecipient: recomputeRecipientAmount(form.amount, sender, accountRecipient, exchangeFn),
    })
  }

  const amountErrors = (raw: string, withFormula: boolean): string | null => {
    if (!isNotEmpty(raw)) {
      return t('common.validation.required_field')
    }
    if (withFormula) {
      if (!isValidFormula(raw)) {
        return t('common.validation.invalid_formula')
      }
      const evaluated = evaluatedAmount(raw)
      if (tryNormalize(evaluated) === null) {
        return t('common.validation.invalid_number')
      }
      return null
    }
    if (!isValidNumber(raw)) {
      return t('common.validation.invalid_number')
    }
    if (!isValidDecimalNumber(raw)) {
      return t('common.validation.invalid_decimal_number')
    }
    return null
  }

  const validate = (): boolean => {
    const next: Record<string, string> = {}
    const amountError = amountErrors(form.amount, true)
    if (amountError) {
      next.amount = amountError
    }
    if (crossCurrency) {
      const recipientError = amountErrors(form.amountRecipient, false)
      if (recipientError) {
        next.amountRecipient = recipientError
      }
    }
    setErrors(next)
    return Object.keys(next).length === 0
  }

  const submit = async () => {
    if (!validate() || !form.accountId) {
      return
    }
    const payload = buildPayload(form)
    try {
      if (params.postRecurring) {
        await postRecurring.mutateAsync({ ...payload, recurringId: params.postRecurring.id })
      } else if (form.isNew) {
        await createTransaction.mutateAsync(payload)
        if (isTransfer && payload.accountRecipientId) {
          setSwitchAccountPrompt(payload.accountRecipientId)
        }
      } else {
        await updateTransaction.mutateAsync(payload)
      }
      onDone()
    } catch {
      // dialog stays open on failure
    }
  }

  const dateOnly = dayKey(form.date)
  const pending = createTransaction.isPending || updateTransaction.isPending || postRecurring.isPending
  // posting a template creates an ordinary transaction (prefilled from it), so
  // it reads as the regular add dialog rather than a mode of its own
  const title = form.isNew
    ? t('transactions.modal.create_form.header')
    : t('transactions.modal.update_form.header')

  const accountToOption = (a: (typeof accounts)[number]) => ({
    value: a.id,
    label: `${a.name} (${moneyFormat(a.balance, a.currency)})`,
    icon: a.icon,
    disabled: !canWriteToAccount(a, user?.id),
  })

  return (
    <ResponsiveDialog
      open
      caps
      fullScreen
      hideHeader
      dismissible={false}
      onOpenChange={(o) => !o && onDone()}
      title={title}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="transaction-dialog-form" disabled={pending}>
            {form.isNew ? t('common.button.add.label') : t('common.button.update.label')}
          </Button>
        </div>
      }
    >
    <form
      id="transaction-dialog-form"
      className="flex flex-col gap-4"
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      {/* the dialog header is visually hidden — the title shares one row with
          the back-a-day arrow + date chip (Vue header row) */}
      <div className="flex items-center justify-between gap-2">
        <span className="text-lg font-normal uppercase tracking-wide">{title}</span>
        <span className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-7"
            aria-label="previous day"
            onClick={() => {
              const d = parseDateTime(form.date)
              d.setHours(d.getHours() - 24)
              patch({ date: formatDateTime(d) })
            }}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <Popover open={dateOpen} onOpenChange={setDateOpen}>
            <PopoverTrigger asChild>
              <Button type="button" variant="secondary" className="h-7 rounded bg-econumo-card px-2 text-xs font-normal" aria-label="date">
                {dateOnly}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-0" align="end">
              <Calendar
                mode="single"
                weekStartsOn={1}
                locale={calendarLocale(i18n.language)}
                selected={parseDateTime(dateOnly)}
                onSelect={(day) => {
                  if (day) {
                    patch({ date: `${formatDate(day)} 00:00:00` })
                    setDateOpen(false)
                  }
                }}
              />
            </PopoverContent>
          </Popover>
        </span>
      </div>

      <div className="flex rounded-lg bg-econumo-card p-1" role="radiogroup" aria-label="type">
        {TYPE_ORDER.map((type) => (
          <button
            key={type}
            type="button"
            role="radio"
            aria-checked={form.type === type}
            className={`flex-1 rounded-md px-2 py-2 text-[13px] uppercase tracking-wide transition-colors ${
              form.type === type ? 'bg-econumo-magenta text-white' : 'text-muted-foreground hover:text-foreground'
            }`}
            onClick={() => setType(type)}
          >
            {t(`transactions.modal.transaction_type.${type}`)}
          </button>
        ))}
      </div>

      {/* Vue fuses the account and the oversized amount into one gray card */}
      <div className="flex flex-col rounded-lg bg-econumo-card px-3 py-2">
        {!isTransfer ? (
          <div className="[&_[data-slot=entity-select]]:h-auto [&_[data-slot=entity-select]]:border-0 [&_[data-slot=entity-select]]:px-0 [&_[data-slot=entity-select]]:py-1 [&_[data-slot=entity-select]]:ring-0 [&_[data-slot=entity-select]]:bg-transparent dark:[&_[data-slot=entity-select]]:bg-transparent">
            <EntitySelect
              aria-label="account"
              value={form.accountId}
              onChange={(id) => patch({ accountId: id })}
              options={selectableAccounts.map(accountToOption)}
            />
          </div>
        ) : null}
        <div className={amountCardInputClass}>
          <Label htmlFor="tx-amount" className="sr-only">
            {t('transactions.modal.form.amount.label')}
          </Label>
          <CalculatorInput id="tx-amount" autoFocus placeholder={t('transactions.modal.form.amount.label')} value={form.amount} onChange={setAmount} />
        </div>
        {errors.amount ? <p className="pb-1 text-sm text-destructive">{errors.amount}</p> : null}
      </div>

      {isTransfer ? (
        <>
          {/* Vue order: the exchanged amount lives right under the main amount, ABOVE the accounts */}
          {crossCurrency ? (
            <CardField
              label={t('transactions.modal.form.amount_recipient.label', { currency: accountRecipient?.currency.code ?? '' })}
              htmlFor="tx-amount-recipient"
              error={errors.amountRecipient}
            >
              <Input
                id="tx-amount-recipient"
                className={cardFieldControlClass}
                inputMode="decimal"
                value={form.amountRecipient}
                onChange={(e) => patch({ amountRecipient: e.target.value })}
              />
            </CardField>
          ) : null}
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-1">
              <div className="min-w-0 flex-1">
                <SelectCard label={t('transactions.modal.form.from.label')}>
                  <EntitySelect
                    aria-label="from account"
                    value={form.accountId}
                    onChange={setSenderAccount}
                    options={selectableAccounts.filter((a) => a.id !== form.accountRecipientId).map(accountToOption)}
                  />
                </SelectCard>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="text-muted-foreground"
                aria-label="swap accounts"
                onClick={() => swapAccounts(exchangeFn)}
              >
                <ArrowUpDown className="size-4" />
              </Button>
            </div>
            <SelectCard label={t('transactions.modal.form.to.label')}>
              <EntitySelect
                aria-label="to account"
                value={form.accountRecipientId}
                onChange={setRecipientAccount}
                options={selectableAccounts.filter((a) => a.id !== form.accountId).map(accountToOption)}
              />
            </SelectCard>
          </div>
        </>
      ) : (
        <>
          <SelectCard label={t('transactions.modal.form.category.label')}>
              <EntitySelect
                aria-label={t('transactions.modal.form.category.label')}
                value={form.categoryId}
                onChange={(id) => patch({ categoryId: id })}
                options={currentCategories.map((c) => ({ value: c.id, label: c.name, icon: c.icon || 'pending' }))}
                clearable
                onCreate={
                  canEditData
                    ? (name) => {
                        createCategory.mutate(
                          { name, type: form.type as 'expense' | 'income', accountId: form.accountId ?? undefined, ownerUserId: ownerId },
                          { onSuccess: (item) => patch({ categoryId: item.id }) },
                        )
                      }
                    : undefined
                }
                createValidator={isValidCategoryName}
              />
          </SelectCard>

          <SelectCard label={t(`transactions.modal.form.payee.${form.type}`)}>
              <EntitySelect
                aria-label={t(`transactions.modal.form.payee.${form.type}`)}
                value={form.payeeId}
                onChange={(id) => patch({ payeeId: id })}
                options={currentPayees.map((p) => ({ value: p.id, label: p.name }))}
                clearable
                onCreate={
                  canEditData
                    ? (name) => {
                        createPayee.mutate(
                          { name, accountId: form.accountId ?? undefined, ownerUserId: ownerId },
                          { onSuccess: (item) => patch({ payeeId: item.id }) },
                        )
                      }
                    : undefined
                }
                createValidator={isValidPayeeName}
              />
          </SelectCard>

          {isExpense ? (
            <CardField label={t('accounts.page.preview_transaction_modal.tags.label')}>
              <div className="flex items-center gap-2">
                <ClassificationChips chips={chips} onToggle={(chip) => patch(toggleClassification(form, chip.kind, chip.id))} />
                {canEditData ? (
                  <button
                    type="button"
                    aria-label={t('classifications.tags.forms.tag.add_button')}
                    title={t('common.button.add.label')}
                    className="shrink-0 text-muted-foreground hover:text-foreground"
                    onClick={() => setAddTagOpen(true)}
                  >
                    <Plus className="size-4" />
                  </button>
                ) : null}
              </div>
            </CardField>
          ) : null}
        </>
      )}

      <CardField label={t('transactions.modal.form.description.label')} htmlFor="tx-description">
        <Textarea
          id="tx-description"
          className={`${cardFieldControlClass} min-h-16 resize-none`}
          placeholder={t('transactions.modal.form.description.placeholder')}
          value={form.description}
          onChange={(e) => patch({ description: e.target.value })}
        />
      </CardField>
    </form>

      <TagDialog
        open={addTagOpen}
        onClose={() => setAddTagOpen(false)}
        accountId={form.accountId ?? undefined}
        ownerUserId={ownerId}
        onCreated={(kind, item) => {
          // the create hooks resolve an existing name to that row instead of
          // creating a duplicate, so the id may already be attached
          patch(kind === 'tag' ? { tagId: item.id } : { labelIds: form.labelIds.includes(item.id) ? form.labelIds : [...form.labelIds, item.id] })
        }}
      />
    </ResponsiveDialog>
  )
}

export function TransactionDialog() {
  const params = useUiStore((s) => s.transactionModal)
  const close = useUiStore((s) => s.closeTransactionModal)

  if (!params) {
    return null
  }

  return <TransactionForm params={params} onDone={close} />
}
