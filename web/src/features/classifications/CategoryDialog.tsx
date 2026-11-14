import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useApplyIconSuggestion } from '@/features/iconSuggestion/useIconSuggestion'
import { defaultCategoryIcon } from '@/lib/icons'
import { isNotEmpty, isValidCategoryName } from '@/lib/validation'
import type { CategoryDto, CategoryType } from '@/api/dto/category'

interface CategoryDialogProps {
  open: boolean
  /** the row being edited; only its name/type/icon are read, so a caller that
   *  holds a category by another shape (a budget plan element) can prefill too */
  category?: Pick<CategoryDto, 'name' | 'type' | 'icon'> | null
  onClose: () => void
  onSubmit: (form: { name: string; type: CategoryType; icon: string }) => void
  /** a new category of this type only: the type switch is not shown */
  fixedType?: CategoryType
}

export function CategoryDialog({ open, category, onClose, onSubmit, fixedType }: CategoryDialogProps) {
  const { t } = useTranslation()
  const online = useOnline()
  const isNew = !category
  const [name, setName] = useState('')
  const [type, setType] = useState<CategoryType>('expense')
  const [icon, setIcon] = useState(defaultCategoryIcon)
  // a picked icon is the user's: no suggestion replaces it
  const [iconPicked, setIconPicked] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // an existing category's icon never changes on its own
  useApplyIconSuggestion({ kind: 'category', name, type, enabled: open && isNew && !iconPicked }, icon, setIcon, 'dialog')

  useEffect(() => {
    if (open) {
      setName(category?.name ?? '')
      setType(category?.type ?? fixedType ?? 'expense')
      setIcon(category?.icon || defaultCategoryIcon)
      setIconPicked(false)
      setError(null)
    }
  }, [open, category, fixedType])

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('classifications.categories.forms.category.name.validation.required_field'))
      return
    }
    if (!isValidCategoryName(name)) {
      setError(t('classifications.categories.forms.category.name.validation.invalid_name'))
      return
    }
    onSubmit({ name, type, icon })
  }

  return (
    <ResponsiveDialog
      open={open}
      caps
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('classifications.categories.modals.create.header') : t('classifications.categories.modals.edit.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="category-dialog-form" disabled={!online}>
            {isNew ? t('common.button.create.label') : t('common.button.update.label')}
          </Button>
        </div>
      }
    >
      <form
        id="category-dialog-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        {fixedType ? null : (
          <div className="flex rounded-md border p-0.5" role="radiogroup" aria-label="type">
            {(['income', 'expense'] as const).map((option) => (
              <button
                key={option}
                type="button"
                role="radio"
                aria-checked={type === option}
                disabled={!isNew}
                className={`flex-1 rounded px-2 py-1.5 text-sm ${type === option ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'} disabled:opacity-60`}
                onClick={() => setType(option)}
              >
                {t(`classifications.categories.forms.category.type.${option}`)}
              </button>
            ))}
          </div>
        )}

        <div className="flex flex-col gap-2">
          <Label htmlFor="category-name">{t('classifications.categories.forms.category.name.label')}</Label>
          <Input
            id="category-name"
            maxLength={64}
            placeholder={t('classifications.categories.forms.category.name.placeholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label>{t('classifications.categories.forms.category.icon.label')}</Label>
          <IconPicker
            value={icon}
            onChange={(picked) => {
              setIcon(picked)
              setIconPicked(true)
            }}
            aria-label={t('classifications.categories.forms.category.icon.label')} />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
