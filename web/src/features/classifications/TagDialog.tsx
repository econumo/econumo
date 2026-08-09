import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { CLASSIFICATION_KINDS, DEFAULT_ICON, type ClassificationKind } from '@/lib/classificationKind'
import { apiErrorMessage } from '@/lib/apiError'
import { isNotEmpty, isValidLabelName, isValidTagName } from '@/lib/validation'
import { useUserData } from '@/features/user/queries'
import { useCreateLabel, useCreateTag, useUpdateLabel, useUpdateTag } from './queries'

export interface TagDialogItem {
  id: string
  name: string
  kind: ClassificationKind
  icon: string
}

interface TagDialogProps {
  open: boolean
  item?: TagDialogItem | null
  onClose: () => void
  /** creating in the context of a shared account: the server owns the new row
   *  to that account's OWNER, not to the caller */
  accountId?: string
  /** the resolved account owner, when it differs from the signed-in user — the
   *  create hooks dedupe by name WITHIN one owner's rows */
  ownerUserId?: string
  /** creating on behalf of a caller who will attach the row to something (a
   *  transaction, a recurring template) rather than just adding it to a list */
  onCreated?: (kind: ClassificationKind, item: { id: string }) => void
}

export function TagDialog({ open, item, onClose, accountId, ownerUserId, onCreated }: TagDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const isNew = !item
  const [kind, setKind] = useState<ClassificationKind>(CLASSIFICATION_KINDS[0])
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)

  const createTag = useCreateTag()
  const createLabel = useCreateLabel()
  const updateTag = useUpdateTag()
  const updateLabel = useUpdateLabel()

  useEffect(() => {
    if (open) {
      setKind(item?.kind ?? CLASSIFICATION_KINDS[0])
      setName(item?.name ?? '')
      setError(null)
    }
  }, [open, item])

  const title = isNew
    ? kind === 'tag'
      ? t('classifications.tags.modals.create.header')
      : t('classifications.labels.modals.create.header')
    : item?.kind === 'tag'
      ? t('classifications.tags.modals.edit.header')
      : t('classifications.labels.modals.edit.header')

  const validate = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('classifications.tags.forms.tag.name.validation.required_field')
    }
    const valid = kind === 'tag' ? isValidTagName(value) : isValidLabelName(value)
    if (!valid) {
      return kind === 'tag'
        ? t('classifications.tags.forms.tag.name.validation.invalid_name')
        : t('classifications.labels.forms.label.name.validation.invalid_name')
    }
    return null
  }

  const submit = () => {
    const message = validate(name)
    if (message) {
      setError(message)
      return
    }
    // Both kinds share one name namespace, so the server rejects a name the OTHER
    // kind already holds — a rejection the client cannot predict from this form.
    const onError = (err: unknown) => setError(apiErrorMessage(err))
    if (item) {
      // Kind is immutable once saved, so an edit always targets the row's own kind.
      const mutation = item.kind === 'tag' ? updateTag : updateLabel
      mutation.mutate({ id: item.id, name }, { onSuccess: onClose, onError })
    } else {
      const mutation = kind === 'tag' ? createTag : createLabel
      const onSuccess = (created: { id: string }) => {
        onCreated?.(kind, created)
        onClose()
      }
      mutation.mutate({ name, accountId, ownerUserId: ownerUserId ?? user?.id }, { onSuccess, onError })
    }
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
        <div className="flex flex-col gap-3">
          <div data-testid="kind-icon">
            {/* create: preview-only DEFAULT_ICON, since nothing is saved yet.
                edit: the row's own stored icon (no picker here to change it). */}
            <EntityIcon name={isNew ? DEFAULT_ICON[kind] : (item?.icon ?? DEFAULT_ICON[kind])} className="text-2xl text-muted-foreground" />
          </div>
          {isNew ? (
            <div className="flex flex-col gap-1.5" role="radiogroup">
              {CLASSIFICATION_KINDS.map((option) => (
                <button
                  key={option}
                  type="button"
                  role="radio"
                  aria-checked={kind === option}
                  aria-label={t(`classifications.tags.forms.tag.kind.${option}_option`)}
                  className={`rounded-md border px-3 py-2 text-left ${kind === option ? 'border-primary bg-accent' : 'hover:bg-accent/50'}`}
                  onClick={() => setKind(option)}
                >
                  <span className="block text-sm font-medium">
                    {t(`classifications.tags.forms.tag.kind.${option}_option`)}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {t(`classifications.tags.forms.tag.kind.${option}_hint`)}
                  </span>
                </button>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground" data-testid="kind-locked-note">
              {t('classifications.tags.forms.tag.kind.locked_note')}
            </p>
          )}
        </div>

        <CardField label={t('classifications.tags.forms.tag.name.label')} htmlFor="tag-dialog-name" error={error}>
          <Input
            id="tag-dialog-name"
            className={cardFieldControlClass}
            autoFocus
            maxLength={64}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </CardField>

        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit">{isNew ? t('common.button.create.label') : t('common.button.update.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
