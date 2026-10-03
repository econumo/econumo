import { fuzzyMatch } from './fuzzy'

export type ClassificationType = 'category' | 'payee' | 'tag' | 'label'

export function matchRank(text: string, query: string): number | null {
  const q = query.trim().toLowerCase()
  if (q === '') {
    return 0
  }
  const s = text.toLowerCase()
  if (s === q) {
    return 0
  }
  if (s.startsWith(q)) {
    return 1
  }
  if (s.includes(q)) {
    return 2
  }
  return fuzzyMatch(s, q) ? 3 : null
}

function bestRank(names: string | string[], query: string): number | null {
  let best: number | null = null
  for (const name of typeof names === 'string' ? [names] : names) {
    const rank = matchRank(name, query)
    if (rank !== null && (best === null || rank < best)) {
      best = rank
    }
  }
  return best
}

export function rankByName<T>(items: T[], getName: (item: T) => string | string[], query: string): T[] {
  if (query.trim() === '') {
    return items
  }
  return items
    .map((item, index) => ({ item, index, rank: bestRank(getName(item), query) }))
    .filter((entry): entry is { item: T; index: number; rank: number } => entry.rank !== null)
    .sort((a, b) => a.rank - b.rank || a.index - b.index)
    .map((entry) => entry.item)
}

export interface TermFields {
  /** names and free text: a term may skip characters, but only within one word */
  text: string[]
  /** amounts, dates, signs: substring only */
  exact: string[]
}

// Subsequence matching is confined to single words: across one joined
// haystack a short term like "cofe" would match "come for elections".
export function matchesTerms(fields: TermFields, query: string): boolean {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) {
    return true
  }
  const text = fields.text.map((f) => f.toLowerCase())
  const exact = fields.exact.map((f) => f.toLowerCase())
  const words = text.flatMap((f) => f.split(/\s+/).filter(Boolean))
  return terms.every(
    (term) =>
      text.some((f) => f.includes(term)) ||
      exact.some((f) => f.includes(term)) ||
      words.some((w) => fuzzyMatch(w, term)),
  )
}
