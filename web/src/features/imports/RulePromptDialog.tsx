import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRuleMatchField, ImportRuleMatchType, ImportRuleScopeDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { useUiStore } from '@/app/uiStore'
import type { RulePromptParams } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { suggestMatchValue } from '@/lib/importMatch'
import { useApplyImportRule, useCreateImportRule, useImportRules, usePreviewImportRule, useUpdateImportRule } from './queries'

const PREVIEW_DEBOUNCE_MS = 300

type Step = 'define' | 'apply' | 'source'

export function RulePromptDialog() {
  const params = useUiStore((s) => s.rulePrompt)
  const setRulePrompt = useUiStore((s) => s.setRulePrompt)
  if (!params) {
    return null
  }
  // keyed on the link so a new prompt always starts from a fresh form state
  return <RulePrompt key={params.link.id} params={params} onDone={() => setRulePrompt(null)} />
}

// The classification the user just saved, phrased for the prompt copy.
function useDiffSummary(diff: RulePromptParams['diff']) {
  const { t } = useTranslation()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  if (diff.categoryId) {
    return t('imports.rules.prompt.changed_category', { name: categories.find((c) => c.id === diff.categoryId)?.name ?? '' })
  }
  if (diff.payeeId) {
    return t('imports.rules.prompt.changed_payee', { name: payees.find((p) => p.id === diff.payeeId)?.name ?? '' })
  }
  if (diff.tagId) {
    return t('imports.rules.prompt.changed_tag', { name: tags.find((x) => x.id === diff.tagId)?.name ?? '' })
  }
  const names = (diff.labelIds ?? []).map((id) => labels.find((l) => l.id === id)?.name ?? '').filter(Boolean)
  return t('imports.rules.prompt.changed_labels', { names: names.join(', ') })
}

function RulePrompt({ params, onDone }: { params: RulePromptParams; onDone: () => void }) {
  const { t } = useTranslation()
  const { link, diff } = params
  const { data: rules = [] } = useImportRules()
  const existing = link.appliedRuleId ? rules.find((r) => r.id === link.appliedRuleId) ?? null : null
  const summary = useDiffSummary(diff)

  const [matchField, setMatchField] = useState<ImportRuleMatchField>('external_payee')
  const [matchType, setMatchType] = useState<ImportRuleMatchType>('contains')
  const [matchValue, setMatchValue] = useState(() => suggestMatchValue(link.externalPayee))
  const [step, setStep] = useState<Step>('define')
  const [includeEdited, setIncludeEdited] = useState(false)
  const [ruleId, setRuleId] = useState<string | null>(null)

  // The rule spec: the existing rule's match (when updating) or the editable
  // one, with the classification the user just chose layered over it.
  const spec: ImportRuleSpecDto = useMemo(() => {
    const base: ImportRuleSpecDto = existing
      ? { ...existing }
      : { sourceId: '', action: 'classify', matchField, matchType, matchValue, isCaseSensitive: false, categoryId: '', payeeId: '', tagId: '', labelIds: [], priority: 0 }
    return {
      ...base,
      categoryId: diff.categoryId ?? base.categoryId,
      payeeId: diff.payeeId ?? base.payeeId,
      tagId: diff.tagId ?? base.tagId,
      labelIds: diff.labelIds ?? base.labelIds,
    }
  }, [existing, matchField, matchType, matchValue, diff])

  const runScope: ImportRuleScopeDto = useMemo(
    () => (link.runId ? { scope: 'run', runId: link.runId, scopeSourceId: link.sourceId } : { scope: 'source', runId: '', scopeSourceId: link.sourceId }),
    [link.runId, link.sourceId],
  )
  const sourceScope: ImportRuleScopeDto = useMemo(() => ({ scope: 'source', runId: '', scopeSourceId: link.sourceId }), [link.sourceId])

  const preview = usePreviewImportRule()
  const sourcePreview = usePreviewImportRule()
  const create = useCreateImportRule()
  const update = useUpdateImportRule()
  const apply = useApplyImportRule()
  const { mutate: runPreview } = preview

  useEffect(() => {
    if (step !== 'define') {
      return
    }
    const handle = setTimeout(() => runPreview({ spec, scope: runScope }), PREVIEW_DEBOUNCE_MS)
    return () => clearTimeout(handle)
  }, [spec, step, runPreview, runScope])

  const fieldLabel = matchField === 'description' ? t('imports.rules.field.description') : t('imports.rules.field.external_payee')
  const typeLabel = t(`imports.rules.type.${matchType}`)
  const matched = preview.data?.matched ?? 0
  const alreadyEdited = preview.data?.alreadyEdited ?? 0

  const save = async () => {
    try {
      // the debounced live-count preview may not have settled yet (e.g. the
      // user clicked Create right away) — refresh it alongside the save so
      // the apply step's count is never stale
      const [saved] = await Promise.all([
        existing ? update.mutateAsync({ id: existing.id, spec }) : create.mutateAsync({ spec }),
        preview.mutateAsync({ spec, scope: runScope }),
      ])
      setRuleId(saved.id)
      setStep('apply')
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const applyTo = async (scope: ImportRuleScopeDto, next: Step | null) => {
    if (!ruleId) {
      return
    }
    try {
      const result = await apply.mutateAsync({ ruleId, scope, includeEdited })
      toast.success(t('imports.rules.prompt.applied_toast', { count: result.updated }))
      if (next && link.runId) {
        sourcePreview.mutate({ spec, scope: sourceScope })
        setStep(next)
      } else {
        onDone()
      }
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const body = () => {
    if (step === 'apply') {
      return (
        <>
          <p className="text-sm">{t('imports.rules.prompt.apply_question', { count: matched })}</p>
          {alreadyEdited > 0 ? (
            <>
              <p className="text-sm text-muted-foreground">{t('imports.rules.prompt.skipped_edited', { count: alreadyEdited })}</p>
              <label className="flex items-center gap-2 text-sm">
                <Checkbox checked={includeEdited} onCheckedChange={(v) => setIncludeEdited(v === true)} aria-label={t('imports.rules.prompt.include_edited', { count: alreadyEdited })} />
                {t('imports.rules.prompt.include_edited', { count: alreadyEdited })}
              </label>
            </>
          ) : null}
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.skip_apply')}</Button>
            <Button type="button" disabled={apply.isPending} onClick={() => void applyTo(runScope, 'source')}>{t('imports.rules.prompt.apply')}</Button>
          </div>
        </>
      )
    }
    if (step === 'source') {
      return (
        <>
          <p className="text-sm">{t('imports.rules.prompt.source_question', { source: link.sourceName, count: sourcePreview.data?.matched ?? 0 })}</p>
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.done')}</Button>
            <Button type="button" disabled={apply.isPending || sourcePreview.isPending} onClick={() => void applyTo(sourceScope, null)}>{t('imports.rules.prompt.apply_source')}</Button>
          </div>
        </>
      )
    }
    return (
      <>
        <p className="text-sm">{summary}</p>
        {existing ? (
          <p className="text-sm text-muted-foreground">
            {t('imports.rules.prompt.existing_rule', { rule: `${existing.matchField === 'description' ? t('imports.rules.field.description') : t('imports.rules.field.external_payee')} ${t(`imports.rules.type.${existing.matchType}`)} ${existing.matchValue}` })}
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="grid grid-cols-2 gap-2">
              <NativeSelect aria-label={t('imports.rules.editor.field')} value={matchField} onChange={(e) => setMatchField(e.target.value as ImportRuleMatchField)}>
                <NativeSelectOption value="external_payee">{t('imports.rules.field.external_payee')}</NativeSelectOption>
                <NativeSelectOption value="description">{t('imports.rules.field.description')}</NativeSelectOption>
              </NativeSelect>
              <NativeSelect aria-label={t('imports.rules.editor.type')} value={matchType} onChange={(e) => setMatchType(e.target.value as ImportRuleMatchType)}>
                <NativeSelectOption value="contains">{t('imports.rules.type.contains')}</NativeSelectOption>
                <NativeSelectOption value="prefix">{t('imports.rules.type.prefix')}</NativeSelectOption>
                <NativeSelectOption value="exact">{t('imports.rules.type.exact')}</NativeSelectOption>
              </NativeSelect>
            </div>
            <Label htmlFor="rule-prompt-value" className="sr-only">{`${fieldLabel} ${typeLabel}`}</Label>
            <Input id="rule-prompt-value" value={matchValue} onChange={(e) => setMatchValue(e.target.value)} autoComplete="off" />
          </div>
        )}
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {preview.isPending ? t('common.app.modal.loading.data_loading') : t('imports.rules.prompt.match_count', { count: matched })}
        </p>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.not_now')}</Button>
          <Button type="button" disabled={create.isPending || update.isPending || spec.matchValue.trim() === ''} onClick={() => void save()}>
            {existing ? t('imports.rules.prompt.update_rule') : t('imports.rules.prompt.create_rule')}
          </Button>
        </div>
      </>
    )
  }

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onDone()} title={t('imports.rules.prompt.title')}>
      <div className="flex flex-col gap-3">{body()}</div>
    </ResponsiveDialog>
  )
}
