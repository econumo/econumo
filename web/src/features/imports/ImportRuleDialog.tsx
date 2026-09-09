import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRuleAction, ImportRuleDto, ImportRuleMatchField, ImportRuleMatchType, ImportRuleSpecDto } from '@/api/dto/imports'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { ruleSpec } from './ruleSummary'
import { useCreateImportRule, usePreviewImportRule, useUpdateImportRule } from './queries'

const PREVIEW_DEBOUNCE_MS = 300
const ALL_SCOPE = { scope: 'all', runId: '', scopeSourceId: '' } as const

export const emptyRuleSpec = (priority: number): ImportRuleSpecDto => ({
  sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: '', isCaseSensitive: false,
  categoryId: '', payeeId: '', tagId: '', labelIds: [], priority,
})

interface ImportRuleDialogProps {
  open: boolean
  // editing an existing rule (its id is posted to update-rule) ...
  rule?: ImportRuleDto
  // ... or creating one, optionally prefilled (an accepted-for-editing suggestion)
  initial?: ImportRuleSpecDto
  onClose: () => void
  onSaved?: (rule: ImportRuleDto) => void
}

export function ImportRuleDialog({ open, rule, initial, onClose, onSaved }: ImportRuleDialogProps) {
  const { t } = useTranslation()
  const [spec, setSpec] = useState<ImportRuleSpecDto>(() => rule ? ruleSpec(rule) : initial ?? emptyRuleSpec(0))
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const preview = usePreviewImportRule()
  const { mutate: runPreview } = preview
  const create = useCreateImportRule()
  const update = useUpdateImportRule()

  useEffect(() => {
    if (!open || spec.matchValue.trim() === '') {
      return
    }
    const handle = setTimeout(() => runPreview({ spec, scope: ALL_SCOPE }), PREVIEW_DEBOUNCE_MS)
    return () => clearTimeout(handle)
  }, [open, spec, runPreview])

  const patch = (p: Partial<ImportRuleSpecDto>) => setSpec((s) => ({ ...s, ...p }))
  const setAction = (action: ImportRuleAction) =>
    // a skip rule carries no targets: clear them so the server never sees stale ones
    setSpec((s) => action === 'skip' ? { ...s, action, categoryId: '', payeeId: '', tagId: '', labelIds: [] } : { ...s, action })
  const toggleLabel = (id: string) =>
    setSpec((s) => ({ ...s, labelIds: s.labelIds.includes(id) ? s.labelIds.filter((l) => l !== id) : [...s.labelIds, id] }))

  const save = async () => {
    try {
      const saved = rule
        ? await update.mutateAsync({ id: rule.id, spec })
        : await create.mutateAsync({ spec })
      onSaved?.(saved)
      onClose()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const canSave = spec.matchValue.trim() !== '' && (spec.action === 'skip' || spec.categoryId || spec.payeeId || spec.tagId || spec.labelIds.length > 0)

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={rule ? t('imports.rules.editor.title_edit') : t('imports.rules.editor.title_create')}>
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="rule-action">{t('imports.rules.editor.action')}</Label>
          <NativeSelect id="rule-action" className="w-full" value={spec.action} onChange={(e) => setAction(e.target.value as ImportRuleAction)}>
            <NativeSelectOption value="classify">{t('imports.rules.action.classify')}</NativeSelectOption>
            <NativeSelectOption value="skip">{t('imports.rules.action.skip')}</NativeSelectOption>
          </NativeSelect>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div className="flex flex-col gap-1">
            <Label htmlFor="rule-field">{t('imports.rules.editor.field')}</Label>
            <NativeSelect id="rule-field" className="w-full" value={spec.matchField} onChange={(e) => patch({ matchField: e.target.value as ImportRuleMatchField })}>
              <NativeSelectOption value="external_payee">{t('imports.rules.field.external_payee')}</NativeSelectOption>
              <NativeSelectOption value="description">{t('imports.rules.field.description')}</NativeSelectOption>
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="rule-type">{t('imports.rules.editor.type')}</Label>
            <NativeSelect id="rule-type" className="w-full" value={spec.matchType} onChange={(e) => patch({ matchType: e.target.value as ImportRuleMatchType })}>
              <NativeSelectOption value="contains">{t('imports.rules.type.contains')}</NativeSelectOption>
              <NativeSelectOption value="prefix">{t('imports.rules.type.prefix')}</NativeSelectOption>
              <NativeSelectOption value="exact">{t('imports.rules.type.exact')}</NativeSelectOption>
            </NativeSelect>
          </div>
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="rule-value">{t('imports.rules.editor.value')}</Label>
          <Input id="rule-value" value={spec.matchValue} maxLength={255} autoComplete="off" onChange={(e) => patch({ matchValue: e.target.value })} />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={spec.isCaseSensitive} onCheckedChange={(v) => patch({ isCaseSensitive: v === true })} aria-label={t('imports.rules.editor.case_sensitive')} />
          {t('imports.rules.editor.case_sensitive')}
        </label>
        {spec.action === 'classify' ? (
          <>
            <div className="flex flex-col gap-1">
              <Label htmlFor="rule-category">{t('imports.rules.editor.category')}</Label>
              <NativeSelect id="rule-category" className="w-full" value={spec.categoryId} onChange={(e) => patch({ categoryId: e.target.value })}>
                <NativeSelectOption value="">—</NativeSelectOption>
                {categories.filter((c) => !c.isArchived || c.id === spec.categoryId).map((c) => <NativeSelectOption key={c.id} value={c.id}>{c.name}</NativeSelectOption>)}
              </NativeSelect>
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="rule-payee">{t('imports.rules.editor.payee')}</Label>
              <NativeSelect id="rule-payee" className="w-full" value={spec.payeeId} onChange={(e) => patch({ payeeId: e.target.value })}>
                <NativeSelectOption value="">—</NativeSelectOption>
                {payees.filter((p) => !p.isArchived || p.id === spec.payeeId).map((p) => <NativeSelectOption key={p.id} value={p.id}>{p.name}</NativeSelectOption>)}
              </NativeSelect>
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="rule-tag">{t('imports.rules.editor.tag')}</Label>
              <NativeSelect id="rule-tag" className="w-full" value={spec.tagId} onChange={(e) => patch({ tagId: e.target.value })}>
                <NativeSelectOption value="">—</NativeSelectOption>
                {tags.filter((x) => !x.isArchived || x.id === spec.tagId).map((x) => <NativeSelectOption key={x.id} value={x.id}>{x.name}</NativeSelectOption>)}
              </NativeSelect>
            </div>
            {labels.length > 0 ? (
              <fieldset className="flex flex-col gap-1">
                <legend className="text-sm">{t('imports.rules.editor.labels')}</legend>
                <div className="flex flex-wrap gap-3">
                  {labels.filter((l) => !l.isArchived || spec.labelIds.includes(l.id)).map((l) => (
                    <label key={l.id} className="flex items-center gap-2 text-sm">
                      <Checkbox checked={spec.labelIds.includes(l.id)} onCheckedChange={() => toggleLabel(l.id)} aria-label={l.name} />
                      {l.name}
                    </label>
                  ))}
                </div>
              </fieldset>
            ) : null}
          </>
        ) : null}
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {spec.matchValue.trim() === '' ? '' : preview.isPending ? t('common.app.modal.loading.data_loading') : t('imports.rules.editor.match_count', { count: preview.data?.matched ?? 0 })}
        </p>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>{t('common.button.cancel.label')}</Button>
          <Button type="button" disabled={!canSave || create.isPending || update.isPending} onClick={() => void save()}>{t('common.button.save.label')}</Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
