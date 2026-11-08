// create-account and create-category refuse names outside 3-64 characters
// (code points, as the server counts them)
export const NAME_MIN = 3
export const NAME_MAX = 64

export function nameLengthValid(name: string): boolean {
  const n = [...name.trim()].length
  return n >= NAME_MIN && n <= NAME_MAX
}
