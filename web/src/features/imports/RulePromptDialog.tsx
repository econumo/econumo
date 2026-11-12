import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type {
  ImportRuleMatchField,
  ImportRuleMatchType,
  ImportRuleScopeDto,
  ImportRuleSpecDto,
  ImportSuggestionDto,
  PreviewImportRuleDto,
  TransactionImportLinkDto,
} from '@/api/dto/imports'
import type { RuleDiff } from '@/lib/importMatch'
import { useUiStore } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { Checkbox } from '@/components/ui/checkbox'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { SelectCard } from '@/features/transactions/SelectCard'
import { useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { pluralPick } from '@/lib/plural'
import { suggestMatchValue } from '@/lib/importMatch'
import { isAiEnabled } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import {
  useAcceptImportSuggestion,
  useApplyImportRule,
  useCreateImportRule,
  useImportRules,
  useSuggestMatchText,
  usePreviewImportRule,
  useUpdateImportRule,
} from './queries'
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
  const onDone = () => setRulePrompt(null)
  // keyed so a new prompt always starts from a fresh form state
  return params.kind === 'edit'
    ? <RulePrompt key={`edit-${params.link.id}`} link={params.link} diff={params.diff} onDone={onDone} />
    : <SuggestionPrompt key={`suggestion-${params.suggestion.id}`} suggestion={params.suggestion} onDone={onDone} />
}

// The classification the user just saved, phrased for the prompt copy.
function useDiffSummary(diff: RuleDiff) {
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

// Shown at whichever step is about to apply, scoped to THAT step's own
// already-edited count — the run-scoped and import-wide applies are
// separate actions, so each gets its own control.
function IncludeEditedControl({ count, checked, onChange }: { count: number; checked: boolean; onChange: (v: boolean) => void }) {
  const { t } = useTranslation()
  if (count <= 0) {
    return null
  }
  return (
    <>
      <p className="text-sm text-muted-foreground">{t('imports.rules.prompt.skipped_edited', { count })}</p>
      <label className="flex items-center gap-2 text-sm">
        <Checkbox checked={checked} onCheckedChange={(v) => onChange(v === true)} aria-label={t('imports.rules.prompt.include_edited', { count })} />
        {t('imports.rules.prompt.include_edited', { count })}
      </label>
    </>
  )
}

// The import-wide step. Its include-edited choice starts unchecked: the
// run-scoped choice must not silently carry over to this wider action.
function AllStep({ ruleId, preview, loading, onDone }: { ruleId: string | null; preview: PreviewImportRuleDto | undefined; loading: boolean; onDone: () => void }) {
  const { t } = useTranslation()
  const online = useOnline()
  const apply = useApplyImportRule()
  const [includeEdited, setIncludeEdited] = useState(false)
  const applyAll = async () => {
    if (!ruleId) {
      return
    }
    try {
      const result = await apply.mutateAsync({ ruleId, scope: ALL_SCOPE, includeEdited })
      toast.success(t('imports.rules.prompt.applied_toast', { count: result.updated }))
      onDone()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }
  return (
    <>
      <p className="text-sm">
        {loading || preview === undefined
          ? t('common.app.modal.loading.data_loading')
          : t('imports.rules.prompt.all_question', { count: preview.matched })}
      </p>
      <IncludeEditedControl count={preview?.alreadyEdited ?? 0} checked={includeEdited} onChange={setIncludeEdited} />
      <div className={dialogActionsClass}>
        <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.done')}</Button>
        <Button type="button" disabled={apply.isPending || loading || !online} onClick={() => void applyAll()}>{t('imports.rules.prompt.apply_all')}</Button>
      </div>
    </>
  )
}

function RulePrompt({ link, diff, onDone }: { link: TransactionImportLinkDto; diff: RuleDiff; onDone: () => void }) {
  const { t } = useTranslation()
  const online = useOnline()
  const { data: rules = [] } = useImportRules()
  const existing = link.appliedRuleId ? rules.find((r) => r.id === link.appliedRuleId) ?? null : null
  const summary = useDiffSummary(diff)

  const [matchField, setMatchField] = useState<ImportRuleMatchField>('external_payee')
  const [matchType, setMatchType] = useState<ImportRuleMatchType>('contains')
  const [matchValue, setMatchValue] = useState(() => suggestMatchValue(link.externalPayee))
  const [step, setStep] = useState<Step>('define')
  const [includeEdited, setIncludeEdited] = useState(false)
  const [ruleId, setRuleId] = useState<string | null>(null)
  const [touched, setTouched] = useState(false)
  const [aiPick, setAiPick] = useState<{ field: ImportRuleMatchField; text: string } | null>(null)
  // covers the save AND the count refreshes after it, so a second tap cannot create a second rule
  const [busy, setBusy] = useState(false)

  const { data: user } = useUserData()
  // a lapsed-trial user's POST would only answer 402; wait until the access level is known
  const aiOn = !existing && isAiEnabled() && online && user !== undefined && user.accessLevel !== 'readonly'
  const aiMatch = useSuggestMatchText(link.id, aiOn)
  const aiAnswer = aiOn ? aiMatch.data : undefined
  useEffect(() => {
    if (!aiAnswer || aiAnswer.matchText === '' || touched) {
      return
    }
    const field = aiAnswer.matchField || 'external_payee'
    setMatchField(field)
    setMatchValue(aiAnswer.matchText)
    setAiPick({ field, text: aiAnswer.matchText })
  }, [aiAnswer, touched])

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

  // The import-wide step: shown only when the rule would change something
  // there, else the prompt is finished. A failed count shows the step anyway
  // (its own loading/zero text), since the answer is unknown.
  const goToAll = async () => {
    if (!link.runId) {
      onDone()
      return
    }
    const all = await allPreview.mutateAsync({ spec, scope: ALL_SCOPE }).catch(() => undefined)
    if (all?.matched === 0) {
      onDone()
      return
    }
    setStep('all')
  }

  const save = async () => {
    setBusy(true)
    try {
      // the create/update and the count refresh are independent outcomes: if
      // the rule is saved but the refresh then fails, we must still record
      // ruleId and advance — otherwise a second click on "Create rule" would
      // create a SECOND rule (this call site passes no id, so the hook mints a
      // fresh one), which is exactly the near-duplicate stacking appliedRuleId
      // exists to avoid.
      const saved = existing ? await update.mutateAsync({ id: existing.id, spec }) : await create.mutateAsync({ spec })
      setRuleId(saved.id)
      if (!existing && aiPick !== null && matchValue === aiPick.text && matchField === aiPick.field) {
        trackEvent(METRICS.IMPORT_RULE_AI_MATCH_USED)
      }
      // the debounced live-count preview may not have settled yet (e.g. the
      // user clicked Create right away) — refresh it so the apply step's
      // count is never stale; a refresh failure just leaves the count at 0,
      // it must not undo the save that already succeeded
      const refreshed = await preview.mutateAsync({ spec, scope: runScope }).catch(() => undefined)
      if (refreshed?.matched === 0) {
        // the run step (or, with no run, the only step) has nothing to apply
        await goToAll()
        return
      }
      setStep('apply')
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const applyToRun = async () => {
    if (!ruleId) {
      return
    }
    try {
      const result = await apply.mutateAsync({ ruleId, scope: runScope, includeEdited })
      toast.success(t('imports.rules.prompt.applied_toast', { count: result.updated }))
      await goToAll()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const body = () => {
    if (step === 'apply') {
      return (
        <>
          <p className="text-sm">{t(link.runId ? 'imports.rules.prompt.apply_question' : 'imports.rules.prompt.apply_all_question', { count: matched })}</p>
          <IncludeEditedControl count={alreadyEdited} checked={includeEdited} onChange={setIncludeEdited} />
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={onDone}>{t('imports.rules.prompt.skip_apply')}</Button>
            {/* stays disabled while goToAll waits for the import-wide count */}
            <Button type="button" disabled={apply.isPending || allPreview.isPending || !online} onClick={() => void applyToRun()}>{t('imports.rules.prompt.apply')}</Button>
          </div>
        </>
      )
    }
    if (step === 'all') {
      return <AllStep ruleId={ruleId} preview={allPreview.data} loading={allPreview.isPending} onDone={onDone} />
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
              setTouched(true)
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
          <Button type="button" disabled={busy || create.isPending || update.isPending || !online || spec.matchValue.trim() === ''} onClick={() => void save()}>
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

// A rule the server proposed for a group of imported transactions: the match
// and the classification arrive prefilled and stay editable. Saving creates
// the rule and accepts the suggestion, which fills the group's empty fields.
function SuggestionPrompt({ suggestion, onDone }: { suggestion: ImportSuggestionDto; onDone: () => void }) {
  const { t, i18n } = useTranslation()
  const online = useOnline()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: user } = useUserData()

  const [matchField, setMatchField] = useState<ImportRuleMatchField>(suggestion.matchField)
  const [matchType, setMatchType] = useState<ImportRuleMatchType>('contains')
  const [matchValue, setMatchValue] = useState(suggestion.matchText)
  const [categoryId, setCategoryId] = useState(suggestion.categoryId)
  const [payeeId, setPayeeId] = useState(suggestion.payeeId)
  const [step, setStep] = useState<'define' | 'all'>('define')
  const [ruleId, setRuleId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const spec: ImportRuleSpecDto = useMemo(() => ({
    sourceId: '', action: 'classify', matchField, matchType, matchValue, isCaseSensitive: false,
    categoryId, payeeId, tagId: '', labelIds: [], priority: 0,
  }), [matchField, matchType, matchValue, categoryId, payeeId])
  // the server refuses (and will not preview) a classify rule that sets nothing
  const complete = spec.matchValue.trim() !== '' && (categoryId !== '' || payeeId !== '')

  const preview = usePreviewImportRule()
  const allPreview = usePreviewImportRule()
  const create = useCreateImportRule()
  const update = useUpdateImportRule()
  const accept = useAcceptImportSuggestion()
  const setSuggestionRule = useUiStore((s) => s.setSuggestionRule)
  const { mutate: runPreview } = preview

  useEffect(() => {
    if (step !== 'define' || !complete) {
      return
    }
    const handle = setTimeout(() => runPreview({ spec, scope: ALL_SCOPE }), PREVIEW_DEBOUNCE_MS)
    return () => clearTimeout(handle)
  }, [spec, step, complete, runPreview])

  const ownerId = user?.id
  const own = <T extends { ownerUserId: string }>(rows: T[]) => rows.filter((r) => !ownerId || r.ownerUserId === ownerId)
  const fieldLabel = matchField === 'description' ? t('imports.rules.field.description') : t('imports.rules.field.external_payee')
  const typeLabel = t(`imports.rules.type.${matchType}`)

  const save = async () => {
    setBusy(true)
    try {
      // The rule created by an earlier Save whose accept failed is kept in the
      // store (it outlives Cancel and a reopen) and reused, updated to the
      // current spec, so one suggestion never leaves two rules behind.
      const saved = useUiStore.getState().suggestionRules[suggestion.id]
      let id: string
      if (!saved) {
        id = (await create.mutateAsync({ spec })).id
        setSuggestionRule(suggestion.id, { ruleId: id, spec })
      } else {
        id = saved.ruleId
        if (JSON.stringify(saved.spec) !== JSON.stringify(spec)) {
          await update.mutateAsync({ id, spec })
          setSuggestionRule(suggestion.id, { ruleId: id, spec })
        }
      }
      setRuleId(id)
      const accepted = await accept.mutateAsync({
        id: suggestion.id,
        ruleId: id,
        changes: {
          changedMatch: matchField !== suggestion.matchField || matchType !== 'contains' || matchValue !== suggestion.matchText,
          changedCategory: categoryId !== suggestion.categoryId,
          changedPayee: payeeId !== suggestion.payeeId,
        },
      })
      setSuggestionRule(suggestion.id, null)
      // the accept filled the suggestion's own rows, so this counts the others
      const all = await allPreview.mutateAsync({ spec, scope: ALL_SCOPE }).catch(() => undefined)
      if (all !== undefined && all.matched > 0) {
        setStep('all')
        return
      }
      toast.success(t('imports.rules.prompt.applied_toast', { count: accepted.updated }))
      onDone()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  if (step === 'all') {
    return (
      <ResponsiveDialog open onOpenChange={(o) => !o && onDone()} title={t('imports.rules.prompt.title')}>
        <div className="flex flex-col gap-3">
          <AllStep ruleId={ruleId} preview={allPreview.data} loading={allPreview.isPending} onDone={onDone} />
        </div>
      </ResponsiveDialog>
    )
  }

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onDone()} title={t('imports.rules.prompt.title')}>
      <div className="flex flex-col gap-3">
        <p className="text-sm">{pluralPick(t('imports.suggestions.prompt_summary'), suggestion.transactions.length, i18n.language)}</p>
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
        <SelectCard label={t('imports.rules.editor.category')}>
          <EntitySelect aria-label={t('imports.rules.editor.category')} value={categoryId || null} clearable placeholder={t('imports.rules.editor.not_set')}
            onChange={(id) => setCategoryId(id ?? '')}
            options={own(categories).filter((c) => (!c.isArchived || c.id === categoryId) && (!suggestion.type || c.type === suggestion.type)).map((c) => ({ value: c.id, label: c.name, icon: c.icon || 'pending' }))} />
        </SelectCard>
        <SelectCard label={t('imports.rules.editor.payee')}>
          <EntitySelect aria-label={t('imports.rules.editor.payee')} value={payeeId || null} clearable placeholder={t('imports.rules.editor.not_set')}
            onChange={(id) => setPayeeId(id ?? '')}
            options={own(payees).filter((p) => !p.isArchived || p.id === payeeId).map((p) => ({ value: p.id, label: p.name }))} />
        </SelectCard>
        <p className="min-h-4 px-1 text-xs text-muted-foreground" aria-live="polite">
          {!complete
            ? ''
            : preview.data === undefined || preview.isPending
              ? t('common.app.modal.loading.data_loading')
              : t('imports.rules.editor.match_count', { count: preview.data.matched })}
        </p>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onDone}>{t('common.button.cancel.label')}</Button>
          <Button type="button" disabled={busy || !online || !complete} onClick={() => void save()}>{t('common.button.save.label')}</Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
