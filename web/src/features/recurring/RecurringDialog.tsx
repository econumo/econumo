import { ChevronLeft, Plus } from 'lucide-react'
import { useState } from 'react'
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
import { calendarLocale } from '@/lib/calendarLocale'
import { dayKey, formatDate, formatDateTime, parseDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { tryNormalize } from '@/lib/decimal'
import { isNotEmpty, isValidFormula } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import type { OpenRecurringParams } from '@/app/uiStore'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useCategories, useLabels, usePayees, useTags, useCreateLabel, useCreateTag } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import {
  accountOptions,
  canChangeAccountData,
  categoryOptions,
  classificationChips,
  evaluatedAmount,
  toggleClassification,
} from '@/features/transactions/useTransactionForm'
import { AddTagDialog } from '@/features/transactions/AddTagDialog'
import { ClassificationChips } from '@/features/transactions/ClassificationChips'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { SelectCard } from '@/features/transactions/SelectCard'
import type { TransactionType } from '@/api/dto/transaction'
import type { RecurringSchedule } from '@/api/dto/recurring'
import { useCreateRecurring, useUpdateRecurring } from './queries'
import { useRecurringForm, buildRecurringPayload } from './useRecurringForm'

const TYPE_ORDER: TransactionType[] = ['income', 'transfer', 'expense']
const SCHEDULE_ORDER: RecurringSchedule[] = ['weekly', 'biweekly', 'monthly', 'quarterly', 'yearly']

function RecurringForm({ params, onDone }: { params: OpenRecurringParams; onDone: () => void }) {
  const { t, i18n } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()

  const { data: user } = useUserData()
  const createRecurring = useCreateRecurring()
  const updateRecurring = useUpdateRecurring()
  const createTag = useCreateTag()
  const createLabel = useCreateLabel()

  const { form, patch, setType, setSchedule, account } = useRecurringForm(params, accounts)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [dateOpen, setDateOpen] = useState(false)
  const [addTagOpen, setAddTagOpen] = useState(false)

  const isTransfer = form.type === 'transfer'
  const isExpense = form.type === 'expense'
  const ownerId = account?.owner.id
  const canEditData = canChangeAccountData(account, user?.id)

  const selectableAccounts = accountOptions(accounts, folders, form.isNew)
  const currentCategories = categoryOptions(categories, form.type, ownerId)
  const currentPayees = payees.filter((p) => p.isArchived === 0 && (!ownerId || p.ownerUserId === ownerId))
  const chips = classificationChips(tags, labels, form, ownerId)

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
    } else if (tryNormalize(evaluatedAmount(form.amount)) === null) {
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

  const dateOnly = dayKey(form.nextPaymentAt)
  const pending = createRecurring.isPending || updateRecurring.isPending
  // one title for both modes: this names what the form edits — the footer
  // button already distinguishes Add from Update
  const title = t('recurring.preview.header')

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
        {/* the dialog header is visually hidden — the title shares one row with
            the back-a-day arrow + date chip, exactly as in TransactionDialog.
            That chip IS the next payment, so there is no separate date row. */}
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
                const d = parseDateTime(form.nextPaymentAt)
                d.setHours(d.getHours() - 24)
                // a manual date is the user's own — stop re-deriving it from
                // the source transaction on schedule changes
                patch({ nextPaymentAt: formatDateTime(d), anchorDate: null })
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
                      patch({ nextPaymentAt: `${formatDate(day)} 00:00:00`, anchorDate: null })
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
              /* the same chip row as TransactionDialog, not a select */
              <CardField label={t('accounts.page.preview_transaction_modal.tags.label')}>
                <div className="flex items-center gap-2">
                  <ClassificationChips chips={chips} onToggle={(chip) => patch(toggleClassification(form, chip.kind, chip.id))} />
                  {canEditData ? (
                    <button
                      type="button"
                      aria-label="add tag"
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

        <CardField label={t('recurring.modal.form.schedule.label')} htmlFor="rt-schedule">
          <div className="[&_button]:h-auto [&_button]:w-full [&_button]:border-0 [&_button]:bg-transparent [&_button]:p-0 [&_button]:text-sm [&_button]:shadow-none [&_button]:focus-visible:ring-0">
            <Select value={form.schedule} onValueChange={(v) => setSchedule(v as RecurringSchedule)}>
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

      <AddTagDialog
        open={addTagOpen}
        onClose={() => setAddTagOpen(false)}
        onSubmit={(kind, name) => {
          const input = { name, accountId: form.accountId ?? undefined, ownerUserId: ownerId }
          const attach = (item: { id: string }) => {
            // the create hooks resolve an existing name to that row instead of
            // creating a duplicate, so the id may already be attached
            patch(kind === 'tag' ? { tagId: item.id } : { labelIds: form.labelIds.includes(item.id) ? form.labelIds : [...form.labelIds, item.id] })
            setAddTagOpen(false)
          }
          if (kind === 'tag') {
            createTag.mutate(input, { onSuccess: attach })
          } else {
            createLabel.mutate(input, { onSuccess: attach })
          }
        }}
      />
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
