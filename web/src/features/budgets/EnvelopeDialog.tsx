import { useEffect, useState } from 'react'
import { ChevronRight, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { fuzzyMatch } from '@/components/CurrencySelect'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { defaultEnvelopeIcon } from '@/lib/icons'
import { isNotEmpty } from '@/lib/validation'
import type { BudgetElementDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { useCategories } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
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
  const { data: currencies } = useCurrencies()
  const isNew = !envelope

  const [name, setName] = useState('')
  const [icon, setIcon] = useState(defaultEnvelopeIcon)
  const [currencyId, setCurrencyId] = useState<Id | null>(null)
  const [currencyOpen, setCurrencyOpen] = useState(false)
  const [selected, setSelected] = useState<Set<Id>>(new Set())
  const [isArchived, setIsArchived] = useState<0 | 1>(0)
  const [error, setError] = useState<string | null>(null)
  const [categorySearch, setCategorySearch] = useState('')

  useEffect(() => {
    if (open) {
      setName(envelope?.name ?? '')
      setIcon(envelope?.icon || defaultEnvelopeIcon)
      setCurrencyId(envelope?.currencyId ?? budgetCurrencyId ?? userCurrencyId(user))
      setSelected(new Set(envelope?.children.map((c) => c.id) ?? []))
      setIsArchived(envelope?.isArchived ?? 0)
      setError(null)
      setCategorySearch('')
    }
  }, [open, envelope, budgetCurrencyId, user])

  // the Vue envelope form offers non-archived EXPENSE categories only
  const options = categories.filter((c) => c.isArchived === 0 && c.type === 'expense')
  const shownOptions = options.filter((c) => !categorySearch || fuzzyMatch(c.name, categorySearch))

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
      caps
      fullScreen
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('modules.budget.modal.create_envelope_form.header') : t('modules.budget.modal.update_envelope_form.header')}
      footer={
        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit" form="envelope-dialog-form">
            {isNew ? t('elements.button.create.label') : t('elements.button.save.label')}
          </Button>
        </div>
      }
    >
      <form
        id="envelope-dialog-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <CardField label={t('modules.budget.form.budget_envelope.name.label')} htmlFor="envelope-name" error={error}>
          <Input
            id="envelope-name"
            className={cardFieldControlClass}
            maxLength={64}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </CardField>

        {/* same card shape, but a picker row: tap opens the currency search dialog */}
        <button
          type="button"
          className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover"
          title={t('modules.budget.form.budget_envelope.currency.label')}
          onClick={() => setCurrencyOpen(true)}
        >
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="text-[11px] text-muted-foreground">{t('modules.budget.form.budget_envelope.currency.label')}</span>
            <span className="truncate text-sm">{currencies?.find((c) => c.id === currencyId)?.code ?? ''}</span>
          </span>
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        </button>

        {!isNew ? (
          <CardField label={t('modules.budget.form.budget_envelope.status.label')} htmlFor="envelope-status">
            <div className="[&_button]:h-auto [&_button]:border-0 [&_button]:bg-transparent [&_button]:p-0 [&_button]:text-sm [&_button]:shadow-none [&_button]:focus-visible:ring-0">
              <Select value={String(isArchived)} onValueChange={(v) => setIsArchived(v === '1' ? 1 : 0)}>
                <SelectTrigger id="envelope-status" aria-label={t('modules.budget.form.budget_envelope.status.label')} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">{t('modules.budget.form.budget_envelope.status.option.active')}</SelectItem>
                  <SelectItem value="1">{t('modules.budget.form.budget_envelope.status.option.archive')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </CardField>
        ) : null}

        <div className="flex flex-col gap-0.5 rounded-lg bg-econumo-card px-4 py-2.5">
          <span className="flex items-baseline justify-between">
            <Label className="text-[11px] font-normal text-muted-foreground">
              {t('modules.budget.form.budget_envelope.categories.label')}
            </Label>
            <span className="text-[11px] text-muted-foreground">
              {t('modules.budget.form.budget_envelope.categories.selected', { count: String(selected.size) })}
            </span>
          </span>
          {options.length >= 6 ? (
            <span className="mt-1 flex items-center gap-2 rounded-md bg-background px-2.5 py-1.5">
              <Search className="size-4 shrink-0 text-muted-foreground" />
              <input
                aria-label={t('pages.account.toolbar.search')}
                placeholder={t('pages.account.toolbar.search')}
                className="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                value={categorySearch}
                onChange={(e) => setCategorySearch(e.target.value)}
              />
            </span>
          ) : null}
          <ul className="flex max-h-44 flex-col overflow-x-hidden overflow-y-auto scrollbar-slim">
            {shownOptions.map((category) => (
              <li key={category.id}>
                <Label
                  htmlFor={`env-cat-${category.id}`}
                  className="flex items-center gap-2.5 rounded-md py-2 font-normal hover:bg-econumo-hover"
                >
                  <EntityIcon name={category.icon} className="text-lg text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate text-sm">{category.name}</span>
                  <Checkbox
                    id={`env-cat-${category.id}`}
                    className="bg-background"
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
                </Label>
              </li>
            ))}
          </ul>
        </div>

        <div className="flex flex-col gap-2">
          <Label>{t('modules.budget.form.budget_envelope.icon.label')}</Label>
          <IconPicker value={icon} onChange={setIcon} aria-label={t('modules.budget.form.budget_envelope.icon.label')} />
        </div>
      </form>

      <CurrencyPickerDialog
        open={currencyOpen}
        title={t('modules.budget.form.budget_envelope.currency.label')}
        value={currencyId}
        onClose={() => setCurrencyOpen(false)}
        onPick={setCurrencyId}
      />
    </ResponsiveDialog>
  )
}
