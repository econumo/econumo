import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { OptionSelect } from '@/components/OptionSelect'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyField } from '@/features/currencies/CurrencyField'
import { IconPicker } from '@/components/IconPicker'
import { ChecklistSheet, PickerRow } from '@/components/ChecklistField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { defaultEnvelopeIcon } from '@/lib/icons'
import { isNotEmpty } from '@/lib/validation'
import type { Id } from '@/api/types'
import { useCategories } from '@/features/classifications/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'

// A structural subset of BudgetElementDto/PlanElementDto — both the budget
// table and the plan sheet can pass their own element straight through,
// with no adapter object needed.
export interface EnvelopeDialogTarget {
  id: Id
  name: string
  icon: string
  currencyId: Id | null
  isArchived: 0 | 1
  children: { id: Id }[]
}

interface EnvelopeDialogProps {
  open: boolean
  envelope?: EnvelopeDialogTarget | null
  budgetCurrencyId: Id
  side: 'expense' | 'income'
  onClose: () => void
  onSubmit: (form: { name: string; icon: string; currencyId: Id; categories: Id[]; isArchived: 0 | 1 }) => void
}

const isValidEnvelopeName = (v: string) => v.length >= 3 && v.length <= 64

export function EnvelopeDialog({ open, envelope, budgetCurrencyId, side, onClose, onSubmit }: EnvelopeDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: categories = [] } = useCategories()
  const isNew = !envelope

  const [name, setName] = useState('')
  const [icon, setIcon] = useState(defaultEnvelopeIcon)
  const [currencyId, setCurrencyId] = useState<Id | null>(null)
  const [selected, setSelected] = useState<Set<Id>>(new Set())
  const [isArchived, setIsArchived] = useState<0 | 1>(0)
  const [error, setError] = useState<string | null>(null)
  const [categoriesOpen, setCategoriesOpen] = useState(false)

  useEffect(() => {
    if (open) {
      setName(envelope?.name ?? '')
      setIcon(envelope?.icon || defaultEnvelopeIcon)
      setCurrencyId(envelope?.currencyId ?? budgetCurrencyId ?? userCurrencyId(user))
      setSelected(new Set(envelope?.children.map((c) => c.id) ?? []))
      setIsArchived(envelope?.isArchived ?? 0)
      setError(null)
    }
  }, [open, envelope, budgetCurrencyId, user])

  // a single dialog never mixes sides: the category picker offers only the
  // side it was opened for (non-archived, matching type)
  const options = categories.filter((c) => c.isArchived === 0 && c.type === side)
  const picked = options.filter((c) => selected.has(c.id))

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('budgets.form.budget_envelope.name.validation.required_field'))
      return
    }
    if (!isValidEnvelopeName(name)) {
      setError(t('budgets.form.budget_envelope.name.validation.invalid_name'))
      return
    }
    if (!currencyId) {
      setError(t('budgets.form.budget_envelope.currency.validation.required_field'))
      return
    }
    onSubmit({ name, icon, currencyId, categories: [...selected], isArchived })
  }

  return (
    <ResponsiveDialog
      open={open}
      caps
      fullScreen
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('budgets.modal.create_envelope_form.header') : t('budgets.modal.update_envelope_form.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="envelope-dialog-form">
            {isNew ? t('common.button.create.label') : t('common.button.save.label')}
          </Button>
        </div>
      }
    >
      <form
        id="envelope-dialog-form"
        // min-h-full: on the full-screen mobile page the last (icon) block grows
        // into the leftover height; desktop's auto-height dialog ignores it
        className="flex min-h-full flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <CardField label={t('budgets.form.budget_envelope.name.label')} htmlFor="envelope-name" error={error}>
          <Input
            id="envelope-name"
            className={cardFieldControlClass}
            maxLength={64}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </CardField>

        <CurrencyField
          label={t('budgets.form.budget_envelope.currency.label')}
          value={currencyId}
          onChange={setCurrencyId}
        />

        {!isNew ? (
          <CardField label={t('budgets.form.budget_envelope.status.label')} htmlFor="envelope-status">
            <div className="[&_button]:h-auto [&_button]:border-0 [&_button]:bg-transparent [&_button]:p-0 [&_button]:text-sm [&_button]:shadow-none [&_button]:focus-visible:ring-0">
              <OptionSelect
                id="envelope-status"
                aria-label={t('budgets.form.budget_envelope.status.label')}
                title={t('budgets.form.budget_envelope.status.label')}
                className="w-full"
                value={String(isArchived)}
                onValueChange={(v) => setIsArchived(v === '1' ? 1 : 0)}
                options={[
                  { value: '0', label: t('budgets.form.budget_envelope.status.option.active') },
                  { value: '1', label: t('budgets.form.budget_envelope.status.option.archive') },
                ]}
              />
            </div>
          </CardField>
        ) : null}

        {/* a picker row like the budget's accounts: the form keeps its size
            however many categories there are */}
        <PickerRow
          label={t('budgets.form.budget_envelope.categories.label')}
          aside={t('budgets.form.budget_envelope.categories.selected', { count: String(selected.size) })}
          value={picked.length > 0 ? picked.map((c) => c.name).join(', ') : t('budgets.modal.budget_form.savings.none')}
          muted={picked.length === 0}
          testId="envelope-categories-field"
          onOpen={() => setCategoriesOpen(true)}
        />
        <ChecklistSheet
          open={categoriesOpen}
          title={t('budgets.form.budget_envelope.categories.label')}
          groups={[{ items: options }]}
          checked={selected}
          onToggle={(id, on) =>
            setSelected((prev) => {
              const next = new Set(prev)
              if (on) {
                next.add(id)
              } else {
                next.delete(id)
              }
              return next
            })
          }
          onClose={() => setCategoriesOpen(false)}
        />

        <div className="flex min-h-0 flex-1 flex-col gap-2">
          <Label>{t('budgets.form.budget_envelope.icon.label')}</Label>
          <IconPicker fill value={icon} onChange={setIcon} aria-label={t('budgets.form.budget_envelope.icon.label')} />
        </div>
      </form>

    </ResponsiveDialog>
  )
}
