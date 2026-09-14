import type { TFunction } from 'i18next'
import type { ImportRuleSpecDto } from '@/api/dto/imports'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'

// The wire spec only: a stored rule or a suggestion carries extra keys
// (id/createdAt/updatedAt, reason) that must not be posted back.
export function ruleSpec(r: ImportRuleSpecDto): ImportRuleSpecDto {
  const { sourceId, action, matchField, matchType, matchValue, isCaseSensitive, categoryId, payeeId, tagId, labelIds, priority } = r
  return { sourceId, action, matchField, matchType, matchValue, isCaseSensitive, categoryId, payeeId, tagId, labelIds, priority }
}

export function describeMatch(spec: Pick<ImportRuleSpecDto, 'matchField' | 'matchType' | 'matchValue'>, t: TFunction): string {
  return `${t(`imports.rules.field.${spec.matchField}`)} ${t(`imports.rules.type.${spec.matchType}`)} ${spec.matchValue}`
}

export interface TargetNames {
  category: (id: string) => string | undefined
  payee: (id: string) => string | undefined
  tag: (id: string) => string | undefined
  label: (id: string) => string | undefined
}

// "Food · Grocer · health" — every target the rule sets, in a fixed order.
export function describeTargets(spec: ImportRuleSpecDto, names: TargetNames): string {
  const parts = [
    spec.categoryId ? names.category(spec.categoryId) : undefined,
    spec.payeeId ? names.payee(spec.payeeId) : undefined,
    spec.tagId ? names.tag(spec.tagId) : undefined,
    ...spec.labelIds.map(names.label),
  ]
  return parts.filter((p): p is string => Boolean(p)).join(' · ')
}

export function useTargetNames(): TargetNames {
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  return {
    category: (id) => categories.find((c) => c.id === id)?.name,
    payee: (id) => payees.find((p) => p.id === id)?.name,
    tag: (id) => tags.find((x) => x.id === id)?.name,
    label: (id) => labels.find((l) => l.id === id)?.name,
  }
}
