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

// What the suggestion's dialog edits. A rule stored under the suggestion's id may have been edited
// on the rules page since; its other fields (tag, labels, priority, source, case, action) are kept.
export function suggestionSpec(rule: ImportRuleDto, spec: ImportRuleSpecDto): ImportRuleSpecDto | null {
  if (rule.matchField === spec.matchField && rule.matchType === spec.matchType && rule.matchValue === spec.matchValue.trim() &&
    rule.categoryId === spec.categoryId && rule.payeeId === spec.payeeId) {
    return null
  }
  return {
    sourceId: rule.sourceId, action: rule.action, isCaseSensitive: rule.isCaseSensitive, tagId: rule.tagId,
    labelIds: rule.labelIds, priority: rule.priority,
    matchField: spec.matchField, matchType: spec.matchType, matchValue: spec.matchValue, categoryId: spec.categoryId, payeeId: spec.payeeId,
  }
}
