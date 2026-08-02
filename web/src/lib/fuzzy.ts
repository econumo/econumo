// Port of the Vue CurrencySelect: subsequence ("fuzzy") match.
export function fuzzyMatch(str: string, pattern: string): boolean {
  const p = pattern.toLowerCase()
  const s = str.toLowerCase()
  let patternIdx = 0
  let strIdx = 0
  while (patternIdx < p.length && strIdx < s.length) {
    if (p[patternIdx] === s[strIdx]) {
      patternIdx++
    }
    strIdx++
  }
  return patternIdx === p.length
}
