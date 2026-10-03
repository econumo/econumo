import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GripVertical } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportRuleDto, ImportRuleSpecDto, ImportRuleSuggestionDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { SortableList } from '@/components/SortableList'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { isAiEnabled } from '@/lib/config'
import { ImportRuleDialog, emptyRuleSpec } from './ImportRuleDialog'
import { describeMatch, describeTargets, ruleSpec, useTargetNames } from './ruleSummary'
import { useCreateImportRule, useDeleteImportRule, useImportRules, usePreviewImportRule, useSuggestImportRules, useUpdateImportRule } from './queries'

const ALL_SCOPE = { scope: 'all', runId: '', scopeSourceId: '' } as const

type Editor = { kind: 'closed' } | { kind: 'create'; initial: ImportRuleSpecDto } | { kind: 'edit'; rule: ImportRuleDto }

export function ImportRulesPage() {
  const { t } = useTranslation()
  const { data: rules = [], isPending } = useImportRules()
  const names = useTargetNames()
  const update = useUpdateImportRule()
  const remove = useDeleteImportRule()
  const create = useCreateImportRule()
  const suggest = useSuggestImportRules()
  const [editor, setEditor] = useState<Editor>({ kind: 'closed' })
  const [deleting, setDeleting] = useState<ImportRuleDto | null>(null)
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

  const confirmDelete = async () => {
    if (!deleting) {
      return
    }
    try {
      await remove.mutateAsync(deleting.id)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setDeleting(null)
    }
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

  return (
    <SettingsShell title={t('imports.rules.page.title')} backTo={RouterPage.SETTINGS_DATA}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-3">
        <InfoBox>{t('imports.rules.page.intro')}</InfoBox>
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={() => setEditor({ kind: 'create', initial: emptyRuleSpec(rules.length) })}>{t('imports.rules.page.add')}</Button>
          {isAiEnabled() ? (
            <Button type="button" variant="secondary" disabled={suggest.isPending} onClick={runSuggest}>
              {suggest.isPending ? t('imports.rules.suggest.running') : t('imports.rules.suggest.button')}
            </Button>
          ) : null}
        </div>
        {suggestions ? (
          <section aria-label={t('imports.rules.suggest.heading')} className="flex flex-col gap-2 rounded-lg border p-3">
            <h2 className="text-sm font-medium">{t('imports.rules.suggest.heading')}</h2>
            {suggestions.map((s, i) => (
              <SuggestionRow key={i} suggestion={s} targets={describeTargets(s, names)}
                onAccept={() => void accept(s)} onEdit={() => { setEditor({ kind: 'create', initial: { ...ruleSpec(s), priority: rules.length } }); discard(s) }} onDiscard={() => discard(s)} />
            ))}
          </section>
        ) : null}
        {isPending ? (
          <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p>
        ) : rules.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('imports.rules.page.empty')}</p>
        ) : (
          <SortableList items={rules} onReorder={(ids) => void reorder(ids)} disabled={update.isPending} renderItem={(r, handle) => (
            <div className="flex items-center gap-2 rounded-lg bg-econumo-card px-3 py-2.5 text-sm">
              <button type="button" aria-label={t('imports.rules.page.reorder')} className="cursor-grab touch-none text-muted-foreground" {...handle.attributes} {...handle.listeners}>
                <GripVertical className="size-4" />
              </button>
              <div className="flex min-w-0 flex-1 flex-col">
                <span className="truncate">{describeMatch(r, t)}</span>
                {r.action === 'skip'
                  ? <span className="text-xs text-muted-foreground">{t('imports.rules.page.skip_hint')}</span>
                  : <span className="truncate text-xs text-muted-foreground">{describeTargets(r, names)}</span>}
              </div>
              <Badge variant={r.action === 'skip' ? 'destructive' : 'secondary'}>{t(`imports.rules.action.${r.action}`)}</Badge>
              <Button type="button" variant="ghost" size="sm" onClick={() => setEditor({ kind: 'edit', rule: r })}>{t('imports.rules.page.edit')}</Button>
              <Button type="button" variant="ghost" size="sm" onClick={() => setDeleting(r)}>{t('imports.rules.page.delete')}</Button>
            </div>
          )} />
        )}
      </div>
      {editor.kind !== 'closed' ? (
        <ImportRuleDialog key={editor.kind === 'edit' ? editor.rule.id : 'create'} open
          rule={editor.kind === 'edit' ? editor.rule : undefined}
          initial={editor.kind === 'create' ? editor.initial : undefined}
          onClose={() => setEditor({ kind: 'closed' })} />
      ) : null}
      <ConfirmDialog open={deleting !== null} onClose={() => setDeleting(null)} onConfirm={() => void confirmDelete()}
        title={t('imports.rules.page.delete_title')} question={deleting ? describeMatch(deleting, t) : ''}
        confirmLabel={t('imports.rules.page.delete_confirm')} cancelLabel={t('common.button.cancel.label')} destructive />
    </SettingsShell>
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
