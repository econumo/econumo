import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

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
        <div className="flex flex-col gap-2">
          <Label htmlFor="prompt-input">{inputLabel}</Label>
          <Input id="prompt-input" autoFocus maxLength={64} value={value} onChange={(e) => setValue(e.target.value)} />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={onClose}>
            {cancelLabel}
          </Button>
          <Button type="submit">{submitLabel}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
