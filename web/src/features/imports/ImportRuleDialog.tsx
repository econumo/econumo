import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRuleAction, ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { Button } from '@/components/ui/button'
import { CardField } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { ClassificationChips } from '@/features/transactions/ClassificationChips'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { SelectCard } from '@/features/transactions/SelectCard'
import { classificationChips, toggleClassification } from '@/features/transactions/useTransactionForm'
import { apiErrorMessage } from '@/lib/apiError'
import { RuleCondition } from './RuleCondition'
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


  const patch = (p: Partial<ImportRuleSpecDto>) => setSpec((s) => ({ ...s, ...p }))
  const setAction = (action: ImportRuleAction) =>
    // a skip rule carries no targets: clear them so the server never sees stale ones
    setSpec((s) => action === 'skip' ? { ...s, action, categoryId: '', payeeId: '', tagId: '', labelIds: [] } : { ...s, action })

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

  const canSave = spec.matchValue.trim() !== '' && (spec.action === 'skip' || spec.categoryId !== '' || spec.payeeId !== '' || spec.tagId !== '' || spec.labelIds.length > 0)

  // The server refuses to preview a rule it would refuse to save (a classify
  // rule with nothing to set), so the count waits for a complete rule.
  useEffect(() => {
    if (!open || !canSave) {
      return
    }
    const handle = setTimeout(() => runPreview({ spec, scope: ALL_SCOPE }), PREVIEW_DEBOUNCE_MS)
    return () => clearTimeout(handle)
  }, [open, canSave, spec, runPreview])

  const chips = classificationChips(tags, labels, { tagId: spec.tagId || null, labelIds: spec.labelIds }, undefined)
  const matchCount = !canSave
    ? ''
    : preview.data
      ? t('imports.rules.editor.match_count', { count: preview.data.matched })
      : preview.isPending ? t('common.app.modal.loading.data_loading') : ''

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={rule ? t('imports.rules.editor.title_edit') : t('imports.rules.editor.title_create')}>
      <div className="flex flex-col gap-4">
        <section className="flex flex-col gap-2">
          <h3 className="px-1 text-xs uppercase tracking-wide text-muted-foreground">{t('imports.rules.editor.when')}</h3>
          <RuleCondition id="rule-value" matchField={spec.matchField} matchType={spec.matchType} matchValue={spec.matchValue}
            valueLabel={t('imports.rules.editor.value')} onChange={patch}
            caseSensitive={{ value: spec.isCaseSensitive, onChange: (v) => patch({ isCaseSensitive: v }) }} />
          <p className="min-h-4 px-1 text-xs text-muted-foreground" aria-live="polite">{matchCount}</p>
        </section>

        <section className="flex flex-col gap-2">
          <h3 className="px-1 text-xs uppercase tracking-wide text-muted-foreground">{t('imports.rules.editor.then')}</h3>
          <div className="flex rounded-lg bg-econumo-card p-1" role="radiogroup" aria-label={t('imports.rules.editor.action')}>
            {(['classify', 'skip'] as const).map((action) => (
              <button key={action} type="button" role="radio" aria-checked={spec.action === action}
                className={`flex-1 rounded-md px-2 py-2 text-[13px] uppercase tracking-wide transition-colors ${
                  spec.action === action ? 'bg-econumo-magenta text-white' : 'text-muted-foreground hover:text-foreground'
                }`}
                onClick={() => setAction(action)}>
                {t(`imports.rules.action.${action}`)}
              </button>
            ))}
          </div>
          {spec.action === 'classify' ? (
            <>
              <SelectCard label={t('imports.rules.editor.category')}>
                <EntitySelect aria-label={t('imports.rules.editor.category')} value={spec.categoryId || null} clearable placeholder={t('imports.rules.editor.not_set')}
                  onChange={(id) => patch({ categoryId: id ?? '' })}
                  options={categories.filter((c) => !c.isArchived || c.id === spec.categoryId).map((c) => ({ value: c.id, label: c.name, icon: c.icon || 'pending' }))} />
              </SelectCard>
              <SelectCard label={t('imports.rules.editor.payee')}>
                <EntitySelect aria-label={t('imports.rules.editor.payee')} value={spec.payeeId || null} clearable placeholder={t('imports.rules.editor.not_set')}
                  onChange={(id) => patch({ payeeId: id ?? '' })}
                  options={payees.filter((p) => !p.isArchived || p.id === spec.payeeId).map((p) => ({ value: p.id, label: p.name }))} />
              </SelectCard>
              {chips.length > 0 ? (
                <CardField label={t('accounts.page.preview_transaction_modal.tags.label')}>
                  <ClassificationChips chips={chips} onToggle={(chip) => setSpec((s) => {
                    const next = toggleClassification({ tagId: s.tagId || null, labelIds: s.labelIds }, chip.kind, chip.id)
                    return { ...s, tagId: next.tagId ?? '', labelIds: next.labelIds }
                  })} />
                </CardField>
              ) : null}
            </>
          ) : (
            <p className="rounded-lg bg-econumo-card px-4 py-3 text-sm text-muted-foreground">{t('imports.rules.page.skip_hint')}</p>
          )}
        </section>

        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>{t('common.button.cancel.label')}</Button>
          <Button type="button" disabled={!canSave || create.isPending || update.isPending} onClick={() => void save()}>{t('common.button.save.label')}</Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
