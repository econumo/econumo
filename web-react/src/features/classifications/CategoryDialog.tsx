import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { availableIcons, defaultCategoryIcon } from '@/lib/icons'
import { isNotEmpty, isValidCategoryName } from '@/lib/validation'
import type { CategoryDto, CategoryType } from '@/api/dto/category'

interface CategoryDialogProps {
  open: boolean
  category?: CategoryDto | null
  onClose: () => void
  onSubmit: (form: { name: string; type: CategoryType; icon: string }) => void
}

export function CategoryDialog({ open, category, onClose, onSubmit }: CategoryDialogProps) {
  const { t } = useTranslation()
  const isNew = !category
  const [name, setName] = useState('')
  const [type, setType] = useState<CategoryType>('expense')
  const [icon, setIcon] = useState(defaultCategoryIcon)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setName(category?.name ?? '')
      setType(category?.type ?? 'expense')
      setIcon(category?.icon || defaultCategoryIcon)
      setError(null)
    }
  }, [open, category])

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('modules.classifications.categories.forms.category.name.validation.required_field'))
      return
    }
    if (!isValidCategoryName(name)) {
      setError(t('modules.classifications.categories.forms.category.name.validation.invalid_name'))
      return
    }
    onSubmit({ name, type, icon })
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('modules.classifications.categories.modals.create.header') : t('modules.classifications.categories.modals.edit.header')}
    >
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
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
              {t(`modules.classifications.categories.forms.category.type.${option}`)}
            </button>
          ))}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="category-name">{t('modules.classifications.categories.forms.category.name.label')}</Label>
          <Input
            id="category-name"
            maxLength={64}
            placeholder={t('modules.classifications.categories.forms.category.name.placeholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label>{t('modules.classifications.categories.forms.category.icon.label')}</Label>
          <div className="grid max-h-40 grid-cols-9 gap-1 overflow-y-auto" role="listbox" aria-label={t('modules.classifications.categories.forms.category.icon.label')}>
            {availableIcons.map((iconName) => (
              <button
                key={iconName}
                type="button"
                role="option"
                aria-selected={icon === iconName}
                aria-label={iconName}
                className={`flex items-center justify-center rounded-md p-1.5 hover:bg-accent ${icon === iconName ? 'bg-accent ring-1 ring-ring' : ''}`}
                onClick={() => setIcon(iconName)}
              >
                <EntityIcon name={iconName} className="text-xl" />
              </button>
            ))}
          </div>
        </div>

        <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit">{isNew ? t('elements.button.create.label') : t('elements.button.update.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
