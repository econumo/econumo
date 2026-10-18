import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRuleMatchField, ImportRuleMatchType, ImportRuleScopeDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { useUiStore } from '@/app/uiStore'
import type { RulePromptParams } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { suggestMatchValue } from '@/lib/importMatch'
import { useApplyImportRule, useCreateImportRule, useImportRules, usePreviewImportRule, useUpdateImportRule } from './queries'
import { RuleCondition } from './RuleCondition'
import { describeMatch } from './ruleSummary'

const PREVIEW_DEBOUNCE_MS = 300
// mirrors model.MaxImportRuleLabels — the server rejects a longer set
const MAX_RULE_LABELS = 10

const ALL_SCOPE: ImportRuleScopeDto = { scope: 'all', runId: '', scopeSourceId: '' }

type Step = 'define' | 'apply' | 'all'

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
      // labels are the one asymmetric field: the diff carries only the ADDED
      // ids while update-rule REPLACES the rule's whole label set, so they
      // must be unioned — assigning the diff would silently drop every label
      // the rule already sets. Capped like the server's own limit.
      labelIds: diff.labelIds ? [...new Set([...base.labelIds, ...diff.labelIds])].slice(0, MAX_RULE_LABELS) : base.labelIds,
    }
  }, [existing, matchField, matchType, matchValue, diff])

  const runScope: ImportRuleScopeDto = useMemo(
    () => (link.runId ? { scope: 'run', runId: link.runId, scopeSourceId: link.sourceId } : ALL_SCOPE),
    [link.runId, link.sourceId],
  )


  const preview = usePreviewImportRule()
  const allPreview = usePreviewImportRule()
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
      // the create/update and the count refresh are independent outcomes: if
      // the rule is saved but the refresh then fails, we must still record
      // ruleId and advance — otherwise a second click on "Create rule" would
      // create a SECOND rule (this call site passes no id, so the hook mints a
      // fresh one), which is exactly the near-duplicate stacking appliedRuleId
      // exists to avoid.
      const saved = existing ? await update.mutateAsync({ id: existing.id, spec }) : await create.mutateAsync({ spec })
      setRuleId(saved.id)
      // the debounced live-count preview may not have settled yet (e.g. the
      // user clicked Create right away) — refresh it so the apply step's
      // count is never stale; a refresh failure just leaves the count at 0,
      // it must not undo the save that already succeeded
      await preview.mutateAsync({ spec, scope: runScope }).catch(() => {})
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
        // applying to every import is a separate, wider-reaching action — the
        // run-scoped "include edited" choice must not silently carry over to it
        setIncludeEdited(false)
        allPreview.mutate({ spec, scope: ALL_SCOPE })
        setStep(next)
      } else {
        onDone()
      }
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  // Shown at whichever step is about to apply, scoped to THAT step's own
  // already-edited count — the run-scoped and import-wide applies are
  // separate actions (state reset in applyTo), so each gets its own control.
  const includeEditedControl = (count: number) =>
    count > 0 ? (
      <>
        <p className="text-sm text-muted-foreground">{t('imports.rules.prompt.skipped_edited', { count })}</p>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={includeEdited} onCheckedChange={(v) => setIncludeEdited(v === true)} aria-label={t('imports.rules.prompt.include_edited', { count })} />
          {t('imports.rules.prompt.include_edited', { count })}
        </label>
      </>
    ) : null

  const body = () => {
    if (step === 'apply') {
      return (
        <>
          <p className="text-sm">{t(link.runId ? 'imports.rules.prompt.apply_question' : 'imports.rules.prompt.apply_all_question', { count: matched })}</p>
          {includeEditedControl(alreadyEdited)}
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.skip_apply')}</Button>
            <Button type="button" disabled={apply.isPending} onClick={() => void applyTo(runScope, 'all')}>{t('imports.rules.prompt.apply')}</Button>
          </div>
        </>
      )
    }
    if (step === 'all') {
      const allLoading = allPreview.data === undefined || allPreview.isPending
      return (
        <>
          <p className="text-sm">
            {allLoading
              ? t('common.app.modal.loading.data_loading')
              : t('imports.rules.prompt.all_question', { count: allPreview.data?.matched ?? 0 })}
          </p>
          {includeEditedControl(allPreview.data?.alreadyEdited ?? 0)}
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.done')}</Button>
            <Button type="button" disabled={apply.isPending || allPreview.isPending} onClick={() => void applyTo(ALL_SCOPE, null)}>{t('imports.rules.prompt.apply_all')}</Button>
          </div>
        </>
      )
    }
    return (
      <>
        <p className="text-sm">{summary}</p>
        {existing ? (
          <p className="text-sm text-muted-foreground">
            {t('imports.rules.prompt.existing_rule', { rule: describeMatch(existing, t) })}
          </p>
        ) : (
          <RuleCondition id="rule-prompt-value" matchField={matchField} matchType={matchType} matchValue={matchValue}
            valueLabel={`${fieldLabel} ${typeLabel}`}
            onChange={(p) => {
              if (p.matchField) {
                setMatchField(p.matchField)
              }
              if (p.matchType) {
                setMatchType(p.matchType)
              }
              if (p.matchValue !== undefined) {
                setMatchValue(p.matchValue)
              }
            }} />
        )}
        <p className="px-1 text-xs text-muted-foreground" aria-live="polite">
          {preview.data === undefined || preview.isPending
            ? t('common.app.modal.loading.data_loading')
            : t('imports.rules.prompt.match_count', { count: matched })}
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
