import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Sparkles } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportRuleDto, ImportRuleSpecDto, ImportRuleSuggestionDto } from '@/api/dto/imports'
import { Button } from '@/components/ui/button'
import { ClassificationList, type ClassificationItem } from '@/features/classifications/ClassificationList'
import { useCategories } from '@/features/classifications/queries'
import { useAccessState } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { isAiEnabled } from '@/lib/config'
import { ImportRuleDialog, emptyRuleSpec } from './ImportRuleDialog'
import { describeMatch, describeTargets, ruleSpec, useTargetNames } from './ruleSummary'
import { useCreateImportRule, useDeleteImportRule, useImportRules, usePreviewImportRule, useSuggestImportRules, useUpdateImportRule } from './queries'

const ALL_SCOPE = { scope: 'all', runId: '', scopeSourceId: '' } as const

type Editor = { kind: 'closed' } | { kind: 'create'; initial: ImportRuleSpecDto } | { kind: 'edit'; rule: ImportRuleDto }

interface RuleItem extends ClassificationItem {
  rule: ImportRuleDto
}

export function ImportRulesPage() {
  const { t } = useTranslation()
  const { data: rules = [], isPending } = useImportRules()
  const { data: categories = [] } = useCategories()
  const names = useTargetNames()
  const update = useUpdateImportRule()
  const remove = useDeleteImportRule()
  const create = useCreateImportRule()
  const suggest = useSuggestImportRules()
  // Readonly users are refused by the server (402) before any AI call; show
  // the action disabled rather than let the click be the first they hear of it.
  const { state: accessState } = useAccessState()
  const [editor, setEditor] = useState<Editor>({ kind: 'closed' })
  const [suggestions, setSuggestions] = useState<ImportRuleSuggestionDto[] | null>(null)

  // Priority is the list position; a drop rewrites only the rows that moved.
  const reorder = async (orderedIds: string[]) => {
    const byId = new Map(rules.map((r) => [r.id, r]))
    try {
      for (const [index, id] of orderedIds.entries()) {
        const r = byId.get(id)
        if (r && r.priority !== index) {
          await update.mutateAsync({ id, spec: { ...ruleSpec(r), priority: index } })
        }
      }
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const move = ({ id, afterId }: { id: string; afterId: string | null }) => {
    const ids = rules.map((r) => r.id).filter((x) => x !== id)
    ids.splice(afterId === null ? 0 : ids.indexOf(afterId) + 1, 0, id)
    void reorder(ids)
  }

  const runSuggest = () => suggest.mutate(ALL_SCOPE, {
    onSuccess: (items) => setSuggestions(items),
    onError: (err) => toast.error(apiErrorMessage(err)),
  })

  const accept = async (s: ImportRuleSuggestionDto) => {
    try {
      await create.mutateAsync({ spec: { ...ruleSpec(s), priority: rules.length } })
      discard(s)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }
  const discard = (s: ImportRuleSuggestionDto) =>
    setSuggestions((list) => {
      const next = (list ?? []).filter((x) => x !== s)
      return next.length > 0 ? next : null
    })

  // A classify rule wears its category's icon, as the category list does; a
  // skip rule is marked as the one kind that drops rows.
  const items: RuleItem[] = rules.map((r) => ({
    id: r.id,
    name: describeMatch(r, t),
    position: r.priority,
    isArchived: 0,
    icon: r.action === 'skip' ? 'block' : categories.find((c) => c.id === r.categoryId)?.icon || 'bolt',
    rule: r,
  }))

  return (
    <>
      <ClassificationList<RuleItem>
        title={t('imports.rules.page.title')}
        info={t('imports.rules.page.intro')}
        createLabel={t('imports.rules.page.add')}
        secondaryAction={isAiEnabled() ? (
          <Button type="button" variant="ghost" size="sm" className="normal-case tracking-normal text-muted-foreground"
            disabled={suggest.isPending || accessState === 'readonly'} onClick={runSuggest}>
            <Sparkles className="size-4 text-econumo-purple" />
            {suggest.isPending ? t('imports.rules.suggest.running') : t('imports.rules.suggest.button')}
          </Button>
        ) : undefined}
        emptyText={isPending ? t('common.app.modal.loading.data_loading') : t('imports.rules.page.empty')}
        deleteTitle={t('imports.rules.page.delete_title')}
        archivedLabel=""
        items={items}
        analyticsType="import_rule"
        showIcon
        iconClassName={(item) => item.rule.action === 'skip' ? 'text-destructive' : 'text-muted-foreground'}
        meta={(item) => (
          <span className="truncate text-xs text-muted-foreground">
            {item.rule.action === 'skip' ? t('imports.rules.page.skip_hint') : describeTargets(item.rule, names)}
          </span>
        )}
        rowSwitch={() => null}
        alert={suggestions ? (
          <section aria-label={t('imports.rules.suggest.heading')} className="flex flex-col gap-2 rounded-lg border p-3">
            <h2 className="text-sm">{t('imports.rules.suggest.heading')}</h2>
            {suggestions.map((s, i) => (
              <SuggestionRow key={i} suggestion={s} targets={describeTargets(s, names)}
                onAccept={() => void accept(s)} onEdit={() => { setEditor({ kind: 'create', initial: { ...ruleSpec(s), priority: rules.length } }); discard(s) }} onDiscard={() => discard(s)} />
            ))}
          </section>
        ) : undefined}
        onCreate={() => setEditor({ kind: 'create', initial: emptyRuleSpec(rules.length) })}
        onEdit={(item) => setEditor({ kind: 'edit', rule: item.rule })}
        onDelete={(id) => remove.mutate(id, { onError: (err) => toast.error(apiErrorMessage(err)) })}
        onMove={update.isPending ? undefined : move}
      />
      {editor.kind !== 'closed' ? (
        <ImportRuleDialog key={editor.kind === 'edit' ? editor.rule.id : 'create'} open
          rule={editor.kind === 'edit' ? editor.rule : undefined}
          initial={editor.kind === 'create' ? editor.initial : undefined}
          onClose={() => setEditor({ kind: 'closed' })} />
      ) : null}
    </>
  )
}

function SuggestionRow({ suggestion, targets, onAccept, onEdit, onDiscard }: { suggestion: ImportRuleSuggestionDto; targets: string; onAccept: () => void; onEdit: () => void; onDiscard: () => void }) {
  const { t } = useTranslation()
  const preview = usePreviewImportRule()
  const { mutate: runPreview } = preview
  useEffect(() => {
    runPreview({ spec: suggestion, scope: ALL_SCOPE })
  }, [suggestion, runPreview])
  return (
    <div className="flex flex-col gap-1 rounded-lg bg-econumo-card px-3 py-2.5 text-sm">
      <span>{describeMatch(suggestion, t)}</span>
      <span className="text-xs text-muted-foreground">{targets}</span>
      <span className="text-xs text-muted-foreground">{suggestion.reason}</span>
      <span className="text-xs text-muted-foreground" aria-live="polite">
        {preview.isPending || !preview.data ? t('common.app.modal.loading.data_loading') : t('imports.rules.editor.match_count', { count: preview.data.matched })}
      </span>
      <div className="flex gap-2 pt-1">
        <Button type="button" size="sm" onClick={onAccept}>{t('imports.rules.suggest.accept')}</Button>
        <Button type="button" size="sm" variant="secondary" onClick={onEdit}>{t('imports.rules.page.edit')}</Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDiscard}>{t('imports.rules.suggest.discard')}</Button>
      </div>
    </div>
  )
}
