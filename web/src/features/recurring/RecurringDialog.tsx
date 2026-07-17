import { useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Textarea } from '@/components/ui/textarea'
import { CalculatorInput } from '@/components/CalculatorInput'
import { amountCardInputClass, CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { dayKey, formatDate, parseDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { isNotEmpty, isValidFormula } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import type { OpenRecurringParams } from '@/app/uiStore'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { accountOptions, categoryOptions, evaluatedNumber } from '@/features/transactions/useTransactionForm'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import type { TransactionType } from '@/api/dto/transaction'
import type { RecurringSchedule } from '@/api/dto/recurring'
import { useCreateRecurring, useUpdateRecurring } from './queries'
import { useRecurringForm, buildRecurringPayload } from './useRecurringForm'

const TYPE_ORDER: TransactionType[] = ['income', 'transfer', 'expense']
const SCHEDULE_ORDER: RecurringSchedule[] = ['weekly', 'biweekly', 'monthly', 'quarterly', 'yearly']

// strips the EntitySelect trigger button down so the CardField carries the chrome
const cardSelectClass =
  '[&_button]:h-auto [&_button]:w-full [&_button]:border-0 [&_button]:bg-transparent [&_button]:p-0 [&_button]:font-normal [&_button]:shadow-none'

// same whole-card tap target as TransactionDialog's SelectCard
function SelectCard({ label, error, children }: { label: string; error?: string | null; children: ReactNode }) {
  const wasOpen = useRef(false)
  const trigger = (root: HTMLElement) => root.querySelector<HTMLButtonElement>('button[role="combobox"]')
  return (
    <div
      className="cursor-pointer"
      onPointerDownCapture={(e) => {
        wasOpen.current = trigger(e.currentTarget)?.getAttribute('aria-expanded') === 'true'
      }}
      onClick={(e) => {
        if (wasOpen.current || (e.target as HTMLElement).closest('button')) {
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

function RecurringForm({ params, onDone }: { params: OpenRecurringParams; onDone: () => void }) {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()

  const createRecurring = useCreateRecurring()
  const updateRecurring = useUpdateRecurring()

  const { form, patch, setType, account } = useRecurringForm(params, accounts)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [dateOpen, setDateOpen] = useState(false)

  const isTransfer = form.type === 'transfer'
  const isExpense = form.type === 'expense'
  const ownerId = account?.owner.id

  const selectableAccounts = accountOptions(accounts, folders, form.isNew)
  const currentCategories = categoryOptions(categories, form.type, ownerId)
  const currentPayees = payees.filter((p) => p.isArchived === 0 && (!ownerId || p.ownerUserId === ownerId))
  const currentTags = tags.filter((tg) => tg.isArchived === 0 && (!ownerId || tg.ownerUserId === ownerId))

  const accountToOption = (a: (typeof accounts)[number]) => ({
    value: a.id,
    label: `${a.name} (${moneyFormat(a.balance, a.currency)})`,
    icon: a.icon,
  })

  const validate = (): boolean => {
    const next: Record<string, string> = {}
    if (!isNotEmpty(form.amount)) {
      next.amount = t('common.validation.required_field')
    } else if (!isValidFormula(form.amount)) {
      next.amount = t('common.validation.invalid_formula')
    } else if (Number.isNaN(evaluatedNumber(form.amount))) {
      next.amount = t('common.validation.invalid_number')
    }
    if (isTransfer && !form.accountRecipientId) {
      next.accountRecipientId = t('common.validation.required_field')
    }
    setErrors(next)
    return Object.keys(next).length === 0
  }

  const submit = async () => {
    if (!validate() || !form.accountId) {
      return
    }
    const payload = buildRecurringPayload(form)
    try {
      if (form.isNew) {
        await createRecurring.mutateAsync(payload)
      } else {
        await updateRecurring.mutateAsync(payload)
      }
      onDone()
    } catch {
      // dialog stays open on failure
    }
  }

  const pending = createRecurring.isPending || updateRecurring.isPending
  const title = form.isNew ? t('recurring.modal.create_form.header') : t('recurring.modal.update_form.header')

  return (
    <ResponsiveDialog
      open
      caps
      fullScreen
      dismissible={false}
      onOpenChange={(o) => !o && onDone()}
      title={title}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="recurring-dialog-form" disabled={pending}>
            {form.isNew ? t('common.button.add.label') : t('common.button.update.label')}
          </Button>
        </div>
      }
    >
      <form
        id="recurring-dialog-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
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

        <div className="flex flex-col rounded-lg bg-econumo-card px-3 py-2">
          {!isTransfer ? (
            <div className="[&_button]:h-auto [&_button]:border-0 [&_button]:bg-transparent [&_button]:px-0 [&_button]:py-1 [&_button]:shadow-none">
              <EntitySelect
                aria-label="account"
                value={form.accountId}
                onChange={(id) => patch({ accountId: id })}
                options={selectableAccounts.map(accountToOption)}
              />
            </div>
          ) : null}
          <div className={amountCardInputClass}>
            <Label htmlFor="rt-amount" className="sr-only">
              {t('transactions.modal.form.amount.label')}
            </Label>
            <CalculatorInput
              id="rt-amount"
              autoFocus
              placeholder={t('transactions.modal.form.amount.label')}
              value={form.amount}
              onChange={(amount) => patch({ amount })}
            />
          </div>
          {errors.amount ? <p className="pb-1 text-sm text-destructive">{errors.amount}</p> : null}
        </div>

        {isTransfer ? (
          <div className="flex flex-col gap-2">
            <SelectCard label={t('transactions.modal.form.from.label')}>
              <EntitySelect
                aria-label="from account"
                value={form.accountId}
                onChange={(id) => patch({ accountId: id })}
                options={selectableAccounts.filter((a) => a.id !== form.accountRecipientId).map(accountToOption)}
              />
            </SelectCard>
            <SelectCard label={t('transactions.modal.form.to.label')} error={errors.accountRecipientId}>
              <EntitySelect
                aria-label="to account"
                value={form.accountRecipientId}
                onChange={(id) => patch({ accountRecipientId: id })}
                options={selectableAccounts.filter((a) => a.id !== form.accountId).map(accountToOption)}
              />
            </SelectCard>
          </div>
        ) : (
          <>
            <SelectCard label={t('transactions.modal.form.category.label')}>
              <EntitySelect
                aria-label={t('transactions.modal.form.category.label')}
                value={form.categoryId}
                onChange={(id) => patch({ categoryId: id })}
                options={currentCategories.map((c) => ({ value: c.id, label: c.name, icon: c.icon || 'pending' }))}
              />
            </SelectCard>

            <SelectCard label={t(`transactions.modal.form.payee.${form.type}`)}>
              <EntitySelect
                aria-label={t(`transactions.modal.form.payee.${form.type}`)}
                value={form.payeeId}
                onChange={(id) => patch({ payeeId: id })}
                options={currentPayees.map((p) => ({ value: p.id, label: p.name }))}
                clearable
              />
            </SelectCard>

            {isExpense ? (
              <SelectCard label={t('accounts.page.preview_transaction_modal.tags.label')}>
                <EntitySelect
                  aria-label={t('accounts.page.preview_transaction_modal.tags.label')}
                  value={form.tagId}
                  onChange={(id) => patch({ tagId: id })}
                  options={currentTags.map((tg) => ({ value: tg.id, label: tg.name }))}
                  clearable
                />
              </SelectCard>
            ) : null}
          </>
        )}

        <CardField label={t('recurring.modal.form.schedule.label')} htmlFor="rt-schedule">
          <div className="[&_button]:h-auto [&_button]:w-full [&_button]:border-0 [&_button]:bg-transparent [&_button]:p-0 [&_button]:text-sm [&_button]:shadow-none [&_button]:focus-visible:ring-0">
            <Select value={form.schedule} onValueChange={(v) => patch({ schedule: v as RecurringSchedule })}>
              <SelectTrigger id="rt-schedule" aria-label={t('recurring.modal.form.schedule.label')} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SCHEDULE_ORDER.map((schedule) => (
                  <SelectItem key={schedule} value={schedule}>
                    {t(`recurring.schedule.${schedule}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardField>

        <CardField label={t('recurring.modal.form.next_payment.label')}>
          <Popover open={dateOpen} onOpenChange={setDateOpen}>
            <PopoverTrigger asChild>
              <button
                type="button"
                aria-label={t('recurring.modal.form.next_payment.label')}
                className="w-full text-left text-sm"
              >
                {dayKey(form.nextPaymentAt)}
              </button>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-0" align="start">
              <Calendar
                mode="single"
                weekStartsOn={1}
                selected={parseDateTime(dayKey(form.nextPaymentAt))}
                onSelect={(day) => {
                  if (day) {
                    patch({ nextPaymentAt: `${formatDate(day)} 00:00:00` })
                    setDateOpen(false)
                  }
                }}
              />
            </PopoverContent>
          </Popover>
        </CardField>

        <CardField label={t('transactions.modal.form.description.label')} htmlFor="rt-description">
          <Textarea
            id="rt-description"
            className={`${cardFieldControlClass} min-h-16 resize-none`}
            placeholder={t('transactions.modal.form.description.placeholder')}
            value={form.description}
            onChange={(e) => patch({ description: e.target.value })}
          />
        </CardField>
      </form>
    </ResponsiveDialog>
  )
}

export function RecurringDialog() {
  const params = useUiStore((s) => s.recurringModal)
  const close = useUiStore((s) => s.closeRecurringModal)

  if (!params) {
    return null
  }

  return <RecurringForm params={params} onDone={close} />
}
