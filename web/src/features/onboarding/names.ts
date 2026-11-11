// create-account and create-category refuse names outside 3-64 characters
// (code points, as the server counts them)
export const NAME_MIN = 3
export const NAME_MAX = 64

export function nameLengthValid(name: string): boolean {
  const n = [...name.trim()].length
  return n >= NAME_MIN && n <= NAME_MAX
}

// An untouched default carries its catalogue key, so it reads (and is created) in the current UI language.
export function labelled<T extends { name: string; labelKey?: string }>(row: T, t: (key: string) => string): T {
  return row.labelKey ? { ...row, name: t(row.labelKey) } : row
}
