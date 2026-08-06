import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { CLASSIFICATION_KINDS, DEFAULT_ICON, kindAccentClass, type ClassificationKind } from '@/lib/classificationKind'
import { isNotEmpty, isValidLabelName, isValidTagName } from '@/lib/validation'

interface AddTagDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (kind: ClassificationKind, name: string) => void
}

export function AddTagDialog({ open, onClose, onSubmit }: AddTagDialogProps) {
  const { t } = useTranslation()
  const [kind, setKind] = useState<ClassificationKind>('tag')
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setKind('tag')
      setName('')
      setError(null)
    }
  }, [open])

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('classifications.tags.forms.tag.name.validation.required_field'))
      return
    }
    const valid = kind === 'tag' ? isValidTagName(name) : isValidLabelName(name)
    if (!valid) {
      setError(
        kind === 'tag'
          ? t('classifications.tags.forms.tag.name.validation.invalid_name')
          : t('classifications.labels.forms.label.name.validation.invalid_name'),
      )
      return
    }
    onSubmit(kind, name)
    setName('')
    setError(null)
  }

  const title = kind === 'tag' ? t('classifications.tags.modals.create.header') : t('classifications.labels.modals.create.header')

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title} hideHeader>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex items-center gap-3">
          {/* nothing is saved yet, so the preview is the kind's default icon */}
          <EntityIcon name={DEFAULT_ICON[kind]} className={`text-2xl ${kindAccentClass(kind)}`} />
          <div className="flex rounded-md border p-0.5" role="radiogroup">
            {CLASSIFICATION_KINDS.map((option) => (
              <button
                key={option}
                type="button"
                role="radio"
                aria-checked={kind === option}
                className={`flex-1 rounded px-2 py-1.5 text-sm ${kind === option ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'}`}
                onClick={() => setKind(option)}
              >
                {option === 'tag' ? t('classifications.tags.forms.tag.kind.tag') : t('classifications.tags.forms.tag.kind.label')}
              </button>
            ))}
          </div>
        </div>

        <CardField label={t('classifications.tags.forms.tag.name.label')} htmlFor="new-tag-name" error={error}>
          <Input
            id="new-tag-name"
            className={cardFieldControlClass}
            maxLength={64}
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
        </CardField>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit">{t('common.button.add.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
