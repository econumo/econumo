import type { ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'
import type { Id } from '@/api/types'
import { similarityText } from '@/lib/importMatch'

// What the suggestion's dialog edits. A rule stored under the suggestion's id may have been edited
// on the rules page since; its other fields (tag, labels, priority, source, case) are kept. The
// action is always classify: accept refuses a skip rule, so one changed to skip there is turned back.
export function suggestionSpec(rule: ImportRuleDto, spec: ImportRuleSpecDto): ImportRuleSpecDto | null {
  if (rule.action === 'classify' && rule.matchField === spec.matchField && rule.matchType === spec.matchType &&
    rule.matchValue === spec.matchValue.trim() && rule.categoryId === spec.categoryId && rule.payeeId === spec.payeeId) {
    return null
  }
  return {
    sourceId: rule.sourceId, action: 'classify', isCaseSensitive: rule.isCaseSensitive, tagId: rule.tagId,
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

export type SuggestedPayee<T> = { kind: 'active'; payee: T } | { kind: 'archived' } | { kind: 'new' }

// What a suggestion's new payee name comes to among the owner's payees, compared as the server
// compares names: an active payee of that name is reused; one only archived means no payee, as on
// the server (the user archived it on purpose); none means a payee to create.
export function resolveSuggestedPayee<T extends { name: string; ownerUserId: Id; isArchived: 0 | 1 }>(
  payees: T[] | undefined, name: string, ownerUserId: Id | undefined,
): SuggestedPayee<T> {
  const key = similarityText(name)
  if (key === '') {
    return { kind: 'new' }
  }
  const same = (payees ?? []).filter((p) => (!ownerUserId || p.ownerUserId === ownerUserId) && similarityText(p.name) === key)
  const active = same.find((p) => !p.isArchived)
  if (active) {
    return { kind: 'active', payee: active }
  }
  return same.length > 0 ? { kind: 'archived' } : { kind: 'new' }
}
