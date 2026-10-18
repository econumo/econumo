import { useEffect, useState } from 'react'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { CategoryType } from '@/api/dto/category'
import type { ImportRuleAction, ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { Button } from '@/components/ui/button'
import { CardField } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { TagDialog } from '@/features/classifications/TagDialog'
import { useCategories, useCreateCategory, useCreatePayee, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { ClassificationChips } from '@/features/transactions/ClassificationChips'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { SelectCard } from '@/features/transactions/SelectCard'
import { classificationChips, toggleClassification } from '@/features/transactions/useTransactionForm'
import { useAccessState, useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { isValidCategoryName, isValidPayeeName } from '@/lib/validation'
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
  const createCategory = useCreateCategory()
  const createPayee = useCreatePayee()
  const { data: user } = useUserData()
  const { state: accessState } = useAccessState()
  const [addTagOpen, setAddTagOpen] = useState(false)
  // A category is either kind, and a new one needs a kind before it exists,
  // so the picker shows one kind at a time, starting from the rule's own.
  const [categoryType, setCategoryType] = useState<CategoryType>(
    () => categories.find((c) => c.id === spec.categoryId)?.type ?? 'expense',
  )
  // The server accepts only the caller's own classifications as rule targets
  // (rules apply to the caller's imports), so rows shared in by a connection
  // are not offered, and new rows are created for the caller.
  const ownerId = user?.id
  const own = <T extends { ownerUserId: string }>(rows: T[]) => rows.filter((r) => !ownerId || r.ownerUserId === ownerId)
  const canCreate = accessState !== 'readonly'

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

  const chips = classificationChips(tags, labels, { tagId: spec.tagId || null, labelIds: spec.labelIds }, ownerId)
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
              <div className="relative">
                <SelectCard label={t('imports.rules.editor.category')}>
                  <EntitySelect aria-label={t('imports.rules.editor.category')} value={spec.categoryId || null} clearable placeholder={t('imports.rules.editor.not_set')}
                    onChange={(id) => patch({ categoryId: id ?? '' })}
                    options={own(categories).filter((c) => c.id === spec.categoryId || (!c.isArchived && c.type === categoryType)).map((c) => ({ value: c.id, label: c.name, icon: c.icon || 'pending' }))}
                    onCreate={canCreate ? (name) => createCategory.mutate(
                      { name, type: categoryType, ownerUserId: ownerId },
                      { onSuccess: (item) => patch({ categoryId: item.id }), onError: (err) => toast.error(apiErrorMessage(err)) },
                    ) : undefined}
                    createValidator={isValidCategoryName} />
                </SelectCard>
                <div className="absolute top-2 right-3 flex gap-0.5" role="radiogroup" aria-label={t('imports.rules.editor.category_type')}>
                  {(['expense', 'income'] as const).map((type) => (
                    <button key={type} type="button" role="radio" aria-checked={categoryType === type}
                      className={`rounded px-1.5 py-0.5 text-[11px] transition-colors ${categoryType === type ? 'bg-background text-foreground' : 'text-muted-foreground hover:text-foreground'}`}
                      onClick={() => setCategoryType(type)}>
                      {t(`transactions.modal.transaction_type.${type}`)}
                    </button>
                  ))}
                </div>
              </div>
              <SelectCard label={t('imports.rules.editor.payee')}>
                <EntitySelect aria-label={t('imports.rules.editor.payee')} value={spec.payeeId || null} clearable placeholder={t('imports.rules.editor.not_set')}
                  onChange={(id) => patch({ payeeId: id ?? '' })}
                  options={own(payees).filter((p) => !p.isArchived || p.id === spec.payeeId).map((p) => ({ value: p.id, label: p.name }))}
                  onCreate={canCreate ? (name) => createPayee.mutate(
                    { name, ownerUserId: ownerId },
                    { onSuccess: (item) => patch({ payeeId: item.id }), onError: (err) => toast.error(apiErrorMessage(err)) },
                  ) : undefined}
                  createValidator={isValidPayeeName} />
              </SelectCard>
              {chips.length > 0 || canCreate ? (
                <CardField label={t('accounts.page.preview_transaction_modal.tags.label')}>
                  <div className="flex items-center gap-2">
                    <ClassificationChips chips={chips} onToggle={(chip) => setSpec((s) => {
                      const next = toggleClassification({ tagId: s.tagId || null, labelIds: s.labelIds }, chip.kind, chip.id)
                      return { ...s, tagId: next.tagId ?? '', labelIds: next.labelIds }
                    })} />
                    {canCreate ? (
                      <button type="button" aria-label={t('classifications.tags.forms.tag.add_button')} title={t('common.button.add.label')}
                        className="shrink-0 text-muted-foreground hover:text-foreground" onClick={() => setAddTagOpen(true)}>
                        <Plus className="size-4" />
                      </button>
                    ) : null}
                  </div>
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
      <TagDialog open={addTagOpen} onClose={() => setAddTagOpen(false)} ownerUserId={ownerId}
        onCreated={(kind, item) => setSpec((s) => kind === 'tag'
          ? { ...s, tagId: item.id }
          : { ...s, labelIds: s.labelIds.includes(item.id) ? s.labelIds : [...s.labelIds, item.id] })} />
    </ResponsiveDialog>
  )
}
