import type { CreateTransactionDto } from '@/api/dto/transaction'
import type { ImportRuleSpecDto, TransactionImportLinkDto } from '@/api/dto/imports'
import type { Id } from '@/api/types'

// Mirror of internal/imports/rules.go (NormalizeText + matchRule). The count
// the prompt shows must equal what the server later matches, so the two
// implementations must stay identical.
export function normalizeMatchText(s: string): string {
  return s.split(/\s+/).filter(Boolean).join(' ')
}

export interface MatchText {
  payee: string
  description: string
}

export function matchesRule(spec: Pick<ImportRuleSpecDto, 'matchField' | 'matchType' | 'matchValue' | 'isCaseSensitive'>, text: MatchText): boolean {
  const raw = spec.matchField === 'description' && text.description !== '' ? text.description : text.payee
  let subject = normalizeMatchText(raw)
  let value = normalizeMatchText(spec.matchValue)
  if (!spec.isCaseSensitive) {
    subject = subject.toLowerCase()
    value = value.toLowerCase()
  }
  if (value === '') {
    return false
  }
  switch (spec.matchType) {
    case 'exact':
      return subject === value
    case 'prefix':
      return subject.startsWith(value)
    case 'contains':
      return subject.includes(value)
  }
  return false
}

const REGION_CODE = /^[A-Z]{2}$/
const hasLetter = (token: string) => /\p{L}/u.test(token)

// A first guess at the merchant part of a bank/wallet payee string: drop the
// processor prefix ("SQ *"), the reference/store/date tokens, and a trailing
// region code. It is only a prefill — the user edits it with a live count.
export function suggestMatchValue(raw: string): string {
  const normalized = normalizeMatchText(raw)
  if (normalized === '') {
    return ''
  }
  const parts = normalized.split('*').map((p) => p.trim()).filter(Boolean)
  let part = parts[0] ?? normalized
  for (const candidate of parts) {
    if (candidate.replace(/[^\p{L}]/gu, '').length > part.replace(/[^\p{L}]/gu, '').length) {
      part = candidate
    }
  }
  const tokens: string[] = []
  for (const token of part.split(' ')) {
    if (!hasLetter(token) || token.startsWith('#')) {
      break
    }
    tokens.push(token)
  }
  while (tokens.length > 1 && REGION_CODE.test(tokens[tokens.length - 1])) {
    tokens.pop()
  }
  const suggestion = tokens.join(' ')
  return suggestion.length >= 3 ? suggestion : normalized
}

export interface RuleDiff {
  categoryId?: Id
  payeeId?: Id
  tagId?: Id
  labelIds?: Id[]
}

// Only a NEW non-empty value that differs from what the import applied is a
// rule-worthy change: clearing a field produces no classification to encode,
// and re-saving an already corrected transaction must not prompt again.
// Labels are set-valued and asymmetric: a rule can only ADD labels (there is
// no "unset this label" rule action), so only newly added ids are rule-worthy
// — a label REMOVAL is not something the prompt's "you added…" copy can
// truthfully offer, and re-applying an already-applied label must not prompt.
export function ruleDiff(payload: CreateTransactionDto, link: TransactionImportLinkDto): RuleDiff | null {
  const diff: RuleDiff = {}
  if (payload.categoryId && payload.categoryId !== link.appliedCategoryId) {
    diff.categoryId = payload.categoryId
  }
  if (payload.payeeId && payload.payeeId !== link.appliedPayeeId) {
    diff.payeeId = payload.payeeId
  }
  if (payload.tagId && payload.tagId !== link.appliedTagId) {
    diff.tagId = payload.tagId
  }
  const addedLabelIds = payload.labelIds.filter((id) => !link.appliedLabelIds.includes(id))
  if (addedLabelIds.length > 0) {
    diff.labelIds = addedLabelIds
  }
  return Object.keys(diff).length > 0 ? diff : null
}

export function latestImportLink(links: TransactionImportLinkDto[]): TransactionImportLinkDto | null {
  let latest: TransactionImportLinkDto | null = null
  for (const l of links) {
    if (!latest || l.importedAt > latest.importedAt) {
      latest = l
    }
  }
  return latest
}
