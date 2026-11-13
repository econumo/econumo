import { v5 as uuidv5 } from 'uuid'
import type { ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'
import type { Id } from '@/api/types'

// Fixed forever: changing it would give every open suggestion a second rule.
const SUGGESTION_RULE_NAMESPACE = '6f1d2c4e-8b3a-4e7f-9c21-5a0b7d3e9f48'

// The rule a suggestion's Save creates has an id derived from the suggestion, so a retry after a
// reload (or on another tab) replays the rule already stored instead of creating a second one.
export function suggestionRuleId(suggestionId: Id): Id {
  return uuidv5(suggestionId, SUGGESTION_RULE_NAMESPACE)
}

export function sameRuleSpec(rule: ImportRuleDto, spec: ImportRuleSpecDto): boolean {
  const labels = (ids: Id[]) => [...ids].sort().join(',')
  return rule.sourceId === spec.sourceId && rule.action === spec.action && rule.matchField === spec.matchField &&
    rule.matchType === spec.matchType && rule.matchValue === spec.matchValue.trim() &&
    rule.isCaseSensitive === spec.isCaseSensitive && rule.categoryId === spec.categoryId &&
    rule.payeeId === spec.payeeId && rule.tagId === spec.tagId && labels(rule.labelIds) === labels(spec.labelIds) &&
    rule.priority === spec.priority
}
