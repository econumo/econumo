import { useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ArrowUpDown, ChevronLeft, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Textarea } from '@/components/ui/textarea'
import { CalculatorInput } from '@/components/CalculatorInput'
import { amountCardInputClass, CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { formatDate, parseDateTime, formatDateTime, dayKey } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { isNotEmpty, isValidDecimalNumber, isValidFormula, isValidNumber, isValidCategoryName, isValidPayeeName } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import type { OpenTransactionParams } from '@/app/uiStore'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useCategories, usePayees, useTags, useCreateCategory, useCreatePayee, useCreateTag } from '@/features/classifications/queries'
import { useExchange } from '@/features/currencies/useExchange'
import { useUserData } from '@/features/user/queries'
import { useCreateTransaction, useUpdateTransaction } from './queries'
import { useTransactionForm, buildPayload, accountOptions, categoryOptions, canChangeAccountData, evaluatedNumber } from './useTransactionForm'
import { EntitySelect } from './EntitySelect'
import { AddTagDialog } from './AddTagDialog'
import type { TransactionType } from '@/api/dto/transaction'

const TYPE_ORDER: TransactionType[] = ['income', 'transfer', 'expense']

// strips the EntitySelect field down so the CardField carries the chrome
const cardSelectClass =
  '[&_[data-slot=entity-select]]:h-auto [&_[data-slot=entity-select]]:border-0 [&_[data-slot=entity-select]]:px-0 [&_[data-slot=entity-select]]:ring-0 [&_[data-slot=entity-select]]:bg-transparent dark:[&_[data-slot=entity-select]]:bg-transparent'

// CardField around an EntitySelect where the WHOLE card is the tap target —
// clicks on the label/padding forward to the field inside
function SelectCard({ label, error, children }: { label: string; error?: string | null; children: ReactNode }) {
  // if the picker was open at pointerdown, that press already dismissed it —
  // forwarding the click would immediately reopen
  const wasOpen = useRef(false)
  const trigger = (root: HTMLElement) => root.querySelector<HTMLInputElement>('[data-slot=entity-select] input')
  return (
    <div
      className="cursor-pointer"
      onPointerDownCapture={(e) => {
        wasOpen.current = trigger(e.currentTarget)?.getAttribute('aria-expanded') === 'true'
      }}
      onClick={(e) => {
        if (wasOpen.current || (e.target as HTMLElement).closest('input, button')) {
          return
        }
        trigger(e.currentTarget)?.click()
      }}
    >
      <CardField label={label} error={error}>
        <div className={cardSelectClass}>{children}</div>
      </CardField>
    </div>
  )
}

function TransactionForm({ params, onDone }: { params: OpenTransactionParams; onDone: () => void }) {
  const { t } = useTranslation()
  const { id: routeAccountId } = useParams()
  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: user } = useUserData()
  const exchangeFn = useExchange()
  const setSwitchAccountPrompt = useUiStore((s) => s.setSwitchAccountPrompt)

  const createTransaction = useCreateTransaction()
  const updateTransaction = useUpdateTransaction()
  const createCategory = useCreateCategory()
  const createPayee = useCreatePayee()
  const createTag = useCreateTag()

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

  const selectableAccounts = accountOptions(accounts, folders, form.isNew)
  const currentCategories = categoryOptions(categories, form.type, ownerId)
  const currentPayees = payees.filter((p) => p.isArchived === 0 && (!ownerId || p.ownerUserId === ownerId))
  const selectedTag = tags.find((tag) => tag.id === form.tagId)
  const visibleTags = tags.filter((tag) => tag.isArchived === 0 && (!ownerId || tag.ownerUserId === ownerId))
  const tagRow = selectedTag && !visibleTags.some((tg) => tg.id === selectedTag.id) ? [...visibleTags, selectedTag] : visibleTags

  const crossCurrency = isTransfer && account && accountRecipient && account.currency.id !== accountRecipient.currency.id

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

  const amountErrors = (raw: string, withFormula: boolean): string | null => {
    if (!isNotEmpty(raw)) {
      return t('elements.validation.required_field')
    }
    if (withFormula) {
      if (!isValidFormula(raw)) {
        return t('elements.validation.invalid_formula')
      }
      const evaluated = evaluatedNumber(raw)
      if (Number.isNaN(evaluated)) {
        return t('elements.validation.invalid_number')
      }
      return null
    }
    if (!isValidNumber(raw)) {
      return t('elements.validation.invalid_number')
    }
    if (!isValidDecimalNumber(raw)) {
      return t('elements.validation.invalid_decimal_number')
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
    if (!isTransfer && !form.categoryId) {
      next.category = t('modals.transaction.form.category.validation.required_field')
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
      if (form.isNew) {
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
  const pending = createTransaction.isPending || updateTransaction.isPending
  const title = form.isNew ? t('modals.transaction.create_form.header') : t('modals.transaction.update_form.header')

  const accountToOption = (a: (typeof accounts)[number]) => ({
    value: a.id,
    label: `${a.name} (${moneyFormat(a.balance, a.currency)})`,
    icon: a.icon,
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
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit" form="transaction-dialog-form" disabled={pending}>
            {form.isNew ? t('elements.button.add.label') : t('elements.button.update.label')}
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
            {t(`modals.transaction.transaction_type.${type}`)}
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
            {t('modals.transaction.form.amount.label')}
          </Label>
          <CalculatorInput id="tx-amount" autoFocus placeholder={t('modals.transaction.form.amount.label')} value={form.amount} onChange={setAmount} />
        </div>
        {errors.amount ? <p className="pb-1 text-sm text-destructive">{errors.amount}</p> : null}
      </div>

      {isTransfer ? (
        <>
          {/* Vue order: the exchanged amount lives right under the main amount, ABOVE the accounts */}
          {crossCurrency ? (
            <CardField
              label={t('modals.transaction.form.amount_recipient.label', { currency: accountRecipient?.currency.code ?? '' })}
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
                <SelectCard label={t('modals.transaction.form.from.label')}>
                  <EntitySelect
                    aria-label="from account"
                    value={form.accountId}
                    onChange={(id) => patch({ accountId: id })}
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
            <SelectCard label={t('modals.transaction.form.to.label')}>
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
          <SelectCard label={t('modals.transaction.form.category.label')} error={errors.category}>
              <EntitySelect
                aria-label={t('modals.transaction.form.category.label')}
                value={form.categoryId}
                onChange={(id) => patch({ categoryId: id })}
                options={currentCategories.map((c) => ({ value: c.id, label: c.name, icon: c.icon || 'pending' }))}
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

          <SelectCard label={t(`modals.transaction.form.payee.${form.type}`)}>
              <EntitySelect
                aria-label={t(`modals.transaction.form.payee.${form.type}`)}
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
            <CardField label={t('pages.account.preview_transaction_modal.tags.label')}>
              <div className="flex items-center gap-2">
                <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5 py-0.5">
                  {tagRow.map((tag) => {
                    const toggleTag = () => patch({ tagId: form.tagId === tag.id ? null : tag.id })
                    return (
                      <Badge
                        key={tag.id}
                        role="checkbox"
                        aria-checked={form.tagId === tag.id}
                        aria-label={tag.name}
                        tabIndex={0}
                        variant={form.tagId === tag.id ? 'default' : 'secondary'}
                        className="cursor-pointer"
                        onClick={toggleTag}
                        onKeyDown={(e) => {
                          if (!e.repeat && (e.key === 'Enter' || e.key === ' ')) {
                            e.preventDefault()
                            toggleTag()
                          }
                        }}
                      >
                        {tag.name}
                      </Badge>
                    )
                  })}
                </div>
                {canEditData ? (
                  <button
                    type="button"
                    aria-label="add tag"
                    title={t('elements.button.add.label')}
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

      <CardField label={t('modals.transaction.form.description.label')} htmlFor="tx-description">
        <Textarea
          id="tx-description"
          className={`${cardFieldControlClass} min-h-16 resize-none`}
          placeholder={t('modals.transaction.form.description.placeholder')}
          value={form.description}
          onChange={(e) => patch({ description: e.target.value })}
        />
      </CardField>
    </form>

      <AddTagDialog
        open={addTagOpen}
        onClose={() => setAddTagOpen(false)}
        onSubmit={(name) => {
          createTag.mutate(
            { name, accountId: form.accountId ?? undefined, ownerUserId: ownerId },
            {
              onSuccess: (item) => {
                patch({ tagId: item.id })
                setAddTagOpen(false)
              },
            },
          )
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
