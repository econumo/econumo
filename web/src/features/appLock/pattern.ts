export const PATTERN_MIN_NODES = 4
export const PATTERN_NODES = 9

const ENCODED = /^p:[0-8](?:-[0-8]){3,8}$/

// The node a straight line from a to b passes over on the 3x3 grid, if any.
export function nodeBetween(a: number, b: number): number | null {
  const rowSum = Math.floor(a / 3) + Math.floor(b / 3)
  const colSum = (a % 3) + (b % 3)
  if (rowSum % 2 !== 0 || colSum % 2 !== 0) {
    return null
  }
  const mid = (rowSum / 2) * 3 + colSum / 2
  return mid === a || mid === b ? null : mid
}

// Android's rule: a node the line crosses on its way joins the pattern
// first, unless it is already part of it; a node is never used twice.
export function extendPattern(nodes: readonly number[], next: number): number[] {
  if (!Number.isInteger(next) || next < 0 || next >= PATTERN_NODES || nodes.includes(next)) {
    return [...nodes]
  }
  const out = [...nodes]
  const last = out.at(-1)
  if (last !== undefined) {
    const mid = nodeBetween(last, next)
    if (mid !== null && !out.includes(mid)) {
      out.push(mid)
    }
  }
  out.push(next)
  return out
}

export function encodePattern(nodes: readonly number[]): string {
  return `p:${nodes.join('-')}`
}

export function isValidPattern(value: string): boolean {
  if (!ENCODED.test(value)) {
    return false
  }
  const nodes = value.slice(2).split('-')
  return new Set(nodes).size === nodes.length
}

export function patternLength(value: string): number {
  return value.slice(2).split('-').length
}
