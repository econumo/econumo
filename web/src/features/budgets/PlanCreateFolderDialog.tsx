import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { PlanElementDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { elementDisplayName } from './budgetMath'

type Side = 'income' | 'expense'

export function PlanCreateFolderDialog({
  open,
  elements,
  onClose,
  onSubmit,
}: {
  open: boolean
  elements: PlanElementDto[]
  onClose: () => void
  onSubmit: (form: { name: string; memberIds: Id[] }) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [side, setSide] = useState<Side>('expense')
  const [picked, setPicked] = useState<Id[]>([])

  // A folder's side is derived from its members, and a memberless folder renders
  // in the expense band whatever the user intended — so a member is required, and
  // switching sides clears the selection rather than letting a folder mix sides.
  const options = elements.filter(
    (el) => el.id !== UNCATEGORIZED_ID && el.isArchived === 0 && (isIncomeType(el.type) ? 'income' : 'expense') === side,
  )
  const switchSide = (next: Side) => {
    setSide(next)
    setPicked([])
  }
  const close = () => {
    setName('')
    setSide('expense')
    setPicked([])
    onClose()
  }
  if (!open) {
    return null
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && close()} title={t('budgets.modal.create_folder_form.header')}>
      <form
        className="flex flex-col gap-3"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim() !== '' && picked.length > 0) {
            onSubmit({ name: name.trim(), memberIds: picked })
          }
        }}
      >
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="plan-folder-name">{t('budgets.form.budget.folder_name.label')}</Label>
          <Input id="plan-folder-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>

        <div role="tablist" aria-label="folder side" className="flex w-fit rounded-md border p-0.5">
          {(['expense', 'income'] as const).map((s) => (
            <button
              key={s}
              type="button"
              role="tab"
              aria-selected={side === s}
              className={`rounded px-3 py-1 text-sm ${side === s ? 'bg-accent font-bold' : 'text-muted-foreground'}`}
              onClick={() => switchSide(s)}
            >
              {t(s === 'income' ? 'budgets.page.plan.section.income' : 'budgets.page.plan.section.expenses')}
            </button>
          ))}
        </div>

        <ul className="flex max-h-64 flex-col gap-1 overflow-y-auto scrollbar-slim">
          {options.map((el) => {
            const label = elementDisplayName(el.id, el.name, t)
            return (
              <li key={el.id} className="flex items-center gap-2">
                <Checkbox
                  id={`plan-folder-member-${el.id}`}
                  aria-label={label}
                  checked={picked.includes(el.id)}
                  onCheckedChange={(v) => setPicked((prev) => (v ? [...prev, el.id] : prev.filter((p) => p !== el.id)))}
                />
                <Label htmlFor={`plan-folder-member-${el.id}`} className="truncate font-normal">
                  {label}
                </Label>
              </li>
            )
          })}
        </ul>

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={close}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={name.trim() === '' || picked.length === 0}>
            {t('common.button.create.label')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
