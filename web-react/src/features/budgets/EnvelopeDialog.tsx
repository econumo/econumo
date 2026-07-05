import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CurrencySelect } from '@/components/CurrencySelect'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { defaultEnvelopeIcon } from '@/lib/icons'
import { isNotEmpty } from '@/lib/validation'
import type { BudgetElementDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { useCategories } from '@/features/classifications/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'

interface EnvelopeDialogProps {
  open: boolean
  envelope?: BudgetElementDto | null
  budgetCurrencyId: Id
  onClose: () => void
  onSubmit: (form: { name: string; icon: string; currencyId: Id; categories: Id[]; isArchived: 0 | 1 }) => void
}

const isValidEnvelopeName = (v: string) => v.length >= 3 && v.length <= 64

export function EnvelopeDialog({ open, envelope, budgetCurrencyId, onClose, onSubmit }: EnvelopeDialogProps) {
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

  // the Vue envelope form offers non-archived EXPENSE categories only
  const options = categories.filter((c) => c.isArchived === 0 && c.type === 'expense')

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('modules.budget.form.budget_envelope.name.validation.required_field'))
      return
    }
    if (!isValidEnvelopeName(name)) {
      setError(t('modules.budget.form.budget_envelope.name.validation.invalid_name'))
      return
    }
    if (!currencyId) {
      setError(t('modules.budget.form.budget_envelope.currency.validation.required_field'))
      return
    }
    onSubmit({ name, icon, currencyId, categories: [...selected], isArchived })
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('modules.budget.modal.create_envelope_form.header') : t('modules.budget.modal.update_envelope_form.header')}
    >
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="envelope-name">{t('modules.budget.form.budget_envelope.name.label')}</Label>
          <Input id="envelope-name" maxLength={64} value={name} onChange={(e) => setName(e.target.value)} />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="envelope-currency">{t('modules.budget.form.budget_envelope.currency.label')}</Label>
          <CurrencySelect
            id="envelope-currency"
            aria-label={t('modules.budget.form.budget_envelope.currency.label')}
            value={currencyId}
            onChange={setCurrencyId}
          />
        </div>

        {!isNew ? (
          <div className="flex flex-col gap-2">
            <Label>{t('modules.budget.form.budget_envelope.status.label')}</Label>
            <Select value={String(isArchived)} onValueChange={(v) => setIsArchived(v === '1' ? 1 : 0)}>
              <SelectTrigger aria-label={t('modules.budget.form.budget_envelope.status.label')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">{t('modules.budget.form.budget_envelope.status.option.active')}</SelectItem>
                <SelectItem value="1">{t('modules.budget.form.budget_envelope.status.option.archive')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        ) : null}

        <div className="flex flex-col gap-2">
          <Label>{t('modules.budget.form.budget_envelope.categories.label')}</Label>
          <ul className="flex max-h-36 flex-col gap-1 overflow-y-auto">
            {options.map((category) => (
              <li key={category.id} className="flex items-center gap-2 px-1">
                <Checkbox
                  id={`env-cat-${category.id}`}
                  checked={selected.has(category.id)}
                  onCheckedChange={(checked) => {
                    setSelected((prev) => {
                      const next = new Set(prev)
                      if (checked) {
                        next.add(category.id)
                      } else {
                        next.delete(category.id)
                      }
                      return next
                    })
                  }}
                />
                <Label htmlFor={`env-cat-${category.id}`} className="flex items-center gap-1 font-normal">
                  <EntityIcon name={category.icon} className="text-sm text-muted-foreground" />
                  {category.name}
                </Label>
              </li>
            ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            {t('modules.budget.form.budget_envelope.categories.selected', { count: String(selected.size) })}
          </p>
        </div>

        <div className="flex flex-col gap-2">
          <Label>{t('modules.budget.form.budget_envelope.icon.label')}</Label>
          <IconPicker value={icon} onChange={setIcon} aria-label={t('modules.budget.form.budget_envelope.icon.label')} />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit">{isNew ? t('elements.button.create.label') : t('elements.button.save.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
