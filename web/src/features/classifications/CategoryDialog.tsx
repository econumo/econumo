import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { defaultCategoryIcon } from '@/lib/icons'
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
      fullScreen
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('classifications.categories.modals.create.header') : t('classifications.categories.modals.edit.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="category-dialog-form">
            {isNew ? t('common.button.create.label') : t('common.button.update.label')}
          </Button>
        </div>
      }
    >
      <form
        id="category-dialog-form"
        // min-h-full: on the full-screen mobile page the last (icon) block grows
        // into the leftover height; desktop's auto-height dialog ignores it
        className="flex min-h-full flex-col gap-4"
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
              {t(`classifications.categories.forms.category.type.${option}`)}
            </button>
          ))}
        </div>

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

        <div className="flex min-h-0 flex-1 flex-col gap-2">
          <Label>{t('classifications.categories.forms.category.icon.label')}</Label>
          <IconPicker fill value={icon} onChange={setIcon} aria-label={t('classifications.categories.forms.category.icon.label')} />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
