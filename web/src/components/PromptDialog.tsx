import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'

interface PromptDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (value: string) => void
  title: string
  inputLabel: string
  initialValue?: string
  validate?: (value: string) => string | null
  submitLabel: string
  cancelLabel: string
}

export function PromptDialog({ open, onClose, onSubmit, title, inputLabel, initialValue = '', validate, submitLabel, cancelLabel }: PromptDialogProps) {
  const [value, setValue] = useState(initialValue)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setValue(initialValue)
      setError(null)
    }
  }, [open, initialValue])

  const submit = () => {
    const message = validate ? validate(value) : null
    if (message) {
      setError(message)
      return
    }
    onSubmit(value)
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title} dismissible={false}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <CardField label={inputLabel} htmlFor="prompt-input" error={error}>
          <Input
            id="prompt-input"
            className={cardFieldControlClass}
            autoFocus
            maxLength={64}
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
        </CardField>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {cancelLabel}
          </Button>
          <Button type="submit">{submitLabel}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
