import type { CreateTransactionDto } from '@/api/dto/transaction'
import type { TransactionImportLinkDto } from '@/api/dto/imports'
import type { Id } from '@/api/types'

// Mirror of internal/imports.NormalizeText. Kept identical so any future
// client-side use of the normalized text (e.g. a prefill suggestion) agrees
// with the server's own normalization.
export function normalizeMatchText(s: string): string {
  return s.split(/\s+/).filter(Boolean).join(' ')
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

/** The pre-edit classification of the transaction being saved. */
export type RuleDiffBaseline = Pick<CreateTransactionDto, 'categoryId' | 'payeeId' | 'tagId' | 'labelIds'>

// Only a NEW non-empty value that differs from BOTH what the import applied
// and what the transaction already held is a rule-worthy change: clearing a
// field produces no classification to encode, and only THIS save's own
// correction may prompt. The pre-edit comparison is what stops a correction
// from re-prompting forever: a default apply skips the row it was created
// from (it is "edited" by construction), so its applied_* snapshot is never
// refreshed and the payload keeps diverging from it on every later save.
// Labels are set-valued and asymmetric: a rule can only ADD labels (there is
// no "unset this label" rule action), so only newly added ids are rule-worthy
// — a label REMOVAL is not something the prompt's "you added…" copy can
// truthfully offer, and re-applying an already-applied label must not prompt.
export function ruleDiff(payload: CreateTransactionDto, link: TransactionImportLinkDto, before: RuleDiffBaseline | null): RuleDiff | null {
  const diff: RuleDiff = {}
  if (payload.categoryId && payload.categoryId !== link.appliedCategoryId && payload.categoryId !== before?.categoryId) {
    diff.categoryId = payload.categoryId
  }
  if (payload.payeeId && payload.payeeId !== link.appliedPayeeId && payload.payeeId !== before?.payeeId) {
    diff.payeeId = payload.payeeId
  }
  if (payload.tagId && payload.tagId !== link.appliedTagId && payload.tagId !== before?.tagId) {
    diff.tagId = payload.tagId
  }
  const addedLabelIds = payload.labelIds.filter((id) => !link.appliedLabelIds.includes(id) && !(before?.labelIds ?? []).includes(id))
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
