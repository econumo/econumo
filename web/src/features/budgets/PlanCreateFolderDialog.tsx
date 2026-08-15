import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { PlanElementDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { isNotEmpty, isValidBudgetFolderName } from '@/lib/validation'
import { elementDisplayName } from './budgetMath'

type Side = 'income' | 'expense'

// The form's state lives in a CHILD of the `!open` early return, so closing unmounts
// it and React discards every field. Co-locating that state with the early return
// would instead let it survive a close (the component itself never unmounts), and a
// re-open would restore the just-created folder's name and members — a second submit
// then duplicates it. The reset must not depend on a local callback: the success path
// closes from the parent, so only unmounting covers every close path.
export function PlanCreateFolderDialog(props: {
  open: boolean
  elements: PlanElementDto[]
  onClose: () => void
  onSubmit: (form: { name: string; memberIds: Id[] }) => void
}) {
  if (!props.open) {
    return null
  }
  return <CreateFolderForm {...props} />
}

function CreateFolderForm({
  elements,
  onClose,
  onSubmit,
}: {
  elements: PlanElementDto[]
  onClose: () => void
  onSubmit: (form: { name: string; memberIds: Id[] }) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [side, setSide] = useState<Side>('income')
  const [picked, setPicked] = useState<Id[]>([])
  const [error, setError] = useState<string | null>(null)

  const validate = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('budgets.form.budget.folder_name.validation.required_field')
    }
    if (!isValidBudgetFolderName(value)) {
      return t('budgets.form.budget.folder_name.validation.invalid_name')
    }
    return null
  }

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
  return (
    <ResponsiveDialog
      open
      caps
      onOpenChange={(o) => !o && onClose()}
      title={t('budgets.modal.create_folder_form.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="plan-create-folder-form" disabled={picked.length === 0}>
            {t('common.button.create.label')}
          </Button>
        </div>
      }
    >
      <form
        id="plan-create-folder-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          const trimmed = name.trim()
          const message = validate(trimmed)
          if (message !== null) {
            setError(message)
            return
          }
          if (picked.length > 0) {
            onSubmit({ name: trimmed, memberIds: picked })
          }
        }}
      >
        <CardField label={t('budgets.form.budget.folder_name.label')} htmlFor="plan-folder-name" error={error}>
          <Input
            id="plan-folder-name"
            className={cardFieldControlClass}
            maxLength={64}
            value={name}
            aria-invalid={error !== null}
            onChange={(e) => {
              setName(e.target.value)
              setError(null)
            }}
          />
        </CardField>

        <div className="flex flex-col gap-0.5 rounded-lg bg-econumo-card px-4 py-2.5">
          <span className="flex items-baseline justify-between">
            <Label className="text-[11px] font-normal text-muted-foreground">
              {t('budgets.form.budget_envelope.categories.label')}
            </Label>
            <span className="text-[11px] text-muted-foreground">
              {t('budgets.form.budget_envelope.categories.selected', { count: String(picked.length) })}
            </span>
          </span>

          <div role="tablist" aria-label="folder side" className="mt-1 flex rounded-md bg-background p-0.5">
            {(['income', 'expense'] as const).map((s) => (
              <button
                key={s}
                type="button"
                role="tab"
                aria-selected={side === s}
                className={`flex-1 rounded px-3 py-1 text-sm ${side === s ? 'bg-accent font-medium' : 'text-muted-foreground'}`}
                onClick={() => switchSide(s)}
              >
                {t(s === 'income' ? 'budgets.page.plan.section.income' : 'budgets.page.plan.section.expenses')}
              </button>
            ))}
          </div>

          <ul className="mt-1 flex max-h-56 flex-col overflow-x-hidden overflow-y-auto scrollbar-slim">
            {options.map((el) => {
              const label = elementDisplayName(el.id, el.name, t)
              return (
                <li key={el.id}>
                  <Label
                    htmlFor={`plan-folder-member-${el.id}`}
                    className="flex items-center gap-2.5 rounded-md py-2 font-normal hover:bg-econumo-hover"
                  >
                    <EntityIcon name={el.icon} className="text-lg text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate text-sm">{label}</span>
                    <Checkbox
                      id={`plan-folder-member-${el.id}`}
                      className="bg-background"
                      aria-label={label}
                      checked={picked.includes(el.id)}
                      onCheckedChange={(v) => setPicked((prev) => (v ? [...prev, el.id] : prev.filter((p) => p !== el.id)))}
                    />
                  </Label>
                </li>
              )
            })}
          </ul>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
