import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { isValidTagName } from '@/lib/validation'

interface AddTagDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (name: string) => void
}

export function AddTagDialog({ open, onClose, onSubmit }: AddTagDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)

  const submit = () => {
    if (!name) {
      setError(t('modals.transaction.dialog.new_tag.name.validation.required_field'))
      return
    }
    if (!isValidTagName(name)) {
      setError(t('modals.transaction.dialog.new_tag.name.validation.required_field'))
      return
    }
    onSubmit(name)
    setName('')
    setError(null)
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modals.transaction.dialog.new_tag.header')}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="new-tag-name">{t('modals.transaction.dialog.new_tag.name.label')}</Label>
          <Input id="new-tag-name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit">{t('elements.button.add.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
