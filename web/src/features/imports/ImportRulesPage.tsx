import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { ClassificationList, type ClassificationItem } from '@/features/classifications/ClassificationList'
import { useCategories } from '@/features/classifications/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { ImportRuleDialog, emptyRuleSpec } from './ImportRuleDialog'
import { describeMatch, describeTargets, ruleSpec, useTargetNames } from './ruleSummary'
import { useDeleteImportRule, useImportRules, useUpdateImportRule } from './queries'

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
  const [editor, setEditor] = useState<Editor>({ kind: 'closed' })

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
