import type { ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'

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

// The spec of a stored rule, so a preview describes what apply-rule will run.
export function ruleSpec(rule: ImportRuleDto): ImportRuleSpecDto {
  return {
    sourceId: rule.sourceId, action: rule.action, matchField: rule.matchField, matchType: rule.matchType, matchValue: rule.matchValue,
    isCaseSensitive: rule.isCaseSensitive, categoryId: rule.categoryId, payeeId: rule.payeeId, tagId: rule.tagId,
    labelIds: rule.labelIds, priority: rule.priority,
  }
}
